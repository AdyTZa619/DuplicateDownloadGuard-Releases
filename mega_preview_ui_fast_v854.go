package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

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

	// Whole-folder WebDAV root prepared by the scan. Construct the child URL
	// locally; do not HEAD it here. HEAD was the main source of false misses on
	// MEGAcmd WebDAV and forced the 15+ second per-file fallback.
	if st.RemotePath == megaWarmRootRefV86 {
		child, err := megaWebDAVChildURL(st.StreamURL, item.Path)
		if err == nil && child != "" {
			a.resetPreviewTTLLocked()
			a.logf("MEGA UI Fast Preview root hit: %s -> %s", item.Path, child)
			return child, "MEGA FAST ROOT", true
		}
	}

	// A per-file WebDAV node already exists for this exact item. Reusing it is
	// also zero-command and should be immediate when returning to a row.
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

	// Recheck after acquiring the MEGA gate: another request may have warmed the
	// root while this request was waiting.
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
		rootURL, err := startMegaWarmRootV86(context.Background(), a.preview.Exe)
		if err == nil {
			a.preview.RemotePath = megaWarmRootRefV86
			a.preview.StreamURL = rootURL
			a.resetPreviewTTLLocked()
			child, childErr := megaWebDAVChildURL(rootURL, item.Path)
			if childErr == nil && child != "" {
				a.logf("MEGA Fast Preview: root reparat fără relogin -> %s", rootURL)
				return child, nil
			}
		}
		a.logf("MEGA Fast Preview: sesiunea era caldă, dar root WebDAV nu a putut fi refăcut: %v", err)
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
	if oldSession != "" {
		_, _ = runMegaTimed(ctx, 8*time.Second, exe, "logout", "--keep-session")
	} else {
		// keep-session is intentional even for a folder-link session restored by
		// MEGAcmd itself; it avoids deleting a useful public-folder cache.
		_, _ = runMegaTimed(ctx, 8*time.Second, exe, "logout", "--keep-session")
	}

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
	child, err := megaWebDAVChildURL(rootURL, item.Path)
	if err != nil || child == "" {
		return "", fmt.Errorf("WebDAV root este activ, dar calea fișierului nu poate fi construită: %w", err)
	}
	a.logf("MEGA Fast Preview restart: cache folder reluat cu --resume -> %s", rootURL)
	return child, nil
}

func (a *App) startMegaPreviewForUIV854(item RemoteItem, forceFallback bool) (string, string, time.Duration, error) {
	started := time.Now()
	if !forceFallback {
		if streamURL, mode, ok := a.tryMegaPreviewUICacheV854(item); ok {
			return streamURL, mode, time.Since(started), nil
		}
		streamURL, err := a.startMegaPreviewResumeV856(item)
		if err == nil {
			return streamURL, "MEGA FAST RESUME", time.Since(started), nil
		}
		// Keep the old per-file path as a compatibility fallback. It is only
		// reached when resume/root startup genuinely failed.
		a.logf("MEGA Fast Resume nereușit; încerc fallback per-fișier: %v", err)
	}
	streamURL, err := a.startMegaPreview(item)
	if err != nil {
		return "", "MEGA FALLBACK", time.Since(started), err
	}
	return streamURL, "MEGA FALLBACK", time.Since(started), nil
}
