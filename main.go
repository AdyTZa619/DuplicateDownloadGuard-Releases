package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/md5"
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"encoding/csv"
	"encoding/gob"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

//go:embed web/*
var webFS embed.FS

const appVersion = "8.5.3 Pro Smart Media Guard"
const defaultUpdateManifestURL = "https://raw.githubusercontent.com/AdyTZa619/DuplicateDownloadGuard-Releases/main/update.json"

type FileEntry struct {
	Path   string
	Name   string
	Size   int64
	MTime  int64
	SHA256 string
	MD5    string
}

type Profile struct {
	Name       string   `json:"name"`
	LocalPaths []string `json:"localPaths"`
	Mode       string   `json:"mode"`
	Extensions string   `json:"extensions"`
	MinSize    int64    `json:"minSize"`
}

type Config struct {
	LocalPaths            []string  `json:"localPaths"`
	MegaClientPath        string    `json:"megaClientPath"`
	PlayerPath            string    `json:"playerPath"`
	FFprobePath           string    `json:"ffprobePath"`
	FFmpegPath            string    `json:"ffmpegPath"`
	YtDlpPath             string    `json:"ytDlpPath"`
	GalleryDLPath         string    `json:"galleryDlPath"`
	Aria2Path             string    `json:"aria2Path"`
	DownloadDir           string    `json:"downloadDir"`
	DownloadMethod        string    `json:"downloadMethod"`
	DownloadGuardMode     string    `json:"downloadGuardMode"`
	JDFolder              string    `json:"jdFolder"`
	Mode                  string    `json:"mode"`
	Extensions            string    `json:"extensions"`
	MinSize               int64     `json:"minSize"`
	Profiles              []Profile `json:"profiles"`
	LastMegaURL           string    `json:"lastMegaUrl"`
	SampleBlocks          int       `json:"sampleBlocks"`
	SampleBlockKB         int       `json:"sampleBlockKB"`
	FullVerifyMaxMB       int       `json:"fullVerifyMaxMB"`
	VisualImageMaxMB      int       `json:"visualImageMaxMB"`
	DownloadConcurrency   int       `json:"downloadConcurrency"`
	DownloadRetries       int       `json:"downloadRetries"`
	AriaConnections       int       `json:"ariaConnections"`
	SpeedLimitKB          int       `json:"speedLimitKB"`
	YtCookiesBrowser      string    `json:"ytCookiesBrowser"`
	AIEnabled             bool      `json:"aiEnabled"`
	AIEndpoint            string    `json:"aiEndpoint"`
	AIModel               string    `json:"aiModel"`
	AIVision              bool      `json:"aiVision"`
	UpdateManifestURL     string    `json:"updateManifestUrl"`
	AutoUpdateCheck       bool      `json:"autoUpdateCheck"`
	AutoUpdateInstall     bool      `json:"autoUpdateInstall"`
	LiveRefreshCompare    bool      `json:"liveRefreshCompare"`
	PortableMigrationDone bool      `json:"portableMigrationDone"`
}

type RemoteItem struct {
	ID           int     `json:"id"`
	Path         string  `json:"path"`
	Name         string  `json:"name"`
	Size         int64   `json:"size"`
	MTime        string  `json:"mtime,omitempty"`
	HashType     string  `json:"hashType,omitempty"`
	Hash         string  `json:"hash,omitempty"`
	URL          string  `json:"url,omitempty"`
	DirectURL    string  `json:"directUrl,omitempty"`
	Handle       string  `json:"handle,omitempty"`
	Source       string  `json:"source"`
	ContentType  string  `json:"contentType,omitempty"`
	ETag         string  `json:"etag,omitempty"`
	AcceptRanges bool    `json:"acceptRanges,omitempty"`
	Extractor    string  `json:"extractor,omitempty"`
	ProviderID   string  `json:"providerId,omitempty"`
	Duration     float64 `json:"duration,omitempty"`
	ApproxSize   bool    `json:"approxSize,omitempty"`
}

type Result struct {
	ID             int        `json:"id"`
	Status         string     `json:"status"`
	Confidence     string     `json:"confidence"`
	Remote         RemoteItem `json:"remote"`
	LocalPath      string     `json:"localPath,omitempty"`
	Candidates     int        `json:"candidates"`
	Reason         string     `json:"reason"`
	Manual         bool       `json:"manual"`
	ManualStatus   string     `json:"manualStatus,omitempty"`
	ManualAt       int64      `json:"manualAt,omitempty"`
	AutoStatus     string     `json:"autoStatus,omitempty"`
	AutoConfidence string     `json:"autoConfidence,omitempty"`
	AutoReason     string     `json:"autoReason,omitempty"`
	NameScore      int        `json:"nameScore,omitempty"`
	MatchScore     int        `json:"matchScore,omitempty"`
	SameSize       bool       `json:"sameSize,omitempty"`
	SameExt        bool       `json:"sameExt,omitempty"`
	MediaKind      string     `json:"mediaKind,omitempty"`
	SampleMatched  int        `json:"sampleMatched,omitempty"`
	SampleTotal    int        `json:"sampleTotal,omitempty"`
	VisualScore    int        `json:"visualScore,omitempty"`
	VisualMethod   string     `json:"visualMethod,omitempty"`
	VerifiedBytes  int64      `json:"verifiedBytes,omitempty"`
	DownloadedAt   int64      `json:"downloadedAt,omitempty"`
	DownloadPath   string     `json:"downloadPath,omitempty"`
	AIVerdict      string     `json:"aiVerdict,omitempty"`
	AIConfidence   int        `json:"aiConfidence,omitempty"`
	AIReason       string     `json:"aiReason,omitempty"`
	AIModel        string     `json:"aiModel,omitempty"`
	AIAt           int64      `json:"aiAt,omitempty"`
	GuardVerdict   string     `json:"guardVerdict,omitempty"`
	GuardMethod    string     `json:"guardMethod,omitempty"`
	GuardReason    string     `json:"guardReason,omitempty"`
	GuardAt        int64      `json:"guardAt,omitempty"`
}

type Decision struct {
	Status    string `json:"status"`
	LocalPath string `json:"localPath,omitempty"`
	Note      string `json:"note,omitempty"`
	UpdatedAt int64  `json:"updatedAt"`
}

type MarkSnapshot struct {
	ID          int
	Before      Result
	Key         string
	HadDecision bool
	Decision    Decision
}

type MarkHistory struct {
	Items []MarkSnapshot
}

type Candidate struct {
	Path       string `json:"path"`
	Name       string `json:"name"`
	Size       int64  `json:"size"`
	SizeDelta  int64  `json:"sizeDelta"`
	NameScore  int    `json:"nameScore"`
	SameExt    bool   `json:"sameExt"`
	SameSize   bool   `json:"sameSize"`
	Rank       int    `json:"rank"`
	MatchScore int    `json:"matchScore"`
}

type MediaInfo struct {
	OK         bool    `json:"ok"`
	Source     string  `json:"source"`
	Duration   float64 `json:"duration,omitempty"`
	Format     string  `json:"format,omitempty"`
	BitRate    int64   `json:"bitRate,omitempty"`
	VideoCodec string  `json:"videoCodec,omitempty"`
	Width      int     `json:"width,omitempty"`
	Height     int     `json:"height,omitempty"`
	FPS        string  `json:"fps,omitempty"`
	AudioCodec string  `json:"audioCodec,omitempty"`
	SampleRate string  `json:"sampleRate,omitempty"`
	Channels   int     `json:"channels,omitempty"`
	Error      string  `json:"error,omitempty"`
}

type MegaPreviewState struct {
	Active          bool
	SourceURL       string
	RemotePath      string
	StreamURL       string
	PreviousSession string
	Exe             string
}

type Progress struct {
	Active    bool   `json:"active"`
	Phase     string `json:"phase"`
	Message   string `json:"message"`
	Detail    string `json:"detail"`
	State     string `json:"state"` // running|success|error|cancelled|idle
	Step      int    `json:"step"`
	StepTotal int    `json:"stepTotal"`
	Current   int64  `json:"current"`
	Total     int64  `json:"total"`
	Files     int64  `json:"files"`
	Bytes     int64  `json:"bytes"`
	StartedAt int64  `json:"startedAt"`
	CanCancel bool   `json:"canCancel"`
}

type App struct {
	mu         sync.RWMutex
	guardMu    sync.Mutex
	persistMu  sync.Mutex
	previewMu  sync.Mutex
	preview    MegaPreviewState
	previewTTL *time.Timer
	cfg        Config
	index      map[string]FileEntry
	bySize     map[int64][]string
	byName     map[string][]string
	results    []Result
	decisions  map[string]Decision
	undoMarks  []MarkHistory
	logs       []string
	progress   Progress
	appDir     string
	cancel     context.CancelFunc
	nextRemote int
	opRunning  atomic.Bool
	revision   atomic.Uint64
}

func main() {
	if handled, exitCode := maybeRunNativeUpdater(os.Args); handled {
		os.Exit(exitCode)
	}
	a, err := newApp()
	if err != nil {
		log.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", a.handleWeb)
	mux.HandleFunc("/api/about", a.handleAbout)
	mux.HandleFunc("/api/config", a.handleConfig)
	mux.HandleFunc("/api/pick-folder", a.handlePickFolder)
	mux.HandleFunc("/api/pick-player", a.handlePickPlayer)
	mux.HandleFunc("/api/pick-ffprobe", a.handlePickFFprobe)
	mux.HandleFunc("/api/index/start", a.handleIndexStart)
	mux.HandleFunc("/api/cancel", a.handleCancel)
	mux.HandleFunc("/api/status", a.handleStatus)
	mux.HandleFunc("/api/mega/scan", a.handleMegaScan)
	mux.HandleFunc("/api/url/scan", a.handleURLScan)
	mux.HandleFunc("/api/source/scan", a.handleUniversalScan)
	mux.HandleFunc("/api/tools", a.handleTools)
	mux.HandleFunc("/api/tools/manage", a.handleToolManage)
	mux.HandleFunc("/api/tools/managed", a.handleManagedTools)
	mux.HandleFunc("/api/source/batch", a.handleBatchSourceScan)
	mux.HandleFunc("/api/ai/status", a.handleAIStatus)
	mux.HandleFunc("/api/ai/models", a.handleAIModels)
	mux.HandleFunc("/api/ai/analyze", a.handleAIAnalyze)
	mux.HandleFunc("/api/ai/pull", a.handleAIPull)
	mux.HandleFunc("/api/queue/add", a.handleQueueAdd)
	mux.HandleFunc("/api/queue/list", a.handleQueueList)
	mux.HandleFunc("/api/queue/action", a.handleQueueAction)
	mux.HandleFunc("/api/app/heartbeat", a.handleUIHeartbeat)
	mux.HandleFunc("/api/app/exit-hint", a.handleUIExitHint)
	mux.HandleFunc("/api/update/status", a.handleUpdateStatus)
	mux.HandleFunc("/api/update/check", a.handleUpdateCheck)
	mux.HandleFunc("/api/update/install-online", a.handleUpdateInstallOnline)
	mux.HandleFunc("/api/update/apply", a.handleUpdateApply)
	mux.HandleFunc("/api/import", a.handleImport)
	mux.HandleFunc("/api/results", a.handleResults)
	mux.HandleFunc("/api/results/summary", a.handleResultsSummary)
	mux.HandleFunc("/api/results/select", a.handleSmartSelect)
	mux.HandleFunc("/api/results/candidates", a.handleCandidates)
	mux.HandleFunc("/api/results/candidate", a.handleSelectCandidate)
	mux.HandleFunc("/api/results/deep-verify", a.handleDeepVerify)
	mux.HandleFunc("/api/results/smart-verify", a.handleSmartVerify)
	mux.HandleFunc("/api/results/full-verify", a.handleFullVerify)
	mux.HandleFunc("/api/results/visual-verify", a.handleVisualVerify)
	mux.HandleFunc("/api/download/start", a.handleDownloadStart)
	mux.HandleFunc("/api/download/preflight", a.handleDownloadPreflight)
	mux.HandleFunc("/api/download/jd2", a.handleDownloadJD2)
	mux.HandleFunc("/api/media/compare", a.handleMediaCompare)
	mux.HandleFunc("/api/results/mark", a.handleMark)
	mux.HandleFunc("/api/results/undo-mark", a.handleUndoMark)
	mux.HandleFunc("/api/results/clear", a.handleClearResults)
	mux.HandleFunc("/api/open-data-folder", a.handleOpenDataFolder)
	mux.HandleFunc("/api/export/csv", a.handleExportCSV)
	mux.HandleFunc("/api/export/missing", a.handleExportMissing)
	mux.HandleFunc("/api/open-local", a.handleOpenLocal)
	mux.HandleFunc("/api/open-local-player", a.handleOpenLocalPlayer)
	mux.HandleFunc("/api/open-remote", a.handleOpenRemote)
	mux.HandleFunc("/api/remote-preview/start", a.handleRemotePreviewStart)
	mux.HandleFunc("/api/remote-preview/stop", a.handleRemotePreviewStop)
	mux.HandleFunc("/api/remote-preview/player", a.handleRemotePreviewPlayer)
	mux.HandleFunc("/api/local-preview", a.handleLocalPreview)
	mux.HandleFunc("/api/local-meta", a.handleLocalMeta)
	mux.HandleFunc("/api/logs", a.handleLogs)
	mux.HandleFunc("/api/index/stats", a.handleIndexStats)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	addr := "http://" + ln.Addr().String()
	a.logf("Pornit %s pe %s", appVersion, addr)
	go markUpdateHealthyLater(a.appDir)
	shutdownCh := make(chan struct{}, 1)
	startUIWatchdog(shutdownCh)
	srv := &http.Server{Handler: mux}
	go func() {
		time.Sleep(350 * time.Millisecond)
		openAppWindow(addr)
	}()
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ln) }()
	select {
	case err := <-serveErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	case <-shutdownCh:
		a.logf("Interfața aplicației s-a închis; opresc DDG controlat")
		shutdownApp(a)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = srv.Shutdown(ctx)
		cancel()
	}
}

func newApp() (*App, error) {
	dir, err := portableDataDir()
	if err != nil {
		return nil, err
	}
	_ = migrateLegacyPortableData(dir)
	a := &App{appDir: dir, index: map[string]FileEntry{}, bySize: map[int64][]string{}, byName: map[string][]string{}, decisions: map[string]Decision{}}
	a.cfg = Config{Mode: "balanced", SampleBlocks: 9, SampleBlockKB: 512, FullVerifyMaxMB: 12, VisualImageMaxMB: 25, DownloadMethod: "auto", DownloadGuardMode: guardModeSmart, DownloadConcurrency: 2, DownloadRetries: 3, AriaConnections: 8, AIEndpoint: "http://127.0.0.1:11434", AIVision: true, UpdateManifestURL: defaultUpdateManifestURL, AutoUpdateCheck: true, LiveRefreshCompare: true}
	_ = a.loadConfig()
	// v8.2 uses the project repository directly; no manifest URL setup is required.
	a.cfg.UpdateManifestURL = defaultUpdateManifestURL
	if a.cfg.Mode == "" {
		a.cfg.Mode = "balanced"
	}
	if a.cfg.SampleBlocks <= 0 {
		a.cfg.SampleBlocks = 5
	}
	if a.cfg.SampleBlockKB <= 0 {
		a.cfg.SampleBlockKB = 256
	}
	if a.cfg.FullVerifyMaxMB <= 0 {
		a.cfg.FullVerifyMaxMB = 12
	}
	if a.cfg.VisualImageMaxMB <= 0 {
		a.cfg.VisualImageMaxMB = 20
	}
	if a.cfg.DownloadMethod == "" {
		a.cfg.DownloadMethod = "auto"
	}
	a.cfg.DownloadGuardMode = normalizeGuardMode(a.cfg.DownloadGuardMode)
	if a.cfg.DownloadConcurrency <= 0 {
		a.cfg.DownloadConcurrency = 2
	}
	if a.cfg.DownloadConcurrency > 8 {
		a.cfg.DownloadConcurrency = 8
	}
	if a.cfg.DownloadRetries <= 0 {
		a.cfg.DownloadRetries = 3
	}
	if a.cfg.AriaConnections <= 0 {
		a.cfg.AriaConnections = 8
	}
	if a.cfg.AriaConnections > 16 {
		a.cfg.AriaConnections = 16
	}
	if strings.TrimSpace(a.cfg.AIEndpoint) == "" {
		a.cfg.AIEndpoint = "http://127.0.0.1:11434"
	}
	if strings.TrimSpace(a.cfg.DownloadDir) == "" {
		a.cfg.DownloadDir = portableDownloadsDir()
	}
	_ = a.loadIndex()
	_ = a.loadDecisions()
	_ = a.loadResults()
	a.migrateManualDecisions()
	a.rebuildMaps()
	a.enrichLoadedResults()
	a.revision.Store(1)
	return a, nil
}

func (a *App) configPath() string    { return filepath.Join(a.appDir, "config.json") }
func (a *App) indexPath() string     { return filepath.Join(a.appDir, "index.gob.gz") }
func (a *App) resultsPath() string   { return filepath.Join(a.appDir, "last_results.json.gz") }
func (a *App) decisionsPath() string { return filepath.Join(a.appDir, "manual_decisions.json") }
func (a *App) logPath() string       { return filepath.Join(a.appDir, "DuplicateDownloadGuard.log") }

func decisionKey(x RemoteItem) string {
	ref := strings.TrimSpace(x.Handle)
	if ref == "" {
		ref = strings.TrimSpace(x.Path)
	}
	return strings.ToUpper(strings.TrimSpace(x.Source)) + "|" + strings.TrimSpace(x.URL) + "|" + ref + "|" + strconv.FormatInt(x.Size, 10)
}

func (a *App) loadDecisions() error {
	b, err := os.ReadFile(a.decisionsPath())
	if err != nil {
		return err
	}
	var d map[string]Decision
	if err := json.Unmarshal(b, &d); err != nil {
		return err
	}
	if d == nil {
		d = map[string]Decision{}
	}
	a.decisions = d
	return nil
}

func (a *App) saveDecisions() error {
	a.persistMu.Lock()
	defer a.persistMu.Unlock()
	a.mu.RLock()
	d := make(map[string]Decision, len(a.decisions))
	for k, v := range a.decisions {
		d[k] = v
	}
	a.mu.RUnlock()
	b, _ := json.MarshalIndent(d, "", "  ")
	tmp := a.decisionsPath() + ".tmp"
	if err := os.WriteFile(tmp, b, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, a.decisionsPath())
}

func (a *App) migrateManualDecisions() {
	a.mu.Lock()
	changed := false
	for i := range a.results {
		r := &a.results[i]
		if r.AutoStatus == "" && !r.Manual {
			r.AutoStatus, r.AutoConfidence, r.AutoReason = r.Status, r.Confidence, r.Reason
		}
		if !r.Manual {
			continue
		}
		if r.ManualStatus == "" {
			r.ManualStatus = r.Status
		}
		key := decisionKey(r.Remote)
		if _, ok := a.decisions[key]; !ok {
			a.decisions[key] = Decision{Status: r.Status, LocalPath: r.LocalPath, Note: "Migrat din sesiunea anterioară", UpdatedAt: time.Now().Unix()}
			changed = true
		}
	}
	a.mu.Unlock()
	if changed {
		_ = a.saveDecisions()
		_ = a.saveResults()
	}
}
func (a *App) loadConfig() error {
	b, e := os.ReadFile(a.configPath())
	if e != nil {
		return e
	}
	return json.Unmarshal(b, &a.cfg)
}
func (a *App) saveConfig() error {
	a.persistMu.Lock()
	defer a.persistMu.Unlock()
	a.mu.RLock()
	cfg := a.cfg
	a.mu.RUnlock()
	b, _ := json.MarshalIndent(cfg, "", "  ")
	return os.WriteFile(a.configPath(), b, 0644)
}
func (a *App) loadIndex() error {
	f, e := os.Open(a.indexPath())
	if e != nil {
		return e
	}
	defer f.Close()
	gz, e := gzip.NewReader(f)
	if e != nil {
		return e
	}
	defer gz.Close()
	return gob.NewDecoder(gz).Decode(&a.index)
}
func (a *App) saveIndex() error {
	a.persistMu.Lock()
	defer a.persistMu.Unlock()
	a.mu.RLock()
	idx := make(map[string]FileEntry, len(a.index))
	for path, entry := range a.index {
		idx[path] = entry
	}
	a.mu.RUnlock()
	tmp := a.indexPath() + ".tmp"
	f, e := os.Create(tmp)
	if e != nil {
		return e
	}
	gz := gzip.NewWriter(f)
	enc := gob.NewEncoder(gz)
	e = enc.Encode(idx)
	if e2 := gz.Close(); e == nil {
		e = e2
	}
	if e2 := f.Close(); e == nil {
		e = e2
	}
	if e != nil {
		return e
	}
	return os.Rename(tmp, a.indexPath())
}
func (a *App) loadResults() error {
	f, err := os.Open(a.resultsPath())
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	var rows []Result
	if err := json.NewDecoder(gz).Decode(&rows); err != nil {
		return err
	}
	a.results = rows
	return nil
}
func (a *App) saveResults() error {
	a.persistMu.Lock()
	defer a.persistMu.Unlock()
	tmp := a.resultsPath() + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	gz := gzip.NewWriter(f)
	a.mu.RLock()
	rows := append([]Result(nil), a.results...)
	a.mu.RUnlock()
	err = json.NewEncoder(gz).Encode(rows)
	if e := gz.Close(); err == nil {
		err = e
	}
	if e := f.Close(); err == nil {
		err = e
	}
	if err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, a.resultsPath())
}

func (a *App) rebuildMaps() {
	a.bySize = map[int64][]string{}
	a.byName = map[string][]string{}
	for p, e := range a.index {
		a.bySize[e.Size] = append(a.bySize[e.Size], p)
		a.byName[strings.ToLower(e.Name)] = append(a.byName[strings.ToLower(e.Name)], p)
	}
}
func (a *App) logf(format string, args ...any) {
	s := time.Now().Format("15:04:05") + "  " + fmt.Sprintf(format, args...)
	a.mu.Lock()
	a.logs = append(a.logs, s)
	if len(a.logs) > 1500 {
		a.logs = a.logs[len(a.logs)-1500:]
	}
	a.mu.Unlock()
	lp := a.logPath()
	if st, err := os.Stat(lp); err == nil && st.Size() > 5<<20 {
		_ = os.Remove(lp + ".old")
		_ = os.Rename(lp, lp+".old")
	}
	if f, err := os.OpenFile(lp, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); err == nil {
		_, _ = fmt.Fprintln(f, time.Now().Format("2006-01-02 ")+s)
		_ = f.Close()
	}
}
func (a *App) setProgress(p Progress)            { a.mu.Lock(); a.progress = p; a.mu.Unlock() }
func (a *App) updateProgress(fn func(*Progress)) { a.mu.Lock(); fn(&a.progress); a.mu.Unlock() }

func (a *App) handleWeb(w http.ResponseWriter, r *http.Request) {
	p := strings.TrimPrefix(r.URL.Path, "/")
	if p == "" {
		p = "web/index.html"
	} else {
		p = "web/" + p
	}
	b, e := webFS.ReadFile(p)
	if e != nil {
		http.NotFound(w, r)
		return
	}
	switch filepath.Ext(p) {
	case ".html":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	case ".css":
		w.Header().Set("Content-Type", "text/css")
	case ".js":
		w.Header().Set("Content-Type", "application/javascript")
	}
	w.Write(b)
}
func jsonOut(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
func decodeJSON(r *http.Request, v any) error {
	return json.NewDecoder(io.LimitReader(r.Body, 2<<20)).Decode(v)
}

func (a *App) handleAbout(w http.ResponseWriter, r *http.Request) {
	jsonOut(w, map[string]any{"version": appVersion, "appDir": a.appDir, "platform": runtime.GOOS + "/" + runtime.GOARCH})
}
func (a *App) handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		a.mu.RLock()
		c := a.cfg
		a.mu.RUnlock()
		jsonOut(w, c)
		return
	}
	var c Config
	if e := decodeJSON(r, &c); e != nil {
		http.Error(w, e.Error(), 400)
		return
	}
	if c.Mode == "" {
		c.Mode = "balanced"
	}
	if c.SampleBlocks <= 0 {
		c.SampleBlocks = 5
	}
	if c.SampleBlocks > 9 {
		c.SampleBlocks = 9
	}
	if c.SampleBlockKB <= 0 {
		c.SampleBlockKB = 256
	}
	if c.SampleBlockKB > 1024 {
		c.SampleBlockKB = 1024
	}
	if c.FullVerifyMaxMB <= 0 {
		c.FullVerifyMaxMB = 12
	}
	if c.FullVerifyMaxMB > 256 {
		c.FullVerifyMaxMB = 256
	}
	if c.VisualImageMaxMB <= 0 {
		c.VisualImageMaxMB = 20
	}
	if c.VisualImageMaxMB > 100 {
		c.VisualImageMaxMB = 100
	}
	if c.DownloadMethod == "" {
		c.DownloadMethod = "auto"
	}
	c.DownloadGuardMode = normalizeGuardMode(c.DownloadGuardMode)
	if c.DownloadConcurrency <= 0 {
		c.DownloadConcurrency = 2
	}
	if c.DownloadConcurrency > 8 {
		c.DownloadConcurrency = 8
	}
	if c.DownloadRetries <= 0 {
		c.DownloadRetries = 3
	}
	if c.DownloadRetries > 20 {
		c.DownloadRetries = 20
	}
	if c.AriaConnections <= 0 {
		c.AriaConnections = 8
	}
	if c.AriaConnections > 16 {
		c.AriaConnections = 16
	}
	if c.SpeedLimitKB < 0 {
		c.SpeedLimitKB = 0
	}
	if strings.TrimSpace(c.AIEndpoint) == "" {
		c.AIEndpoint = "http://127.0.0.1:11434"
	}
	a.mu.Lock()
	a.cfg = c
	a.mu.Unlock()
	_ = a.saveConfig()
	jsonOut(w, map[string]bool{"ok": true})
}
func (a *App) handlePickFolder(w http.ResponseWriter, r *http.Request) {
	if runtime.GOOS != "windows" {
		http.Error(w, "Folder picker disponibil în build-ul Windows", 501)
		return
	}
	script := `$s=New-Object -ComObject Shell.Application; $f=$s.BrowseForFolder(0,'Alege folderul sau discul pentru indexare',0,0); if($f){[Console]::OutputEncoding=[Text.Encoding]::UTF8; $f.Self.Path}`
	cmd := exec.Command("powershell.exe", "-NoProfile", "-STA", "-Command", script)
	hideChildWindow(cmd)
	b, e := cmd.Output()
	if e != nil {
		http.Error(w, e.Error(), 500)
		return
	}
	p := strings.TrimSpace(string(b))
	jsonOut(w, map[string]string{"path": p})
}

func (a *App) handlePickPlayer(w http.ResponseWriter, r *http.Request) {
	if runtime.GOOS != "windows" {
		http.Error(w, "Selectorul de player este disponibil în build-ul Windows", 501)
		return
	}
	script := `Add-Type -AssemblyName System.Windows.Forms; $d=New-Object System.Windows.Forms.OpenFileDialog; $d.Title='Alege playerul extern (VLC, MPC-HC etc.)'; $d.Filter='Executabile (*.exe)|*.exe|Toate fișierele (*.*)|*.*'; if($d.ShowDialog() -eq 'OK'){[Console]::OutputEncoding=[Text.Encoding]::UTF8; $d.FileName}`
	cmd := exec.Command("powershell.exe", "-NoProfile", "-STA", "-Command", script)
	hideChildWindow(cmd)
	b, e := cmd.Output()
	if e != nil {
		http.Error(w, e.Error(), 500)
		return
	}
	p := strings.TrimSpace(string(b))
	jsonOut(w, map[string]string{"path": p})
}

func (a *App) handlePickFFprobe(w http.ResponseWriter, r *http.Request) {
	if runtime.GOOS != "windows" {
		http.Error(w, "Selectorul ffprobe este disponibil în build-ul Windows", 501)
		return
	}
	script := `Add-Type -AssemblyName System.Windows.Forms; $d=New-Object System.Windows.Forms.OpenFileDialog; $d.Title='Alege ffprobe.exe (din FFmpeg)'; $d.Filter='ffprobe.exe|ffprobe.exe|Executabile (*.exe)|*.exe|Toate fișierele (*.*)|*.*'; if($d.ShowDialog() -eq 'OK'){[Console]::OutputEncoding=[Text.Encoding]::UTF8; $d.FileName}`
	cmd := exec.Command("powershell.exe", "-NoProfile", "-STA", "-Command", script)
	hideChildWindow(cmd)
	b, e := cmd.Output()
	if e != nil {
		http.Error(w, e.Error(), 500)
		return
	}
	jsonOut(w, map[string]string{"path": strings.TrimSpace(string(b))})
}

func (a *App) beginOp(ctx context.Context, phase, msg string) bool {
	if !a.opRunning.CompareAndSwap(false, true) {
		return false
	}
	cctx, cancel := context.WithCancel(ctx)
	_ = cctx
	a.mu.Lock()
	a.cancel = cancel
	a.progress = Progress{Active: true, Phase: phase, Message: msg, StartedAt: time.Now().Unix(), CanCancel: true}
	a.mu.Unlock()
	return true
}
func (a *App) endOp(msg string) {
	a.opRunning.Store(false)
	a.mu.Lock()
	a.cancel = nil
	a.progress.Active = false
	a.progress.CanCancel = false
	a.progress.State = "success"
	a.progress.Message = msg
	a.progress.Detail = "Operația s-a încheiat cu succes."
	a.mu.Unlock()
}
func (a *App) failOp(msg, detail string) {
	a.opRunning.Store(false)
	a.mu.Lock()
	a.cancel = nil
	a.progress.Active = false
	a.progress.CanCancel = false
	a.progress.State = "error"
	a.progress.Message = msg
	a.progress.Detail = detail
	a.mu.Unlock()
}
func (a *App) handleCancel(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	if a.cancel != nil {
		a.cancel()
	}
	a.mu.Unlock()
	jsonOut(w, map[string]bool{"ok": true})
}
func (a *App) handleStatus(w http.ResponseWriter, r *http.Request) {
	a.mu.RLock()
	p := a.progress
	a.mu.RUnlock()
	jsonOut(w, p)
}

func (a *App) handleIndexStart(w http.ResponseWriter, r *http.Request) {
	if a.opRunning.Load() {
		http.Error(w, "Există deja o operație în curs", 409)
		return
	}
	a.mu.RLock()
	paths := append([]string(nil), a.cfg.LocalPaths...)
	ext := a.cfg.Extensions
	minSize := a.cfg.MinSize
	a.mu.RUnlock()
	if len(paths) == 0 {
		http.Error(w, "Adaugă cel puțin un folder local", 400)
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.mu.Lock()
	a.cancel = cancel
	a.mu.Unlock()
	if !a.opRunning.CompareAndSwap(false, true) {
		http.Error(w, "ocupat", 409)
		return
	}
	a.setProgress(Progress{Active: true, Phase: "index", Message: "Indexare locală...", StartedAt: time.Now().Unix(), CanCancel: true})
	go a.runIndex(ctx, paths, ext, minSize)
	jsonOut(w, map[string]bool{"started": true})
}
func parseExts(s string) map[string]bool {
	m := map[string]bool{}
	for _, x := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ';' || r == ' ' }) {
		x = strings.ToLower(strings.TrimSpace(x))
		if x != "" {
			if !strings.HasPrefix(x, ".") {
				x = "." + x
			}
			m[x] = true
		}
	}
	return m
}
func isUnder(path, root string) bool {
	rp, _ := filepath.Rel(root, path)
	return rp != ".." && !strings.HasPrefix(rp, ".."+string(os.PathSeparator))
}
func (a *App) runIndex(ctx context.Context, roots []string, extensions string, minSize int64) {
	defer func() {
		a.opRunning.Store(false)
		a.mu.Lock()
		a.cancel = nil
		a.progress.Active = false
		a.progress.CanCancel = false
		a.mu.Unlock()
	}()
	extSet := parseExts(extensions)
	seen := map[string]bool{}
	var count, bytes int64
	a.logf("Indexare pornită: %d locații", len(roots))
	a.mu.RLock()
	old := make(map[string]FileEntry, len(a.index))
	for k, v := range a.index {
		old[k] = v
	}
	a.mu.RUnlock()
	idx := old
	for _, root := range roots {
		root = filepath.Clean(root)
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if err != nil {
				a.logf("Ignorat: %s (%v)", path, err)
				return nil
			}
			if d.IsDir() {
				return nil
			}
			info, e := d.Info()
			if e != nil {
				return nil
			}
			if info.Size() < minSize {
				return nil
			}
			if len(extSet) > 0 && !extSet[strings.ToLower(filepath.Ext(d.Name()))] {
				return nil
			}
			seen[path] = true
			ent := FileEntry{Path: path, Name: d.Name(), Size: info.Size(), MTime: info.ModTime().UnixNano()}
			if o, ok := old[path]; ok && o.Size == ent.Size && o.MTime == ent.MTime {
				ent.SHA256 = o.SHA256
				ent.MD5 = o.MD5
			}
			idx[path] = ent
			count++
			bytes += info.Size()
			if count%100 == 0 {
				a.updateProgress(func(p *Progress) {
					p.Files = count
					p.Bytes = bytes
					p.Current = count
					p.Message = fmt.Sprintf("Indexate %d fișiere • %s", count, human(bytes))
				})
			}
			return nil
		})
		if err != nil && ctx.Err() == nil {
			a.logf("Eroare la %s: %v", root, err)
		}
		if ctx.Err() != nil {
			a.logf("Indexare anulată")
			a.endOp("Indexare anulată")
			return
		}
	}
	for p := range idx {
		for _, root := range roots {
			if isUnder(p, filepath.Clean(root)) && !seen[p] {
				delete(idx, p)
				break
			}
		}
	}
	a.mu.Lock()
	a.index = idx
	a.rebuildMaps()
	a.mu.Unlock()
	if e := a.saveIndex(); e != nil {
		a.logf("Eroare salvare index: %v", e)
		a.endOp("Index creat, dar salvarea a eșuat")
		return
	}
	a.logf("Indexare terminată: %d fișiere, %s", count, human(bytes))
	a.endOp(fmt.Sprintf("Gata • %d fișiere • %s", count, human(bytes)))
}

func (a *App) detectMegaClient() string {
	a.mu.RLock()
	custom := a.cfg.MegaClientPath
	a.mu.RUnlock()
	if custom != "" {
		if _, e := os.Stat(custom); e == nil {
			return custom
		}
	}
	cand := []string{filepath.Join(portableToolsDir(), "megacmd", "MegaClient.exe"), filepath.Join(portableToolsDir(), "MegaClient.exe"), filepath.Join(os.Getenv("LOCALAPPDATA"), "MEGAcmd", "MegaClient.exe"), filepath.Join(os.Getenv("ProgramFiles"), "MEGAcmd", "MegaClient.exe")}
	for _, p := range cand {
		if _, e := os.Stat(p); e == nil {
			return p
		}
	}
	if p, e := exec.LookPath("MegaClient.exe"); e == nil {
		return p
	}
	return ""
}
func runMega(ctx context.Context, exe string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, exe, args...)
	// MEGAcmd is a console program. Background MEGA operations must never
	// steal focus or create visible console windows on Windows.
	hideChildWindow(cmd)
	cmd.Env = os.Environ()
	b, e := cmd.CombinedOutput()
	s := strings.TrimSpace(string(b))
	if e != nil {
		return s, fmt.Errorf("%w: %s", e, s)
	}
	return s, nil
}

func runMegaTimed(parent context.Context, timeout time.Duration, exe string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	out, err := runMega(ctx, exe, args...)
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return out, fmt.Errorf("timeout după %s", timeout.Round(time.Second))
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return out, context.Canceled
	}
	return out, err
}

var sessionRE = regexp.MustCompile(`(?m)^([A-Za-z0-9_-]{40,})\s*$`)

func extractSession(s string) string {
	m := sessionRE.FindStringSubmatch(s)
	if len(m) > 1 {
		return m[1]
	}
	return ""
}

func (a *App) handleMegaScan(w http.ResponseWriter, r *http.Request) {
	// Remote preview uses MEGAcmd's shared session. Restore it before starting a new scan.
	_ = a.stopMegaPreview("scan nou")
	var req struct {
		URL  string `json:"url"`
		Mode string `json:"mode"`
	}
	if e := decodeJSON(r, &req); e != nil {
		http.Error(w, e.Error(), 400)
		return
	}
	req.URL = strings.TrimSpace(req.URL)
	if req.URL == "" {
		http.Error(w, "Link MEGA lipsă", 400)
		return
	}
	if a.opRunning.Load() {
		http.Error(w, "Există deja o operație în curs", 409)
		return
	}
	exe := a.detectMegaClient()
	if exe == "" {
		http.Error(w, "MEGAcmd nu a fost găsit. Instalează MEGAcmd sau setează calea către MegaClient.exe în Settings.", 400)
		return
	}
	a.mu.Lock()
	a.cfg.LastMegaURL = req.URL
	a.mu.Unlock()
	_ = a.saveConfig()
	ctx, cancel := context.WithCancel(context.Background())
	a.mu.Lock()
	a.cancel = cancel
	a.mu.Unlock()
	a.opRunning.Store(true)
	a.setProgress(Progress{Active: true, Phase: "mega", State: "running", Step: 1, StepTotal: 6, Message: "MEGA • Pas 1/6 — verific sesiunea MEGAcmd", Detail: "Această etapă are timeout 10 secunde; dacă MEGAcmd nu răspunde, programul continuă în mod sigur.", StartedAt: time.Now().Unix(), CanCancel: true})
	go a.runMegaScan(ctx, exe, req.URL, req.Mode)
	jsonOut(w, map[string]bool{"started": true})
}
func (a *App) runMegaScan(ctx context.Context, exe, link, mode string) {
	defer func() {
		a.opRunning.Store(false)
		a.mu.Lock()
		a.cancel = nil
		a.progress.Active = false
		a.progress.CanCancel = false
		a.mu.Unlock()
	}()
	a.updateProgress(func(p *Progress) {
		p.State = "running"
		p.Message = "MEGA • aștept sesiunea exclusivă"
		p.Detail = "Scanarea, preview-ul și downloadul MEGA folosesc pe rând aceeași sesiune pentru a evita logout/login concurent."
	})
	if err := acquireMegaSession(ctx); err != nil {
		a.failOp("MEGA: operație anulată", "Nu am putut obține sesiunea MEGA: "+err.Error())
		return
	}
	defer releaseMegaSession()
	if err := a.stopMegaPreviewWhileSessionOwned("pornire scanare MEGA"); err != nil {
		a.logf("MEGA: cleanup preview înainte de scanare: %v", err)
	}
	a.logf("MEGA: scanare folder public")
	oldSession := ""

	a.updateProgress(func(p *Progress) {
		p.Step = 1
		p.StepTotal = 6
		p.State = "running"
		p.Message = "MEGA • Pas 1/6 — verific sesiunea MEGAcmd"
		p.Detail = "Timeout maxim: 10 secunde. Nu se descarcă fișiere."
	})
	if s, e := runMegaTimed(ctx, 10*time.Second, exe, "session"); e == nil {
		oldSession = extractSession(s)
	} else {
		a.logf("MEGA: verificarea sesiunii nu a răspuns normal (%v); continui", e)
	}
	if ctx.Err() != nil {
		a.failOp("MEGA: operație anulată", "Scanarea a fost oprită de utilizator.")
		return
	}

	a.updateProgress(func(p *Progress) {
		p.Step = 2
		p.Message = "MEGA • Pas 2/6 — pregătesc conexiunea"
		p.Detail = "Închid temporar sesiunea MEGAcmd curentă. Timeout maxim: 10 secunde."
	})
	if oldSession != "" {
		a.logf("MEGA: sesiune existentă detectată; va fi restaurată")
		if _, e := runMegaTimed(ctx, 10*time.Second, exe, "logout", "--keep-session"); e != nil {
			a.logf("MEGA: logout keep-session: %v", e)
		}
	} else {
		if _, e := runMegaTimed(ctx, 10*time.Second, exe, "logout"); e != nil {
			a.logf("MEGA: logout inițial: %v", e)
		}
	}
	if ctx.Err() != nil {
		a.failOp("MEGA: operație anulată", "Scanarea a fost oprită de utilizator.")
		return
	}

	a.updateProgress(func(p *Progress) {
		p.Step = 3
		p.Message = "MEGA • Pas 3/6 — deschid folderul public"
		p.Detail = "Autentific folderul public din link. Timeout maxim: 45 secunde."
	})
	out, e := runMegaTimed(ctx, 45*time.Second, exe, "login", link)
	if e != nil {
		a.logf("MEGA login eșuat: %s", sanitizeMega(out))
		a.restoreMega(exe, oldSession)
		problem := classifyMegaProblem(out, e)
		a.failOp(problem.Title, problem.Message+" "+problem.Action)
		return
	}

	a.updateProgress(func(p *Progress) {
		p.Step = 4
		p.Message = "MEGA • Pas 4/6 — citesc lista de fișiere"
		p.Detail = "Se citesc doar nume, căi și dimensiuni; conținutul video/foto NU este descărcat. Timeout maxim: 5 minute."
	})
	out, e = runMegaTimed(ctx, 5*time.Minute, exe, "find", "/", "-l", "--type=f", "--show-handles", "--time-format=ISO6081_WITH_TIME")
	if e != nil {
		a.logf("MEGA find eșuat: %s", sanitizeMega(out))
		a.restoreMega(exe, oldSession)
		problem := classifyMegaProblem(out, e)
		a.failOp(problem.Title, problem.Message+" "+problem.Action)
		return
	}
	items := parseMegaLong(out, "MEGA", link)
	if len(items) == 0 {
		a.updateProgress(func(p *Progress) {
			p.Detail = "Formatul principal nu a produs fișiere; încerc listarea recursivă alternativă (maxim 5 minute)."
		})
		out2, e2 := runMegaTimed(ctx, 5*time.Minute, exe, "ls", "-lR", "--show-handles", "--time-format=ISO6081_WITH_TIME", "/")
		if e2 == nil {
			items = parseMegaLong(out2, "MEGA", link)
		} else {
			a.logf("MEGA fallback ls: %v", e2)
		}
	}
	if len(items) == 0 {
		a.restoreMega(exe, oldSession)
		a.failOp("MEGA: 0 fișiere detectate", "Folderul s-a deschis, dar parserul nu a găsit fișiere în răspuns. Verifică Jurnalul tehnic.")
		return
	}

	a.updateProgress(func(p *Progress) {
		p.Step = 5
		p.Files = int64(len(items))
		p.Total = int64(len(items))
		p.Current = 0
		p.Message = fmt.Sprintf("MEGA • Pas 5/6 — compar %d fișiere cu indexul PC", len(items))
		p.Detail = "Comparația se face local; nu există trafic de download pentru conținutul remote."
	})
	a.compareRemote(ctx, items, mode)
	if ctx.Err() != nil {
		a.failOp("MEGA: operație anulată", "Comparația a fost oprită de utilizator.")
		return
	}

	a.updateProgress(func(p *Progress) {
		p.Step = 6
		p.Current = int64(len(items))
		p.Message = "MEGA • Pas 6/6 — finalizez"
		p.Detail = "Păstrez temporar sesiunea folderului pentru pornirea rapidă a preview-ului."
	})
	if err := a.prepareMegaWarmRootAfterScanV86(ctx, exe, link, oldSession); err != nil {
		a.logf("MEGA Fast Preview: WebDAV root nu a putut fi pregătit (%v); păstrez fallback-ul existent", err)
		a.keepMegaSessionWarm(exe, link, oldSession)
	}
	a.logf("MEGA: %d fișiere comparate", len(items))
	a.endOp(fmt.Sprintf("MEGA gata ✓ • %d fișiere comparate", len(items)))
}

func sanitizeMega(s string) string {
	if len(s) > 500 {
		s = s[:500]
	}
	return strings.ReplaceAll(s, "\r", "")
}
func (a *App) restoreMega(exe, old string) {
	ctx, c := context.WithTimeout(context.Background(), 30*time.Second)
	defer c()
	_, _ = runMegaTimed(ctx, 10*time.Second, exe, "logout")
	if old != "" {
		a.updateProgress(func(p *Progress) { p.Message = "MEGA: restaurez sesiunea anterioară..." })
		if _, e := runMegaTimed(ctx, 30*time.Second, exe, "login", old); e != nil {
			a.logf("Atenție: sesiunea MEGAcmd anterioară nu a putut fi restaurată automat")
		} else {
			a.logf("MEGA: sesiunea anterioară restaurată")
		}
	}
}

func (a *App) keepMegaSessionWarm(exe, sourceURL, previousSession string) {
	a.previewMu.Lock()
	defer a.previewMu.Unlock()
	a.preview = MegaPreviewState{
		Active:          true,
		SourceURL:       sourceURL,
		PreviousSession: previousSession,
		Exe:             exe,
	}
	a.resetPreviewTTLLocked()
	a.logf("MEGA preview: sesiune pregătită după scanare")
}

var longLineRE = regexp.MustCompile(`^\s*([dribx-][a-z-]{3})\s+(\d+)\s+(\d+)\s+(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})\s+(.+?)\s*$`)
var megaHandleRE = regexp.MustCompile(`(?:^|\s)H:([A-Za-z0-9_-]{8,})(?:\s|$)`)

func parseMegaLong(out, source, srcURL string) []RemoteItem {
	var items []RemoteItem
	sc := bufio.NewScanner(strings.NewReader(out))
	buf := make([]byte, 64*1024)
	sc.Buffer(buf, 8*1024*1024)
	id := 0
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "FLAGS ") {
			continue
		}
		handle := ""
		if hm := megaHandleRE.FindStringSubmatch(line); len(hm) > 1 {
			handle = hm[1]
			line = strings.TrimSpace(megaHandleRE.ReplaceAllString(line, " "))
		}
		m := longLineRE.FindStringSubmatch(line)
		if len(m) == 6 {
			if strings.HasPrefix(m[1], "d") {
				continue
			}
			sz, _ := strconv.ParseInt(m[3], 10, 64)
			name := strings.TrimSpace(m[5])
			if name == "" {
				continue
			}
			id++
			items = append(items, RemoteItem{ID: id, Path: name, Name: filepath.Base(strings.ReplaceAll(name, "/", string(os.PathSeparator))), Size: sz, MTime: m[4], Source: source, URL: srcURL, Handle: handle})
			continue
		}
		// more tolerant parser: FLAGS VERS SIZE DATE NAME
		fields := strings.Fields(line)
		if len(fields) >= 5 && len(fields[0]) == 4 && (fields[0][0] == '-' || fields[0][0] == 'd' || fields[0][0] == 'r') {
			sz, e := strconv.ParseInt(fields[2], 10, 64)
			if e != nil || fields[0][0] == 'd' {
				continue
			}
			name := strings.Join(fields[4:], " ")
			id++
			items = append(items, RemoteItem{ID: id, Path: name, Name: filepath.Base(strings.ReplaceAll(name, "/", string(os.PathSeparator))), Size: sz, MTime: fields[3], Source: source, URL: srcURL, Handle: handle})
		}
	}
	return items
}

func megaItemURL(base, handle string) string {
	base = strings.TrimSpace(base)
	handle = strings.TrimSpace(handle)
	if base == "" || handle == "" {
		return base
	}
	// Modern public folder format supports selecting a node by appending /file/HANDLE
	// after the folder key fragment, e.g. .../folder/ID#KEY/file/NODE.
	if strings.Contains(base, "/folder/") {
		if i := strings.Index(base, "/file/"); i >= 0 {
			base = base[:i]
		}
		// A previously selected subfolder may also be present.
		if i := strings.Index(base, "/folder/"); i >= 0 {
			rest := base[i+len("/folder/"):]
			if j := strings.Index(rest, "/folder/"); j >= 0 {
				base = base[:i+len("/folder/")+j]
			}
		}
		return strings.TrimRight(base, "/") + "/file/" + handle
	}
	return base
}

func (a *App) handleURLScan(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL  string `json:"url"`
		Mode string `json:"mode"`
	}
	if e := decodeJSON(r, &req); e != nil {
		http.Error(w, e.Error(), 400)
		return
	}
	u := strings.TrimSpace(req.URL)
	if u == "" {
		http.Error(w, "URL lipsă", 400)
		return
	}
	item, e := scanURL(u)
	if e != nil {
		http.Error(w, e.Error(), 400)
		return
	}
	a.compareRemote(context.Background(), []RemoteItem{item}, req.Mode)
	jsonOut(w, item)
}
func scanURLLegacy(u string) (RemoteItem, error) {
	c := http.Client{Timeout: 25 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > 10 {
			return errors.New("prea multe redirectări")
		}
		return nil
	}}
	req, _ := http.NewRequest("HEAD", u, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 DuplicateDownloadGuard/6.0")
	resp, e := c.Do(req)
	if e != nil {
		return RemoteItem{}, e
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return RemoteItem{}, fmt.Errorf("server HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength < 0 {
		return RemoteItem{}, errors.New("serverul nu publică Content-Length; comparația fără transfer nu este sigură")
	}
	name := ""
	cd := resp.Header.Get("Content-Disposition")
	if cd != "" {
		if _, p, e := mimeParseMediaType(cd); e == nil {
			name = p["filename"]
		}
	}
	if name == "" {
		pu, _ := url.Parse(resp.Request.URL.String())
		name = filepath.Base(pu.Path)
	}
	if name == "" || name == "/" {
		name = "download"
	}
	it := RemoteItem{ID: 1, Path: name, Name: name, Size: resp.ContentLength, Source: "HTTP", URL: u}
	if h := resp.Header.Get("X-Checksum-Sha256"); validHex(h, 64) {
		it.HashType = "sha256"
		it.Hash = strings.ToLower(h)
	} else if d := resp.Header.Get("Digest"); d != "" {
		for _, part := range strings.Split(d, ",") {
			kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
			if len(kv) == 2 && strings.EqualFold(kv[0], "sha-256") {
				if b, e := base64.StdEncoding.DecodeString(kv[1]); e == nil && len(b) == 32 {
					it.HashType = "sha256"
					it.Hash = hex.EncodeToString(b)
				}
			}
		}
	} else if m := resp.Header.Get("Content-MD5"); m != "" {
		if b, e := base64.StdEncoding.DecodeString(m); e == nil && len(b) == 16 {
			it.HashType = "md5"
			it.Hash = hex.EncodeToString(b)
		}
	}
	return it, nil
}

// tiny Content-Disposition parser sufficient for filename=, avoiding external packages.
func mimeParseMediaType(v string) (string, map[string]string, error) {
	parts := strings.Split(v, ";")
	params := map[string]string{}
	for _, p := range parts[1:] {
		kv := strings.SplitN(strings.TrimSpace(p), "=", 2)
		if len(kv) == 2 {
			params[strings.ToLower(kv[0])] = strings.Trim(kv[1], "\"")
		}
	}
	return strings.TrimSpace(parts[0]), params, nil
}
func validHex(s string, n int) bool {
	if len(s) != n {
		return false
	}
	_, e := hex.DecodeString(s)
	return e == nil
}

func (a *App) handleImport(w http.ResponseWriter, r *http.Request) {
	// Body: CSV text. Columns accepted: path,size,sha256,md5,url
	b, e := io.ReadAll(io.LimitReader(r.Body, 50<<20))
	if e != nil {
		http.Error(w, e.Error(), 400)
		return
	}
	cr := csv.NewReader(strings.NewReader(string(b)))
	cr.FieldsPerRecord = -1
	var items []RemoteItem
	id := 0
	for {
		row, e := cr.Read()
		if e == io.EOF {
			break
		}
		if e != nil {
			continue
		}
		if len(row) < 2 {
			continue
		}
		sz, e := strconv.ParseInt(strings.TrimSpace(row[1]), 10, 64)
		if e != nil {
			continue
		}
		id++
		it := RemoteItem{ID: id, Path: strings.TrimSpace(row[0]), Name: filepath.Base(strings.TrimSpace(row[0])), Size: sz, Source: "CSV"}
		if len(row) > 2 && validHex(strings.TrimSpace(row[2]), 64) {
			it.HashType = "sha256"
			it.Hash = strings.ToLower(strings.TrimSpace(row[2]))
		}
		if it.Hash == "" && len(row) > 3 && validHex(strings.TrimSpace(row[3]), 32) {
			it.HashType = "md5"
			it.Hash = strings.ToLower(strings.TrimSpace(row[3]))
		}
		if len(row) > 4 {
			it.URL = strings.TrimSpace(row[4])
		}
		items = append(items, it)
	}
	a.compareRemote(context.Background(), items, a.cfg.Mode)
	jsonOut(w, map[string]int{"items": len(items)})
}

var noisyNameTokenRE = regexp.MustCompile(`(?i)\b(?:2160p|1440p|1080p|720p|576p|480p|360p|4k|8k|uhd|fhd|hd|source|original|orig|download|copy|final)\b|\b\d{2,5}x\d{2,5}\b`)
var dateNameTokenRE = regexp.MustCompile(`(?i)\b(?:19|20)\d{2}[-_. ](?:0?[1-9]|1[0-2])[-_. ](?:0?[1-9]|[12]\d|3[01])\b`)
var duplicateSuffixRE = regexp.MustCompile(`(?i)(?:[-_ ](?:d)?[0-9a-f]{4,8}|\s*\(\d+\)|[-_ ]copy(?:[-_ ]\d+)?)$`)
var nameSepRE = regexp.MustCompile(`[^a-z0-9]+`)

func duplicateAwareStem(name string) string {
	base := strings.ToLower(filepath.Base(name))
	base = strings.TrimSuffix(base, filepath.Ext(base))
	// Some download/dedup tools append more than one collision marker. Strip
	// all trailing markers, but only when separated from the real basename.
	for {
		trimmed := strings.TrimSpace(duplicateSuffixRE.ReplaceAllString(base, ""))
		if trimmed == base {
			break
		}
		base = trimmed
	}
	base = nameSepRE.ReplaceAllString(base, " ")
	return strings.TrimSpace(strings.Join(strings.Fields(base), " "))
}

func normalizedStem(name string) string {
	base := strings.ToLower(filepath.Base(name))
	ext := filepath.Ext(base)
	base = strings.TrimSuffix(base, ext)
	base = duplicateSuffixRE.ReplaceAllString(base, "")
	base = dateNameTokenRE.ReplaceAllString(base, " ")
	base = noisyNameTokenRE.ReplaceAllString(base, " ")
	base = nameSepRE.ReplaceAllString(base, " ")
	return strings.TrimSpace(strings.Join(strings.Fields(base), " "))
}
func levenshtein(a, b string) int {
	ar, br := []rune(a), []rune(b)
	if len(ar) == 0 {
		return len(br)
	}
	if len(br) == 0 {
		return len(ar)
	}
	prev := make([]int, len(br)+1)
	cur := make([]int, len(br)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ar); i++ {
		cur[0] = i
		for j := 1; j <= len(br); j++ {
			cost := 0
			if ar[i-1] != br[j-1] {
				cost = 1
			}
			x := prev[j] + 1
			if cur[j-1]+1 < x {
				x = cur[j-1] + 1
			}
			if prev[j-1]+cost < x {
				x = prev[j-1] + cost
			}
			cur[j] = x
		}
		prev, cur = cur, prev
	}
	return prev[len(br)]
}
func tokenJaccard(a, b string) int {
	as, bs := map[string]bool{}, map[string]bool{}
	for _, x := range strings.Fields(a) {
		if len(x) > 1 {
			as[x] = true
		}
	}
	for _, x := range strings.Fields(b) {
		if len(x) > 1 {
			bs[x] = true
		}
	}
	if len(as) == 0 && len(bs) == 0 {
		return 100
	}
	inter, union := 0, len(as)
	for x := range bs {
		if as[x] {
			inter++
		} else {
			union++
		}
	}
	if union == 0 {
		return 0
	}
	return int(math.Round(float64(inter) * 100 / float64(union)))
}
func nameSimilarity(a, b string) int {
	// Check collision suffixes before removing generic words such as
	// "original". Otherwise original.jpg and original-D3558.jpg normalize to
	// two empty strings even though they clearly describe the same basename.
	rawA, rawB := duplicateAwareStem(a), duplicateAwareStem(b)
	if rawA != "" && rawA == rawB {
		return 99
	}
	a, b = normalizedStem(a), normalizedStem(b)
	if a == "" || b == "" {
		return 0
	}
	if a == b {
		return 100
	}
	mx := len([]rune(a))
	if n := len([]rune(b)); n > mx {
		mx = n
	}
	lev := 100
	if mx > 0 {
		lev = int(math.Round((1 - float64(levenshtein(a, b))/float64(mx)) * 100))
	}
	jac := tokenJaccard(a, b)
	score := int(math.Round(float64(lev)*0.62 + float64(jac)*0.38))
	if strings.Contains(a, b) || strings.Contains(b, a) {
		if score < 82 {
			score = 82
		}
	}
	// Download packs often prepend dates/resolution or append "_source" while
	// preserving a long content/id token. Reward such a strong shared token.
	bt := map[string]bool{}
	for _, x := range strings.Fields(b) {
		bt[x] = true
	}
	longest := 0
	for _, x := range strings.Fields(a) {
		if bt[x] && len(x) > longest {
			longest = len(x)
		}
	}
	if longest >= 16 && score < 94 {
		score = 94
	} else if longest >= 8 && score < 88 {
		score = 88
	}
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	return score
}
func abs64(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}

func candidateMatchScore(remote RemoteItem, e FileEntry) int {
	ns := nameSimilarity(remote.Name, e.Name)
	delta := abs64(e.Size - remote.Size)
	sameSize := delta == 0
	sameExt := strings.EqualFold(filepath.Ext(remote.Name), filepath.Ext(e.Name))
	// Scor explicabil 0..99. 100 este rezervat unei verificări de conținut/hash.
	score := int(math.Round(float64(ns) * 0.45))
	if sameExt {
		score += 10
	}
	if sameSize {
		score += 45
	} else if remote.Size > 0 {
		ratio := float64(delta) / float64(remote.Size)
		switch {
		case ratio <= .001:
			score += 32
		case ratio <= .005:
			score += 24
		case ratio <= .01:
			score += 16
		case ratio <= .05:
			score += 7
		}
	}
	if sameSize && ns >= 94 && score < 96 {
		score = 96
	}
	if sameSize && ns == 100 && score < 99 {
		score = 99
	}
	if score < 0 {
		return 0
	}
	if score > 99 {
		return 99
	}
	return score
}

func rankCandidate(remote RemoteItem, e FileEntry) Candidate {
	ns := nameSimilarity(remote.Name, e.Name)
	delta := e.Size - remote.Size
	sameSize := delta == 0
	sameExt := strings.EqualFold(filepath.Ext(remote.Name), filepath.Ext(e.Name))
	matchScore := candidateMatchScore(remote, e)
	// Rankul păstrează scorul general dominant, apoi numele pentru departajare.
	rank := matchScore*1000 + ns*10
	if remote.Size > 0 {
		ratio := float64(abs64(delta)) / float64(remote.Size)
		if ratio <= .001 {
			rank += 20
		}
	}
	return Candidate{Path: e.Path, Name: e.Name, Size: e.Size, SizeDelta: delta, NameScore: ns, SameExt: sameExt, SameSize: sameSize, Rank: rank, MatchScore: matchScore}
}

func enrichResult(r *Result, idx map[string]FileEntry) {
	r.MediaKind = remoteMediaKind(r.Remote.Name)
	r.SameSize, r.SameExt = false, false
	if r.LocalPath != "" {
		if e, ok := idx[r.LocalPath]; ok {
			c := rankCandidate(r.Remote, e)
			r.NameScore = c.NameScore
			r.MatchScore = c.MatchScore
			r.SameSize = c.SameSize
			r.SameExt = c.SameExt
		}
	}
	st := r.AutoStatus
	if st == "" {
		st = r.Status
	}
	switch st {
	case "VERIFIED":
		r.MatchScore = 100
	case "SAMPLED":
		if r.MatchScore < 99 {
			r.MatchScore = 99
		}
	case "MISSING":
		if r.LocalPath == "" {
			r.MatchScore = 0
		}
	}
}

func (a *App) enrichLoadedResults() {
	a.mu.Lock()
	defer a.mu.Unlock()
	for i := range a.results {
		enrichResult(&a.results[i], a.index)
	}
}

func (a *App) candidatesFor(remote RemoteItem, limit int) []Candidate {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := make([]Candidate, 0, limit*2)
	for _, e := range a.index {
		c := rankCandidate(remote, e)
		// Keep same-size files, similar names, or near-size same-extension media.
		ratio := 1.0
		if remote.Size > 0 {
			ratio = float64(abs64(c.SizeDelta)) / float64(remote.Size)
		}
		if c.SameSize || c.NameScore >= 42 || (c.SameExt && ratio <= .05) {
			out = append(out, c)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Rank != out[j].Rank {
			return out[i].Rank > out[j].Rank
		}
		if abs64(out[i].SizeDelta) != abs64(out[j].SizeDelta) {
			return abs64(out[i].SizeDelta) < abs64(out[j].SizeDelta)
		}
		return strings.ToLower(out[i].Path) < strings.ToLower(out[j].Path)
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func (a *App) compareRemote(ctx context.Context, items []RemoteItem, mode string) {
	a.mu.RLock()
	liveRefresh := a.cfg.LiveRefreshCompare
	a.mu.RUnlock()
	if liveRefresh {
		a.updateProgress(func(p *Progress) {
			p.Message = "Actualizez indexul local înainte de verdict..."
			p.Detail = "Caut fișiere adăugate, mutate sau redenumite de la ultima indexare."
		})
		_, scan, err := a.refreshLiveIndexForGuard(ctx, "")
		if err != nil {
			a.logf("Index live înainte de comparație: %v", err)
		} else {
			_ = a.saveIndex()
			a.logf("Index live înainte de comparație: %d fișiere în %d locații", scan.Files, len(scan.Roots))
		}
		if ctx.Err() != nil {
			return
		}
	}
	if mode == "" {
		a.mu.RLock()
		mode = a.cfg.Mode
		a.mu.RUnlock()
	}
	a.mu.RLock()
	bySize := a.bySize
	byName := a.byName
	idx := a.index
	a.mu.RUnlock()
	res := make([]Result, 0, len(items))
	for i, it := range items {
		if ctx.Err() != nil {
			return
		}
		it.ID = i + 1
		r := Result{ID: i + 1, Remote: it, Status: "MISSING", Confidence: "—", Reason: "Nu există candidat local cu aceeași dimensiune."}
		nameKeys := byName[strings.ToLower(it.Name)]
		sameNameSameSize := ""
		sameNameDifferent := false
		for _, p := range nameKeys {
			e := idx[p]
			if e.Size == it.Size {
				sameNameSameSize = p
				break
			} else {
				sameNameDifferent = true
			}
		}
		candidates := bySize[it.Size]
		r.Candidates = len(candidates)
		if it.Hash != "" && (mode == "strict" || mode == "balanced") {
			match := ""
			for _, p := range candidates {
				h, e := a.ensureHash(p, it.HashType)
				if e == nil && strings.EqualFold(h, it.Hash) {
					match = p
					break
				}
			}
			if match != "" {
				r.Status = "VERIFIED"
				r.Confidence = "100% hash"
				r.LocalPath = match
				r.Reason = strings.ToUpper(it.HashType) + " remote = hash local."
			} else if sameNameSameSize != "" {
				r.Status = "DIFFERENT"
				r.Confidence = "100% hash"
				r.LocalPath = sameNameSameSize
				r.Reason = "Numele și dimensiunea coincid, dar hash-ul diferă."
			}
		} else if sameNameSameSize != "" {
			r.Status = "HAVE"
			r.Confidence = "Ridicată"
			r.LocalPath = sameNameSameSize
			r.NameScore = 100
			r.Reason = "Același nume și exact același număr de bytes. Nu este verificare bit-cu-bit."
		} else if sameNameDifferent {
			r.Status = "DIFFERENT"
			r.Confidence = "Ridicată"
			best := nameKeys[0]
			bestDelta := abs64(idx[best].Size - it.Size)
			for _, p := range nameKeys[1:] {
				if d := abs64(idx[p].Size - it.Size); d < bestDelta {
					best, bestDelta = p, d
				}
			}
			r.LocalPath = best
			r.NameScore = 100
			r.Reason = "Există același nume local, dar dimensiunea diferă."
		} else if len(candidates) > 0 {
			best := candidates[0]
			bestScore := nameSimilarity(it.Name, idx[best].Name)
			for _, p := range candidates[1:] {
				if sc := nameSimilarity(it.Name, idx[p].Name); sc > bestScore {
					best, bestScore = p, sc
				}
			}
			r.NameScore = bestScore
			if bestScore >= 55 {
				r.Status = "POSSIBLE"
				r.Confidence = "Medie"
				r.LocalPath = best
				r.Reason = fmt.Sprintf("Aceeași dimensiune și nume suficient de apropiat; %d candidat(ți), cel mai bun nume: %d%%.", len(candidates), bestScore)
			} else {
				r.Status = "MISSING"
				r.Confidence = "Doar mărime — nu este potrivire"
				r.Reason = fmt.Sprintf("Există %d fișier(e) cu aceeași dimensiune, dar numele sunt fără legătură (maxim %d%%). Nu sunt afișate ca POSIBIL; ExactGuard le verifică totuși înainte de download.", len(candidates), bestScore)
			}
		}
		if mode == "strict" && it.Hash == "" && r.Status == "HAVE" {
			r.Confidence = "Nedemonstrată strict"
			r.Reason += " Sursa nu oferă un hash comparabil, deci identitatea criptografică nu poate fi confirmată fără transfer."
		}
		normalizeInitialMediaResultV85(&r)
		r.AutoStatus, r.AutoConfidence, r.AutoReason = r.Status, r.Confidence, r.Reason
		a.mu.RLock()
		d, hasDecision := a.decisions[decisionKey(it)]
		a.mu.RUnlock()
		if hasDecision && (d.Status == "HAVE" || d.Status == "MISSING" || d.Status == "DIFFERENT") {
			r.Status = d.Status
			r.Manual = true
			r.ManualStatus = d.Status
			r.ManualAt = d.UpdatedAt
			r.Confidence = "Confirmat manual"
			if d.LocalPath != "" {
				r.LocalPath = d.LocalPath
			}
			r.Reason = "Decizie manuală persistentă aplicată automat la această scanare."
			if d.Note != "" {
				r.Reason += " " + d.Note
			}
		}
		enrichResult(&r, idx)
		res = append(res, r)
		if i%200 == 0 {
			a.updateProgress(func(p *Progress) {
				p.Current = int64(i)
				p.Total = int64(len(items))
				p.Files = int64(len(items))
				p.Message = fmt.Sprintf("Compar %d / %d...", i, len(items))
			})
		}
	}
	a.mu.Lock()
	a.results = res
	a.mu.Unlock()
	a.revision.Add(1)
	if err := a.saveResults(); err != nil {
		a.logf("Atenție: nu am putut salva ultima sesiune de rezultate: %v", err)
	}
}
func (a *App) ensureHash(path, kind string) (string, error) {
	a.mu.RLock()
	e, ok := a.index[path]
	a.mu.RUnlock()
	if !ok {
		return "", os.ErrNotExist
	}
	if kind == "sha256" && e.SHA256 != "" {
		return e.SHA256, nil
	}
	if kind == "md5" && e.MD5 != "" {
		return e.MD5, nil
	}
	f, er := os.Open(path)
	if er != nil {
		return "", er
	}
	defer f.Close()
	var h string
	if kind == "md5" {
		x := md5.New()
		_, er = io.Copy(x, f)
		if er == nil {
			h = hex.EncodeToString(x.Sum(nil))
			e.MD5 = h
		}
	} else {
		x := sha256.New()
		_, er = io.Copy(x, f)
		if er == nil {
			h = hex.EncodeToString(x.Sum(nil))
			e.SHA256 = h
		}
	}
	if er == nil {
		a.mu.Lock()
		a.index[path] = e
		a.mu.Unlock()
	}
	return h, er
}

func resultAutoStatus(x Result) string {
	if x.AutoStatus != "" {
		return x.AutoStatus
	}
	return x.Status
}

func resultPendingReview(x Result) bool {
	if x.Manual {
		return false
	}
	s := resultAutoStatus(x)
	// Review = există un candidat local/plauzibil, dar nu avem confirmare exactă/manuală.
	return s == "HAVE" || s == "POSSIBLE" || s == "SAMPLED"
}

func resultMatchesFilter(x Result, q, status string) bool {
	status = strings.ToUpper(strings.TrimSpace(status))
	if status != "" && status != "ALL" {
		switch status {
		case "REVIEW", "CONFIRM":
			if !resultPendingReview(x) {
				return false
			}
		case "MANUAL":
			if !x.Manual {
				return false
			}
		case "UNDECIDED":
			if x.Manual {
				return false
			}
		case "AUTO_HAVE":
			if x.Manual || resultAutoStatus(x) != "HAVE" {
				return false
			}
		case "AUTO_POSSIBLE":
			if x.Manual || resultAutoStatus(x) != "POSSIBLE" {
				return false
			}
		default:
			if x.Status != status {
				return false
			}
		}
	}
	q = strings.ToLower(strings.TrimSpace(q))
	if q != "" {
		hay := strings.ToLower(x.Remote.Path + " " + x.Remote.Name + " " + x.LocalPath + " " + x.Reason + " " + x.Confidence + " " + x.Status + " " + x.AutoStatus)
		if !strings.Contains(hay, q) {
			return false
		}
	}
	return true
}

func buildResultSummary(src []Result) map[string]any {
	effective := map[string]int{}
	auto := map[string]int{}
	workflow := map[string]int{}
	bytesByStatus := map[string]int64{}
	bytesWorkflow := map[string]int64{}
	var totalBytes int64
	for _, x := range src {
		effective[x.Status]++
		auto[resultAutoStatus(x)]++
		bytesByStatus[x.Status] += x.Remote.Size
		totalBytes += x.Remote.Size
		if x.Manual {
			workflow["MANUAL"]++
			bytesWorkflow["MANUAL"] += x.Remote.Size
			as := resultAutoStatus(x)
			if as == "HAVE" || as == "POSSIBLE" || as == "SAMPLED" {
				workflow["REVIEW_DONE"]++
				bytesWorkflow["REVIEW_DONE"] += x.Remote.Size
			}
		} else {
			workflow["UNDECIDED"]++
			bytesWorkflow["UNDECIDED"] += x.Remote.Size
			switch resultAutoStatus(x) {
			case "HAVE":
				workflow["AUTO_HAVE"]++
			case "POSSIBLE":
				workflow["AUTO_POSSIBLE"]++
			case "SAMPLED":
				workflow["AUTO_SAMPLED"]++
			}
		}
		if resultPendingReview(x) {
			workflow["REVIEW"]++
			bytesWorkflow["REVIEW"] += x.Remote.Size
		}
		if x.Status == "MISSING" {
			workflow["DOWNLOAD"]++
			bytesWorkflow["DOWNLOAD"] += x.Remote.Size
		}
		if x.Status == "DIFFERENT" {
			workflow["DIFFERENT"]++
			bytesWorkflow["DIFFERENT"] += x.Remote.Size
		}
		if x.Status == "VERIFIED" {
			workflow["VERIFIED"]++
			bytesWorkflow["VERIFIED"] += x.Remote.Size
		}
		if x.LocalPath != "" && x.SameSize {
			workflow["SAME_SIZE"]++
			if !x.Manual && x.Status != "DIFFERENT" && x.Status != "MISSING" {
				switch {
				case x.MatchScore >= 95:
					workflow["SMART_95"]++
					bytesWorkflow["SMART_95"] += x.Remote.Size
				case x.MatchScore >= 85:
					workflow["SMART_85"]++
					bytesWorkflow["SMART_85"] += x.Remote.Size
				case x.MatchScore >= 70:
					workflow["SMART_70"]++
					bytesWorkflow["SMART_70"] += x.Remote.Size
				}
			}
			if !x.Manual && x.SameExt && x.NameScore < 100 && x.NameScore >= 50 {
				workflow["RENAMED"]++
				bytesWorkflow["RENAMED"] += x.Remote.Size
			}
		}
	}
	reviewBase := workflow["REVIEW"] + workflow["REVIEW_DONE"]
	reviewPct := 100
	if reviewBase > 0 {
		reviewPct = int(math.Round(float64(workflow["REVIEW_DONE"]) * 100 / float64(reviewBase)))
	}
	return map[string]any{
		"total":         len(src),
		"totalBytes":    totalBytes,
		"effective":     effective,
		"auto":          auto,
		"workflow":      workflow,
		"bytesByStatus": bytesByStatus,
		"bytesWorkflow": bytesWorkflow,
		"reviewPercent": reviewPct,
	}
}

func resultLess(a, b Result, sortBy string) bool {
	switch sortBy {
	case "size":
		if a.Remote.Size != b.Remote.Size {
			return a.Remote.Size < b.Remote.Size
		}
	case "status":
		if a.Status != b.Status {
			return a.Status < b.Status
		}
	case "score", "match":
		if a.MatchScore != b.MatchScore {
			return a.MatchScore < b.MatchScore
		}
	case "nameScore":
		if a.NameScore != b.NameScore {
			return a.NameScore < b.NameScore
		}
	case "local":
		la, lb := strings.ToLower(a.LocalPath), strings.ToLower(b.LocalPath)
		if la != lb {
			return la < lb
		}
	default:
		pa, pb := strings.ToLower(a.Remote.Path), strings.ToLower(b.Remote.Path)
		if pa != pb {
			return pa < pb
		}
	}
	return a.ID < b.ID
}

func (a *App) handleResults(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	status := r.URL.Query().Get("status")
	sortBy := r.URL.Query().Get("sort")
	order := strings.ToLower(r.URL.Query().Get("order"))
	off, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	lim, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if lim <= 0 || lim > 1000 {
		lim = 200
	}
	a.mu.RLock()
	src := append([]Result(nil), a.results...)
	a.mu.RUnlock()
	filtered := make([]Result, 0, len(src))
	for _, x := range src {
		if resultMatchesFilter(x, q, status) {
			filtered = append(filtered, x)
		}
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		if order == "desc" {
			return resultLess(filtered[j], filtered[i], sortBy)
		}
		return resultLess(filtered[i], filtered[j], sortBy)
	})
	total := len(filtered)
	if off < 0 {
		off = 0
	}
	if off > total {
		off = total
	}
	end := off + lim
	if end > total {
		end = total
	}
	jsonOut(w, map[string]any{
		"total":    total,
		"rows":     filtered[off:end],
		"all":      len(src),
		"summary":  buildResultSummary(src),
		"revision": a.revision.Load(),
	})
}

func (a *App) handleResultsSummary(w http.ResponseWriter, r *http.Request) {
	a.mu.RLock()
	src := append([]Result(nil), a.results...)
	a.mu.RUnlock()
	jsonOut(w, map[string]any{"summary": buildResultSummary(src), "revision": a.revision.Load()})
}

type smartSelectRequest struct {
	Rule     string `json:"rule"`
	Scope    string `json:"scope"`
	Query    string `json:"query"`
	Status   string `json:"status"`
	ScoreMin int    `json:"scoreMin"`
	ScoreMax int    `json:"scoreMax"`
	NameMin  int    `json:"nameMin"`
	SameSize string `json:"sameSize"`
	SameExt  string `json:"sameExt"`
	Manual   string `json:"manual"`
	HasLocal string `json:"hasLocal"`
	Media    string `json:"media"`
	Path     string `json:"path"`
	MinSize  int64  `json:"minSize"`
	MaxSize  int64  `json:"maxSize"`
}

func boolCriterion(v string, actual bool) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "yes", "true", "1":
		return actual
	case "no", "false", "0":
		return !actual
	default:
		return true
	}
}

func smartRuleMatch(x Result, rule string) bool {
	rule = strings.ToLower(strings.TrimSpace(rule))
	switch rule {
	case "review":
		return resultPendingReview(x)
	case "very-likely":
		return !x.Manual && x.LocalPath != "" && x.SameSize && x.MatchScore >= 95 && x.Status != "DIFFERENT" && x.Status != "MISSING"
	case "likely":
		return !x.Manual && x.LocalPath != "" && x.SameSize && x.MatchScore >= 85 && x.MatchScore < 95 && x.Status != "DIFFERENT" && x.Status != "MISSING"
	case "renamed":
		return !x.Manual && x.LocalPath != "" && x.SameSize && x.SameExt && x.NameScore >= 50 && x.NameScore < 100
	case "uncertain":
		return !x.Manual && (resultAutoStatus(x) == "POSSIBLE" || resultAutoStatus(x) == "SAMPLED")
	case "missing":
		return x.Status == "MISSING"
	case "different":
		return x.Status == "DIFFERENT"
	case "manual":
		return x.Manual
	case "verified":
		return x.Status == "VERIFIED"
	case "same-size":
		return x.LocalPath != "" && x.SameSize
	case "unreviewed":
		return !x.Manual
	case "all", "filtered", "":
		return true
	default:
		return true
	}
}

func (a *App) handleSmartSelect(w http.ResponseWriter, r *http.Request) {
	var req smartSelectRequest
	if err := decodeJSON(r, &req); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if req.ScoreMin < 0 {
		req.ScoreMin = 0
	}
	if req.ScoreMax <= 0 || req.ScoreMax > 100 {
		req.ScoreMax = 100
	}
	if req.NameMin < 0 {
		req.NameMin = 0
	}
	a.mu.RLock()
	src := append([]Result(nil), a.results...)
	a.mu.RUnlock()
	ids := make([]int, 0, len(src))
	breakdown := map[string]int{}
	var bytesTotal int64
	pathNeedle := strings.ToLower(strings.TrimSpace(req.Path))
	media := strings.ToLower(strings.TrimSpace(req.Media))
	for _, x := range src {
		if strings.EqualFold(req.Scope, "filtered") && !resultMatchesFilter(x, req.Query, req.Status) {
			continue
		}
		if !smartRuleMatch(x, req.Rule) {
			continue
		}
		if x.MatchScore < req.ScoreMin || x.MatchScore > req.ScoreMax || x.NameScore < req.NameMin {
			continue
		}
		if !boolCriterion(req.SameSize, x.SameSize) || !boolCriterion(req.SameExt, x.SameExt) || !boolCriterion(req.Manual, x.Manual) || !boolCriterion(req.HasLocal, x.LocalPath != "") {
			continue
		}
		if media != "" && media != "all" {
			mk := x.MediaKind
			if mk == "" {
				mk = remoteMediaKind(x.Remote.Name)
			}
			if mk != media {
				continue
			}
		}
		if pathNeedle != "" && !strings.Contains(strings.ToLower(x.Remote.Path+" "+x.LocalPath), pathNeedle) {
			continue
		}
		if req.MinSize > 0 && x.Remote.Size < req.MinSize {
			continue
		}
		if req.MaxSize > 0 && x.Remote.Size > req.MaxSize {
			continue
		}
		ids = append(ids, x.ID)
		bytesTotal += x.Remote.Size
		breakdown[x.Status]++
	}
	jsonOut(w, map[string]any{
		"ids":       ids,
		"count":     len(ids),
		"bytes":     bytesTotal,
		"breakdown": breakdown,
		"revision":  a.revision.Load(),
	})
}

func (a *App) handleCandidates(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(r.URL.Query().Get("id"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if id <= 0 {
		http.Error(w, "ID rezultat invalid", 400)
		return
	}
	res, ok := a.resultByID(id)
	if !ok {
		http.Error(w, "Rezultatul nu mai există", 404)
		return
	}
	rows := a.candidatesFor(res.Remote, limit)
	jsonOut(w, map[string]any{"rows": rows, "total": len(rows)})
}

func (a *App) handleSelectCandidate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID   int    `json:"id"`
		Path string `json:"path"`
	}
	if err := decodeJSON(r, &req); err != nil || req.ID <= 0 || strings.TrimSpace(req.Path) == "" {
		http.Error(w, "ID/cale candidat invalidă", 400)
		return
	}
	if !a.localPathAllowed(req.Path) {
		http.Error(w, "Candidatul nu aparține indexului local", 403)
		return
	}
	a.mu.Lock()
	var out Result
	found := false
	for i := range a.results {
		if a.results[i].ID != req.ID {
			continue
		}
		e, ok := a.index[req.Path]
		if !ok {
			break
		}
		c := rankCandidate(a.results[i].Remote, e)
		a.results[i].LocalPath = req.Path
		a.results[i].NameScore = c.NameScore
		a.results[i].MatchScore = c.MatchScore
		a.results[i].SameSize = c.SameSize
		a.results[i].SameExt = c.SameExt
		a.results[i].MediaKind = remoteMediaKind(a.results[i].Remote.Name)
		candReason := fmt.Sprintf("Candidat local selectat pentru verificare • similaritate nume %d%% • diferență mărime %s.", c.NameScore, human(abs64(c.SizeDelta)))
		if a.results[i].AutoStatus == "" {
			a.results[i].AutoStatus, a.results[i].AutoConfidence, a.results[i].AutoReason = a.results[i].Status, a.results[i].Confidence, a.results[i].Reason
		}
		if a.results[i].AutoStatus == "MISSING" {
			a.results[i].AutoStatus = "POSSIBLE"
			a.results[i].AutoConfidence = "Candidat selectat"
			a.results[i].AutoReason = candReason
		}
		if !a.results[i].Manual {
			a.results[i].Status = a.results[i].AutoStatus
			a.results[i].Confidence = a.results[i].AutoConfidence
			a.results[i].Reason = a.results[i].AutoReason
		} else {
			a.results[i].Reason = "Verdictul manual rămâne activ. " + candReason
		}
		out = a.results[i]
		found = true
		break
	}
	a.mu.Unlock()
	if found {
		a.revision.Add(1)
	}
	if !found {
		http.Error(w, "Rezultatul/candidatul nu mai există", 404)
		return
	}
	_ = a.saveResults()
	jsonOut(w, out)
}

func sampleRanges(size, blockSize int64, blocks int) [][2]int64 {
	if size <= 0 {
		return nil
	}
	if blockSize <= 0 {
		blockSize = 256 << 10
	}
	if blockSize > size {
		blockSize = size
	}
	if blocks < 3 {
		blocks = 3
	}
	if blocks > 9 {
		blocks = 9
	}
	// For small files, a manual deep-check can cover the full file and become exact.
	if size <= blockSize*int64(blocks) {
		return [][2]int64{{0, size - 1}}
	}
	maxStart := size - blockSize
	seen := map[int64]bool{}
	out := make([][2]int64, 0, blocks)
	for i := 0; i < blocks; i++ {
		var start int64
		if i == 0 {
			start = 0
		} else if i == blocks-1 {
			start = maxStart
		} else {
			start = int64(math.Round(float64(maxStart) * float64(i) / float64(blocks-1)))
		}
		if seen[start] {
			continue
		}
		seen[start] = true
		out = append(out, [2]int64{start, start + blockSize - 1})
	}
	return out
}

func fetchHTTPRange(ctx context.Context, target string, start, end int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", target, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))
	req.Header.Set("User-Agent", "DuplicateDownloadGuard/6.0")
	c := http.Client{Timeout: 45 * time.Second}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	expected := end - start + 1
	if resp.StatusCode == http.StatusOK {
		if start != 0 || (resp.ContentLength > 0 && resp.ContentLength > expected) {
			return nil, fmt.Errorf("serverul nu confirmă HTTP Range (status 200 în loc de 206)")
		}
	} else if resp.StatusCode != http.StatusPartialContent {
		return nil, fmt.Errorf("serverul nu acceptă intervalul cerut (HTTP %d)", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, expected+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) != expected {
		return nil, fmt.Errorf("interval incomplet: %d/%d bytes", len(b), expected)
	}
	return b, nil
}

func (a *App) handleDeepVerify(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID   int    `json:"id"`
		Path string `json:"path"`
	}
	if err := decodeJSON(r, &req); err != nil || req.ID <= 0 {
		http.Error(w, "ID invalid", 400)
		return
	}
	res, ok := a.resultByID(req.ID)
	if !ok {
		http.Error(w, "Rezultat inexistent", 404)
		return
	}
	local := strings.TrimSpace(req.Path)
	if local == "" {
		local = res.LocalPath
	}
	if local == "" || !a.localPathAllowed(local) {
		http.Error(w, "Alege mai întâi un candidat local", 400)
		return
	}
	st, err := os.Stat(local)
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	if st.Size() != res.Remote.Size {
		http.Error(w, "Deep Verify exact pe mostre necesită aceeași dimensiune local/remote.", 409)
		return
	}
	target := strings.TrimSpace(res.Remote.DirectURL)
	if target == "" {
		target = res.Remote.URL
	}
	if strings.EqualFold(res.Remote.Source, "MEGA") {
		target, err = a.startMegaPreview(res.Remote)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
	}
	if target == "" {
		http.Error(w, "Sursa remote nu are URL utilizabil", 400)
		return
	}
	a.mu.RLock()
	blocks := a.cfg.SampleBlocks
	blockKB := a.cfg.SampleBlockKB
	a.mu.RUnlock()
	if blocks <= 0 {
		blocks = 5
	}
	if blocks > 9 {
		blocks = 9
	}
	if blockKB < 64 {
		blockKB = 64
	}
	if blockKB > 1024 {
		blockKB = 1024
	}
	ranges := sampleRanges(st.Size(), int64(blockKB)<<10, blocks)
	f, err := os.Open(local)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer f.Close()
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()
	matched := 0
	var transferred int64
	mismatchAt := -1
	for i, rg := range ranges {
		n := rg[1] - rg[0] + 1
		lb := make([]byte, n)
		if _, err := f.ReadAt(lb, rg[0]); err != nil && err != io.EOF {
			http.Error(w, err.Error(), 500)
			return
		}
		rb, err := fetchHTTPRange(ctx, target, rg[0], rg[1])
		if err != nil {
			http.Error(w, "Remote Range: "+err.Error(), 502)
			return
		}
		transferred += int64(len(rb))
		if !bytes.Equal(lb, rb) {
			mismatchAt = i
			break
		}
		matched++
	}
	full := len(ranges) == 1 && ranges[0][0] == 0 && ranges[0][1] == st.Size()-1 && mismatchAt < 0
	a.mu.Lock()
	for i := range a.results {
		if a.results[i].ID != req.ID {
			continue
		}
		a.results[i].LocalPath = local
		a.results[i].SampleMatched = matched
		a.results[i].SampleTotal = len(ranges)
		autoStatus, autoConfidence, autoReason := "SAMPLED", fmt.Sprintf("Foarte ridicată • %d/%d mostre", matched, len(ranges)), fmt.Sprintf("Deep Verify: %d blocuri distribuite în fișier coincid (%s trafic remote). Nu este verificare integrală.", matched, human(transferred))
		if mismatchAt >= 0 {
			autoStatus = "DIFFERENT"
			autoConfidence = "Mostră diferită"
			autoReason = fmt.Sprintf("Deep Verify: diferență detectată în blocul %d/%d; comparația s-a oprit.", mismatchAt+1, len(ranges))
		} else if full {
			autoStatus = "VERIFIED"
			autoConfidence = "100% conținut"
			autoReason = fmt.Sprintf("Deep Verify manual: întregul fișier (%s) a fost citit remote și comparat byte-cu-byte.", human(st.Size()))
		}
		a.results[i].AutoStatus = autoStatus
		a.results[i].AutoConfidence = autoConfidence
		a.results[i].AutoReason = autoReason
		if autoStatus == "VERIFIED" {
			a.results[i].MatchScore = 100
		} else if autoStatus == "SAMPLED" && a.results[i].MatchScore < 99 {
			a.results[i].MatchScore = 99
		} else if autoStatus == "DIFFERENT" {
			a.results[i].MatchScore = 0
		}
		if !a.results[i].Manual {
			a.results[i].Status = autoStatus
			a.results[i].Confidence = autoConfidence
			a.results[i].Reason = autoReason
		} else {
			a.results[i].Reason = "Verdict manual activ. Rezultatul automat Deep Verify: " + autoReason
		}
		res = a.results[i]
		break
	}
	a.mu.Unlock()
	a.revision.Add(1)
	_ = a.saveResults()
	jsonOut(w, map[string]any{"result": res, "matched": matched, "blocks": len(ranges), "transferred": transferred, "full": full, "different": mismatchAt >= 0})
}

func (a *App) detectFFprobe() string {
	a.mu.RLock()
	custom := strings.TrimSpace(a.cfg.FFprobePath)
	a.mu.RUnlock()
	if custom != "" {
		if _, err := os.Stat(custom); err == nil {
			return custom
		}
	}
	for _, n := range []string{"ffprobe.exe", "ffprobe"} {
		if p, err := exec.LookPath(n); err == nil {
			return p
		}
	}
	if runtime.GOOS == "windows" {
		cands := []string{
			filepath.Join(portableToolsDir(), "ffmpeg", "ffprobe.exe"),
			filepath.Join(portableToolsDir(), "ffmpeg", "bin", "ffprobe.exe"),
			filepath.Join(os.Getenv("ProgramFiles"), "FFmpeg", "bin", "ffprobe.exe"),
			filepath.Join(os.Getenv("ProgramFiles"), "ffmpeg", "bin", "ffprobe.exe"),
			`C:\ffmpeg\bin\ffprobe.exe`,
		}
		for _, x := range cands {
			if _, err := os.Stat(x); err == nil {
				return x
			}
		}
	}
	return ""
}
func probeMedia(ctx context.Context, ff, target, source string) MediaInfo {
	mi := MediaInfo{Source: source}
	cmd := exec.CommandContext(ctx, ff, "-v", "error", "-show_entries", "format=duration,format_name,bit_rate:stream=codec_type,codec_name,width,height,r_frame_rate,sample_rate,channels", "-of", "json", target)
	hideChildWindow(cmd)
	b, err := cmd.Output()
	if err != nil {
		mi.Error = err.Error()
		return mi
	}
	var raw struct {
		Format struct {
			Duration   string `json:"duration"`
			FormatName string `json:"format_name"`
			BitRate    string `json:"bit_rate"`
		} `json:"format"`
		Streams []struct {
			CodecType  string `json:"codec_type"`
			CodecName  string `json:"codec_name"`
			Width      int    `json:"width"`
			Height     int    `json:"height"`
			RFrameRate string `json:"r_frame_rate"`
			SampleRate string `json:"sample_rate"`
			Channels   int    `json:"channels"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		mi.Error = err.Error()
		return mi
	}
	mi.Duration, _ = strconv.ParseFloat(raw.Format.Duration, 64)
	mi.Format = raw.Format.FormatName
	mi.BitRate, _ = strconv.ParseInt(raw.Format.BitRate, 10, 64)
	for _, st := range raw.Streams {
		if st.CodecType == "video" && mi.VideoCodec == "" {
			mi.VideoCodec = st.CodecName
			mi.Width = st.Width
			mi.Height = st.Height
			mi.FPS = st.RFrameRate
		}
		if st.CodecType == "audio" && mi.AudioCodec == "" {
			mi.AudioCodec = st.CodecName
			mi.SampleRate = st.SampleRate
			mi.Channels = st.Channels
		}
	}
	mi.OK = true
	return mi
}
func (a *App) handleMediaCompare(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID   int    `json:"id"`
		Path string `json:"path"`
	}
	if err := decodeJSON(r, &req); err != nil || req.ID <= 0 {
		http.Error(w, "ID invalid", 400)
		return
	}
	res, ok := a.resultByID(req.ID)
	if !ok {
		http.Error(w, "Rezultat inexistent", 404)
		return
	}
	local := strings.TrimSpace(req.Path)
	if local == "" {
		local = res.LocalPath
	}
	if local == "" || !a.localPathAllowed(local) {
		http.Error(w, "Alege un candidat local", 400)
		return
	}
	ff := a.detectFFprobe()
	if ff == "" {
		http.Error(w, "ffprobe.exe nu a fost găsit. Instalează FFmpeg sau selectează ffprobe în Reguli & profiluri.", 400)
		return
	}
	target := strings.TrimSpace(res.Remote.DirectURL)
	if target == "" {
		target = res.Remote.URL
	}
	var err error
	if strings.EqualFold(res.Remote.Source, "MEGA") {
		target, err = a.startMegaPreview(res.Remote)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
	}
	if target == "" {
		http.Error(w, "Sursa remote nu are URL media", 400)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 75*time.Second)
	defer cancel()
	localInfo := probeMedia(ctx, ff, local, "LOCAL")
	remoteInfo := probeMedia(ctx, ff, target, "REMOTE")
	score, total := 0, 0
	notes := []string{}
	if localInfo.OK && remoteInfo.OK {
		if localInfo.Duration > 0 && remoteInfo.Duration > 0 {
			total++
			tol := math.Max(.25, math.Max(localInfo.Duration, remoteInfo.Duration)*.002)
			if math.Abs(localInfo.Duration-remoteInfo.Duration) <= tol {
				score++
				notes = append(notes, "durată ≈ identică")
			} else {
				notes = append(notes, fmt.Sprintf("durată diferită %.2fs vs %.2fs", remoteInfo.Duration, localInfo.Duration))
			}
		}
		if localInfo.Width > 0 && remoteInfo.Width > 0 {
			total++
			if localInfo.Width == remoteInfo.Width && localInfo.Height == remoteInfo.Height {
				score++
				notes = append(notes, "rezoluție identică")
			} else {
				notes = append(notes, "rezoluție diferită")
			}
		}
		if localInfo.VideoCodec != "" && remoteInfo.VideoCodec != "" {
			total++
			if strings.EqualFold(localInfo.VideoCodec, remoteInfo.VideoCodec) {
				score++
				notes = append(notes, "codec video identic")
			} else {
				notes = append(notes, "codec video diferit")
			}
		}
		if localInfo.AudioCodec != "" && remoteInfo.AudioCodec != "" {
			total++
			if strings.EqualFold(localInfo.AudioCodec, remoteInfo.AudioCodec) {
				score++
				notes = append(notes, "codec audio identic")
			} else {
				notes = append(notes, "codec audio diferit")
			}
		}
	}
	percent := 0
	if total > 0 {
		percent = int(math.Round(float64(score) * 100 / float64(total)))
	}
	verdict := "Date insuficiente"
	if total > 0 {
		if percent == 100 {
			verdict = "Metadate foarte apropiate"
		} else if percent >= 50 {
			verdict = "Metadate parțial apropiate"
		} else {
			verdict = "Metadate diferite"
		}
	}
	jsonOut(w, map[string]any{"local": localInfo, "remote": remoteInfo, "score": percent, "verdict": verdict, "notes": notes, "ffprobe": ff})
}

func validManualStatus(s string) bool {
	return s == "HAVE" || s == "MISSING" || s == "DIFFERENT" || s == "AUTO"
}

func (a *App) handleMark(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs    []int  `json:"ids"`
		Status string `json:"status"`
		Note   string `json:"note"`
	}
	if e := decodeJSON(r, &req); e != nil {
		http.Error(w, e.Error(), 400)
		return
	}
	req.Status = strings.ToUpper(strings.TrimSpace(req.Status))
	if !validManualStatus(req.Status) {
		http.Error(w, "status manual invalid", 400)
		return
	}
	set := map[int]bool{}
	for _, id := range req.IDs {
		set[id] = true
	}
	if len(set) == 0 {
		http.Error(w, "nu ai selectat rezultate", 400)
		return
	}
	now := time.Now().Unix()
	history := MarkHistory{}
	updated := []Result{}
	a.mu.Lock()
	for i := range a.results {
		if !set[a.results[i].ID] {
			continue
		}
		x := &a.results[i]
		key := decisionKey(x.Remote)
		prev, had := a.decisions[key]
		history.Items = append(history.Items, MarkSnapshot{ID: x.ID, Before: *x, Key: key, HadDecision: had, Decision: prev})
		if x.AutoStatus == "" {
			x.AutoStatus, x.AutoConfidence, x.AutoReason = x.Status, x.Confidence, x.Reason
		}
		if req.Status == "AUTO" {
			delete(a.decisions, key)
			x.Status = x.AutoStatus
			x.Confidence = x.AutoConfidence
			x.Reason = x.AutoReason
			x.Manual = false
			x.ManualStatus = ""
			x.ManualAt = 0
		} else {
			x.Status = req.Status
			x.Manual = true
			x.ManualStatus = req.Status
			x.ManualAt = now
			x.Confidence = "Confirmat manual"
			x.Reason = "Decizie manuală a utilizatorului; este păstrată pentru scanările viitoare."
			if strings.TrimSpace(req.Note) != "" {
				x.Reason += " " + strings.TrimSpace(req.Note)
			}
			a.decisions[key] = Decision{Status: req.Status, LocalPath: x.LocalPath, Note: strings.TrimSpace(req.Note), UpdatedAt: now}
		}
		updated = append(updated, *x)
	}
	if len(history.Items) > 0 {
		a.undoMarks = append(a.undoMarks, history)
		if len(a.undoMarks) > 50 {
			a.undoMarks = a.undoMarks[len(a.undoMarks)-50:]
		}
	}
	a.mu.Unlock()
	if len(updated) > 0 {
		a.revision.Add(1)
	}
	if len(updated) == 0 {
		http.Error(w, "rezultatele selectate nu mai există", 404)
		return
	}
	_ = a.saveDecisions()
	_ = a.saveResults()
	a.logf("Marcaj manual: %s pentru %d rezultat(e)", req.Status, len(updated))
	a.mu.RLock()
	src := append([]Result(nil), a.results...)
	a.mu.RUnlock()
	jsonOut(w, map[string]any{"ok": true, "updated": updated, "count": len(updated), "canUndo": true, "summary": buildResultSummary(src), "revision": a.revision.Load()})
}

func (a *App) handleUndoMark(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	if len(a.undoMarks) == 0 {
		a.mu.Unlock()
		http.Error(w, "nu există un marcaj de anulat", 409)
		return
	}
	h := a.undoMarks[len(a.undoMarks)-1]
	a.undoMarks = a.undoMarks[:len(a.undoMarks)-1]
	ids := make([]int, 0, len(h.Items))
	for _, snap := range h.Items {
		for i := range a.results {
			if a.results[i].ID == snap.ID {
				a.results[i] = snap.Before
				ids = append(ids, snap.ID)
				break
			}
		}
		if snap.HadDecision {
			a.decisions[snap.Key] = snap.Decision
		} else {
			delete(a.decisions, snap.Key)
		}
	}
	canUndo := len(a.undoMarks) > 0
	a.mu.Unlock()
	a.revision.Add(1)
	_ = a.saveDecisions()
	_ = a.saveResults()
	a.logf("Anulat ultimul marcaj manual (%d rezultat(e))", len(ids))
	a.mu.RLock()
	src := append([]Result(nil), a.results...)
	a.mu.RUnlock()
	jsonOut(w, map[string]any{"ok": true, "ids": ids, "count": len(ids), "canUndo": canUndo, "summary": buildResultSummary(src), "revision": a.revision.Load()})
}

func (a *App) handleOpenDataFolder(w http.ResponseWriter, r *http.Request) {
	if runtime.GOOS == "windows" {
		if err := exec.Command("explorer.exe", a.appDir).Start(); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
	}
	jsonOut(w, map[string]any{"ok": true, "path": a.appDir})
}

func (a *App) handleClearResults(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	a.results = nil
	a.mu.Unlock()
	a.revision.Add(1)
	_ = os.Remove(a.resultsPath())
	jsonOut(w, map[string]bool{"ok": true})
}
func (a *App) handleOpenLocal(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	_ = decodeJSON(r, &req)
	if req.Path == "" {
		http.Error(w, "path lipsă", 400)
		return
	}
	if runtime.GOOS == "windows" {
		exec.Command("explorer.exe", "/select,", req.Path).Start()
	}
	jsonOut(w, map[string]bool{"ok": true})
}

func (a *App) localPathAllowed(p string) bool {
	if p == "" {
		return false
	}
	ap, err := filepath.Abs(filepath.Clean(p))
	if err != nil {
		return false
	}
	a.mu.RLock()
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

func (a *App) handleOpenLocalPlayer(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if e := decodeJSON(r, &req); e != nil || req.Path == "" {
		http.Error(w, "path lipsă", 400)
		return
	}
	if !a.localPathAllowed(req.Path) {
		http.Error(w, "Fișierul nu aparține indexului local curent", 403)
		return
	}
	a.mu.RLock()
	player := strings.TrimSpace(a.cfg.PlayerPath)
	a.mu.RUnlock()
	var err error
	if runtime.GOOS == "windows" {
		if player != "" {
			if _, e := os.Stat(player); e == nil {
				err = exec.Command(player, req.Path).Start()
			} else {
				err = fmt.Errorf("playerul configurat nu există: %s", player)
			}
		} else {
			// Windows opens the file with the application associated with its extension.
			cmd := exec.Command("cmd.exe", "/C", "start", "", req.Path)
			hideChildWindow(cmd)
			err = cmd.Start()
		}
	} else if runtime.GOOS == "darwin" {
		err = exec.Command("open", req.Path).Start()
	} else {
		err = exec.Command("xdg-open", req.Path).Start()
	}
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	jsonOut(w, map[string]bool{"ok": true})
}

func (a *App) handleOpenRemote(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL    string `json:"url"`
		Handle string `json:"handle"`
		Source string `json:"source"`
	}
	if e := decodeJSON(r, &req); e != nil {
		http.Error(w, e.Error(), 400)
		return
	}
	u := strings.TrimSpace(req.URL)
	if u == "" {
		http.Error(w, "URL remote lipsă", 400)
		return
	}
	if strings.EqualFold(req.Source, "MEGA") && req.Handle != "" {
		u = megaItemURL(u, req.Handle)
	}
	pu, err := url.Parse(u)
	if err != nil || (pu.Scheme != "https" && pu.Scheme != "http") {
		http.Error(w, "URL remote invalid", 400)
		return
	}
	openBrowser(u)
	jsonOut(w, map[string]string{"ok": "true", "url": u})
}

var webdavURLRE = regexp.MustCompile(`https?://(?:127\.0\.0\.1|localhost):\d+/[^\s]+`)

func extractWebDAVURL(out, remotePath string) string {
	remotePath = strings.TrimSpace(remotePath)
	for _, line := range strings.Split(strings.ReplaceAll(out, "\r", ""), "\n") {
		if remotePath != "" && !strings.Contains(line, remotePath) {
			continue
		}
		if u := webdavURLRE.FindString(line); u != "" {
			return strings.TrimSpace(u)
		}
	}
	return webdavURLRE.FindString(out)
}

func megaRemoteRef(item RemoteItem) string {
	h := strings.TrimSpace(item.Handle)
	if h != "" {
		// MEGAcmd officially supports addressing a node by H:HANDLE. This is
		// much more reliable than replaying a path parsed from a recursive list,
		// especially for public folders and duplicate/renamed filenames.
		return "H:" + h
	}
	return strings.TrimSpace(item.Path)
}

func remoteMediaKind(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp", ".avif":
		return "image"
	case ".mp4", ".webm", ".ogv", ".mov", ".m4v", ".mkv", ".avi", ".flv", ".ts", ".mts", ".m2ts":
		return "video"
	case ".mp3", ".wav", ".ogg", ".m4a", ".aac", ".flac", ".opus":
		return "audio"
	default:
		return "other"
	}
}

func (a *App) resultByID(id int) (Result, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	for _, r := range a.results {
		if r.ID == id {
			return r, true
		}
	}
	return Result{}, false
}

func (a *App) resetPreviewTTLLocked() {
	if a.previewTTL != nil {
		a.previewTTL.Stop()
	}
	a.previewTTL = time.AfterFunc(60*time.Minute, func() {
		if err := a.stopMegaPreview("timeout 60 minute"); err != nil {
			a.logf("MEGA preview cleanup timeout: %v", err)
		}
	})
}

func (a *App) restoreMegaSessionSilent(exe, oldSession string) {
	ctx := context.Background()
	_, _ = runMegaTimed(ctx, 10*time.Second, exe, "logout")
	if oldSession != "" {
		if _, err := runMegaTimed(ctx, 30*time.Second, exe, "login", oldSession); err != nil {
			a.logf("MEGA preview: sesiunea anterioară nu a putut fi restaurată: %v", err)
		} else {
			a.logf("MEGA preview: sesiunea anterioară restaurată")
		}
	}
}

func (a *App) stopMegaPreviewLocked(reason string) error {
	if !a.preview.Active {
		return nil
	}
	st := a.preview
	if a.previewTTL != nil {
		a.previewTTL.Stop()
		a.previewTTL = nil
	}
	var firstErr error
	ctx := context.Background()
	if st.Exe != "" && st.RemotePath != "" {
		if out, err := runMegaTimed(ctx, 12*time.Second, st.Exe, "webdav", "-d", st.RemotePath); err != nil {
			a.logf("MEGA preview: oprire WebDAV (%s): %v • %s", reason, err, sanitizeMega(out))
			firstErr = err
		}
	}
	if st.Exe != "" {
		a.restoreMegaSessionSilent(st.Exe, st.PreviousSession)
	}
	a.preview = MegaPreviewState{}
	a.logf("MEGA preview oprit (%s)", reason)
	return firstErr
}

func (a *App) stopMegaPreview(reason string) error {
	// Fast path: most Stop calls arrive after another MEGA operation has already
	// cleaned the preview. Do not wait for a long download when there is nothing
	// left to stop.
	a.previewMu.Lock()
	active := a.preview.Active
	a.previewMu.Unlock()
	if !active {
		return nil
	}
	gateCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := acquireMegaSession(gateCtx); err != nil {
		return fmt.Errorf("MEGA este ocupat cu altă operație; preview-ul nu a putut fi oprit încă: %w", err)
	}
	defer releaseMegaSession()
	a.previewMu.Lock()
	defer a.previewMu.Unlock()
	return a.stopMegaPreviewLocked(reason)
}

func (a *App) startMegaPreview(item RemoteItem) (string, error) {
	gateCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := acquireMegaSession(gateCtx); err != nil {
		return "", fmt.Errorf("MEGA este ocupat cu scanare sau download; încearcă preview-ul din nou după terminarea operației: %w", err)
	}
	defer releaseMegaSession()
	a.previewMu.Lock()
	defer a.previewMu.Unlock()

	remoteRef := megaRemoteRef(item)
	if remoteRef == "" {
		return "", errors.New("fișierul MEGA nu are nici handle, nici cale remote utilizabilă")
	}

	// Fast path: the scan already serves the whole public folder through one
	// warm WebDAV root. Derive the selected child URL locally and avoid a new
	// MEGAcmd webdav command for every row click.
	if a.preview.Active && a.preview.SourceURL == item.URL && a.preview.RemotePath == megaWarmRootRefV86 && a.preview.StreamURL != "" {
		if streamURL, ok := warmRootPreviewURLV86(a.preview, item); ok {
			a.resetPreviewTTLLocked()
			a.logf("MEGA Fast Preview hit: %s -> %s", item.Path, streamURL)
			return streamURL, nil
		}
		a.logf("MEGA Fast Preview miss pentru %s; folosesc WebDAV per fișier", item.Path)
	}

	if a.preview.Active && a.preview.SourceURL == item.URL && a.preview.RemotePath == remoteRef && a.preview.StreamURL != "" {
		a.resetPreviewTTLLocked()
		return a.preview.StreamURL, nil
	}

	// Same public folder: start and validate the new WebDAV node before stopping
	// the old one so a failed switch never interrupts the currently playing file.
	if a.preview.Active && a.preview.SourceURL == item.URL && a.preview.Exe != "" {
		old := a.preview
		streamURL, err := a.switchMegaPreviewSameSourceV85(old, remoteRef)
		if err != nil {
			return "", err
		}
		a.preview = MegaPreviewState{Active: true, SourceURL: item.URL, RemotePath: remoteRef, StreamURL: streamURL, PreviousSession: old.PreviousSession, Exe: old.Exe}
		a.resetPreviewTTLLocked()
		a.logf("MEGA preview: %s [%s] -> %s", item.Path, remoteRef, streamURL)
		return streamURL, nil
	}

	if a.preview.Active {
		_ = a.stopMegaPreviewLocked("schimbare sursă")
	}

	exe := a.detectMegaClient()
	if exe == "" {
		return "", errors.New("MEGAcmd nu a fost găsit")
	}
	ctx := context.Background()
	oldSession := ""
	if out, err := runMegaTimed(ctx, 10*time.Second, exe, "session"); err == nil {
		oldSession = extractSession(out)
	}
	if oldSession != "" {
		_, _ = runMegaTimed(ctx, 10*time.Second, exe, "logout", "--keep-session")
	} else {
		_, _ = runMegaTimed(ctx, 10*time.Second, exe, "logout")
	}
	loginOut, err := runMegaTimed(ctx, 45*time.Second, exe, "login", item.URL)
	if err != nil {
		a.restoreMegaSessionSilent(exe, oldSession)
		problem := classifyMegaProblem(loginOut, err)
		return "", newMegaProblemError(problem, loginOut)
	}
	out, err := runMegaTimed(ctx, 30*time.Second, exe, "webdav", remoteRef)
	if err != nil {
		a.restoreMegaSessionSilent(exe, oldSession)
		problem := classifyMegaProblem(out, err)
		return "", newMegaProblemError(problem, out)
	}
	streamURL := extractWebDAVURL(out, remoteRef)
	if streamURL == "" {
		listing, _ := runMegaTimed(ctx, 10*time.Second, exe, "webdav")
		streamURL = extractWebDAVURL(listing, remoteRef)
	}
	if streamURL == "" {
		_, _ = runMegaTimed(ctx, 10*time.Second, exe, "webdav", "-d", remoteRef)
		a.restoreMegaSessionSilent(exe, oldSession)
		return "", errors.New("MEGAcmd a activat WebDAV, dar nu a returnat URL-ul de streaming")
	}
	a.preview = MegaPreviewState{Active: true, SourceURL: item.URL, RemotePath: remoteRef, StreamURL: streamURL, PreviousSession: oldSession, Exe: exe}
	a.resetPreviewTTLLocked()
	a.logf("MEGA preview pornit: %s [%s] -> %s", item.Path, remoteRef, streamURL)
	return streamURL, nil
}

func (a *App) handleRemotePreviewStart(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID            int  `json:"id"`
		ForceFallback bool `json:"forceFallback,omitempty"`
	}
	if err := decodeJSON(r, &req); err != nil || req.ID <= 0 {
		http.Error(w, "ID rezultat invalid", 400)
		return
	}
	res, ok := a.resultByID(req.ID)
	if !ok {
		http.Error(w, "Rezultatul nu mai există", 404)
		return
	}
	kind := remoteMediaKind(res.Remote.Name)
	if !strings.EqualFold(res.Remote.Source, "MEGA") {
		previewURL := strings.TrimSpace(res.Remote.DirectURL)
		if previewURL == "" {
			previewURL = res.Remote.URL
		}
		pu, err := url.Parse(previewURL)
		if err != nil || (pu.Scheme != "http" && pu.Scheme != "https") {
			http.Error(w, "Sursa remote nu poate fi previzualizată direct", 400)
			return
		}
		jsonOut(w, map[string]any{"url": previewURL, "kind": kind, "streaming": true, "source": res.Remote.Source})
		return
	}
	if kind == "other" {
		http.Error(w, "Formatul nu are preview media integrat", 415)
		return
	}
	streamURL, previewMode, prepareDuration, err := a.startMegaPreviewForUIV854(res.Remote, req.ForceFallback)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	jsonOut(w, map[string]any{
		"url":         streamURL,
		"kind":        kind,
		"streaming":   true,
		"source":      previewMode,
		"previewMode": previewMode,
		"prepareMs":   prepareDuration.Milliseconds(),
		"note":        "Fast-path-ul UI reutilizează WebDAV-ul pregătit la scanare fără comandă MEGAcmd suplimentară. Fallback-ul per-fișier rămâne disponibil dacă nu există cache.",
	})
}

func (a *App) handleRemotePreviewStop(w http.ResponseWriter, r *http.Request) {
	err := a.stopMegaPreview("cerere UI")
	if err != nil {
		jsonOut(w, map[string]any{"ok": true, "warning": err.Error()})
		return
	}
	jsonOut(w, map[string]bool{"ok": true})
}

func (a *App) handleRemotePreviewPlayer(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID int `json:"id"`
	}
	if err := decodeJSON(r, &req); err != nil || req.ID <= 0 {
		http.Error(w, "ID rezultat invalid", 400)
		return
	}
	res, ok := a.resultByID(req.ID)
	if !ok {
		http.Error(w, "Rezultatul nu mai există", 404)
		return
	}
	target := strings.TrimSpace(res.Remote.DirectURL)
	if target == "" {
		target = res.Remote.URL
	}
	if strings.EqualFold(res.Remote.Source, "MEGA") {
		var err error
		target, err = a.startMegaPreview(res.Remote)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
	}
	a.mu.RLock()
	player := strings.TrimSpace(a.cfg.PlayerPath)
	a.mu.RUnlock()
	var err error
	if runtime.GOOS == "windows" && player != "" {
		if _, statErr := os.Stat(player); statErr == nil {
			err = exec.Command(player, target).Start()
		} else {
			err = fmt.Errorf("playerul configurat nu există: %s", player)
		}
	} else if runtime.GOOS == "darwin" {
		err = exec.Command("open", target).Start()
	} else if runtime.GOOS != "windows" {
		err = exec.Command("xdg-open", target).Start()
	} else {
		openBrowser(target)
	}
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	jsonOut(w, map[string]any{"ok": true, "url": target})
}

func (a *App) handleLocalPreview(w http.ResponseWriter, r *http.Request) {
	p := strings.TrimSpace(r.URL.Query().Get("path"))
	if p == "" {
		http.Error(w, "path lipsă", 400)
		return
	}
	if !a.localPathAllowed(p) {
		http.Error(w, "Fișier local neautorizat", 403)
		return
	}
	st, err := os.Stat(p)
	if err != nil || st.IsDir() {
		http.Error(w, "Fișierul local nu mai există", 404)
		return
	}
	w.Header().Set("Content-Disposition", "inline")
	w.Header().Set("Cache-Control", "private, max-age=60")
	http.ServeFile(w, r, p)
}

func (a *App) handleLocalMeta(w http.ResponseWriter, r *http.Request) {
	p := strings.TrimSpace(r.URL.Query().Get("path"))
	if p == "" || !a.localPathAllowed(p) {
		http.Error(w, "path local invalid", 400)
		return
	}
	st, err := os.Stat(p)
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	a.mu.RLock()
	e := a.index[p]
	a.mu.RUnlock()
	jsonOut(w, map[string]any{
		"path":      p,
		"name":      filepath.Base(p),
		"extension": strings.ToLower(filepath.Ext(p)),
		"bytes":     st.Size(),
		"size":      human(st.Size()),
		"modified":  st.ModTime().Format("2006-01-02 15:04:05"),
		"sha256":    e.SHA256,
		"md5":       e.MD5,
	})
}
func (a *App) handleLogs(w http.ResponseWriter, r *http.Request) {
	a.mu.RLock()
	l := append([]string(nil), a.logs...)
	a.mu.RUnlock()
	jsonOut(w, l)
}
func (a *App) handleIndexStats(w http.ResponseWriter, r *http.Request) {
	a.mu.RLock()
	n := len(a.index)
	var bytes int64
	for _, e := range a.index {
		bytes += e.Size
	}
	a.mu.RUnlock()
	jsonOut(w, map[string]any{"files": n, "bytes": bytes, "size": human(bytes), "path": a.indexPath()})
}

func (a *App) handleExportCSV(w http.ResponseWriter, r *http.Request) {
	a.mu.RLock()
	rows := append([]Result(nil), a.results...)
	a.mu.RUnlock()
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=DuplicateGuard_results.csv")
	w.Write([]byte{0xEF, 0xBB, 0xBF})
	cw := csv.NewWriter(w)
	defer cw.Flush()
	cw.Write([]string{"status", "manual", "auto_status", "confidence", "remote_path", "bytes", "local_path", "candidates", "reason", "source", "url"})
	for _, x := range rows {
		cw.Write([]string{x.Status, strconv.FormatBool(x.Manual), x.AutoStatus, x.Confidence, x.Remote.Path, strconv.FormatInt(x.Remote.Size, 10), x.LocalPath, strconv.Itoa(x.Candidates), x.Reason, x.Remote.Source, x.Remote.URL})
	}
}
func (a *App) handleExportMissing(w http.ResponseWriter, r *http.Request) {
	a.mu.RLock()
	rows := append([]Result(nil), a.results...)
	destination := a.cfg.DownloadDir
	a.mu.RUnlock()
	guardRows := make([]Result, 0, len(rows))
	for _, x := range rows {
		if x.Status == "MISSING" {
			guardRows = append(guardRows, x)
		}
	}
	allowed := map[int]bool{}
	if len(guardRows) > 0 {
		report, err := a.runDownloadGuard(r.Context(), guardRows, destination, "")
		if err != nil {
			http.Error(w, "ExactGuard: "+err.Error(), http.StatusConflict)
			return
		}
		for _, decision := range report.Decisions {
			if decision.Verdict == guardDownload {
				allowed[decision.ResultID] = true
			}
		}
	}
	format := r.URL.Query().Get("format")
	if format == "jd2" {
		w.Header().Set("Content-Disposition", "attachment; filename=missing_links.crawljob")
	} else {
		w.Header().Set("Content-Disposition", "attachment; filename=missing_links.txt")
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	seen := map[string]bool{}
	for _, x := range rows {
		if x.Status != "MISSING" || !allowed[x.ID] {
			continue
		}
		u := resultDownloadURL(x)
		if u != "" && !seen[u] {
			fmt.Fprintln(w, u)
			seen[u] = true
		}
	}
}

func human(n int64) string {
	f := float64(n)
	u := []string{"B", "KB", "MB", "GB", "TB", "PB"}
	i := 0
	for f >= 1024 && i < len(u)-1 {
		f /= 1024
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%d %s", n, u[i])
	}
	return fmt.Sprintf("%.2f %s", f, u[i])
}
func openAppWindow(u string) {
	if runtime.GOOS == "windows" {
		cands := []string{
			filepath.Join(os.Getenv("ProgramFiles(x86)"), "Microsoft", "Edge", "Application", "msedge.exe"),
			filepath.Join(os.Getenv("ProgramFiles"), "Microsoft", "Edge", "Application", "msedge.exe"),
			filepath.Join(os.Getenv("LOCALAPPDATA"), "Microsoft", "Edge", "Application", "msedge.exe"),
		}
		for _, edge := range cands {
			if edge == "" {
				continue
			}
			if _, err := os.Stat(edge); err == nil {
				if exec.Command(edge, "--app="+u, "--start-maximized").Start() == nil {
					return
				}
			}
		}
		if edge, err := exec.LookPath("msedge.exe"); err == nil {
			if exec.Command(edge, "--app="+u, "--start-maximized").Start() == nil {
				return
			}
		}
	}
	openBrowser(u)
}

func openBrowser(u string) {
	if runtime.GOOS == "windows" {
		exec.Command("rundll32", "url.dll,FileProtocolHandler", u).Start()
	} else if runtime.GOOS == "darwin" {
		exec.Command("open", u).Start()
	} else {
		exec.Command("xdg-open", u).Start()
	}
}
