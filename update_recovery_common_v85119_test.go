package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateRecoveryManifestV85119(t *testing.T) {
	good := recoveryManifestV85119{
		Version: "8.5.49-test.119",
		URL:     "https://raw.githubusercontent.com/AdyTZa619/DuplicateDownloadGuard-Releases/testing/test-releases/DuplicateDownloadGuard_PRO_TEST.exe",
		SHA256:  strings.Repeat("a", 64),
	}
	if err := validateRecoveryManifestV85119(good); err != nil {
		t.Fatalf("valid manifest rejected: %v", err)
	}
	badURL := good
	badURL.URL = "https://example.com/not-ddg.exe"
	if err := validateRecoveryManifestV85119(badURL); err == nil {
		t.Fatal("foreign update URL must be rejected")
	}
	badHash := good
	badHash.SHA256 = "1234"
	if err := validateRecoveryManifestV85119(badHash); err == nil {
		t.Fatal("invalid SHA-256 must be rejected")
	}
}

func TestRepairInterruptedExecutableV85119RestoresReplacing(t *testing.T) {
	dir := t.TempDir()
	current := filepath.Join(dir, "DuplicateDownloadGuard_PRO.exe")
	replacing := current + ".replacing"
	copying := current + ".copying"
	if err := os.WriteFile(replacing, []byte("old-good"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(copying, []byte("partial"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := repairInterruptedExecutableV85119(current); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(current)
	if err != nil || string(b) != "old-good" {
		t.Fatalf("current not restored: %q %v", string(b), err)
	}
	if _, err := os.Stat(copying); !os.IsNotExist(err) {
		t.Fatalf("stale .copying survived: %v", err)
	}
}

func TestRepairInterruptedExecutableV85119CleansTempsWhenCurrentExists(t *testing.T) {
	dir := t.TempDir()
	current := filepath.Join(dir, "DuplicateDownloadGuard_PRO.exe")
	if err := os.WriteFile(current, []byte("live"), 0755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(current+".copying", []byte("partial"), 0755)
	_ = os.WriteFile(current+".replacing", []byte("stale"), 0755)
	if err := repairInterruptedExecutableV85119(current); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{".copying", ".replacing"} {
		if _, err := os.Stat(current + suffix); !os.IsNotExist(err) {
			t.Fatalf("stale %s survived: %v", suffix, err)
		}
	}
}
