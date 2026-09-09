package main

import (
	"strings"
	"testing"
)

func TestCheckActivityAndJournalModuleV8563(t *testing.T) {
	b, err := webFS.ReadFile("web/check_activity_v8563.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, marker := range []string{
		"Jurnal verificări",
		"ddgSourceCheckProgress",
		"ddgPreflightCheckProgress",
		"/api/source/scan",
		"/api/download/preflight",
		"/api/queue/add",
		"/api/status",
		"ddgCheckJournalV1",
	} {
		if !strings.Contains(s, marker) {
			t.Fatalf("check activity module missing marker %q", marker)
		}
	}
}

func TestBunkrJDownloaderCompatV8563(t *testing.T) {
	b, err := webFS.ReadFile("web/jdownloader_bunkr_compat_v8563.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, marker := range []string{
		"/flashgot",
		"/^\\/f\\//i",
		"'/d/'",
		"HTMLFormElement.prototype.submit",
		"ddgJDownloaderBunkrCompatV8563",
	} {
		if !strings.Contains(s, marker) {
			t.Fatalf("Bunkr JD compatibility module missing marker %q", marker)
		}
	}

	boot, err := webFS.ReadFile("web/preview_quick_v86.js")
	if err != nil {
		t.Fatal(err)
	}
	bs := string(boot)
	activity := strings.Index(bs, "/check_activity_v8563.js")
	compat := strings.Index(bs, "/jdownloader_bunkr_compat_v8563.js")
	final := strings.Index(bs, "/jdownloader_final_v8551.js")
	if activity < 0 || compat < 0 || final < 0 {
		t.Fatal("new TEST modules are not loaded by compatibility bootstrap")
	}
	if compat > final {
		t.Fatal("Bunkr JD compatibility module must load before final JDownloader routing")
	}
}
