package main

import (
	"os"
	"strings"
	"testing"
)

func TestLocalPreviewDirectSafeV8576RestoresFastDirectPath(t *testing.T) {
	b, err := os.ReadFile("web/local_preview_direct_safe_v8576.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, marker := range []string{
		"SAFE_FIRST_IMAGE_EXTS",
		"/api/local-preview?path=",
		"/api/local-thumb?path=",
		"preload=\"metadata\"",
		"img.removeAttribute('src')",
		"loading=\"eager\"",
		"decoding=\"async\"",
		"fetchpriority=\"high\"",
		"function switchSafe",
	} {
		if !strings.Contains(s, marker) {
			t.Fatalf("TEST93 direct-safe local preview missing marker %q", marker)
		}
	}
	for _, forbidden := range []string{
		"IMAGE_WATCHDOG_MS",
		"direct-watchdog",
		"setTimeout(",
		"URL.createObjectURL",
		"response.arrayBuffer()",
		"preload=\"none\"",
		"VIDEO • POSTER CACHE",
		"/api/index/start",
		"/api/mega/scan",
		"/api/download/preflight",
	} {
		if strings.Contains(s, forbidden) {
			t.Fatalf("TEST93 direct-safe preview must not contain expensive/regressive path %q", forbidden)
		}
	}
}

func TestLocalPreviewDirectSafeV8576IsSoleActiveOwner(t *testing.T) {
	b, err := os.ReadFile("web/preview_quick_v86.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, "/local_preview_direct_safe_v8576.js") {
		t.Fatal("TEST93 direct-safe module is not loaded")
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
