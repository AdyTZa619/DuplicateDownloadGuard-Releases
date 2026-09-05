package main

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// The updater helper used during N -> N+1 is copied from version N. That means
// cleanup logic introduced in N+1 cannot rely only on the helper that performed
// the replacement. A normal N+1 startup therefore performs a second, safe
// cleanup only after the current binary has written its own health marker.
func init() {
	if runningNativeUpdaterMode(os.Args) {
		return
	}
	go func() {
		// markUpdateHealthyLater writes health.ok after 2.2s. Give normal startup
		// additional time so cleanup never runs before the current build is healthy.
		time.Sleep(4 * time.Second)
		appDir, err := portableDataDir()
		if err != nil {
			return
		}
		if !postHealthUpdaterCleanup(appDir) {
			return
		}
		// On Windows the helper that launched this build may still have its EXE
		// mapped during the first pass. Retry once after it has had time to exit.
		time.Sleep(5 * time.Second)
		postHealthUpdaterCleanup(appDir)
	}()
}

func runningNativeUpdaterMode(args []string) bool {
	if len(args) < 2 {
		return false
	}
	return args[1] == nativeUpdaterModeArg || args[1] == nativeUpdaterCleanupModeArg
}

func currentHealthConfirmed(appDir string) bool {
	b, err := os.ReadFile(updateHealthPath(appDir))
	if err != nil {
		return false
	}
	health := strings.TrimSpace(string(b))
	return health != "" && strings.HasPrefix(health, appVersion)
}

func newestUpdaterBackup(updatesDir string) string {
	backupDir := filepath.Join(updatesDir, "backup")
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return ""
	}

	var newest string
	var newestTime time.Time
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		lower := strings.ToLower(name)
		if !strings.HasPrefix(lower, "duplicatedownloadguard_") || !strings.HasSuffix(lower, ".exe") {
			continue
		}
		path := filepath.Join(backupDir, name)
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if newest == "" || info.ModTime().After(newestTime) || (info.ModTime().Equal(newestTime) && strings.Compare(strings.ToLower(path), strings.ToLower(newest)) > 0) {
			newest = path
			newestTime = info.ModTime()
		}
	}
	return newest
}

// postHealthUpdaterCleanup returns true only when cleanup was allowed by the
// current-version health marker. It deliberately ignores individual remove
// failures: a still-running Windows helper is retried by the caller later.
func postHealthUpdaterCleanup(appDir string) bool {
	if !currentHealthConfirmed(appDir) {
		return false
	}
	updatesDir := filepath.Join(appDir, "updates")
	keepBackup := newestUpdaterBackup(updatesDir)
	cleanupNativeUpdateArtifacts(updatesDir, keepBackup, "", true, true)
	return true
}
