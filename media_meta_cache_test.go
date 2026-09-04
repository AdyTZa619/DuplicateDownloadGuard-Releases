package main

import "testing"

func TestDurationCompatibleV85(t *testing.T) {
	remote := MediaInfo{OK: true, Duration: 600, Width: 1920, Height: 1080}
	near := MediaInfo{OK: true, Duration: 600.8, Width: 1280, Height: 720}
	if ratio, ok := durationCompatibleV85(remote, near); !ok || ratio <= 0 {
		t.Fatalf("near re-encode should be compatible, ratio=%f ok=%v", ratio, ok)
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
	e := FileEntry{Path: `D:\video\clip.mp4`, Size: 1000, MTime: 1234}
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
