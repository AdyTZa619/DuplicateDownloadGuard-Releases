package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestMegaPreviewMediaServerV8523PersistentServerAndCancellation(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Range"); got != "bytes=0-3" {
			t.Fatalf("Range upstream = %q, want bytes=0-3", got)
		}
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Content-Range", "bytes 0-3/8")
		w.Header().Set("Content-Length", "4")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte("ABCD"))
	}))
	defer upstream.Close()

	s := newMegaPreviewMediaServerV8523()
	defer func() {
		s.mu.Lock()
		srv := s.server
		s.mu.Unlock()
		if srv != nil {
			_ = srv.Close()
		}
	}()

	first, err := s.activate(upstream.URL + "/first.mp4")
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.activate(upstream.URL + "/second.mp4")
	if err != nil {
		t.Fatal(err)
	}

	u1, _ := url.Parse(first)
	u2, _ := url.Parse(second)
	if u1.Scheme != u2.Scheme || u1.Host != u2.Host {
		t.Fatalf("serverul local trebuie sa ramana persistent: %s vs %s", first, second)
	}
	if u1.Path == u2.Path {
		t.Fatalf("fiecare selectie trebuie sa aiba generatie proprie: %s", first)
	}

	staleResp, err := http.Get(first)
	if err != nil {
		t.Fatal(err)
	}
	_ = staleResp.Body.Close()
	if staleResp.StatusCode != http.StatusGone {
		t.Fatalf("URL vechi status = %d, want %d", staleResp.StatusCode, http.StatusGone)
	}

	req, err := http.NewRequest(http.MethodGet, second, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Range", "bytes=0-3")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusPartialContent {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusPartialContent)
	}
	if string(body) != "ABCD" {
		t.Fatalf("body = %q, want ABCD", body)
	}
	if resp.Header.Get("Accept-Ranges") != "bytes" {
		t.Fatalf("Accept-Ranges nu a fost propagat")
	}

	s.stopCurrent()
	stoppedResp, err := http.Get(second)
	if err != nil {
		t.Fatal(err)
	}
	_ = stoppedResp.Body.Close()
	if stoppedResp.StatusCode != http.StatusGone {
		t.Fatalf("dupa stop status = %d, want %d", stoppedResp.StatusCode, http.StatusGone)
	}
}
