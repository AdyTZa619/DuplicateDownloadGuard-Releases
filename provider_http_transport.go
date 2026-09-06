package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// providerHTTPContextV86 keeps short-lived host authentication/header state
// outside RemoteItem. It must never be serialized into results, queue history,
// diagnostics, or logs.
type providerHTTPContextV86 struct {
	Headers   http.Header
	SourceURL string
	ExpiresAt time.Time
}

type providerAwareTransportV86 struct {
	base http.RoundTripper
}

var (
	providerContextMuV86 sync.RWMutex
	providerContextV86   = map[string]providerHTTPContextV86{}

	providerResolveMuV86 sync.Mutex
	providerSourceAtV86  = map[string]time.Time{}

	gofileTokenMuV86 sync.Mutex
	gofileTokenV86   string
	gofileTokenAtV86 time.Time
)

func init() {
	base := http.DefaultTransport
	if _, exists := base.(*providerAwareTransportV86); exists {
		return
	}
	http.DefaultTransport = &providerAwareTransportV86{base: base}
}

func (t *providerAwareTransportV86) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil || req.URL == nil {
		return t.base.RoundTrip(req)
	}
	host := strings.ToLower(req.URL.Hostname())
	if !providerSpecialHostV86(host) {
		return t.base.RoundTrip(req)
	}

	clone := req.Clone(req.Context())
	clone.Header = req.Header.Clone()

	// GoFile download servers require the guest account cookie. Keep it only in
	// RAM and mint it through GoFile's public guest-account endpoint when needed.
	if isGofileDownloadHostV86(host) && clone.Header.Get("Cookie") == "" {
		if token, err := gofileGuestTokenV86(clone.Context(), t.base); err == nil && token != "" {
			clone.Header.Set("Cookie", "accountToken="+token)
			if clone.Header.Get("Referer") == "" {
				clone.Header.Set("Referer", "https://gofile.io/")
			}
		}
	}

	if ctx, ok := providerContextForURLV86(clone.URL.String()); ok {
		applyProviderHeadersV86(clone.Header, ctx.Headers)
	} else if source := strings.TrimSpace(clone.Header.Get("Referer")); source != "" && providerSourceNeedsResolutionV86(source) {
		// Bunkr, Cyberdrop and similar extractors can expose a special Referer,
		// Origin, cookie, or temporary URL through gallery-dl private metadata.
		// Resolve once per source and keep the result in memory only.
		_ = ensureGalleryProviderContextV86(clone.Context(), source)
		if ctx, ok := providerContextForURLV86(clone.URL.String()); ok {
			applyProviderHeadersV86(clone.Header, ctx.Headers)
		}
	}
	return t.base.RoundTrip(clone)
}

func providerSpecialHostV86(host string) bool {
	h := strings.ToLower(strings.TrimSpace(host))
	if h == "" {
		return false
	}
	return strings.HasSuffix(h, ".gofile.io") || h == "gofile.io" || h == "api.gofile.io" ||
		strings.Contains(h, "bunkr") || strings.Contains(h, "bunkrr") ||
		strings.Contains(h, "cyberdrop")
}

func isGofileDownloadHostV86(host string) bool {
	h := strings.ToLower(strings.TrimSpace(host))
	return strings.HasSuffix(h, ".gofile.io") && h != "api.gofile.io" && h != "gofile.io" && h != "www.gofile.io"
}

func providerSourceNeedsResolutionV86(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	h := strings.ToLower(u.Hostname())
	return strings.Contains(h, "bunkr") || strings.Contains(h, "bunkrr") || strings.Contains(h, "cyberdrop")
}

func providerContextKeyV86(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return strings.TrimSpace(raw)
	}
	u.Fragment = ""
	return u.String()
}

func providerContextForURLV86(raw string) (providerHTTPContextV86, bool) {
	key := providerContextKeyV86(raw)
	providerContextMuV86.RLock()
	ctx, ok := providerContextV86[key]
	providerContextMuV86.RUnlock()
	if !ok {
		return providerHTTPContextV86{}, false
	}
	if !ctx.ExpiresAt.IsZero() && time.Now().After(ctx.ExpiresAt) {
		providerContextMuV86.Lock()
		delete(providerContextV86, key)
		providerContextMuV86.Unlock()
		return providerHTTPContextV86{}, false
	}
	ctx.Headers = ctx.Headers.Clone()
	return ctx, true
}

func rememberProviderContextV86(directURL, sourceURL string, headers http.Header, ttl time.Duration) {
	if strings.TrimSpace(directURL) == "" || len(headers) == 0 {
		return
	}
	if ttl <= 0 {
		ttl = 20 * time.Minute
	}
	providerContextMuV86.Lock()
	providerContextV86[providerContextKeyV86(directURL)] = providerHTTPContextV86{
		Headers:   sanitizeProviderHeadersV86(headers),
		SourceURL: sourceURL,
		ExpiresAt: time.Now().Add(ttl),
	}
	providerContextMuV86.Unlock()
}

func sanitizeProviderHeadersV86(in http.Header) http.Header {
	out := make(http.Header)
	for key, vals := range in {
		canonical := http.CanonicalHeaderKey(strings.TrimSpace(key))
		switch strings.ToLower(canonical) {
		case "", "host", "content-length", "transfer-encoding", "connection", "range":
			continue
		}
		for _, val := range vals {
			if v := strings.TrimSpace(val); v != "" {
				out.Add(canonical, v)
			}
		}
	}
	return out
}

func applyProviderHeadersV86(dst, src http.Header) {
	for key, vals := range sanitizeProviderHeadersV86(src) {
		// Provider metadata is authoritative for host-required Referer/Origin and
		// cookies. User Range remains untouched because it is filtered above.
		dst.Del(key)
		for _, val := range vals {
			dst.Add(key, val)
		}
	}
}

func gofileGuestRetryableStatusV86(status int) bool {
	switch status {
	case http.StatusRequestTimeout, http.StatusTooEarly, http.StatusTooManyRequests,
		http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func gofileGuestRetryDelayV86(attempt int, resp *http.Response) time.Duration {
	if resp != nil && resp.StatusCode == http.StatusTooManyRequests {
		return gofileRetryAfterV86(resp)
	}
	delay := time.Duration(attempt+1) * 500 * time.Millisecond
	if delay > 2*time.Second {
		delay = 2 * time.Second
	}
	return delay
}

func gofileGuestTokenV86(ctx context.Context, base http.RoundTripper) (string, error) {
	gofileTokenMuV86.Lock()
	defer gofileTokenMuV86.Unlock()

	// Reuse one account token for the whole process. Unlike the old 3-hour
	// timer, validity is decided by GoFile itself when metadata is requested.
	if token := strings.TrimSpace(gofileTokenV86); token != "" {
		return token, nil
	}
	if configured := gofileConfiguredTokenV8542(); configured != "" {
		gofileTokenV86 = configured
		gofileTokenAtV86 = time.Now()
		return configured, nil
	}
	if cached, ok := loadCachedGofileGuestTokenV8542(); ok {
		gofileTokenV86 = cached
		gofileTokenAtV86 = time.Now()
		return cached, nil
	}
	if remaining := gofileGuestCooldownRemainingV8542(); remaining > 0 {
		return "", fmt.Errorf("GoFile guest account în cooldown încă %s după rate-limit", remaining.Round(time.Second))
	}

	if base == nil {
		if wrapped, ok := http.DefaultTransport.(*providerAwareTransportV86); ok && wrapped.base != nil {
			base = wrapped.base
		} else {
			base = http.DefaultTransport
		}
	}

	const maxAttempts = 3
	const attemptTimeout = 15 * time.Second
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		attemptCtx, cancel := context.WithTimeout(ctx, attemptTimeout)
		req, err := http.NewRequestWithContext(attemptCtx, http.MethodPost, "https://api.gofile.io/accounts", bytes.NewBufferString("{}"))
		if err != nil {
			cancel()
			return "", err
		}
		req.Header.Set("Origin", "https://gofile.io")
		req.Header.Set("Referer", "https://gofile.io/")
		req.Header.Set("User-Agent", gofileUserAgentV86)
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-BL", "en-US")
		req.Header.Set("X-Website-Token", gofileWebsiteTokenV86("", time.Now()))

		client := &http.Client{Transport: base}
		resp, err := client.Do(req)
		if err != nil {
			cancel()
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			lastErr = fmt.Errorf("încercarea %d/%d: %w", attempt+1, maxAttempts, err)
			if attempt+1 < maxAttempts {
				if sleepErr := gofileSleepV86(ctx, time.Duration(attempt+1)*time.Second); sleepErr != nil {
					return "", sleepErr
				}
				continue
			}
			break
		}

		status := resp.StatusCode
		var env struct {
			Status string `json:"status"`
			Data   struct {
				Token string `json:"token"`
			} `json:"data"`
		}
		decodeErr := json.NewDecoder(resp.Body).Decode(&env)
		rateLimited := status == http.StatusTooManyRequests || strings.EqualFold(strings.TrimSpace(env.Status), "error-rateLimit")
		cooldown := time.Duration(0)
		if rateLimited {
			cooldown = gofileGuestCooldownDelayV8542(resp)
		}
		_ = resp.Body.Close()
		cancel()

		if rateLimited {
			setGofileGuestCooldownV8542(cooldown)
			return "", fmt.Errorf("GoFile guest account rate-limit; nu mai creez alte conturi timp de %s", cooldown.Round(time.Second))
		}
		if status >= 200 && status < 300 && decodeErr == nil && strings.EqualFold(strings.TrimSpace(env.Status), "ok") && strings.TrimSpace(env.Data.Token) != "" {
			token := strings.TrimSpace(env.Data.Token)
			gofileTokenV86 = token
			gofileTokenAtV86 = time.Now()
			clearGofileGuestCooldownV8542()
			_ = persistGofileGuestTokenV8542(token)
			return token, nil
		}
		if status == http.StatusRequestTimeout || status == http.StatusTooEarly || status >= 500 {
			lastErr = fmt.Errorf("GoFile guest account HTTP %d", status)
			if attempt+1 < maxAttempts {
				if sleepErr := gofileSleepV86(ctx, time.Duration(attempt+1)*time.Second); sleepErr != nil {
					return "", sleepErr
				}
				continue
			}
			break
		}
		if decodeErr != nil {
			return "", fmt.Errorf("răspuns GoFile guest invalid: %w", decodeErr)
		}
		return "", fmt.Errorf("GoFile guest account HTTP %d (%s)", status, strings.TrimSpace(env.Status))
	}
	if lastErr == nil {
		lastErr = errors.New("eroare necunoscută")
	}
	return "", fmt.Errorf("GoFile guest account indisponibil după %d încercări: %w", maxAttempts, lastErr)
}

func detectGalleryDLForProviderV86() string {
	var candidates []string
	if runtime.GOOS == "windows" {
		candidates = []string{
			filepath.Join(portableToolsDir(), "gallery-dl", "gallery-dl.exe"),
			filepath.Join(portableToolsDir(), "gallery-dl.exe"),
			filepath.Join(os.Getenv("USERPROFILE"), "bin", "gallery-dl.exe"),
			`C:\gallery-dl\gallery-dl.exe`,
		}
	}
	return firstExisting("", []string{"gallery-dl.exe", "gallery-dl"}, candidates)
}

type netscapeCookieV86 struct {
	Domain, Path, Name, Value string
	IncludeSubdomains, Secure bool
	Expires                   int64
}

func parseNetscapeCookiesV86(data []byte) []netscapeCookieV86 {
	out := []netscapeCookieV86{}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "#HttpOnly_") {
			line = strings.TrimPrefix(line, "#HttpOnly_")
		} else if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 7 {
			continue
		}
		expires, _ := strconv.ParseInt(parts[4], 10, 64)
		out = append(out, netscapeCookieV86{
			Domain:            strings.TrimPrefix(strings.ToLower(strings.TrimSpace(parts[0])), "."),
			IncludeSubdomains: strings.EqualFold(strings.TrimSpace(parts[1]), "TRUE"),
			Path:              strings.TrimSpace(parts[2]),
			Secure:            strings.EqualFold(strings.TrimSpace(parts[3]), "TRUE"),
			Expires:           expires,
			Name:              strings.TrimSpace(parts[5]),
			Value:             strings.TrimSpace(parts[6]),
		})
	}
	return out
}

func cookiesForURLV86(raw string, cookies []netscapeCookieV86) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	path := u.EscapedPath()
	if path == "" {
		path = "/"
	}
	now := time.Now().Unix()
	pairs := []string{}
	for _, c := range cookies {
		if c.Name == "" || c.Domain == "" || (c.Expires > 0 && c.Expires < now) {
			continue
		}
		domainOK := host == c.Domain || (c.IncludeSubdomains && strings.HasSuffix(host, "."+c.Domain))
		if !domainOK || (c.Secure && u.Scheme != "https") {
			continue
		}
		cp := c.Path
		if cp == "" {
			cp = "/"
		}
		if !strings.HasPrefix(path, cp) {
			continue
		}
		pairs = append(pairs, c.Name+"="+c.Value)
	}
	return strings.Join(pairs, "; ")
}

func ensureGalleryProviderContextV86(parent context.Context, sourceURL string) error {
	sourceURL = strings.TrimSpace(sourceURL)
	if sourceURL == "" {
		return errors.New("sursă goală")
	}
	providerResolveMuV86.Lock()
	defer providerResolveMuV86.Unlock()
	if at := providerSourceAtV86[sourceURL]; !at.IsZero() && time.Since(at) < 10*time.Minute {
		return nil
	}
	exe := detectGalleryDLForProviderV86()
	if exe == "" {
		return errors.New("gallery-dl lipsește pentru contextul HTTP al providerului")
	}

	ctx, cancel := context.WithTimeout(parent, 60*time.Second)
	defer cancel()
	cookieFile, err := os.CreateTemp("", "ddg-gallery-cookies-*.txt")
	if err != nil {
		return err
	}
	cookiePath := cookieFile.Name()
	_ = cookieFile.Close()
	defer os.Remove(cookiePath)

	// -J resolves intermediary URLs. JSONL is explicitly enabled so each URL
	// message can be parsed incrementally even for albums with thousands of files.
	// output.private exposes extractor-provided _http_headers such as Bunkr's
	// required per-file Referer. Cookies are exported to a temporary file only.
	args := []string{"-J", "--no-colors", "-o", "output.private=true", "-o", "output.jsonl=true", "--cookies-export", cookiePath, sourceURL}
	cmd := exec.CommandContext(ctx, exe, args...)
	hideChildWindow(cmd)
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("gallery-dl context: %w", err)
	}
	cookieData, _ := os.ReadFile(cookiePath)
	cookies := parseNetscapeCookiesV86(cookieData)
	resolved := parseGalleryResolvedContextV86(output, sourceURL, cookies)
	if resolved == 0 {
		return errors.New("gallery-dl nu a furnizat context HTTP utilizabil")
	}
	providerSourceAtV86[sourceURL] = time.Now()
	return nil
}

func parseGalleryResolvedContextV86(output []byte, sourceURL string, cookies []netscapeCookieV86) int {
	count := 0
	scanner := bufio.NewScanner(bytes.NewReader(output))
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, 16*1024*1024)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 || line[0] != '[' {
			continue
		}
		dec := json.NewDecoder(bytes.NewReader(line))
		dec.UseNumber()
		var msg []any
		if err := dec.Decode(&msg); err != nil || len(msg) < 3 {
			continue
		}
		direct, ok := msg[1].(string)
		if !ok || (!strings.HasPrefix(direct, "http://") && !strings.HasPrefix(direct, "https://")) {
			continue
		}
		meta, _ := msg[2].(map[string]any)
		headers := make(http.Header)
		if raw, ok := meta["_http_headers"].(map[string]any); ok {
			for key, val := range raw {
				switch x := val.(type) {
				case string:
					headers.Set(key, x)
				case []any:
					for _, item := range x {
						if s, ok := item.(string); ok {
							headers.Add(key, s)
						}
					}
				}
			}
		}
		if cookie := cookiesForURLV86(direct, cookies); cookie != "" {
			headers.Set("Cookie", cookie)
		}
		if len(headers) == 0 {
			continue
		}
		rememberProviderContextV86(direct, sourceURL, headers, 20*time.Minute)
		count++
	}
	return count
}
