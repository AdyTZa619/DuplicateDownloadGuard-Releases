package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// switchWarmRootToPerFileV858 is the true browser-error fallback. The previous
// implementation called startMegaPreview(), which could immediately return the
// same warm-root child URL that had just failed in the browser. This helper
// starts the requested node by handle/path without deleting the root. MEGAcmd
// supports multiple WebDAV routes, while per-route deletion proved unsafe for
// its shared Windows command service.
func switchWarmRootToPerFileV858(old MegaPreviewState, remoteRef string, run megaWebDAVRunnerV85) (megaWebDAVSwitchResultV85, error) {
	if run == nil {
		return megaWebDAVSwitchResultV85{}, errors.New("MEGA WebDAV runner lipsă")
	}
	remoteRef = strings.TrimSpace(remoteRef)
	if remoteRef == "" {
		return megaWebDAVSwitchResultV85{}, errors.New("referință MEGA remote lipsă")
	}
	return switchSameSourceWebDAVV85(old, remoteRef, run)
}

func (a *App) startMegaPreviewPerFileFallbackV858(item RemoteItem) (string, error) {
	if a == nil || !strings.EqualFold(item.Source, "MEGA") {
		return "", errors.New("sursa nu este MEGA")
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

	ctx := context.Background()
	// Normal case: FAST ROOT is active for this folder. Do not relogin. Replace
	// only the WebDAV exposure with the specific file node and keep the public
	// folder session/cache warm.
	if a.preview.Active && a.preview.SourceURL == item.URL && a.preview.Exe != "" {
		old := a.preview
		run := func(timeout time.Duration, args ...string) (string, error) {
			return runMegaControlTimed(ctx, timeout, old.Exe, args...)
		}
		result, err := switchWarmRootToPerFileV858(old, remoteRef, run)
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
		a.logf("MEGA TRUE FALLBACK: warm root -> per-file %s [%s] -> %s", item.Path, remoteRef, result.StreamURL)
		return result.StreamURL, nil
	}

	// Restart/no warm session: preserve any current account, resume the public
	// folder cache, but expose the requested node directly instead of recreating
	// another warm-root URL that could reproduce the browser failure.
	if a.preview.Active {
		_ = a.stopMegaPreviewLocked("fallback per-fișier / schimbare sursă")
	}
	exe := a.detectMegaClient()
	if exe == "" {
		return "", errors.New("MEGAcmd nu a fost găsit")
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

	run := func(timeout time.Duration, args ...string) (string, error) {
		return runMegaControlTimed(ctx, timeout, exe, args...)
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
	a.resetPreviewTTLLocked()
	a.logf("MEGA TRUE FALLBACK restart: --resume + per-file %s [%s] -> %s", item.Path, remoteRef, result.StreamURL)
	return result.StreamURL, nil
}
