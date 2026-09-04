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
		// Keep the decoded component in Path. url.URL.String() performs the
		// escaping exactly once; pre-escaping here would turn %20 into %2520.
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

// extractExactWebDAVURLV8511 fixes a subtle root-selection bug in the older
// parser. Searching a listing for "/" matched every path and every URL, so a
// per-file endpoint listed before the root could accidentally be treated as the
// root. Match the served MEGA path on the left side of the URL exactly instead.
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
	out, err := runMegaTimed(ctx, timeout, exe, "webdav")
	if err != nil {
		return "", err
	}
	return extractExactWebDAVURLV8511(out, megaWarmRootRefV86), nil
}

func startMegaWarmRootV86(ctx context.Context, exe string) (string, error) {
	if strings.TrimSpace(exe) == "" {
		return "", errors.New("MEGAcmd lipsă")
	}
	out, err := runMegaTimed(ctx, 15*time.Second, exe, "webdav", megaWarmRootRefV86)
	if err != nil {
		return "", err
	}
	rootURL := extractExactWebDAVURLV8511(out, megaWarmRootRefV86)
	if rootURL == "" {
		listing, listErr := runMegaTimed(ctx, 4*time.Second, exe, "webdav")
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

// prepareMegaWarmRootAfterScanV86 pays the WebDAV startup cost while the MEGA
// public-folder session is already hot. The scan context may be almost expired
// after a long listing, so root preparation gets its own short bounded context.
func (a *App) prepareMegaWarmRootAfterScanV86(ctx context.Context, exe, sourceURL, previousSession string) error {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	rootCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	rootURL, err := startMegaWarmRootV86(rootCtx, exe)
	if err != nil {
		return err
	}
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
	a.logf("MEGA Fast Preview: WebDAV root pregătit -> %s", rootURL)
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
