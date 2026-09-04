package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFindDownloadedMegaFileRejectsStaleExistingTarget(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "target.bin")
	if err := os.WriteFile(path, []byte("12345"), 0644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	res := Result{Remote: RemoteItem{Name: "target.bin", Size: 5}}
	started := time.Now()
	if got := findDownloadedMegaFile(dir, res, started); got != "" {
		t.Fatalf("stale file was accepted as a fresh MEGA result: %s", got)
	}

	now := time.Now()
	if err := os.Chtimes(path, now, now); err != nil {
		t.Fatal(err)
	}
	if got := findDownloadedMegaFile(dir, res, started); got != path {
		t.Fatalf("fresh exact result not found: got %q want %q", got, path)
	}
}

func TestFindDownloadedMegaFileRejectsAmbiguousFreshFallbacks(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"renamed-a.bin", "renamed-b.bin"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("12345"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	res := Result{Remote: RemoteItem{Name: "target.bin", Size: 5}}
	if got := findDownloadedMegaFile(dir, res, time.Now()); got != "" {
		t.Fatalf("ambiguous same-size fallback was guessed as success: %s", got)
	}
}
