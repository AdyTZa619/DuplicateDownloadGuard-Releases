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
	// v8.5.7: the UI already persists folder changes through /api/config.
	// Reuse this cheap heartbeat to notice that configuration change and refresh
	// the live local index/results once, instead of leaving the old result table
	// stale until the next explicit remote scan.
	noteLocalFolderConfigHeartbeatV857(a)
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

// settleMegaOnShutdown waits for any cancelled MEGA worker to finish its own
// deferred session restoration. Once the global MEGA gate becomes available,
// it also tears down a warm/WebDAV preview and restores the session that was
// active before DDG opened the public folder. The whole cleanup is bounded so
// a broken MEGAcmd/network cannot keep the DDG executable locked forever.
func settleMegaOnShutdown(a *App) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := acquireMegaSession(ctx); err != nil {
		a.logf("MEGA shutdown: sesiunea nu s-a eliberat la timp: %v", err)
		return
	}
	defer releaseMegaSession()

	a.previewMu.Lock()
	st := a.preview
	if a.previewTTL != nil {
		a.previewTTL.Stop()
		a.previewTTL = nil
	}
	a.preview = MegaPreviewState{}
	a.previewMu.Unlock()

	if !st.Active || st.Exe == "" {
		return
	}
	if st.RemotePath != "" {
		out, err := runMegaTimed(ctx, 4*time.Second, st.Exe, "webdav", "-d", st.RemotePath)
		if err != nil && ctx.Err() == nil {
			a.logf("MEGA shutdown: WebDAV nu s-a oprit curat: %v • %s", err, sanitizeMega(out))
		}
	}
	if ctx.Err() != nil {
		return
	}
	// Keep the public-folder session/cache so a subsequent login <folder> --resume
	// can reuse the local MEGAcmd cache instead of rebuilding a large public
	// folder from scratch. The previously active user/account session is still
	// restored immediately below, so DDG does not leave MEGAcmd on the folder.
	_, _ = runMegaTimed(ctx, 4*time.Second, st.Exe, "logout", "--keep-session")
	if st.PreviousSession != "" && ctx.Err() == nil {
		out, err := runMegaTimed(ctx, 10*time.Second, st.Exe, "login", st.PreviousSession)
		if err != nil {
			a.logf("MEGA shutdown: sesiunea anterioară nu a putut fi restaurată: %v • %s", err, sanitizeMega(out))
		} else {
			a.logf("MEGA shutdown: sesiunea anterioară restaurată; cache-ul folderului public a fost păstrat")
		}
	}
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

	// Ensure MEGAcmd does not keep a public-folder/WebDAV session behind after
	// the UI is gone. This also waits briefly for cancelled MEGA workers to run
	// their deferred session cleanup. The public-folder cache itself is retained
	// so the next DDG start can use MEGAcmd's --resume path.
	settleMegaOnShutdown(a)

	// aria2 is a long-lived helper. Stop it explicitly instead of leaving an
	// orphan until Windows notices the parent process disappeared.
	shutdownAriaRPC(a)

	// Defensive final timer cleanup for the no-preview path.
	a.previewMu.Lock()
	if a.previewTTL != nil {
		a.previewTTL.Stop()
		a.previewTTL = nil
	}
	a.previewMu.Unlock()
}
