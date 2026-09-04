package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestChooseQueueEngineV855IsDeterministic(t *testing.T) {
	cases := []struct {
		res  Result
		want string
	}{
		{Result{Remote: RemoteItem{Source: "MEGA"}}, "mega"},
		{Result{Remote: RemoteItem{Source: "YT-DLP", URL: "https://site/video"}}, "yt-dlp"},
		{Result{Remote: RemoteItem{Source: "HTTP", DirectURL: "https://cdn/x.jpg"}}, "internal"},
		{Result{Remote: RemoteItem{Source: "GALLERY-DL", URL: "https://site/post", DirectURL: "https://cdn/x.jpg"}}, "internal"},
		{Result{Remote: RemoteItem{Source: "HTTP", DirectURL: "https://cdn/live.m3u8", ContentType: "stream/manifest"}}, "yt-dlp"},
	}
	for _, tc := range cases {
		if got := chooseQueueEngineV855(tc.res, "auto"); got != tc.want {
			t.Fatalf("source=%s url=%s got=%s want=%s", tc.res.Remote.Source, tc.res.Remote.DirectURL, got, tc.want)
		}
	}
	if got := chooseQueueEngineV855(cases[2].res, "aria2"); got != "aria2" {
		t.Fatalf("explicit engine ignored: %s", got)
	}
}

func TestStableRemoteIdentityDoesNotTrustResultID(t *testing.T) {
	j := &DownloadJob{ResultID: 1, Remote: RemoteItem{Source: "MEGA", Handle: "Handle_A", Name: "a.mp4"}}
	a := Result{ID: 1, Remote: RemoteItem{Source: "MEGA", Handle: "Handle_A", Name: "renamed.mp4"}}
	b := Result{ID: 1, Remote: RemoteItem{Source: "MEGA", Handle: "Handle_B", Name: "a.mp4"}}
	if !sameQueueRemoteV855(j, a) {
		t.Fatal("same MEGA handle must be the same queued source")
	}
	if sameQueueRemoteV855(j, b) {
		t.Fatal("reused ResultID must not make a different MEGA handle equal")
	}
}

func TestDownloadResultForJobUsesSnapshotAfterResultsChange(t *testing.T) {
	job := &DownloadJob{ResultID: 1, Name: "old.mp4", Source: "HTTP", URL: "https://old.example/old.mp4", BytesTotal: 10, Remote: RemoteItem{Name: "old.mp4", Path: "old.mp4", Source: "HTTP", URL: "https://page.example/old", DirectURL: "https://cdn.example/old.mp4", Size: 10}}
	a := &App{}
	a.results = []Result{{ID: 1, Remote: RemoteItem{Name: "new.mp4", Source: "HTTP", DirectURL: "https://cdn.example/new.mp4", Size: 10}}}
	got := a.downloadResultForJobV855(job)
	if got.Remote.Name != "old.mp4" || got.Remote.DirectURL != "https://cdn.example/old.mp4" {
		t.Fatalf("job snapshot was replaced by reused ResultID: %#v", got.Remote)
	}
}

func TestInternalDownloadV855SendsRefererAndCompletes(t *testing.T) {
	payload := []byte("0123456789abcdefghijklmnopqrstuvwxyz")
	pageURL := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/page" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path != "/media.bin" {
			http.NotFound(w, r)
			return
		}
		if r.Referer() != pageURL {
			http.Error(w, "referer required", http.StatusForbidden)
			return
		}
		http.ServeContent(w, r, "media.bin", modTimeV855, strings.NewReader(string(payload)))
	}))
	defer srv.Close()
	pageURL = srv.URL + "/page"

	res := Result{Remote: RemoteItem{Name: "media.bin", Source: "GALLERY-DL", URL: pageURL, DirectURL: srv.URL + "/media.bin", Size: int64(len(payload))}}
	dest := t.TempDir()
	path, err := internalDownloadV855(context.Background(), res, dest, func(int64, int64) {})
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Fatalf("payload mismatch: %q", got)
	}
}

func TestInternalDownloadV855UsesCollisionSafeFinalName(t *testing.T) {
	payload := "abc123"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, payload)
	}))
	defer srv.Close()
	dest := t.TempDir()
	if err := os.WriteFile(filepath.Join(dest, "same.txt"), []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	res := Result{Remote: RemoteItem{Name: "same.txt", Source: "HTTP", URL: srv.URL, DirectURL: srv.URL, Size: int64(len(payload))}}
	path, err := internalDownloadV855(context.Background(), res, dest, func(int64, int64) {})
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != "same (1).txt" {
		t.Fatalf("collision path=%s", path)
	}
	if b, _ := os.ReadFile(filepath.Join(dest, "same.txt")); string(b) != "old" {
		t.Fatal("existing file was overwritten")
	}
}

func TestInternalDownloadV855PartNameIsStablePerSource(t *testing.T) {
	a := sourcePartPathV855("C:/D", "x.mp4", "https://a/x")
	b := sourcePartPathV855("C:/D", "x.mp4", "https://b/x")
	if a == b {
		t.Fatal("different sources must not share the same .part file")
	}
	if sourcePartPathV855("C:/D", "x.mp4", "https://a/x") != a {
		t.Fatal("same source must produce a stable .part path for resume")
	}
}

func TestDownloadJobRemoteSnapshotJSONRoundTrip(t *testing.T) {
	job := DownloadJob{ID: "j1", ResultID: 7, Name: "v.mp4", Source: "MEGA", Remote: RemoteItem{Name: "v.mp4", Source: "MEGA", URL: "folder", Handle: "HANDLE7"}, RequestedEngine: "auto"}
	b, err := json.Marshal(job)
	if err != nil {
		t.Fatal(err)
	}
	var got DownloadJob
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.Remote.Handle != "HANDLE7" || got.RequestedEngine != "auto" {
		t.Fatalf("snapshot lost after JSON roundtrip: %#v", got)
	}
}

func TestInternalDownloadV855RejectsHTMLInsteadOfMedia(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, "<html>login</html>")
	}))
	defer srv.Close()
	res := Result{Remote: RemoteItem{Name: "clip.mp4", Source: "HTTP", URL: srv.URL, DirectURL: srv.URL, ContentType: "video/mp4"}}
	_, err := internalDownloadV855(context.Background(), res, t.TempDir(), func(int64, int64) {})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "pagină html") {
		t.Fatalf("expected HTML response rejection, got %v", err)
	}
}

var modTimeV855 = time.Unix(1700000000, 0)
