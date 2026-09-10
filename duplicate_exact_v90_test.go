package main

import (
	"context"
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
