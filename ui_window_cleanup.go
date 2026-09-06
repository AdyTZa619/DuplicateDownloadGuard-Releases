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

// isDDGAppWindowTitle is intentionally strict. We only close the dedicated
// Edge --app window whose title is exactly the DDG application title. We do
// not match ordinary browser windows such as
// "Duplicate Download Guard Pro - Microsoft Edge", because closing one of
// those could also close unrelated tabs in the same browser window.
func isDDGAppWindowTitle(title string) bool {
	return strings.EqualFold(strings.TrimSpace(title), ddgAppWindowTitle)
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

// Each normal DDG process opens its Edge --app window only later, from main().
// Therefore every exact-title DDG app window that exists during init belongs
// to an older process/UI instance. Close it on every normal startup, not only
// when apply_update.json is still present. This also recovers from a failed or
// partially completed updater handoff where the old Edge window survived but
// its localhost backend is already gone (the UI would otherwise display
// "Monitor local indisponibil").
func init() {
	// Updater helper modes run from the same executable and must never touch UI
	// windows. Only a real application launch performs stale-window cleanup.
	if runningNativeUpdaterMode(os.Args) {
		return
	}
	closeDDGAppWindowsNative()
}
