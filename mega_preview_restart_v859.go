package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// v8.5.9 keeps only a non-secret hint that DDG intentionally left the public
// MEGA folder session active. The MEGAcmd session token itself is never written
// by DDG. On the next start we can try WebDAV directly before paying for another
// session/logout/login cycle.
type megaPreviewRestartHintV859 struct {
	SourceURL string `json:"sourceUrl"`
	KeptAt    int64  `json:"keptAt"`
}

func (a *App) megaPreviewRestartHintPathV859() string {
	if a == nil || strings.TrimSpace(a.appDir) == "" {
		return ""
	}
	return filepath.Join(a.appDir, "cache", "mega_preview_restart.json")
}

func (a *App) saveMegaPreviewRestartHintV859(sourceURL string) {
	path := a.megaPreviewRestartHintPathV859()
	sourceURL = strings.TrimSpace(sourceURL)
	if path == "" || sourceURL == "" {
		return
	}
	_ = os.MkdirAll(filepath.Dir(path), 0755)
	b, err := json.Marshal(megaPreviewRestartHintV859{SourceURL: sourceURL, KeptAt: time.Now().Unix()})
	if err != nil {
		return
	}
	_ = os.WriteFile(path, b, 0600)
}

func (a *App) clearMegaPreviewRestartHintV859() {
	if path := a.megaPreviewRestartHintPathV859(); path != "" {
		_ = os.Remove(path)
	}
}

func (a *App) matchesMegaPreviewRestartHintV859(sourceURL string) bool {
	path := a.megaPreviewRestartHintPathV859()
	if path == "" {
		return false
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var hint megaPreviewRestartHintV859
	if json.Unmarshal(b, &hint) != nil {
		return false
	}
	if hint.KeptAt <= 0 || time.Since(time.Unix(hint.KeptAt, 0)) > 7*24*time.Hour {
		a.clearMegaPreviewRestartHintV859()
		return false
	}
	return strings.TrimSpace(hint.SourceURL) != "" && strings.TrimSpace(hint.SourceURL) == strings.TrimSpace(sourceURL)
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
	// MegaClient itself can wait up to MEGAcmd's RESUME_SESSION_TIMEOUT while
	// the background server restores its cache. The old four-second deadline
	// aborted that legitimate cold resume and immediately stacked a complete
	// session/logout/login sequence behind it, producing the observed ~30 s.
	out, err := run(18*time.Second, "webdav", remoteRef)
	result.StartOutput = out
	if err != nil {
		return result, err
	}
	result.StreamURL = extractWebDAVURL(out, remoteRef)
	if result.StreamURL == "" {
		listing, _ := run(4*time.Second, "webdav")
		result.StreamURL = extractWebDAVURL(listing, remoteRef)
	}
	if result.StreamURL == "" {
		_, _ = run(4*time.Second, "webdav", "-d", remoteRef)
		return result, errors.New(megaWebDAVURLMissingV85)
	}
	return result, nil
}
