package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestDetectorSelfTestClassificationV901(t *testing.T) {
	if got := detectorClassificationV901(DownloadGuardDecision{}); got != "FĂRĂ DOVADĂ" {
		t.Fatalf("got %q", got)
	}
	if got := detectorClassificationV901(DownloadGuardDecision{Detector: &DuplicateEvidenceV90{Classification: "IDENTIC"}}); got != "IDENTIC" {
		t.Fatalf("got %q", got)
	}
}

func TestDetectorSelfTestJDGuardV901(t *testing.T) {
	missing := decorateGuardDecision(DownloadGuardDecision{Verdict: guardDownload})
	duplicate := decorateGuardDecision(DownloadGuardDecision{Verdict: guardDuplicate})
	review := decorateGuardDecision(DownloadGuardDecision{Verdict: guardReview})
	if !jdownloaderDecisionAllowedV901(missing) {
		t.Fatal("final LIPSĂ must pass JD guard")
	}
	if jdownloaderDecisionAllowedV901(duplicate) || jdownloaderDecisionAllowedV901(review) {
		t.Fatal("duplicate/review crossed JD guard")
	}
}

func TestDetectorSelfTestEndToEndV901(t *testing.T) {
	if os.Getenv("DDG_SELF_TEST") != "1" {
		t.Skip("set DDG_SELF_TEST=1 to run the encoded-media self-test")
	}
	ff, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Fatal(err)
	}
	fp, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	a := &App{
		appDir:     dir,
		index:      map[string]FileEntry{},
		bySize:     map[int64][]string{},
		byName:     map[string][]string{},
		byIdentity: map[string][]string{},
		decisions:  map[string]Decision{},
		cfg:        Config{FFmpegPath: ff, FFprobePath: fp, DownloadDir: filepath.Join(dir, "downloads")},
	}
	if err := os.MkdirAll(a.cfg.DownloadDir, 0755); err != nil {
		t.Fatal(err)
	}
	detectorSelfTestV901.Lock()
	detectorSelfTestV901.state = detectorSelfTestStateV901{Running: true}
	detectorSelfTestV901.Unlock()
	a.runDetectorSelfTestV901()
	state := detectorSelfTestSnapshotV901()
	if state.Error != "" || state.Report == nil {
		t.Fatalf("self-test failed: %#v", state)
	}
	if !state.Report.Passed {
		t.Fatalf("self-test report did not pass: %#v", state.Report)
	}
	if _, err := os.Stat(state.ReportPath); err != nil {
		t.Fatalf("report missing: %v", err)
	}
}
