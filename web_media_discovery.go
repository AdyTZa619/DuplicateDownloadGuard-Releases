package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	webPageMaxBytes = 12 << 20
	webMediaMaxItems = 300
)

type webMediaCandidate struct {
	URL      string
	Kind     string
	Evidence string
}

var webTagRE = regexp.MustCompile(`(?is)<(img|video|audio|source|a|link|meta)\b([^>]*)>`)
var webAttrRE = regexp.MustCompile("(?is)([a-zA-Z_:][-a-zA-Z0-9_:.]*)\\s*=\\s*(?:\"([^\"]*)\"|'([^']*)'|([^\\s\"'=<>`]+))")
var webJSONLDRE = regexp.MustCompile(`(?is)<script\b[^>]*type\s*=\s*["']application/ld\+json["'][^>]*>(.*?)</script>`)

func parseWebAttrs(raw string) map[string]string {
	out := map[string]string{}
	for _, m := range webAttrRE.FindAllStringSubmatch(raw, -1) {
		if len(m) < 5 {
			continue
		}
		v := m[2]
		if v == "" {
			v = m[3]
		}
		if v == "" {
			v = m[4]
		}
		out[strings.ToLower(strings.TrimSpace(m[1]))] = html.UnescapeString(strings.TrimSpace(v))
	}
	return out
}

func mediaKindFromContentType(ct string) string {
	ct = strings.ToLower(strings.TrimSpace(strings.Split(ct, ";")[0]))
	switch {
	case strings.HasPrefix(ct, "image/"):
		return "image"
	case strings.HasPrefix(ct, "video/"):
		return "video"
	case strings.HasPrefix(ct, "audio/"):
		return "audio"
	default:
		return ""
	}
}

func mediaKindFromURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	ext := strings.ToLower(path.Ext(u.Path))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp", ".avif", ".heic", ".heif":
		return "image"
	case ".mp4", ".webm", ".ogv", ".mov", ".m4v", ".mkv", ".avi", ".flv", ".ts", ".mts", ".m2ts":
		return "video"
	case ".mp3", ".wav", ".ogg", ".m4a", ".aac", ".flac", ".opus":
		return "audio"
	default:
		return ""
	}
}

func resolveWebMediaURL(base *url.URL, raw string) string {
	raw = strings.TrimSpace(html.UnescapeString(raw))
	if raw == "" || strings.HasPrefix(raw, "#") {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if !u.IsAbs() {
		u = base.ResolveReference(u)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return ""
	}
	u.Fragment = ""
	return u.String()
}

func bestSrcsetValue(srcset string) string {
	type choice struct {
		u     string
		score float64
		pos   int
	}
	var best choice
	for i, part := range strings.Split(srcset, ",") {
		f := strings.Fields(strings.TrimSpace(part))
		if len(f) == 0 {
			continue
		}
		score := float64(i + 1)
		if len(f) > 1 {
			d := strings.ToLower(f[len(f)-1])
			if strings.HasSuffix(d, "w") {
				if n, err := strconv.ParseFloat(strings.TrimSuffix(d, "w"), 64); err == nil {
					score = n
				}
			} else if strings.HasSuffix(d, "x") {
				if n, err := strconv.ParseFloat(strings.TrimSuffix(d, "x"), 64); err == nil {
					score = n * 100000
				}
			}
		}
		if best.u == "" || score > best.score || (score == best.score && i > best.pos) {
			best = choice{u: f[0], score: score, pos: i}
		}
	}
	return best.u
}

func extractWebMediaCandidates(body []byte, pageURL string) []webMediaCandidate {
	base, err := url.Parse(pageURL)
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	out := make([]webMediaCandidate, 0, 32)
	add := func(raw, hint, evidence string) {
		if len(out) >= webMediaMaxItems {
			return
		}
		u := resolveWebMediaURL(base, raw)
		if u == "" || seen[u] {
			return
		}
		kind := hint
		if kind == "" {
			kind = mediaKindFromURL(u)
		}
		seen[u] = true
		out = append(out, webMediaCandidate{URL: u, Kind: kind, Evidence: evidence})
	}

	s := string(body)
	for _, m := range webTagRE.FindAllStringSubmatch(s, -1) {
		if len(m) < 3 {
			continue
		}
		tag := strings.ToLower(m[1])
		a := parseWebAttrs(m[2])
		switch tag {
		case "img":
			if v := bestSrcsetValue(a["srcset"]); v != "" {
				add(v, "image", "img:srcset")
			} else {
				for _, key := range []string{"data-original", "data-full", "data-src", "data-lazy-src", "src"} {
					if a[key] != "" {
						add(a[key], "image", "img:"+key)
						break
					}
				}
		case "video":
			add(a["src"], "video", "video:src")
			add(a["poster"], "image", "video:poster")
		case "audio":
			add(a["src"], "audio", "audio:src")
		case "source":
			kind := mediaKindFromContentType(a["type"])
			if v := bestSrcsetValue(a["srcset"]); v != "" {
				if kind == "" {
					kind = "image"
				}
				add(v, kind, "source:srcset")
			}
			add(a["src"], kind, "source:src")
		case "a", "link":
			href := a["href"]
			kind := mediaKindFromContentType(a["type"])
			if kind != "" || mediaKindFromURL(href) != "" {
				add(href, kind, tag+":href")
			}
		case "meta":
			key := strings.ToLower(strings.TrimSpace(a["property"]))
			if key == "" {
				key = strings.ToLower(strings.TrimSpace(a["name"]))
			}
			kind := ""
			switch key {
			case "og:image", "og:image:url", "og:image:secure_url", "twitter:image", "twitter:image:src":
				kind = "image"
			case "og:video", "og:video:url", "og:video:secure_url", "twitter:player:stream":
				kind = "video"
			case "og:audio", "og:audio:url", "og:audio:secure_url":
				kind = "audio"
			}
			if kind != "" {
				add(a["content"], kind, "meta:"+key)
			}
		}
	}

	for _, m := range webJSONLDRE.FindAllStringSubmatch(s, -1) {
		if len(m) < 2 {
			continue
		}
		var root any
		if json.Unmarshal([]byte(html.UnescapeString(strings.TrimSpace(m[1]))), &root) != nil {
			continue
		}
		collectJSONLDMedia(root, "", func(raw, kind, evidence string) { add(raw, kind, evidence) })
	}
	return out
}

func collectJSONLDMedia(v any, parentType string, add func(string, string, string)) {
	switch x := v.(type) {
	case []any:
		for _, item := range x {
			collectJSONLDMedia(item, parentType, add)
		}
	case map[string]any:
		typ := parentType
		if t, ok := x["@type"].(string); ok {
			typ = strings.ToLower(t)
		}
		for k, val := range x {
			lk := strings.ToLower(k)
			kind := ""
			switch lk {
			case "contenturl", "uploadurl":
				if strings.Contains(typ, "image") {
					kind = "image"
				} else if strings.Contains(typ, "audio") {
					kind = "audio"
				} else {
					kind = "video"
				}
			case "thumbnailurl", "image":
				kind = "image"
			case "video":
				kind = "video"
			case "url":
				if strings.Contains(typ, "imageobject") {
					kind = "image"
				} else if strings.Contains(typ, "videoobject") {
					kind = "video"
				} else if strings.Contains(typ, "audioobject") {
					kind = "audio"
				}
			}
			if kind != "" {
				switch y := val.(type) {
				case string:
					add(y, kind, "jsonld:"+lk)
				case []any:
					for _, z := range y {
						if s, ok := z.(string); ok {
							add(s, kind, "jsonld:"+lk)
						}
					}
				}
			}
			collectJSONLDMedia(val, typ, add)
		}
	}
}

func parseContentRangeTotal(v string) int64 {
	// bytes 0-0/123456
	if i := strings.LastIndex(v, "/"); i >= 0 && i+1 < len(v) {
		n, _ := strconv.ParseInt(strings.TrimSpace(v[i+1:]), 10, 64)
		return n
	}
	return -1
}

func mediaNameFromResponse(resp *http.Response, rawURL, kind string, ordinal int) string {
	if resp != nil {
		if cd := resp.Header.Get("Content-Disposition"); cd != "" {
			if _, p, err := mime.ParseMediaType(cd); err == nil && strings.TrimSpace(p["filename"]) != "" {
				return sanitizeFilename(p["filename"])
			}
		}
	}
	u, _ := url.Parse(rawURL)
	name := path.Base(u.Path)
	if name != "" && name != "." && name != "/" {
		return sanitizeFilename(name)
	}
	ext := ""
	if resp != nil {
		switch mediaKindFromContentType(resp.Header.Get("Content-Type")) {
		case "image":
			ext = ".jpg"
		case "video":
			ext = ".mp4"
		case "audio":
			ext = ".mp3"
		}
	}
	if ext == "" {
		switch kind {
		case "image":
			ext = ".jpg"
		case "video":
			ext = ".mp4"
		case "audio":
			ext = ".mp3"
		}
	}
	return fmt.Sprintf("web-media-%03d%s", ordinal, ext)
}

func webMediaStableID(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	stable := strings.ToLower(u.Scheme+"://"+u.Host) + u.EscapedPath()
	h := sha256.Sum256([]byte(stable))
	return "web:" + hex.EncodeToString(h[:12])
}

func probeHTMLMediaCandidate(ctx context.Context, pageURL string, c webMediaCandidate, ordinal int) (RemoteItem, bool) {
	kind := c.Kind
	client := &http.Client{Timeout: 12 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > 8 {
			return errors.New("prea multe redirectări")
		}
		return nil
	}}
	do := func(method string, useRange bool) (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, method, c.URL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/140 Safari/537.36 DuplicateDownloadGuard/8.6")
		req.Header.Set("Accept", "image/avif,image/webp,image/apng,image/*,video/*,audio/*,*/*;q=0.8")
		req.Header.Set("Referer", pageURL)
		if useRange {
			req.Header.Set("Range", "bytes=0-0")
		}
		return client.Do(req)
	}

	resp, err := do(http.MethodHead, false)
	if err != nil || resp.StatusCode == http.StatusMethodNotAllowed || resp.StatusCode == http.StatusForbidden {
		if resp != nil {
			resp.Body.Close()
		}
		resp, err = do(http.MethodGet, true)
	}
	if err != nil {
		if kind == "" {
			kind = mediaKindFromURL(c.URL)
		}
		if kind == "" {
			return RemoteItem{}, false
		}
		return RemoteItem{Path: c.URL, Name: mediaNameFromResponse(nil, c.URL, kind, ordinal), Size: -1, Source: "HTML", URL: pageURL, DirectURL: c.URL, ProviderID: webMediaStableID(c.URL), ApproxSize: true}, true
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return RemoteItem{}, false
	}
	ct := resp.Header.Get("Content-Type")
	ctKind := mediaKindFromContentType(ct)
	if ctKind != "" {
		kind = ctKind
	}
	if kind == "" {
		kind = mediaKindFromURL(resp.Request.URL.String())
	}
	if kind == "" {
		return RemoteItem{}, false
	}

	size := resp.ContentLength
	if total := parseContentRangeTotal(resp.Header.Get("Content-Range")); total > 0 {
		size = total
	}
	if size == 1 && resp.StatusCode == http.StatusPartialContent {
		size = parseContentRangeTotal(resp.Header.Get("Content-Range"))
	}
	if size <= 0 {
		size = -1
	}
	finalURL := resp.Request.URL.String()
	item := RemoteItem{
		Path:         finalURL,
		Name:         mediaNameFromResponse(resp, finalURL, kind, ordinal),
		Size:         size,
		Source:       "HTML",
		URL:          pageURL,
		DirectURL:    finalURL,
		ProviderID:   webMediaStableID(finalURL),
		ContentType:  ct,
		ETag:         resp.Header.Get("ETag"),
		AcceptRanges: strings.Contains(strings.ToLower(resp.Header.Get("Accept-Ranges")), "bytes") || resp.StatusCode == http.StatusPartialContent,
		ApproxSize:   size <= 0,
	}
	return item, true
}

func discoverHTMLMedia(ctx context.Context, pageURL string) ([]RemoteItem, error) {
	pu, err := url.Parse(strings.TrimSpace(pageURL))
	if err != nil || (pu.Scheme != "http" && pu.Scheme != "https") {
		return nil, errors.New("URL pagină invalid")
	}
	client := &http.Client{Timeout: 25 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > 10 {
			return errors.New("prea multe redirectări")
		}
		return nil
	}}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/140 Safari/537.36 DuplicateDownloadGuard/8.6")
	req.Header.Set("Accept", "text/html,application/xhtml+xml;q=0.9,*/*;q=0.5")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("pagina HTTP %d", resp.StatusCode)
	}
	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	if ct != "" && !strings.Contains(ct, "text/html") && !strings.Contains(ct, "application/xhtml") {
		return nil, errors.New("resursa nu este pagină HTML")
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, webPageMaxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > webPageMaxBytes {
		return nil, fmt.Errorf("pagina HTML depășește %d MB", webPageMaxBytes>>20)
	}
	finalPage := resp.Request.URL.String()
	candidates := extractWebMediaCandidates(body, finalPage)
	if len(candidates) == 0 {
		return nil, errors.New("pagina nu conține URL-uri media publice detectabile")
	}

	items := make([]RemoteItem, 0, len(candidates))
	for i, c := range candidates {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		item, ok := probeHTMLMediaCandidate(ctx, finalPage, c, i+1)
		if !ok {
			continue
		}
		item.ID = len(items) + 1
		items = append(items, item)
	}
	if len(items) == 0 {
		return nil, errors.New("am găsit referințe media în HTML, dar niciuna nu a putut fi validată")
	}
	sort.SliceStable(items, func(i, j int) bool {
		ki, kj := remoteMediaKind(items[i].Name), remoteMediaKind(items[j].Name)
		if ki != kj {
			return ki < kj
		}
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})
	for i := range items {
		items[i].ID = i + 1
	}
	return items, nil
}
