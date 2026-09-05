//go:build windows

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// recoverMegaControlServerV8529 is used only after MegaClient explicitly
// reports Windows error 231. MEGAcmd has one background server process; killing
// only that process is safer than killing MegaClient process trees and lets the
// server reload its persisted session/cache on restart.
func recoverMegaControlServerV8529(exe string) (string, error) {
	exe = strings.TrimSpace(exe)
	if exe == "" {
		return "", fmt.Errorf("cale MegaClient lipsă")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	kill := exec.CommandContext(ctx, "taskkill.exe", "/F", "/IM", "MEGAcmdServer.exe")
	hideControlWindow(kill)
	killOutput, killErr := kill.CombinedOutput()
	if ctx.Err() != nil {
		return strings.TrimSpace(string(killOutput)), fmt.Errorf("oprirea MEGAcmdServer a expirat: %w", ctx.Err())
	}

	serverPath := filepath.Join(filepath.Dir(exe), "MEGAcmdServer.exe")
	if _, err := os.Stat(serverPath); err != nil {
		return strings.TrimSpace(string(killOutput)), fmt.Errorf("MEGAcmdServer.exe nu există lângă MegaClient.exe: %w", err)
	}
	server := exec.Command(serverPath)
	hideControlWindow(server)
	server.Dir = filepath.Dir(serverPath)
	if err := server.Start(); err != nil {
		return strings.TrimSpace(string(killOutput)), fmt.Errorf("MEGAcmdServer.exe nu a putut fi repornit: %w", err)
	}
	_ = server.Process.Release()

	// A killed server may need a short interval to release/recreate its named
	// pipes. The following per-file command remains the actual readiness probe.
	time.Sleep(900 * time.Millisecond)
	result := "target=MEGAcmdServer.exe restart=started"
	if output := strings.TrimSpace(string(killOutput)); output != "" {
		result += " • taskkill=" + output
	}
	if killErr != nil {
		// The server may already have exited; starting the known sibling binary is
		// still a valid recovery. Preserve the taskkill result only for diagnostics.
		result += " • taskkill-error=" + killErr.Error()
	}
	return result, nil
}
