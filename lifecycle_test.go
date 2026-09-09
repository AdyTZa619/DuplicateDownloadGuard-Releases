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
	last := now.Add(-50 * time.Second).UnixNano()
	hint := now.Add(-45 * time.Second).UnixNano()
	if shouldStopUIWatchdogV85112(now, last, hint, true, 1000) {
		t.Fatal("pagehide must never stop DDG while the native app window still exists")
	}
}

func TestUIWatchdogRequiresSustainedWindowAbsenceV85112(t *testing.T) {
	now := time.Now()
	last := now.Add(-50 * time.Second).UnixNano()
	hint := now.Add(-45 * time.Second).UnixNano()
	if shouldStopUIWatchdogV85112(now, last, hint, false, uiWatchdogMissingWindowTicksV85112-1) {
		t.Fatal("a transient missing-window observation must not stop DDG")
	}
	if !shouldStopUIWatchdogV85112(now, last, hint, false, uiWatchdogMissingWindowTicksV85112) {
		t.Fatal("a real close should stop DDG after sustained native-window absence")
	}
}

func TestUIWatchdogFreshExitHintNeedsAgeGuardV85119(t *testing.T) {
	now := time.Now()
	last := now.Add(-25 * time.Second).UnixNano()
	hint := now.Add(-20 * time.Second).UnixNano()
	if shouldStopUIWatchdogV85112(now, last, hint, false, uiWatchdogMissingWindowTicksV85112) {
		t.Fatal("a recent Edge pagehide must not kill the backend even if native-window enumeration is temporarily empty")
	}
}

func TestUIWatchdogFreshHeartbeatCancelsPagehideV85112(t *testing.T) {
	now := time.Now()
	hint := now.Add(-50 * time.Second).UnixNano()
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

func TestUIWatchdogDoesNotKillIdleBackendWhenWindowEnumerationTemporarilyFailsV85115(t *testing.T) {
	now := time.Now()
	last := now.Add(-2 * time.Hour).UnixNano()
	if shouldStopUIWatchdogV85112(now, last, 0, false, 10000) {
		t.Fatal("idle/minimize/lock without an explicit pagehide hint must never stop DDG")
	}
}

func TestUIWatchdogStillStopsAfterExplicitExitHintV85119(t *testing.T) {
	now := time.Now()
	hint := now.Add(-45 * time.Second).UnixNano()
	last := now.Add(-46 * time.Second).UnixNano()
	if !shouldStopUIWatchdogV85112(now, last, hint, false, uiWatchdogMissingWindowTicksV85112) {
		t.Fatal("explicit old exit hint plus sustained native-window absence should stop DDG")
	}
}

func TestNativeWindowCloseDoesNotRequirePagehideBeaconV85132(t *testing.T) {
	if shouldStopNativeWindowGoneV85132(false, true, uiNativeWindowGoneTicksV85132-1) {
		t.Fatal("native close must survive the short replacement grace interval")
	}
	if !shouldStopNativeWindowGoneV85132(false, true, uiNativeWindowGoneTicksV85132) {
		t.Fatal("destroyed native DDG window must stop the backend even without pagehide/sendBeacon")
	}
}

func TestNativeWindowCloseGuardRejectsReplacementOrUnlatchedWindowV85132(t *testing.T) {
	if shouldStopNativeWindowGoneV85132(true, true, 100) {
		t.Fatal("a replacement DDG window proves the application is still open")
	}
	if shouldStopNativeWindowGoneV85132(false, false, 100) {
		t.Fatal("missing enumeration without a destroyed latched HWND is not enough to stop DDG")
	}
}

func TestDestroyedNativeWindowRemainsLatchedAcrossWatchdogTicksV85133(t *testing.T) {
	var latch ddgWindowLatchStateV85133
	if !latch.observe(false, 101) {
		t.Fatal("the first matching DDG HWND must be latched")
	}

	for tick := 1; tick <= uiNativeWindowGoneTicksV85132; tick++ {
		if latch.observe(false, 0) {
			t.Fatalf("destroyed window reported present on tick %d", tick)
		}
		if !latch.definitelyClosed() {
			t.Fatalf("destroyed evidence was lost on tick %d", tick)
		}
		// startUIWatchdog calls its presence probe after its exact-close probe.
		// This second observation must not erase the destruction evidence.
		if latch.observe(false, 0) || !latch.definitelyClosed() {
			t.Fatalf("presence probe erased the destroyed latch on tick %d", tick)
		}
	}

	if !latch.observe(false, 202) || latch.definitelyClosed() {
		t.Fatal("a replacement DDG HWND must cancel the close decision")
	}
}
