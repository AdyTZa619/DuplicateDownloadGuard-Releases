package main

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const recoveryHelperModeArgV85119 = "--ddg-recovery-helper"

var recoverySHA256V85119 = regexp.MustCompile(`(?i)^[0-9a-f]{64}$`)

type recoveryManifestV85119 struct {
	Version     string `json:"version"`
	URL         string `json:"url"`
	SHA256      string `json:"sha256"`
	Notes       string `json:"notes,omitempty"`
	PublishedAt string `json:"publishedAt,omitempty"`
	Channel     string `json:"channel,omitempty"`
}

func runningRecoveryHelperModeV85119(args []string) bool {
	return len(args) >= 2 && args[1] == recoveryHelperModeArgV85119
}

func validateRecoveryManifestV85119(m recoveryManifestV85119) error {
	if strings.TrimSpace(m.Version) == "" {
		return errors.New("manifest fără versiune")
	}
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(m.URL)), "https://raw.githubusercontent.com/adytza619/duplicatedownloadguard-releases/") {
		return errors.New("URL EXE din manifest nu aparține canalului DDG oficial")
	}
	if !recoverySHA256V85119.MatchString(strings.TrimSpace(m.SHA256)) {
		return errors.New("SHA-256 invalid în manifest")
	}
	return nil
}

// repairInterruptedExecutableV85119 repairs only the two temp names created by
// DDG's own atomic replacement routine. It never guesses another executable.
// If the live EXE still exists, stale temp files are simply removed. If the
// live EXE is missing but .replacing exists, the last known-good file is put
// back before any new update attempt starts.
func repairInterruptedExecutableV85119(current string) error {
	if strings.TrimSpace(current) == "" || !filepath.IsAbs(current) {
		return errors.New("cale executabil invalidă")
	}
	copying := current + ".copying"
	replacing := current + ".replacing"
	if st, err := os.Stat(current); err == nil && !st.IsDir() {
		_ = os.Remove(copying)
		_ = os.Remove(replacing)
		return nil
	}
	if st, err := os.Stat(replacing); err == nil && !st.IsDir() {
		_ = os.Remove(copying)
		if err := os.Rename(replacing, current); err != nil {
			return err
		}
		return nil
	}
	return errors.New("executabilul DDG și copia .replacing lipsesc")
}
