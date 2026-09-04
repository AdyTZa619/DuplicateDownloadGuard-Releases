package main

import (
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestTrueFallbackStopsWarmRootThenStartsRequestedFileV858(t *testing.T) {
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
	want := []string{"webdav -d /", "webdav H:NEW"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls=%v want=%v", calls, want)
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
		t.Fatal("browser root failure must request true backend fallback")
	}

	// Static regression guard for the backend strategy. The production handler
	// must use the managed root controller, not the legacy per-file UI path.
	src, err := osReadTextForTestV858("main.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(src, "a.startMegaPreviewControlledV8511(res.Remote") {
		t.Fatal("preview handler is not routed through the managed root controller")
	}
	if strings.Contains(src, "a.startMegaPreviewForUIV854(res.Remote") {
		t.Fatal("preview handler still uses the legacy per-file UI route")
	}
}

func osReadTextForTestV858(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
