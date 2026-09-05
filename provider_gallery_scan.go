package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type galleryRemoteEntryV86 struct {
	Item RemoteItem
}

func providerSourceLabelV86(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "GALLERY-DL"
	}
	h := strings.ToLower(u.Hostname())
	switch {
	case h == "gofile.io" || strings.HasSuffix(h, ".gofile.io"):
		return "GOFILE"
	case hostHasPrefixLabelV86(h, "bunkr"):
		return "BUNKR"
	case hostHasPrefixLabelV86(h, "cyberdrop"):
		return "CYBERDROP"
	default:
		return "GALLERY-DL"
	}
}

func hostHasPrefixLabelV86(host, prefix string) bool {
	for _, label := range strings.Split(strings.ToLower(host), ".") {
		if strings.HasPrefix(label, strings.ToLower(prefix)) {
			return true
		}
	}
	return false
}

func galleryStringV86(meta map[string]any, keys ...string) string {
	for _, key := range keys {
		if s, ok := meta[key].(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func galleryInt64V86(meta map[string]any, keys ...string) int64 {
	for _, key := range keys {
		switch v := meta[key].(type) {
		case json.Number:
			if n, err := v.Int64(); err == nil && n > 0 {
				return n
			}
		case float64:
			if v > 0 {
				return int64(v)
			}
		case int64:
			if v > 0 {
				return v
			}
		case string:
			if n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil && n > 0 {
				return n
			}
		}
	}
	return -1
}

func galleryRemoteNameV86(meta map[string]any, direct string) string {
	name := galleryStringV86(meta, "name", "original", "filename")
	ext := strings.TrimPrefix(galleryStringV86(meta, "extension", "ext"), ".")
	if name != "" && ext != "" && filepath.Ext(name) == "" {
		name += "." + ext
	}
	if name == "" {
		if u, err := url.Parse(direct); err == nil {
			name = filepath.Base(u.Path)
			if unescaped, err := url.PathUnescape(name); err == nil {
				name = unescaped
			}
		}
	}
	if strings.TrimSpace(name) == "" || name == "." || name == "/" {
		name = "remote-file"
	}
	return name
}

func galleryRemotePathV86(meta map[string]any, name string) string {
	album := galleryStringV86(meta, "album_name", "album", "title")
	if album == "" || strings.EqualFold(album, name) {
		return name
	}
	return filepath.ToSlash(filepath.Join(album, name))
}

func parseGalleryRemoteItemsV86(output []byte, sourceURL string) []RemoteItem {
	source := providerSourceLabelV86(sourceURL)
	items := []RemoteItem{}
	seen := map[string]bool{}
	scanner := bufio.NewScanner(bytes.NewReader(output))
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
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
		name := galleryRemoteNameV86(meta, direct)
		providerID := galleryStringV86(meta, "id", "id_url", "uuid", "slug", "file_id")
		key := providerContextKeyV86(direct) + "|" + strings.ToLower(name)
		if seen[key] {
			continue
		}
		seen[key] = true
		items = append(items, RemoteItem{
			Path:       galleryRemotePathV86(meta, name),
			Name:       name,
			Size:       galleryInt64V86(meta, "size", "filesize", "file_size", "content_size"),
			Source:     source,
			URL:        sourceURL,
			DirectURL:  direct,
			Extractor:  strings.ToLower(source),
			ProviderID: providerID,
		})
	}
	for i := range items {
		items[i].ID = i + 1
	}
	return items
}

func mergeGalleryProbeV86(item RemoteItem, probe RemoteItem) RemoteItem {
	if item.Size <= 0 && probe.Size > 0 {
		item.Size = probe.Size
	}
	if probe.Hash != "" {
		item.Hash = probe.Hash
		item.HashType = probe.HashType
	}
	if probe.ContentType != "" {
		item.ContentType = probe.ContentType
	}
	if probe.ETag != "" {
		item.ETag = probe.ETag
	}
	item.AcceptRanges = probe.AcceptRanges
	return item
}

func (a *App) probeGalleryDLRichV86(ctx context.Context, sourceURL string) ([]RemoteItem, error) {
	exe := a.detectGalleryDL()
	if exe == "" {
		return nil, errors.New("gallery-dl nu este instalat/configurat")
	}
	cookieFile, err := os.CreateTemp("", "ddg-gallery-scan-cookies-*.txt")
	if err != nil {
		return nil, err
	}
	cookiePath := cookieFile.Name()
	_ = cookieFile.Close()
	defer os.Remove(cookiePath)

	cmdCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	args := []string{"-J", "--no-colors", "-o", "output.private=true", "-o", "output.jsonl=true", "--cookies-export", cookiePath, sourceURL}
	cmd := exec.CommandContext(cmdCtx, exe, args...)
	hideChildWindow(cmd)
	output, err := cmd.Output()
	if err != nil {
		return nil, errors.New("gallery-dl nu a putut extrage sursa")
	}
	cookieData, _ := os.ReadFile(cookiePath)
	cookies := parseNetscapeCookiesV86(cookieData)
	_ = parseGalleryResolvedContextV86(output, sourceURL, cookies)
	items := parseGalleryRemoteItemsV86(output, sourceURL)
	if len(items) == 0 {
		return nil, errors.New("gallery-dl nu a returnat fișiere")
	}

	// Metadata from the extractor is primary. Small parallel HEAD/range probes
	// enrich only transport details (exact size where missing, hash headers,
	// content type, ETag, Range support). Provider auth is already cached above.
	workers := 8
	if len(items) < workers {
		workers = len(items)
	}
	jobs := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				probe, err := probeHTTPMeta(items[idx].DirectURL)
				if err == nil {
					items[idx] = mergeGalleryProbeV86(items[idx], probe)
				}
			}
		}()
	}
	for i := range items {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	for i := range items {
		items[i].ID = i + 1
	}
	return items, nil
}
