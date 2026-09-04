package main

import (
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

func (a *App) startMegaPreviewForUIV854(item RemoteItem) (string, string, time.Duration, error) {
	started := time.Now()
	if streamURL, mode, ok := a.tryMegaPreviewUICacheV854(item); ok {
		return streamURL, mode, time.Since(started), nil
	}
	streamURL, err := a.startMegaPreview(item)
	if err != nil {
		return "", "MEGA FALLBACK", time.Since(started), err
	}
	return streamURL, "MEGA FALLBACK", time.Since(started), nil
}
