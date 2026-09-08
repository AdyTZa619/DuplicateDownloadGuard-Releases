package main

import (
	"os"
	"strings"
	"testing"
)

func TestBunkrRangeProbeHasHardTimeout(t *testing.T) {
	b, err := os.ReadFile("web/provider_buffer_v8561.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, marker := range []string{
		"const PROBE_TIMEOUT_MS = 12000;",
		"setTimeout(() => {",
		"controller.abort();",
		"clearTimeout(timeoutID);",
		"state.probeTimedOut = true;",
		"Oprirea testului previne blocarea preview-ului",
	} {
		if !strings.Contains(s, marker) {
			t.Fatalf("missing Bunkr timeout guard marker %q", marker)
		}
	}
}

func TestBunkrRangeTimeoutRemainsProviderScoped(t *testing.T) {
	b, err := os.ReadFile("web/provider_buffer_v8561.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, "currentSource() !== 'BUNKR'") {
		t.Fatal("Bunkr watchdog is no longer provider-scoped")
	}
	for _, forbidden := range []string{
		"/api/mega/scan",
		"/api/local-preview",
		"/api/index/start",
		"/api/download/preflight",
	} {
		if strings.Contains(s, forbidden) {
			t.Fatalf("Bunkr timeout module unexpectedly touches protected route %q", forbidden)
		}
	}
}
