package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const gofileWebsiteSaltDefaultV86 = gofileCurrentWebsiteSaltV8542
const gofileUserAgentV86 = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"
const gofileAPIMinIntervalV86 = 500 * time.Millisecond
const gofileAPIMax429RetriesV86 = 5

var (
	gofileAPIRateMuV86 sync.Mutex
	gofileAPINextAtV86 time.Time
)

type gofileContentV86 struct {
	ID       string                      `json:"id"`
	Type     string                      `json:"type"`
	Name     string                      `json:"name"`
	Size     int64                       `json:"size"`
	Link     string                      `json:"link"`
	MD5      string                      `json:"md5"`
	MimeType string                      `json:"mimetype"`
	Children map[string]gofileContentV86 `json:"children"`
}

type gofileContentEnvelopeV86 struct {
	Status string           `json:"status"`
	Data   gofileContentV86 `json:"data"`
}

func gofileFolderCodeV86(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", errors.New("link GoFile invalid")
	}
	host := strings.ToLower(u.Hostname())
	if host != "gofile.io" && !strings.HasSuffix(host, ".gofile.io") {
		return "", errors.New("linkul nu este GoFile")
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 || !strings.EqualFold(parts[0], "d") || strings.TrimSpace(parts[1]) == "" {
		return "", errors.New("cod folder GoFile lipsă")
	}
	return strings.TrimSpace(parts[1]), nil
}

func gofileWebsiteSaltV86() string {
	if override := strings.TrimSpace(os.Getenv("GOFILE_WT_SALT")); override != "" {
		return override
	}
	return gofileWebsiteSaltDefaultV86
}

func gofileWebsiteTokenV86(accountToken string, now time.Time) string {
	bucket := now.Unix() / 14400
	raw := gofileUserAgentV86 + "::en-US::" + accountToken + "::" + strconv.FormatInt(bucket, 10) + "::" + gofileWebsiteSaltV86()
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func gofileAPIClientV86() *http.Client {
	return &http.Client{Timeout: 20 * time.Second}
}

func waitGofileAPISlotV86(ctx context.Context) error {
	gofileAPIRateMuV86.Lock()
	at := time.Now()
	if gofileAPINextAtV86.After(at) {
		at = gofileAPINextAtV86
	}
	gofileAPINextAtV86 = at.Add(gofileAPIMinIntervalV86)
	gofileAPIRateMuV86.Unlock()

	delay := time.Until(at)
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func gofileSleepV86(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func gofileRetryAfterV86(resp *http.Response) time.Duration {
	const fallback = 5 * time.Second
	const maxDelay = 30 * time.Second
	if resp == nil {
		return fallback
	}
	raw := strings.TrimSpace(resp.Header.Get("Retry-After"))
	if raw == "" {
		return fallback
	}
	if seconds, err := strconv.Atoi(raw); err == nil {
		if seconds < 0 {
			seconds = 0
		}
		delay := time.Duration(seconds) * time.Second
		if delay > maxDelay {
			return maxDelay
		}
		return delay
	}
	if when, err := http.ParseTime(raw); err == nil {
		delay := time.Until(when)
		if delay < 0 {
			return 0
		}
		if delay > maxDelay {
			return maxDelay
		}
		return delay
	}
	return fallback
}

func fetchGofileContentOneSaltV8542(ctx context.Context, client *http.Client, token, contentID, salt string) (gofileContentV86, error) {
	endpoint := "https://api.gofile.io/contents/" + url.PathEscape(contentID)
	q := url.Values{}
	q.Set("contentFilter", "")
	q.Set("page", "1")
	q.Set("pageSize", "1000")
	q.Set("sortField", "createTime")
	q.Set("sortDirection", "-1")
	endpoint += "?" + q.Encode()

	for attempt := 0; attempt <= gofileAPIMax429RetriesV86; attempt++ {
		if err := waitGofileAPISlotV86(ctx); err != nil {
			return gofileContentV86{}, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return gofileContentV86{}, err
		}
		req.Header.Set("User-Agent", gofileUserAgentV86)
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-Website-Token", gofileWebsiteTokenForSaltV8542(token, salt, time.Now()))
		req.Header.Set("X-BL", "en-US")
		req.Header.Set("Origin", "https://gofile.io")
		req.Header.Set("Referer", "https://gofile.io/")

		resp, err := client.Do(req)
		if err != nil {
			return gofileContentV86{}, fmt.Errorf("GoFile API indisponibil: %w", err)
		}
		delay := time.Duration(0)
		if resp.StatusCode == http.StatusTooManyRequests {
			delay = gofileRetryAfterV86(resp)
		}
		var env gofileContentEnvelopeV86
		decodeErr := json.NewDecoder(resp.Body).Decode(&env)
		_ = resp.Body.Close()

		apiStatus := strings.TrimSpace(env.Status)
		if resp.StatusCode == http.StatusTooManyRequests || strings.EqualFold(apiStatus, "error-rateLimit") {
			if attempt >= gofileAPIMax429RetriesV86 {
				return gofileContentV86{}, &gofileAPIErrorV8542{HTTPStatus: http.StatusTooManyRequests, APIStatus: apiStatus}
			}
			if delay <= 0 {
				delay = 5 * time.Second
			}
			if err := gofileSleepV86(ctx, delay); err != nil {
				return gofileContentV86{}, err
			}
			continue
		}

		if resp.StatusCode >= 200 && resp.StatusCode < 300 && decodeErr == nil && strings.EqualFold(apiStatus, "ok") {
			return env.Data, nil
		}
		if resp.StatusCode >= 200 && resp.StatusCode < 300 && decodeErr != nil {
			return gofileContentV86{}, fmt.Errorf("răspuns GoFile invalid: %w", decodeErr)
		}
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return gofileContentV86{}, &gofileAPIErrorV8542{HTTPStatus: resp.StatusCode, APIStatus: apiStatus}
		}
		return gofileContentV86{}, &gofileAPIErrorV8542{HTTPStatus: resp.StatusCode, APIStatus: apiStatus}
	}
	return gofileContentV86{}, errors.New("GoFile API: limită de reîncercări depășită")
}

func fetchGofileContentV86(ctx context.Context, client *http.Client, token, contentID string) (gofileContentV86, error) {
	candidates := gofileWebsiteSaltCandidatesV8542()
	var lastErr error
	for i, salt := range candidates {
		data, err := fetchGofileContentOneSaltV8542(ctx, client, token, contentID, salt)
		if err == nil {
			rememberWorkingGofileSaltV8542(salt)
			return data, nil
		}
		lastErr = err
		if gofileAccountTokenRejectedV8542(err) || !gofileWebsiteTokenCandidateErrorV8542(err) || i == len(candidates)-1 {
			break
		}
	}
	if lastErr == nil {
		lastErr = errors.New("GoFile nu are niciun website-token candidat")
	}
	return gofileContentV86{}, lastErr
}

func gofileAppendFileV86(items *[]RemoteItem, sourceURL, folderPath, token string, c gofileContentV86) {
	name := strings.TrimSpace(c.Name)
	if name == "" {
		name = strings.TrimSpace(c.ID)
	}
	if name == "" || strings.TrimSpace(c.Link) == "" {
		return
	}
	path := name
	if strings.TrimSpace(folderPath) != "" {
		path = filepath.ToSlash(filepath.Join(folderPath, name))
	}
	item := RemoteItem{
		Path:        path,
		Name:        name,
		Size:        c.Size,
		Source:      "GOFILE",
		URL:         sourceURL,
		DirectURL:   strings.TrimSpace(c.Link),
		Extractor:   "gofile-native",
		ProviderID:  strings.TrimSpace(c.ID),
		ContentType: strings.TrimSpace(c.MimeType),
	}
	if validHex(strings.TrimSpace(c.MD5), 32) {
		item.HashType = "md5"
		item.Hash = strings.ToLower(strings.TrimSpace(c.MD5))
	}
	if item.Size <= 0 {
		item.Size = -1
	}
	if item.DirectURL != "" {
		headers := make(http.Header)
		headers.Set("Cookie", "accountToken="+token)
		headers.Set("Referer", "https://gofile.io/")
		rememberProviderContextV86(item.DirectURL, sourceURL, headers, 3*time.Hour)
	}
	*items = append(*items, item)
}

func walkGofileContentV86(ctx context.Context, client *http.Client, token, sourceURL, folderPath string, c gofileContentV86, visited map[string]bool, items *[]RemoteItem) error {
	kind := strings.ToLower(strings.TrimSpace(c.Type))
	if kind == "file" {
		gofileAppendFileV86(items, sourceURL, folderPath, token, c)
		return nil
	}
	if kind != "folder" && len(c.Children) == 0 {
		return nil
	}

	currentPath := folderPath
	if name := strings.TrimSpace(c.Name); name != "" && folderPath != "" {
		currentPath = filepath.ToSlash(filepath.Join(folderPath, name))
	}
	for _, child := range c.Children {
		if strings.EqualFold(strings.TrimSpace(child.Type), "folder") && len(child.Children) == 0 && strings.TrimSpace(child.ID) != "" {
			id := strings.TrimSpace(child.ID)
			if visited[id] {
				continue
			}
			visited[id] = true
			fresh, err := fetchGofileContentV86(ctx, client, token, id)
			if err != nil {
				return err
			}
			if strings.TrimSpace(fresh.Name) == "" {
				fresh.Name = child.Name
			}
			if err := walkGofileContentV86(ctx, client, token, sourceURL, currentPath, fresh, visited, items); err != nil {
				return err
			}
			continue
		}
		if err := walkGofileContentV86(ctx, client, token, sourceURL, currentPath, child, visited, items); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) probeGofileNativeV86(ctx context.Context, sourceURL string) ([]RemoteItem, error) {
	code, err := gofileFolderCodeV86(sourceURL)
	if err != nil {
		return nil, err
	}
	a.logf("GoFile: pornesc listarea nativă pentru folderul %s", code)
	token, err := gofileGuestTokenV86(ctx, nil)
	if err != nil {
		a.logf("GoFile: token guest eșuat: %v", err)
		return nil, err
	}
	client := gofileAPIClientV86()
	root, err := fetchGofileContentV86(ctx, client, token, code)
	if err != nil && gofileAccountTokenRejectedV8542(err) && gofileConfiguredTokenV8542() == "" {
		a.logf("GoFile: API a confirmat token guest invalid; șterg cache-ul și îl refac o singură dată")
		invalidatePersistentGofileGuestTokenV8542()
		if freshToken, tokenErr := gofileGuestTokenV86(ctx, nil); tokenErr == nil {
			token = freshToken
			root, err = fetchGofileContentV86(ctx, client, token, code)
		} else {
			err = tokenErr
		}
	}
	if err != nil {
		a.logf("GoFile: citire metadata eșuată: %v", err)
		return nil, err
	}
	items := make([]RemoteItem, 0, len(root.Children))
	visited := map[string]bool{code: true}
	if err := walkGofileContentV86(ctx, client, token, sourceURL, "", root, visited, &items); err != nil {
		a.logf("GoFile: parcurgere folder eșuată: %v", err)
		return nil, err
	}
	if len(items) == 0 {
		a.logf("GoFile: API a răspuns, dar folderul nu conține fișiere listabile")
		return nil, errors.New("GoFile a răspuns, dar nu a returnat fișiere")
	}
	for i := range items {
		items[i].ID = i + 1
	}
	a.logf("GoFile: listare nativă reușită, %d fișiere", len(items))
	return items, nil
}
