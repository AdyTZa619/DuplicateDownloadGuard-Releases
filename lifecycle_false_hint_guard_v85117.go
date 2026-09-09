package main

import "time"

const uiFalseExitHintGraceV85117 = 6 * time.Second

// Edge can emit pagehide while the --app window itself remains alive (renderer
// recycle, suspend/resume, internal navigation). If that hint survives because
// renderer timers/heartbeats are then throttled, the old watchdog could later
// interpret a temporary native-window miss as a real close.
//
// A real user close removes the native window quickly. A hint that is already
// older than this grace period while the native DDG window is still valid is
// therefore stale and must not be allowed to kill the backend later.
func clearFalseUIExitHintV85117(now time.Time, hintNS int64, windowPresent bool) bool {
	if hintNS <= 0 || !windowPresent {
		return false
	}
	hint := time.Unix(0, hintNS)
	return now.Sub(hint) >= uiFalseExitHintGraceV85117
}

func init() {
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for now := range ticker.C {
			hint := uiExitHintNS.Load()
			if hint <= 0 {
				continue
			}
			if !clearFalseUIExitHintV85117(now, hint, ddgAppWindowPresentNative()) {
				continue
			}
			uiExitHintNS.CompareAndSwap(hint, 0)
		}
	}()
}
