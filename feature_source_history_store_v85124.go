package main

import (
	"encoding/json"
	"errors"
	"hash/fnv"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const sourceHistoryServiceV85124 = "ddg-source-history-v1"
const sourceHistoryPortBaseV85124 = 38650
const sourceHistoryPortSpanV85124 = 200
const sourceHistoryPortAttemptsV85124 = 8
const sourceHistoryMaxLinksV85124 = 1000
const sourceHistoryMaxSnapshotsV85124 = 20

type sourceHistorySnapshotV85124 struct {
	At                int64  `json:"at"`
	Revision          uint64 `json:"revision,omitempty"`
	Total             int    `json:"total"`
	Local             int    `json:"local"`
	Missing           int    `json:"missing"`
	Review            int    `json:"review"`
	Manual            int    `json:"manual"`
	InternalCompleted int    `json:"internalCompleted,omitempty"`
}

type sourceHistoryEntryV85124 struct {
	URL       string                        `json:"url"`
	Key       string                        `json:"key"`
	FirstAt   int64                         `json:"firstAt"`
	LastAt    int64                         `json:"lastAt"`
	Checks    int                           `json:"checks"`
	Snapshots []sourceHistorySnapshotV85124 `json:"snapshots"`
}

type sourceHistoryStoreV85124 struct {
	Version int                                 `json:"version"`
	Links   map[string]sourceHistoryEntryV85124 `json:"links"`
}

type sourceHistorySnapshotRequestV85124 struct {
	URL               string `json:"url"`
	Revision          uint64 `json:"revision,omitempty"`
	Total             int    `json:"total"`
	Local             int    `json:"local"`
	Missing           int    `json:"missing"`
	Review            int    `json:"review"`
	Manual            int    `json:"manual"`
	InternalCompleted int    `json:"internalCompleted,omitempty"`
}

type sourceHistoryServiceStateV85124 struct {
	mu       sync.Mutex
	path     string
	store    sourceHistoryStoreV85124
	loaded   bool
	basePort int
	port     int
}

var sourceHistoryStateV85124 sourceHistoryServiceStateV85124

func init() {
	base := filepath.Base(os.Args[0])
	if strings.Contains(strings.ToLower(base), ".test") {
		return
	}
	go startSourceHistoryServiceV85124()
}

func sourceHistoryDataDirV85124() string {
	dir, err := portableDataDir()
	if err == nil && strings.TrimSpace(dir) != "" {
		return filepath.Clean(dir)
	}
	return filepath.Clean(filepath.Join(executableDir(), "data"))
}

func sourceHistoryPortSeedV85124(dataDir string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.ToLower(filepath.Clean(dataDir))))
	return sourceHistoryPortBaseV85124 + int(h.Sum32()%sourceHistoryPortSpanV85124)
}

func startSourceHistoryServiceV85124() {
	dataDir := sourceHistoryDataDirV85124()
	_ = os.MkdirAll(dataDir, 0755)
	base := sourceHistoryPortSeedV85124(dataDir)

	var ln net.Listener
	var err error
	for i := 0; i < sourceHistoryPortAttemptsV85124; i++ {
		port := base + i
		ln, err = net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
		if err != nil {
			if sourceHistoryExistingServiceV85124(port) {
				return
			}
			continue
		}
		sourceHistoryStateV85124.mu.Lock()
		sourceHistoryStateV85124.path = filepath.Join(dataDir, "source_history.json")
		sourceHistoryStateV85124.basePort = base
		sourceHistoryStateV85124.port = port
		sourceHistoryStateV85124.mu.Unlock()
		break
	}
	if err != nil || ln == nil {
		return
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", sourceHistoryHealthV85124)
	mux.HandleFunc("/history", sourceHistoryGetV85124)
	mux.HandleFunc("/snapshot", sourceHistorySnapshotV85124Handler)
	server := &http.Server{
		Handler:           sourceHistoryCORSV85124(mux),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	_ = server.Serve(ln)
}

func sourceHistoryExistingServiceV85124(port int) bool {
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
	return reply.OK && reply.Service == sourceHistoryServiceV85124
}

func sourceHistoryCORSV85124(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := strings.TrimSpace(r.Header.Get("Origin"))
		if origin != "" {
			u, err := url.Parse(origin)
			host := ""
			if err == nil {
				host = strings.ToLower(u.Hostname())
			}
			if host != "127.0.0.1" && host != "localhost" && host != "::1" {
				http.Error(w, "origine source-history refuzată", http.StatusForbidden)
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

func sourceHistoryHealthV85124(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "GET necesar", http.StatusMethodNotAllowed)
		return
	}
	sourceHistoryStateV85124.mu.Lock()
	port, base := sourceHistoryStateV85124.port, sourceHistoryStateV85124.basePort
	sourceHistoryStateV85124.mu.Unlock()
	jsonOut(w, map[string]any{"ok": true, "service": sourceHistoryServiceV85124, "version": 1, "port": port, "basePort": base})
}

func sourceHistoryGetV85124(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "GET necesar", http.StatusMethodNotAllowed)
		return
	}
	raw := strings.TrimSpace(r.URL.Query().Get("url"))
	key, err := canonicalSourceURLV85124(raw)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	sourceHistoryStateV85124.mu.Lock()
	defer sourceHistoryStateV85124.mu.Unlock()
	_ = sourceHistoryLoadLockedV85124()
	entry, ok := sourceHistoryStateV85124.store.Links[key]
	jsonOut(w, map[string]any{"ok": true, "known": ok, "key": key, "entry": func() any {
		if ok {
			return entry
		}
		return nil
	}()})
}

func sourceHistorySnapshotV85124Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST necesar", http.StatusMethodNotAllowed)
		return
	}
	var req sourceHistorySnapshotRequestV85124
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	key, err := canonicalSourceURLV85124(req.URL)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Total <= 0 {
		http.Error(w, "total invalid", http.StatusBadRequest)
		return
	}
	req.Local = clampSourceHistoryCountV85124(req.Local, req.Total)
	req.Missing = clampSourceHistoryCountV85124(req.Missing, req.Total)
	req.Review = clampSourceHistoryCountV85124(req.Review, req.Total)
	req.Manual = clampSourceHistoryCountV85124(req.Manual, req.Total)
	if req.InternalCompleted < 0 {
		req.InternalCompleted = 0
	}

	now := time.Now().UnixMilli()
	snap := sourceHistorySnapshotV85124{
		At: now, Revision: req.Revision, Total: req.Total, Local: req.Local,
		Missing: req.Missing, Review: req.Review, Manual: req.Manual,
		InternalCompleted: req.InternalCompleted,
	}

	sourceHistoryStateV85124.mu.Lock()
	defer sourceHistoryStateV85124.mu.Unlock()
	_ = sourceHistoryLoadLockedV85124()
	entry, exists := sourceHistoryStateV85124.store.Links[key]
	if !exists {
		entry = sourceHistoryEntryV85124{URL: strings.TrimSpace(req.URL), Key: key, FirstAt: now, Snapshots: []sourceHistorySnapshotV85124{}}
	}
	entry.URL = strings.TrimSpace(req.URL)
	entry.Key = key
	entry.LastAt = now

	duplicate := false
	if n := len(entry.Snapshots); n > 0 {
		last := entry.Snapshots[n-1]
		if req.Revision > 0 && last.Revision == req.Revision {
			entry.Snapshots[n-1] = snap
			duplicate = true
		} else if req.Revision == 0 && now-last.At < 30_000 && sourceHistorySameCountsV85124(last, snap) {
			entry.Snapshots[n-1] = snap
			duplicate = true
		}
	}
	if !duplicate {
		entry.Checks++
		entry.Snapshots = append(entry.Snapshots, snap)
	}
	if entry.Checks <= 0 {
		entry.Checks = 1
	}
	if len(entry.Snapshots) > sourceHistoryMaxSnapshotsV85124 {
		entry.Snapshots = append([]sourceHistorySnapshotV85124(nil), entry.Snapshots[len(entry.Snapshots)-sourceHistoryMaxSnapshotsV85124:]...)
	}
	sourceHistoryStateV85124.store.Links[key] = entry
	sourceHistoryTrimLockedV85124()
	if err := sourceHistorySaveLockedV85124(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOut(w, map[string]any{"ok": true, "known": true, "deduplicated": duplicate, "entry": entry})
}

func sourceHistoryLoadLockedV85124() error {
	if sourceHistoryStateV85124.loaded {
		return nil
	}
	sourceHistoryStateV85124.loaded = true
	sourceHistoryStateV85124.store = sourceHistoryStoreV85124{Version: 1, Links: map[string]sourceHistoryEntryV85124{}}
	b, err := os.ReadFile(sourceHistoryStateV85124.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var store sourceHistoryStoreV85124
	if err := json.Unmarshal(b, &store); err != nil {
		return err
	}
	if store.Links == nil {
		store.Links = map[string]sourceHistoryEntryV85124{}
	}
	if store.Version <= 0 {
		store.Version = 1
	}
	sourceHistoryStateV85124.store = store
	return nil
}

func sourceHistorySaveLockedV85124() error {
	if sourceHistoryStateV85124.path == "" {
		return errors.New("cale source_history indisponibilă")
	}
	if err := os.MkdirAll(filepath.Dir(sourceHistoryStateV85124.path), 0755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(sourceHistoryStateV85124.store, "", "  ")
	if err != nil {
		return err
	}
	tmp := sourceHistoryStateV85124.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0644); err != nil {
		return err
	}
	if err := os.Rename(tmp, sourceHistoryStateV85124.path); err == nil {
		return nil
	}
	_ = os.Remove(sourceHistoryStateV85124.path)
	return os.Rename(tmp, sourceHistoryStateV85124.path)
}

func sourceHistoryTrimLockedV85124() {
	if len(sourceHistoryStateV85124.store.Links) <= sourceHistoryMaxLinksV85124 {
		return
	}
	type pair struct {
		Key string
		At  int64
	}
	rows := make([]pair, 0, len(sourceHistoryStateV85124.store.Links))
	for key, entry := range sourceHistoryStateV85124.store.Links {
		rows = append(rows, pair{Key: key, At: entry.LastAt})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].At > rows[j].At })
	for _, row := range rows[sourceHistoryMaxLinksV85124:] {
		delete(sourceHistoryStateV85124.store.Links, row.Key)
	}
}

func sourceHistorySameCountsV85124(a, b sourceHistorySnapshotV85124) bool {
	return a.Total == b.Total && a.Local == b.Local && a.Missing == b.Missing && a.Review == b.Review && a.Manual == b.Manual && a.InternalCompleted == b.InternalCompleted
}

func clampSourceHistoryCountV85124(v, total int) int {
	if v < 0 {
		return 0
	}
	if v > total {
		return total
	}
	return v
}

func canonicalSourceURLV85124(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("URL lipsă")
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return "", errors.New("URL invalid")
	}
	u.Scheme = strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Hostname())
	host = strings.TrimPrefix(host, "www.")
	port := u.Port()
	if (u.Scheme == "https" && port == "443") || (u.Scheme == "http" && port == "80") {
		port = ""
	}
	if port != "" {
		u.Host = net.JoinHostPort(host, port)
	} else {
		u.Host = host
	}
	isMega := host == "mega.nz" || host == "mega.co.nz" || strings.HasSuffix(host, ".mega.nz") || strings.HasSuffix(host, ".mega.co.nz")
	if !isMega {
		u.Fragment = ""
	}
	q := u.Query()
	for key := range q {
		lower := strings.ToLower(key)
		if strings.HasPrefix(lower, "utm_") || sourceHistoryTrackingKeyV85124(lower) {
			q.Del(key)
		}
	}
	u.RawQuery = q.Encode()
	if len(u.Path) > 1 {
		u.Path = strings.TrimRight(u.Path, "/")
	}
	return u.String(), nil
}

func sourceHistoryTrackingKeyV85124(key string) bool {
	switch key {
	case "fbclid", "gclid", "dclid", "msclkid", "mc_cid", "mc_eid":
		return true
	default:
		return false
	}
}
