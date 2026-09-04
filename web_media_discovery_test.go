package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
)

func TestExtractWebMediaCandidatesUsesBestSrcsetAndStructuredMetadata(t *testing.T) {
	body := []byte(`<!doctype html><html><head>
<meta property="og:image" content="/og-cover.jpg">
<script type="application/ld+json">{"@type":"VideoObject","contentUrl":"/structured.webm","thumbnailUrl":"/structured-poster.jpg"}</script>
</head><body>
<img src="/thumb.jpg" srcset="/small.jpg 320w, /large.jpg 1920w">
<video poster="/poster.jpg"><source src="/movie.mp4" type="video/mp4"></video>
<a href="/full.png?token=abc">full image</a>
<a href="javascript:alert(1)">bad</a>
<img src="data:image/png;base64,AAAA">
</body></html>`)
	rows := extractWebMediaCandidates(body, "https://example.test/gallery/page")
	urls := map[string]bool{}
	for _, r := range rows {
		urls[r.URL] = true
	}
	for _, want := range []string{
		"https://example.test/large.jpg",
		"https://example.test/og-cover.jpg",
		"https://example.test/movie.mp4",
		"https://example.test/poster.jpg",
		"https://example.test/structured.webm",
		"https://example.test/structured-poster.jpg",
		"https://example.test/full.png?token=abc",
	} {
		if !urls[want] {
			t.Fatalf("missing candidate %s; got %#v", want, rows)
		}
	}
	if urls["https://example.test/small.jpg"] || urls["https://example.test/thumb.jpg"] {
		t.Fatalf("srcset should prefer the largest candidate instead of thumbnail: %#v", rows)
	}
	for u := range urls {
		if strings.HasPrefix(u, "data:") || strings.HasPrefix(u, "javascript:") {
			t.Fatalf("unsafe/non-http candidate leaked: %s", u)
		}
	}
}

func TestDiscoverHTMLMediaSendsRefererAndFindsImagesVideo(t *testing.T) {
	var base string
	media := map[string]string{
		"/photo.jpg": "image/jpeg",
		"/clip.mp4":  "video/mp4",
		"/extra.webp": "image/webp",
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/page" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprintf(w, `<html><head><meta property="og:image" content="/extra.webp"></head><body><img src="/photo.jpg"><video src="/clip.mp4"></video></body></html>`)
			return
		}
		ct, ok := media[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		if r.Referer() != base+"/page" {
			http.Error(w, "missing referer", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", ct)
		w.Header().Set("Content-Length", "12345")
		w.Header().Set("Accept-Ranges", "bytes")
		if r.Method == http.MethodGet && r.Header.Get("Range") != "" {
			w.Header().Set("Content-Range", "bytes 0-0/12345")
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write([]byte{0})
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	base = ts.URL

	rows, err := discoverHTMLMedia(context.Background(), ts.URL+"/page")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 media items, got %d: %#v", len(rows), rows)
	}
	names := make([]string, 0, len(rows))
	for _, r := range rows {
		if r.Source != "HTML" {
			t.Fatalf("unexpected source %q", r.Source)
		}
		if r.URL != ts.URL+"/page" {
			t.Fatalf("page source not retained: %#v", r)
		}
		if r.DirectURL == "" || r.Size != 12345 || r.ApproxSize {
			t.Fatalf("bad probed metadata: %#v", r)
		}
		if r.ProviderID == "" {
			t.Fatalf("stable provider id missing: %#v", r)
		}
		names = append(names, r.Name)
	}
	sort.Strings(names)
	if strings.Join(names, ",") != "clip.mp4,extra.webp,photo.jpg" {
		t.Fatalf("unexpected names: %v", names)
	}
}

func TestWebMediaStableIDIgnoresSignedQuery(t *testing.T) {
	a := webMediaStableID("https://cdn.example.test/media/photo.jpg?token=one&expires=1")
	b := webMediaStableID("https://cdn.example.test/media/photo.jpg?token=two&expires=2")
	if a == "" || a != b {
		t.Fatalf("signed query should not change stable id: %q vs %q", a, b)
	}
	c := webMediaStableID("https://cdn.example.test/media/other.jpg?token=two")
	if c == a {
		t.Fatal("different media path must have different stable id")
	}
}

func TestParseContentRangeTotal(t *testing.T) {
	if got := parseContentRangeTotal("bytes 0-0/987654"); got != 987654 {
		t.Fatalf("got %d", got)
	}
	if got := parseContentRangeTotal("bytes */*"); got != 0 && got != -1 {
		t.Fatalf("unexpected invalid range total %d", got)
	}
}
