package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMegaPreviewProxyForwardsRangeV8513(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Range"); got != "bytes=10-19" {
			t.Fatalf("Range=%q", got)
		}
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Content-Range", "bytes 10-19/100")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte("0123456789"))
	}))
	defer up.Close()

	proxyURL, err := wrapMegaPreviewProxyURLV8513(up.URL + "/video.mp4")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(proxyURL, up.Listener.Addr().String()+"/video.mp4") {
		t.Fatalf("browser still received raw upstream URL: %s", proxyURL)
	}
	req, _ := http.NewRequest(http.MethodGet, proxyURL, nil)
	req.Header.Set("Range", "bytes=10-19")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusPartialContent || string(b) != "0123456789" {
		t.Fatalf("status=%d body=%q", resp.StatusCode, string(b))
	}
	if resp.Header.Get("Content-Range") != "bytes 10-19/100" || resp.Header.Get("X-DDG-MEGA-Proxy") != "v8514" {
		t.Fatalf("proxy headers missing: %#v", resp.Header)
	}
}

func TestMegaPreviewProxyRejectsNonLoopbackV8513(t *testing.T) {
	proxyURL, err := wrapMegaPreviewProxyURLV8513("https://example.com/video.mp4")
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Get(proxyURL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d", resp.StatusCode)
	}
}
