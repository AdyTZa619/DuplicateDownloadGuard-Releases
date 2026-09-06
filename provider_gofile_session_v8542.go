package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const gofileGuestStateVersionV8542 = 1
const gofileCurrentWebsiteSaltV8542 = "12af056dacea0b"

var gofileGuestStateMuV8542 sync.Mutex

type gofileGuestStateV8542 struct {
	Version       int    `json:"version"`
	Token         string `json:"token,omitempty"`
	CreatedAt     int64  `json:"createdAt,omitempty"`
	CooldownUntil int64  `json:"cooldownUntil,omitempty"`
	WorkingSalt   string `json:"workingSalt,omitempty"`
}

type gofileAPIErrorV8542 struct {
	HTTPStatus int
	APIStatus  string
}

func (e *gofileAPIErrorV8542) Error() string {
	status := strings.TrimSpace(e.APIStatus)
	if status != "" {
		return fmt.Sprintf("GoFile API HTTP %d (%s)", e.HTTPStatus, status)
	}
	return fmt.Sprintf("GoFile API HTTP %d", e.HTTPStatus)
}

func gofileGuestStatePathV8542() string {
	if override := strings.TrimSpace(os.Getenv("DDG_GOFILE_STATE_PATH")); override != "" {
		return override
	}
	if dataDir, err := portableDataDir(); err == nil {
		return filepath.Join(dataDir, "cache", "gofile_guest_state.json")
	}
	return filepath.Join(executableDir(), "data", "cache", "gofile_guest_state.json")
}

func loadGofileGuestStateUnlockedV8542() gofileGuestStateV8542 {
	var state gofileGuestStateV8542
	data, err := os.ReadFile(gofileGuestStatePathV8542())
	if err != nil {
		return state
	}
	if json.Unmarshal(data, &state) != nil || state.Version != gofileGuestStateVersionV8542 {
		return gofileGuestStateV8542{}
	}
	return state
}

func saveGofileGuestStateUnlockedV8542(state gofileGuestStateV8542) error {
	state.Version = gofileGuestStateVersionV8542
	path := gofileGuestStatePathV8542()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	_ = os.Remove(path)
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func loadCachedGofileGuestTokenV8542() (string, bool) {
	gofileGuestStateMuV8542.Lock()
	defer gofileGuestStateMuV8542.Unlock()
	state := loadGofileGuestStateUnlockedV8542()
	token := strings.TrimSpace(state.Token)
	return token, token != ""
}

func persistGofileGuestTokenV8542(token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return errors.New("token GoFile gol")
	}
	gofileGuestStateMuV8542.Lock()
	defer gofileGuestStateMuV8542.Unlock()
	state := loadGofileGuestStateUnlockedV8542()
	state.Token = token
	state.CreatedAt = time.Now().Unix()
	state.CooldownUntil = 0
	return saveGofileGuestStateUnlockedV8542(state)
}

func clearCachedGofileGuestTokenV8542() {
	gofileGuestStateMuV8542.Lock()
	defer gofileGuestStateMuV8542.Unlock()
	state := loadGofileGuestStateUnlockedV8542()
	state.Token = ""
	state.CreatedAt = 0
	_ = saveGofileGuestStateUnlockedV8542(state)
}

func gofileConfiguredTokenV8542() string {
	for _, key := range []string{"GOFILE_TOKEN", "GOFILE_API_TOKEN", "GF_TOKEN"} {
		if token := strings.TrimSpace(os.Getenv(key)); token != "" {
			return token
		}
	}
	return ""
}

func setGofileGuestCooldownV8542(delay time.Duration) {
	if delay <= 0 {
		return
	}
	gofileGuestStateMuV8542.Lock()
	defer gofileGuestStateMuV8542.Unlock()
	state := loadGofileGuestStateUnlockedV8542()
	until := time.Now().Add(delay).Unix()
	if until > state.CooldownUntil {
		state.CooldownUntil = until
		_ = saveGofileGuestStateUnlockedV8542(state)
	}
}

func clearGofileGuestCooldownV8542() {
	gofileGuestStateMuV8542.Lock()
	defer gofileGuestStateMuV8542.Unlock()
	state := loadGofileGuestStateUnlockedV8542()
	if state.CooldownUntil != 0 {
		state.CooldownUntil = 0
		_ = saveGofileGuestStateUnlockedV8542(state)
	}
}

func gofileGuestCooldownRemainingV8542() time.Duration {
	gofileGuestStateMuV8542.Lock()
	defer gofileGuestStateMuV8542.Unlock()
	state := loadGofileGuestStateUnlockedV8542()
	if state.CooldownUntil <= 0 {
		return 0
	}
	remaining := time.Until(time.Unix(state.CooldownUntil, 0))
	if remaining <= 0 {
		state.CooldownUntil = 0
		_ = saveGofileGuestStateUnlockedV8542(state)
		return 0
	}
	return remaining
}

func rememberWorkingGofileSaltV8542(salt string) {
	salt = strings.TrimSpace(salt)
	if salt == "" {
		return
	}
	gofileGuestStateMuV8542.Lock()
	defer gofileGuestStateMuV8542.Unlock()
	state := loadGofileGuestStateUnlockedV8542()
	if state.WorkingSalt == salt {
		return
	}
	state.WorkingSalt = salt
	_ = saveGofileGuestStateUnlockedV8542(state)
}

func cachedWorkingGofileSaltV8542() string {
	gofileGuestStateMuV8542.Lock()
	defer gofileGuestStateMuV8542.Unlock()
	return strings.TrimSpace(loadGofileGuestStateUnlockedV8542().WorkingSalt)
}

func gofileWebsiteSaltCandidatesV8542() []string {
	out := make([]string, 0, 4)
	seen := map[string]bool{}
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			return
		}
		seen[s] = true
		out = append(out, s)
	}
	add(os.Getenv("GOFILE_WT_SALT"))
	add(cachedWorkingGofileSaltV8542())
	add(gofileCurrentWebsiteSaltV8542)
	// Known historical values are fallback candidates only. Reusing the same
	// account token with another website-token candidate is cheap and avoids
	// creating throw-away guest accounts when GoFile rotates its web token.
	add("5d4f7g8sd45fsd")
	add("9844d94d963d30")
	return out
}

func gofileWebsiteTokenForSaltV8542(accountToken, salt string, now time.Time) string {
	bucket := now.Unix() / 14400
	raw := gofileUserAgentV86 + "::en-US::" + accountToken + "::" + strconv.FormatInt(bucket, 10) + "::" + salt
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func gofileAPIErrorInfoV8542(err error) (int, string, bool) {
	var apiErr *gofileAPIErrorV8542
	if !errors.As(err, &apiErr) {
		return 0, "", false
	}
	return apiErr.HTTPStatus, strings.ToLower(strings.TrimSpace(apiErr.APIStatus)), true
}

func gofileAccountTokenRejectedV8542(err error) bool {
	_, status, ok := gofileAPIErrorInfoV8542(err)
	if !ok {
		return false
	}
	switch status {
	case "error-wrongtoken", "error-notauthenticated", "error-badtoken", "error-invalidtoken":
		return true
	default:
		return false
	}
}

func gofileWebsiteTokenCandidateErrorV8542(err error) bool {
	httpStatus, status, ok := gofileAPIErrorInfoV8542(err)
	if !ok {
		return false
	}
	if gofileAccountTokenRejectedV8542(err) {
		return false
	}
	if httpStatus == http.StatusUnauthorized || httpStatus == http.StatusForbidden {
		return true
	}
	// GoFile can mask a rejected/rotated website token as HTTP 200
	// error-notFound. Treat it as ambiguous while other known salts remain;
	// fetchGofileContentV86 still returns notFound after all candidates fail.
	switch status {
	case "error-notpremium", "error-websitetoken", "error-wrongwebsitetoken", "error-notfound":
		return true
	default:
		return false
	}
}

func gofileRateLimitedV8542(err error) bool {
	httpStatus, status, ok := gofileAPIErrorInfoV8542(err)
	if ok && (httpStatus == http.StatusTooManyRequests || status == "error-ratelimit") {
		return true
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "guest account rate-limit") || strings.Contains(text, "guest account în cooldown") || strings.Contains(text, "guest account in cooldown")
}

func gofileGuestCooldownDelayV8542(resp *http.Response) time.Duration {
	delay := 5 * time.Minute
	if resp != nil {
		raw := strings.TrimSpace(resp.Header.Get("Retry-After"))
		if seconds, err := strconv.Atoi(raw); err == nil && seconds > 0 {
			delay = time.Duration(seconds) * time.Second
		} else if when, err := http.ParseTime(raw); err == nil {
			if d := time.Until(when); d > 0 {
				delay = d
			}
		}
	}
	if delay < 30*time.Second {
		delay = 30 * time.Second
	}
	if delay > 30*time.Minute {
		delay = 30 * time.Minute
	}
	return delay
}

func invalidatePersistentGofileGuestTokenV8542() {
	invalidateGoFileGuestTokenV86()
	clearCachedGofileGuestTokenV8542()
}
