package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDetectorFingerprintPersistsAcrossReloadV90(t *testing.T) {
	a := &App{appDir: t.TempDir()}
	e := FileEntry{Path: filepath.Join(a.appDir, "video.mp4"), Name: "video.mp4", Size: 1000, MTime: 5}
	fp := richFixtureFingerprintV90(200)
	cacheLocalVideoFingerprintV85(a, e, fp)
	if err := flushLocalVideoFingerprintCacheV85(a); err != nil {
		t.Fatal(err)
	}
	localVideoFingerprintCacheState.Lock()
	localVideoFingerprintCacheState.Loaded = false
	localVideoFingerprintCacheState.Unlock()
	restarted := &App{appDir: a.appDir}
	got, ok := cachedLocalVideoFingerprintV85(restarted, e)
	if !ok || !richVideoFingerprintUsableV90(got) || got.Frames[3].PHash != fp.Frames[3].PHash {
		t.Fatal("persistent fingerprint was not reloaded")
	}
	if _, err := os.Stat(filepath.Join(a.appDir, "video_fingerprint_cache.json")); !os.IsNotExist(err) {
		t.Fatal("9.0 TEST overwrote the stable fingerprint cache")
	}
	changed := e
	changed.MTime++
	if _, ok := cachedLocalVideoFingerprintV85(restarted, changed); ok {
		t.Fatal("changed file reused old fingerprint")
	}
}

func TestRemoteFingerprintCacheIsBoundToValidatorV90(t *testing.T) {
	a := &App{appDir: t.TempDir()}
	s := &detectorRemoteV90{Original: "https://example.invalid/media.mp4", Size: 1000, Validator: `"version-one"`}
	ctx := context.WithValue(context.Background(), detectorRemoteKeyV90{}, s)
	fp := richFixtureFingerprintV90(100)
	saveRemoteFingerprintV90(a, ctx, fp)
	if _, ok := cachedRemoteFingerprintV90(&App{appDir: a.appDir}, ctx); !ok {
		t.Fatal("remote fingerprint did not persist")
	}
	s.Validator = `"version-two"`
	if _, ok := cachedRemoteFingerprintV90(a, ctx); ok {
		t.Fatal("changed remote reused cached fingerprint")
	}
	s.Validator = ""
	if _, ok := cachedRemoteFingerprintV90(a, ctx); ok {
		t.Fatal("remote without validator reused cached fingerprint")
	}
}
