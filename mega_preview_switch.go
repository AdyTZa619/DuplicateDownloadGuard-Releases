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

	// A warm MEGAcmd session normally answers in a few seconds. Keep a bounded
	// safety timeout, but do not let an individual preview monopolize the UI for
	// the old 30 seconds.
	out, err := run(15*time.Second, "webdav", remoteRef)
	result := megaWebDAVSwitchResultV85{StartOutput: out}
	if err != nil {
		return result, err
	}
	streamURL := extractWebDAVURL(out, remoteRef)
	if streamURL == "" {
		// Some MEGAcmd builds enable the node but only expose the URL in the
		// subsequent listing. This lookup should be quick; a long listing timeout
		// would make preview feel frozen.
		listing, _ := run(3*time.Second, "webdav")
		streamURL = extractWebDAVURL(listing, remoteRef)
	}
	if streamURL == "" {
		// The new node may have been enabled even though MEGAcmd did not return a
		// usable URL. Clean only the new node; never touch the known-good old one.
		if old.RemotePath != remoteRef {
			_, _ = run(4*time.Second, "webdav", "-d", remoteRef)
		}
		return result, errors.New(megaWebDAVURLMissingV85)
	}
	result.StreamURL = streamURL
	return result, nil
}

func (a *App) cleanupPreviousMegaPreviewAsyncV86(old MegaPreviewState, newRemoteRef string) {
	if old.Exe == "" || old.RemotePath == "" || old.RemotePath == newRemoteRef || old.RemotePath == megaWarmRootRefV86 {
		return
	}
	go func() {
		// Give the external player/browser a short hand-off interval. The old URL
		// can remain alive briefly while the new one starts buffering.
		time.Sleep(1200 * time.Millisecond)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		out, err := runMegaTimed(ctx, 5*time.Second, old.Exe, "webdav", "-d", old.RemotePath)
		if err != nil {
			a.logf("MEGA preview: cleanup WebDAV vechi în fundal: %v • %s", err, sanitizeMega(out))
		}
	}()
}

func (a *App) switchMegaPreviewSameSourceV85(old MegaPreviewState, remoteRef string) (string, error) {
	// Warm-root fast path: if the scan already exposed the whole public folder,
	// no MEGAcmd command is necessary for a child preview.
	if childURL, ok := warmRootPreviewURLV86(old, RemoteItem{URL: old.SourceURL, Path: remoteRef}); ok {
		return childURL, nil
	}

	ctx := context.Background()
	run := func(timeout time.Duration, args ...string) (string, error) {
		return runMegaTimed(ctx, timeout, old.Exe, args...)
	}
	result, err := switchSameSourceWebDAVV85(old, remoteRef, run)
	if err != nil {
		// Missing stream URL is a local WebDAV response problem, not a quota or
		// login problem. Preserve it even when MEGAcmd printed non-empty output.
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
