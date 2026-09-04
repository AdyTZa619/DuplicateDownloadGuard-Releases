package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

func (a *App) snapshotMegaPreviewV8516() MegaPreviewState {
	if a == nil {
		return MegaPreviewState{}
	}
	a.previewMu.Lock()
	st := a.preview
	a.previewMu.Unlock()
	return st
}

func (a *App) commitMegaPreviewV8516(st MegaPreviewState) {
	if a == nil {
		return
	}
	a.previewMu.Lock()
	a.preview = st
	a.resetPreviewTTLLocked()
	a.previewMu.Unlock()
}

func (a *App) touchMegaPreviewV8516(sourceURL, remotePath, streamURL string) bool {
	if a == nil {
		return false
	}
	a.previewMu.Lock()
	defer a.previewMu.Unlock()
	if !a.preview.Active || a.preview.SourceURL != sourceURL || a.preview.RemotePath != remotePath || a.preview.StreamURL != streamURL {
		return false
	}
	a.resetPreviewTTLLocked()
	return true
}

func (a *App) activateMegaRootV8511(item RemoteItem, exe, rootURL, previousSession string) (string, error) {
	child, err := megaWebDAVChildURL(rootURL, item.Path)
	if err != nil || child == "" {
		if err == nil {
			err = errors.New("URL copil gol")
		}
		return "", fmt.Errorf("WebDAV root este activ, dar calea fișierului este invalidă: %w", err)
	}
	a.commitMegaPreviewV8516(MegaPreviewState{
		Active:          true,
		SourceURL:       item.URL,
		RemotePath:      megaWarmRootRefV86,
		StreamURL:       rootURL,
		PreviousSession: previousSession,
		Exe:             exe,
	})
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

	// v8.5.16: previewMu protects only the in-memory state. Never keep it while
	// MegaClient, filesystem diagnostics, network probes or cleanup are running.
	// The MEGA session gate already serializes the actual MEGAcmd operations.
	old := a.snapshotMegaPreviewV8516()

	if old.Active && old.SourceURL == item.URL && old.RemotePath == megaWarmRootRefV86 && old.StreamURL != "" {
		if child, err := megaWebDAVChildURL(old.StreamURL, item.Path); err == nil && child != "" {
			a.touchMegaPreviewV8516(old.SourceURL, old.RemotePath, old.StreamURL)
			return child, nil
		}
	}
	if old.Active && old.SourceURL == item.URL && old.RemotePath == remoteRef && old.StreamURL != "" {
		a.touchMegaPreviewV8516(old.SourceURL, old.RemotePath, old.StreamURL)
		return old.StreamURL, nil
	}

	// The scan (or a previous preview) proves this exact public-folder session
	// is already active. Never rebuild the long whole-folder root on this click.
	if old.Active && old.SourceURL == item.URL && old.Exe != "" {
		ctx := context.Background()
		lookupStarted := time.Now()
		if rootURL, err := listMegaWarmRootV8511(ctx, old.Exe, 1500*time.Millisecond); err == nil && rootURL != "" {
			child, childErr := a.activateMegaRootV8511(item, old.Exe, rootURL, old.PreviousSession)
			if childErr == nil {
				a.cleanupPreviousMegaPreviewAsyncV86(old, megaWarmRootRefV86)
				megaPreviewDiagfV8514("RETURN PREP  item=%q path=root-rediscover elapsed=%s", item.Path, time.Since(lookupStarted).Round(time.Millisecond))
				a.previewLogfAsyncV8515("MEGA ROOT REDISCOVER: root existent găsit în %d ms -> %s", time.Since(lookupStarted).Milliseconds(), rootURL)
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
		megaPreviewDiagfV8514("POST CMD     item=%q path=hot-per-file elapsed=%s", item.Path, time.Since(fallbackStarted).Round(time.Millisecond))
		a.commitMegaPreviewV8516(MegaPreviewState{
			Active:          true,
			SourceURL:       item.URL,
			RemotePath:      remoteRef,
			StreamURL:       result.StreamURL,
			PreviousSession: old.PreviousSession,
			Exe:             old.Exe,
		})
		a.cleanupPreviousMegaPreviewAsyncV86(old, remoteRef)
		megaPreviewDiagfV8514("RETURN PREP  item=%q path=hot-per-file elapsed=%s", item.Path, time.Since(fallbackStarted).Round(time.Millisecond))
		a.previewLogfAsyncV8515("MEGA HOT PER-FILE: root absent; endpointul cerut a fost pregătit în %d ms", time.Since(fallbackStarted).Milliseconds())
		return result.StreamURL, nil
	}

	if old.Active {
		if err := a.stopMegaPreviewWhileSessionOwned("restart direct preview / schimbare sursă"); err != nil {
			a.previewLogfAsyncV8515("MEGA preview: oprirea sesiunii vechi înainte de sursă nouă: %v", err)
		}
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
				megaPreviewDiagfV8514("RETURN PREP  item=%q path=restart-root-reuse", item.Path)
				a.previewLogfAsyncV8515("MEGA ROOT REUSE: root existent găsit în MEGAcmd -> %s", rootURL)
				return child, nil
			}
		} else if err != nil {
			a.previewLogfAsyncV8515("MEGA ROOT REUSE: listarea WebDAV nu a răspuns normal: %v", err)
		}

		restartStarted := time.Now()
		result, err := tryMegaCurrentSessionWebDAVV859(remoteRef, run)
		if err == nil && result.StreamURL != "" {
			megaPreviewDiagfV8514("POST CMD     item=%q path=current-session elapsed=%s", item.Path, time.Since(restartStarted).Round(time.Millisecond))
			a.commitMegaPreviewV8516(MegaPreviewState{
				Active:     true,
				SourceURL:  item.URL,
				RemotePath: remoteRef,
				StreamURL:  result.StreamURL,
				Exe:        exe,
			})
			megaPreviewDiagfV8514("RETURN PREP  item=%q path=current-session elapsed=%s", item.Path, time.Since(restartStarted).Round(time.Millisecond))
			a.previewLogfAsyncV8515("MEGA CURRENT SESSION: compatibilitate per-file %s [%s] -> %s", item.Path, remoteRef, result.StreamURL)
			return result.StreamURL, nil
		}
		a.clearMegaPreviewRestartHintV859()
		a.previewLogfAsyncV8515("MEGA CURRENT SESSION indisponibil; abia acum folosesc --resume: %v", err)
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
	a.previewLogfAsyncV8515("MEGA COLD RESUME: login --resume a durat %d ms", time.Since(loginStarted).Milliseconds())

	if rootURL, rootErr := startMegaWarmRootV86(ctx, exe); rootErr == nil && rootURL != "" {
		warmMegaWebDAVTransportV8512(rootURL)
		child, childErr := a.activateMegaRootV8511(item, exe, rootURL, oldSession)
		if childErr == nil {
			if oldSession == "" {
				a.saveMegaPreviewRestartHintV859(item.URL)
			} else {
				a.clearMegaPreviewRestartHintV859()
			}
			a.previewLogfAsyncV8515("MEGA ROOT RESUME: --resume + un singur WebDAV root -> %s", rootURL)
			return child, nil
		}
	} else {
		a.previewLogfAsyncV8515("MEGA ROOT RESUME indisponibil; folosesc per-file: %v", rootErr)
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

	a.commitMegaPreviewV8516(MegaPreviewState{
		Active:          true,
		SourceURL:       item.URL,
		RemotePath:      remoteRef,
		StreamURL:       result.StreamURL,
		PreviousSession: oldSession,
		Exe:             exe,
	})
	if oldSession == "" {
		a.saveMegaPreviewRestartHintV859(item.URL)
	} else {
		a.clearMegaPreviewRestartHintV859()
	}
	a.previewLogfAsyncV8515("MEGA DIRECT RESUME fallback: --resume + per-file %s [%s] -> %s", item.Path, remoteRef, result.StreamURL)
	return result.StreamURL, nil
}
