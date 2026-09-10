package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestDetectorRemoteBudgetAndBlockReuseV90(t *testing.T) {
	data := bytes.Repeat([]byte("range-data-123456"), 2<<20)
	var transfers atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"stable"`)
		http.ServeContent(corpusByteWriterV90{w, &transfers}, r, "video", fixedTime, bytes.NewReader(data))
	}))
	defer server.Close()
	ctx, _, close, err := prepareDetectorRemoteV90(context.Background(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer close()
	s := ctx.Value(detectorRemoteKeyV90{}).(*detectorRemoteV90)
	first, err := s.block(ctx, 0)
	if err != nil || len(first) != int(detectorRemoteBlockV90) {
		t.Fatalf("block: %d %v", len(first), err)
	}
	_, err = s.block(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	if transfers.Load() != detectorRemoteBlockV90 {
		t.Fatalf("block was downloaded twice: %d", transfers.Load())
	}
	for start := detectorRemoteBlockV90; start < detectorRemoteBudgetV90; start += detectorRemoteBlockV90 {
		if _, err = s.block(ctx, start); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = s.block(ctx, detectorRemoteBudgetV90); err == nil {
		t.Fatal("budget not enforced")
	}
	if s.Bytes > detectorRemoteBudgetV90 || transfers.Load() > detectorRemoteBudgetV90 {
		t.Fatalf("budget exceeded %d/%d", s.Bytes, transfers.Load())
	}
}

func TestDetectorRefusesIgnoredRangeAndChangedValidatorV90(t *testing.T) {
	for _, ignored := range []bool{true, false} {
		t.Run(map[bool]string{true: "ignored", false: "changed"}[ignored], func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Length", "4096")
				w.Header().Set("ETag", `"one"`)
				if r.Method == http.MethodHead {
					return
				}
				if !ignored {
					w.Header().Set("ETag", `"two"`)
					w.Header().Set("Content-Range", "bytes 0-4095/4096")
					w.WriteHeader(206)
				}
				_, _ = w.Write(make([]byte, 4096))
			}))
			defer server.Close()
			ctx, _, close, err := prepareDetectorRemoteV90(context.Background(), server.URL)
			if err != nil {
				t.Fatal(err)
			}
			defer close()
			s := ctx.Value(detectorRemoteKeyV90{}).(*detectorRemoteV90)
			if _, err = s.block(ctx, 0); err == nil || s.Bytes != 0 {
				t.Fatalf("unsafe response accepted %v bytes=%d", err, s.Bytes)
			}
		})
	}
}

func TestDetectorLoopbackServesSeekAndCancelsV90(t *testing.T) {
	data := bytes.Repeat([]byte("seek-"), 100000)
	server := contentServer(data)
	defer server.Close()
	ctx, target, close, err := prepareDetectorRemoteV90(context.Background(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodGet, target, nil)
	req.Header.Set("Range", "bytes=300000-300099")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		close()
		t.Fatal(err)
	}
	b, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil || resp.StatusCode != 206 || !bytes.Equal(b, data[300000:300100]) {
		close()
		t.Fatal("seek returned wrong content")
	}
	close()
	if ctx.Err() == nil {
		t.Fatal("session did not cancel")
	}
}
