package main

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFuncV86 func(*http.Request) (*http.Response, error)

func (f roundTripFuncV86) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestGofileFolderCodeV86(t *testing.T) {
	for _, raw := range []string{"https://gofile.io/d/INtYpg", "https://www.gofile.io/d/INtYpg?x=1"} {
		got, err := gofileFolderCodeV86(raw)
		if err != nil || got != "INtYpg" {
			t.Fatalf("parse %q = %q, %v", raw, got, err)
		}
	}
	if _, err := gofileFolderCodeV86("https://example.com/d/INtYpg"); err == nil {
		t.Fatal("non-GoFile host must fail")
	}
}

func TestGofileWebsiteTokenV86StablePerBucket(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	a := gofileWebsiteTokenV86("guest-token", now)
	b := gofileWebsiteTokenV86("guest-token", now.Add(10*time.Minute))
	if len(a) != 64 || a != b {
		t.Fatalf("website token must be stable SHA-256 inside the same 4h bucket: %q %q", a, b)
	}
	c := gofileWebsiteTokenV86("guest-token", now.Add(5*time.Hour))
	if a == c {
		t.Fatal("website token must change after the time bucket changes")
	}
}

func TestFetchGofileContentV86SendsCurrentHeaders(t *testing.T) {
	client := &http.Client{Transport: roundTripFuncV86(func(r *http.Request) (*http.Response, error) {
		if !strings.Contains(r.URL.String(), "/contents/ROOT") {
			t.Fatalf("unexpected URL: %s", r.URL)
		}
		if r.Header.Get("Authorization") != "Bearer guest-token" {
			t.Fatalf("missing bearer auth: %q", r.Header.Get("Authorization"))
		}
		if len(r.Header.Get("X-Website-Token")) != 64 {
			t.Fatalf("missing website token: %q", r.Header.Get("X-Website-Token"))
		}
		if r.Header.Get("X-BL") != "en-US" || r.Header.Get("Origin") != "https://gofile.io" {
			t.Fatalf("missing GoFile web headers: %#v", r.Header)
		}
		body := `{"status":"ok","data":{"id":"ROOT","type":"folder","name":"Root","children":{"F1":{"id":"F1","type":"file","name":"a.mp4","size":123,"link":"https://store1.gofile.io/download/a","md5":"0123456789abcdef0123456789abcdef","mimetype":"video/mp4"}}}}`
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})}
	root, err := fetchGofileContentV86(context.Background(), client, "guest-token", "ROOT")
	if err != nil {
		t.Fatal(err)
	}
	if len(root.Children) != 1 || root.Children["F1"].Name != "a.mp4" {
		t.Fatalf("unexpected GoFile content: %#v", root)
	}
}

func TestWalkGofileContentV86BuildsRemoteItems(t *testing.T) {
	root := gofileContentV86{
		ID: "ROOT", Type: "folder", Name: "Root",
		Children: map[string]gofileContentV86{
			"F1": {ID: "F1", Type: "file", Name: "clip.mp4", Size: 777, Link: "https://store1.gofile.io/download/clip", MD5: "0123456789abcdef0123456789abcdef", MimeType: "video/mp4"},
		},
	}
	var items []RemoteItem
	if err := walkGofileContentV86(context.Background(), &http.Client{}, "guest-token", "https://gofile.io/d/ROOT", "", root, map[string]bool{"ROOT": true}, &items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	it := items[0]
	if it.Source != "GOFILE" || it.Extractor != "gofile-native" || it.ProviderID != "F1" || it.Size != 777 || it.HashType != "md5" {
		t.Fatalf("unexpected remote item: %#v", it)
	}
	if ctx, ok := providerContextForURLV86(it.DirectURL); !ok || !strings.Contains(ctx.Headers.Get("Cookie"), "accountToken=") {
		t.Fatalf("download context was not cached: %#v, %v", ctx, ok)
	}
}
