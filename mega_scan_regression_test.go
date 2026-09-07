package main

import (
	"os"
	"strings"
	"testing"
)

func TestMegaScanPathIsPresentAndIndependentFromLocalPreview(t *testing.T) {
	indexBytes, err := os.ReadFile("web/index.html")
	if err != nil {
		t.Fatal(err)
	}
	index := string(indexBytes)
	for _, marker := range []string{
		"async function scanMega()",
		"/api/mega/scan",
		"onclick=\"scanMega()\"",
	} {
		if !strings.Contains(index, marker) {
			t.Fatalf("MEGA scan path missing from main UI: %q", marker)
		}
	}

	providerBytes, err := os.ReadFile("web/provider_sources.js")
	if err != nil {
		t.Fatal(err)
	}
	provider := string(providerBytes)
	for _, marker := range []string{
		"provider?.id === 'mega'",
		"window.scanMega()",
	} {
		if !strings.Contains(provider, marker) {
			t.Fatalf("universal MEGA routing missing marker: %q", marker)
		}
	}

	bootBytes, err := os.ReadFile("web/preview_quick_v86.js")
	if err != nil {
		t.Fatal(err)
	}
	boot := string(bootBytes)
	if !strings.Contains(boot, "/provider_sources.js") {
		t.Fatal("provider_sources.js is not loaded by the UI bootstrap")
	}
	if !strings.Contains(boot, "/local_preview_direct_safe_v8576.js") {
		t.Fatal("TEST92 direct-safe local preview is not loaded")
	}

	localBytes, err := os.ReadFile("web/local_preview_direct_safe_v8576.js")
	if err != nil {
		t.Fatal(err)
	}
	local := string(localBytes)
	for _, forbidden := range []string{"/api/mega/scan", "scanMega(", "window.scanMega", "/api/remote-preview/"} {
		if strings.Contains(local, forbidden) {
			t.Fatalf("LOCAL preview must not touch MEGA scan/preview path: %q", forbidden)
		}
	}
}
