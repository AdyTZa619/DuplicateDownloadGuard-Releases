package main

import (
	"os"
	"strings"
	"testing"
)

func TestLocalPreviewRobustV8575UsesDirectThenSafeImageFallback(t *testing.T) {
	b, err := os.ReadFile("web/local_preview_robust_v8575.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, marker := range []string{
		"IMAGE_FALLBACK_MS = 2500",
		"/api/local-preview?path=",
		"/api/local-thumb?path=",
		"function switchToSafeImage",
		"IMAGE • DIRECT OK",
		"IMAGE • SAFE OK",
		"preload=\"metadata\"",
		"onloadedmetadata=\"ddgLocalPreviewRobustV8575.onMediaReady(this,'VIDEO')\"",
		"onloadedmetadata=\"ddgLocalPreviewRobustV8575.onMediaReady(this,'AUDIO')\"",
	} {
		if !strings.Contains(s, marker) {
			t.Fatalf("robust local preview missing marker %q", marker)
		}
	}
	for _, forbidden := range []string{
		"URL.createObjectURL",
		"response.arrayBuffer()",
		"ddgVideoShell",
		"preload=\"none\"",
		"/api/index/start",
		"/api/mega/scan",
		"/api/download/preflight",
	} {
		if strings.Contains(s, forbidden) {
			t.Fatalf("robust local preview must not contain stale/expensive path %q", forbidden)
		}
	}
}

func TestLocalPreviewRobustV8575IsSoleActiveOwner(t *testing.T) {
	b, err := os.ReadFile("web/preview_quick_v86.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, "/local_preview_robust_v8575.js") {
		t.Fatal("TEST90 robust local preview module is not loaded")
	}
	for _, old := range []string{
		"/local_preview_resilience_v8570.js",
		"/local_preview_resilience_v8571.js",
		"/local_preview_fast_v8573.js",
		"/local_preview_thumb_v8574.js",
	} {
		if strings.Contains(s, old) {
			t.Fatalf("legacy local preview owner still active: %s", old)
		}
	}
}
