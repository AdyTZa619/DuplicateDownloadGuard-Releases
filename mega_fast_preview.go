package main

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const megaWarmRootRefV86 = "/"

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
	clean := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || part == "." {
			continue
		}
		if part == ".." {
			return "", errors.New("cale remote WebDAV invalidă")
		}
		clean = append(clean, part)
	}
	if len(clean) == 0 {
		return root.String(), nil
	}
	root.RawQuery = ""
	root.Fragment = ""
	root.Path = strings.TrimRight(root.Path, "/") + "/" + strings.Join(clean, "/")
	root.RawPath = ""
	return root.String(), nil
}

func extractExactWebDAVURLV8511(out, remotePath string) string {
	remotePath = strings.TrimSpace(remotePath)
	for _, line := range strings.Split(strings.ReplaceAll(out, "\r", ""), "\n") {
		u := webdavURLRE.FindString(line)
		if u == "" {
			continue
		}
		idx := strings.Index(line, u)
		if idx < 0 {
			continue
		}
		left := strings.TrimSpace(line[:idx])
		left = strings.TrimSpace(strings.TrimSuffix(left, ":"))
		const prefix = "serving via webdav "
		if lower := strings.ToLower(left); strings.HasPrefix(lower, prefix) {
			left = strings.TrimSpace(left[len(prefix):])
		}
		if left == remotePath {
			return strings.TrimSpace(u)
		}
	}
	return ""
}

func listMegaWarmRootV8511(ctx context.Context, exe string, timeout time.Duration) (string, error) {
	if strings.TrimSpace(exe) == "" {
		return "", errors.New("MEGAcmd lipsă")
	}
	out, err := runMegaTimedPreviewV8514(ctx, timeout, exe, "webdav")
	if err != nil {
		return "", err
	}
	return extractExactWebDAVURLV8511(out, megaWarmRootRefV86), nil
}

func startMegaWarmRootV86(ctx context.Context, exe string) (string, error) {
	if strings.TrimSpace(exe) == "" {
		return "", errors.New("MEGAcmd lipsă")
	}
	out, err := runMegaTimedPreviewV8514(ctx, 15*time.Second, exe, "webdav", megaWarmRootRefV86)
	if err != nil {
		return "", err
	}
	rootURL := extractExactWebDAVURLV8511(out, megaWarmRootRefV86)
	if rootURL == "" {
		listing, listErr := runMegaTimedPreviewV8514(ctx, 4*time.Second, exe, "webdav")
		if listErr != nil {
			return "", listErr
		}
		rootURL = extractExactWebDAVURLV8511(listing, megaWarmRootRefV86)
	}
	if rootURL == "" {
		return "", errors.New("MEGAcmd nu a returnat URL pentru WebDAV root")
	}
	return rootURL, nil
}

func ensureMegaWarmRootAfterScanV8512(ctx context.Context, exe string) (string, error) {
	if rootURL, err := listMegaWarmRootV8511(ctx, exe, 1500*time.Millisecond); err == nil && rootURL != "" {
		return rootURL, nil
	}
	out, err := runMegaTimedPreviewV8514(ctx, 30*time.Second, exe, "webdav", megaWarmRootRefV86)
	if err != nil {
		if rootURL, listErr := listMegaWarmRootV8511(ctx, exe, 2500*time.Millisecond); listErr == nil && rootURL != "" {
			return rootURL, nil
		}
		return "", err
	}
	if rootURL := extractExactWebDAVURLV8511(out, megaWarmRootRefV86); rootURL != "" {
		return rootURL, nil
	}
	rootURL, listErr := listMegaWarmRootV8511(ctx, exe, 2500*time.Millisecond)
	if listErr != nil {
		return "", listErr
	}
	if rootURL == "" {
		return "", errors.New("MEGAcmd nu a returnat URL pentru WebDAV root după scanare")
	}
	return rootURL, nil
}

func warmMegaWebDAVTransportV8512(rootURL string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2500*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "PROPFIND", rootURL, nil)
	if err != nil {
		return
	}
	req.Header.Set("Depth", "0")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	_ = resp.Body.Close()
}

func (a *App) prepareMegaWarmRootAfterScanV86(ctx context.Context, exe, sourceURL, previousSession string) error {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	rootCtx, cancel := context.WithTimeout(context.Background(), 38*time.Second)
	defer cancel()
	started := time.Now()
	rootURL, err := ensureMegaWarmRootAfterScanV8512(rootCtx, exe)
	if err != nil {
		return err
	}
	warmMegaWebDAVTransportV8512(rootURL)
	a.previewMu.Lock()
	a.preview = MegaPreviewState{
		Active:          true,
		SourceURL:       sourceURL,
		RemotePath:      megaWarmRootRefV86,
		StreamURL:       rootURL,
		PreviousSession: previousSession,
		Exe:             exe,
	}
	a.resetPreviewTTLLocked()
	a.previewMu.Unlock()
	a.logf("MEGA Fast Preview: WebDAV root pregătit și încălzit în %d ms -> %s", time.Since(started).Milliseconds(), rootURL)
	return nil
}

func probeWarmWebDAVChildV86(parent context.Context, target string) bool {
	ctx, cancel := context.WithTimeout(parent, 350*time.Millisecond)
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
