package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const maxHTMLDiscoveryBytesV860 = 8 << 20
const maxHTMLMediaItemsV860 = 500

var htmlMediaTagREV860 = regexp.MustCompile(`(?is)<(img|video|source|meta|a|link)\b([^>]*)>`)
var htmlAttrREV860 = regexp.MustCompile(`(?is)([a-zA-Z_:][a-zA-Z0-9_:.-]*)\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s"'=<>]+))`)
var jsonLDREV860 = regexp.MustCompile(`(?is)<script\b[^>]*type\s*=\s*["']application/ld\+json["'][^>]*>(.*?)</script>`)

func htmlAttrsV860(raw string) map[string]string {
	out := map[string]string{}
	for _, m := range htmlAttrREV860.FindAllStringSubmatch(raw, -1) {
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

func resolveWebMediaURLV860(base *url.URL, raw string) string {
	raw = strings.TrimSpace(html.UnescapeString(raw))
	if raw == "" || strings.HasPrefix(strings.ToLower(raw), "data:") || strings.HasPrefix(strings.ToLower(raw), "blob:") || strings.HasPrefix(raw, "#") {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if base != nil {
		u = base.ResolveReference(u)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return ""
	}
	u.Fragment = ""
	return u.String()
}

func mediaExtV860(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return strings.ToLower(path.Ext(u.Path))
}

func contentTypeForMediaExtV860(ext string) string {
	switch strings.ToLower(ext) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".bmp":
		return "image/bmp"
	case ".avif":
		return "image/avif"
	case ".mp4", ".m4v":
		return "video/mp4"
	case ".webm":
		return "video/webm"
	case ".mov":
		return "video/quicktime"
	case ".mkv":
		return "video/x-matroska"
	case ".avi":
		return "video/x-msvideo"
	case ".flv":
		return "video/x-flv"
	case ".ts", ".m2ts", ".mts":
		return "video/mp2t"
	case ".mp3":
		return "audio/mpeg"
	case ".m4a", ".aac":
		return "audio/mp4"
	case ".ogg", ".opus":
		return "audio/ogg"
	case ".flac":
		return "audio/flac"
	case ".wav":
		return "audio/wav"
	case ".m3u8", ".mpd":
		return "stream/manifest"
	default:
		return ""
	}
}

func isKnownMediaURLV860(rawURL string) bool {
	return contentTypeForMediaExtV860(mediaExtV860(rawURL)) != ""
}

func bestSrcsetURLV860(raw string) string {
	type candidate struct {
		u     string
		score float64
	}
	var rows []candidate
	for i, part := range strings.Split(raw, ",") {
		fields := strings.Fields(strings.TrimSpace(part))
		if len(fields) == 0 {
			continue
		}
		score := float64(i + 1)
		if len(fields) > 1 {
			d := strings.ToLower(fields[1])
			if strings.HasSuffix(d, "w") {
				if n, err := strconv.ParseFloat(strings.TrimSuffix(d, "w"), 64); err == nil {
					score = n
				}
			} else if strings.HasSuffix(d, "x") {
				if n, err := strconv.ParseFloat(strings.TrimSuffix(d, "x"), 64); err == nil {
					score = n * 10000
				}
			}
		}
		rows = append(rows, candidate{u: fields[0], score: score})
	}
	if len(rows) == 0 {
		return ""
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].score > rows[j].score })
	return rows[0].u
}

type discoveredMediaV860 struct {
	URL  string
	Kind string
}

func extractHTMLMediaV860(pageURL string, body []byte) []RemoteItem {
	base, _ := url.Parse(pageURL)
	seen := map[string]bool{}
	found := make([]discoveredMediaV860, 0, 64)
	add := func(raw, kind string, requireKnown bool) {
		if len(found) >= maxHTMLMediaItemsV860 {
			return
		}
		u := resolveWebMediaURLV860(base, raw)
		if u == "" || seen[u] {
			return
		}
		if requireKnown && !isKnownMediaURLV860(u) {
			return
		}
		seen[u] = true
		found = append(found, discoveredMediaV860{URL: u, Kind: kind})
	}

	text := string(body)
	for _, m := range htmlMediaTagREV860.FindAllStringSubmatch(text, -1) {
		tag := strings.ToLower(m[1])
		a := htmlAttrsV860(m[2])
		switch tag {
		case "img":
			if srcset := a["srcset"]; srcset != "" {
				add(bestSrcsetURLV860(srcset), "image", false)
			} else if srcset := a["data-srcset"]; srcset != "" {
				add(bestSrcsetURLV860(srcset), "image", false)
			}
			for _, key := range []string{"src", "data-src", "data-original", "data-lazy-src"} {
				add(a[key], "image", false)
			}
		case "video":
			add(a["src"], "video", false)
			add(a["poster"], "image", false)
		case "source":
			kind := "video"
			if strings.HasPrefix(strings.ToLower(a["type"]), "audio/") {
				kind = "audio"
			}
			add(a["src"], kind, false)
			if srcset := a["srcset"]; srcset != "" {
				add(bestSrcsetURLV860(srcset), kind, false)
			}
		case "meta":
			key := strings.ToLower(a["property"])
			if key == "" {
				key = strings.ToLower(a["name"])
			}
			kind := ""
			switch key {
			case "og:image", "og:image:url", "og:image:secure_url", "twitter:image", "twitter:image:src":
				kind = "image"
			case "og:video", "og:video:url", "og:video:secure_url", "twitter:player:stream":
				kind = "video"
			}
			if kind != "" {
				add(a["content"], kind, false)
			}
		case "a":
			add(a["href"], "", true)
		case "link":
			rel := strings.ToLower(a["rel"])
			as := strings.ToLower(a["as"])
			if strings.Contains(rel, "image_src") || (strings.Contains(rel, "preload") && as == "image") {
				add(a["href"], "image", false)
			} else if strings.Contains(rel, "preload") && (as == "video" || as == "audio") {
				add(a["href"], as, false)
			}
		}
	}

	var walkJSON func(any, string)
	walkJSON = func(v any, parentKey string) {
		switch x := v.(type) {
		case map[string]any:
			for k, val := range x {
				lk := strings.ToLower(k)
				switch y := val.(type) {
				case string:
					switch lk {
					case "contenturl":
						add(y, "", false)
					case "thumbnailurl", "image":
						add(y, "image", false)
					case "video":
						add(y, "video", false)
					case "embedurl":
						add(y, "video", true)
					}
				default:
					walkJSON(val, lk)
				}
			}
		case []any:
			for _, item := range x {
				walkJSON(item, parentKey)
			}
		case string:
			if parentKey == "image" || parentKey == "thumbnailurl" {
				add(x, "image", false)
			}
		}
	}
	for _, m := range jsonLDREV860.FindAllSubmatch(body, -1) {
		if len(m) < 2 {
			continue
		}
		var v any
		if json.Unmarshal(m[1], &v) == nil {
			walkJSON(v, "")
		}
	}

	items := make([]RemoteItem, 0, len(found))
	for i, f := range found {
		u, _ := url.Parse(f.URL)
		name := path.Base(u.Path)
		if name == "" || name == "." || name == "/" {
			name = fmt.Sprintf("web-media-%03d", i+1)
		}
		if decoded, err := url.PathUnescape(name); err == nil {
			name = decoded
		}
		ct := contentTypeForMediaExtV860(mediaExtV860(f.URL))
		if ct == "" && f.Kind != "" {
			ct = f.Kind + "/unknown"
		}
		items = append(items, RemoteItem{
			ID:          i + 1,
			Path:        name,
			Name:        name,
			Size:        -1,
			Source:      "HTML",
			URL:         pageURL,
			DirectURL:   f.URL,
			ContentType: ct,
			ApproxSize:  true,
		})
	}
	return items
}

func (a *App) probeHTMLMediaV860(ctx context.Context, pageURL string) ([]RemoteItem, error) {
	u, err := url.Parse(strings.TrimSpace(pageURL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, errors.New("URL HTML invalid")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 DuplicateDownloadGuard/8.6")
	req.Header.Set("Accept", "text/html,application/xhtml+xml;q=0.9,*/*;q=0.5")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return nil, fmt.Errorf("pagina a răspuns HTTP %d", resp.StatusCode)
	}
	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	if ct != "" && !strings.Contains(ct, "text/html") && !strings.Contains(ct, "application/xhtml") {
		return nil, errors.New("URL-ul nu este o pagină HTML")
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxHTMLDiscoveryBytesV860+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxHTMLDiscoveryBytesV860 {
		return nil, fmt.Errorf("pagina HTML depășește limita de analiză de %d MB", maxHTMLDiscoveryBytesV860>>20)
	}
	finalURL := pageURL
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL.String()
	}
	items := extractHTMLMediaV860(finalURL, b)
	if len(items) == 0 {
		return nil, errors.New("pagina nu conține media publică detectabilă în HTML")
	}
	return items, nil
}
