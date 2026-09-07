package main

import (
	"os"
	"strings"
	"testing"
)

func TestLocalPreviewV8571HasSilentHangRecovery(t *testing.T) {
	b, err := os.ReadFile("web/local_preview_resilience_v8571.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, marker := range []string{
		"DIRECT_WATCHDOG_MS = 1250",
		"FETCH_TIMEOUT_MS = 7000",
		"function armWatchdog(img)",
		"void blobRetry(img, 'watchdog')",
		"function sniffImageMime",
		"'Range': `bytes=0-${PROBE_BYTES - 1}`",
		"image/jpeg",
		"image/png",
		"image/webp",
		"image/avif",
		"AbortController",
		"IMAGE • FALLBACK OK",
	} {
		if !strings.Contains(s, marker) {
			t.Fatalf("local preview v8571 missing regression marker %q", marker)
		}
	}
}

func TestPreviewBootstrapUsesOnlyLocalPreviewV8574(t *testing.T) {
	b, err := os.ReadFile("web/preview_quick_v86.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, "/local_preview_thumb_v8574.js") {
		t.Fatal("preview bootstrap does not load thumbnail-first local preview v8574")
	}
	for _, stale := range []string{
		"/local_preview_resilience_v8570.js",
		"/local_preview_resilience_v8571.js",
		"/local_preview_fast_v8573.js",
	} {
		if strings.Contains(s, stale) {
			t.Fatalf("preview bootstrap still loads stale local preview owner %s", stale)
		}
	}
}
