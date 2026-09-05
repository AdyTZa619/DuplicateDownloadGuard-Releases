package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// startMegaPreviewResumeDirectV858 is the restart path. v8.5.9 first tries the
// exact public-folder session that DDG deliberately preserved at the previous
// shutdown. Only if that short direct attempt fails does it pay for the older
// session/logout/login --resume sequence.
func (a *App) startMegaPreviewResumeDirectV858(item RemoteItem) (string, error) {
	if a == nil || !strings.EqualFold(item.Source, "MEGA") {
		return "", errors.New("sursa nu este MEGA")
	}
	if strings.TrimSpace(item.URL) == "" {
		return "", errors.New("link MEGA lipsă")
	}
	remoteRef := megaRemoteRef(item)
	if remoteRef == "" {
		return "", errors.New("fișierul MEGA nu are handle/cale remote utilizabilă")
	}

	gateCtx, gateCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer gateCancel()
	if err := acquireMegaSession(gateCtx); err != nil {
		return "", fmt.Errorf("MEGA este ocupat cu altă operație: %w", err)
	}
	defer releaseMegaSession()

	a.previewMu.Lock()
	defer a.previewMu.Unlock()

	// A fresh scan may already have a working warm root. Do not disturb it.
	if a.preview.Active && a.preview.SourceURL == item.URL && a.preview.RemotePath == megaWarmRootRefV86 && a.preview.StreamURL != "" {
		if child, err := megaWebDAVChildURL(a.preview.StreamURL, item.Path); err == nil && child != "" {
			a.resetPreviewTTLLocked()
			return child, nil
		}
	}
	// Exact per-file cache hit.
	if a.preview.Active && a.preview.SourceURL == item.URL && a.preview.RemotePath == remoteRef && a.preview.StreamURL != "" {
		a.resetPreviewTTLLocked()
		return a.preview.StreamURL, nil
	}
	// Same public folder already resumed: switch directly to requested node.
	if a.preview.Active && a.preview.SourceURL == item.URL && a.preview.Exe != "" {
		old := a.preview
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
		a.preview = MegaPreviewState{
			Active:          true,
			SourceURL:       item.URL,
			RemotePath:      remoteRef,
			StreamURL:       result.StreamURL,
			PreviousSession: old.PreviousSession,
			Exe:             old.Exe,
		}
		a.resetPreviewTTLLocked()
		return result.StreamURL, nil
	}

	if a.preview.Active {
		_ = a.stopMegaPreviewLocked("restart direct preview / schimbare sursă")
	}

	exe := a.detectMegaClient()
	if exe == "" {
		return "", errors.New("MEGAcmd nu a fost găsit")
	}
	ctx := context.Background()
	run := func(timeout time.Duration, args ...string) (string, error) {
		return runMegaControlTimed(ctx, timeout, exe, args...)
	}

	// v8.5.9 cold-start fast path. A graceful DDG shutdown can leave the public
	// folder session active when there was no previous account session to
	// restore. The non-secret hint tells us this exact source should still be
	// current, so try only WebDAV first. A stale hint costs at most a few seconds
	// and is immediately discarded.
	if a.matchesMegaPreviewRestartHintV859(item.URL) {
		result, err := tryMegaCurrentSessionWebDAVV859(remoteRef, run)
		if err == nil && result.StreamURL != "" {
			a.preview = MegaPreviewState{
				Active:     true,
				SourceURL:  item.URL,
				RemotePath: remoteRef,
				StreamURL:  result.StreamURL,
				Exe:        exe,
			}
			a.resetPreviewTTLLocked()
			a.logf("MEGA CURRENT SESSION: direct per-file %s [%s] -> %s", item.Path, remoteRef, result.StreamURL)
			return result.StreamURL, nil
		}
		a.clearMegaPreviewRestartHintV859()
		a.logf("MEGA CURRENT SESSION indisponibil; folosesc --resume: %v", err)
	}

	oldSession := ""
	if out, err := runMegaControlTimed(ctx, 8*time.Second, exe, "session"); err == nil {
		oldSession = extractSession(out)
	}
	_, _ = runMegaControlTimed(ctx, 8*time.Second, exe, "logout", "--keep-session")
	loginOut, err := runMegaControlTimed(ctx, 45*time.Second, exe, megaPublicLoginArgsV856(item.URL)...)
	if err != nil {
		a.restoreMegaSessionSilent(exe, oldSession)
		problem := classifyMegaProblem(loginOut, err)
		return "", newMegaProblemError(problem, loginOut)
	}

	result, err := switchSameSourceWebDAVV85(MegaPreviewState{Exe: exe}, remoteRef, run)
	if err != nil {
		a.restoreMegaSessionSilent(exe, oldSession)
		if err.Error() == megaWebDAVURLMissingV85 {
			return "", err
		}
		problem := classifyMegaProblem(result.StartOutput, err)
		return "", newMegaProblemError(problem, result.StartOutput)
	}

	a.preview = MegaPreviewState{
		Active:          true,
		SourceURL:       item.URL,
		RemotePath:      remoteRef,
		StreamURL:       result.StreamURL,
		PreviousSession: oldSession,
		Exe:             exe,
	}
	if oldSession == "" {
		a.saveMegaPreviewRestartHintV859(item.URL)
	} else {
		a.clearMegaPreviewRestartHintV859()
	}
	a.resetPreviewTTLLocked()
	a.logf("MEGA DIRECT RESUME: --resume + per-file %s [%s] -> %s", item.Path, remoteRef, result.StreamURL)
	return result.StreamURL, nil
}
