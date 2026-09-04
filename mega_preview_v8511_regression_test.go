package main

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestExactWebDAVRootParserIgnoresPerFileFirstV8511(t *testing.T) {
	listing := strings.Join([]string{
		"WEBDAV SERVED LOCATIONS:",
		"H:FILE: http://127.0.0.1:4443/abc/movie.mp4",
		"/: http://127.0.0.1:4443/abc/Cloud%20Drive",
	}, "\n")
	got := extractExactWebDAVURLV8511(listing, megaWarmRootRefV86)
	want := "http://127.0.0.1:4443/abc/Cloud%20Drive"
	if got != want {
		t.Fatalf("expected exact root %q, got %q", want, got)
	}
}

func TestExactWebDAVRootParserUnderstandsStartOutputV8511(t *testing.T) {
	out := "Serving via webdav /: http://127.0.0.1:4443/token/Cloud%20Drive\n"
	got := extractExactWebDAVURLV8511(out, megaWarmRootRefV86)
	if got != "http://127.0.0.1:4443/token/Cloud%20Drive" {
		t.Fatalf("unexpected root URL: %q", got)
	}
}

func TestTrueFallbackNeverDeletesWarmRootV8511(t *testing.T) {
	old := MegaPreviewState{
		Active:     true,
		SourceURL:  "https://mega.nz/folder/X#KEY",
		RemotePath: megaWarmRootRefV86,
		StreamURL:  "http://127.0.0.1:4443/token/Cloud%20Drive",
		Exe:        "mega-webdav",
	}
	var commands []string
	run := func(_ time.Duration, args ...string) (string, error) {
		commands = append(commands, strings.Join(args, " "))
		if len(args) == 2 && args[0] == "webdav" && args[1] == "H:NEW" {
			return "Serving via webdav H:NEW: http://127.0.0.1:4443/token/new.mp4", nil
		}
		return "", nil
	}
	result, err := switchWarmRootToPerFileV858(old, "H:NEW", run)
	if err != nil {
		t.Fatal(err)
	}
	if result.StreamURL == "" {
		t.Fatal("fallback must return a stream URL")
	}
	for _, cmd := range commands {
		if cmd == "webdav -d /" {
			t.Fatalf("fallback deleted canonical root: %v", commands)
		}
	}
	if len(commands) != 1 || commands[0] != "webdav H:NEW" {
		t.Fatalf("unexpected fallback command sequence: %v", commands)
	}
}

func TestBrowserFallbackHasSingleAuthoritativeRetryV8511(t *testing.T) {
	src, err := os.ReadFile("web/preview_quick_v86.js")
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	if !strings.Contains(text, "MEGA TRUE FALLBACK • EROARE") {
		t.Fatal("new fallback must suppress the older exact_guard fallback after failure")
	}
	if !strings.Contains(text, "Root-ul rămâne activ pentru următorul fișier") {
		t.Fatal("UI must describe root-preserving fallback behavior")
	}
	if !strings.Contains(text, "MEGA DIRECT RESUME") {
		t.Fatal("cold root-resume mode must be eligible for one browser retry")
	}
}
