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

const sourceHistoryServiceV85117 = "ddg-source-history-v1"
const sourceHistoryPortBaseV85117 = 38650
const sourceHistoryPortSpanV85117 = 200
const sourceHistoryPortAttemptsV85117 = 8
const sourceHistoryMaxLinksV85117 = 1000
const sourceHistoryMaxSnapshotsV85117 = 20

type sourceHistorySnapshotV85117 struct {
	At                int64  `json:"at"`
	Revision          uint64 `json:"revision,omitempty"`
	Total             int    `json:"total"`
	Local             int    `json:"local"`
	Missing           int    `json:"missing"`
	Review            int    `json:"review"`
	Manual            int    `json:"manual"`
	InternalCompleted int    `json:"internalCompleted,omitempty"`
}

type sourceHistoryEntryV85117 struct {
	URL       string                        `json:"url"`
	Key       string                        `json:"key"`
	FirstAt   int64                         `json:"firstAt"`
	LastAt    int64                         `json:"lastAt"`
	Checks    int                           `json:"checks"`
	Snapshots []sourceHistorySnapshotV85117 `json:"snapshots"`
}

type sourceHistoryStoreV85117 struct {
	Version int                                 `json:"version"`
	Links   map[string]sourceHistoryEntryV85117 `json:"links"`
}

type sourceHistorySnapshotRequestV85117 struct {
	URL               string `json:"url"`
	Revision          uint64 `json:"revision,omitempty"`
	Total             int    `json:"total"`
	Local             int    `json:"local"`
	Missing           int    `json:"missing"`
	Review            int    `json:"review"`
	Manual            int    `json:"manual"`
	InternalCompleted int    `json:"internalCompleted,omitempty"`
}

type sourceHistoryServiceStateV85117 struct {
	mu       sync.Mutex
	path     string
	store    sourceHistoryStoreV85117
	loaded   bool
	basePort int
	port     int
}

var sourceHistoryStateV85117 sourceHistoryServiceStateV85117

func init() {
	base := filepath.Base(os.Args[0])
	if strings.Contains(strings.ToLower(base), ".test") {
		return
	}
	go startSourceHistoryServiceV85117()
}

func sourceHistoryDataDirV85117() string {
	dir, err := portableDataDir()
	if err == nil && strings.TrimSpace(dir) != "" {
		return filepath.Clean(dir)
	}
	return filepath.Clean(filepath.Join(executableDir(), "data"))
}

func sourceHistoryPortSeedV85117(dataDir string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.ToLower(filepath.Clean(dataDir))))
	return sourceHistoryPortBaseV85117 + int(h.Sum32()%sourceHistoryPortSpanV85117)
}

func startSourceHistoryServiceV85117() {
	dataDir := sourceHistoryDataDirV85117()
	_ = os.MkdirAll(dataDir, 0755)
	base := sourceHistoryPortSeedV85117(dataDir)

	var ln net.Listener
	var err error
	for i := 0; i < sourceHistoryPortAttemptsV85117; i++ {
		port := base + i
		ln, err = net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
		if err != nil {
			if sourceHistoryExistingServiceV85117(port) {
				return
			}
			continue
		}
		sourceHistoryStateV85117.mu.Lock()
		sourceHistoryStateV85117.path = filepath.Join(dataDir, "source_history.json")
		sourceHistoryStateV85117.basePort = base
		sourceHistoryStateV85117.port = port
		sourceHistoryStateV85117.mu.Unlock()
		break
	}
	if err != nil || ln == nil {
		return
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", sourceHistoryHealthV85117)
	mux.HandleFunc("/history", sourceHistoryGetV85117)
	mux.HandleFunc("/snapshot", sourceHistorySnapshotV85117Handler)
	server := &http.Server{
		Handler:           sourceHistoryCORSV85117(mux),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	_ = server.Serve(ln)
}

func sourceHistoryExistingServiceV85117(port int) bool {
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
	return reply.OK && reply.Service == sourceHistoryServiceV85117
}

func sourceHistoryCORSV85117(next http.Handler) http.Handler {
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

func sourceHistoryHealthV85117(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "GET necesar", http.StatusMethodNotAllowed)
		return
	}
	sourceHistoryStateV85117.mu.Lock()
	port, base := sourceHistoryStateV85117.port, sourceHistoryStateV85117.basePort
	sourceHistoryStateV85117.mu.Unlock()
	jsonOut(w, map[string]any{"ok": true, "service": sourceHistoryServiceV85117, "version": 1, "port": port, "basePort": base})
}

func sourceHistoryGetV85117(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "GET necesar", http.StatusMethodNotAllowed)
		return
	}
	raw := strings.TrimSpace(r.URL.Query().Get("url"))
	key, err := canonicalSourceURLV85117(raw)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	sourceHistoryStateV85117.mu.Lock()
	defer sourceHistoryStateV85117.mu.Unlock()
	_ = sourceHistoryLoadLockedV85117()
	entry, ok := sourceHistoryStateV85117.store.Links[key]
	jsonOut(w, map[string]any{"ok": true, "known": ok, "key": key, "entry": func() any {
		if ok {
			return entry
		}
		return nil
	}()})
}

func sourceHistorySnapshotV85117Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST necesar", http.StatusMethodNotAllowed)
		return
	}
	var req sourceHistorySnapshotRequestV85117
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	key, err := canonicalSourceURLV85117(req.URL)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Total <= 0 {
		http.Error(w, "total invalid", http.StatusBadRequest)
		return
	}
	req.Local = clampSourceHistoryCountV85117(req.Local, req.Total)
	req.Missing = clampSourceHistoryCountV85117(req.Missing, req.Total)
	req.Review = clampSourceHistoryCountV85117(req.Review, req.Total)
	req.Manual = clampSourceHistoryCountV85117(req.Manual, req.Total)
	if req.InternalCompleted < 0 {
		req.InternalCompleted = 0
	}

	now := time.Now().UnixMilli()
	snap := sourceHistorySnapshotV85117{
		At: now, Revision: req.Revision, Total: req.Total, Local: req.Local,
		Missing: req.Missing, Review: req.Review, Manual: req.Manual,
		InternalCompleted: req.InternalCompleted,
	}

	sourceHistoryStateV85117.mu.Lock()
	defer sourceHistoryStateV85117.mu.Unlock()
	_ = sourceHistoryLoadLockedV85117()
	entry, exists := sourceHistoryStateV85117.store.Links[key]
	if !exists {
		entry = sourceHistoryEntryV85117{URL: strings.TrimSpace(req.URL), Key: key, FirstAt: now, Snapshots: []sourceHistorySnapshotV85117{}}
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
		} else if req.Revision == 0 && now-last.At < 30_000 && sourceHistorySameCountsV85117(last, snap) {
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
	if len(entry.Snapshots) > sourceHistoryMaxSnapshotsV85117 {
		entry.Snapshots = append([]sourceHistorySnapshotV85117(nil), entry.Snapshots[len(entry.Snapshots)-sourceHistoryMaxSnapshotsV85117:]...)
	}
	sourceHistoryStateV85117.store.Links[key] = entry
	sourceHistoryTrimLockedV85117()
	if err := sourceHistorySaveLockedV85117(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOut(w, map[string]any{"ok": true, "known": true, "deduplicated": duplicate, "entry": entry})
}

func sourceHistoryLoadLockedV85117() error {
	if sourceHistoryStateV85117.loaded {
		return nil
	}
	sourceHistoryStateV85117.loaded = true
	sourceHistoryStateV85117.store = sourceHistoryStoreV85117{Version: 1, Links: map[string]sourceHistoryEntryV85117{}}
	b, err := os.ReadFile(sourceHistoryStateV85117.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var store sourceHistoryStoreV85117
	if err := json.Unmarshal(b, &store); err != nil {
		return err
	}
	if store.Links == nil {
		store.Links = map[string]sourceHistoryEntryV85117{}
	}
	if store.Version <= 0 {
		store.Version = 1
	}
	sourceHistoryStateV85117.store = store
	return nil
}

func sourceHistorySaveLockedV85117() error {
	if sourceHistoryStateV85117.path == "" {
		return errors.New("cale source_history indisponibilă")
	}
	if err := os.MkdirAll(filepath.Dir(sourceHistoryStateV85117.path), 0755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(sourceHistoryStateV85117.store, "", "  ")
	if err != nil {
		return err
	}
	tmp := sourceHistoryStateV85117.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0644); err != nil {
		return err
	}
	if err := os.Rename(tmp, sourceHistoryStateV85117.path); err == nil {
		return nil
	}
	_ = os.Remove(sourceHistoryStateV85117.path)
	return os.Rename(tmp, sourceHistoryStateV85117.path)
}

func sourceHistoryTrimLockedV85117() {
	if len(sourceHistoryStateV85117.store.Links) <= sourceHistoryMaxLinksV85117 {
		return
	}
	type pair struct {
		Key string
		At  int64
	}
	rows := make([]pair, 0, len(sourceHistoryStateV85117.store.Links))
	for key, entry := range sourceHistoryStateV85117.store.Links {
		rows = append(rows, pair{Key: key, At: entry.LastAt})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].At > rows[j].At })
	for _, row := range rows[sourceHistoryMaxLinksV85117:] {
		delete(sourceHistoryStateV85117.store.Links, row.Key)
	}
}

func sourceHistorySameCountsV85117(a, b sourceHistorySnapshotV85117) bool {
	return a.Total == b.Total && a.Local == b.Local && a.Missing == b.Missing && a.Review == b.Review && a.Manual == b.Manual && a.InternalCompleted == b.InternalCompleted
}

func clampSourceHistoryCountV85117(v, total int) int {
	if v < 0 {
		return 0
	}
	if v > total {
		return total
	}
	return v
}

func canonicalSourceURLV85117(raw string) (string, error) {
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
		if strings.HasPrefix(lower, "utm_") || sourceHistoryTrackingKeyV85117(lower) {
			q.Del(key)
		}
	}
	u.RawQuery = q.Encode()
	if len(u.Path) > 1 {
		u.Path = strings.TrimRight(u.Path, "/")
	}
	return u.String(), nil
}

func sourceHistoryTrackingKeyV85117(key string) bool {
	switch key {
	case "fbclid", "gclid", "dclid", "msclkid", "mc_cid", "mc_eid":
		return true
	default:
		return false
	}
}
