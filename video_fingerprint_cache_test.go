package main

import "testing"

func testVideoFingerprintV85() videoFingerprintV85 {
	return videoFingerprintV85{
		Info:   MediaInfo{OK: true, Duration: 60, Width: 1920, Height: 1080},
		Hashes: []uint64{1, 2, 3, 4, 5, 6, 7},
		Valid:  []bool{true, true, true, true, true, true, true},
	}
}

func TestVideoFingerprintCacheInvalidatesOnFileChange(t *testing.T) {
	a := &App{appDir: t.TempDir()}
	e := FileEntry{Path: `D:\video\clip.mp4`, Name: "clip.mp4", Size: 1000, MTime: 1234}
	cacheLocalVideoFingerprintV85(a, e, testVideoFingerprintV85())
	if err := flushLocalVideoFingerprintCacheV85(a); err != nil {
		t.Fatal(err)
	}
	if got, ok := cachedLocalVideoFingerprintV85(a, e); !ok || got.Hashes[3] != 4 {
		t.Fatalf("cached fingerprint missing: %#v ok=%v", got, ok)
	}
	changed := e
	changed.Size++
	if _, ok := cachedLocalVideoFingerprintV85(a, changed); ok {
		t.Fatal("fingerprint cache must invalidate when size changes")
	}
	changed = e
	changed.MTime++
	if _, ok := cachedLocalVideoFingerprintV85(a, changed); ok {
		t.Fatal("fingerprint cache must invalidate when mtime changes")
	}
}

func TestVideoFingerprintCachePrunesStaleEntries(t *testing.T) {
	a := &App{appDir: t.TempDir()}
	keep := FileEntry{Path: `D:\video\keep.mp4`, Name: "keep.mp4", Size: 1000, MTime: 10}
	stale := FileEntry{Path: `D:\video\stale.mp4`, Name: "stale.mp4", Size: 2000, MTime: 20}
	fp := testVideoFingerprintV85()
	cacheLocalVideoFingerprintV85(a, keep, fp)
	cacheLocalVideoFingerprintV85(a, stale, fp)
	if !pruneLocalVideoFingerprintCacheV85(a, []FileEntry{keep}) {
		t.Fatal("prune should remove stale fingerprint")
	}
	if _, ok := cachedLocalVideoFingerprintV85(a, keep); !ok {
		t.Fatal("valid fingerprint was removed")
	}
	if _, ok := cachedLocalVideoFingerprintV85(a, stale); ok {
		t.Fatal("stale fingerprint remains")
	}
}
