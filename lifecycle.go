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

const (
	// Slow fallback: if Edge loses/recreates its app window in an unusual way,
	// keep the conservative protection that avoids killing a healthy backend.
	uiWatchdogMissingWindowTicksV85112 = 90
	// TEST122: a real click on X destroys the already-latched native HWND. Once
	// that exact handle is gone and no replacement DDG window exists, a short
	// two-second grace is enough to distinguish a real close from a reload.
	uiNativeCloseGraceV85122 = 2 * time.Second
	// TEST132: Edge does not guarantee that pagehide/sendBeacon is delivered
	// when its --app window is closed.  The native HWND is the authoritative
	// signal in that case.  Require several consecutive observations so a very
	// short shell recreation can still recover without losing the backend.
	uiNativeWindowGoneTicksV85132 = 3
	// Normal close must not leave the backend visible in Task Manager for tens
	// of seconds merely because MEGAcmd's control pipe is wedged.
	uiShutdownMegaBudgetV85132       = 2 * time.Second
	uiShutdownPreviewLogBudgetV85132 = 750 * time.Millisecond
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

// shouldStopUIWatchdogV85112 is the conservative fallback. It is deliberately
// slow because it covers situations where Edge/GPU/desktop switching can make
// window enumeration temporarily unreliable.
func shouldStopUIWatchdogV85112(now time.Time, lastNS, hintNS int64, windowPresent bool, missingWindowTicks int) bool {
	if lastNS <= 0 || windowPresent || missingWindowTicks < uiWatchdogMissingWindowTicksV85112 {
		return false
	}
	if hintNS <= 0 {
		return false
	}

	hint := time.Unix(0, hintNS)
	return lastNS <= hintNS && now.Sub(hint) > 30*time.Second
}

func shouldStopNativeCloseV85122(now time.Time, lastNS, hintNS int64, windowPresent, exactWindowDestroyed bool) bool {
	if lastNS <= 0 || hintNS <= 0 || windowPresent || !exactWindowDestroyed {
		return false
	}
	if lastNS > hintNS {
		return false
	}
	return now.Sub(time.Unix(0, hintNS)) >= uiNativeCloseGraceV85122
}

func shouldStopNativeWindowGoneV85132(windowPresent, exactWindowDestroyed bool, goneTicks int) bool {
	return !windowPresent && exactWindowDestroyed && goneTicks >= uiNativeWindowGoneTicksV85132
}

// startUIWatchdog has two paths:
//  1. fast, high-confidence graceful close when the exact HWND that DDG had
//     latched is destroyed and no replacement DDG window exists;
//  2. the old conservative 90-second fallback for uncertain Edge states.
//
// This makes X close DDG promptly without reintroducing the OFFLINE regression
// caused by renderer reloads/minimize/sleep.
func startUIWatchdog(stop chan<- struct{}) {
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		missingWindowTicks := 0
		nativeWindowGoneTicks := 0
		for now := range ticker.C {
			if !uiSeen.Load() {
				continue
			}
			lastNS := uiHeartbeatNS.Load()
			if lastNS <= 0 {
				continue
			}

			exactWindowDestroyed := ddgNativeWindowDefinitelyClosedV85122()
			windowPresent := ddgAppWindowPresentNative()
			if windowPresent {
				missingWindowTicks = 0
				nativeWindowGoneTicks = 0
			} else if missingWindowTicks < uiWatchdogMissingWindowTicksV85112 {
				missingWindowTicks++
				if exactWindowDestroyed {
					nativeWindowGoneTicks++
				} else {
					nativeWindowGoneTicks = 0
				}
			}

			hintNS := uiExitHintNS.Load()
			if shouldStopNativeWindowGoneV85132(windowPresent, exactWindowDestroyed, nativeWindowGoneTicks) {
				writeBackendExitDiagnosticV85119("ui_native_window_destroyed_without_beacon", now, lastNS, hintNS, windowPresent, missingWindowTicks)
				appStopOnce.Do(func() {
					select {
					case stop <- struct{}{}:
					default:
					}
				})
				return
			}
			if shouldStopNativeCloseV85122(now, lastNS, hintNS, windowPresent, exactWindowDestroyed) {
				writeBackendExitDiagnosticV85119("ui_native_window_confirmed_close", now, lastNS, hintNS, windowPresent, missingWindowTicks)
				appStopOnce.Do(func() {
					select {
					case stop <- struct{}{}:
					default:
					}
				})
				return
			}
			if shouldStopUIWatchdogV85112(now, lastNS, hintNS, windowPresent, missingWindowTicks) {
				writeBackendExitDiagnosticV85119("ui_watchdog_confirmed_close", now, lastNS, hintNS, windowPresent, missingWindowTicks)
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

// settleMegaOnShutdown waits for any MEGA control command already owning the
// shared session gate. It deliberately does not execute per-route `webdav -d`:
// real Windows diagnostics showed that command can wedge MEGAcmd's shared pipe.
// v8.5.9 deliberately keeps the public-folder MEGAcmd session active when DDG
// did not replace a previous account/session. That lets the next DDG start try
// WebDAV directly instead of paying another long login --resume cycle. If DDG
// did replace a previous session, the old session is still restored exactly as
// before.
func settleMegaOnShutdown(a *App) {
	ctx, cancel := context.WithTimeout(context.Background(), uiShutdownMegaBudgetV85132)
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
	if st.PreviousSession == "" && st.SourceURL != "" {
		a.saveMegaPreviewRestartHintV859(st.SourceURL)
		a.logf("MEGA shutdown: sesiunea folderului public a rămas activă pentru preview rapid la următoarea pornire")
		return
	}

	a.clearMegaPreviewRestartHintV859()
	_, _ = runMegaControlTimed(ctx, 4*time.Second, st.Exe, "logout", "--keep-session")
	if st.PreviousSession != "" && ctx.Err() == nil {
		out, err := runMegaControlTimed(ctx, 10*time.Second, st.Exe, "login", st.PreviousSession)
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

	// Stop a scan/tool operation first. exec.CommandContext children use the
	// Windows tree-kill cancellation path, so gallery-dl/yt-dlp cannot keep DDG
	// alive after the user closes the app.
	a.mu.Lock()
	cancel := a.cancel
	a.cancel = nil
	a.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	a.closeMegaPreviewControllerV8526("închiderea aplicației")

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

	settleMegaOnShutdown(a)
	shutdownAriaRPC(a)

	a.previewMu.Lock()
	if a.previewTTL != nil {
		a.previewTTL.Stop()
		a.previewTTL = nil
	}
	a.previewMu.Unlock()

	// TEST122: recovery is intentionally detached so it can survive a crash,
	// but a normal X-close is not a crash. Kill only the recovery helper that
	// lives under THIS portable installation, otherwise Windows keeps an extra
	// DuplicateDownloadGuard.recovery_*.exe process for up to 30 minutes.
	stopRecoveryHelpersForAppDirV85122(a.appDir)

	// Persist all preview diagnostics accepted before close and prevent their
	// background worker from touching portable files after shutdown completes.
	_ = flushLocalPreviewLogsV85130(uiShutdownPreviewLogBudgetV85132)
}
