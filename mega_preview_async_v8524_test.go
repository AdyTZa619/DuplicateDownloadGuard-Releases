package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAsyncPreviewStartReturnsLocalURLFromWarmCacheV8524(t *testing.T) {
	a := &App{}
	item := RemoteItem{ID: 7, Source: "MEGA", URL: "https://mega.nz/folder/test", Path: "folder/video.mp4", Name: "video.mp4", Handle: "ABC123"}
	a.preview = MegaPreviewState{Active: true, SourceURL: item.URL, RemotePath: megaWarmRootRefV86, StreamURL: "http://127.0.0.1:4443/root"}

	job, localURL, err := a.beginMegaAsyncPreviewV8524(item, false)
	if err != nil {
		t.Fatal(err)
	}
	defer cancelMegaAsyncPreviewV8524()
	if job == nil || job.generation == 0 {
		t.Fatal("job/generation missing")
	}
	if !strings.HasPrefix(localURL, "/api/remote-preview/media?v=") {
		t.Fatalf("local URL=%q", localURL)
	}
	select {
	case <-job.done:
	default:
		t.Fatal("warm-cache job should already be ready")
	}
	if job.upstream == "" {
		t.Fatal("warm-cache upstream missing")
	}
}

func TestAsyncPreviewProxyPreservesRangeAnd206V8524(t *testing.T) {
	var gotRange string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRange = r.Header.Get("Range")
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Content-Range", "bytes 10-19/100")
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Content-Length", "10")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte("0123456789"))
	}))
	defer upstream.Close()

	job := &megaAsyncPreviewJobV8524{generation: 99, done: make(chan struct{}), upstream: upstream.URL, mode: "TEST"}
	job.ctx, job.cancel = contextWithCancelForTestV8524()
	close(job.done)
	megaAsyncPreviewStateV8524.Lock()
	megaAsyncPreviewStateV8524.current = job
	megaAsyncPreviewStateV8524.Unlock()
	defer cancelMegaAsyncPreviewV8524()

	a := &App{}
	req := httptest.NewRequest(http.MethodGet, "/api/remote-preview/media?v=99", nil)
	req.Header.Set("Range", "bytes=10-19")
	rr := httptest.NewRecorder()
	a.handleMegaAsyncPreviewMediaV8524(rr, req)

	if rr.Code != http.StatusPartialContent {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if gotRange != "bytes=10-19" {
		t.Fatalf("upstream Range=%q", gotRange)
	}
	if rr.Header().Get("Content-Range") != "bytes 10-19/100" {
		t.Fatalf("Content-Range=%q", rr.Header().Get("Content-Range"))
	}
	if rr.Body.String() != "0123456789" {
		t.Fatalf("body=%q", rr.Body.String())
	}
}

func TestSupersededAsyncPreviewReturnsGoneV8524(t *testing.T) {
	job := &megaAsyncPreviewJobV8524{generation: 10, done: make(chan struct{})}
	job.ctx, job.cancel = contextWithCancelForTestV8524()
	megaAsyncPreviewStateV8524.Lock()
	megaAsyncPreviewStateV8524.current = &megaAsyncPreviewJobV8524{generation: 11}
	megaAsyncPreviewStateV8524.Unlock()

	a := &App{}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/remote-preview/media?v=%d", job.generation), nil)
	a.handleMegaAsyncPreviewMediaV8524(rr, req)
	if rr.Code != http.StatusGone {
		t.Fatalf("status=%d want=%d", rr.Code, http.StatusGone)
	}
}

func contextWithCancelForTestV8524() (context.Context, context.CancelFunc) {
	return context.WithCancel(context.Background())
}
