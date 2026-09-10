package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDetectorPreflightKeepsStatusAPIResponsiveV90(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	data := bytes.Repeat([]byte("same"), 4096)
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		_, _ = w.Write(data)
	}))
	defer remote.Close()
	collection, download := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(collection, "renamed.bin"), data, 0644); err != nil {
		t.Fatal(err)
	}
	a := guardTestApp(t, collection, download, RemoteItem{Name: "remote.bin", Source: "HTTP", Size: int64(len(data)), DirectURL: remote.URL})
	mux := http.NewServeMux()
	mux.HandleFunc("/api/download/preflight", a.handleDownloadPreflight)
	mux.HandleFunc("/api/status", a.handleStatus)
	api := httptest.NewServer(mux)
	defer api.Close()
	done := make(chan int, 1)
	go func() {
		resp, err := http.Post(api.URL+"/api/download/preflight", "application/json", bytes.NewBufferString(`{"ids":[1],"mode":"smart"}`))
		if err != nil {
			done <- 0
			return
		}
		resp.Body.Close()
		done <- resp.StatusCode
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		close(release)
		t.Fatal("preflight did not start")
	}
	client := &http.Client{Timeout: time.Second}
	resp, err := client.Get(api.URL + "/api/status")
	close(release)
	if err != nil {
		t.Fatal("status API blocked behind detector I/O:", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	select {
	case code := <-done:
		if code != 200 {
			t.Fatalf("preflight=%d", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("preflight failed to complete")
	}
}
