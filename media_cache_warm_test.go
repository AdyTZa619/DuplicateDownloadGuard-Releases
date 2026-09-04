package main

import "testing"

func TestMergeWarmEntriesKeepsOnlyMediaAndDeduplicates(t *testing.T) {
	a := FileEntry{Path: `D:\media\a.jpg`, Name: "a.jpg", Size: 10, MTime: 1}
	b := FileEntry{Path: `D:\media\b.mp4`, Name: "b.mp4", Size: 20, MTime: 2}
	text := FileEntry{Path: `D:\media\note.txt`, Name: "note.txt", Size: 30, MTime: 3}
	newerA := a
	newerA.Size = 11
	newerA.MTime = 4
	got := mergeWarmEntriesV85([]FileEntry{a}, []FileEntry{b, text, newerA})
	if len(got) != 2 {
		t.Fatalf("len=%d, want 2 media entries: %#v", len(got), got)
	}
	foundA := false
	foundB := false
	for _, e := range got {
		switch e.Name {
		case "a.jpg":
			foundA = true
			if e.Size != 11 || e.MTime != 4 {
				t.Fatalf("duplicate path did not keep newest snapshot: %#v", e)
			}
		case "b.mp4":
			foundB = true
		}
	}
	if !foundA || !foundB {
		t.Fatalf("missing expected entries: %#v", got)
	}
}

func TestForegroundIdleForWarmRespectsBusySignals(t *testing.T) {
	a := &App{}
	if !foregroundIdleForWarmV85(a) {
		t.Fatal("fresh app should be idle for warmup")
	}
	a.opRunning.Store(true)
	if foregroundIdleForWarmV85(a) {
		t.Fatal("opRunning must block warmup")
	}
	a.opRunning.Store(false)
	a.guardMu.Lock()
	if foregroundIdleForWarmV85(a) {
		a.guardMu.Unlock()
		t.Fatal("active Download Guard must block warmup")
	}
	a.guardMu.Unlock()
	if !foregroundIdleForWarmV85(a) {
		t.Fatal("warmup should resume after foreground guard releases")
	}
}
