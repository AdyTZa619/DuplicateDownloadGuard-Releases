package main

import (
	"os"
	"strings"
	"testing"
)

func TestLocalPreviewDirectSafeV8576ExactFastPathAndIsolation(t *testing.T) {
	b, err := os.ReadFile("web/local_preview_direct_safe_v8576.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, marker := range []string{
		"SAFE_FIRST_IMAGE_EXTS",
		"['heic', 'heif', 'tif', 'tiff']",
		"return '/api/local-preview?path=' + encodeURIComponent(path)",
		"return '/api/local-thumb?path=' + encodeURIComponent(path)",
		"img.removeAttribute('src')",
		"loading=\"eager\"",
		"decoding=\"async\"",
		"fetchpriority=\"high\"",
		"preload=\"metadata\"",
		"data-ddg-stage=\"${stage}\"",
		"switchSafe(img, 'direct-error')",
	} {
		if !strings.Contains(s, marker) {
			t.Fatalf("missing TEST93 fast-preview marker %q", marker)
		}
	}
	for _, forbidden := range []string{
		"IMAGE_WATCHDOG_MS",
		"setTimeout(",
		"direct-watchdog",
		"&_ddg=",
		"URL.createObjectURL",
		"arrayBuffer()",
		"preload=\"none\"",
		"/api/mega/scan",
		"/api/remote-preview/",
		"/api/download/preflight",
		"/api/index/start",
	} {
		if strings.Contains(s, forbidden) {
			t.Fatalf("TEST93 fast path contains forbidden regression %q", forbidden)
		}
	}
}
