package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGenericMediaAllowedRootV85127(t *testing.T) {
	allowed := []string{
		"https://example.com/watch/123",
		"https://filmepornonline.org/video/test",
		"http://127.0.0.1:8080/page",
	}
	for _, raw := range allowed {
		if !genericMediaAllowedRootV85127(raw) {
			t.Fatalf("expected generic URL to be allowed: %s", raw)
		}
	}
	blocked := []string{
		"https://www.erome.com/a/abc",
		"https://gofile.io/d/abc",
		"https://bunkr.cr/a/abc",
		"https://cyberdrop.me/a/abc",
		"https://mega.nz/folder/x#y",
	}
	for _, raw := range blocked {
		if genericMediaAllowedRootV85127(raw) {
			t.Fatalf("dedicated provider must not use generic media discovery: %s", raw)
		}
	}
}

func TestExtractGenericMediaHTMLV85127FindsTagsJSAndStreams(t *testing.T) {
	base := "https://example.com/watch/page"
	html := `
	<html><body>
	  <video src="/media/main.mp4"></video>
	  <source src="/hls/master.m3u8?token=1">
	  <img data-src="/images/photo.webp" srcset="/images/small.jpg 1x, /images/big.jpg 2x">
	  <iframe src="/embed/player"></iframe>
	  <script>
	    const dash = "\/dash\/manifest.mpd";
	    const duplicate = "/media/main.mp4";
	  </script>
	</body></html>`
	items, frames := extractGenericMediaHTMLV85127(base, []byte(html))
	want := map[string]string{
		"https://example.com/media/main.mp4":          "video",
		"https://example.com/hls/master.m3u8?token=1": "hls",
		"https://example.com/images/photo.webp":       "image",
		"https://example.com/images/small.jpg":        "image",
		"https://example.com/images/big.jpg":          "image",
		"https://example.com/dash/manifest.mpd":       "dash",
	}
	got := map[string]string{}
	for _, item := range items {
		got[item.URL] = item.Kind
	}
	for raw, kind := range want {
		if got[raw] != kind {
			t.Fatalf("missing %s as %s; got=%v", raw, kind, got)
		}
	}
	if len(frames) != 1 || frames[0] != "https://example.com/embed/player" {
		t.Fatalf("unexpected iframe list: %#v", frames)
	}
	countMain := 0
	for _, item := range items {
		if item.URL == "https://example.com/media/main.mp4" {
			countMain++
		}
	}
	if countMain != 1 {
		t.Fatalf("duplicate media URL was not deduplicated: %d", countMain)
	}
}

func TestCrawlGenericMediaHTMLV85127RecursesIframeWithoutDownloadingMedia(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/root":
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprintf(w, `<html><iframe src="/embed"></iframe><video src="/video/root.mp4"></video></html>`)
		case "/embed":
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprintf(w, `<html><script>window.player={file:"%s/live/master.m3u8",dash:"/dash/manifest.mpd"}</script></html>`, server.URL)
		case "/video/root.mp4", "/live/master.m3u8", "/dash/manifest.mpd":
			t.Fatalf("crawler must discover media URLs from HTML without requesting media body: %s", r.URL.Path)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	items, warnings := crawlGenericMediaHTMLV85127(ctx, server.URL+"/root")
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	got := map[string]string{}
	for _, item := range items {
		got[item.URL] = item.Kind
	}
	for raw, kind := range map[string]string{
		server.URL + "/video/root.mp4":    "video",
		server.URL + "/live/master.m3u8":  "hls",
		server.URL + "/dash/manifest.mpd": "dash",
	} {
		if got[raw] != kind {
			t.Fatalf("missing iframe discovery %s as %s; got=%v", raw, kind, got)
		}
	}
}

func TestGenericMediaKindV85127(t *testing.T) {
	cases := map[string]string{
		"https://x.test/a.m3u8": "hls",
		"https://x.test/a.mpd":  "dash",
		"https://x.test/a.mp4":  "video",
		"https://x.test/a.webp": "image",
		"https://x.test/a.m4a":  "audio",
	}
	for raw, want := range cases {
		if got := genericMediaKindV85127(raw, ""); got != want {
			t.Fatalf("%s: got %s want %s", raw, got, want)
		}
	}
	if got := genericMediaKindV85127("https://x.test/stream", "application/vnd.apple.mpegurl"); got != "hls" {
		t.Fatalf("content-type HLS: got %s", got)
	}
	if got := genericMediaKindV85127("https://x.test/stream", "application/dash+xml"); got != "dash" {
		t.Fatalf("content-type DASH: got %s", got)
	}
}

func TestCappedWriterV85127DoesNotBackpressureChildProcess(t *testing.T) {
	w := &cappedWriterV85127{limit: 8}
	payload := strings.Repeat("x", 1024)
	n, err := w.Write([]byte(payload))
	if err != nil || n != len(payload) {
		t.Fatalf("writer must report full consumption: n=%d err=%v", n, err)
	}
	if len(w.String()) != 8 || !w.truncated {
		t.Fatalf("unexpected capped writer state len=%d truncated=%v", len(w.String()), w.truncated)
	}
}
