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

// startMegaPreviewResumeV856 is the restart path for the embedded player.
// MEGAcmd normally reloads an exported folder from scratch when login receives
// only the folder URL. --resume tells it to reuse its local folder cache. DDG
// preserves that cache on shutdown (logout --keep-session), so reopening the
// application does not need another full public-folder bootstrap.
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

	// A scan may have left the public-folder session warm even if WebDAV root
	// creation failed. In that case do not login again; just retry the root.
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
	if out, err := runMegaTimed(ctx, 8*time.Second, exe, "session"); err == nil {
		oldSession = extractSession(out)
	}
	// Always preserve the current MEGAcmd cache before switching entity. This is
	// cheap when there is no session and essential when the current entity is a
	// folder link that DDG may want to resume later.
	_, _ = runMegaTimed(ctx, 8*time.Second, exe, "logout", "--keep-session")

	loginArgs := megaPublicLoginArgsV856(item.URL)
	loginOut, err := runMegaTimed(ctx, 45*time.Second, exe, loginArgs...)
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

// startMegaPreviewResumeCoalescedV856 prevents the background restart prewarm
// and a user click from launching two competing MEGAcmd login/webdav sequences.
// All callers share one initialization, then resolve their own child URL from
// the warmed root.
func (a *App) startMegaPreviewResumeCoalescedV856(item RemoteItem) (string, error) {
	for attempt := 0; attempt < 2; attempt++ {
		if streamURL, _, ok := a.tryMegaPreviewUICacheV854(item); ok {
			return streamURL, nil
		}

		megaPreviewResumeFlightV856.Lock()
		if inFlight := megaPreviewResumeFlightV856.done; inFlight != nil {
			megaPreviewResumeFlightV856.Unlock()
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

		streamURL, err := a.startMegaPreviewResumeV856(item)
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

func (a *App) startMegaPreviewForUIV854(item RemoteItem, forceFallback bool) (string, string, time.Duration, error) {
	started := time.Now()
	if !forceFallback {
		if streamURL, mode, ok := a.tryMegaPreviewUICacheV854(item); ok {
			return streamURL, mode, time.Since(started), nil
		}
		streamURL, err := a.startMegaPreviewResumeCoalescedV856(item)
		if err == nil {
			return streamURL, "MEGA FAST RESUME", time.Since(started), nil
		}
		a.logf("MEGA Fast Resume nereușit; încerc fallback per-fișier: %v", err)
	}
	streamURL, err := a.startMegaPreview(item)
	if err != nil {
		return "", "MEGA FALLBACK", time.Since(started), err
	}
	return streamURL, "MEGA FALLBACK", time.Since(started), nil
}
