package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func resetLocalAudioSegmentCacheStateV85ForTest() {
	localAudioSegmentCacheStateV85.Lock()
	localAudioSegmentCacheStateV85.AppDir = ""
	localAudioSegmentCacheStateV85.Loaded = false
	localAudioSegmentCacheStateV85.Dirty = false
	localAudioSegmentCacheStateV85.Generation = 0
	localAudioSegmentCacheStateV85.Entries = nil
	localAudioSegmentCacheStateV85.Unlock()
}

func TestLocalAudioSegmentCachePersistsAndReloads(t *testing.T) {
	resetLocalAudioSegmentCacheStateV85ForTest()
	defer resetLocalAudioSegmentCacheStateV85ForTest()
	a := &App{appDir: t.TempDir()}
	path := filepath.Join(a.appDir, "clip.mp4")
	if err := os.WriteFile(path, []byte("media bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	want := []uint32{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	if err := cacheLocalAudioSegmentV85(a, path, 12.5, 12, want); err != nil {
		t.Fatal(err)
	}
	if err := flushLocalAudioSegmentCacheV85(a); err != nil {
		t.Fatal(err)
	}
	resetLocalAudioSegmentCacheStateV85ForTest()
	got, ok := cachedLocalAudioSegmentV85(a, path, 12.5, 12)
	if !ok || !reflect.DeepEqual(got, want) {
		t.Fatalf("persistent audio cache miss: ok=%v got=%v", ok, got)
	}
}

func TestLocalAudioSegmentCacheInvalidatesWhenFileChanges(t *testing.T) {
	resetLocalAudioSegmentCacheStateV85ForTest()
	defer resetLocalAudioSegmentCacheStateV85ForTest()
	a := &App{appDir: t.TempDir()}
	path := filepath.Join(a.appDir, "clip.mp4")
	if err := os.WriteFile(path, []byte("abcdefgh"), 0600); err != nil {
		t.Fatal(err)
	}
	fp := []uint32{11, 12, 13, 14, 15, 16, 17, 18}
	if err := cacheLocalAudioSegmentV85(a, path, 4, 12, fp); err != nil {
		t.Fatal(err)
	}
	if _, ok := cachedLocalAudioSegmentV85(a, path, 4, 12); !ok {
		t.Fatal("fresh audio cache entry was not found")
	}
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatal(err)
	}
	if _, ok := cachedLocalAudioSegmentV85(a, path, 4, 12); ok {
		t.Fatal("mtime change must invalidate cached audio fingerprint")
	}
}

func TestAudioSegmentKeySeparatesDifferentOffsets(t *testing.T) {
	path := `D:\media\clip.mp4`
	if audioSegmentKeyV85(path, 10, 12) == audioSegmentKeyV85(path, 11, 12) {
		t.Fatal("different audio offsets must not share cache key")
	}
	if audioSegmentKeyV85(path, 10, 12) == audioSegmentKeyV85(path, 10, 15) {
		t.Fatal("different segment durations must not share cache key")
	}
}
