package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
)

func TestMegaPreviewEphemeralOriginUsesFreshPortAndForwardsRangeV8522(t *testing.T) {
	var mu sync.Mutex
	seenRanges := []string{}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seenRanges = append(seenRanges, r.Header.Get("Range"))
		mu.Unlock()
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Accept-Ranges", "bytes")
		if r.Header.Get("Range") != "" {
			w.Header().Set("Content-Range", "bytes 1-3/6")
			w.Header().Set("Content-Length", "3")
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write([]byte("bcd"))
			return
		}
		w.Header().Set("Content-Length", "6")
		_, _ = w.Write([]byte("abcdef"))
	}))
	defer upstream.Close()

	first, err := startMegaPreviewEphemeralOriginV8522(upstream.URL + "/clip.mp4")
	if err != nil {
		t.Fatal(err)
	}
	second, err := startMegaPreviewEphemeralOriginV8522(upstream.URL + "/clip.mp4")
	if err != nil {
		t.Fatal(err)
	}
	firstURL, _ := url.Parse(first)
	secondURL, _ := url.Parse(second)
	if firstURL.Host == "" || secondURL.Host == "" {
		t.Fatalf("missing isolated hosts: first=%q second=%q", first, second)
	}
	if firstURL.Host == secondURL.Host {
		t.Fatalf("each preview must get a fresh browser origin/port, both were %s", firstURL.Host)
	}

	req, _ := http.NewRequest(http.MethodGet, first, nil)
	req.Header.Set("Range", "bytes=1-3")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent {
		t.Fatalf("expected 206, got %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Range"); got != "bytes 1-3/6" {
		t.Fatalf("Content-Range lost: %q", got)
	}
	if got := resp.Header.Get("X-DDG-MEGA-Origin"); got != "v8522" {
		t.Fatalf("isolation marker missing: %q", got)
	}
	if string(body) != "bcd" {
		t.Fatalf("unexpected body %q", body)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(seenRanges) == 0 || seenRanges[0] != "bytes=1-3" {
		t.Fatalf("Range was not forwarded: %#v", seenRanges)
	}
}

func TestMegaPreviewEphemeralOriginRejectsNonLoopbackV8522(t *testing.T) {
	if _, err := startMegaPreviewEphemeralOriginV8522("https://example.com/video.mp4"); err == nil {
		t.Fatal("non-loopback upstream must be rejected")
	}
}

func TestMegaPreviewUIRoutesEverySuccessThroughFreshOriginV8522(t *testing.T) {
	b, err := os.ReadFile("mega_preview_ui_fast_v854.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if strings.Count(s, "browserReadyMegaPreviewV8522(") < 4 {
		t.Fatalf("all success paths must route through isolated browser origin")
	}
}
