package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

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

func galleryMessageTypeV86(v any) int {
	switch x := v.(type) {
	case json.Number:
		n, _ := strconv.Atoi(x.String())
		return n
	case float64:
		return int(x)
	case int:
		return x
	case int64:
		return int(x)
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(x))
		return n
	default:
		return 0
	}
}

func appendGalleryMessageV86(items *[]RemoteItem, seen map[string]bool, msg []any, sourceURL, source string) {
	// gallery-dl Message.Url == 3. Directory (2) and Queue (6) entries are
	// metadata/control messages and must not become fake downloadable files.
	if len(msg) < 3 || galleryMessageTypeV86(msg[0]) != 3 {
		return
	}
	direct, ok := msg[1].(string)
	if !ok || (!strings.HasPrefix(direct, "http://") && !strings.HasPrefix(direct, "https://")) {
		return
	}
	meta, _ := msg[2].(map[string]any)
	if meta == nil {
		meta = map[string]any{}
	}
	name := galleryRemoteNameV86(meta, direct)
	providerID := galleryStringV86(meta, "id", "id_url", "uuid", "slug", "file_id")
	key := providerContextKeyV86(direct) + "|" + strings.ToLower(name)
	if seen[key] {
		return
	}
	seen[key] = true
	*items = append(*items, RemoteItem{
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

func walkGalleryJSONV86(v any, items *[]RemoteItem, seen map[string]bool, sourceURL, source string) {
	arr, ok := v.([]any)
	if !ok {
		return
	}
	if len(arr) >= 3 && galleryMessageTypeV86(arr[0]) != 0 {
		appendGalleryMessageV86(items, seen, arr, sourceURL, source)
		return
	}
	for _, child := range arr {
		walkGalleryJSONV86(child, items, seen, sourceURL, source)
	}
}

func parseGalleryRemoteItemsV86(output []byte, sourceURL string) []RemoteItem {
	source := providerSourceLabelV86(sourceURL)
	items := []RemoteItem{}
	seen := map[string]bool{}

	// Newer gallery-dl can emit one message per JSON line with output.jsonl.
	// Older builds ignore that option and emit one classic JSON document whose
	// top-level array contains all message tuples. json.Decoder accepts both:
	// multiple consecutive JSON values (JSONL) and a single nested JSON array.
	dec := json.NewDecoder(bytes.NewReader(output))
	dec.UseNumber()
	for {
		var raw any
		err := dec.Decode(&raw)
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
		walkGalleryJSONV86(raw, &items, seen, sourceURL, source)
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

func shouldEnrichGalleryHTTPV86(source string) bool {
	switch strings.ToUpper(strings.TrimSpace(source)) {
	case "GOFILE", "BUNKR", "CYBERDROP":
		// These extractors already provide the metadata needed for the initial
		// local comparison. Probing every signed/authenticated CDN URL here can
		// block the whole scan for tens of seconds per file. Transport details
		// are resolved lazily by preview/verify/download instead.
		return false
	default:
		return true
	}
}

// galleryScanArgsV8555 keeps provider-specific compatibility local to the
// metadata-only scan. Bunkr changes/proxies domains frequently; gallery-dl's
// bunkr.tlds option allows its maintained Bunkr extractor to accept those
// domains instead of rejecting an otherwise valid album before extraction.
// No other provider is changed by this option.
func galleryScanArgsV8555(sourceURL, cookiePath string) []string {
	args := []string{"-J", "--no-colors", "-o", "output.private=true", "-o", "output.jsonl=true"}
	if providerSourceLabelV86(sourceURL) == "BUNKR" {
		args = append(args, "-o", "extractor.bunkr.tlds=true")
	}
	args = append(args, "--cookies-export", cookiePath, sourceURL)
	return args
}

func (a *App) probeGalleryDLRichV86(ctx context.Context, sourceURL string) ([]RemoteItem, error) {
	source := providerSourceLabelV86(sourceURL)
	var nativeErr error
	if source == "GOFILE" {
		nativeCtx, nativeCancel := context.WithTimeout(ctx, 45*time.Second)
		items, err := a.probeGofileNativeV86(nativeCtx, sourceURL)
		nativeCancel()
		if err == nil && len(items) > 0 {
			return items, nil
		}
		nativeErr = err
		if err != nil {
			if gofileRateLimitedV8542(err) {
				a.logf("GoFile: rate-limit detectat; opresc scanarea fără fallback ca să nu creez alte conturi guest: %v", err)
				return nil, err
			}
			a.logf("GoFile: adapterul nativ a eșuat; încerc fallback gallery-dl: %v", err)
		}
	}

	exe := a.detectGalleryDL()
	if exe == "" {
		if nativeErr != nil {
			return nil, errors.New("GoFile API: " + nativeErr.Error() + " | gallery-dl nu este instalat/configurat")
		}
		return nil, errors.New("gallery-dl nu este instalat/configurat")
	}
	cookieFile, err := os.CreateTemp("", "ddg-gallery-scan-cookies-*.txt")
	if err != nil {
		return nil, err
	}
	cookiePath := cookieFile.Name()
	_ = cookieFile.Close()
	defer os.Remove(cookiePath)

	timeout := 3 * time.Minute
	if source == "GOFILE" {
		// Native GoFile is the primary path. The fallback must fail visibly rather
		// than keeping the UI apparently idle for several minutes.
		timeout = 45 * time.Second
	}
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	args := galleryScanArgsV8555(sourceURL, cookiePath)
	cmd := exec.CommandContext(cmdCtx, exe, args...)
	hideChildWindow(cmd)
	output, err := cmd.Output()
	if err != nil {
		detail := "gallery-dl nu a putut extrage sursa"
		if cmdCtx.Err() != nil {
			detail = "gallery-dl a depășit limita de timp"
		}
		if nativeErr != nil {
			return nil, errors.New("GoFile API: " + nativeErr.Error() + " | " + detail)
		}
		return nil, errors.New(detail)
	}
	cookieData, _ := os.ReadFile(cookiePath)
	cookies := parseNetscapeCookiesV86(cookieData)
	_ = parseGalleryResolvedContextV86(output, sourceURL, cookies)
	items := parseGalleryRemoteItemsV86(output, sourceURL)
	if len(items) == 0 {
		if nativeErr != nil {
			return nil, errors.New("GoFile API: " + nativeErr.Error() + " | gallery-dl a răspuns, dar DDG nu a găsit fișiere în metadata")
		}
		return nil, errors.New("gallery-dl a răspuns, dar DDG nu a găsit niciun fișier în metadata")
	}

	if !shouldEnrichGalleryHTTPV86(source) {
		return items, nil
	}

	// Generic gallery-dl sources may not expose a reliable size. Keep the older
	// HTTP enrichment only there; known provider scans must return immediately
	// after extraction instead of waiting on one network request per file.
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
