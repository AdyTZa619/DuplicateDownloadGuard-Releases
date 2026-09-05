package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsDDGAppWindowTitleStrict(t *testing.T) {
	if !isDDGAppWindowTitle("Duplicate Download Guard Pro") {
		t.Fatal("exact DDG app title must match")
	}
	if !isDDGAppWindowTitle("  Duplicate Download Guard Pro  ") {
		t.Fatal("surrounding whitespace should be ignored")
	}
	for _, title := range []string{
		"Duplicate Download Guard Pro - Microsoft Edge",
		"Duplicate Download Guard Pro - Google Chrome",
		"Duplicate Download Guard",
		"Other window",
	} {
		if isDDGAppWindowTitle(title) {
			t.Fatalf("must not match ordinary browser/unrelated title %q", title)
		}
	}
}

func TestCleanupModeIsExcludedFromWindowHandoffCleanup(t *testing.T) {
	for _, args := range [][]string{
		{"ddg.exe", nativeUpdaterModeArg, "request.json"},
		{"ddg.exe", nativeUpdaterCleanupModeArg, "123", `C:\data\updates`, `C:\data\updates\backup\old.exe`, `C:\data\updates\helper.exe`},
	} {
		if !runningNativeUpdaterMode(args) {
			t.Fatalf("updater helper args must be excluded from UI window cleanup: %v", args)
		}
	}
	if runningNativeUpdaterMode([]string{"ddg.exe"}) {
		t.Fatal("normal DDG launch must not be treated as updater helper mode")
	}
}

func TestUpdateHandoffPendingAtRoot(t *testing.T) {
	root := t.TempDir()
	if updateHandoffPendingAtRoot(root) {
		t.Fatal("marker must not be reported before it exists")
	}
	marker := updateHandoffMarkerPathForRoot(root)
	if err := os.MkdirAll(filepath.Dir(marker), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(marker, []byte(`{"parentPid":123}`), 0600); err != nil {
		t.Fatal(err)
	}
	if !updateHandoffPendingAtRoot(root) {
		t.Fatal("existing apply_update.json must mark a post-update handoff")
	}
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	if updateHandoffPendingAtRoot(root) {
		t.Fatal("removed marker must no longer trigger cleanup")
	}
}
