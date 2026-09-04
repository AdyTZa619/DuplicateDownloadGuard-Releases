package main

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMegaPreviewProxyForwardsRangeV860(t *testing.T) {
	body := []byte("0123456789abcdefghijklmnopqrstuvwxyz")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/folder/video.mp4" {
			t.Fatalf("unexpected upstream path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Range"); got != "bytes=10-15" {
			t.Fatalf("range not forwarded: %q", got)
		}
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Content-Range", fmt.Sprintf("bytes 10-15/%d", len(body)))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(body[10:16])
	}))
	defer upstream.Close()

	a := &App{results: []Result{{ID: 7, Remote: RemoteItem{Source: "MEGA", URL: "https://mega.nz/folder/test", Path: "folder/video.mp4", Name: "video.mp4"}}}}
	a.preview = MegaPreviewState{Active: true, SourceURL: "https://mega.nz/folder/test", RemotePath: megaWarmRootRefV86, StreamURL: upstream.URL}

	req := httptest.NewRequest(http.MethodGet, previewProxyURLV860(7), nil)
	req.Header.Set("Range", "bytes=10-15")
	rr := httptest.NewRecorder()
	a.handleRemotePreviewProxyV860(rr, req)

	if rr.Code != http.StatusPartialContent {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if rr.Header().Get("X-DDG-Preview-Proxy") != "1" {
		t.Fatalf("proxy marker missing")
	}
	if rr.Header().Get("Content-Range") != fmt.Sprintf("bytes 10-15/%d", len(body)) {
		t.Fatalf("content-range=%q", rr.Header().Get("Content-Range"))
	}
	if !bytes.Equal(rr.Body.Bytes(), body[10:16]) {
		t.Fatalf("body=%q", rr.Body.Bytes())
	}
}

func TestMegaPreviewProxyRejectsNonMegaV860(t *testing.T) {
	a := &App{results: []Result{{ID: 3, Remote: RemoteItem{Source: "HTTP", URL: "https://example.com/a.mp4", Name: "a.mp4"}}}}
	req := httptest.NewRequest(http.MethodGet, previewProxyURLV860(3), nil)
	rr := httptest.NewRecorder()
	a.handleRemotePreviewProxyV860(rr, req)
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d", rr.Code)
	}
}

func TestLoopbackPreviewUpstreamV860(t *testing.T) {
	for _, okURL := range []string{"http://127.0.0.1:4443/a", "http://localhost:4443/a", "http://[::1]:4443/a"} {
		if !isLoopbackPreviewUpstreamV860(okURL) {
			t.Fatalf("expected loopback: %s", okURL)
		}
	}
	for _, badURL := range []string{"https://example.com/a", "file:///tmp/a", "javascript:alert(1)"} {
		if isLoopbackPreviewUpstreamV860(badURL) {
			t.Fatalf("unexpected accepted upstream: %s", badURL)
		}
	}
}
