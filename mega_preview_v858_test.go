package main

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestTrueFallbackPreservesWarmRootAndStartsRequestedFileV8511(t *testing.T) {
	old := MegaPreviewState{Exe: "MegaClient.exe", RemotePath: megaWarmRootRefV86, StreamURL: "http://127.0.0.1:4443/"}
	calls := []string{}
	run := func(_ time.Duration, args ...string) (string, error) {
		call := strings.Join(args, " ")
		calls = append(calls, call)
		if call == "webdav H:NEW" {
			return "H:NEW http://127.0.0.1:4443/new.mp4", nil
		}
		return "", nil
	}
	got, err := switchWarmRootToPerFileV858(old, "H:NEW", run)
	if err != nil {
		t.Fatal(err)
	}
	if got.StreamURL != "http://127.0.0.1:4443/new.mp4" {
		t.Fatalf("stream URL=%q", got.StreamURL)
	}
	if len(calls) != 1 || calls[0] != "webdav H:NEW" {
		t.Fatalf("fallback must keep root and start only requested file; calls=%v", calls)
	}
}

func TestTrueFallbackDoesNotRepeatWarmRootURLV858(t *testing.T) {
	oldURL := "http://127.0.0.1:4443/folder/video.mp4"
	old := MegaPreviewState{Exe: "MegaClient.exe", RemotePath: megaWarmRootRefV86, StreamURL: "http://127.0.0.1:4443/"}
	run := func(_ time.Duration, args ...string) (string, error) {
		if strings.Join(args, " ") == "webdav H:NEW" {
			return "H:NEW http://127.0.0.1:4443/direct/video.mp4", nil
		}
		return "", nil
	}
	got, err := switchWarmRootToPerFileV858(old, "H:NEW", run)
	if err != nil {
		t.Fatal(err)
	}
	if got.StreamURL == oldURL || got.StreamURL == old.StreamURL {
		t.Fatalf("fallback reused failed warm-root URL: %q", got.StreamURL)
	}
}

func TestRestartPreviewUsesRootFirstResumePathV8511(t *testing.T) {
	b, err := webFS.ReadFile("web/preview_quick_v86.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(b)
	if strings.Contains(js, "prewarmMegaAfterRestart") {
		t.Fatal("startup MEGA prewarm must stay removed")
	}
	if strings.Contains(js, "setTimeout(prewarmMegaAfterRestart") {
		t.Fatal("startup prewarm timer must stay removed")
	}
	if !strings.Contains(js, "forceFallback:true") {
		t.Fatal("browser root failure must request true backend fallback")
	}
	if !strings.Contains(js, "MEGA TRUE FALLBACK • EROARE") {
		t.Fatal("new browser fallback must suppress the older duplicate retry")
	}

	// Coalesced user initialization remains the single entry point; the direct
	// resume implementation now discovers/promotes one root before per-file or
	// login --resume compatibility work.
	src, err := osReadTextForTestV858("mega_preview_ui_fast_v854.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(src, "streamURL, err := a.startMegaPreviewResumeDirectV858(item)") {
		t.Fatal("restart click is not routed through the root-first resume controller")
	}
	if !strings.Contains(src, `return streamURL, "MEGA DIRECT RESUME"`) {
		t.Fatal("resume mode is not exposed to UI diagnostics")
	}

	resumeSrc, err := osReadTextForTestV858("mega_preview_resume_direct_v858.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resumeSrc, "listMegaWarmRootV8511") || !strings.Contains(resumeSrc, "MEGA ROOT REUSE") {
		t.Fatal("restart path must discover and reuse the existing MEGAcmd root first")
	}
}

func osReadTextForTestV858(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
