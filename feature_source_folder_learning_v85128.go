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

const sourceFolderLearningServiceV85128 = "ddg-source-folder-learning-v1"
const sourceFolderLearningPortBaseV85128 = 39250
const sourceFolderLearningPortSpanV85128 = 200
const sourceFolderLearningPortAttemptsV85128 = 8
const sourceFolderLearningMaxLinksV85128 = 1000
const sourceFolderLearningMaxObservationsV85128 = 24

type sourceFolderObservationV85128 struct {
	At         int64  `json:"at"`
	Folder     string `json:"folder"`
	Confidence string `json:"confidence"`
	Evidence   int    `json:"evidence"`
}

type sourceFolderProfileV85128 struct {
	URL          string                            `json:"url"`
	Key          string                            `json:"key"`
	Host         string                            `json:"host"`
	Family       string                            `json:"family"`
	LastAt       int64                             `json:"lastAt"`
	Observations []sourceFolderObservationV85128   `json:"observations"`
}

type sourceFolderLearningStoreV85128 struct {
	Version int                                  `json:"version"`
	Links   map[string]sourceFolderProfileV85128 `json:"links"`
}

type sourceFolderObserveRequestV85128 struct {
	URL        string `json:"url"`
	Folder     string `json:"folder"`
	Confidence string `json:"confidence"`
	Evidence   int    `json:"evidence"`
}

type sourceFolderSuggestionV85128 struct {
	Folder       string  `json:"folder"`
	Score        float64 `json:"score"`
	Observations int     `json:"observations"`
	Exact        int     `json:"exact"`
	Family       int     `json:"family"`
	Host         int     `json:"host"`
}

type sourceFolderLearningStateV85128 struct {
	mu       sync.Mutex
	path     string
	store    sourceFolderLearningStoreV85128
	loaded   bool
	basePort int
	port     int
}

var sourceFolderLearningStateV85128 sourceFolderLearningStateV85128

func init() {
	base := strings.ToLower(filepath.Base(os.Args[0]))
	if strings.HasSuffix(base, ".test") {
		return
	}
	go startSourceFolderLearningServiceV85128()
}

func sourceFolderLearningDataDirV85128() string {
	dir, err := portableDataDir()
	if err == nil && strings.TrimSpace(dir) != "" {
		return filepath.Clean(dir)
	}
	return filepath.Clean(filepath.Join(executableDir(), "data"))
}

func sourceFolderLearningPortSeedV85128(dataDir string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.ToLower(filepath.Clean(dataDir))))
	return sourceFolderLearningPortBaseV85128 + int(h.Sum32()%sourceFolderLearningPortSpanV85128)
}

func startSourceFolderLearningServiceV85128() {
	dataDir := sourceFolderLearningDataDirV85128()
	_ = os.MkdirAll(dataDir, 0755)
	base := sourceFolderLearningPortSeedV85128(dataDir)

	var ln net.Listener
	for i := 0; i < sourceFolderLearningPortAttemptsV85128; i++ {
		port := base + i
		candidate, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
		if err != nil {
			if sourceFolderLearningExistingServiceV85128(port) {
				return
			}
			continue
		}
		ln = candidate
		sourceFolderLearningStateV85128.mu.Lock()
		sourceFolderLearningStateV85128.path = filepath.Join(dataDir, "source_folder_learning.json")
		sourceFolderLearningStateV85128.basePort = base
		sourceFolderLearningStateV85128.port = port
		sourceFolderLearningStateV85128.mu.Unlock()
		break
	}
	if ln == nil {
		return
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", sourceFolderLearningHealthV85128)
	mux.HandleFunc("/suggest", sourceFolderLearningSuggestV85128)
	mux.HandleFunc("/observe", sourceFolderLearningObserveV85128)
	server := &http.Server{
		Handler:           sourceFolderLearningCORSV85128(mux),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	_ = server.Serve(ln)
}

func sourceFolderLearningExistingServiceV85128(port int) bool {
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
	return reply.OK && reply.Service == sourceFolderLearningServiceV85128
}

func sourceFolderLearningCORSV85128(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := strings.TrimSpace(r.Header.Get("Origin"))
		if origin != "" {
			u, err := url.Parse(origin)
			host := ""
			if err == nil {
				host = strings.ToLower(u.Hostname())
			}
			if host != "127.0.0.1" && host != "localhost" && host != "::1" {
				http.Error(w, "origine source-folder refuzată", http.StatusForbidden)
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

func sourceFolderLearningHealthV85128(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "GET necesar", http.StatusMethodNotAllowed)
		return
	}
	sourceFolderLearningStateV85128.mu.Lock()
	port, base := sourceFolderLearningStateV85128.port, sourceFolderLearningStateV85128.basePort
	sourceFolderLearningStateV85128.mu.Unlock()
	jsonOut(w, map[string]any{"ok": true, "service": sourceFolderLearningServiceV85128, "version": 1, "port": port, "basePort": base})
}

func sourceFolderLearningObserveV85128(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST necesar", http.StatusMethodNotAllowed)
		return
	}
	var req sourceFolderObserveRequestV85128
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	key, err := canonicalSourceURLV85124(req.URL)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	folder, err := normalizeSourceFolderV85128(req.Folder)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	confidence := normalizeSourceFolderConfidenceV85128(req.Confidence)
	if confidence == "" {
		http.Error(w, "confidence invalid", http.StatusBadRequest)
		return
	}
	if req.Evidence < 1 {
		http.Error(w, "evidence invalid", http.StatusBadRequest)
		return
	}
	if req.Evidence > 10000 {
		req.Evidence = 10000
	}
	host, family := sourceFolderSourceShapeV85128(key)
	now := time.Now().UnixMilli()
	obs := sourceFolderObservationV85128{At: now, Folder: folder, Confidence: confidence, Evidence: req.Evidence}

	sourceFolderLearningStateV85128.mu.Lock()
	defer sourceFolderLearningStateV85128.mu.Unlock()
	_ = sourceFolderLearningLoadLockedV85128()
	profile, exists := sourceFolderLearningStateV85128.store.Links[key]
	if !exists {
		profile = sourceFolderProfileV85128{Key: key, Observations: []sourceFolderObservationV85128{}}
	}
	profile.URL = strings.TrimSpace(req.URL)
	profile.Key = key
	profile.Host = host
	profile.Family = family
	profile.LastAt = now

	deduplicated := false
	if n := len(profile.Observations); n > 0 {
		last := profile.Observations[n-1]
		if strings.EqualFold(last.Folder, folder) && now-last.At < 5*60_000 {
			profile.Observations[n-1] = obs
			deduplicated = true
		}
	}
	if !deduplicated {
		profile.Observations = append(profile.Observations, obs)
	}
	if len(profile.Observations) > sourceFolderLearningMaxObservationsV85128 {
		profile.Observations = append([]sourceFolderObservationV85128(nil), profile.Observations[len(profile.Observations)-sourceFolderLearningMaxObservationsV85128:]...)
	}
	sourceFolderLearningStateV85128.store.Links[key] = profile
	sourceFolderLearningTrimLockedV85128()
	if err := sourceFolderLearningSaveLockedV85128(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOut(w, map[string]any{"ok": true, "deduplicated": deduplicated, "profile": profile})
}

func sourceFolderLearningSuggestV85128(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "GET necesar", http.StatusMethodNotAllowed)
		return
	}
	key, err := canonicalSourceURLV85124(strings.TrimSpace(r.URL.Query().Get("url")))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	host, family := sourceFolderSourceShapeV85128(key)
	sourceFolderLearningStateV85128.mu.Lock()
	_ = sourceFolderLearningLoadLockedV85128()
	store := sourceFolderLearningStateV85128.store
	sourceFolderLearningStateV85128.mu.Unlock()
	suggestions := sourceFolderLearningSuggestFromStoreV85128(store, key, host, family, time.Now())
	jsonOut(w, map[string]any{"ok": true, "key": key, "host": host, "family": family, "suggestions": suggestions})
}

func sourceFolderLearningSuggestFromStoreV85128(store sourceFolderLearningStoreV85128, key, host, family string, now time.Time) []sourceFolderSuggestionV85128 {
	type scoreRow struct {
		sourceFolderSuggestionV85128
		key string
	}
	scores := map[string]*scoreRow{}
	for profileKey, profile := range store.Links {
		relation := 0.0
		relationKind := ""
		switch {
		case profileKey == key:
			relation, relationKind = 6.0, "exact"
		case family != "" && profile.Family == family:
			relation, relationKind = 2.2, "family"
		case host != "" && profile.Host == host:
			relation, relationKind = 0.55, "host"
		default:
			continue
		}
		for _, obs := range profile.Observations {
			folder := strings.TrimSpace(obs.Folder)
			if folder == "" {
				continue
			}
			confidenceFactor := 1.0
			if normalizeSourceFolderConfidenceV85128(obs.Confidence) == "ridicata" {
				confidenceFactor = 1.35
			}
			evidenceFactor := 1.0 + float64(minIntV85128(obs.Evidence, 10))*0.08
			age := now.Sub(time.UnixMilli(obs.At))
			recency := 1.0
			if age > 180*24*time.Hour {
				recency = 0.55
			} else if age > 90*24*time.Hour {
				recency = 0.70
			} else if age > 30*24*time.Hour {
				recency = 0.85
			}
			mapKey := strings.ToLower(folder)
			row := scores[mapKey]
			if row == nil {
				row = &scoreRow{sourceFolderSuggestionV85128: sourceFolderSuggestionV85128{Folder: folder}, key: mapKey}
				scores[mapKey] = row
			}
			row.Score += relation * confidenceFactor * evidenceFactor * recency
			row.Observations++
			switch relationKind {
			case "exact":
				row.Exact++
			case "family":
				row.Family++
			case "host":
				row.Host++
			}
		}
	}
	rows := make([]sourceFolderSuggestionV85128, 0, len(scores))
	for _, row := range scores {
		rows = append(rows, row.sourceFolderSuggestionV85128)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Score != rows[j].Score {
			return rows[i].Score > rows[j].Score
		}
		if rows[i].Exact != rows[j].Exact {
			return rows[i].Exact > rows[j].Exact
		}
		return strings.ToLower(rows[i].Folder) < strings.ToLower(rows[j].Folder)
	})
	if len(rows) > 3 {
		rows = rows[:3]
	}
	return rows
}

func sourceFolderSourceShapeV85128(key string) (string, string) {
	u, err := url.Parse(key)
	if err != nil {
		return "", ""
	}
	host := strings.ToLower(u.Hostname())
	parts := strings.FieldsFunc(strings.Trim(u.Path, "/"), func(r rune) bool { return r == '/' })
	family := host
	if len(parts) > 0 {
		family += "/" + strings.ToLower(parts[0])
	}
	return host, family
}

func normalizeSourceFolderV85128(raw string) (string, error) {
	folder := strings.TrimSpace(strings.ReplaceAll(raw, "/", "\\"))
	folder = strings.TrimRight(folder, "\\")
	if len(folder) == 2 && folder[1] == ':' {
		folder += "\\"
	}
	if folder == "" || len(folder) > 1024 || strings.ContainsAny(folder, "\x00\r\n") {
		return "", errors.New("folder invalid")
	}
	isDrive := len(folder) >= 3 && ((folder[0] >= 'A' && folder[0] <= 'Z') || (folder[0] >= 'a' && folder[0] <= 'z')) && folder[1] == ':' && folder[2] == '\\'
	isUNC := strings.HasPrefix(folder, "\\\\")
	if !isDrive && !isUNC {
		return "", errors.New("folderul trebuie să fie cale Windows absolută")
	}
	return folder, nil
}

func normalizeSourceFolderConfidenceV85128(raw string) string {
	v := strings.ToLower(strings.TrimSpace(raw))
	replacer := strings.NewReplacer("ă", "a", "â", "a", "î", "i", "ș", "s", "ş", "s", "ț", "t", "ţ", "t")
	v = replacer.Replace(v)
	switch v {
	case "ridicata", "high":
		return "ridicata"
	case "medie", "medium":
		return "medie"
	default:
		return ""
	}
}

func sourceFolderLearningLoadLockedV85128() error {
	if sourceFolderLearningStateV85128.loaded {
		return nil
	}
	sourceFolderLearningStateV85128.loaded = true
	sourceFolderLearningStateV85128.store = sourceFolderLearningStoreV85128{Version: 1, Links: map[string]sourceFolderProfileV85128{}}
	b, err := os.ReadFile(sourceFolderLearningStateV85128.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var store sourceFolderLearningStoreV85128
	if err := json.Unmarshal(b, &store); err != nil {
		return err
	}
	if store.Links == nil {
		store.Links = map[string]sourceFolderProfileV85128{}
	}
	if store.Version <= 0 {
		store.Version = 1
	}
	sourceFolderLearningStateV85128.store = store
	return nil
}

func sourceFolderLearningSaveLockedV85128() error {
	if sourceFolderLearningStateV85128.path == "" {
		return errors.New("cale source_folder_learning indisponibilă")
	}
	if err := os.MkdirAll(filepath.Dir(sourceFolderLearningStateV85128.path), 0755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(sourceFolderLearningStateV85128.store, "", "  ")
	if err != nil {
		return err
	}
	tmp := sourceFolderLearningStateV85128.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0644); err != nil {
		return err
	}
	if err := os.Rename(tmp, sourceFolderLearningStateV85128.path); err == nil {
		return nil
	}
	_ = os.Remove(sourceFolderLearningStateV85128.path)
	return os.Rename(tmp, sourceFolderLearningStateV85128.path)
}

func sourceFolderLearningTrimLockedV85128() {
	if len(sourceFolderLearningStateV85128.store.Links) <= sourceFolderLearningMaxLinksV85128 {
		return
	}
	type pair struct {
		Key string
		At  int64
	}
	rows := make([]pair, 0, len(sourceFolderLearningStateV85128.store.Links))
	for key, profile := range sourceFolderLearningStateV85128.store.Links {
		rows = append(rows, pair{Key: key, At: profile.LastAt})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].At > rows[j].At })
	for _, row := range rows[sourceFolderLearningMaxLinksV85128:] {
		delete(sourceFolderLearningStateV85128.store.Links, row.Key)
	}
}

func minIntV85128(a, b int) int {
	if a < b {
		return a
	}
	return b
}
