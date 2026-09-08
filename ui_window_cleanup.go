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

// isDDGAppWindowTitle is intentionally strict and is used only when DDG wants
// to close an old dedicated app window during startup/update handoff. We must
// never close a normal browser window just because one tab contains DDG.
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

// Normal DDG launches are single-instance. TEST125 also performs one migration
// cleanup for older builds: those builds could have their Edge window closed
// while their localhost backend stayed alive, so repeated launches accumulated
// several DuplicateDownloadGuard_PRO processes. Updater/recovery helper modes
// remain independent and are never treated as application instances.
func init() {
	if runningNativeUpdaterMode(os.Args) {
		return
	}

	// Once a TEST125+ instance owns the mutex, a second launch simply restores
	// the existing DDG window and exits. It must not close or restart the healthy
	// instance.
	if !claimDDGSingleInstanceNative() {
		activateExistingDDGWindowNative()
		os.Exit(0)
	}

	// Migration path for pre-TEST125 instances, which do not own the mutex.
	// Close any stale Edge --app window and then terminate only backend processes
	// running the exact same executable path. This removes the orphan processes
	// visible in Task Manager without touching DDG copies in other folders.
	closeDDGAppWindowsNative()
	terminateOtherDDGProcessesSameImageNative()
}
