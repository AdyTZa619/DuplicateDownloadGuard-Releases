package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

type corpusCaseV90 struct {
	Label, Remote, Local, Expected, RemoteName, LocalName string
}

type corpusByteWriterV90 struct {
	http.ResponseWriter
	bytes *atomic.Int64
}

func (w corpusByteWriterV90) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	w.bytes.Add(int64(n))
	return n, err
}

// This opt-in integration suite runs real HTTP, filesystem/index persistence,
// ffprobe, FFmpeg and encoded media. Unit tests never pretend to run this suite.
func TestDuplicateCorpusV90(t *testing.T) {
	root := os.Getenv("DDG_MEDIA_CORPUS")
	if root == "" {
		t.Skip("set DDG_MEDIA_CORPUS to generated corpus directory")
	}
	b, err := os.ReadFile(filepath.Join(root, "cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cases []corpusCaseV90
	if err := json.Unmarshal(b, &cases); err != nil {
		t.Fatal(err)
	}
	var transferred atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.FileServer(http.Dir(root)).ServeHTTP(corpusByteWriterV90{w, &transferred}, r)
	}))
	defer server.Close()
	rows := make([]map[string]any, 0, len(cases))
	for _, tc := range cases {
		t.Run(tc.Label, func(t *testing.T) {
			collection, download := t.TempDir(), t.TempDir()
			local := filepath.Join(collection, filepath.FromSlash(tc.LocalName))
			if err := os.MkdirAll(filepath.Dir(local), 0755); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(filepath.Join(root, tc.Local))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(local, data, 0644); err != nil {
				t.Fatal(err)
			}
			st, err := os.Stat(filepath.Join(root, tc.Remote))
			if err != nil {
				t.Fatal(err)
			}
			remote := RemoteItem{Name: tc.RemoteName, Size: st.Size(), Source: "HTTP", DirectURL: server.URL + "/" + tc.Remote}
			a := guardTestApp(t, collection, download, remote)
			start := time.Now()
			a.runIndex(context.Background(), []string{collection}, "", 0)
			indexMS := float64(time.Since(start).Microseconds()) / 1000
			before := transferred.Load()
			start = time.Now()
			cold, err := a.runDownloadGuard(context.Background(), a.results, download, guardModeSmart)
			if err != nil {
				t.Fatal(err)
			}
			coldMS := float64(time.Since(start).Microseconds()) / 1000
			coldBytes := transferred.Load() - before
			before = transferred.Load()
			start = time.Now()
			warm, err := a.runDownloadGuard(context.Background(), a.results, download, guardModeSmart)
			if err != nil {
				t.Fatal(err)
			}
			warmMS := float64(time.Since(start).Microseconds()) / 1000
			d := warm.Decisions[0]
			passed := false
			switch tc.Expected {
			case "exact":
				passed = d.Exact && d.LocalPath == local
			case "related":
				passed = !d.Exact && d.LocalPath == local && d.Similarity >= 85
			case "different":
				passed = !d.Exact && d.Verdict != guardDuplicate && d.Similarity < 85
			}
			row := map[string]any{"case": tc.Label, "expected": tc.Expected, "passed": passed,
				"indexMS": indexMS, "coldMS": coldMS, "warmMS": warmMS, "coldBytes": coldBytes,
				"warmBytes": transferred.Load() - before, "cold": cold.Decisions[0], "warm": d}
			rows = append(rows, row)
			encoded, _ := json.Marshal(row)
			t.Log(string(encoded))
			if !passed && os.Getenv("DDG_CORPUS_STRICT") == "1" {
				t.Errorf("corpus acceptance failed: %s", tc.Label)
			}
		})
	}
	if path := os.Getenv("DDG_CORPUS_REPORT"); path != "" {
		b, err := json.MarshalIndent(rows, "", "  ")
		if err == nil {
			err = os.WriteFile(path, b, 0644)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
}
