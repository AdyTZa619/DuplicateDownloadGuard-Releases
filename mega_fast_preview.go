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
	out, err := runMegaTimed(ctx, 15*time.Second, exe, "webdav", megaWarmRootRefV86)
	if err != nil {
		return "", err
	}
	rootURL := extractWebDAVURL(out, "")
	if rootURL == "" {
		listing, listErr := runMegaTimed(ctx, 4*time.Second, exe, "webdav")
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
	ctx, cancel := context.WithTimeout(parent, 900*time.Millisecond)
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
