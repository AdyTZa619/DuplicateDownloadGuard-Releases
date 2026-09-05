package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const nativeUpdaterModeArg = "--ddg-native-updater"
const nativeUpdaterCleanupModeArg = "--ddg-native-updater-cleanup"

type nativeUpdateRequest struct {
	ParentPID       int    `json:"parentPid"`
	Current         string `json:"current"`
	Pending         string `json:"pending"`
	Backup          string `json:"backup"`
	Health          string `json:"health"`
	Log             string `json:"log"`
	ExpectedVersion string `json:"expectedVersion"`
	ExpectedSHA256  string `json:"expectedSha256"`
}

func maybeRunNativeUpdater(args []string) (bool, int) {
	if len(args) >= 2 && args[1] == nativeUpdaterCleanupModeArg {
		if len(args) != 6 {
			return true, 64
		}
		parentPID, err := strconv.Atoi(args[2])
		if err != nil || parentPID < 0 {
			return true, 64
		}
		return true, runNativeUpdaterCleanup(parentPID, args[3], args[4], args[5])
	}
	if len(args) < 2 || args[1] != nativeUpdaterModeArg {
		return false, 0
	}
	if len(args) != 3 {
		return true, 64
	}
	return true, runNativeUpdater(args[2])
}

func runNativeUpdater(reqPath string) int {
	b, err := os.ReadFile(reqPath)
	if err != nil {
		return 65
	}
	var req nativeUpdateRequest
	if err := json.Unmarshal(b, &req); err != nil {
		return 65
	}
	logUpdate := func(message string) {
		if req.Log == "" {
			return
		}
		_ = os.MkdirAll(filepath.Dir(req.Log), 0755)
		f, err := os.OpenFile(req.Log, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return
		}
		_, _ = fmt.Fprintf(f, "%s %s\n", time.Now().Format(time.RFC3339), message)
		_ = f.Close()
	}

	if err := validateNativeUpdateRequest(req); err != nil {
		logUpdate("Cerere updater refuzată: " + err.Error())
		return 65
	}
	versionLabel := strings.TrimSpace(req.ExpectedVersion)
	if versionLabel == "" {
		versionLabel = "update local"
	}
	logUpdate("Updater nativ pornit pentru " + versionLabel)

	// The UI responds first, then the parent exits. Do not touch the mapped EXE
	// until Windows confirms that the original DDG process has actually ended.
	if req.ParentPID > 0 {
		logUpdate(fmt.Sprintf("Aștept închiderea procesului DDG PID %d...", req.ParentPID))
		if !waitForProcessExit(req.ParentPID, 30*time.Second) {
			logUpdate("Procesul DDG nu s-a închis în 30s; update abandonat fără a modifica EXE-ul.")
			return 7
		}
		logUpdate("Procesul DDG s-a închis; continui înlocuirea.")
	} else {
		time.Sleep(1500 * time.Millisecond)
	}

	// The new backup is about to be created from the still-valid current EXE.
	// Remove backups/helpers from older updates first so disk usage cannot grow
	// with every version. Keep the currently running helper and current pending.
	updatesDir := filepath.Dir(req.Pending)
	self, _ := os.Executable()
	cleanupNativeUpdateArtifacts(updatesDir, "", self, false, false)

	if err := retryFor(60*time.Second, func() error {
		return copyFileDurable(req.Current, req.Backup)
	}); err != nil {
		logUpdate("Backup eșuat: " + err.Error())
		return 2
	}
	if err := retryFor(60*time.Second, func() error {
		if err := copyFileDurable(req.Pending, req.Current); err != nil {
			return err
		}
		got, err := sha256Path(req.Current)
		if err != nil {
			return err
		}
		if !strings.EqualFold(got, req.ExpectedSHA256) {
			return errors.New("SHA-256 diferit după copiere")
		}
		return nil
	}); err != nil {
		logUpdate("Înlocuire eșuată: " + err.Error())
		_ = restoreAndStart(req, logUpdate)
		return 3
	}

	_ = os.Remove(req.Health)
	p, err := startUpdatedExecutable(req.Current)
	if err != nil {
		logUpdate("Pornire versiune nouă eșuată: " + err.Error())
		_ = restoreAndStart(req, logUpdate)
		return 4
	}
	if waitForExpectedHealth(req.Health, req.ExpectedVersion, 35*time.Second) {
		logUpdate("Update confirmat sănătos.")
		_ = p.Release()
		// A tiny second instance of the freshly installed EXE waits for this
		// helper to exit, then removes helper/pending/temp files. This avoids
		// relying on Windows to delete an executable while it is still mapped.
		if err := scheduleNativeUpdaterCleanup(req.Current, os.Getpid(), updatesDir, req.Backup, self); err != nil {
			logUpdate("Curățarea automată a updaterului nu a putut fi programată: " + err.Error())
			_ = os.Remove(req.Pending)
			_ = os.Remove(reqPath)
		}
		return 0
	}

	logUpdate("Health-check eșuat; rollback automat.")
	_ = p.Kill()
	_, _ = p.Wait()
	if err := restoreAndStart(req, logUpdate); err != nil {
		logUpdate("ROLLBACK EȘUAT: " + err.Error())
		return 6
	}
	return 5
}

func validateNativeUpdateRequest(req nativeUpdateRequest) error {
	for label, path := range map[string]string{
		"current": req.Current, "pending": req.Pending, "backup": req.Backup,
		"health": req.Health, "log": req.Log,
	} {
		if !filepath.IsAbs(path) {
			return fmt.Errorf("%s nu este cale absolută", label)
		}
	}
	if req.ParentPID < 0 {
		return errors.New("parentPid invalid")
	}
	if !strings.HasSuffix(strings.ToLower(req.Current), ".exe") || !strings.HasSuffix(strings.ToLower(req.Pending), ".exe") {
		return errors.New("fișierele de update trebuie să fie EXE")
	}
	if len(strings.TrimSpace(req.ExpectedSHA256)) != 64 {
		return errors.New("SHA-256 așteptat invalid")
	}
	return nil
}

func retryFor(timeout time.Duration, action func() error) error {
	deadline := time.Now().Add(timeout)
	var last error
	for {
		if err := action(); err == nil {
			return nil
		} else {
			last = err
		}
		if time.Now().After(deadline) {
			return last
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func copyFileDurable(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	tmp := dst + ".copying"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0755)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	syncErr := out.Sync()
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return copyErr
	}
	if syncErr != nil {
		_ = os.Remove(tmp)
		return syncErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	if err := os.Chmod(tmp, 0755); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := replaceFile(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func replaceFile(tmp, dst string) error {
	old := dst + ".replacing"
	_ = os.Remove(old)
	if _, err := os.Stat(dst); err == nil {
		if err := os.Rename(dst, old); err != nil {
			return err
		}
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Rename(old, dst)
		return err
	}
	_ = os.Remove(old)
	return nil
}

func sha256Path(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func startUpdatedExecutable(path string) (*os.Process, error) {
	cmd := exec.Command(path)
	hideChildWindow(cmd)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd.Process, nil
}

func waitForExpectedHealth(path, version string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		b, err := os.ReadFile(path)
		if err == nil {
			health := strings.TrimSpace(string(b))
			expected := strings.TrimSpace(version)
			if health != "" && (expected == "" || strings.HasPrefix(health, expected)) {
				return true
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return false
}

func restoreAndStart(req nativeUpdateRequest, logUpdate func(string)) error {
	if err := retryFor(30*time.Second, func() error { return copyFileDurable(req.Backup, req.Current) }); err != nil {
		return err
	}
	p, err := startUpdatedExecutable(req.Current)
	if err != nil {
		return err
	}
	_ = p.Release()
	logUpdate("Rollback terminat; versiunea anterioară a fost repornită.")
	return nil
}

func sameCleanPath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

func pathInside(base, path string) bool {
	if base == "" || path == "" || !filepath.IsAbs(base) || !filepath.IsAbs(path) {
		return false
	}
	rel, err := filepath.Rel(filepath.Clean(base), filepath.Clean(path))
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)))
}

func cleanupNativeUpdateArtifacts(updatesDir, keepBackup, keepHelper string, removePending, removeRequest bool) {
	updatesDir = filepath.Clean(updatesDir)
	backupDir := filepath.Join(updatesDir, "backup")

	if entries, err := os.ReadDir(backupDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			path := filepath.Join(backupDir, entry.Name())
			if sameCleanPath(path, keepBackup) {
				continue
			}
			lower := strings.ToLower(entry.Name())
			if (strings.HasPrefix(lower, "duplicatedownloadguard_") && strings.HasSuffix(lower, ".exe")) ||
				strings.HasSuffix(lower, ".copying") || strings.HasSuffix(lower, ".replacing") || strings.HasSuffix(lower, ".download") {
				_ = os.Remove(path)
			}
		}
	}

	entries, err := os.ReadDir(updatesDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(updatesDir, entry.Name())
		if sameCleanPath(path, keepHelper) {
			continue
		}
		lower := strings.ToLower(entry.Name())
		switch {
		case strings.HasPrefix(lower, "duplicatedownloadguard.updater_") && strings.HasSuffix(lower, ".exe"):
			_ = os.Remove(path)
		case removePending && lower == "duplicatedownloadguard.pending.exe":
			_ = os.Remove(path)
		case removeRequest && lower == "apply_update.json":
			_ = os.Remove(path)
		case strings.HasSuffix(lower, ".copying") || strings.HasSuffix(lower, ".replacing") || strings.HasSuffix(lower, ".download"):
			_ = os.Remove(path)
		}
	}
}

func scheduleNativeUpdaterCleanup(current string, parentPID int, updatesDir, keepBackup, helper string) error {
	cmd := exec.Command(current, nativeUpdaterCleanupModeArg, strconv.Itoa(parentPID), updatesDir, keepBackup, helper)
	hideChildWindow(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

func runNativeUpdaterCleanup(parentPID int, updatesDir, keepBackup, helper string) int {
	if parentPID < 0 || !filepath.IsAbs(updatesDir) || !filepath.IsAbs(keepBackup) || !filepath.IsAbs(helper) {
		return 65
	}
	if !pathInside(updatesDir, keepBackup) || !pathInside(updatesDir, helper) {
		return 65
	}
	if parentPID > 0 && !waitForProcessExit(parentPID, 30*time.Second) {
		return 7
	}
	cleanupNativeUpdateArtifacts(updatesDir, keepBackup, "", true, true)
	return 0
}
