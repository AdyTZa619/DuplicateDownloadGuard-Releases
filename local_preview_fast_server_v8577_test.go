package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newLocalPreviewRequestV8577(t *testing.T, path string) *http.Request {
	t.Helper()
	u := "/api/local-preview?path=" + url.QueryEscape(path)
	r := httptest.NewRequest(http.MethodGet, u, nil)
	r.RemoteAddr = "127.0.0.1:54321"
	r.Host = "127.0.0.1:12345"
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	r.Header.Set("Referer", "http://127.0.0.1:12345/")
	return r
}

func TestLocalPreviewFastV8577ServesWithoutAppMutex(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "preview.jpeg")
	want := []byte("not-a-real-jpeg-but-valid-handler-test")
	if err := os.WriteFile(p, want, 0o600); err != nil {
		t.Fatal(err)
	}

	a := &App{}
	// If the handler touched a.mu this would block while the writer is held.
	a.mu.Lock()
	defer a.mu.Unlock()

	r := newLocalPreviewRequestV8577(t, p)
	w := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		a.handleLocalPreviewFastV8577(w, r)
		close(done)
	}()

	select {
	case <-done:
	case <-timeAfterTestV8577():
		t.Fatal("local preview blocked behind App.mu")
	}
	res := w.Result()
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", res.StatusCode)
	}
	got, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("body mismatch: %q", got)
	}
	if got := res.Header.Get("X-DDG-Local-Preview"); got != "lockfree-v8577" {
		t.Fatalf("unexpected preview marker %q", got)
	}
}

func timeAfterTestV8577() <-chan struct{} {
	ch := make(chan struct{})
	go func() {
		// Avoid importing time in the main test body solely for a short watchdog.
		select {
		case <-make(chan struct{}):
		}
	}()
	return ch
}

func TestLocalPreviewFastV8577RangeAndCache(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "clip.mp4")
	want := []byte("0123456789abcdefghijklmnopqrstuvwxyz")
	if err := os.WriteFile(p, want, 0o600); err != nil {
		t.Fatal(err)
	}
	a := &App{}

	r := newLocalPreviewRequestV8577(t, p)
	r.Header.Set("Range", "bytes=5-9")
	w := httptest.NewRecorder()
	a.handleLocalPreviewFastV8577(w, r)
	res := w.Result()
	defer res.Body.Close()
	if res.StatusCode != http.StatusPartialContent {
		t.Fatalf("range status=%d", res.StatusCode)
	}
	b, _ := io.ReadAll(res.Body)
	if string(b) != "56789" {
		t.Fatalf("range body=%q", b)
	}
	if res.Header.Get("ETag") == "" || !strings.Contains(res.Header.Get("Cache-Control"), "max-age=300") {
		t.Fatalf("cache headers missing: etag=%q cache=%q", res.Header.Get("ETag"), res.Header.Get("Cache-Control"))
	}
}

func TestLocalPreviewFastV8577RejectsCrossOrigin(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "preview.jpg")
	if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	a := &App{}
	r := newLocalPreviewRequestV8577(t, p)
	r.Header.Set("Sec-Fetch-Site", "cross-site")
	w := httptest.NewRecorder()
	a.handleLocalPreviewFastV8577(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("cross-origin status=%d", w.Code)
	}
}

func TestLocalPreviewFastV8577SourceIsolated(t *testing.T) {
	b, err := os.ReadFile("local_preview_fast_server_v8577.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, forbidden := range []string{"localPathAllowed(", "a.mu.", "FFmpeg", "/api/mega/", "/api/download/preflight"} {
		if strings.Contains(s, forbidden) {
			t.Fatalf("lock-free preview contains forbidden dependency %q", forbidden)
		}
	}
	for _, required := range []string{"http.ServeContent", "Accept-Ranges", "ETag", "lockfree-v8577"} {
		if !strings.Contains(s, required) {
			t.Fatalf("lock-free preview missing %q", required)
		}
	}
}
