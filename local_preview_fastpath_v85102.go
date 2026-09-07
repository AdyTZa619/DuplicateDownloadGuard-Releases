package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// TEST v8.5.102: LOCAL preview must not wait behind App.mu. The diagnostic
// evidence from TEST .101 showed auth waits of 6.7s and 29.7s even though
// os.Stat/read took milliseconds. Preview authorization therefore uses a small
// config-root snapshot that is independent from the global application mutex.
// If a path is not covered by the configured roots, TryRLock provides a safe
// exact-index fallback without ever blocking the media request.

type localPreviewRootSnapshotV85102 struct {
	mu     sync.Mutex
	stamp  int64
	size   int64
	loaded bool
	roots  []string
}

var localPreviewRootSnapshotsV85102 sync.Map // map[*App]*localPreviewRootSnapshotV85102

// LOCAL preview diagnostics must never hold an HTTP response open while the
// shared App mutex is busy. Queue journal writes onto one worker instead of
// calling a.logf synchronously from media/trace handlers.
type localPreviewLogEntryV85102 struct {
	a      *App
	format string
	args   []any
}

var localPreviewLogStateV85102 = struct {
	once sync.Once
	q    chan localPreviewLogEntryV85102
}{q: make(chan localPreviewLogEntryV85102, 2048)}

func startLocalPreviewLogWorkerV85102() {
	go func() {
		for entry := range localPreviewLogStateV85102.q {
			if entry.a != nil {
				entry.a.logf(entry.format, entry.args...)
			}
		}
	}()
}

func localPreviewLogfV85102(a *App, format string, args ...any) {
	if a == nil || strings.TrimSpace(format) == "" {
		return
	}
	localPreviewLogStateV85102.once.Do(startLocalPreviewLogWorkerV85102)
	copyArgs := append([]any(nil), args...)
	select {
	case localPreviewLogStateV85102.q <- localPreviewLogEntryV85102{a: a, format: format, args: copyArgs}:
	default:
		// Never block preview for diagnostics. stderr is not available in the
		// windowsgui build, so a full diagnostic queue is simply dropped.
		_ = fmt.Sprintf(format, args...)
	}
}

func localPreviewRootStateV85102(a *App) *localPreviewRootSnapshotV85102 {
	if raw, ok := localPreviewRootSnapshotsV85102.Load(a); ok {
		return raw.(*localPreviewRootSnapshotV85102)
	}
	st := &localPreviewRootSnapshotV85102{}
	actual, _ := localPreviewRootSnapshotsV85102.LoadOrStore(a, st)
	return actual.(*localPreviewRootSnapshotV85102)
}

func normalizeLocalPreviewRootsV85102(paths []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(paths))
	for _, raw := range paths {
		p := strings.TrimSpace(raw)
		if p == "" {
			continue
		}
		if abs, err := filepath.Abs(filepath.Clean(p)); err == nil {
			p = abs
		} else {
			p = filepath.Clean(p)
		}
		key := pathKey(p)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, p)
	}
	return out
}

func (a *App) localPreviewRootsV85102() []string {
	if a == nil {
		return nil
	}
	st := localPreviewRootStateV85102(a)
	configPath := a.configPath()
	info, statErr := os.Stat(configPath)
	stamp, size := int64(-1), int64(-1)
	if statErr == nil {
		stamp = info.ModTime().UnixNano()
		size = info.Size()
	}

	st.mu.Lock()
	defer st.mu.Unlock()
	if st.loaded && st.stamp == stamp && st.size == size {
		return append([]string(nil), st.roots...)
	}

	type diskConfig struct {
		LocalPaths  []string `json:"localPaths"`
		DownloadDir string   `json:"downloadDir"`
	}
	var cfg diskConfig
	if b, err := os.ReadFile(configPath); err == nil {
		_ = json.Unmarshal(b, &cfg)
	}
	paths := append([]string(nil), cfg.LocalPaths...)
	paths = append(paths, cfg.DownloadDir)
	if strings.TrimSpace(cfg.DownloadDir) == "" {
		paths = append(paths, portableDownloadsDir())
	}
	// appDir is always trusted for DDG-owned temporary/generated media.
	paths = append(paths, a.appDir)
	st.roots = normalizeLocalPreviewRootsV85102(paths)
	st.stamp = stamp
	st.size = size
	st.loaded = true
	return append([]string(nil), st.roots...)
}

func (a *App) localPreviewPathAllowedV85102(p string) bool {
	p = strings.TrimSpace(p)
	if a == nil || p == "" {
		return false
	}
	ap, err := filepath.Abs(filepath.Clean(p))
	if err != nil {
		return false
	}
	for _, root := range a.localPreviewRootsV85102() {
		if isUnder(ap, root) {
			return true
		}
	}

	// Exact fallback for legacy index entries outside current roots. Crucially,
	// this never waits for a long writer on App.mu.
	if !a.mu.TryRLock() {
		return false
	}
	defer a.mu.RUnlock()
	if _, ok := a.index[ap]; ok {
		return true
	}
	if _, ok := a.index[p]; ok {
		return true
	}
	for _, x := range a.results {
		if x.LocalPath == p || x.LocalPath == ap {
			return true
		}
	}
	return false
}

// Dedicated LOCAL media origin. The main DDG origin also carries heartbeat,
// logs and API calls that can all queue behind App.mu. Chromium/WebView limits
// connections per origin, so LOCAL image bytes get their own loopback listener
// just like the MEGA media path already does.
type localPreviewDedicatedStateV85102 struct {
	once sync.Once
	base string
	err  error
}

var localPreviewDedicatedStatesV85102 sync.Map // map[*App]*localPreviewDedicatedStateV85102

func localPreviewDedicatedStateForV85102(a *App) *localPreviewDedicatedStateV85102 {
	if raw, ok := localPreviewDedicatedStatesV85102.Load(a); ok {
		return raw.(*localPreviewDedicatedStateV85102)
	}
	st := &localPreviewDedicatedStateV85102{}
	actual, _ := localPreviewDedicatedStatesV85102.LoadOrStore(a, st)
	return actual.(*localPreviewDedicatedStateV85102)
}

func (a *App) ensureLocalPreviewDedicatedV85102() (string, error) {
	if a == nil {
		return "", errors.New("aplicație indisponibilă")
	}
	st := localPreviewDedicatedStateForV85102(a)
	st.once.Do(func() {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			st.err = err
			return
		}
		mux := http.NewServeMux()
		mux.HandleFunc("/api/local-preview", a.handleLocalPreviewDiagV8599)
		mux.HandleFunc("/api/local-preview-buffered", a.handleLocalPreviewBufferedV8599)
		h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Timing-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Origin", "*")
			mux.ServeHTTP(w, r)
		})
		srv := &http.Server{Handler: h}
		st.base = "http://" + ln.Addr().String()
		go func() {
			_ = srv.Serve(ln)
		}()
		localPreviewLogfV85102(a, "LOCAL Preview media listener dedicat: %s", st.base)
	})
	return st.base, st.err
}

func (a *App) handleLocalPreviewBaseV85102(w http.ResponseWriter, r *http.Request) {
	base, err := a.ensureLocalPreviewDedicatedV85102()
	if err != nil || strings.TrimSpace(base) == "" {
		if err == nil {
			err = errors.New("listener LOCAL indisponibil")
		}
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	jsonOut(w, map[string]any{"ok": true, "base": base, "mode": "dedicated-buffered-image"})
}
