package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCopyFileDurableReplacesAndPreservesContent(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.exe")
	dst := filepath.Join(dir, "target.exe")
	want := []byte("MZ-new-ddg-binary")
	if err := os.WriteFile(src, want, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := copyFileDurable(src, dst); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestWaitForExpectedHealthRejectsStaleVersion(t *testing.T) {
	dir := t.TempDir()
	health := filepath.Join(dir, "health.ok")
	if err := os.WriteFile(health, []byte("8.3.0\nold"), 0644); err != nil {
		t.Fatal(err)
	}
	if waitForExpectedHealth(health, "8.3.1", 20*time.Millisecond) {
		t.Fatal("stale health file was accepted")
	}
	if err := os.WriteFile(health, []byte("8.3.1 Pro\nnow"), 0644); err != nil {
		t.Fatal(err)
	}
	if !waitForExpectedHealth(health, "8.3.1", 20*time.Millisecond) {
		t.Fatal("expected version was not accepted")
	}
	if !waitForExpectedHealth(health, "", 20*time.Millisecond) {
		t.Fatal("fresh local-update health was not accepted")
	}
}

func TestValidateNativeUpdateRequest(t *testing.T) {
	dir := t.TempDir()
	sum := sha256.Sum256([]byte("x"))
	req := nativeUpdateRequest{
		Current: filepath.Join(dir, "current.exe"), Pending: filepath.Join(dir, "pending.exe"),
		Backup: filepath.Join(dir, "backup.exe"), Health: filepath.Join(dir, "health.ok"),
		Log: filepath.Join(dir, "updater.log"), ExpectedVersion: "8.3.1",
		ExpectedSHA256: hex.EncodeToString(sum[:]),
	}
	if err := validateNativeUpdateRequest(req); err != nil {
		t.Fatal(err)
	}
}
