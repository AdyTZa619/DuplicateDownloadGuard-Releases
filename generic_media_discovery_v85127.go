package main

import (
	"bytes"
	"context"
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
const genericMediaToolOutputLimitV85127 = 4 << 20

type genericMediaCandidateV85127 struct {
	URL  string `json:"url"`
	Kind string `json:"kind"`
	Via  string `json:"via"`
	Page string `json:"page,omitempty"`
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
			items = append(items, genericMediaCandidateV85127{URL: finalURL, Kind: kind, Via: genericMediaViaV85127(kind, "http"), Page: job.URL})
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
					items = append(items, genericMediaCandidateV85127{URL: resolved, Kind: kind, Via: genericMediaViaV85127(kind, "html"), Page: pageURL})
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
		items = append(items, genericMediaCandidateV85127{URL: resolved, Kind: kind, Via: genericMediaViaV85127(kind, "js"), Page: pageURL})
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
		args = []string{"--no-warnings", "--skip-download", "--get-url", "--yes-playlist", raw}
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
	items := parseGenericToolURLsV85127(out.String(), raw, tool)
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
			kind = "video"
		}
		items = append(items, genericMediaCandidateV85127{URL: line, Kind: kind, Via: via, Page: pageURL})
	}
	return dedupeGenericCandidatesV85127(items)
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
