package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestShutdownAppPausesActiveQueueAndPersistsIt(t *testing.T) {
	a := &App{appDir: t.TempDir()}
	cancelled := make(chan struct{}, 1)
	q := &DownloadQueue{
		Cancels: map[string]context.CancelFunc{
			"run": func() { cancelled <- struct{}{} },
		},
		Jobs: []*DownloadJob{
			{ID: "run", Status: "running", GuardVersion: downloadGuardVersion},
			{ID: "queued", Status: "queued", GuardVersion: downloadGuardVersion},
			{ID: "done", Status: "completed", GuardVersion: downloadGuardVersion},
		},
	}
	queueRegistry.Store(a, q)
	t.Cleanup(func() { queueRegistry.Delete(a) })

	shutdownApp(a)

	if q.Jobs[0].Status != "paused" || q.Jobs[1].Status != "paused" {
		t.Fatalf("active jobs were not paused: %#v %#v", q.Jobs[0], q.Jobs[1])
	}
	if q.Jobs[2].Status != "completed" {
		t.Fatalf("completed job changed state: %#v", q.Jobs[2])
	}
	if q.Jobs[0].GuardVersion != 0 || q.Jobs[1].GuardVersion != 0 {
		t.Fatal("paused jobs must be rechecked by ExactGuard after restart")
	}
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("running worker context was not cancelled")
	}

	b, err := os.ReadFile(filepath.Join(a.appDir, "download_queue.json"))
	if err != nil {
		t.Fatalf("queue was not persisted: %v", err)
	}
	var saved []*DownloadJob
	if err := json.Unmarshal(b, &saved); err != nil {
		t.Fatal(err)
	}
	if len(saved) != 3 || saved[0].Status != "paused" || saved[1].Status != "paused" || saved[2].Status != "completed" {
		t.Fatalf("unexpected saved queue: %#v", saved)
	}
}

func TestSettleMegaOnShutdownClearsWarmPreview(t *testing.T) {
	a := &App{
		appDir:     t.TempDir(),
		preview:    MegaPreviewState{Active: true, SourceURL: "https://mega.nz/folder/test"},
		previewTTL: time.NewTimer(time.Hour),
	}
	settleMegaOnShutdown(a)
	if a.preview.Active || a.preview.SourceURL != "" {
		t.Fatalf("MEGA preview survived shutdown: %#v", a.preview)
	}
	if a.previewTTL != nil {
		t.Fatal("MEGA preview timer survived shutdown")
	}
}

func TestUIWatchdogDoesNotStopOnPagehideWhileWindowExistsV85112(t *testing.T) {
	now := time.Now()
	last := now.Add(-20 * time.Second).UnixNano()
	hint := now.Add(-15 * time.Second).UnixNano()
	if shouldStopUIWatchdogV85112(now, last, hint, true, 100) {
		t.Fatal("pagehide must never stop DDG while the native app window still exists")
	}
}

func TestUIWatchdogRequiresSustainedWindowAbsenceV85112(t *testing.T) {
	now := time.Now()
	last := now.Add(-20 * time.Second).UnixNano()
	hint := now.Add(-15 * time.Second).UnixNano()
	if shouldStopUIWatchdogV85112(now, last, hint, false, uiWatchdogMissingWindowTicksV85112-1) {
		t.Fatal("a transient missing-window observation must not stop DDG")
	}
	if !shouldStopUIWatchdogV85112(now, last, hint, false, uiWatchdogMissingWindowTicksV85112) {
		t.Fatal("a real close should stop DDG after sustained native-window absence")
	}
}

func TestUIWatchdogFreshHeartbeatCancelsPagehideV85112(t *testing.T) {
	now := time.Now()
	hint := now.Add(-20 * time.Second).UnixNano()
	last := now.Add(-2 * time.Second).UnixNano()
	if shouldStopUIWatchdogV85112(now, last, hint, false, uiWatchdogMissingWindowTicksV85112) {
		t.Fatal("a heartbeat newer than pagehide proves the UI recovered/reloaded")
	}
}

func TestUIWatchdogDoesNotKillMinimizedOrSuspendedWindowV85112(t *testing.T) {
	now := time.Now()
	last := now.Add(-30 * time.Minute).UnixNano()
	if shouldStopUIWatchdogV85112(now, last, 0, true, 1000) {
		t.Fatal("stale heartbeat alone must not kill a backend whose native window still exists")
	}
}

func TestUIWatchdogFallbackNeedsWindowGoneAndVeryStaleHeartbeatV85112(t *testing.T) {
	now := time.Now()
	last := now.Add(-11 * time.Minute).UnixNano()
	if !shouldStopUIWatchdogV85112(now, last, 0, false, uiWatchdogMissingWindowTicksV85112) {
		t.Fatal("orphan backend should eventually stop when the native window is gone and heartbeat is very stale")
	}
}
