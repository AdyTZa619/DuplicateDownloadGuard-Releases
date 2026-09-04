package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type DownloadDiagnosticCheckV860 struct {
	Name       string `json:"name"`
	Status     string `json:"status"` // pass|warn|fail|skip
	Detail     string `json:"detail"`
	Action     string `json:"action,omitempty"`
	DurationMs int64  `json:"durationMs"`
}

type DownloadDiagnosticReportV860 struct {
	Overall  string                        `json:"overall"`
	Checks   []DownloadDiagnosticCheckV860 `json:"checks"`
	ResultID int                           `json:"resultId,omitempty"`
	Source   string                        `json:"source,omitempty"`
	Engine   string                        `json:"engine,omitempty"`
	At       int64                         `json:"at"`
}

func diagnosticCheckV860(name string, fn func() (string, error)) DownloadDiagnosticCheckV860 {
	started := time.Now()
	detail, err := fn()
	row := DownloadDiagnosticCheckV860{Name: name, Status: "pass", Detail: detail, DurationMs: time.Since(started).Milliseconds()}
	if err != nil {
		row.Status = "fail"
		row.Detail = err.Error()
	}
	return row
}

func runLocalDownloadSelfTestV860(a *App) (string, error) {
	if a == nil {
		return "", errors.New("aplicație indisponibilă")
	}
	payload := bytes.Repeat([]byte("DDG-v8.6-diagnostic-"), 8192)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	defer ln.Close()
	serverURL := "http://" + ln.Addr().String()
	const requiredReferer = "https://ddg.local/diagnostic-page"

	mux := http.NewServeMux()
	mux.HandleFunc("/media.bin", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Referer") != requiredReferer {
			http.Error(w, "Referer required", http.StatusForbidden)
			return
		}
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Content-Type", "application/octet-stream")
		start := int64(0)
		if raw := r.Header.Get("Range"); strings.HasPrefix(raw, "bytes=") {
			part := strings.TrimSuffix(strings.TrimPrefix(raw, "bytes="), "-")
			if n, parseErr := strconv.ParseInt(part, 10, 64); parseErr == nil && n >= 0 && n < int64(len(payload)) {
				start = n
				w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, len(payload)-1, len(payload)))
				w.WriteHeader(http.StatusPartialContent)
			}
		}
		_, _ = w.Write(payload[start:])
	})
	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(ln) }()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		_ = srv.Shutdown(ctx)
		cancel()
	}()

	dest, err := os.MkdirTemp(a.appDir, "download-diagnostic-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dest)
	res := Result{ID: -860, Remote: RemoteItem{
		Name:        "diagnostic.bin",
		Path:        "diagnostic.bin",
		Size:        int64(len(payload)),
		Source:      "HTML",
		URL:         requiredReferer,
		DirectURL:   serverURL + "/media.bin",
		ContentType: "application/octet-stream",
	}}
	part := sourcePartPathV855(dest, res.Remote.Name, res.Remote.DirectURL)
	resumeAt := len(payload) / 3
	if err := os.WriteFile(part, payload[:resumeAt], 0644); err != nil {
		return "", err
	}
	path, err := internalDownloadV855(context.Background(), res, dest, func(int64, int64) {})
	if err != nil {
		return "", err
	}
	got, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if !bytes.Equal(got, payload) {
		return "", errors.New("fișierul final diferă după resume")
	}
	return fmt.Sprintf("HTTP intern OK • Referer OK • resume %d→%d bytes • integritate OK", resumeAt, len(payload)), nil
}

func probeDirectDownloadV860(ctx context.Context, res Result) (string, error) {
	u := strings.TrimSpace(resultDownloadURL(res))
	if u == "" {
		return "", errors.New("URL direct lipsă")
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) DuplicateDownloadGuard/8.6 Diagnostic")
	req.Header.Set("Range", "bytes=0-0")
	if referer := downloadRefererV855(res); referer != "" {
		req.Header.Set("Referer", referer)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	ct := strings.ToLower(strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0]))
	if strings.HasPrefix(ct, "text/html") {
		return "", errors.New("serverul returnează HTML în locul fișierului media")
	}
	return fmt.Sprintf("HTTP %d • %s • Range=%s • Referer=%t", resp.StatusCode, ct, resp.Header.Get("Accept-Ranges"), downloadRefererV855(res) != ""), nil
}

func (a *App) diagnosticYtDlpV860(ctx context.Context, res Result) (string, error) {
	exe := a.detectYtDlp()
	if exe == "" {
		return "", errors.New("yt-dlp lipsește")
	}
	u := ytDlpInputURLV855(res)
	if u == "" {
		return "", errors.New("URL yt-dlp lipsă")
	}
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	args := []string{"--simulate", "--no-warnings", "--no-playlist"}
	a.mu.RLock()
	cookies := strings.TrimSpace(a.cfg.YtCookiesBrowser)
	a.mu.RUnlock()
	if cookies != "" {
		args = append(args, "--cookies-from-browser", cookies)
	}
	args = append(args, u)
	cmd := exec.CommandContext(ctx, exe, args...)
	hideChildWindow(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if len(msg) > 500 {
			msg = msg[len(msg)-500:]
		}
		if msg != "" {
			return "", fmt.Errorf("yt-dlp simulate: %s", msg)
		}
		return "", err
	}
	return "yt-dlp poate rezolva sursa fără download", nil
}

func (a *App) runDownloadDiagnosticV860(ctx context.Context, id int, includeNetwork bool) DownloadDiagnosticReportV860 {
	report := DownloadDiagnosticReportV860{ResultID: id, At: time.Now().Unix()}
	report.Checks = append(report.Checks, diagnosticCheckV860("Downloader intern — Referer + resume + integritate", func() (string, error) {
		return runLocalDownloadSelfTestV860(a)
	}))

	toolChecks := []struct {
		name string
		path string
		req  bool
	}{
		{"MEGAcmd", a.detectMegaClient(), false},
		{"yt-dlp", a.detectYtDlp(), false},
		{"gallery-dl", a.detectGalleryDL(), false},
		{"aria2", a.detectAria2(), false},
		{"FFmpeg", a.detectFFmpeg(), false},
	}
	for _, tc := range toolChecks {
		row := DownloadDiagnosticCheckV860{Name: "Tool — " + tc.name, Status: "pass"}
		if tc.path == "" {
			row.Status = "warn"
			row.Detail = "nu este instalat/configurat; Auto îl folosește numai când sursa îl cere"
			row.Action = "Instalează din AI & Tool Manager dacă ai nevoie de acest tip de sursă."
		} else {
			row.Detail = filepath.Base(tc.path) + " • disponibil"
		}
		report.Checks = append(report.Checks, row)
	}

	if id > 0 {
		res, ok := a.resultByID(id)
		if !ok {
			report.Checks = append(report.Checks, DownloadDiagnosticCheckV860{Name: "Rezultat selectat", Status: "fail", Detail: "rezultatul nu mai există"})
		} else {
			report.Source = res.Remote.Source
			engine := chooseQueueEngineV855(res, "auto")
			report.Engine = engine
			row := diagnosticCheckV860("Motor Auto pentru rezultatul selectat", func() (string, error) {
				if err := a.validateDownloadEngineV855(res, engine); err != nil {
					return "", err
				}
				return fmt.Sprintf("%s → %s", res.Remote.Source, engine), nil
			})
			report.Checks = append(report.Checks, row)
			if includeNetwork && row.Status == "pass" {
				switch engine {
				case "internal", "aria2":
					report.Checks = append(report.Checks, diagnosticCheckV860("Probă remote 1 byte", func() (string, error) { return probeDirectDownloadV860(ctx, res) }))
				case "yt-dlp":
					report.Checks = append(report.Checks, diagnosticCheckV860("yt-dlp simulate", func() (string, error) { return a.diagnosticYtDlpV860(ctx, res) }))
				case "mega":
					report.Checks = append(report.Checks, DownloadDiagnosticCheckV860{Name: "MEGA acces", Status: "warn", Detail: "motorul și sursa sunt valide; testul nu descarcă automat un fișier MEGA complet", Action: "Folosește un job real mic pentru validarea cotei/transferului MEGA."})
				}
			}
		}
	}

	report.Overall = "pass"
	for _, c := range report.Checks {
		if c.Status == "fail" {
			report.Overall = "fail"
			break
		}
		if c.Status == "warn" && report.Overall == "pass" {
			report.Overall = "warn"
		}
	}
	return report
}

func (a *App) handleDownloadDiagnosticV860(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST necesar", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ID             int  `json:"id"`
		IncludeNetwork bool `json:"includeNetwork"`
	}
	if err := decodeJSON(r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	report := a.runDownloadDiagnosticV860(r.Context(), req.ID, req.IncludeNetwork)
	jsonOut(w, report)
}
