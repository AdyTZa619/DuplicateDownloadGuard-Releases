package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// switchWarmRootToPerFileV858 is the browser-error fallback. v8.5.10 no longer
// tears down the whole WebDAV root when one child URL fails in the browser.
// MEGAcmd can expose the requested file node alongside the root, so the current
// item gets a genuinely different endpoint while all following items keep the
// zero-command FAST ROOT path.
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
	if a.preview.Active && a.preview.SourceURL == item.URL && a.preview.Exe != "" {
		old := a.preview
		run := func(timeout time.Duration, args ...string) (string, error) {
			return runMegaTimed(ctx, timeout, old.Exe, args...)
		}
		result, err := switchWarmRootToPerFileV858(old, remoteRef, run)
		if err != nil {
			if err.Error() == megaWebDAVURLMissingV85 {
				return "", err
			}
			problem := classifyMegaProblem(result.StartOutput, err)
			return "", newMegaProblemError(problem, result.StartOutput)
		}

		if old.RemotePath == megaWarmRootRefV86 && old.StreamURL != "" {
			// Critical v8.5.10 behavior: the per-file endpoint is temporary for this
			// browser failure only. Keep the root as DDG's canonical preview state so
			// the next row does not inherit a 15-second per-file startup path.
			a.preview = old
			a.resetPreviewTTLLocked()
			a.logf("MEGA TRUE FALLBACK: per-file temporar, root păstrat pentru următorul fișier %s [%s] -> %s", item.Path, remoteRef, result.StreamURL)
			return result.StreamURL, nil
		}

		// Compatibility path for sessions where a whole-folder root could not be
		// created. Keep the latest per-file node as before.
		a.preview = MegaPreviewState{
			Active:          true,
			SourceURL:       item.URL,
			RemotePath:      remoteRef,
			StreamURL:       result.StreamURL,
			PreviousSession: old.PreviousSession,
			Exe:             old.Exe,
		}
		a.resetPreviewTTLLocked()
		a.logf("MEGA TRUE FALLBACK: per-file %s [%s] -> %s", item.Path, remoteRef, result.StreamURL)
		return result.StreamURL, nil
	}

	// Restart/no warm session: preserve any current account, resume the public
	// folder cache and prefer one whole-folder root. This avoids entering a
	// per-file-only state after a cold browser fallback.
	if a.preview.Active {
		_ = a.stopMegaPreviewLocked("fallback / schimbare sursă")
	}
	exe := a.detectMegaClient()
	if exe == "" {
		return "", errors.New("MEGAcmd nu a fost găsit")
	}
	oldSession := ""
	if out, err := runMegaTimed(ctx, 8*time.Second, exe, "session"); err == nil {
		oldSession = extractSession(out)
	}
	_, _ = runMegaTimed(ctx, 8*time.Second, exe, "logout", "--keep-session")
	loginOut, err := runMegaTimed(ctx, 45*time.Second, exe, megaPublicLoginArgsV856(item.URL)...)
	if err != nil {
		a.restoreMegaSessionSilent(exe, oldSession)
		problem := classifyMegaProblem(loginOut, err)
		return "", newMegaProblemError(problem, loginOut)
	}

	if rootURL, rootErr := startMegaWarmRootV86(ctx, exe); rootErr == nil && rootURL != "" {
		childURL, childErr := megaWebDAVChildURL(rootURL, item.Path)
		if childErr == nil && childURL != "" {
			a.preview = MegaPreviewState{
				Active:          true,
				SourceURL:       item.URL,
				RemotePath:      megaWarmRootRefV86,
				StreamURL:       rootURL,
				PreviousSession: oldSession,
				Exe:             exe,
			}
			if oldSession == "" {
				a.saveMegaPreviewRestartRootV8510(item.URL, rootURL)
			} else {
				a.clearMegaPreviewRestartHintV859()
			}
			a.resetPreviewTTLLocked()
			a.logf("MEGA TRUE FALLBACK restart: root refăcut -> %s", rootURL)
			return childURL, nil
		}
	}

	run := func(timeout time.Duration, args ...string) (string, error) {
		return runMegaTimed(ctx, timeout, exe, args...)
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
	a.logf("MEGA TRUE FALLBACK restart: per-file compatibilitate %s [%s] -> %s", item.Path, remoteRef, result.StreamURL)
	return result.StreamURL, nil
}
