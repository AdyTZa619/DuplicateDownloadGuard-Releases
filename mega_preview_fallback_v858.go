package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

type megaTempFallbackV8511 struct {
	RemotePath string
	StreamURL  string
	Exe        string
}

var megaTempFallbackByAppV8511 sync.Map

func currentMegaTempFallbackV8511(a *App) megaTempFallbackV8511 {
	if a == nil {
		return megaTempFallbackV8511{}
	}
	if raw, ok := megaTempFallbackByAppV8511.Load(a); ok {
		if st, ok := raw.(megaTempFallbackV8511); ok {
			return st
		}
	}
	return megaTempFallbackV8511{}
}

func rememberMegaTempFallbackV8511(a *App, next megaTempFallbackV8511) {
	if a == nil || next.RemotePath == "" || next.Exe == "" {
		return
	}
	previous := currentMegaTempFallbackV8511(a)
	megaTempFallbackByAppV8511.Store(a, next)
	if previous.RemotePath == "" || previous.Exe == "" || previous.RemotePath == next.RemotePath || previous.Exe != next.Exe {
		return
	}

	go func(old megaTempFallbackV8511) {
		time.Sleep(1500 * time.Millisecond)
		gateCtx, gateCancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
		defer gateCancel()
		if err := acquireMegaSession(gateCtx); err != nil {
			return
		}
		defer releaseMegaSession()
		current := currentMegaTempFallbackV8511(a)
		if current.RemotePath == old.RemotePath && current.Exe == old.Exe {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
		defer cancel()
		_, _ = runMegaTimedPreviewV8514(ctx, 1500*time.Millisecond, old.Exe, "webdav", "-d", old.RemotePath)
	}(previous)
}

func cleanupMegaTempFallbackWhileSessionOwnedV8511(a *App, ctx context.Context) {
	if a == nil {
		return
	}
	raw, ok := megaTempFallbackByAppV8511.LoadAndDelete(a)
	if !ok {
		return
	}
	st, ok := raw.(megaTempFallbackV8511)
	if !ok || st.RemotePath == "" || st.Exe == "" {
		return
	}
	_, _ = runMegaTimedPreviewV8514(ctx, 3*time.Second, st.Exe, "webdav", "-d", st.RemotePath)
}

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

		if old.RemotePath == megaWarmRootRefV86 {
			if tmp := currentMegaTempFallbackV8511(a); tmp.Exe == old.Exe && tmp.RemotePath == remoteRef && tmp.StreamURL != "" {
				a.resetPreviewTTLLocked()
				return tmp.StreamURL, nil
			}
		}

		run := func(timeout time.Duration, args ...string) (string, error) {
			return runMegaTimedPreviewV8514(ctx, timeout, old.Exe, args...)
		}
		result, err := switchWarmRootToPerFileV858(old, remoteRef, run)
		if err != nil {
			if err.Error() == megaWebDAVURLMissingV85 {
				return "", err
			}
			problem := classifyMegaProblem(result.StartOutput, err)
			return "", newMegaProblemError(problem, result.StartOutput)
		}

		if old.RemotePath == megaWarmRootRefV86 {
			rememberMegaTempFallbackV8511(a, megaTempFallbackV8511{RemotePath: remoteRef, StreamURL: result.StreamURL, Exe: old.Exe})
			a.preview = old
			a.resetPreviewTTLLocked()
			a.logf("MEGA TRUE FALLBACK: per-file temporar pentru %s [%s]; root-ul rămâne activ", item.Path, remoteRef)
			return result.StreamURL, nil
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
		a.logf("MEGA TRUE FALLBACK: per-file %s [%s] -> %s", item.Path, remoteRef, result.StreamURL)
		return result.StreamURL, nil
	}

	if a.preview.Active {
		_ = a.stopMegaPreviewLocked("fallback per-fișier / schimbare sursă")
	}
	exe := a.detectMegaClient()
	if exe == "" {
		return "", errors.New("MEGAcmd nu a fost găsit")
	}
	oldSession := ""
	if out, err := runMegaTimedPreviewV8514(ctx, 8*time.Second, exe, "session"); err == nil {
		oldSession = extractSession(out)
	}
	_, _ = runMegaTimedPreviewV8514(ctx, 8*time.Second, exe, "logout", "--keep-session")
	loginOut, err := runMegaTimedPreviewV8514(ctx, 45*time.Second, exe, megaPublicLoginArgsV856(item.URL)...)
	if err != nil {
		a.restoreMegaSessionSilent(exe, oldSession)
		problem := classifyMegaProblem(loginOut, err)
		return "", newMegaProblemError(problem, loginOut)
	}

	run := func(timeout time.Duration, args ...string) (string, error) {
		return runMegaTimedPreviewV8514(ctx, timeout, exe, args...)
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
