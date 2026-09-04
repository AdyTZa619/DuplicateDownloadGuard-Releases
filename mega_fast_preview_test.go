package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMegaWebDAVChildURL(t *testing.T) {
	got, err := megaWebDAVChildURL("http://127.0.0.1:4443/Public%20Folder/", "/sub/My Video #1.mp4")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "/Public%20Folder/sub/My%20Video%20%231.mp4") {
		t.Fatalf("unexpected child URL: %s", got)
	}
	if _, err := megaWebDAVChildURL("http://127.0.0.1:4443/root", "../secret.mp4"); err == nil {
		t.Fatal("parent traversal must be rejected")
	}
}

func TestPostScanWarmContextIsIndependentV8520(t *testing.T) {
	parent, cancelParent := context.WithCancel(context.Background())
	cancelParent()
	if parent.Err() == nil {
		t.Fatal("test setup: parent must be cancelled")
	}

	ctx, cancel := newMegaPostScanWarmContextV8520()
	defer cancel()
	if ctx.Err() != nil {
		t.Fatalf("post-scan warm context inherited cancellation: %v", ctx.Err())
	}
}

func TestWarmRootPreviewURLUsesExistingRoot(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/root/sub/clip.mp4" {
			w.Header().Set("Content-Type", "video/mp4")
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	st := MegaPreviewState{Active: true, SourceURL: "https://mega.nz/folder/X#KEY", RemotePath: megaWarmRootRefV86, StreamURL: ts.URL + "/root"}
	item := RemoteItem{URL: st.SourceURL, Path: "sub/clip.mp4", Name: "clip.mp4", Source: "MEGA"}
	got, ok := warmRootPreviewURLV86(st, item)
	if !ok || got != ts.URL+"/root/sub/clip.mp4" {
		t.Fatalf("expected warm child URL, got ok=%v url=%q", ok, got)
	}
}

func TestWarmRootPreviewFallsBackWhenChildMissing(t *testing.T) {
	ts := httptest.NewServer(http.NotFoundHandler())
	defer ts.Close()
	st := MegaPreviewState{Active: true, SourceURL: "mega-source", RemotePath: megaWarmRootRefV86, StreamURL: ts.URL + "/root"}
	item := RemoteItem{URL: "mega-source", Path: "missing.mp4", Name: "missing.mp4", Source: "MEGA"}
	if got, ok := warmRootPreviewURLV86(st, item); ok || got != "" {
		t.Fatalf("missing child must fall back, got ok=%v url=%q", ok, got)
	}
}
