package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	localThumbMaxBytesV8574 = int64(256 << 20) // 256 MiB persistent cache cap
	localThumbWidthV8574    = 1280
	localThumbHeightV8574   = 960
)

type localThumbCallV8574 struct {
	done chan struct{}
	err  error
}

var (
	localThumbMuV8574       sync.Mutex
	localThumbInflightV8574 = map[string]*localThumbCallV8574{}
	localThumbCleanupV8574  atomic.Bool
)

func localPreviewKindV8574(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".jpg", ".jpeg", ".jpe", ".jfif", ".png", ".gif", ".webp", ".bmp", ".avif", ".heic", ".heif", ".tif", ".tiff":
		return "image"
	case ".mp4", ".webm", ".ogv", ".mov", ".m4v", ".mkv", ".avi", ".flv", ".ts", ".mts", ".m2ts":
		return "video"
	default:
		return "other"
	}
}

func localThumbCacheDirV8574(a *App) string {
	return filepath.Join(a.appDir, "data", "preview-cache-v1")
}

func localThumbKeyV8574(path string, st os.FileInfo) string {
	clean := strings.ToLower(filepath.Clean(path))
	s := fmt.Sprintf("%s\n%d\n%d\n%d\n%d", clean, st.Size(), st.ModTime().UnixNano(), localThumbWidthV8574, localThumbHeightV8574)
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func localThumbPathV8574(a *App, key string) string {
	return filepath.Join(localThumbCacheDirV8574(a), key+".jpg")
}

func localThumbServeV8574(w http.ResponseWriter, r *http.Request, path, etag string) {
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Content-Disposition", "inline")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.Header().Set("ETag", `"`+etag+`"`)
	if strings.TrimSpace(r.Header.Get("If-None-Match")) == `"`+etag+`"` {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	http.ServeFile(w, r, path)
}

func localThumbFFmpegV8574(path, kind string) ([]byte, error) {
	ff := fallbackFFmpegV85()
	if strings.TrimSpace(ff) == "" {
		return nil, fmt.Errorf("FFmpeg nu este disponibil pentru thumbnail local")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin"}
	if kind == "video" {
		// Input-side seek keeps poster extraction quick even for large files.
		args = append(args, "-ss", "0.35")
	}
	args = append(args,
		"-i", path,
		"-map", "0:v:0",
		"-frames:v", "1",
		"-vf", fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease:force_divisible_by=2", localThumbWidthV8574, localThumbHeightV8574),
		"-q:v", "4",
		"-f", "image2pipe",
		"-vcodec", "mjpeg",
		"pipe:1",
	)
	cmd := exec.CommandContext(ctx, ff, args...)
	hideChildWindow(cmd)
	out, err := cmd.Output()
	if ctx.Err() != nil {
		return nil, fmt.Errorf("thumbnail local a depășit 12 secunde: %w", ctx.Err())
	}
	if err != nil {
		return nil, fmt.Errorf("FFmpeg thumbnail local: %w", err)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("FFmpeg nu a produs thumbnail")
	}
	return out, nil
}

func ensureLocalThumbV8574(a *App, sourcePath, cachePath, key, kind string) error {
	if st, err := os.Stat(cachePath); err == nil && !st.IsDir() && st.Size() > 0 {
		return nil
	}

	localThumbMuV8574.Lock()
	if call, ok := localThumbInflightV8574[key]; ok {
		localThumbMuV8574.Unlock()
		<-call.done
		return call.err
	}
	call := &localThumbCallV8574{done: make(chan struct{})}
	localThumbInflightV8574[key] = call
	localThumbMuV8574.Unlock()

	defer func() {
		localThumbMuV8574.Lock()
		delete(localThumbInflightV8574, key)
		close(call.done)
		localThumbMuV8574.Unlock()
	}()

	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		call.err = err
		return err
	}
	data, err := localThumbFFmpegV8574(sourcePath, kind)
	if err != nil {
		call.err = err
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(cachePath), key+"-*.tmp")
	if err != nil {
		call.err = err
		return err
	}
	tmpName := tmp.Name()
	renamed := false
	defer func() {
		_ = tmp.Close()
		if !renamed {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err = tmp.Write(data); err != nil {
		call.err = err
		return err
	}
	if err = tmp.Close(); err != nil {
		call.err = err
		return err
	}
	if err = os.Rename(tmpName, cachePath); err != nil {
		// Same-key requests are deduplicated in-process. Still accept a valid
		// cache entry if an external process happened to create it first.
		if st, statErr := os.Stat(cachePath); statErr != nil || st.IsDir() || st.Size() == 0 {
			call.err = err
			return err
		}
		_ = os.Remove(tmpName)
	} else {
		renamed = true
	}
	call.err = nil
	go cleanupLocalThumbCacheV8574(localThumbCacheDirV8574(a), localThumbMaxBytesV8574)
	return nil
}

func cleanupLocalThumbCacheV8574(dir string, maxBytes int64) {
	if !localThumbCleanupV8574.CompareAndSwap(false, true) {
		return
	}
	defer localThumbCleanupV8574.Store(false)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	type item struct {
		path string
		size int64
		when time.Time
	}
	items := make([]item, 0, len(entries))
	var total int64
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".jpg") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		items = append(items, item{path: filepath.Join(dir, entry.Name()), size: info.Size(), when: info.ModTime()})
		total += info.Size()
	}
	if total <= maxBytes {
		return
	}
	sort.Slice(items, func(i, j int) bool { return items[i].when.Before(items[j].when) })
	for _, it := range items {
		if total <= maxBytes {
			break
		}
		if os.Remove(it.path) == nil {
			total -= it.size
		}
	}
}

func (a *App) handleLocalThumbV8574(w http.ResponseWriter, r *http.Request) {
	p := strings.TrimSpace(r.URL.Query().Get("path"))
	if p == "" {
		http.Error(w, "path lipsă", http.StatusBadRequest)
		return
	}
	if !a.localPathAllowed(p) {
		http.Error(w, "Fișier local neautorizat", http.StatusForbidden)
		return
	}
	st, err := os.Stat(p)
	if err != nil || st.IsDir() {
		http.Error(w, "Fișierul local nu mai există", http.StatusNotFound)
		return
	}
	kind := localPreviewKindV8574(p)
	if kind != "image" && kind != "video" {
		http.Error(w, "Thumbnail disponibil doar pentru imagine/video", http.StatusUnsupportedMediaType)
		return
	}
	key := localThumbKeyV8574(p, st)
	cachePath := localThumbPathV8574(a, key)
	if err := ensureLocalThumbV8574(a, p, cachePath, key, kind); err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	localThumbServeV8574(w, r, cachePath, key)
}
