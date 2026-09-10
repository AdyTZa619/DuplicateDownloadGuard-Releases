package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestDigestRevalidatesChangedAndReplacedFilesV90(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.bin")
	if err := os.WriteFile(path, []byte("original"), 0644); err != nil {
		t.Fatal(err)
	}
	st, _ := os.Stat(path)
	a := &App{index: map[string]FileEntry{path: {Path: path, Size: st.Size(), MTime: st.ModTime().UnixNano()}}}
	first, err := a.ensureHash(path, "sha256")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("modified"), 0644); err != nil {
		t.Fatal(err)
	}
	second, err := a.ensureHash(path, "sha256")
	if err != nil || first == second {
		t.Fatalf("stale digest reused: %s, %v", second, err)
	}
	tmp := filepath.Join(dir, "new.bin")
	if err := os.WriteFile(tmp, []byte("replaced"), 0644); err != nil {
		t.Fatal(err)
	}
	st, _ = os.Stat(path)
	if err := os.Chtimes(tmp, st.ModTime(), st.ModTime()); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, path); err != nil {
		t.Fatal(err)
	}
	third, err := a.ensureHash(path, "sha256")
	if err != nil || third == second {
		t.Fatalf("replacement kept old digest: %s, %v", third, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := a.ensureHashContextV90(ctx, path, "sha256"); err == nil {
		t.Fatal("cancelled hash was accepted")
	}
}

func TestDigestRapidRewritesWithRestoredTimestampV90(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rapid.bin")
	if err := os.WriteFile(path, []byte("00000000"), 0644); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	a := &App{index: map[string]FileEntry{path: {Path: path, Size: st.Size(), MTime: st.ModTime().UnixNano()}}}
	for i := 0; i < 32; i++ {
		payload := []byte(fmt.Sprintf("%08d", i))
		if err := os.WriteFile(path, payload, 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, st.ModTime(), st.ModTime()); err != nil {
			t.Fatal(err)
		}
		got, err := a.ensureHash(path, "sha256")
		want := fmt.Sprintf("%x", sha256.Sum256(payload))
		if err != nil || got != want {
			t.Fatalf("rewrite %d reused stale content: got %s, want %s, err %v", i, got, want, err)
		}
	}
}
