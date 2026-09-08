//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestMain cleans up the TEST119 recovery helper spawned by package init.
// Without this, Windows keeps the helper EXE mapped inside go-build's temp
// directory and `go test` exits non-zero when the Go tool tries to remove it.
func TestMain(m *testing.M) {
	code := m.Run()

	name := fmt.Sprintf("DuplicateDownloadGuard.recovery_%d.exe", os.Getpid())
	cmd := exec.Command("taskkill.exe", "/IM", name, "/T", "/F")
	hideChildWindow(cmd)
	_ = cmd.Run()

	if exe, err := os.Executable(); err == nil {
		path := filepath.Join(filepath.Dir(exe), "data", "updates", name)
		deadline := time.Now().Add(1500 * time.Millisecond)
		for {
			if err := os.Remove(path); err == nil || os.IsNotExist(err) || time.Now().After(deadline) {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
	}

	os.Exit(code)
}
