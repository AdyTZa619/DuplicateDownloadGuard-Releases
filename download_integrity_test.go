package main

import (
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
