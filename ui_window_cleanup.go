package main

import (
	"os"
	"path/filepath"
	"strings"
)

const (
	ddgAppWindowTitle        = "Duplicate Download Guard Pro"
	updateHandoffRequestName = "apply_update.json"
)

// isDDGAppWindowTitle is intentionally strict and is used only during ordinary
// startup cleanup. We must never close a normal browser window just because one
// tab happens to contain DDG text.
func isDDGAppWindowTitle(title string) bool {
	return strings.EqualFold(strings.TrimSpace(title), ddgAppWindowTitle)
}

// isDDGAppWindowPresenceTitle is deliberately more tolerant than the cleanup
// matcher. Edge can temporarily decorate an --app window title (for example
// with a browser/profile suffix) while minimized, suspended, restored or while
// the renderer is being recreated. For lifecycle presence detection a false
// positive is harmless (the backend stays alive a little longer), while a
// false negative can kill a healthy backend and leave the visible UI offline.
func isDDGAppWindowPresenceTitle(title string) bool {
	t := strings.ToLower(strings.TrimSpace(title))
	if t == "" {
		return false
	}
	return strings.Contains(t, strings.ToLower(ddgAppWindowTitle))
}

func updateHandoffMarkerPathForRoot(root string) string {
	return filepath.Join(root, "data", "updates", updateHandoffRequestName)
}

func updateHandoffPendingAtRoot(root string) bool {
	st, err := os.Stat(updateHandoffMarkerPathForRoot(root))
	return err == nil && !st.IsDir()
}

func postUpdateHandoffPending() bool {
	return updateHandoffPendingAtRoot(executableDir())
}

// Normal DDG launches are single-instance per portable installation. During an
// updater handoff, the previous backend has intentionally exited but its Edge
// --app shell can survive and display a stale OFFLINE UI. TEST126 detects the
// handoff marker and closes that tolerant DDG window before the new UI is
// created. Updater/recovery helper modes remain independent application modes.
func init() {
	if runningNativeUpdaterMode(os.Args) {
		return
	}

	// Claim the per-install mutex first. A second launch must only restore the
	// already-running healthy DDG instance and must never perform cleanup around
	// it. TEST126's mutex is path-scoped, so an old TEST125 global mutex cannot
	// block a successful update handoff.
	if !claimDDGSingleInstanceNative() {
		activateExistingDDGWindowNative()
		os.Exit(0)
	}

	if postUpdateHandoffPending() {
		// apply_update.json proves this is the freshly replaced executable. The
		// old Edge shell can carry a decorated title, so use the tolerant handoff
		// matcher here. This is intentionally not used on ordinary launches.
		closeDDGPresenceWindowsForHandoffNative()
	} else {
		closeDDGAppWindowsNative()
	}

	// Migration/recovery path for pre-single-instance backends. Kill only stale
	// DDG processes whose full executable path is exactly this installation.
	terminateOtherDDGProcessesSameImageNative()
}
