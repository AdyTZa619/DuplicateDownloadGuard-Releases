package main

import (
	"fmt"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// TEST95: local preview must never wait behind App.mu. The old preview
// authorization path took the global application read lock and could therefore
// stall behind long-running result/index writers even though the file itself is
// local and already known to the UI. This endpoint is deliberately isolated
// from MEGA/JDownloader/index work.
var localPreviewExtV8577 = map[string]bool{
	".jpg": true, ".jpeg": true, ".jpe": true, ".jfif": true,
	".png": true, ".gif": true, ".webp": true, ".bmp": true,
	".avif": true, ".heic": true, ".heif": true, ".tif": true, ".tiff": true,
	".mp4": true, ".m4v": true, ".webm": true, ".mkv": true, ".avi": true,
	".mov": true, ".wmv": true, ".flv": true, ".ts": true, ".m2ts": true,
	".mts": true, ".mpeg": true, ".mpg": true,
	".mp3": true, ".m4a": true, ".aac": true, ".ogg": true, ".opus": true,
	".wav": true, ".flac": true, ".wma": true,
}

func localPreviewSameOriginV8577(r *http.Request) bool {
	host := strings.TrimSpace(r.RemoteAddr)
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return false
	}

	if site := strings.ToLower(strings.TrimSpace(r.Header.Get("Sec-Fetch-Site"))); site != "" && site != "same-origin" {
		return false
	}
	if ref := strings.TrimSpace(r.Referer()); ref != "" {
		u, err := url.Parse(ref)
		if err != nil || !strings.EqualFold(u.Host, r.Host) {
			return false
		}
	}
	return true
}

func localPreviewContentTypeV8577(ext string) string {
	if t := mime.TypeByExtension(ext); strings.TrimSpace(t) != "" {
		return t
	}
	switch ext {
	case ".jpg", ".jpeg", ".jpe", ".jfif":
		return "image/jpeg"
	case ".webp":
		return "image/webp"
	case ".avif":
		return "image/avif"
	case ".heic", ".heif":
		return "image/heif"
	case ".mkv":
		return "video/x-matroska"
	case ".m2ts", ".mts", ".ts":
		return "video/mp2t"
	case ".flac":
		return "audio/flac"
	case ".opus":
		return "audio/ogg"
	default:
		return "application/octet-stream"
	}
}

func (a *App) handleLocalPreviewFastV8577(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	if !localPreviewSameOriginV8577(r) {
		http.Error(w, "Preview local permis doar din interfața DDG", http.StatusForbidden)
		return
	}

	p := strings.TrimSpace(r.URL.Query().Get("path"))
	if p == "" {
		http.Error(w, "path lipsă", http.StatusBadRequest)
		return
	}
	ap, err := filepath.Abs(filepath.Clean(p))
	if err != nil {
		http.Error(w, "path local invalid", http.StatusBadRequest)
		return
	}
	ext := strings.ToLower(filepath.Ext(ap))
	if !localPreviewExtV8577[ext] {
		http.Error(w, "Format fără preview local", http.StatusUnsupportedMediaType)
		return
	}

	// Open + Stat directly. No App.mu, no result scan, no FFmpeg, no MEGA.
	f, err := os.Open(ap)
	if err != nil {
		http.Error(w, "Fișierul local nu mai există", http.StatusNotFound)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || st.IsDir() {
		http.Error(w, "Fișierul local nu mai există", http.StatusNotFound)
		return
	}

	etag := fmt.Sprintf("\"ddg-%x-%x\"", st.Size(), st.ModTime().UnixNano())
	w.Header().Set("Content-Type", localPreviewContentTypeV8577(ext))
	w.Header().Set("Content-Disposition", "inline")
	w.Header().Set("Cache-Control", "private, max-age=300")
	w.Header().Set("ETag", etag)
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-DDG-Local-Preview", "lockfree-v8577")
	w.Header().Set("Server-Timing", fmt.Sprintf("ddg-open;dur=%.2f", float64(time.Since(started).Microseconds())/1000.0))
	if strings.TrimSpace(r.Header.Get("If-None-Match")) == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	http.ServeContent(w, r, st.Name(), st.ModTime(), f)
}
