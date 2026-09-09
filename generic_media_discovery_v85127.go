package main

import (
	"bytes"
	"context"
	cryptorand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"hash/fnv"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const genericMediaServiceV85127 = "ddg-generic-media-v1"
const genericMediaPortBaseV85127 = 38950
const genericMediaPortSpanV85127 = 200
const genericMediaPortAttemptsV85127 = 8
const genericMediaMaxCandidatesV85127 = 400
const genericMediaMaxPagesV85127 = 8
const genericMediaMaxDepthV85127 = 2
const genericMediaMaxHTMLBytesV85127 = 4 << 20
const genericMediaToolOutputLimitV85127 = 32 << 20

type genericMediaQualityV85132 struct {
	Label    string  `json:"label"`
	FormatID string  `json:"formatId,omitempty"`
	Ext      string  `json:"ext,omitempty"`
	Protocol string  `json:"protocol,omitempty"`
	Width    int     `json:"width,omitempty"`
	Height   int     `json:"height,omitempty"`
	FPS      float64 `json:"fps,omitempty"`
	Bitrate  float64 `json:"bitrate,omitempty"`
	HasAudio bool    `json:"hasAudio"`
}

type genericMediaCandidateV85127 struct {
	Token      string                      `json:"token,omitempty"`
	ID         string                      `json:"id,omitempty"`
	URL        string                      `json:"url"`
	PreviewURL string                      `json:"previewUrl,omitempty"`
	Thumbnail  string                      `json:"thumbnail,omitempty"`
	Title      string                      `json:"title,omitempty"`
	Kind       string                      `json:"kind"`
	Via        string                      `json:"via"`
	Page       string                      `json:"page,omitempty"`
	Extractor  string                      `json:"extractor,omitempty"`
	Duration   float64                     `json:"duration,omitempty"`
	Width      int                         `json:"width,omitempty"`
	Height     int                         `json:"height,omitempty"`
	Qualities  []genericMediaQualityV85132 `json:"qualities,omitempty"`
	Headers    map[string]string           `json:"-"`
}

type genericMediaScanReplyV85127 struct {
	OK         bool                          `json:"ok"`
	URL        string                        `json:"url"`
	Candidates []genericMediaCandidateV85127 `json:"candidates"`
	Counts     map[string]int                `json:"counts"`
	Warnings   []string                      `json:"warnings,omitempty"`
}

type genericMediaServiceStateV85127 struct {
	mu       sync.Mutex
	basePort int
	port     int
}

var genericMediaStateV85127 genericMediaServiceStateV85127

type genericMediaPreviewEntryV85132 struct {
	Candidate genericMediaCandidateV85127
	Expires   time.Time
}

var genericMediaPreviewStoreV85132 = struct {
	sync.Mutex
	Items map[string]genericMediaPreviewEntryV85132
}{Items: map[string]genericMediaPreviewEntryV85132{}}

var genericMediaPreviewClientV85132 = &http.Client{
	Transport: &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		MaxIdleConns:          24,
		MaxIdleConnsPerHost:   8,
		IdleConnTimeout:       45 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
	},
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 8 {
			return errors.New("prea multe redirectări preview")
		}
		return nil
	},
}

var genericTagRxV85127 = regexp.MustCompile(`(?is)<(video|audio|source|img|iframe|a)\b[^>]*>`)
var genericAttrRxV85127 = regexp.MustCompile(`(?is)\b(src|href|data-src|data-lazy-src|data-original|data-file|data-video|data-video-src|srcset)\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s>]+))`)
var genericQuotedMediaRxV85127 = regexp.MustCompile(`(?is)["']([^"']+\.(?:m3u8|mpd|mp4|m4v|webm|mov|mkv|avi|flv|ts|m2ts|mts|wmv|mp3|m4a|aac|ogg|opus|flac|wav|jpg|jpeg|png|gif|webp|bmp|avif|heic|heif)(?:\?[^"']*)?)["']`)

func init() {
	base := strings.ToLower(filepath.Base(os.Args[0]))
	if strings.HasSuffix(base, ".test") {
		return
	}
	go startGenericMediaServiceV85127()
}

func genericMediaPortSeedV85127(dataDir string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.ToLower(filepath.Clean(dataDir))))
	return genericMediaPortBaseV85127 + int(h.Sum32()%genericMediaPortSpanV85127)
}

func startGenericMediaServiceV85127() {
	dataDir, err := portableDataDir()
	if err != nil {
		return
	}
	base := genericMediaPortSeedV85127(dataDir)
	var ln net.Listener
	for i := 0; i < genericMediaPortAttemptsV85127; i++ {
		port := base + i
		ln, err = net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
		if err == nil {
			genericMediaStateV85127.mu.Lock()
			genericMediaStateV85127.basePort = base
			genericMediaStateV85127.port = port
			genericMediaStateV85127.mu.Unlock()
			break
		}
		if genericMediaExistingServiceV85127(port) {
			return
		}
	}
	if ln == nil {
		return
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", genericMediaHealthV85127)
	mux.HandleFunc("/scan", genericMediaScanV85127)
	server := &http.Server{
		Handler:           genericMediaCORSV85127(mux),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	_ = server.Serve(ln)
}

func genericMediaExistingServiceV85127(port int) bool {
	client := &http.Client{Timeout: 350 * time.Millisecond}
	resp, err := client.Get("http://127.0.0.1:" + strconv.Itoa(port) + "/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false
	}
	var reply struct {
		OK      bool   `json:"ok"`
		Service string `json:"service"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&reply); err != nil {
		return false
	}
	return reply.OK && reply.Service == genericMediaServiceV85127
}

func genericMediaCORSV85127(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := strings.TrimSpace(r.Header.Get("Origin"))
		if origin != "" {
			u, err := url.Parse(origin)
			host := ""
			if err == nil {
				host = strings.ToLower(u.Hostname())
			}
			if host != "127.0.0.1" && host != "localhost" && host != "::1" {
				http.Error(w, "origine generic-media refuzată", http.StatusForbidden)
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Cache-Control", "no-store")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func genericMediaHealthV85127(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "GET necesar", http.StatusMethodNotAllowed)
		return
	}
	genericMediaStateV85127.mu.Lock()
	port, base := genericMediaStateV85127.port, genericMediaStateV85127.basePort
	genericMediaStateV85127.mu.Unlock()
	genericMediaJSONV85127(w, map[string]any{"ok": true, "service": genericMediaServiceV85127, "version": 1, "port": port, "basePort": base})
}

func genericMediaScanV85127(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST necesar", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	raw := strings.TrimSpace(req.URL)
	if !genericMediaAllowedRootV85127(raw) {
		http.Error(w, "Media Picker avansat acceptă numai pagini HTTP/HTTPS generice", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 35*time.Second)
	defer cancel()
	reply := discoverGenericMediaV85127(ctx, raw)
	genericMediaJSONV85127(w, reply)
}

func genericMediaJSONV85127(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}

func (a *App) handleGenericMediaDiscoverV85132(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST necesar", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	raw := strings.TrimSpace(req.URL)
	if !genericMediaAllowedRootV85127(raw) {
		http.Error(w, "Media Picker acceptă numai pagini HTTP/HTTPS generice", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 40*time.Second)
	defer cancel()
	reply := discoverGenericMediaV85127(ctx, raw)
	for i := range reply.Candidates {
		reply.Candidates[i] = registerGenericMediaPreviewV85132(reply.Candidates[i], i)
	}
	genericMediaJSONV85127(w, reply)
}

func registerGenericMediaPreviewV85132(candidate genericMediaCandidateV85127, index int) genericMediaCandidateV85127 {
	if strings.TrimSpace(candidate.ID) == "" {
		candidate.ID = "media-" + strconv.Itoa(index+1)
	}
	tokenBytes := make([]byte, 18)
	if _, err := cryptorand.Read(tokenBytes); err == nil {
		candidate.Token = hex.EncodeToString(tokenBytes)
	} else {
		candidate.Token = strconv.FormatInt(time.Now().UnixNano(), 36) + "-" + strconv.Itoa(index)
	}
	now := time.Now()
	genericMediaPreviewStoreV85132.Lock()
	for token, entry := range genericMediaPreviewStoreV85132.Items {
		if now.After(entry.Expires) || len(genericMediaPreviewStoreV85132.Items) >= 1200 {
			delete(genericMediaPreviewStoreV85132.Items, token)
		}
	}
	genericMediaPreviewStoreV85132.Items[candidate.Token] = genericMediaPreviewEntryV85132{Candidate: candidate, Expires: now.Add(20 * time.Minute)}
	genericMediaPreviewStoreV85132.Unlock()
	return candidate
}

func (a *App) handleGenericMediaPreviewV85132(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "GET/HEAD necesar", http.StatusMethodNotAllowed)
		return
	}
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	genericMediaPreviewStoreV85132.Lock()
	entry, ok := genericMediaPreviewStoreV85132.Items[token]
	if ok && time.Now().After(entry.Expires) {
		delete(genericMediaPreviewStoreV85132.Items, token)
		ok = false
	}
	genericMediaPreviewStoreV85132.Unlock()
	if !ok {
		http.Error(w, "preview expirat; reanalizează pagina", http.StatusNotFound)
		return
	}

	target := entry.Candidate.PreviewURL
	if strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("asset")), "thumbnail") {
		target = entry.Candidate.Thumbnail
	}
	if firstHTTPV85132(target) == "" {
		http.Error(w, "preview indisponibil", http.StatusNotFound)
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), r.Method, target, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	for _, name := range []string{"User-Agent", "Referer", "Origin", "Accept", "Accept-Language"} {
		if value := genericMediaHeaderV85132(entry.Candidate.Headers, name); value != "" {
			req.Header.Set(name, value)
		}
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/152 Safari/537.36")
	}
	if req.Header.Get("Referer") == "" && firstHTTPV85132(entry.Candidate.Page) != "" {
		req.Header.Set("Referer", entry.Candidate.Page)
	}
	for _, name := range []string{"Range", "If-Range", "If-None-Match", "If-Modified-Since"} {
		if value := strings.TrimSpace(r.Header.Get(name)); value != "" {
			req.Header.Set(name, value)
		}
	}
	resp, err := genericMediaPreviewClientV85132.Do(req)
	if err != nil {
		http.Error(w, "preview: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	for _, name := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "ETag", "Last-Modified", "Cache-Control"} {
		if value := resp.Header.Get(name); value != "" {
			w.Header().Set(name, value)
		}
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(resp.StatusCode)
	if r.Method == http.MethodGet {
		_, _ = io.Copy(w, resp.Body)
	}
}

func genericMediaHeaderV85132(headers map[string]string, name string) string {
	for key, value := range headers {
		if strings.EqualFold(strings.TrimSpace(key), name) {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func genericMediaAllowedRootV85127(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	h := strings.ToLower(u.Hostname())
	if h == "mega.nz" || h == "mega.co.nz" || strings.HasSuffix(h, ".mega.nz") || strings.HasSuffix(h, ".mega.co.nz") {
		return false
	}
	for _, suffix := range []string{"gofile.io", "erome.com"} {
		if h == suffix || strings.HasSuffix(h, "."+suffix) {
			return false
		}
	}
	for _, label := range strings.Split(h, ".") {
		if strings.HasPrefix(label, "bunkr") || strings.HasPrefix(label, "cyberdrop") {
			return false
		}
	}
	return true
}

func discoverGenericMediaV85127(ctx context.Context, raw string) genericMediaScanReplyV85127 {
	type result struct {
		items    []genericMediaCandidateV85127
		warnings []string
	}
	ch := make(chan result, 3)

	go func() {
		htmlCtx, cancel := context.WithTimeout(ctx, 18*time.Second)
		defer cancel()
		items, warnings := crawlGenericMediaHTMLV85127(htmlCtx, raw)
		ch <- result{items: items, warnings: warnings}
	}()
	go func() {
		toolCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
		defer cancel()
		items, warning := genericToolURLsV85127(toolCtx, raw, "yt-dlp")
		warnings := []string{}
		if warning != "" {
			warnings = append(warnings, warning)
		}
		ch <- result{items: items, warnings: warnings}
	}()
	go func() {
		toolCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
		defer cancel()
		items, warning := genericToolURLsV85127(toolCtx, raw, "gallery-dl")
		warnings := []string{}
		if warning != "" {
			warnings = append(warnings, warning)
		}
		ch <- result{items: items, warnings: warnings}
	}()

	all := []genericMediaCandidateV85127{}
	warnings := []string{}
	for i := 0; i < 3; i++ {
		select {
		case got := <-ch:
			all = append(all, got.items...)
			warnings = append(warnings, got.warnings...)
		case <-ctx.Done():
			warnings = append(warnings, "analiza generică a atins limita de timp")
			i = 3
		}
	}
	all = dedupeGenericCandidatesV85127(all)
	if len(all) > genericMediaMaxCandidatesV85127 {
		all = all[:genericMediaMaxCandidatesV85127]
		warnings = append(warnings, "rezultatele au fost limitate la 400 de URL-uri media")
	}
	counts := map[string]int{}
	for _, item := range all {
		counts[item.Via]++
	}
	warnings = uniqueStringsV85127(warnings)
	return genericMediaScanReplyV85127{OK: true, URL: raw, Candidates: all, Counts: counts, Warnings: warnings}
}

func crawlGenericMediaHTMLV85127(ctx context.Context, root string) ([]genericMediaCandidateV85127, []string) {
	type pageJob struct {
		URL   string
		Depth int
	}
	queue := []pageJob{{URL: root, Depth: 0}}
	seenPages := map[string]bool{}
	items := []genericMediaCandidateV85127{}
	warnings := []string{}
	client := &http.Client{
		Timeout: 8 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 6 {
				return errors.New("prea multe redirectări")
			}
			return nil
		},
	}

	for len(queue) > 0 && len(seenPages) < genericMediaMaxPagesV85127 && len(items) < genericMediaMaxCandidatesV85127 {
		job := queue[0]
		queue = queue[1:]
		key := genericMediaURLKeyV85127(job.URL)
		if key == "" || seenPages[key] {
			continue
		}
		seenPages[key] = true
		body, contentType, finalURL, err := fetchGenericPageV85127(ctx, client, job.URL)
		if err != nil {
			if job.Depth == 0 {
				warnings = append(warnings, "HTML: "+err.Error())
			}
			continue
		}
		if kind := genericMediaKindV85127(finalURL, contentType); kind != "" && !strings.Contains(strings.ToLower(contentType), "html") {
			items = append(items, genericMediaCandidateV85127{URL: finalURL, PreviewURL: finalURL, Title: genericMediaTitleV85132(finalURL), Kind: kind, Via: genericMediaViaV85127(kind, "http"), Page: job.URL})
			continue
		}
		pageItems, frames := extractGenericMediaHTMLV85127(finalURL, body)
		items = append(items, pageItems...)
		if job.Depth < genericMediaMaxDepthV85127 {
			for _, frame := range frames {
				if len(queue)+len(seenPages) >= genericMediaMaxPagesV85127 {
					break
				}
				if k := genericMediaURLKeyV85127(frame); k != "" && !seenPages[k] {
					queue = append(queue, pageJob{URL: frame, Depth: job.Depth + 1})
				}
			}
		}
	}
	return dedupeGenericCandidatesV85127(items), warnings
}

func fetchGenericPageV85127(ctx context.Context, client *http.Client, raw string) ([]byte, string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, "", raw, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/152 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/vnd.apple.mpegurl,application/dash+xml,image/avif,image/webp,*/*;q=0.8")
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", raw, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return nil, resp.Header.Get("Content-Type"), resp.Request.URL.String(), errors.New("HTTP " + strconv.Itoa(resp.StatusCode))
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, genericMediaMaxHTMLBytesV85127+1))
	if err != nil {
		return nil, resp.Header.Get("Content-Type"), resp.Request.URL.String(), err
	}
	if len(b) > genericMediaMaxHTMLBytesV85127 {
		b = b[:genericMediaMaxHTMLBytesV85127]
	}
	return b, strings.ToLower(resp.Header.Get("Content-Type")), resp.Request.URL.String(), nil
}

func extractGenericMediaHTMLV85127(pageURL string, body []byte) ([]genericMediaCandidateV85127, []string) {
	text := string(body)
	items := []genericMediaCandidateV85127{}
	frames := []string{}
	for _, loc := range genericTagRxV85127.FindAllStringIndex(text, -1) {
		tagText := text[loc[0]:loc[1]]
		nameMatch := genericTagRxV85127.FindStringSubmatch(tagText)
		if len(nameMatch) < 2 {
			continue
		}
		tagName := strings.ToLower(nameMatch[1])
		for _, attr := range genericAttrRxV85127.FindAllStringSubmatch(tagText, -1) {
			if len(attr) < 5 {
				continue
			}
			attrName := strings.ToLower(attr[1])
			value := firstNonEmptyV85127(attr[2], attr[3], attr[4])
			values := []string{value}
			if attrName == "srcset" {
				values = splitSrcsetV85127(value)
			}
			for _, raw := range values {
				resolved := resolveGenericURLV85127(pageURL, raw)
				if resolved == "" {
					continue
				}
				if tagName == "iframe" && (attrName == "src" || attrName == "data-src") {
					frames = append(frames, resolved)
					continue
				}
				kind := genericMediaKindV85127(resolved, "")
				switch tagName {
				case "img":
					if kind == "" {
						kind = "image"
					}
				case "video":
					if kind == "" {
						kind = "video"
					}
				case "audio":
					if kind == "" {
						kind = "audio"
					}
				case "source":
					if kind == "" {
						kind = "video"
					}
				case "a":
					if kind == "" {
						continue
					}
				}
				if kind != "" {
					items = append(items, genericMediaCandidateV85127{URL: resolved, PreviewURL: resolved, Title: genericMediaTitleV85132(resolved), Kind: kind, Via: genericMediaViaV85127(kind, "html"), Page: pageURL})
				}
			}
		}
	}

	for _, m := range genericQuotedMediaRxV85127.FindAllStringSubmatch(text, -1) {
		if len(m) < 2 {
			continue
		}
		resolved := resolveGenericURLV85127(pageURL, m[1])
		if resolved == "" {
			continue
		}
		kind := genericMediaKindV85127(resolved, "")
		if kind == "" {
			continue
		}
		items = append(items, genericMediaCandidateV85127{URL: resolved, PreviewURL: resolved, Title: genericMediaTitleV85132(resolved), Kind: kind, Via: genericMediaViaV85127(kind, "js"), Page: pageURL})
	}
	return dedupeGenericCandidatesV85127(items), uniqueStringsV85127(frames)
}

func genericToolURLsV85127(ctx context.Context, raw, tool string) ([]genericMediaCandidateV85127, string) {
	exe := genericMediaToolPathV85127(tool)
	if exe == "" {
		return nil, tool + " indisponibil; continui cu celelalte metode"
	}
	var args []string
	switch tool {
	case "yt-dlp":
		args = []string{"--ignore-config", "--dump-json", "--no-warnings", "--skip-download", "--yes-playlist", "--playlist-end", strconv.Itoa(genericMediaMaxCandidatesV85127), raw}
	case "gallery-dl":
		args = []string{"-g", "--no-colors", raw}
	default:
		return nil, "motor generic necunoscut: " + tool
	}
	cmd := exec.CommandContext(ctx, exe, args...)
	hideChildWindow(cmd)
	out := &cappedWriterV85127{limit: genericMediaToolOutputLimitV85127}
	errOut := &cappedWriterV85127{limit: 64 << 10}
	cmd.Stdout = out
	cmd.Stderr = errOut
	err := cmd.Run()
	items := []genericMediaCandidateV85127{}
	if tool == "yt-dlp" {
		items = parseGenericYtDlpCandidatesV85132(out.String(), raw)
	} else {
		items = parseGenericToolURLsV85127(out.String(), raw, tool)
	}
	if len(items) > 0 {
		return items, ""
	}
	if ctx.Err() != nil {
		return nil, tool + " a depășit limita de timp"
	}
	if err != nil {
		detail := strings.TrimSpace(errOut.String())
		if len(detail) > 220 {
			detail = detail[:220] + "…"
		}
		if detail != "" {
			return nil, tool + ": " + detail
		}
		return nil, tool + ": " + err.Error()
	}
	return nil, ""
}

type cappedWriterV85127 struct {
	buf       bytes.Buffer
	limit     int
	truncated bool
}

func (w *cappedWriterV85127) Write(p []byte) (int, error) {
	original := len(p)
	remaining := w.limit - w.buf.Len()
	if remaining > 0 {
		if len(p) > remaining {
			p = p[:remaining]
			w.truncated = true
		}
		_, _ = w.buf.Write(p)
	} else if original > 0 {
		w.truncated = true
	}
	return original, nil
}

func (w *cappedWriterV85127) String() string { return w.buf.String() }

func genericMediaToolPathV85127(tool string) string {
	dataDir, _ := portableDataDir()
	var cfg struct {
		YtDlpPath     string `json:"ytDlpPath"`
		GalleryDLPath string `json:"galleryDlPath"`
	}
	if b, err := os.ReadFile(filepath.Join(dataDir, "config.json")); err == nil {
		_ = json.Unmarshal(b, &cfg)
	}
	candidates := []string{}
	switch tool {
	case "yt-dlp":
		candidates = append(candidates, cfg.YtDlpPath, filepath.Join(executableDir(), "tools", "yt-dlp", "yt-dlp.exe"))
	case "gallery-dl":
		candidates = append(candidates, cfg.GalleryDLPath, filepath.Join(executableDir(), "tools", "gallery-dl", "gallery-dl.exe"))
	}
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
			return candidate
		}
	}
	if p, err := exec.LookPath(tool); err == nil {
		return p
	}
	return ""
}

func parseGenericToolURLsV85127(output, pageURL, via string) []genericMediaCandidateV85127 {
	items := []genericMediaCandidateV85127{}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "http://") && !strings.HasPrefix(line, "https://") {
			continue
		}
		kind := genericMediaKindV85127(line, "")
		if kind == "" {
			kind = "other"
		}
		items = append(items, genericMediaCandidateV85127{URL: line, PreviewURL: line, Title: genericMediaTitleV85132(line), Kind: kind, Via: via, Page: pageURL})
	}
	return dedupeGenericCandidatesV85127(items)
}

type genericYtDlpFormatV85132 struct {
	FormatID    string            `json:"format_id"`
	FormatNote  string            `json:"format_note"`
	URL         string            `json:"url"`
	Ext         string            `json:"ext"`
	Protocol    string            `json:"protocol"`
	VCodec      string            `json:"vcodec"`
	ACodec      string            `json:"acodec"`
	Width       int               `json:"width"`
	Height      int               `json:"height"`
	FPS         float64           `json:"fps"`
	TBR         float64           `json:"tbr"`
	HTTPHeaders map[string]string `json:"http_headers"`
}

type genericYtDlpEntryV85132 struct {
	ID               string                     `json:"id"`
	Title            string                     `json:"title"`
	FullTitle        string                     `json:"fulltitle"`
	URL              string                     `json:"url"`
	WebpageURL       string                     `json:"webpage_url"`
	OriginalURL      string                     `json:"original_url"`
	Thumbnail        string                     `json:"thumbnail"`
	Extractor        string                     `json:"extractor"`
	ExtractorKey     string                     `json:"extractor_key"`
	Ext              string                     `json:"ext"`
	VCodec           string                     `json:"vcodec"`
	ACodec           string                     `json:"acodec"`
	Width            int                        `json:"width"`
	Height           int                        `json:"height"`
	FPS              float64                    `json:"fps"`
	Duration         float64                    `json:"duration"`
	HTTPHeaders      map[string]string          `json:"http_headers"`
	Formats          []genericYtDlpFormatV85132 `json:"formats"`
	RequestedFormats []genericYtDlpFormatV85132 `json:"requested_formats"`
	Entries          []json.RawMessage          `json:"entries"`
	Thumbnails       []struct {
		URL string `json:"url"`
	} `json:"thumbnails"`
}

func parseGenericYtDlpCandidatesV85132(output, pageURL string) []genericMediaCandidateV85127 {
	decoder := json.NewDecoder(strings.NewReader(output))
	items := []genericMediaCandidateV85127{}
	for {
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			break
		}
		items = append(items, parseGenericYtDlpEntryV85132(raw, pageURL)...)
	}
	return dedupeGenericCandidatesV85127(items)
}

func parseGenericYtDlpEntryV85132(raw json.RawMessage, pageURL string) []genericMediaCandidateV85127 {
	var entry genericYtDlpEntryV85132
	if err := json.Unmarshal(raw, &entry); err != nil {
		return nil
	}
	if len(entry.Entries) > 0 {
		items := []genericMediaCandidateV85127{}
		for _, child := range entry.Entries {
			items = append(items, parseGenericYtDlpEntryV85132(child, pageURL)...)
		}
		return items
	}

	actionURL := firstHTTPV85132(entry.WebpageURL, entry.OriginalURL, pageURL, entry.URL)
	if actionURL == "" {
		return nil
	}
	thumbnail := firstHTTPV85132(entry.Thumbnail)
	if thumbnail == "" {
		for i := len(entry.Thumbnails) - 1; i >= 0; i-- {
			if thumbnail = firstHTTPV85132(entry.Thumbnails[i].URL); thumbnail != "" {
				break
			}
		}
	}

	formats := append([]genericYtDlpFormatV85132{}, entry.Formats...)
	formats = append(formats, entry.RequestedFormats...)
	qualities := genericYtDlpQualitiesV85132(formats)
	preview, headers, bestWidth, bestHeight := genericYtDlpPreviewV85132(entry, formats)
	hasVideo := entry.Width > 0 || entry.Height > 0 || ytDlpCodecPresentV85132(entry.VCodec)
	for _, format := range formats {
		if ytDlpCodecPresentV85132(format.VCodec) {
			hasVideo = true
			break
		}
	}
	kind := genericMediaKindV85127(entry.URL, "")
	if hasVideo {
		kind = "video"
	} else if ytDlpCodecPresentV85132(entry.ACodec) {
		kind = "audio"
	} else if kind == "" {
		kind = "other"
	}
	title := strings.TrimSpace(firstNonEmptyV85127(entry.Title, entry.FullTitle))
	if title == "" {
		title = genericMediaTitleV85132(actionURL)
	}
	extractor := strings.TrimSpace(firstNonEmptyV85127(entry.Extractor, entry.ExtractorKey))
	return []genericMediaCandidateV85127{{
		ID:         strings.TrimSpace(entry.ID),
		URL:        actionURL,
		PreviewURL: preview,
		Thumbnail:  thumbnail,
		Title:      title,
		Kind:       kind,
		Via:        "yt-dlp",
		Page:       pageURL,
		Extractor:  extractor,
		Duration:   entry.Duration,
		Width:      firstPositiveIntV85132(entry.Width, bestWidth),
		Height:     firstPositiveIntV85132(entry.Height, bestHeight),
		Qualities:  qualities,
		Headers:    headers,
	}}
}

func genericYtDlpQualitiesV85132(formats []genericYtDlpFormatV85132) []genericMediaQualityV85132 {
	qualities := []genericMediaQualityV85132{}
	seen := map[string]bool{}
	for _, format := range formats {
		if !ytDlpCodecPresentV85132(format.VCodec) {
			continue
		}
		key := strings.Join([]string{format.FormatID, strconv.Itoa(format.Width), strconv.Itoa(format.Height), strconv.FormatFloat(format.FPS, 'f', 2, 64), format.Ext}, "|")
		if seen[key] {
			continue
		}
		seen[key] = true
		label := genericYtDlpQualityLabelV85132(format)
		qualities = append(qualities, genericMediaQualityV85132{
			Label:    label,
			FormatID: format.FormatID,
			Ext:      format.Ext,
			Protocol: format.Protocol,
			Width:    format.Width,
			Height:   format.Height,
			FPS:      format.FPS,
			Bitrate:  format.TBR,
			HasAudio: ytDlpCodecPresentV85132(format.ACodec),
		})
	}
	sort.SliceStable(qualities, func(i, j int) bool {
		if qualities[i].Height != qualities[j].Height {
			return qualities[i].Height > qualities[j].Height
		}
		if qualities[i].FPS != qualities[j].FPS {
			return qualities[i].FPS > qualities[j].FPS
		}
		if qualities[i].HasAudio != qualities[j].HasAudio {
			return qualities[i].HasAudio
		}
		return qualities[i].Bitrate > qualities[j].Bitrate
	})
	if len(qualities) > 24 {
		qualities = qualities[:24]
	}
	return qualities
}

func genericYtDlpQualityLabelV85132(format genericYtDlpFormatV85132) string {
	parts := []string{}
	if format.Height > 0 {
		label := strconv.Itoa(format.Height) + "p"
		if format.FPS >= 50 {
			label += strconv.Itoa(int(format.FPS + 0.5))
		}
		parts = append(parts, label)
	} else if format.Width > 0 {
		parts = append(parts, strconv.Itoa(format.Width)+"px")
	} else if strings.TrimSpace(format.FormatNote) != "" {
		parts = append(parts, strings.TrimSpace(format.FormatNote))
	} else if strings.TrimSpace(format.FormatID) != "" {
		parts = append(parts, strings.TrimSpace(format.FormatID))
	}
	if strings.TrimSpace(format.Ext) != "" {
		parts = append(parts, strings.ToUpper(strings.TrimSpace(format.Ext)))
	}
	if ytDlpCodecPresentV85132(format.ACodec) {
		parts = append(parts, "audio")
	}
	if len(parts) == 0 {
		return "video"
	}
	return strings.Join(parts, " • ")
}

func genericYtDlpPreviewV85132(entry genericYtDlpEntryV85132, formats []genericYtDlpFormatV85132) (string, map[string]string, int, int) {
	bestURL := ""
	bestHeaders := entry.HTTPHeaders
	bestWidth, bestHeight := entry.Width, entry.Height
	bestScore := float64(-1)
	for _, format := range formats {
		if firstHTTPV85132(format.URL) == "" || !ytDlpCodecPresentV85132(format.VCodec) {
			continue
		}
		score := float64(format.Height)*1000000 + float64(format.Width)*1000 + format.TBR
		if ytDlpCodecPresentV85132(format.ACodec) {
			score += 10000000000
		}
		if score > bestScore {
			bestScore = score
			bestURL = format.URL
			bestHeaders = format.HTTPHeaders
			bestWidth, bestHeight = format.Width, format.Height
		}
	}
	if bestURL == "" {
		bestURL = firstHTTPV85132(entry.URL)
	}
	if len(bestHeaders) == 0 {
		bestHeaders = entry.HTTPHeaders
	}
	return bestURL, bestHeaders, bestWidth, bestHeight
}

func ytDlpCodecPresentV85132(codec string) bool {
	codec = strings.ToLower(strings.TrimSpace(codec))
	return codec != "" && codec != "none"
}

func firstHTTPV85132(values ...string) string {
	for _, value := range values {
		u, err := url.Parse(strings.TrimSpace(value))
		if err == nil && u.Hostname() != "" && (u.Scheme == "http" || u.Scheme == "https") {
			return u.String()
		}
	}
	return ""
}

func firstPositiveIntV85132(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func genericMediaTitleV85132(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return strings.TrimSpace(raw)
	}
	name := strings.TrimSpace(filepath.Base(strings.TrimSuffix(u.Path, "/")))
	if decoded, err := url.PathUnescape(name); err == nil {
		name = decoded
	}
	name = strings.TrimSpace(html.UnescapeString(name))
	if name == "" || name == "." {
		name = u.Hostname()
	}
	return name
}

func genericMediaKindV85127(raw, contentType string) string {
	ct := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	switch {
	case strings.HasPrefix(ct, "image/"):
		return "image"
	case strings.HasPrefix(ct, "video/"):
		return "video"
	case strings.HasPrefix(ct, "audio/"):
		return "audio"
	case strings.Contains(ct, "mpegurl") || strings.Contains(ct, "m3u8"):
		return "hls"
	case strings.Contains(ct, "dash+xml"):
		return "dash"
	}
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	ext := strings.ToLower(filepath.Ext(u.Path))
	switch ext {
	case ".m3u8":
		return "hls"
	case ".mpd":
		return "dash"
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp", ".avif", ".heic", ".heif":
		return "image"
	case ".mp4", ".m4v", ".webm", ".mov", ".mkv", ".avi", ".flv", ".ts", ".m2ts", ".mts", ".wmv":
		return "video"
	case ".mp3", ".m4a", ".aac", ".ogg", ".opus", ".flac", ".wav":
		return "audio"
	default:
		return ""
	}
}

func genericMediaViaV85127(kind, base string) string {
	if kind == "hls" {
		return "hls"
	}
	if kind == "dash" {
		return "dash"
	}
	return base
}

func resolveGenericURLV85127(base, raw string) string {
	raw = strings.TrimSpace(html.UnescapeString(raw))
	if raw == "" {
		return ""
	}
	raw = strings.ReplaceAll(raw, `\/`, "/")
	raw = strings.ReplaceAll(raw, `\u002F`, "/")
	raw = strings.ReplaceAll(raw, `\u002f`, "/")
	raw = strings.ReplaceAll(raw, `\u0026`, "&")
	raw = strings.ReplaceAll(raw, `\x2F`, "/")
	raw = strings.ReplaceAll(raw, `\x2f`, "/")
	if i := strings.IndexAny(raw, " \t\r\n"); i >= 0 {
		raw = raw[:i]
	}
	lower := strings.ToLower(raw)
	if strings.HasPrefix(lower, "data:") || strings.HasPrefix(lower, "blob:") || strings.HasPrefix(lower, "javascript:") || strings.HasPrefix(raw, "#") {
		return ""
	}
	b, err := url.Parse(base)
	if err != nil {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	resolved := b.ResolveReference(u)
	if resolved.Scheme != "http" && resolved.Scheme != "https" {
		return ""
	}
	resolved.Fragment = ""
	return resolved.String()
}

func genericMediaURLKeyV85127(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Hostname() == "" {
		return ""
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	u.Fragment = ""
	return u.String()
}

func dedupeGenericCandidatesV85127(items []genericMediaCandidateV85127) []genericMediaCandidateV85127 {
	out := make([]genericMediaCandidateV85127, 0, len(items))
	seen := map[string]int{}
	for _, item := range items {
		item.URL = strings.TrimSpace(item.URL)
		key := genericMediaURLKeyV85127(item.URL)
		if key == "" {
			continue
		}
		if idx, ok := seen[key]; ok {
			if (out[idx].Kind == "video" || out[idx].Kind == "") && (item.Kind == "hls" || item.Kind == "dash") {
				out[idx].Kind = item.Kind
				out[idx].Via = item.Via
			}
			continue
		}
		seen[key] = len(out)
		out = append(out, item)
	}
	sort.SliceStable(out, func(i, j int) bool {
		weight := func(kind string) int {
			switch kind {
			case "video", "hls", "dash":
				return 0
			case "image":
				return 1
			case "audio":
				return 2
			default:
				return 3
			}
		}
		wi, wj := weight(out[i].Kind), weight(out[j].Kind)
		if wi != wj {
			return wi < wj
		}
		return out[i].URL < out[j].URL
	})
	return out
}

func uniqueStringsV85127(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func firstNonEmptyV85127(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func splitSrcsetV85127(value string) []string {
	out := []string{}
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if i := strings.IndexAny(part, " \t"); i >= 0 {
			part = part[:i]
		}
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
