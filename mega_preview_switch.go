package main

import (
	"context"
	"errors"
	"time"
)

const megaWebDAVURLMissingV85 = "MEGAcmd a activat WebDAV, dar nu a returnat URL-ul de streaming"

type megaWebDAVRunnerV85 func(timeout time.Duration, args ...string) (string, error)

type megaWebDAVSwitchResultV85 struct {
	StreamURL   string
	StartOutput string
}

// switchSameSourceWebDAVV85 starts and validates the requested WebDAV node but
// deliberately does NOT wait for cleanup of the previous node. Cleanup is a
// lifecycle concern and must never keep the player waiting after the new stream
// URL is already available.
func switchSameSourceWebDAVV85(old MegaPreviewState, remoteRef string, run megaWebDAVRunnerV85) (megaWebDAVSwitchResultV85, error) {
	if run == nil {
		return megaWebDAVSwitchResultV85{}, errors.New("MEGA WebDAV runner lipsă")
	}
	if remoteRef == "" {
		return megaWebDAVSwitchResultV85{}, errors.New("referință MEGA remote lipsă")
	}

	out, err := run(15*time.Second, "webdav", remoteRef)
	result := megaWebDAVSwitchResultV85{StartOutput: out}
	if err != nil {
		return result, err
	}
	streamURL := extractWebDAVURL(out, remoteRef)
	if streamURL == "" {
		listing, _ := run(3*time.Second, "webdav")
		streamURL = extractWebDAVURL(listing, remoteRef)
	}
	if streamURL == "" {
		if old.RemotePath != remoteRef {
			_, _ = run(4*time.Second, "webdav", "-d", remoteRef)
		}
		return result, errors.New(megaWebDAVURLMissingV85)
	}
	result.StreamURL = streamURL
	return result, nil
}

// schedulePreviousMegaPreviewCleanupV86 is deliberately separate from the
// switch itself so latency and cleanup can be tested independently. It returns
// immediately after scheduling the old node removal.
func schedulePreviousMegaPreviewCleanupV86(old MegaPreviewState, newRemoteRef string, delay time.Duration, run megaWebDAVRunnerV85) bool {
	if run == nil || old.Exe == "" || old.RemotePath == "" || old.RemotePath == newRemoteRef || old.RemotePath == megaWarmRootRefV86 {
		return false
	}
	go func() {
		if delay > 0 {
			time.Sleep(delay)
		}
		_, _ = run(5*time.Second, "webdav", "-d", old.RemotePath)
	}()
	return true
}

func (a *App) cleanupPreviousMegaPreviewAsyncV86(old MegaPreviewState, newRemoteRef string) {
	run := func(timeout time.Duration, args ...string) (string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		out, err := runMegaTimed(ctx, timeout, old.Exe, args...)
		if err != nil {
			a.logf("MEGA preview: cleanup WebDAV vechi în fundal: %v • %s", err, sanitizeMega(out))
		}
		return out, err
	}
	_ = schedulePreviousMegaPreviewCleanupV86(old, newRemoteRef, 1200*time.Millisecond, run)
}

func (a *App) switchMegaPreviewSameSourceV85(old MegaPreviewState, remoteRef string) (string, error) {
	ctx := context.Background()
	run := func(timeout time.Duration, args ...string) (string, error) {
		return runMegaTimed(ctx, timeout, old.Exe, args...)
	}
	result, err := switchSameSourceWebDAVV85(old, remoteRef, run)
	if err != nil {
		if err.Error() == megaWebDAVURLMissingV85 {
			return "", err
		}
		problem := classifyMegaProblem(result.StartOutput, err)
		return "", newMegaProblemError(problem, result.StartOutput)
	}

	// Critical latency fix: return the new URL immediately and clean the old node
	// in the background instead of blocking the HTTP response for up to 12 s.
	a.cleanupPreviousMegaPreviewAsyncV86(old, remoteRef)
	return result.StreamURL, nil
}
