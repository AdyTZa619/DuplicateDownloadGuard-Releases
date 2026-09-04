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

	if a.preview.Active && a.preview.SourceURL == item.URL && a.preview.RemotePath == megaWarmRootRefV86 && a.preview.StreamURL != "" {
		if child, err := megaWebDAVChildURL(a.preview.StreamURL, item.Path); err == nil && child != "" {
			a.resetPreviewTTLLocked()
			return child, nil
		}
	}
	if a.preview.Active && a.preview.SourceURL == item.URL && a.preview.RemotePath == remoteRef && a.preview.StreamURL != "" {
		a.resetPreviewTTLLocked()
		return a.preview.StreamURL, nil
	}

	if a.preview.Active && a.preview.SourceURL == item.URL && a.preview.Exe != "" {
		old := a.preview
		ctx := context.Background()
		lookupStarted := time.Now()
		if rootURL, err := listMegaWarmRootV8511(ctx, old.Exe, 1500*time.Millisecond); err == nil && rootURL != "" {
			child, childErr := a.activateMegaRootV8511(item, old.Exe, rootURL, old.PreviousSession)
			if childErr == nil {
				a.cleanupPreviousMegaPreviewAsyncV86(old, megaWarmRootRefV86)
				a.logf("MEGA ROOT REDISCOVER: root existent găsit în %d ms -> %s", time.Since(lookupStarted).Milliseconds(), rootURL)
				return child, nil
			}
		}

		run := func(timeout time.Duration, args ...string) (string, error) {
			return runMegaTimedPreviewV8514(ctx, timeout, old.Exe, args...)
		}
		fallbackStarted := time.Now()
		result, err := tryMegaCurrentSessionWebDAVV859(remoteRef, run)
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
		a.logf("MEGA HOT PER-FILE: root absent; endpointul cerut a fost pregătit în %d ms", time.Since(fallbackStarted).Milliseconds())
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
		return runMegaTimedPreviewV8514(ctx, timeout, exe, args...)
	}

	if a.matchesMegaPreviewRestartHintV859(item.URL) {
		if rootURL, err := listMegaWarmRootV8511(ctx, exe, 2500*time.Millisecond); err == nil && rootURL != "" {
			child, childErr := a.activateMegaRootV8511(item, exe, rootURL, "")
			if childErr == nil {
				a.logf("MEGA ROOT REUSE: root existent găsit în MEGAcmd -> %s", rootURL)
				return child, nil
			}
		} else if err != nil {
			a.logf("MEGA ROOT REUSE: listarea WebDAV nu a răspuns normal: %v", err)
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
		a.logf("MEGA CURRENT SESSION indisponibil; abia acum folosesc --resume: %v", err)
	}

	oldSession := ""
	if out, err := runMegaTimedPreviewV8514(ctx, 8*time.Second, exe, "session"); err == nil {
		oldSession = extractSession(out)
	}
	_, _ = runMegaTimedPreviewV8514(ctx, 8*time.Second, exe, "logout", "--keep-session")
	loginStarted := time.Now()
	loginOut, err := runMegaTimedPreviewV8514(ctx, 45*time.Second, exe, megaPublicLoginArgsV856(item.URL)...)
	if err != nil {
		a.restoreMegaSessionSilent(exe, oldSession)
		problem := classifyMegaProblem(loginOut, err)
		return "", newMegaProblemError(problem, loginOut)
	}
	a.logf("MEGA COLD RESUME: login --resume a durat %d ms", time.Since(loginStarted).Milliseconds())

	if rootURL, rootErr := startMegaWarmRootV86(ctx, exe); rootErr == nil && rootURL != "" {
		warmMegaWebDAVTransportV8512(rootURL)
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

	result, err := tryMegaCurrentSessionWebDAVV859(remoteRef, run)
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
