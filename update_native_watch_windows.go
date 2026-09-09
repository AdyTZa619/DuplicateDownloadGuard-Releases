//go:build windows

package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	nativeUpdateWatchTestManifestV8566   = "https://raw.githubusercontent.com/AdyTZa619/DuplicateDownloadGuard-Releases/testing/update-test.json"
	nativeUpdateWatchStableManifestV8566 = "https://raw.githubusercontent.com/AdyTZa619/DuplicateDownloadGuard-Releases/main/update.json"
	nativeUpdateWatchPollV8566           = 20 * time.Second
)

var (
	nativeUpdateVersionRE8566 = regexp.MustCompile(`(?i)(\d+)\.(\d+)\.(\d+)(?:-([0-9a-z.-]+))?`)
	nativeUpdateNotifyMuV8566 sync.Mutex
)

type nativeUpdateManifestV8566 struct {
	Version string `json:"version"`
}

type nativeParsedVersionV8566 struct {
	core [3]int
	pre  []string
}

func nativeUpdateManifestURLV8566() string {
	if strings.Contains(strings.ToLower(appVersion), "-test.") || strings.Contains(strings.ToLower(appVersion), " test") {
		return nativeUpdateWatchTestManifestV8566
	}
	return nativeUpdateWatchStableManifestV8566
}

func parseNativeVersionV8566(value string) (nativeParsedVersionV8566, bool) {
	m := nativeUpdateVersionRE8566.FindStringSubmatch(strings.TrimSpace(value))
	if len(m) < 4 {
		return nativeParsedVersionV8566{}, false
	}
	var out nativeParsedVersionV8566
	for i := 0; i < 3; i++ {
		n, err := strconv.Atoi(m[i+1])
		if err != nil {
			return nativeParsedVersionV8566{}, false
		}
		out.core[i] = n
	}
	if len(m) >= 5 && strings.TrimSpace(m[4]) != "" {
		out.pre = strings.Split(strings.ToLower(m[4]), ".")
	}
	return out, true
}

func compareNativePreV8566(a, b []string) int {
	if len(a) == 0 && len(b) == 0 {
		return 0
	}
	if len(a) == 0 {
		return 1
	}
	if len(b) == 0 {
		return -1
	}
	limit := len(a)
	if len(b) < limit {
		limit = len(b)
	}
	for i := 0; i < limit; i++ {
		if a[i] == b[i] {
			continue
		}
		an, aerr := strconv.Atoi(a[i])
		bn, berr := strconv.Atoi(b[i])
		aNumeric, bNumeric := aerr == nil, berr == nil
		switch {
		case aNumeric && bNumeric:
			if an > bn {
				return 1
			}
			return -1
		case aNumeric != bNumeric:
			if aNumeric {
				return -1
			}
			return 1
		default:
			if a[i] > b[i] {
				return 1
			}
			return -1
		}
	}
	if len(a) == len(b) {
		return 0
	}
	if len(a) > len(b) {
		return 1
	}
	return -1
}

func nativeVersionIsNewerV8566(remote, local string) bool {
	a, okA := parseNativeVersionV8566(remote)
	b, okB := parseNativeVersionV8566(local)
	if !okA || !okB {
		return false
	}
	for i := 0; i < 3; i++ {
		if a.core[i] == b.core[i] {
			continue
		}
		return a.core[i] > b.core[i]
	}
	return compareNativePreV8566(a.pre, b.pre) > 0
}

func extractNativeUpdateVersionV8566(value string) string {
	m := nativeUpdateVersionRE8566.FindString(strings.TrimSpace(value))
	return strings.TrimSpace(m)
}

func nativeUpdateSoundSeenPathV8566() string {
	return filepath.Join(executableDir(), "data", "updates", "native_update_sound_seen.txt")
}

func readNativeUpdateSoundSeenV8566() string {
	b, err := os.ReadFile(nativeUpdateSoundSeenPathV8566())
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func writeNativeUpdateSoundSeenV8566(version string) {
	version = strings.TrimSpace(version)
	if version == "" {
		return
	}
	path := nativeUpdateSoundSeenPathV8566()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(version+"\n"), 0644); err != nil {
		return
	}
	_ = os.Remove(path)
	_ = os.Rename(tmp, path)
}

func notifyNativeUpdateVersionV8566(version string) (played bool, skipped bool) {
	version = extractNativeUpdateVersionV8566(version)
	if version == "" {
		return false, false
	}
	nativeUpdateNotifyMuV8566.Lock()
	defer nativeUpdateNotifyMuV8566.Unlock()
	if strings.EqualFold(readNativeUpdateSoundSeenV8566(), version) {
		return false, true
	}
	played = playNativeUpdateChimeV8554()
	// Mark the version even if Windows rejected Beep/MessageBeep. Repeating a
	// failed notification every 20 seconds would be worse than one missed sound.
	writeNativeUpdateSoundSeenV8566(version)
	return played, false
}

func fetchNativeUpdateVersionV8566(client *http.Client, manifestURL string) string {
	u, err := url.Parse(manifestURL)
	if err != nil {
		return ""
	}
	q := u.Query()
	q.Set("ddg_native", strconv.FormatInt(time.Now().UnixNano(), 10))
	u.RawQuery = q.Encode()
	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("User-Agent", "DuplicateDownloadGuard/native-update-watch")
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ""
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return ""
	}
	var manifest nativeUpdateManifestV8566
	if err := json.Unmarshal(body, &manifest); err != nil {
		return ""
	}
	return strings.TrimSpace(manifest.Version)
}

func nativeUpdateWatchOnceV8566(client *http.Client) {
	remote := fetchNativeUpdateVersionV8566(client, nativeUpdateManifestURLV8566())
	if remote == "" || !nativeVersionIsNewerV8566(remote, appVersion) {
		return
	}
	notifyNativeUpdateVersionV8566(remote)
}

func nativeUpdateWatchLoopV8566() {
	client := &http.Client{Timeout: 8 * time.Second}
	time.Sleep(3 * time.Second)
	nativeUpdateWatchOnceV8566(client)
	ticker := time.NewTicker(nativeUpdateWatchPollV8566)
	defer ticker.Stop()
	for range ticker.C {
		nativeUpdateWatchOnceV8566(client)
	}
}

func init() {
	if runningNativeUpdaterMode(os.Args) {
		return
	}
	go nativeUpdateWatchLoopV8566()
}
