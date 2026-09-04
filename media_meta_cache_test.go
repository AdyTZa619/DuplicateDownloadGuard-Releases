package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDurationCompatibleV85(t *testing.T) {
	remote := MediaInfo{OK: true, Duration: 600, Width: 1920, Height: 1080}
	near := MediaInfo{OK: true, Duration: 600.8, Width: 1280, Height: 720}
	if ratio, ok := durationCompatibleV85(remote, near); !ok || ratio <= 0 {
		t.Fatalf("near re-encode should be compatible, ratio=%f ok=%v", ratio, ok)
	}
	withIntro := MediaInfo{OK: true, Duration: 630, Width: 1920, Height: 1080}
	if ratio, ok := durationCompatibleV85(remote, withIntro); !ok || ratio < .04 || ratio > .06 {
		t.Fatalf("modest intro/outro variant should remain a candidate, ratio=%f ok=%v", ratio, ok)
	}
	wrongDuration := MediaInfo{OK: true, Duration: 500, Width: 1920, Height: 1080}
	if _, ok := durationCompatibleV85(remote, wrongDuration); ok {
		t.Fatal("materially different duration must be rejected")
	}
	wrongAspect := MediaInfo{OK: true, Duration: 600, Width: 1080, Height: 1920}
	if _, ok := durationCompatibleV85(remote, wrongAspect); ok {
		t.Fatal("materially different aspect ratio must be rejected")
	}
}

func TestLocalMediaMetaCacheInvalidatesOnFileChange(t *testing.T) {
	a := &App{appDir: t.TempDir()}
	e := FileEntry{Path: `D:\video\clip.mp4`, Name: "clip.mp4", Size: 1000, MTime: 1234}
	info := MediaInfo{OK: true, Source: "LOCAL", Duration: 42, Width: 1920, Height: 1080}
	cacheLocalMediaInfo(a, e, info)
	if err := saveLocalMediaMetaCache(a); err != nil {
		t.Fatal(err)
	}
	if got, ok := cachedLocalMediaInfo(a, e); !ok || got.Duration != 42 {
		t.Fatalf("cached metadata missing: %#v ok=%v", got, ok)
	}
	changed := e
	changed.Size++
	if _, ok := cachedLocalMediaInfo(a, changed); ok {
		t.Fatal("cache must invalidate when file size changes")
	}
	changed = e
	changed.MTime++
	if _, ok := cachedLocalMediaInfo(a, changed); ok {
		t.Fatal("cache must invalidate when mtime changes")
	}
}

func TestLocalMediaFailureCacheInvalidatesOnFileChange(t *testing.T) {
	a := &App{appDir: t.TempDir()}
	e := FileEntry{Path: `D:\video\broken.mp4`, Name: "broken.mp4", Size: 777, MTime: 50}
	cacheLocalMediaFailureV85(a, e, "invalid media")
	if !cachedLocalMediaFailureV85(a, e) {
		t.Fatal("unreadable media failure was not cached")
	}
	if _, ok := cachedLocalMediaInfo(a, e); ok {
		t.Fatal("unreadable media must never be exposed as valid cached metadata")
	}
	changed := e
	changed.Size++
	if cachedLocalMediaFailureV85(a, changed) {
		t.Fatal("negative cache must invalidate when size changes")
	}
	changed = e
	changed.MTime++
	if cachedLocalMediaFailureV85(a, changed) {
		t.Fatal("negative cache must invalidate when mtime changes")
	}
}

func TestPruneLocalMediaMetaCacheRemovesStaleEntries(t *testing.T) {
	a := &App{appDir: t.TempDir()}
	keep := FileEntry{Path: `D:\video\keep.mp4`, Name: "keep.mp4", Size: 1000, MTime: 10}
	stale := FileEntry{Path: `D:\video\stale.mp4`, Name: "stale.mp4", Size: 2000, MTime: 20}
	info := MediaInfo{OK: true, Source: "LOCAL", Duration: 10, Width: 1920, Height: 1080}
	cacheLocalMediaInfo(a, keep, info)
	cacheLocalMediaInfo(a, stale, info)
	if !pruneLocalMediaMetaCache(a, []FileEntry{keep}) {
		t.Fatal("prune should report a removed stale entry")
	}
	if _, ok := cachedLocalMediaInfo(a, keep); !ok {
		t.Fatal("valid cache entry was removed")
	}
	if _, ok := cachedLocalMediaInfo(a, stale); ok {
		t.Fatal("stale cache entry was not removed")
	}
}

func TestReplaceCacheFileV85ReplacesExistingDestination(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cache.json")
	tmp := path + ".tmp"
	if err := os.WriteFile(path, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tmp, []byte("new"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := replaceCacheFileV85(tmp, path); err != nil {
		t.Fatalf("replace cache file failed: %v", err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "new" {
		t.Fatalf("cache content=%q, want new", string(b))
	}
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Fatalf("temporary cache still exists: %v", err)
	}
}
