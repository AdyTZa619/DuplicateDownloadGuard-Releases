package main

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

var (
	uiHeartbeatNS atomic.Int64
	uiExitHintNS  atomic.Int64
	uiSeen        atomic.Bool
	appStopOnce   sync.Once
)

func (a *App) handleUIHeartbeat(w http.ResponseWriter, r *http.Request) {
	now := time.Now().UnixNano()
	uiHeartbeatNS.Store(now)
	uiSeen.Store(true)
	// A fresh heartbeat cancels a pagehide hint caused by a reload/navigation.
	uiExitHintNS.Store(0)
	jsonOut(w, map[string]any{"ok": true})
}

func (a *App) handleUIExitHint(w http.ResponseWriter, r *http.Request) {
	if uiSeen.Load() {
		uiExitHintNS.Store(time.Now().UnixNano())
	}
	w.WriteHeader(http.StatusNoContent)
}

// startUIWatchdog terminates the local backend after the app-mode UI has gone
// away. pagehide gives a fast close path, while the heartbeat fallback is kept
// deliberately generous because Edge can throttle timers when minimized and a
// sleeping Windows machine can resume after a long wall-clock gap.
func startUIWatchdog(stop chan<- struct{}) {
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for now := range ticker.C {
			if !uiSeen.Load() {
				continue
			}
			lastNS := uiHeartbeatNS.Load()
			if lastNS <= 0 {
				continue
			}
			last := time.Unix(0, lastNS)
			hintNS := uiExitHintNS.Load()
			shouldStop := now.Sub(last) > 90*time.Second
			if hintNS > 0 {
				hint := time.Unix(0, hintNS)
				// If no heartbeat newer than pagehide arrived, this was a real
				// window close rather than a reload/navigation.
				shouldStop = lastNS <= hintNS && now.Sub(hint) > 4*time.Second
			}
			if shouldStop {
				appStopOnce.Do(func() {
					select {
					case stop <- struct{}{}:
					default:
					}
				})
				return
			}
		}
	}()
}

func shutdownApp(a *App) {
	if a == nil {
		return
	}

	// Stop a scan/tool operation first.
	a.mu.Lock()
	cancel := a.cancel
	a.cancel = nil
	a.mu.Unlock()
	if cancel != nil {
		cancel()
	}

	// Persist the queue in a restart-safe state and cancel every active worker.
	if raw, ok := queueRegistry.Load(a); ok {
		q := raw.(*DownloadQueue)
		var cancels []context.CancelFunc
		now := time.Now().Unix()
		q.mu.Lock()
		for _, job := range q.Jobs {
			if job.Status == "running" || job.Status == "queued" {
				job.Status = "paused"
				job.Stage = "pauză sigură la închiderea aplicației"
				job.Error = "Aplicația a fost închisă; apasă Resume pentru continuare."
				job.UpdatedAt = now
				job.GuardVersion = 0
			}
		}
		for _, c := range q.Cancels {
			if c != nil {
				cancels = append(cancels, c)
			}
		}
		q.mu.Unlock()
		for _, c := range cancels {
			c()
		}
		q.save(a)
	}

	// aria2 is a long-lived helper. Stop it explicitly instead of leaving an
	// orphan until Windows notices the parent process disappeared.
	shutdownAriaRPC(a)

	// Stop the preview timeout from firing while the process is shutting down.
	a.previewMu.Lock()
	if a.previewTTL != nil {
		a.previewTTL.Stop()
		a.previewTTL = nil
	}
	a.previewMu.Unlock()
}
