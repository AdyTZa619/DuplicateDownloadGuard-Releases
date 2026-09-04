package main

import (
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestTrueFallbackPreservesWarmRootAndStartsRequestedFileV8510(t *testing.T) {
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
	want := []string{"webdav H:NEW"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("fallback must not destroy warm root; calls=%v want=%v", calls, want)
	}
	for _, call := range calls {
		if call == "webdav -d /" {
			t.Fatal("one failed child must never stop the folder root")
		}
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

func TestRestartPreviewUsesDirectResumePathV858(t *testing.T) {
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
		t.Fatal("browser root failure must request one backend fallback")
	}
	if !strings.Contains(js, "Root-ul rămâne activ pentru următoarele fișiere") {
		t.Fatal("UI must state and preserve the root after a per-file fallback")
	}
	if !strings.Contains(js, "MEGA TRUE FALLBACK • EROARE") {
		t.Fatal("new fallback must suppress the older nested fallback on failure")
	}

	// Coalesced user initialization still enters the dedicated restart helper;
	// v8.5.10 changes that helper internally to prefer persistent/root WebDAV.
	src, err := osReadTextForTestV858("mega_preview_ui_fast_v854.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(src, "streamURL, err := a.startMegaPreviewResumeDirectV858(item)") {
		t.Fatal("restart click is not routed through the dedicated resume helper")
	}
	if !strings.Contains(src, `return streamURL, "MEGA DIRECT RESUME"`) {
		t.Fatal("resume mode is not exposed to UI diagnostics")
	}
}

func osReadTextForTestV858(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
