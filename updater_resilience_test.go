package main

import (
	"strings"
	"testing"
)

func TestUpdaterResilienceModuleIsEmbeddedAndLoaded(t *testing.T) {
	js, err := webFS.ReadFile("web/updater_resilience.js")
	if err != nil {
		t.Fatalf("read updater resilience module: %v", err)
	}
	s := string(js)
	for _, marker := range []string{
		"getJSONWithReconnect",
		"/api/update/check",
		"/api/update/status",
		"cache: 'no-store'",
		"attempts = 3",
	} {
		if !strings.Contains(s, marker) {
			t.Fatalf("updater resilience module missing marker %q", marker)
		}
	}

	bootstrap, err := webFS.ReadFile("web/preview_quick_v86.js")
	if err != nil {
		t.Fatalf("read compatibility bootstrap: %v", err)
	}
	if !strings.Contains(string(bootstrap), "/updater_resilience.js") {
		t.Fatal("updater_resilience.js is not loaded by compatibility bootstrap")
	}
}
