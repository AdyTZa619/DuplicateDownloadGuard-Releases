package main

import (
	"context"
	"errors"
	"fmt"
	"time"
)

type megaWebDAVRunnerV85 func(timeout time.Duration, args ...string) (string, error)

type megaWebDAVSwitchResultV85 struct {
	StreamURL      string
	StartOutput    string
	CleanupWarning error
}

// switchSameSourceWebDAVV85 deliberately starts and validates the new WebDAV
// node before stopping the old one. A failed switch therefore leaves the
// currently playing preview intact instead of creating an avoidable gap.
func switchSameSourceWebDAVV85(old MegaPreviewState, remoteRef string, run megaWebDAVRunnerV85) (megaWebDAVSwitchResultV85, error) {
	if run == nil {
		return megaWebDAVSwitchResultV85{}, errors.New("MEGA WebDAV runner lipsă")
	}
	if remoteRef == "" {
		return megaWebDAVSwitchResultV85{}, errors.New("referință MEGA remote lipsă")
	}

	out, err := run(30*time.Second, "webdav", remoteRef)
	result := megaWebDAVSwitchResultV85{StartOutput: out}
	if err != nil {
		return result, err
	}
	streamURL := extractWebDAVURL(out, remoteRef)
	if streamURL == "" {
		listing, _ := run(10*time.Second, "webdav")
		streamURL = extractWebDAVURL(listing, remoteRef)
	}
	if streamURL == "" {
		// The new node may have been enabled even though MEGAcmd did not return a
		// usable URL. Clean only the new node; never touch the known-good old one.
		if old.RemotePath != remoteRef {
			_, _ = run(10*time.Second, "webdav", "-d", remoteRef)
		}
		return result, errors.New("MEGAcmd a activat WebDAV, dar nu a returnat URL-ul de streaming")
	}
	result.StreamURL = streamURL

	if old.RemotePath != "" && old.RemotePath != remoteRef {
		if cleanupOut, cleanupErr := run(12*time.Second, "webdav", "-d", old.RemotePath); cleanupErr != nil {
			result.CleanupWarning = fmt.Errorf("cleanup WebDAV vechi: %w: %s", cleanupErr, sanitizeMega(cleanupOut))
		}
	}
	return result, nil
}

func (a *App) switchMegaPreviewSameSourceV85(old MegaPreviewState, remoteRef string) (string, error) {
	ctx := context.Background()
	run := func(timeout time.Duration, args ...string) (string, error) {
		return runMegaTimed(ctx, timeout, old.Exe, args...)
	}
	result, err := switchSameSourceWebDAVV85(old, remoteRef, run)
	if err != nil {
		problem := classifyMegaProblem(result.StartOutput, err)
		// Missing stream URL is a local WebDAV response problem, not a quota or
		// login problem; preserve the precise diagnostic in that case.
		if result.StartOutput == "" && err.Error() == "MEGAcmd a activat WebDAV, dar nu a returnat URL-ul de streaming" {
			return "", err
		}
		return "", newMegaProblemError(problem, result.StartOutput)
	}
	if result.CleanupWarning != nil {
		a.logf("MEGA preview: noul stream este activ, dar cleanup-ul celui vechi a eșuat: %v", result.CleanupWarning)
	}
	return result.StreamURL, nil
}
