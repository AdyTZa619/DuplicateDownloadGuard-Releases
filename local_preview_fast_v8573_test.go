package main

import (
	"os"
	"strings"
	"testing"
)

func TestLocalPreviewThumbV8574UsesCachedDerivativeAndLazyMedia(t *testing.T) {
	b, err := os.ReadFile("web/local_preview_thumb_v8574.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, marker := range []string{
		"/api/local-thumb?path=",
		"/api/local-preview?path=",
		"IMAGE • CACHE",
		"VIDEO • POSTER CACHE",
		"preload=\"none\"",
		"ddgVideoShell",
		"activateVideo",
		"onPosterError",
	} {
		if !strings.Contains(s, marker) {
			t.Fatalf("thumbnail-first local preview missing marker %q", marker)
		}
	}
	for _, forbidden := range []string{
		"DIRECT_WATCHDOG_MS = 1250",
		"IMAGE_HARD_TIMEOUT_MS",
		"fetch(freshURL",
		"/api/index/start",
		"/api/mega/scan",
		"/api/download/preflight",
	} {
		if strings.Contains(s, forbidden) {
			t.Fatalf("thumbnail-first preview must not contain old/expensive path %q", forbidden)
		}
	}
}

func TestLocalPreviewThumbV8574IsSoleActiveOwner(t *testing.T) {
	b, err := os.ReadFile("web/preview_quick_v86.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, "/local_preview_thumb_v8574.js") {
		t.Fatal("TEST89 thumbnail-first module is not loaded")
	}
	for _, old := range []string{
		"/local_preview_resilience_v8571.js",
		"/local_preview_fast_v8573.js",
	} {
		if strings.Contains(s, old) {
			t.Fatalf("legacy local preview owner still active: %s", old)
		}
	}
}
