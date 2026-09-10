package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDigestWindowsCachesOnlySettledFilesV90(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settling.bin")
	if err := os.WriteFile(path, []byte("cache after metadata settles"), 0644); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	a := &App{index: map[string]FileEntry{path: {Path: path, Size: st.Size(), MTime: st.ModTime().UnixNano()}}}
	first, err := a.ensureHash(path, "sha256")
	if err != nil {
		t.Fatal(err)
	}
	if a.index[path].HashIdentity != "" {
		t.Fatal("a freshly written digest was marked reusable")
	}
	time.Sleep(2100 * time.Millisecond)
	second, err := a.ensureHash(path, "sha256")
	if err != nil || second != first {
		t.Fatalf("settled digest changed: %s, %v", second, err)
	}
	if a.index[path].HashIdentity == "" {
		t.Fatal("Windows native identity did not enable reuse for a settled file")
	}
}
