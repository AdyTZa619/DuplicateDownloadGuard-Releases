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

// switchSameSourceWebDAVV85 starts and validates the requested WebDAV node. It
// never deletes the previous or newly-created route: MEGAcmd owns all routes
// until the public-folder session is changed.
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
		return result, errors.New(megaWebDAVURLMissingV85)
	}
	result.StreamURL = streamURL
	return result, nil
}

func (a *App) switchMegaPreviewSameSourceV85(old MegaPreviewState, remoteRef string) (string, error) {
	ctx := context.Background()
	run := func(timeout time.Duration, args ...string) (string, error) {
		return runMegaControlTimed(ctx, timeout, old.Exe, args...)
	}
	result, err := switchSameSourceWebDAVV85(old, remoteRef, run)
	if err != nil {
		if err.Error() == megaWebDAVURLMissingV85 {
			return "", err
		}
		problem := classifyMegaProblem(result.StartOutput, err)
		return "", newMegaProblemError(problem, result.StartOutput)
	}

	// Keep every route for this source session. MEGAcmd removes the mappings when
	// the session is changed; per-route cleanup during preview is unsafe.
	return result.StreamURL, nil
}
