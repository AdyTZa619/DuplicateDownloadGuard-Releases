package main

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	detectorSelfTestServiceV901      = "ddg-detector-self-test-v901"
	detectorSelfTestPortBaseV901     = 41450
	detectorSelfTestPortSpanV901     = 200
	detectorSelfTestPortAttemptsV901 = 8
)

func init() {
	if strings.Contains(strings.ToLower(filepath.Base(os.Args[0])), ".test") {
		return
	}
	go startDetectorSelfTestServiceV901()
}

func detectorSelfTestDataDirV901() string {
	dir, err := portableDataDir()
	if err == nil && strings.TrimSpace(dir) != "" {
		return filepath.Clean(dir)
	}
	return filepath.Clean(filepath.Join(executableDir(), "data"))
}

func detectorSelfTestPortSeedV901(dataDir string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.ToLower(filepath.Clean(dataDir))))
	return detectorSelfTestPortBaseV901 + int(h.Sum32()%detectorSelfTestPortSpanV901)
}

func detectorSelfTestCORSV901(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func detectorSelfTestHostAppV901(dataDir string) *App {
	cfg := Config{}
	if b, err := os.ReadFile(filepath.Join(dataDir, "config.json")); err == nil {
		_ = json.Unmarshal(b, &cfg)
	}
	return &App{appDir: dataDir, cfg: cfg, index: map[string]FileEntry{}, bySize: map[int64][]string{}, byName: map[string][]string{}, byIdentity: map[string][]string{}, decisions: map[string]Decision{}}
}

func startDetectorSelfTestServiceV901() {
	dataDir := detectorSelfTestDataDirV901()
	_ = os.MkdirAll(dataDir, 0755)
	base := detectorSelfTestPortSeedV901(dataDir)
	var listener net.Listener
	for i := 0; i < detectorSelfTestPortAttemptsV901; i++ {
		candidate, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", base+i))
		if err == nil {
			listener = candidate
			break
		}
	}
	if listener == nil {
		return
	}
	host := detectorSelfTestHostAppV901(dataDir)
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		jsonOut(w, map[string]any{"ok": true, "service": detectorSelfTestServiceV901})
	})
	mux.HandleFunc("/state", host.handleDetectorSelfTestV901)
	mux.HandleFunc("/start", host.handleDetectorSelfTestV901)
	server := &http.Server{Handler: detectorSelfTestCORSV901(mux), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	_ = server.Serve(listener)
}

type detectorSelfTestCaseV901 struct {
	Name        string                `json:"name"`
	Expected    string                `json:"expected"`
	Actual      string                `json:"actual"`
	Passed      bool                  `json:"passed"`
	ColdMS      int64                 `json:"coldMs"`
	WarmMS      int64                 `json:"warmMs"`
	RemoteBytes int64                 `json:"remoteBytes"`
	Method      string                `json:"method"`
	Reason      string                `json:"reason"`
	Decision    DownloadGuardDecision `json:"decision"`
}

type detectorSelfTestReportV901 struct {
	Version       string                     `json:"version"`
	StartedAt     string                     `json:"startedAt"`
	FinishedAt    string                     `json:"finishedAt"`
	FFmpeg        string                     `json:"ffmpeg"`
	FFprobe       string                     `json:"ffprobe"`
	Passed        bool                       `json:"passed"`
	PassedCases   int                        `json:"passedCases"`
	TotalCases    int                        `json:"totalCases"`
	JDGuardPassed bool                       `json:"jdGuardPassed"`
	Cases         []detectorSelfTestCaseV901 `json:"cases"`
	Limitations   []string                   `json:"limitations"`
}

type detectorSelfTestStateV901 struct {
	Running    bool                        `json:"running"`
	Phase      string                      `json:"phase"`
	Message    string                      `json:"message"`
	Step       int                         `json:"step"`
	StepTotal  int                         `json:"stepTotal"`
	Error      string                      `json:"error,omitempty"`
	ReportPath string                      `json:"reportPath,omitempty"`
	Report     *detectorSelfTestReportV901 `json:"report,omitempty"`
}

var detectorSelfTestV901 = struct {
	sync.RWMutex
	state detectorSelfTestStateV901
}{}

func detectorSelfTestSnapshotV901() detectorSelfTestStateV901 {
	detectorSelfTestV901.RLock()
	defer detectorSelfTestV901.RUnlock()
	return detectorSelfTestV901.state
}

func detectorSelfTestUpdateV901(fn func(*detectorSelfTestStateV901)) {
	detectorSelfTestV901.Lock()
	defer detectorSelfTestV901.Unlock()
	fn(&detectorSelfTestV901.state)
}

func (a *App) handleDetectorSelfTestV901(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		jsonOut(w, detectorSelfTestSnapshotV901())
	case http.MethodPost:
		detectorSelfTestV901.Lock()
		if detectorSelfTestV901.state.Running {
			detectorSelfTestV901.Unlock()
			http.Error(w, "Autotestul detectorului rulează deja", http.StatusConflict)
			return
		}
		detectorSelfTestV901.state = detectorSelfTestStateV901{Running: true, Phase: "prepare", Message: "Pregătesc corpusul media…", Step: 1, StepTotal: 6}
		detectorSelfTestV901.Unlock()
		go a.runDetectorSelfTestV901()
		jsonOut(w, detectorSelfTestSnapshotV901())
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func runSelfTestFFmpegV901(ctx context.Context, ff string, args ...string) error {
	cmd := exec.CommandContext(ctx, ff, append([]string{"-hide_banner", "-loglevel", "error", "-y"}, args...)...)
	hideChildWindow(cmd)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("FFmpeg: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func selfTestAppV901(root, download, appDir, ff, fp string, remote RemoteItem) *App {
	a := &App{
		appDir: appDir, index: map[string]FileEntry{}, bySize: map[int64][]string{}, byName: map[string][]string{}, byIdentity: map[string][]string{}, decisions: map[string]Decision{},
		cfg: Config{LocalPaths: []string{root}, DownloadDir: download, DownloadGuardMode: guardModeSmart, FullVerifyMaxMB: 12, SampleBlocks: 9, SampleBlockKB: 64, DownloadRetries: 1, DownloadConcurrency: 1, FFmpegPath: ff, FFprobePath: fp},
	}
	a.results = []Result{{ID: 1, Status: "MISSING", AutoStatus: "MISSING", Remote: remote}}
	return a
}

func detectorClassificationV901(d DownloadGuardDecision) string {
	if d.Detector == nil {
		return "FĂRĂ DOVADĂ"
	}
	return d.Detector.Classification
}

func (a *App) runDetectorSelfTestV901() {
	fail := func(err error) {
		a.logf("DDG Self-Test eșuat: %v", err)
		detectorSelfTestUpdateV901(func(s *detectorSelfTestStateV901) {
			s.Running = false
			s.Phase = "error"
			s.Message = "Autotest eșuat"
			s.Error = err.Error()
		})
	}
	ff, fp := a.detectFFmpeg(), a.detectFFprobe()
	if ff == "" || fp == "" {
		fail(fmt.Errorf("FFmpeg și ffprobe sunt necesare; instalează pachetul FFmpeg din Tool Manager"))
		return
	}
	work, err := os.MkdirTemp("", "ddg-detector-selftest-")
	if err != nil {
		fail(err)
		return
	}
	defer os.RemoveAll(work)
	localDir, remoteDir, downloadDir := filepath.Join(work, "local"), filepath.Join(work, "remote"), filepath.Join(work, "download")
	for _, p := range []string{localDir, remoteDir, downloadDir} {
		if err := os.MkdirAll(p, 0755); err != nil {
			fail(err)
			return
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	source := filepath.Join(localDir, "ORIGINAL_LOCAL.mp4")
	exact := filepath.Join(remoteDir, "NUME_COMPLET_DIFERIT.mp4")
	recode := filepath.Join(remoteDir, "VERSIUNE_RECODATA.mp4")
	different := filepath.Join(remoteDir, "VIDEO_LIPSA.mp4")
	if err := runSelfTestFFmpegV901(ctx, ff, "-f", "lavfi", "-i", "testsrc2=s=640x360:r=15:d=18", "-f", "lavfi", "-i", "sine=frequency=523:sample_rate=44100:duration=18", "-c:v", "libx264", "-preset", "veryfast", "-crf", "20", "-pix_fmt", "yuv420p", "-c:a", "aac", "-shortest", "-movflags", "+faststart", source); err != nil {
		fail(err)
		return
	}
	b, err := os.ReadFile(source)
	if err != nil {
		fail(err)
		return
	}
	if err := os.WriteFile(exact, b, 0644); err != nil {
		fail(err)
		return
	}
	detectorSelfTestUpdateV901(func(s *detectorSelfTestStateV901) { s.Step = 2; s.Message = "Generez versiunea recodată…" })
	if err := runSelfTestFFmpegV901(ctx, ff, "-i", source, "-vf", "scale=426:240", "-c:v", "libx264", "-preset", "veryfast", "-crf", "30", "-c:a", "aac", "-b:a", "96k", "-movflags", "+faststart", recode); err != nil {
		fail(err)
		return
	}
	if err := runSelfTestFFmpegV901(ctx, ff, "-f", "lavfi", "-i", "smptebars=s=640x360:r=15:d=18", "-f", "lavfi", "-i", "sine=frequency=880:sample_rate=44100:duration=18", "-c:v", "libx264", "-preset", "veryfast", "-crf", "20", "-pix_fmt", "yuv420p", "-c:a", "aac", "-shortest", "-movflags", "+faststart", different); err != nil {
		fail(err)
		return
	}
	server := httptest.NewServer(http.FileServer(http.Dir(remoteDir)))
	defer server.Close()

	report := &detectorSelfTestReportV901{Version: appVersion, StartedAt: time.Now().Format(time.RFC3339), FFmpeg: toolVersion(ff), FFprobe: toolVersion(fp), TotalCases: 3, Limitations: []string{"Autotestul folosește HTTP local și nu autentifică un cont MEGA/Bunkr real.", "JDownloader este verificat la poarta fail-closed fără a trimite sau descărca fișiere."}}
	cases := []struct{ name, file, expected string }{
		{"Copie identică redenumită", exact, "IDENTIC"},
		{"Același video recodat și redimensionat", recode, "ACELAȘI CONȚINUT"},
		{"Video diferit cu durată identică", different, "LIPSĂ"},
	}
	for i, tc := range cases {
		detectorSelfTestUpdateV901(func(s *detectorSelfTestStateV901) {
			s.Step = 3 + i
			s.Phase = "detector"
			s.Message = "Testez: " + tc.name
		})
		st, statErr := os.Stat(tc.file)
		if statErr != nil {
			fail(statErr)
			return
		}
		remote := RemoteItem{Name: filepath.Base(tc.file), Size: st.Size(), Source: "HTTP", DirectURL: server.URL + "/" + url.PathEscape(filepath.Base(tc.file))}
		caseDir := filepath.Join(work, fmt.Sprintf("case-%d", i))
		if err := os.MkdirAll(caseDir, 0755); err != nil {
			fail(err)
			return
		}
		testApp := selfTestAppV901(localDir, downloadDir, caseDir, ff, fp, remote)
		testApp.runIndex(ctx, []string{localDir}, "", 0)
		started := time.Now()
		cold, guardErr := testApp.runDownloadGuard(ctx, testApp.results, downloadDir, guardModeSmart)
		coldMS := time.Since(started).Milliseconds()
		if guardErr != nil {
			fail(fmt.Errorf("%s: %w", tc.name, guardErr))
			return
		}
		started = time.Now()
		warm, guardErr := testApp.runDownloadGuard(ctx, testApp.results, downloadDir, guardModeSmart)
		warmMS := time.Since(started).Milliseconds()
		if guardErr != nil || len(warm.Decisions) != 1 || len(cold.Decisions) != 1 {
			fail(fmt.Errorf("%s: raport detector invalid", tc.name))
			return
		}
		d := warm.Decisions[0]
		actual := detectorClassificationV901(d)
		row := detectorSelfTestCaseV901{Name: tc.name, Expected: tc.expected, Actual: actual, Passed: actual == tc.expected, ColdMS: coldMS, WarmMS: warmMS, Method: d.Method, Reason: d.Reason, Decision: d}
		if d.Detector != nil {
			row.RemoteBytes = d.Detector.RemoteBytes
		}
		report.Cases = append(report.Cases, row)
		if row.Passed {
			report.PassedCases++
		}
	}
	missingDecision := report.Cases[2].Decision
	duplicateDecision := report.Cases[0].Decision
	reviewDecision := DownloadGuardDecision{Verdict: guardReview, Action: actionReview, Detector: &DuplicateEvidenceV90{Classification: "DE VERIFICAT"}}
	report.JDGuardPassed = jdownloaderDecisionAllowedV901(missingDecision) && !jdownloaderDecisionAllowedV901(duplicateDecision) && !jdownloaderDecisionAllowedV901(reviewDecision)
	report.Passed = report.PassedCases == report.TotalCases && report.JDGuardPassed
	report.FinishedAt = time.Now().Format(time.RFC3339)
	reportPath := filepath.Join(a.appDir, "DDG_Detector_SelfTest.json")
	out, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		fail(err)
		return
	}
	if err := os.WriteFile(reportPath, out, 0644); err != nil {
		fail(err)
		return
	}
	a.logf("DDG Self-Test: %d/%d cazuri, JD Guard=%t, rezultat=%t", report.PassedCases, report.TotalCases, report.JDGuardPassed, report.Passed)
	detectorSelfTestUpdateV901(func(s *detectorSelfTestStateV901) {
		s.Running = false
		s.Phase = "done"
		s.Step = 6
		s.Message = fmt.Sprintf("Autotest terminat: %d/%d cazuri", report.PassedCases, report.TotalCases)
		s.ReportPath = reportPath
		s.Report = report
	})
}
