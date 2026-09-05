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

// isDDGAppWindowTitle is intentionally strict. During the one-time post-update
// cleanup we only close the dedicated Edge app window whose title is exactly
// the DDG application title. We do not match ordinary browser windows such as
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

// The native updater keeps apply_update.json until the freshly started version
// writes its health marker. That gives the new executable a safe, precise way
// to know it is a post-update launch. The previous backend is already gone at
// this point, but its Edge --app window may still be visible. Close that stale
// window before main() opens the new UI.
func init() {
	// Both updater helper modes run from the same freshly installed EXE. The
	// cleanup helper must never close the normal post-update UI window.
	if runningNativeUpdaterMode(os.Args) {
		return
	}
	if !postUpdateHandoffPending() {
		return
	}
	closeDDGAppWindowsNative()
}
