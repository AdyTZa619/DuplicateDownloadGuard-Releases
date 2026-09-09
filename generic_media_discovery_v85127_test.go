package main

import (
	"context"
	"encoding/json"
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

func TestParseGenericYtDlpCandidatesV85132KeepsVideosMetadataAndQualities(t *testing.T) {
	output := `{"id":"one","title":"Primul video","webpage_url":"https://example.test/watch/one","thumbnail":"https://img.test/one.jpg","duration":91,"extractor":"example","formats":[{"format_id":"360","url":"https://cdn.test/one-360.mp4","ext":"mp4","vcodec":"h264","acodec":"aac","width":640,"height":360,"fps":30,"tbr":500},{"format_id":"1080","url":"https://cdn.test/one-1080.mp4","ext":"mp4","vcodec":"h264","acodec":"none","width":1920,"height":1080,"fps":60,"tbr":4200}]}
{"id":"two","title":"Al doilea video","webpage_url":"https://example.test/watch/two","thumbnail":"https://img.test/two.jpg","formats":[{"format_id":"720","url":"https://cdn.test/two-720.mp4","ext":"mp4","vcodec":"h264","acodec":"aac","width":1280,"height":720,"fps":30,"tbr":1800}]}`
	items := parseGenericYtDlpCandidatesV85132(output, "https://example.test/playlist")
	if len(items) != 2 {
		t.Fatalf("expected two logical videos, got %#v", items)
	}
	byID := map[string]genericMediaCandidateV85127{}
	for _, item := range items {
		byID[item.ID] = item
	}
	one := byID["one"]
	if one.Title != "Primul video" || one.Thumbnail != "https://img.test/one.jpg" || one.Kind != "video" || one.Via != "yt-dlp" {
		t.Fatalf("metadata lost: %#v", one)
	}
	if one.PreviewURL != "https://cdn.test/one-360.mp4" {
		t.Fatalf("preview should prefer a directly playable audio+video format, got %q", one.PreviewURL)
	}
	if len(one.Qualities) != 2 || one.Qualities[0].Height != 1080 || one.Qualities[1].Height != 360 {
		t.Fatalf("qualities were not preserved/sorted: %#v", one.Qualities)
	}
	if one.Qualities[0].HasAudio || !one.Qualities[1].HasAudio {
		t.Fatalf("audio availability was not preserved: %#v", one.Qualities)
	}
}

func TestParseGenericYtDlpCandidatesV85132DoesNotCallThumbnailVideo(t *testing.T) {
	output := `{"id":"photo","title":"Imagine","webpage_url":"https://example.test/photo","url":"https://img.test/photo.jpg","thumbnail":"https://img.test/thumb.jpg","vcodec":"none","acodec":"none","formats":[]}`
	items := parseGenericYtDlpCandidatesV85132(output, "https://example.test/gallery")
	if len(items) != 1 {
		t.Fatalf("expected one logical item, got %#v", items)
	}
	if items[0].Kind != "image" {
		t.Fatalf("image-only entry must remain an image, not become a video: %#v", items[0])
	}
}

func TestRegisterGenericMediaPreviewV85132HidesRequestHeaders(t *testing.T) {
	item := registerGenericMediaPreviewV85132(genericMediaCandidateV85127{
		URL:     "https://example.test/watch",
		Headers: map[string]string{"Referer": "https://example.test/", "Cookie": "secret=1"},
	}, 0)
	b, err := json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}
	if item.Token == "" || strings.Contains(string(b), "secret=1") || strings.Contains(string(b), "Referer") {
		t.Fatalf("preview token/header boundary invalid: %s", b)
	}
}

func TestGenericMediaPreviewV85132ForwardsRangeAndReferer(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") != "bytes=2-5" {
			t.Errorf("range not forwarded: %q", r.Header.Get("Range"))
		}
		if r.Referer() != "https://example.test/watch" {
			t.Errorf("referer not forwarded: %q", r.Referer())
		}
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Content-Range", "bytes 2-5/8")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte("2345"))
	}))
	defer upstream.Close()

	item := registerGenericMediaPreviewV85132(genericMediaCandidateV85127{
		URL:        "https://example.test/watch",
		PreviewURL: upstream.URL + "/video.mp4",
		Page:       "https://example.test/watch",
	}, 0)
	req := httptest.NewRequest(http.MethodGet, "/api/generic-media/preview?token="+item.Token+"&asset=media", nil)
	req.Header.Set("Range", "bytes=2-5")
	rec := httptest.NewRecorder()
	(&App{}).handleGenericMediaPreviewV85132(rec, req)
	if rec.Code != http.StatusPartialContent || rec.Body.String() != "2345" || rec.Header().Get("Content-Type") != "video/mp4" {
		t.Fatalf("unexpected proxied preview: code=%d type=%q body=%q", rec.Code, rec.Header().Get("Content-Type"), rec.Body.String())
	}
}
