package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Bunkr changes its domains, API and CDN rules frequently. Do not duplicate
// that protocol in DDG. For Bunkr, DDG deliberately delegates extraction and
// integrated downloads to the maintained gallery-dl implementation and keeps
// only DDG's comparison/index/UI layer around it.
//
// The managed binary comes from gdl-org/builds, the same official build source
// already used by DDG's Tool Manager.

const bunkrGalleryDLCheckV8560 = 12 * time.Hour

type bunkrGalleryDLMarkerV8560 struct {
	Tag       string `json:"tag"`
	CheckedAt int64  `json:"checkedAt"`
}

func managedGalleryDLPathV8560() string {
	return filepath.Join(portableToolsDir(), "gallery-dl", "gallery-dl.exe")
}

func bunkrGalleryDLMarkerPathV8560() string {
	return filepath.Join(portableToolsDir(), "gallery-dl", ".ddg-official-build.json")
}

func regularFileV8560(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

func readBunkrGalleryDLMarkerV8560() bunkrGalleryDLMarkerV8560 {
	var marker bunkrGalleryDLMarkerV8560
	b, err := os.ReadFile(bunkrGalleryDLMarkerPathV8560())
	if err == nil {
		_ = json.Unmarshal(b, &marker)
	}
	return marker
}

func writeBunkrGalleryDLMarkerV8560(marker bunkrGalleryDLMarkerV8560) {
	path := bunkrGalleryDLMarkerPathV8560()
	_ = os.MkdirAll(filepath.Dir(path), 0755)
	if b, err := json.Marshal(marker); err == nil {
		_ = os.WriteFile(path, b, 0644)
	}
}

// officialGalleryDLForBunkrV8560 keeps the portable Bunkr extractor current.
// It checks at most twice a day and only downloads when the official build tag
// changed. If GitHub is unavailable, it fails open to the best local executable
// instead of blocking a scan.
func (a *App) officialGalleryDLForBunkrV8560(parent context.Context) string {
	portable := managedGalleryDLPathV8560()
	marker := readBunkrGalleryDLMarkerV8560()
	now := time.Now()
	if regularFileV8560(portable) && marker.CheckedAt > 0 && now.Sub(time.Unix(marker.CheckedAt, 0)) < bunkrGalleryDLCheckV8560 {
		return portable
	}

	checkCtx, cancel := context.WithTimeout(parent, 25*time.Second)
	rel, err := githubLatestRelease(checkCtx, "gdl-org/builds")
	cancel()
	if err != nil {
		if regularFileV8560(portable) {
			return portable
		}
		return a.detectGalleryDL()
	}

	needInstall := !regularFileV8560(portable) || strings.TrimSpace(marker.Tag) != strings.TrimSpace(rel.TagName)
	if needInstall {
		_, assetURL, assetErr := releaseAsset(rel, `(?i)^gallery-dl_windows\.exe$`)
		if assetErr == nil {
			dlCtx, dlCancel := context.WithTimeout(parent, 3*time.Minute)
			dlErr := downloadToFile(dlCtx, assetURL, portable, func(_, _ int64) {})
			dlCancel()
			if dlErr == nil {
				marker.Tag = strings.TrimSpace(rel.TagName)
			} else if a != nil {
				a.logf("BUNKR: actualizarea buildului oficial gallery-dl a eșuat; folosesc fallback local: %v", dlErr)
			}
		} else if a != nil {
			a.logf("BUNKR: assetul oficial gallery-dl Windows nu a fost găsit: %v", assetErr)
		}
	}
	marker.CheckedAt = now.Unix()
	writeBunkrGalleryDLMarkerV8560(marker)

	if regularFileV8560(portable) {
		return portable
	}
	return a.detectGalleryDL()
}

// Provider refresh/preview runs outside the original scan method and cannot
// safely update tools there. Prefer the managed official binary produced by the
// Bunkr scan; fall back to the older detector only when it is absent.
func preferredGalleryDLProviderV8560() string {
	if portable := managedGalleryDLPathV8560(); regularFileV8560(portable) {
		return portable
	}
	return detectGalleryDLForProviderV86()
}

func bunkrMediaPageURLV8560(res Result) string {
	if !strings.EqualFold(strings.TrimSpace(res.Remote.Source), "BUNKR") {
		return ""
	}
	slug := strings.TrimSpace(res.Remote.Handle)
	if slug == "" {
		return ""
	}
	page, err := url.Parse(strings.TrimSpace(res.Remote.URL))
	if err != nil || page.Scheme == "" || page.Host == "" {
		return ""
	}
	return page.Scheme + "://" + page.Host + "/f/" + url.PathEscape(slug)
}

func galleryDLFilenameFormatV8560(name string) string {
	name = sanitizeFilename(name)
	// gallery-dl filename formats use braces. Escape literal braces so a remote
	// filename cannot accidentally become a formatting expression.
	name = strings.ReplaceAll(name, "{", "{{")
	name = strings.ReplaceAll(name, "}", "}}")
	return name
}

// galleryDLDownloadBunkrV8560 lets gallery-dl perform the HTTP/API work. This
// mirrors the proven division of responsibility used by mature download tools:
// the provider plugin resolves fresh URLs/headers; DDG only chooses the selected
// item, destination and records the completed file.
func (a *App) galleryDLDownloadBunkrV8560(ctx context.Context, res Result, dest string, progress func(int64, int64)) (string, error) {
	mediaPage := bunkrMediaPageURLV8560(res)
	if mediaPage == "" {
		return "", errors.New("BUNKR: lipsește slug-ul paginii media; rescanează albumul cu versiunea curentă DDG")
	}
	exe := a.officialGalleryDLForBunkrV8560(ctx)
	if strings.TrimSpace(exe) == "" {
		return "", errors.New("BUNKR: gallery-dl oficial nu este disponibil")
	}
	if err := os.MkdirAll(dest, 0755); err != nil {
		return "", err
	}

	cleanName := sanitizeFilename(res.Remote.Name)
	if cleanName == "" {
		cleanName = "bunkr-file"
	}
	expected := filepath.Join(dest, cleanName)
	format := galleryDLFilenameFormatV8560(cleanName)
	args := []string{
		"--config-ignore", "--no-input", "--no-colors", "--windows-filenames",
		"-D", dest,
		"-f", format,
		"-o", "extractor.bunkr.tlds=true",
		mediaPage,
	}

	cmd := exec.CommandContext(ctx, exe, args...)
	hideChildWindow(cmd)
	var stderr bytes.Buffer
	cmd.Stdout = &stderr
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return "", err
	}

	total := res.Remote.Size
	if total < 0 {
		total = 0
	}
	if progress != nil {
		progress(0, total)
	}
	partCandidates := []string{expected + ".part", expected + ".part~"}
	doneCh := make(chan error, 1)
	go func() { doneCh <- cmd.Wait() }()
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case err := <-doneCh:
			if err != nil {
				msg := strings.TrimSpace(stderr.String())
				if len(msg) > 600 {
					msg = msg[len(msg)-600:]
				}
				if msg == "" {
					msg = err.Error()
				}
				return "", fmt.Errorf("BUNKR/gallery-dl: %s", msg)
			}
			st, statErr := os.Stat(expected)
			if statErr != nil || st.IsDir() {
				return "", errors.New("BUNKR/gallery-dl a terminat fără fișierul așteptat")
			}
			if progress != nil {
				progress(st.Size(), st.Size())
			}
			return expected, nil
		case <-ticker.C:
			if progress == nil {
				continue
			}
			for _, part := range partCandidates {
				if st, err := os.Stat(part); err == nil && !st.IsDir() {
					progress(st.Size(), total)
					break
				}
			}
		case <-ctx.Done():
			_ = cmd.Process.Kill()
			return "", ctx.Err()
		}
	}
}
