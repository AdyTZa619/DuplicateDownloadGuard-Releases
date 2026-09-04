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
	// resume implementation discovers/reuses one root before compatibility work.
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

func TestPreviewUIReleasesBrowserMediaBeforeEveryRowSwitchV8512(t *testing.T) {
	b, err := webFS.ReadFile("web/preview_quick_v86.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(b)
	for _, required := range []string{
		"function releaseRemoteMediaV8512()",
		"media.pause()",
		"media.removeAttribute('src')",
		"media.load()",
	} {
		if !strings.Contains(js, required) {
			t.Fatalf("browser stream release regression missing %q", required)
		}
	}
	showDetailStart := strings.Index(js, "const wrapped = async function(r) {")
	showDetailEnd := strings.Index(js[showDetailStart:], "const out = await original.apply(this, arguments);")
	if showDetailStart < 0 || showDetailEnd < 0 {
		t.Fatal("wrapped showDetail block missing")
	}
	showDetailBlock := js[showDetailStart : showDetailStart+showDetailEnd]
	if !strings.Contains(showDetailBlock, "releaseRemoteMediaV8512()") || !strings.Contains(showDetailBlock, "reset(r)") {
		t.Fatal("showDetail must release the previous browser media before loading the next row")
	}
}

func TestHotSessionClickDoesNotRebuildRootWithLongCommandV8512(t *testing.T) {
	resumeSrc, err := osReadTextForTestV858("mega_preview_resume_direct_v858.go")
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(resumeSrc, "The scan (or a previous preview) proves this exact public-folder session")
	end := strings.Index(resumeSrc, "if a.preview.Active {\n\t\t_ = a.stopMegaPreviewLocked")
	if start < 0 || end <= start {
		t.Fatal("hot-session recovery block not found")
	}
	hot := resumeSrc[start:end]
	if strings.Contains(hot, "startMegaWarmRootV86") {
		t.Fatal("hot-session click must not repeat the long root creation command")
	}
	if !strings.Contains(hot, "listMegaWarmRootV8511") || !strings.Contains(hot, "tryMegaCurrentSessionWebDAVV859") {
		t.Fatal("hot-session click must rediscover root then use bounded per-file compatibility")
	}
}

func TestPostScanRootWarmupOwnsTheLongSetupV8512(t *testing.T) {
	src, err := osReadTextForTestV858("mega_fast_preview.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(src, "ensureMegaWarmRootAfterScanV8512") {
		t.Fatal("post-scan root warmup helper missing")
	}
	if !strings.Contains(src, `http.NewRequestWithContext(ctx, "PROPFIND", rootURL, nil)`) {
		t.Fatal("post-scan WebDAV transport warmup missing")
	}
	if !strings.Contains(src, "WebDAV root pregătit și încălzit") {
		t.Fatal("post-scan warmup timing diagnostic missing")
	}
}

func osReadTextForTestV858(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
