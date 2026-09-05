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
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const gofileWebsiteSaltV86 = "5d4f7g8sd45fsd"
const gofileUserAgentV86 = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) DuplicateDownloadGuard"

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

func gofileWebsiteTokenV86(accountToken string, now time.Time) string {
	bucket := now.Unix() / 14400
	raw := gofileUserAgentV86 + "::en-US::" + accountToken + "::" + strconv.FormatInt(bucket, 10) + "::" + gofileWebsiteSaltV86
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func gofileAPIClientV86() *http.Client {
	return &http.Client{Timeout: 20 * time.Second}
}

func fetchGofileContentV86(ctx context.Context, client *http.Client, token, contentID string) (gofileContentV86, error) {
	endpoint := "https://api.gofile.io/contents/" + url.PathEscape(contentID)
	q := url.Values{}
	q.Set("contentFilter", "")
	q.Set("page", "1")
	q.Set("pageSize", "1000")
	q.Set("sortField", "name")
	q.Set("sortDirection", "1")
	endpoint += "?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return gofileContentV86{}, err
	}
	req.Header.Set("User-Agent", gofileUserAgentV86)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Website-Token", gofileWebsiteTokenV86(token, time.Now()))
	req.Header.Set("X-BL", "en-US")
	req.Header.Set("Origin", "https://gofile.io")
	req.Header.Set("Referer", "https://gofile.io/")

	resp, err := client.Do(req)
	if err != nil {
		return gofileContentV86{}, fmt.Errorf("GoFile API indisponibil: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return gofileContentV86{}, fmt.Errorf("GoFile API HTTP %d", resp.StatusCode)
	}
	var env gofileContentEnvelopeV86
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return gofileContentV86{}, fmt.Errorf("răspuns GoFile invalid: %w", err)
	}
	if !strings.EqualFold(strings.TrimSpace(env.Status), "ok") {
		status := strings.TrimSpace(env.Status)
		if status == "" {
			status = "necunoscut"
		}
		return gofileContentV86{}, fmt.Errorf("GoFile API status %s", status)
	}
	return env.Data, nil
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
