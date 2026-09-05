package main

import (
	"os"
	"path/filepath"
	"testing"
)

func mustWriteUpdaterTestFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestCleanupNativeUpdateArtifactsKeepsOnlyRequestedBackup(t *testing.T) {
	updates := t.TempDir()
	backupDir := filepath.Join(updates, "backup")
	keep := filepath.Join(backupDir, "DuplicateDownloadGuard_20260905_220000.exe")
	old1 := filepath.Join(backupDir, "DuplicateDownloadGuard_20260904_220000.exe")
	old2 := filepath.Join(backupDir, "DuplicateDownloadGuard_20260903_220000.exe")
	pending := filepath.Join(updates, "DuplicateDownloadGuard.pending.exe")
	request := filepath.Join(updates, "apply_update.json")
	helper := filepath.Join(updates, "DuplicateDownloadGuard.Updater_20260905_220000.exe")
	tmp1 := filepath.Join(updates, "stale.download")
	tmp2 := filepath.Join(backupDir, "stale.copying")

	for _, path := range []string{keep, old1, old2, pending, request, helper, tmp1, tmp2} {
		mustWriteUpdaterTestFile(t, path)
	}

	cleanupNativeUpdateArtifacts(updates, keep, "", true, true)

	if _, err := os.Stat(keep); err != nil {
		t.Fatalf("backupul păstrat a fost șters: %v", err)
	}
	for _, path := range []string{old1, old2, pending, request, helper, tmp1, tmp2} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("artefact vechi rămas: %s", path)
		}
	}
}

func TestCleanupNativeUpdateArtifactsPreUpdatePreservesCurrentHelperAndPending(t *testing.T) {
	updates := t.TempDir()
	backupDir := filepath.Join(updates, "backup")
	oldBackup := filepath.Join(backupDir, "DuplicateDownloadGuard_20260904_220000.exe")
	pending := filepath.Join(updates, "DuplicateDownloadGuard.pending.exe")
	request := filepath.Join(updates, "apply_update.json")
	currentHelper := filepath.Join(updates, "DuplicateDownloadGuard.Updater_20260905_220000.exe")
	oldHelper := filepath.Join(updates, "DuplicateDownloadGuard.Updater_20260904_220000.exe")

	for _, path := range []string{oldBackup, pending, request, currentHelper, oldHelper} {
		mustWriteUpdaterTestFile(t, path)
	}

	cleanupNativeUpdateArtifacts(updates, "", currentHelper, false, false)

	for _, path := range []string{pending, request, currentHelper} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("fișier activ șters prematur: %s: %v", path, err)
		}
	}
	for _, path := range []string{oldBackup, oldHelper} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("artefact vechi nu a fost curățat: %s", path)
		}
	}
}

func TestNativeCleanupPathGuard(t *testing.T) {
	updates := filepath.Join(t.TempDir(), "updates")
	inside := filepath.Join(updates, "backup", "x.exe")
	outside := filepath.Join(filepath.Dir(updates), "outside.exe")
	if !pathInside(updates, inside) {
		t.Fatal("cale internă respinsă")
	}
	if pathInside(updates, outside) {
		t.Fatal("cale externă acceptată")
	}
}
