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

func TestPreviewBootstrapUsesLocalPreviewV8571(t *testing.T) {
	b, err := os.ReadFile("web/preview_quick_v86.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, "/local_preview_resilience_v8571.js") {
		t.Fatal("preview bootstrap does not load local preview v8571")
	}
	if strings.Contains(s, "/local_preview_resilience_v8570.js") {
		t.Fatal("preview bootstrap still loads stale local preview v8570")
	}
}
