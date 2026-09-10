package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func sourceDetectorFixtureV90(t *testing.T, data []byte) (*App, string, string) {
	t.Helper()
	collection := t.TempDir()
	local := filepath.Join(collection, "original.bin")
	if err := os.WriteFile(local, data, 0600); err != nil {
		t.Fatal(err)
	}
	a := guardTestApp(t, collection, t.TempDir(), RemoteItem{})
	a.runIndex(context.Background(), []string{collection}, "", 0)
	return a, collection, local
}

func TestPrimaryComparisonPublishesCryptographicEvidenceV90(t *testing.T) {
	data := bytes.Repeat([]byte("source-pipeline"), 20)
	a, _, local := sourceDetectorFixtureV90(t, data)
	a.compareRemote(context.Background(), []RemoteItem{{Name: "renamed.bin", Source: "HTTP", Size: int64(len(data)), HashType: "sha256", Hash: fmt.Sprintf("%x", sha256.Sum256(data))}}, "balanced")
	row, ok := a.resultByID(1)
	if !ok || row.LocalPath != local || row.Detector == nil || row.Detector.Classification != "EXACT" || row.Detector.Score == nil || *row.Detector.Score != 100 {
		t.Fatalf("primary comparison did not publish detector evidence: %#v", row)
	}
}

func waitSourceDetectorV90(t *testing.T, a *App) {
	t.Helper()
	a.duplicateScan.mu.Lock()
	done := a.duplicateScan.done
	a.duplicateScan.mu.Unlock()
	if done == nil {
		t.Fatal("source scan did not schedule detector")
	}
	select {
	case <-done:
	case <-time.After(120 * time.Second):
		a.cancelDuplicateScanV90()
		t.Fatal("detector did not finish")
	}
}
func sourceScanHTTPV90(t *testing.T, a *App, target string) Result {
	t.Helper()
	mux := http.NewServeMux()
	a.registerDuplicateRoutesV90(mux)
	mux.HandleFunc("/api/url/scan", a.handleURLScan)
	mux.HandleFunc("/api/results", a.handleResults)
	body, _ := json.Marshal(map[string]string{"url": target, "mode": "balanced"})
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest("POST", "/api/url/scan", bytes.NewReader(body)))
	if response.Code != 200 {
		t.Fatalf("source scan: %d %s", response.Code, response.Body.String())
	}
	waitSourceDetectorV90(t, a)
	response = httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest("GET", "/api/results", nil))
	if response.Code != 200 || !bytes.Contains(response.Body.Bytes(), []byte(`"detector"`)) {
		t.Fatalf("results API missing evidence: %s", response.Body.String())
	}
	row, _ := a.resultByID(1)
	return row
}
func TestSourceHTTPAutomaticallyChecksContentV90(t *testing.T) {
	data := bytes.Repeat([]byte("actual-source-content"), 100)
	a, _, local := sourceDetectorFixtureV90(t, data)
	server := contentServer(data)
	defer server.Close()
	row := sourceScanHTTPV90(t, a, server.URL+"/renamed.bin")
	if row.Detector.Classification != "EXACT" || row.LocalPath != local || row.Status != "VERIFIED" {
		t.Fatalf("source integration: %#v", row)
	}
}
func TestSourceCancellationAndReplacementV90(t *testing.T) {
	data := bytes.Repeat([]byte("blocked"), 100)
	a, _, _ := sourceDetectorFixtureV90(t, data)
	entered := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "HEAD" {
			w.Header().Set("Content-Length", fmt.Sprint(len(data)))
			return
		}
		select {
		case entered <- struct{}{}:
		default:
		}
		<-r.Context().Done()
	}))
	defer server.Close()
	mux := http.NewServeMux()
	a.registerDuplicateRoutesV90(mux)
	a.compareRemote(context.Background(), []RemoteItem{{Name: "blocked.bin", Size: int64(len(data)), Source: "HTTP", DirectURL: server.URL}}, "balanced")
	a.duplicateScan.mu.Lock()
	oldDone := a.duplicateScan.done
	a.duplicateScan.mu.Unlock()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("detector never requested content")
	}
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest("GET", "/api/duplicates/status", nil))
	if !bytes.Contains(response.Body.Bytes(), []byte(`"active":true`)) {
		t.Fatal(response.Body.String())
	}
	a.compareRemote(context.Background(), []RemoteItem{{Name: "replacement.bin", Size: 99999, Source: "HTTP"}}, "balanced")
	waitSourceDetectorV90(t, a)
	select {
	case <-oldDone:
	case <-time.After(5 * time.Second):
		t.Fatal("previous request survived cancellation")
	}
	row, _ := a.resultByID(1)
	if row.Remote.Name != "replacement.bin" || row.GuardMethod == "full-sha256-error" {
		t.Fatalf("stale worker replaced new source: %#v", row)
	}
}
func TestSourceCorpusV90(t *testing.T) {
	root := os.Getenv("DDG_MEDIA_CORPUS")
	if root == "" {
		t.Skip("set DDG_MEDIA_CORPUS")
	}
	raw, err := os.ReadFile(filepath.Join(root, "cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cases []corpusCaseV90
	if err = json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.FileServer(http.Dir(root)))
	defer server.Close()
	for _, tc := range cases {
		t.Run(tc.Label, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(root, tc.Local))
			if err != nil {
				t.Fatal(err)
			}
			a, collection, old := sourceDetectorFixtureV90(t, data)
			local := filepath.Join(collection, tc.LocalName)
			if err = os.MkdirAll(filepath.Dir(local), 0700); err != nil {
				t.Fatal(err)
			}
			if err = os.Rename(old, local); err != nil {
				t.Fatal(err)
			}
			a.runIndex(context.Background(), []string{collection}, "", 0)
			row := sourceScanHTTPV90(t, a, server.URL+"/"+tc.Remote)
			e := row.Detector
			score := 0
			if e.Score != nil {
				score = *e.Score
			}
			pass := false
			switch tc.Expected {
			case "exact":
				pass = e.Classification == "EXACT" && row.LocalPath == local
			case "related":
				pass = e.Classification != "EXACT" && score >= 85 && row.LocalPath == local
			case "different":
				pass = e.Classification != "EXACT" && score < 85
			}
			if !pass {
				t.Fatalf("%s: class=%s score=%d path=%s reason=%s", tc.Expected, e.Classification, score, row.LocalPath, row.Reason)
			}
			t.Logf("%s: %s score=%d", tc.Expected, e.Classification, score)
		})
	}
}
