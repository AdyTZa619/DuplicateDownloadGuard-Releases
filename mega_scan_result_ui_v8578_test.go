package main

import (
	"os"
	"strings"
	"testing"
)

func TestMegaScanResultUIV8578ShowsFreshResultsWithoutTouchingBackend(t *testing.T) {
	b, err := os.ReadFile("web/mega_scan_result_ui_v8578.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, marker := range []string{
		"let inFlight = false",
		"const before = await resultRevision()",
		"await original.apply(this, args)",
		"after > before",
		"window.goTab('results')",
		"window.loadResults",
		"window.refreshStats",
		"Scanarea MEGA este deja în curs",
	} {
		if !strings.Contains(s, marker) {
			t.Fatalf("MEGA result UI missing marker %q", marker)
		}
	}
	for _, forbidden := range []string{
		"/api/mega/scan",
		"/api/index/start",
		"/api/local-preview",
		"/api/download/preflight",
	} {
		if strings.Contains(s, forbidden) {
			t.Fatalf("MEGA result UI must not call backend path %q", forbidden)
		}
	}
}

func TestMegaScanResultUIV8578LoadedByBootstrap(t *testing.T) {
	b, err := os.ReadFile("web/preview_quick_v86.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, "/mega_scan_result_ui_v8578.js") || !strings.Contains(s, "ddgMegaScanResultUIV8578Script") {
		t.Fatal("MEGA result visibility module is not loaded by bootstrap")
	}
}
