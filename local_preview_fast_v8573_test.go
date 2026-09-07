package main

import (
	"os"
	"strings"
	"testing"
)

func TestLocalPreviewFastV8573UsesStableCachedDirectURLs(t *testing.T) {
	b, err := os.ReadFile("web/local_preview_fast_v8573.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, marker := range []string{
		"function stableURL(path)",
		"return `/api/local-preview?path=${encodeURIComponent(path)}`",
		"const stable = stableURL(path)",
		"preload=\"metadata\" src=\"${stable}\"",
		"onloadedmetadata=\"ddgLocalPreviewFastV8573.onMediaReady",
		"IMAGE_HARD_TIMEOUT_MS = 8000",
		"IMAGE • DIRECT OK",
		"IMAGE • FALLBACK OK",
	} {
		if !strings.Contains(s, marker) {
			t.Fatalf("fast local preview missing marker %q", marker)
		}
	}
	if strings.Contains(s, "DIRECT_WATCHDOG_MS = 1250") {
		t.Fatal("fast local preview must not restore the aggressive 1.25s watchdog")
	}
	for _, forbidden := range []string{"/api/index/start", "/api/mega/scan", "/api/download/preflight"} {
		if strings.Contains(s, forbidden) {
			t.Fatalf("local preview fast path must not trigger %q", forbidden)
		}
	}
}

func TestLocalPreviewFastV8573LoadsAfterResilienceOverride(t *testing.T) {
	b, err := os.ReadFile("web/preview_quick_v86.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	oldPos := strings.Index(s, "/local_preview_resilience_v8571.js")
	fastPos := strings.Index(s, "/local_preview_fast_v8573.js")
	if oldPos < 0 || fastPos < 0 || fastPos <= oldPos {
		t.Fatal("TEST88 fast local preview must load after the resilience module")
	}
}
