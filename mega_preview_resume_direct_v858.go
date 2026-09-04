package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

func (a *App) activateMegaRootV8511(item RemoteItem, exe, rootURL, previousSession string) (string, error) {
	child, err := megaWebDAVChildURL(rootURL, item.Path)
	if err != nil || child == "" {
		if err == nil {
			err = errors.New("URL copil gol")
		}
		return "", fmt.Errorf("WebDAV root este activ, dar calea fișierului este invalidă: %w", err)
	}
	a.preview = MegaPreviewState{
		Active:          true,
		SourceURL:       item.URL,
		RemotePath:      megaWarmRootRefV86,
		StreamURL:       rootURL,
		PreviousSession: previousSession,
		Exe:             exe,
	}
	a.resetPreviewTTLLocked()
	return child, nil
}

// startMegaPreviewResumeDirectV858 is the restart path. v8.5.11 keeps the
// validated 8.5.9 session model but changes the priority: discover/reuse one
// existing whole-folder WebDAV root first, then create that root in the current
// session, and only then fall back to per-file or login --resume work.
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

	// Same public folder is already active in MEGAcmd but DDG currently tracks a
	// legacy per-file endpoint. Promote it back to one root before doing another
	// per-file switch.
	if a.preview.Active && a.preview.SourceURL == item.URL && a.preview.Exe != "" {
		old := a.preview
		ctx := context.Background()
		if rootURL, err := startMegaWarmRootV86(ctx, old.Exe); err == nil && rootURL != "" {
			child, childErr := a.activateMegaRootV8511(item, old.Exe, rootURL, old.PreviousSession)
			if childErr == nil {
				a.cleanupPreviousMegaPreviewAsyncV86(old, megaWarmRootRefV86)
				a.logf("MEGA ROOT PROMOTE: sesiunea existentă a revenit la un singur root -> %s", rootURL)
				return child, nil
			}
		}

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

	// A graceful previous DDG run leaves the public session and canonical root
	// configured. Ask MEGAcmd for its live served-location list; never trust a
	// saved localhost URL blindly and never create a per-file endpoint first.
	if a.matchesMegaPreviewRestartHintV859(item.URL) {
		if rootURL, err := listMegaWarmRootV8511(ctx, exe, 4*time.Second); err == nil && rootURL != "" {
			child, childErr := a.activateMegaRootV8511(item, exe, rootURL, "")
			if childErr == nil {
				a.logf("MEGA ROOT REUSE: root existent găsit în MEGAcmd -> %s", rootURL)
				return child, nil
			}
		} else if err != nil {
			a.logf("MEGA ROOT REUSE: listarea WebDAV nu a răspuns normal: %v", err)
		}

		// If the session survived but its root was not restored, create exactly
		// one root in that same hot session before considering per-file fallback.
		if rootURL, err := startMegaWarmRootV86(ctx, exe); err == nil && rootURL != "" {
			child, childErr := a.activateMegaRootV8511(item, exe, rootURL, "")
			if childErr == nil {
				a.logf("MEGA CURRENT ROOT: sesiunea păstrată a primit root -> %s", rootURL)
				return child, nil
			}
		} else {
			a.logf("MEGA CURRENT ROOT indisponibil; încerc compatibilitatea per-fișier: %v", err)
		}

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
			a.logf("MEGA CURRENT SESSION: compatibilitate per-file %s [%s] -> %s", item.Path, remoteRef, result.StreamURL)
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

	// After the unavoidable cold resume, establish one root for this run. The
	// per-file constructor is now only a compatibility path if root exposure
	// itself fails.
	if rootURL, rootErr := startMegaWarmRootV86(ctx, exe); rootErr == nil && rootURL != "" {
		child, childErr := a.activateMegaRootV8511(item, exe, rootURL, oldSession)
		if childErr == nil {
			if oldSession == "" {
				a.saveMegaPreviewRestartHintV859(item.URL)
			} else {
				a.clearMegaPreviewRestartHintV859()
			}
			a.logf("MEGA ROOT RESUME: --resume + un singur WebDAV root -> %s", rootURL)
			return child, nil
		}
	} else {
		a.logf("MEGA ROOT RESUME indisponibil; folosesc per-file: %v", rootErr)
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
