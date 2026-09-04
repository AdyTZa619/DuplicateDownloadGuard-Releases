package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

var megaPreviewResumeFlightV856 struct {
	sync.Mutex
	done chan struct{}
}

// tryMegaPreviewUICacheV854 is intentionally network-free. The MEGA scan has
// already logged into the public folder and, when possible, exposed its root
// through WebDAV. The UI should not run another MEGAcmd command or synchronous
// HTTP probe for every row selection; the browser itself is about to request
// the media bytes anyway.
func (a *App) tryMegaPreviewUICacheV854(item RemoteItem) (string, string, bool) {
	if a == nil || !strings.EqualFold(item.Source, "MEGA") {
		return "", "", false
	}

	a.previewMu.Lock()
	defer a.previewMu.Unlock()
	st := a.preview
	if !st.Active || st.SourceURL != item.URL || strings.TrimSpace(st.StreamURL) == "" {
		return "", "", false
	}

	if st.RemotePath == megaWarmRootRefV86 {
		child, err := megaWebDAVChildURL(st.StreamURL, item.Path)
		if err == nil && child != "" {
			a.resetPreviewTTLLocked()
			a.logf("MEGA UI Fast Preview root hit: %s -> %s", item.Path, child)
			return child, "MEGA FAST ROOT", true
		}
	}

	remoteRef := megaRemoteRef(item)
	if remoteRef != "" && st.RemotePath == remoteRef {
		a.resetPreviewTTLLocked()
		a.logf("MEGA UI Fast Preview cache hit: %s", item.Path)
		return st.StreamURL, "MEGA FAST CACHE", true
	}
	return "", "", false
}

func megaPublicLoginArgsV856(link string) []string {
	return []string{"login", strings.TrimSpace(link), "--resume"}
}

func (a *App) startMegaPreviewResumeV856(item RemoteItem) (string, error) {
	if a == nil || !strings.EqualFold(item.Source, "MEGA") {
		return "", errors.New("sursa nu este MEGA")
	}
	if strings.TrimSpace(item.URL) == "" {
		return "", errors.New("link MEGA lipsă")
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
		child, err := megaWebDAVChildURL(a.preview.StreamURL, item.Path)
		if err == nil && child != "" {
			a.resetPreviewTTLLocked()
			return child, nil
		}
	}

	if a.preview.Active && a.preview.SourceURL == item.URL && a.preview.Exe != "" && a.preview.StreamURL == "" {
		rootURL, rootErr := startMegaWarmRootV86(context.Background(), a.preview.Exe)
		if rootErr == nil {
			a.preview.RemotePath = megaWarmRootRefV86
			a.preview.StreamURL = rootURL
			a.resetPreviewTTLLocked()
			child, childErr := megaWebDAVChildURL(rootURL, item.Path)
			if childErr == nil && child != "" {
				a.logf("MEGA Fast Preview: root reparat fără relogin -> %s", rootURL)
				return child, nil
			}
		}
		a.logf("MEGA Fast Preview: sesiunea era caldă, dar root WebDAV nu a putut fi refăcut: %v", rootErr)
	}

	if a.preview.Active {
		_ = a.stopMegaPreviewLocked("restart fast preview / schimbare sursă")
	}

	exe := a.detectMegaClient()
	if exe == "" {
		return "", errors.New("MEGAcmd nu a fost găsit")
	}
	ctx := context.Background()
	oldSession := ""
	if out, err := runMegaTimedPreviewV8514(ctx, 8*time.Second, exe, "session"); err == nil {
		oldSession = extractSession(out)
	}
	_, _ = runMegaTimedPreviewV8514(ctx, 8*time.Second, exe, "logout", "--keep-session")

	loginArgs := megaPublicLoginArgsV856(item.URL)
	loginOut, err := runMegaTimedPreviewV8514(ctx, 45*time.Second, exe, loginArgs...)
	if err != nil {
		a.restoreMegaSessionSilent(exe, oldSession)
		problem := classifyMegaProblem(loginOut, err)
		return "", newMegaProblemError(problem, loginOut)
	}

	rootURL, err := startMegaWarmRootV86(ctx, exe)
	if err != nil {
		a.restoreMegaSessionSilent(exe, oldSession)
		return "", fmt.Errorf("folderul MEGA s-a reluat, dar WebDAV root nu a pornit: %w", err)
	}

	a.preview = MegaPreviewState{
		Active:          true,
		SourceURL:       item.URL,
		RemotePath:      megaWarmRootRefV86,
		StreamURL:       rootURL,
		PreviousSession: oldSession,
		Exe:             exe,
	}
	a.resetPreviewTTLLocked()
	child, childErr := megaWebDAVChildURL(rootURL, item.Path)
	if childErr != nil {
		return "", fmt.Errorf("WebDAV root este activ, dar calea fișierului este invalidă: %w", childErr)
	}
	if child == "" {
		return "", errors.New("WebDAV root este activ, dar URL-ul copil este gol")
	}
	a.logf("MEGA Fast Preview restart: cache folder reluat cu --resume -> %s", rootURL)
	return child, nil
}

func (a *App) startMegaPreviewResumeCoalescedV856(item RemoteItem) (string, error) {
	for attempt := 0; attempt < 2; attempt++ {
		if streamURL, _, ok := a.tryMegaPreviewUICacheV854(item); ok {
			return streamURL, nil
		}

		megaPreviewResumeFlightV856.Lock()
		if inFlight := megaPreviewResumeFlightV856.done; inFlight != nil {
			megaPreviewResumeFlightV856.Unlock()
			megaPreviewDiagfV8514("CLICK WAIT   item=%q reason=resume-flight", item.Path)
			select {
			case <-inFlight:
				continue
			case <-time.After(60 * time.Second):
				return "", errors.New("inițializarea MEGA preview nu s-a terminat în 60 secunde")
			}
		}
		done := make(chan struct{})
		megaPreviewResumeFlightV856.done = done
		megaPreviewResumeFlightV856.Unlock()

		streamURL, err := a.startMegaPreviewResumeDirectV858(item)
		megaPreviewResumeFlightV856.Lock()
		if megaPreviewResumeFlightV856.done == done {
			megaPreviewResumeFlightV856.done = nil
			close(done)
		}
		megaPreviewResumeFlightV856.Unlock()
		return streamURL, err
	}
	if streamURL, _, ok := a.tryMegaPreviewUICacheV854(item); ok {
		return streamURL, nil
	}
	return "", errors.New("MEGA preview nu a putut reutiliza inițializarea comună")
}

func (a *App) proxyMegaUIV8513(streamURL, mode string, started time.Time) (string, string, time.Duration, error) {
	proxyURL, err := wrapMegaPreviewProxyURLV8513(streamURL)
	if err != nil {
		megaPreviewDiagfV8514("CLICK END    mode=%s elapsed=%s proxy-error=%v", mode, time.Since(started).Round(time.Millisecond), err)
		return "", mode, time.Since(started), fmt.Errorf("proxy local MEGA indisponibil: %w", err)
	}
	a.logf("MEGA UI PROXY: %s -> %s", mode, proxyURL)
	megaPreviewDiagfV8514("CLICK END    mode=%s elapsed=%s result=OK", mode, time.Since(started).Round(time.Millisecond))
	return proxyURL, mode, time.Since(started), nil
}

func (a *App) startMegaPreviewForUIV854(item RemoteItem, forceFallback bool) (string, string, time.Duration, error) {
	started := time.Now()
	a.previewMu.Lock()
	st := a.preview
	a.previewMu.Unlock()
	megaPreviewDiagfV8514("CLICK START  item=%q forceFallback=%t state={active:%t sameSource:%t root:%t stream:%t exe:%t}", item.Path, forceFallback, st.Active, st.SourceURL == item.URL, st.RemotePath == megaWarmRootRefV86, strings.TrimSpace(st.StreamURL) != "", strings.TrimSpace(st.Exe) != "")

	if !forceFallback {
		if streamURL, mode, ok := a.tryMegaPreviewUICacheV854(item); ok {
			megaPreviewDiagfV8514("CLICK PATH   item=%q path=cache mode=%s", item.Path, mode)
			return a.proxyMegaUIV8513(streamURL, mode, started)
		}
		megaPreviewDiagfV8514("CLICK PATH   item=%q path=resume", item.Path)
		streamURL, err := a.startMegaPreviewResumeCoalescedV856(item)
		if err == nil {
			return a.proxyMegaUIV8513(streamURL, "MEGA DIRECT RESUME", started)
		}
		megaPreviewDiagfV8514("CLICK RESUME item=%q elapsed=%s error=%v", item.Path, time.Since(started).Round(time.Millisecond), err)
		a.logf("MEGA Direct Resume nereușit; încerc fallback per-fișier: %v", err)
	}

	megaPreviewDiagfV8514("CLICK PATH   item=%q path=true-fallback", item.Path)
	streamURL, err := a.startMegaPreviewPerFileFallbackV858(item)
	if err != nil {
		megaPreviewDiagfV8514("CLICK END    item=%q mode=MEGA TRUE FALLBACK elapsed=%s error=%v", item.Path, time.Since(started).Round(time.Millisecond), err)
		return "", "MEGA TRUE FALLBACK", time.Since(started), err
	}
	return a.proxyMegaUIV8513(streamURL, "MEGA TRUE FALLBACK", started)
}
