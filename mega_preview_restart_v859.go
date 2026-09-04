package main

import (
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// v8.5.9 keeps a non-secret hint that DDG intentionally left the public MEGA
// folder session active. v8.5.10 can additionally remember the local WebDAV
// root URL. The MEGAcmd session token itself is never written by DDG.
type megaPreviewRestartHintV859 struct {
	SourceURL string `json:"sourceUrl"`
	RootURL   string `json:"rootUrl,omitempty"`
	KeptAt    int64  `json:"keptAt"`
}

func (a *App) megaPreviewRestartHintPathV859() string {
	if a == nil || strings.TrimSpace(a.appDir) == "" {
		return ""
	}
	return filepath.Join(a.appDir, "cache", "mega_preview_restart.json")
}

func (a *App) writeMegaPreviewRestartHintV8510(sourceURL, rootURL string) {
	path := a.megaPreviewRestartHintPathV859()
	sourceURL = strings.TrimSpace(sourceURL)
	rootURL = strings.TrimSpace(rootURL)
	if path == "" || sourceURL == "" {
		return
	}
	_ = os.MkdirAll(filepath.Dir(path), 0755)
	b, err := json.Marshal(megaPreviewRestartHintV859{SourceURL: sourceURL, RootURL: rootURL, KeptAt: time.Now().Unix()})
	if err != nil {
		return
	}
	_ = os.WriteFile(path, b, 0600)
}

func (a *App) saveMegaPreviewRestartHintV859(sourceURL string) {
	a.writeMegaPreviewRestartHintV8510(sourceURL, "")
}

func (a *App) saveMegaPreviewRestartRootV8510(sourceURL, rootURL string) {
	a.writeMegaPreviewRestartHintV8510(sourceURL, rootURL)
}

func (a *App) clearMegaPreviewRestartHintV859() {
	if path := a.megaPreviewRestartHintPathV859(); path != "" {
		_ = os.Remove(path)
	}
}

func (a *App) loadMegaPreviewRestartHintV8510(sourceURL string) (megaPreviewRestartHintV859, bool) {
	var hint megaPreviewRestartHintV859
	path := a.megaPreviewRestartHintPathV859()
	if path == "" {
		return hint, false
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return hint, false
	}
	if json.Unmarshal(b, &hint) != nil {
		return megaPreviewRestartHintV859{}, false
	}
	if hint.KeptAt <= 0 || time.Since(time.Unix(hint.KeptAt, 0)) > 7*24*time.Hour {
		a.clearMegaPreviewRestartHintV859()
		return megaPreviewRestartHintV859{}, false
	}
	if strings.TrimSpace(hint.SourceURL) == "" || strings.TrimSpace(hint.SourceURL) != strings.TrimSpace(sourceURL) {
		return megaPreviewRestartHintV859{}, false
	}
	return hint, true
}

func (a *App) matchesMegaPreviewRestartHintV859(sourceURL string) bool {
	_, ok := a.loadMegaPreviewRestartHintV8510(sourceURL)
	return ok
}

func localMegaWebDAVRootReachableV8510(rawURL string) bool {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return false
	}
	host := u.Hostname()
	ip := net.ParseIP(host)
	if !strings.EqualFold(host, "localhost") && (ip == nil || !ip.IsLoopback()) {
		return false
	}
	port := u.Port()
	if port == "" {
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 350*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// tryPersistedMegaRootV8510 performs no MEGAcmd command and no remote HTTP
// request. It only verifies that the remembered loopback WebDAV listener is
// still alive, then derives the selected child URL locally.
func (a *App) tryPersistedMegaRootV8510(item RemoteItem) (string, string, bool) {
	hint, ok := a.loadMegaPreviewRestartHintV8510(item.URL)
	if !ok || strings.TrimSpace(hint.RootURL) == "" {
		return "", "", false
	}
	if !localMegaWebDAVRootReachableV8510(hint.RootURL) {
		a.clearMegaPreviewRestartHintV859()
		return "", "", false
	}
	child, err := megaWebDAVChildURL(hint.RootURL, item.Path)
	if err != nil || strings.TrimSpace(child) == "" {
		a.clearMegaPreviewRestartHintV859()
		return "", "", false
	}
	return hint.RootURL, child, true
}

// tryMegaCurrentSessionWebDAVV859 is deliberately short. It is only used when
// the restart hint says DDG left this exact public-folder session active. If the
// session was changed externally, fail quickly and let the normal --resume path
// take over instead of adding another long wait.
func tryMegaCurrentSessionWebDAVV859(remoteRef string, run megaWebDAVRunnerV85) (megaWebDAVSwitchResultV85, error) {
	var result megaWebDAVSwitchResultV85
	remoteRef = strings.TrimSpace(remoteRef)
	if remoteRef == "" {
		return result, errors.New("referință MEGA remote lipsă")
	}
	if run == nil {
		return result, errors.New("MEGA WebDAV runner lipsă")
	}
	out, err := run(4*time.Second, "webdav", remoteRef)
	result.StartOutput = out
	if err != nil {
		return result, err
	}
	result.StreamURL = extractWebDAVURL(out, remoteRef)
	if result.StreamURL == "" {
		listing, _ := run(1200*time.Millisecond, "webdav")
		result.StreamURL = extractWebDAVURL(listing, remoteRef)
	}
	if result.StreamURL == "" {
		_, _ = run(1500*time.Millisecond, "webdav", "-d", remoteRef)
		return result, errors.New(megaWebDAVURLMissingV85)
	}
	return result, nil
}
