package main

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const megaWarmRootRefV86 = "/"

type megaWarmRootEntryV86 struct {
	URL         string
	SourceURL   string
	Exe         string
	ValidatedAt time.Time
}

var megaWarmRootCacheV86 sync.Map

func megaWebDAVChildURL(rootURL, remotePath string) (string, error) {
	root, err := url.Parse(strings.TrimSpace(rootURL))
	if err != nil || (root.Scheme != "http" && root.Scheme != "https") || root.Host == "" {
		return "", errors.New("URL WebDAV root invalid")
	}
	remotePath = strings.Trim(strings.ReplaceAll(remotePath, "\\", "/"), "/")
	if remotePath == "" {
		return root.String(), nil
	}
	parts := strings.Split(remotePath, "/")
	escaped := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || part == "." {
			continue
		}
		if part == ".." {
			return "", errors.New("cale remote WebDAV invalidă")
		}
		escaped = append(escaped, url.PathEscape(part))
	}
	if len(escaped) == 0 {
		return root.String(), nil
	}
	root.RawQuery = ""
	root.Fragment = ""
	root.Path = strings.TrimRight(root.Path, "/") + "/" + strings.Join(escaped, "/")
	root.RawPath = ""
	return root.String(), nil
}

func startMegaWarmRootV86(ctx context.Context, exe string) (string, error) {
	if strings.TrimSpace(exe) == "" {
		return "", errors.New("MEGAcmd lipsă")
	}
	out, err := runMegaTimed(ctx, 12*time.Second, exe, "webdav", megaWarmRootRefV86)
	if err != nil {
		return "", err
	}
	rootURL := extractWebDAVURL(out, "")
	if rootURL == "" {
		listing, listErr := runMegaTimed(ctx, 3*time.Second, exe, "webdav")
		if listErr != nil {
			return "", listErr
		}
		rootURL = extractWebDAVURL(listing, megaWarmRootRefV86)
	}
	if rootURL == "" {
		return "", errors.New("MEGAcmd nu a returnat URL pentru WebDAV root")
	}
	return rootURL, nil
}

func probeWarmWebDAVChildV86(parent context.Context, target string) bool {
	ctx, cancel := context.WithTimeout(parent, 650*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, target, nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 400
}

func megaItemPathForRefV86(a *App, sourceURL, remoteRef string) (string, bool) {
	if a == nil || strings.TrimSpace(sourceURL) == "" || strings.TrimSpace(remoteRef) == "" {
		return "", false
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	for _, r := range a.results {
		if !strings.EqualFold(r.Remote.Source, "MEGA") || r.Remote.URL != sourceURL {
			continue
		}
		if megaRemoteRef(r.Remote) == remoteRef && strings.TrimSpace(r.Remote.Path) != "" {
			return r.Remote.Path, true
		}
	}
	return "", false
}

func cachedWarmRootChildV86(a *App, sourceURL, remoteRef string) (string, bool) {
	raw, ok := megaWarmRootCacheV86.Load(sourceURL)
	if !ok {
		return "", false
	}
	entry, ok := raw.(megaWarmRootEntryV86)
	if !ok || entry.URL == "" {
		megaWarmRootCacheV86.Delete(sourceURL)
		return "", false
	}
	remotePath, ok := megaItemPathForRefV86(a, sourceURL, remoteRef)
	if !ok {
		return "", false
	}
	child, err := megaWebDAVChildURL(entry.URL, remotePath)
	if err != nil {
		return "", false
	}
	// While the user is browsing several rows, trust the already validated local
	// root for 30 seconds. This removes even the HEAD round-trip from the hot path.
	if time.Since(entry.ValidatedAt) <= 30*time.Second {
		return child, true
	}
	if !probeWarmWebDAVChildV86(context.Background(), child) {
		megaWarmRootCacheV86.Delete(sourceURL)
		return "", false
	}
	entry.ValidatedAt = time.Now()
	megaWarmRootCacheV86.Store(sourceURL, entry)
	return child, true
}

func ensureWarmRootChildV86(a *App, old MegaPreviewState, remoteRef string) (string, bool) {
	if child, ok := cachedWarmRootChildV86(a, old.SourceURL, remoteRef); ok {
		return child, true
	}
	// Immediately after a MEGA scan, keepMegaSessionWarm leaves a live session
	// marker with no per-file WebDAV node. Use that exact moment to expose the
	// entire public folder once. If a particular MEGAcmd build does not support
	// this cleanly, return false and the existing per-file path remains fallback.
	if !old.Active || old.Exe == "" || old.SourceURL == "" || old.RemotePath != "" || old.StreamURL != "" {
		return "", false
	}
	remotePath, ok := megaItemPathForRefV86(a, old.SourceURL, remoteRef)
	if !ok {
		return "", false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 13*time.Second)
	defer cancel()
	rootURL, err := startMegaWarmRootV86(ctx, old.Exe)
	if err != nil {
		a.logf("MEGA Fast Preview: WebDAV root indisponibil, folosesc fallback per-fișier: %v", err)
		return "", false
	}
	child, err := megaWebDAVChildURL(rootURL, remotePath)
	if err != nil || !probeWarmWebDAVChildV86(context.Background(), child) {
		// Root was exposed but the path mapping is not compatible with this
		// MEGAcmd build/source. Do not trust it; the established per-file logic is
		// still the fallback and remains fully functional.
		_, _ = runMegaTimed(context.Background(), 3*time.Second, old.Exe, "webdav", "-d", megaWarmRootRefV86)
		return "", false
	}
	megaWarmRootCacheV86.Store(old.SourceURL, megaWarmRootEntryV86{URL: rootURL, SourceURL: old.SourceURL, Exe: old.Exe, ValidatedAt: time.Now()})
	a.logf("MEGA Fast Preview: folderul public este servit WebDAV o singură dată; preview-urile următoare folosesc URL-uri locale derivate")
	return child, true
}

func warmRootPreviewURLV86(st MegaPreviewState, item RemoteItem) (string, bool) {
	if !st.Active || st.RemotePath != megaWarmRootRefV86 || strings.TrimSpace(st.StreamURL) == "" {
		return "", false
	}
	if st.SourceURL != item.URL {
		return "", false
	}
	child, err := megaWebDAVChildURL(st.StreamURL, item.Path)
	if err != nil {
		return "", false
	}
	if !probeWarmWebDAVChildV86(context.Background(), child) {
		return "", false
	}
	return child, true
}
