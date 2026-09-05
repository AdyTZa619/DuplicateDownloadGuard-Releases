package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeCleanupTestFile(t *testing.T, path string, mod time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if !mod.IsZero() {
		if err := os.Chtimes(path, mod, mod); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPostHealthCleanupRefusesOldHealthMarker(t *testing.T) {
	appDir := t.TempDir()
	updates := filepath.Join(appDir, "updates")
	stale := filepath.Join(updates, "DuplicateDownloadGuard.pending.exe")
	writeCleanupTestFile(t, stale, time.Time{})
	writeCleanupTestFile(t, updateHealthPath(appDir), time.Time{})
	if err := os.WriteFile(updateHealthPath(appDir), []byte("0.0.1 old\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if postHealthUpdaterCleanup(appDir) {
		t.Fatal("cleanup permis cu health marker de versiune veche")
	}
	if _, err := os.Stat(stale); err != nil {
		t.Fatalf("fișier șters înainte de health-ul versiunii curente: %v", err)
	}
}

func TestPostHealthCleanupKeepsNewestBackupAndRemovesStaleArtifacts(t *testing.T) {
	appDir := t.TempDir()
	updates := filepath.Join(appDir, "updates")
	backupDir := filepath.Join(updates, "backup")
	oldBackup := filepath.Join(backupDir, "DuplicateDownloadGuard_20260904_120000.exe")
	newBackup := filepath.Join(backupDir, "DuplicateDownloadGuard_20260905_120000.exe")
	base := time.Now().Add(-2 * time.Hour)
	writeCleanupTestFile(t, oldBackup, base)
	writeCleanupTestFile(t, newBackup, base.Add(time.Hour))

	stale := []string{
		filepath.Join(updates, "DuplicateDownloadGuard.Updater_20260904_120000.exe"),
		filepath.Join(updates, "DuplicateDownloadGuard.pending.exe"),
		filepath.Join(updates, "apply_update.json"),
		filepath.Join(updates, "stale.download"),
		filepath.Join(backupDir, "stale.copying"),
	}
	for _, path := range stale {
		writeCleanupTestFile(t, path, time.Time{})
	}
	if err := os.MkdirAll(updates, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(updateHealthPath(appDir), []byte(appVersion+"\nhealthy"), 0644); err != nil {
		t.Fatal(err)
	}

	if !postHealthUpdaterCleanup(appDir) {
		t.Fatal("cleanup refuzat după health marker curent")
	}
	if _, err := os.Stat(newBackup); err != nil {
		t.Fatalf("cel mai nou backup a fost șters: %v", err)
	}
	if _, err := os.Stat(oldBackup); !os.IsNotExist(err) {
		t.Fatalf("backup vechi rămas: %v", err)
	}
	for _, path := range stale {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("artefact vechi rămas: %s", path)
		}
	}
}

func TestRunningNativeUpdaterModeSkipsStartupCleanup(t *testing.T) {
	if !runningNativeUpdaterMode([]string{"ddg.exe", nativeUpdaterModeArg, "request.json"}) {
		t.Fatal("modul updater nativ nu a fost detectat")
	}
	if !runningNativeUpdaterMode([]string{"ddg.exe", nativeUpdaterCleanupModeArg, "1", "x", "y", "z"}) {
		t.Fatal("modul cleanup nativ nu a fost detectat")
	}
	if runningNativeUpdaterMode([]string{"ddg.exe"}) {
		t.Fatal("pornirea normală a fost confundată cu updaterul")
	}
}
