package main

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"math/rand"
	"path/filepath"
	"testing"
)

func richFixtureFingerprintV90(seed int64) videoFingerprintV85 {
	rng := rand.New(rand.NewSource(seed))
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			img.SetRGBA(x, y, color.RGBA{uint8(rng.Intn(256)), uint8(rng.Intn(256)), uint8(rng.Intn(256)), 255})
		}
	}
	sig := makeImageSignatureV85(img)
	fp := videoFingerprintV85{Info: MediaInfo{OK: true, Duration: 60, Width: 1920, Height: 1080}, Hashes: make([]uint64, 7), Valid: make([]bool, 7), Frames: make([]imageSignatureV85, 7)}
	for i := range fp.Hashes {
		fp.Hashes[i], fp.Valid[i], fp.Frames[i] = sig.Hash, true, sig
	}
	return fp
}

func TestCachedRenamedVideoWinsBeyondOldShortlistV90(t *testing.T) {
	a := &App{appDir: t.TempDir()}
	entries := make([]FileEntry, 100)
	winner := richFixtureFingerprintV90(44)
	for i := range entries {
		e := FileEntry{Path: filepath.Join(a.appDir, fmt.Sprintf("item-%03d.mp4", i)), Name: fmt.Sprintf("item-%03d.mp4", i), Size: 1000, MTime: 1}
		fp := richFixtureFingerprintV90(int64(200 + i))
		if i == 99 {
			fp = winner
			e.Name = "completely-renamed.mkv"
			e.Size = 900000
		}
		entries[i] = e
		cacheLocalMediaInfo(a, e, fp.Info)
		cacheLocalVideoFingerprintV85(a, e, fp)
	}
	ctx := detectorCandidateContextV90(context.Background(), a, entries)
	got := a.videoCandidatesV90(ctx, winner, RemoteItem{Name: "item-000.mp4", Size: 1000}, entries)
	if len(got.Candidates) == 0 || got.Candidates[0].Path != entries[99].Path {
		t.Fatalf("renamed winner was hidden: %#v", got.Candidates)
	}
	if len(got.Candidates) > 12 {
		t.Fatal("decode budget exceeded")
	}
	if _, ok := cachedLocalMediaInfo(a, entries[0]); !ok {
		t.Fatal("candidate query deleted authoritative metadata")
	}
}

func TestUnexaminedDurationCandidatesRemainPendingV90(t *testing.T) {
	a := &App{appDir: t.TempDir()}
	entries := make([]FileEntry, 80)
	for i := range entries {
		e := FileEntry{Path: filepath.Join(a.appDir, fmt.Sprintf("%d.mp4", i)), Name: fmt.Sprintf("%d.mp4", i), Size: 1000, MTime: 1}
		entries[i] = e
		cacheLocalMediaInfo(a, e, MediaInfo{OK: true, Duration: 60})
	}
	ctx := detectorCandidateContextV90(context.Background(), a, entries)
	got := a.videoCandidatesV90(ctx, richFixtureFingerprintV90(99), RemoteItem{Name: "new.mp4", Size: 9000}, entries)
	if got.Pending == 0 || len(got.Candidates) > 12 {
		t.Fatalf("incomplete search lost its uncertainty: %#v", got)
	}
}

func BenchmarkDetectorCandidateSnapshotV90(b *testing.B) {
	a := &App{appDir: b.TempDir()}
	entries := make([]FileEntry, 10000)
	for i := range entries {
		entries[i] = FileEntry{Path: fmt.Sprintf("/corpus/%d.mp4", i), Name: fmt.Sprintf("%d.mp4", i), Size: int64(1000000 + i), MTime: 1}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		detectorCandidateContextV90(context.Background(), a, entries)
	}
}

func BenchmarkLegacyMediaCandidateScanV90(b *testing.B) {
	entries := make([]FileEntry, 10000)
	for i := range entries {
		entries[i] = FileEntry{Path: fmt.Sprintf("/corpus/%d.mp4", i), Name: fmt.Sprintf("%d.mp4", i), Size: int64(1000000 + i), MTime: 1}
	}
	remote := RemoteItem{Name: "remote.mp4", Size: 1000000}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mediaGuardCandidates(remote, entries, 5)
	}
}
