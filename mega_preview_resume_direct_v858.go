package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// startMegaPreviewResumeDirectV858 is the restart path. v8.5.10 first tries a
// WebDAV root deliberately kept alive by the previous clean DDG shutdown. That
// path executes no MEGAcmd command. If no root survived, DDG next tries to
// rebuild one on the preserved public-folder session and only then falls back
// to the older session/logout/login --resume sequence.
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

	// v8.5.10 zero-command restart path. A clean shutdown can leave MEGAcmd's
	// loopback root listener alive after DDG exits. Test only the local listener,
	// derive the child URL locally and avoid session/logout/login/webdav entirely.
	if rootURL, childURL, ok := a.tryPersistedMegaRootV8510(item); ok {
		exe := a.detectMegaClient()
		a.previewMu.Lock()
		a.preview = MegaPreviewState{
			Active:     true,
			SourceURL:  item.URL,
			RemotePath: megaWarmRootRefV86,
			StreamURL:  rootURL,
			Exe:        exe,
		}
		a.resetPreviewTTLLocked()
		a.previewMu.Unlock()
		a.logf("MEGA PERSISTENT ROOT: restart fără comandă MEGAcmd -> %s", childURL)
		return childURL, nil
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
	// Same public folder already resumed but no root is available: keep the old
	// per-file compatibility path. Browser-error fallback no longer destroys a
	// healthy root in v8.5.10, so reaching this branch normally means root setup
	// itself was unavailable for this MEGAcmd session.
	if a.preview.Active && a.preview.SourceURL == item.URL && a.preview.Exe != "" {
		old := a.preview
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
		a.cleanupPreviousMegaPreviewAsyncV86(old, remoteRef)
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
		return runMegaTimed(ctx, timeout, exe, args...)
	}

	// A v8.5.9-style hint may have preserved the public-folder session without
	// preserving its root. Rebuild ONE root on that current session first. If it
	// succeeds, all following row changes become local URL derivations.
	if a.matchesMegaPreviewRestartHintV859(item.URL) {
		if rootURL, rootErr := startMegaWarmRootV86(ctx, exe); rootErr == nil && rootURL != "" {
			childURL, childErr := megaWebDAVChildURL(rootURL, item.Path)
			if childErr == nil && childURL != "" {
				a.preview = MegaPreviewState{
					Active:     true,
					SourceURL:  item.URL,
					RemotePath: megaWarmRootRefV86,
					StreamURL:  rootURL,
					Exe:        exe,
				}
				a.saveMegaPreviewRestartRootV8510(item.URL, rootURL)
				a.resetPreviewTTLLocked()
				a.logf("MEGA CURRENT ROOT: sesiunea păstrată a refăcut root-ul -> %s", rootURL)
				return childURL, nil
			}
		}

		// Compatibility fallback for installations where whole-root WebDAV is not
		// available. It is deliberately short and does not perform a login.
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
			a.logf("MEGA CURRENT SESSION: fallback per-file %s [%s] -> %s", item.Path, remoteRef, result.StreamURL)
			return result.StreamURL, nil
		}
		a.clearMegaPreviewRestartHintV859()
		a.logf("MEGA CURRENT SESSION indisponibil; folosesc --resume: %v", err)
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

	// After any cold resume, pay WebDAV setup once for the folder root instead
	// of once for every selected file. Per-file remains only a compatibility
	// fallback if this MEGAcmd installation cannot expose the whole root.
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
			a.logf("MEGA ROOT RESUME: --resume + un singur WebDAV root -> %s", rootURL)
			return childURL, nil
		}
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
	a.logf("MEGA DIRECT RESUME fallback: --resume + per-file %s [%s] -> %s", item.Path, remoteRef, result.StreamURL)
	return result.StreamURL, nil
}
