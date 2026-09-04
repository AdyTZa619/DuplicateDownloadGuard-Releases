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

// schedulePreviousMegaPreviewCleanupV86 remains a small pure scheduling helper
// used by regression tests and non-App callers.
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

// cleanupPreviousMegaPreviewAsyncV86 used to launch an ungated MEGAcmd command
// 1.2 s after every file switch. Several quick previews could therefore overlap
// cleanup commands with the next user-triggered WebDAV start even though
// MEGAcmd has one process-wide session. v8.5.9 makes cleanup strictly
// low-priority: it runs only if the MEGA gate is immediately available, never
// touches the currently active node, and is bounded to a short best-effort
// command. Skipping cleanup is safer than delaying the player's next click.
func (a *App) cleanupPreviousMegaPreviewAsyncV86(old MegaPreviewState, newRemoteRef string) {
	if a == nil || old.Exe == "" || old.RemotePath == "" || old.RemotePath == newRemoteRef || old.RemotePath == megaWarmRootRefV86 {
		return
	}
	go func() {
		time.Sleep(1500 * time.Millisecond)

		gateCtx, gateCancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
		defer gateCancel()
		if err := acquireMegaSession(gateCtx); err != nil {
			// A user operation owns or is about to own MEGA. Do not queue behind it.
			return
		}
		defer releaseMegaSession()

		// A later click may have returned to the node that this older cleanup was
		// going to remove. Never tear down the currently active stream.
		a.previewMu.Lock()
		current := a.preview
		a.previewMu.Unlock()
		if current.Active && current.Exe == old.Exe && current.RemotePath == old.RemotePath {
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
		defer cancel()
		out, err := runMegaTimed(ctx, 1500*time.Millisecond, old.Exe, "webdav", "-d", old.RemotePath)
		if err != nil && ctx.Err() == nil {
			a.logf("MEGA preview: cleanup WebDAV vechi omis/întârziat: %v • %s", err, sanitizeMega(out))
		}
	}()
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

	// Return the new URL immediately. Old-node cleanup is best-effort and must
	// never compete with another user preview.
	a.cleanupPreviousMegaPreviewAsyncV86(old, remoteRef)
	return result.StreamURL, nil
}
