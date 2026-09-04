package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func queueActionRequest(t *testing.T, a *App, body string) {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/api/queue/action", strings.NewReader(body))
	w := httptest.NewRecorder()
	a.handleQueueAction(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("queue action failed: %d %s", w.Code, w.Body.String())
	}
}

func TestQueueActionsClearStaleRuntimeState(t *testing.T) {
	a := &App{appDir: t.TempDir()}
	cancelled := 0
	q := &DownloadQueue{
		Cancels: map[string]context.CancelFunc{
			"run": func() { cancelled++ },
		},
		Jobs: []*DownloadJob{
			{ID: "run", Status: "running", Stage: "downloading", SpeedBps: 1234, ETA: 99, GuardVersion: downloadGuardVersion},
			{ID: "wait", Status: "queued", Stage: "queued", SpeedBps: 88, ETA: 12, GuardVersion: downloadGuardVersion},
			{ID: "failed", Status: "failed", Stage: "old failure", Error: "old", ErrorCode: "MEGA_TIMEOUT", ErrorTitle: "old title", ErrorAction: "old action", SpeedBps: 777, ETA: 44, FinishedAt: 123},
		},
		Started: true,
	}
	queueRegistry.Store(a, q)
	t.Cleanup(func() { queueRegistry.Delete(a) })

	queueActionRequest(t, a, `{"action":"pause-all","ids":[]}`)
	if cancelled != 1 {
		t.Fatalf("running worker cancel count=%d, want 1", cancelled)
	}
	for _, id := range []string{"run", "wait"} {
		var job *DownloadJob
		for _, candidate := range q.Jobs {
			if candidate.ID == id {
				job = candidate
				break
			}
		}
		if job == nil || job.Status != "paused" || job.Stage != "pus pe pauză" || job.SpeedBps != 0 || job.ETA != 0 || job.GuardVersion != 0 {
			t.Fatalf("bad paused state for %s: %#v", id, job)
		}
	}

	queueActionRequest(t, a, `{"action":"resume","ids":["failed"]}`)
	failed := q.Jobs[2]
	if failed.Status != "queued" || failed.Stage != "în așteptare" || failed.Error != "" || failed.ErrorCode != "" || failed.ErrorTitle != "" || failed.ErrorAction != "" || failed.SpeedBps != 0 || failed.ETA != 0 || failed.FinishedAt != 0 || failed.GuardVersion != 0 {
		t.Fatalf("stale state survived resume: %#v", failed)
	}

	queueActionRequest(t, a, `{"action":"stop-all","ids":[]}`)
	for _, job := range q.Jobs {
		if job.Status != "cancelled" || job.Stage != "oprit de utilizator" || job.SpeedBps != 0 || job.ETA != 0 {
			t.Fatalf("bad stopped state: %#v", job)
		}
	}
}
