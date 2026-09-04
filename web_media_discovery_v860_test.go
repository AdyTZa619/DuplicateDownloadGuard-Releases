package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExtractHTMLMediaV860(t *testing.T) {
	html := `<!doctype html>
<html><head>
<meta property="og:image" content="/og/cover.webp">
<meta property="og:video" content="https://cdn.example/video.mp4?token=1">
<script type="application/ld+json">{"@type":"VideoObject","contentUrl":"/media/movie.m3u8","thumbnailUrl":"/thumb.jpg"}</script>
</head><body>
<img src="/small.jpg" srcset="/small.jpg 320w, /large.jpg 1600w">
<img data-src="/lazy.avif">
<video src="/clip.webm" poster="/poster.png"><source src="/clip-1080.mp4" type="video/mp4"></video>
<a href="/files/photo.png?x=1">photo</a>
<a href="/not-media.html">ignore</a>
</body></html>`
	items := extractHTMLMediaV860("https://site.example/gallery/page", []byte(html))
	urls := map[string]RemoteItem{}
	for _, it := range items {
		urls[it.DirectURL] = it
		if it.URL != "https://site.example/gallery/page" {
			t.Fatalf("referer/page URL not preserved: %q", it.URL)
		}
		if it.Source != "HTML" {
			t.Fatalf("unexpected source: %q", it.Source)
		}
	}
	for _, want := range []string{
		"https://site.example/og/cover.webp",
		"https://cdn.example/video.mp4?token=1",
		"https://site.example/media/movie.m3u8",
		"https://site.example/thumb.jpg",
		"https://site.example/large.jpg",
		"https://site.example/small.jpg",
		"https://site.example/lazy.avif",
		"https://site.example/clip.webm",
		"https://site.example/poster.png",
		"https://site.example/clip-1080.mp4",
		"https://site.example/files/photo.png?x=1",
	} {
		if _, ok := urls[want]; !ok {
			t.Fatalf("missing %s; got %#v", want, urls)
		}
	}
	if _, ok := urls["https://site.example/not-media.html"]; ok {
		t.Fatal("non-media anchor must not be discovered")
	}
	if it := urls["https://site.example/media/movie.m3u8"]; it.ContentType != "stream/manifest" {
		t.Fatalf("manifest content type=%q", it.ContentType)
	}
}

func TestProbeHTMLMediaV860UsesFinalPageAsReferer(t *testing.T) {
	var base string
	mux := http.NewServeMux()
	mux.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/gallery/final", http.StatusFound)
	})
	mux.HandleFunc("/gallery/final", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<img src="/assets/a.jpg"><video><source src="/assets/v.mp4"></video>`))
	})
	s := httptest.NewServer(mux)
	defer s.Close()
	base = s.URL
	_ = base

	a := &App{}
	items, err := a.probeHTMLMediaV860(context.Background(), s.URL+"/start")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("items=%d %#v", len(items), items)
	}
	for _, it := range items {
		if it.URL != s.URL+"/gallery/final" {
			t.Fatalf("expected final URL as Referer, got %q", it.URL)
		}
		if !strings.HasPrefix(it.DirectURL, s.URL+"/assets/") {
			t.Fatalf("unexpected direct URL %q", it.DirectURL)
		}
	}
}

func TestBestSrcsetV860(t *testing.T) {
	if got := bestSrcsetURLV860("a.jpg 320w, b.jpg 1280w, c.jpg 640w"); got != "b.jpg" {
		t.Fatalf("got %q", got)
	}
	if got := bestSrcsetURLV860("a.jpg 1x, b.jpg 2x"); got != "b.jpg" {
		t.Fatalf("got %q", got)
	}
}
