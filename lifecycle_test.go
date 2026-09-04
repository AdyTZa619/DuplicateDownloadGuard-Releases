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

func TestSettleMegaOnShutdownPersistsDDGOwnedRootV8510(t *testing.T) {
	source := "https://mega.nz/folder/example#key"
	rootURL := "http://127.0.0.1:4443/public-root/"
	a := &App{
		appDir: t.TempDir(),
		preview: MegaPreviewState{
			Active:     true,
			SourceURL:  source,
			RemotePath: megaWarmRootRefV86,
			StreamURL:  rootURL,
			Exe:        "MegaClient.exe",
		},
		previewTTL: time.NewTimer(time.Hour),
	}

	settleMegaOnShutdown(a)

	if a.preview.Active || a.previewTTL != nil {
		t.Fatal("in-memory preview state must still be cleared on app shutdown")
	}
	hint, ok := a.loadMegaPreviewRestartHintV8510(source)
	if !ok {
		t.Fatal("persistent root hint was not saved")
	}
	if hint.RootURL != rootURL {
		t.Fatalf("saved root=%q want=%q", hint.RootURL, rootURL)
	}
}
