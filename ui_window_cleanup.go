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
