//go:build windows

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const recoveryManifestURLV85119 = "https://raw.githubusercontent.com/AdyTZa619/DuplicateDownloadGuard-Releases/testing/update-test.json"

var recoveryPortsV85119 = []int{51289, 51290, 51291, 51292}

type recoveryAdoptV85119 struct {
	ParentPID int    `json:"parentPid"`
	Current   string `json:"current"`
	Version   string `json:"version"`
	AppDir    string `json:"appDir"`
}

type recoveryApplyV85119 struct {
	ExpectedVersion string `json:"expectedVersion"`
	AppDir          string `json:"appDir"`
}

type recoveryStateV85119 struct {
	mu        sync.Mutex
	parentPID int
	current   string
	version   string
	appDir    string
	lastAdopt time.Time
	applying  bool
}

func recoveryBaseV85119(port int) string { return fmt.Sprintf("http://127.0.0.1:%d", port) }

func recoveryOriginAllowedV85119(origin string) bool {
	origin = strings.TrimSpace(origin)
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	h := strings.ToLower(strings.TrimSpace(u.Hostname()))
	return (u.Scheme == "http" || u.Scheme == "https") && (h == "127.0.0.1" || h == "localhost")
}

func recoveryJSONV85119(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func recoveryProcessAliveV85119(pid int) bool {
	if pid <= 0 {
		return false
	}
	return !waitForProcessExit(pid, 0)
}

func recoveryFetchManifestV85119() (recoveryManifestV85119, error) {
	var m recoveryManifestV85119
	client := &http.Client{Timeout: 12 * time.Second}
	req, _ := http.NewRequest(http.MethodGet, recoveryManifestURLV85119+"?recovery="+strconv.FormatInt(time.Now().UnixNano(), 10), nil)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("User-Agent", "DuplicateDownloadGuard/recovery-helper")
	resp, err := client.Do(req)
	if err != nil {
		return m, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return m, fmt.Errorf("manifest HTTP %d", resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64*1024)).Decode(&m); err != nil {
		return m, err
	}
	if err := validateRecoveryManifestV85119(m); err != nil {
		return m, err
	}
	return m, nil
}

func recoveryDownloadEXEV85119(m recoveryManifestV85119, dst string) error {
	client := &http.Client{Timeout: 90 * time.Second}
	req, _ := http.NewRequest(http.MethodGet, m.URL, nil)
	req.Header.Set("Accept", "application/octet-stream")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("User-Agent", "DuplicateDownloadGuard/recovery-helper")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("EXE HTTP %d", resp.StatusCode)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	tmp := dst + ".download"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0755)
	if err != nil {
		return err
	}
	n, cpErr := io.Copy(f, io.LimitReader(resp.Body, 120*1024*1024+1))
	syncErr := f.Sync()
	closeErr := f.Close()
	if cpErr != nil {
		_ = os.Remove(tmp)
		return cpErr
	}
	if n > 120*1024*1024 {
		_ = os.Remove(tmp)
		return fmt.Errorf("build TEST prea mare")
	}
	if syncErr != nil {
		_ = os.Remove(tmp)
		return syncErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	got, err := sha256Path(tmp)
	if err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if !strings.EqualFold(got, strings.TrimSpace(m.SHA256)) {
		_ = os.Remove(tmp)
		return fmt.Errorf("SHA-256 TEST diferit")
	}
	_ = os.Remove(dst)
	return os.Rename(tmp, dst)
}

func recoveryApplyUpdateV85119(st *recoveryStateV85119, m recoveryManifestV85119, pending string) {
	time.Sleep(350 * time.Millisecond)
	st.mu.Lock()
	pid := st.parentPID
	current := st.current
	appDir := st.appDir
	st.mu.Unlock()

	// The normal updater may be unreachable because the backend is hung. The
	// user explicitly requested this recovery path, so give the parent a short
	// grace interval and then terminate the exact DDG PID that adopted us.
	if pid > 0 && !waitForProcessExit(pid, 2500*time.Millisecond) {
		_ = terminateProcessTree(pid)
		_ = waitForProcessExit(pid, 5*time.Second)
	}

	if err := repairInterruptedExecutableV85119(current); err != nil {
		st.mu.Lock()
		st.applying = false
		st.mu.Unlock()
		return
	}
	updatesDir := filepath.Join(appDir, "updates")
	backupDir := filepath.Join(updatesDir, "backup")
	_ = os.MkdirAll(backupDir, 0755)
	backup := filepath.Join(backupDir, "DuplicateDownloadGuard_recovery_"+time.Now().Format("20060102-150405")+".exe")
	if err := copyFileDurable(current, backup); err != nil {
		st.mu.Lock()
		st.applying = false
		st.mu.Unlock()
		return
	}
	if err := copyFileDurable(pending, current); err != nil {
		_ = copyFileDurable(backup, current)
		st.mu.Lock()
		st.applying = false
		st.mu.Unlock()
		return
	}
	if got, err := sha256Path(current); err != nil || !strings.EqualFold(got, strings.TrimSpace(m.SHA256)) {
		_ = copyFileDurable(backup, current)
		st.mu.Lock()
		st.applying = false
		st.mu.Unlock()
		return
	}

	_ = os.Remove(updateHealthPath(appDir))
	p, err := startUpdatedExecutable(current)
	if err != nil {
		_ = copyFileDurable(backup, current)
		_, _ = startUpdatedExecutable(current)
		st.mu.Lock()
		st.applying = false
		st.mu.Unlock()
		return
	}
	if waitForExpectedHealth(updateHealthPath(appDir), m.Version, 35*time.Second) {
		_ = p.Release()
		_ = os.Remove(pending)
		st.mu.Lock()
		st.version = m.Version
		st.parentPID = p.Pid
		st.lastAdopt = time.Now()
		st.applying = false
		st.mu.Unlock()
		return
	}

	_ = p.Kill()
	_, _ = p.Wait()
	_ = copyFileDurable(backup, current)
	if old, err := startUpdatedExecutable(current); err == nil {
		_ = old.Release()
	}
	st.mu.Lock()
	st.applying = false
	st.mu.Unlock()
}

func runRecoveryHelperV85119(args []string) int {
	if len(args) != 6 {
		return 64
	}
	pid, err := strconv.Atoi(args[2])
	if err != nil || pid <= 0 {
		return 64
	}
	current := filepath.Clean(args[3])
	version := strings.TrimSpace(args[4])
	appDir := filepath.Clean(args[5])
	if !filepath.IsAbs(current) || !filepath.IsAbs(appDir) || filepath.Clean(filepath.Dir(appDir)) != filepath.Clean(filepath.Dir(current)) {
		return 64
	}
	st := &recoveryStateV85119{parentPID: pid, current: current, version: version, appDir: appDir, lastAdopt: time.Now()}

	mux := http.NewServeMux()
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if !recoveryOriginAllowedV85119(origin) {
			http.Error(w, "origin refuzat", http.StatusForbidden)
			return
		}
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}
		st.mu.Lock()
		resp := map[string]any{"ok": true, "version": st.version, "appDir": st.appDir, "parentPid": st.parentPID, "parentAlive": recoveryProcessAliveV85119(st.parentPID), "applying": st.applying}
		st.mu.Unlock()
		recoveryJSONV85119(w, http.StatusOK, resp)
	})
	mux.HandleFunc("/adopt", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			if recoveryOriginAllowedV85119(r.Header.Get("Origin")) {
				w.Header().Set("Access-Control-Allow-Origin", r.Header.Get("Origin"))
				w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
				w.WriteHeader(http.StatusNoContent)
				return
			}
			http.Error(w, "origin refuzat", http.StatusForbidden)
			return
		}
		if r.Method != http.MethodPost || !recoveryOriginAllowedV85119(r.Header.Get("Origin")) {
			http.Error(w, "refuzat", http.StatusForbidden)
			return
		}
		var req recoveryAdoptV85119
		if err := json.NewDecoder(io.LimitReader(r.Body, 16*1024)).Decode(&req); err != nil {
			http.Error(w, "cerere invalidă", 400)
			return
		}
		if req.ParentPID <= 0 || !strings.EqualFold(filepath.Clean(req.Current), current) || !strings.EqualFold(filepath.Clean(req.AppDir), appDir) {
			http.Error(w, "altă instanță DDG", http.StatusConflict)
			return
		}
		st.mu.Lock()
		st.parentPID = req.ParentPID
		st.version = strings.TrimSpace(req.Version)
		st.lastAdopt = time.Now()
		st.mu.Unlock()
		recoveryJSONV85119(w, http.StatusOK, map[string]any{"ok": true})
	})
	mux.HandleFunc("/apply-test", func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && recoveryOriginAllowedV85119(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}
		if r.Method == http.MethodOptions {
			if !recoveryOriginAllowedV85119(origin) {
				http.Error(w, "origin refuzat", 403)
				return
			}
			w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method != http.MethodPost || !recoveryOriginAllowedV85119(origin) {
			http.Error(w, "refuzat", 403)
			return
		}
		var req recoveryApplyV85119
		_ = json.NewDecoder(io.LimitReader(r.Body, 16*1024)).Decode(&req)
		st.mu.Lock()
		if st.applying {
			st.mu.Unlock()
			http.Error(w, "update deja în curs", http.StatusConflict)
			return
		}
		if req.AppDir != "" && !strings.EqualFold(filepath.Clean(req.AppDir), st.appDir) {
			st.mu.Unlock()
			http.Error(w, "helperul aparține altei instanțe DDG", http.StatusConflict)
			return
		}
		st.applying = true
		st.mu.Unlock()

		m, err := recoveryFetchManifestV85119()
		if err != nil {
			st.mu.Lock()
			st.applying = false
			st.mu.Unlock()
			http.Error(w, err.Error(), 502)
			return
		}
		if req.ExpectedVersion != "" && !strings.EqualFold(strings.TrimSpace(req.ExpectedVersion), strings.TrimSpace(m.Version)) {
			st.mu.Lock()
			st.applying = false
			st.mu.Unlock()
			http.Error(w, "manifestul s-a schimbat; verifică din nou update-ul", http.StatusConflict)
			return
		}
		pending := filepath.Join(appDir, "updates", "recovery.pending.exe")
		if err := recoveryDownloadEXEV85119(m, pending); err != nil {
			st.mu.Lock()
			st.applying = false
			st.mu.Unlock()
			http.Error(w, err.Error(), 502)
			return
		}
		recoveryJSONV85119(w, http.StatusAccepted, map[string]any{"ok": true, "version": m.Version, "message": "Recovery helper aplică update-ul și repornește DDG."})
		go recoveryApplyUpdateV85119(st, m, pending)
	})

	var ln net.Listener
	for _, port := range recoveryPortsV85119 {
		candidate, e := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if e == nil {
			ln = candidate
			break
		}
	}
	if ln == nil {
		return 7
	}
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			st.mu.Lock()
			pid, last, applying := st.parentPID, st.lastAdopt, st.applying
			st.mu.Unlock()
			if !applying && !recoveryProcessAliveV85119(pid) && time.Since(last) > 30*time.Minute {
				_ = srv.Close()
				return
			}
		}
	}()
	if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
		return 8
	}
	return 0
}

func tryAdoptRecoveryHelperV85119(current, appDir string, pid int) bool {
	payload, _ := json.Marshal(recoveryAdoptV85119{ParentPID: pid, Current: current, Version: appVersion, AppDir: appDir})
	client := &http.Client{Timeout: 650 * time.Millisecond}
	for _, port := range recoveryPortsV85119 {
		req, _ := http.NewRequest(http.MethodPost, recoveryBaseV85119(port)+"/adopt", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err == nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
			_ = resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return true
			}
		}
	}
	return false
}

func cleanupStaleRecoveryHelpersV85119(updatesDir string) {
	entries, err := os.ReadDir(updatesDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		lower := strings.ToLower(entry.Name())
		if strings.HasPrefix(lower, "duplicatedownloadguard.recovery_") && strings.HasSuffix(lower, ".exe") {
			_ = os.Remove(filepath.Join(updatesDir, entry.Name()))
		}
	}
}

func ensureRecoveryHelperV85119() {
	current, err := os.Executable()
	if err != nil {
		return
	}
	current, _ = filepath.Abs(current)
	appDir, err := portableDataDir()
	if err != nil {
		return
	}
	_ = repairInterruptedExecutableV85119(current)
	if tryAdoptRecoveryHelperV85119(current, appDir, os.Getpid()) {
		return
	}
	updatesDir := filepath.Join(appDir, "updates")
	cleanupStaleRecoveryHelpersV85119(updatesDir)
	helper := filepath.Join(updatesDir, fmt.Sprintf("DuplicateDownloadGuard.recovery_%d.exe", os.Getpid()))
	if err := copyFileDurable(current, helper); err != nil {
		return
	}
	cmd := exec.Command(helper, recoveryHelperModeArgV85119, strconv.Itoa(os.Getpid()), current, appVersion, appDir)
	detachUpdaterProcess(cmd)
	if err := cmd.Start(); err != nil {
		_ = os.Remove(helper)
		return
	}
	_ = cmd.Process.Release()
}

func init() {
	if runningRecoveryHelperModeV85119(os.Args) {
		os.Exit(runRecoveryHelperV85119(os.Args))
	}
	if runningNativeUpdaterMode(os.Args) {
		return
	}
	go func() {
		time.Sleep(700 * time.Millisecond)
		ensureRecoveryHelperV85119()
	}()
}
