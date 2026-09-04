package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestVerifyDownloadedAgainstRemoteChecksExactSizeWithoutHash(t *testing.T) {
	path := filepath.Join(t.TempDir(), "download.bin")
	if err := os.WriteFile(path, []byte("12345"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := verifyDownloadedAgainstRemote(path, RemoteItem{Size: 5}); err != nil {
		t.Fatalf("matching size rejected: %v", err)
	}
	if err := verifyDownloadedAgainstRemote(path, RemoteItem{Size: 6}); err == nil {
		t.Fatal("truncated/wrong-size download was accepted without a hash")
	}
}

func TestVerifyDownloadedAgainstRemoteAllowsApproximateMetadataSize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "media.bin")
	if err := os.WriteFile(path, []byte("12345"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := verifyDownloadedAgainstRemote(path, RemoteItem{Size: 999, ApproxSize: true}); err != nil {
		t.Fatalf("approximate source size must not be enforced as exact: %v", err)
	}
}

func TestInternalDownloadClosesPartAndProducesFinalFile(t *testing.T) {
	data := []byte("durable internal download payload")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "33")
		_, _ = w.Write(data)
	}))
	defer server.Close()

	dir := t.TempDir()
	path, err := internalDownload(context.Background(), server.URL, dir, "result.bin", func(int64, int64) {})
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(data) {
		t.Fatalf("got %q, want %q", got, data)
	}
	if _, err := os.Stat(path + ".part"); !os.IsNotExist(err) {
		t.Fatalf("part file survived successful finalization: %v", err)
	}
}
