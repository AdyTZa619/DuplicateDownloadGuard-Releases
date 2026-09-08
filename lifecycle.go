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
	// TEST119: Edge can recycle/recreate an app window or transiently disappear
	// from EnumWindows during a renderer/GPU failure. Twelve seconds was still
	// aggressive enough to kill a usable backend. A close hint must now be
	// accompanied by 90 consecutive seconds without a DDG native window.
	uiWatchdogMissingWindowTicksV85112 = 90
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

// shouldStopUIWatchdogV85112 keeps the lifecycle decision isolated and
// testable. TEST119 deliberately makes native-window absence a sustained,
// high-confidence condition. Edge renderer/GPU crashes, desktop switching,
// minimize/suspend and temporary title/handle recreation are not enough to
// kill the local backend.
func shouldStopUIWatchdogV85112(now time.Time, lastNS, hintNS int64, windowPresent bool, missingWindowTicks int) bool {
	if lastNS <= 0 || windowPresent || missingWindowTicks < uiWatchdogMissingWindowTicksV85112 {
		return false
	}
	if hintNS <= 0 {
		return false
	}

	hint := time.Unix(0, hintNS)
	// A real close has all three signals: no newer heartbeat than pagehide,
	// the native DDG window is gone, and that absence persisted long enough.
	// The extra age guard prevents a delayed watchdog tick from turning a fresh
	// renderer pagehide into an application shutdown.
	return lastNS <= hintNS && now.Sub(hint) > 30*time.Second
}

// startUIWatchdog terminates the local backend only after the DDG app window
// is genuinely gone following an explicit UI exit hint. pagehide is a hint,
// not a shutdown command: Edge can emit it during reload/navigation. Likewise,
// stale heartbeats or temporary native-window invisibility during minimize,
// screen lock, sleep/resume or desktop switching must never kill a healthy DDG.
func startUIWatchdog(stop chan<- struct{}) {
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		missingWindowTicks := 0
		for now := range ticker.C {
			if !uiSeen.Load() {
				continue
			}
			lastNS := uiHeartbeatNS.Load()
			if lastNS <= 0 {
				continue
			}

			windowPresent := ddgAppWindowPresentNative()
			if windowPresent {
				missingWindowTicks = 0
			} else if missingWindowTicks < uiWatchdogMissingWindowTicksV85112 {
				missingWindowTicks++
			}

			hintNS := uiExitHintNS.Load()
			if shouldStopUIWatchdogV85112(now, lastNS, hintNS, windowPresent, missingWindowTicks) {
				// Persist the exact watchdog evidence before stopping. If DDG is ever
				// found OFFLINE again, backend_last_exit.json tells us immediately
				// whether lifecycle logic was responsible instead of guessing.
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
	// No previous MEGA account/session existed when DDG opened this public
	// folder. Do not log out the folder just to log it back in on the next run.
	// The hint contains only the public URL, never a MEGA session token.
	if st.PreviousSession == "" && st.SourceURL != "" {
		a.saveMegaPreviewRestartHintV859(st.SourceURL)
		a.logf("MEGA shutdown: sesiunea folderului public a rămas activă pentru preview rapid la următoarea pornire")
		return
	}

	// A real previous session existed. Preserve the public-folder cache but put
	// MEGAcmd back exactly where the user had it before DDG opened the folder.
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

	// Stop a scan/tool operation first.
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

	// Stop WebDAV cleanly and preserve/restore the appropriate MEGA session.
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
