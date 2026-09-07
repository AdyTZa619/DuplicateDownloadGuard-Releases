package main

import (
	"os"
	"strings"
	"testing"
)

func TestLocalPreviewSingleWinnerGuardsStaleAsyncWork(t *testing.T) {
	b, err := os.ReadFile("web/local_preview_resilience_v8571.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, marker := range []string{
		"function generationOf(img)",
		"function isCurrent(img",
		"generation !== previewGeneration",
		"data-ddg-generation=\"${generation}\"",
		"ddgSettled === 'success'",
		"if (!isCurrent(img, generation)",
		"function stopDirectRequest(img, generation)",
		"img.removeAttribute('src')",
		"if (stage === 'blob-fetch') return",
		"const old = document.getElementById('localImage')",
		"cancelPreviewWork(old)",
	} {
		if !strings.Contains(s, marker) {
			t.Fatalf("single-winner local preview missing marker %q", marker)
		}
	}
}

func TestLocalPreviewRaceGuardKeepsFastPathAndAvoidsRescan(t *testing.T) {
	b, err := os.ReadFile("web/local_preview_resilience_v8571.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, marker := range []string{
		"DIRECT_WATCHDOG_MS = 1250",
		"/api/local-preview?path=",
		"'Range': `bytes=0-${PROBE_BYTES - 1}`",
		"IMAGE • DIRECT OK",
		"IMAGE • FALLBACK OK",
	} {
		if !strings.Contains(s, marker) {
			t.Fatalf("local preview fast/fallback path missing marker %q", marker)
		}
	}
	for _, forbidden := range []string{"/api/index/start", "/api/mega/scan", "/api/download/preflight"} {
		if strings.Contains(s, forbidden) {
			t.Fatalf("local preview race guard must not trigger %q", forbidden)
		}
	}
}
