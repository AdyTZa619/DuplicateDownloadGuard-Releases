package main

import (
	"strings"
	"testing"
)

func TestUniversalProviderSourcesAreEmbeddedAndLoaded(t *testing.T) {
	providerJS, err := webFS.ReadFile("web/provider_sources.js")
	if err != nil {
		t.Fatalf("read provider sources module: %v", err)
	}
	s := string(providerJS)
	for _, marker := range []string{
		"GoFile",
		"Bunkr",
		"Cyberdrop",
		"gallery-dl",
		"scanUniversal",
		"/api/source/scan",
		"/api/tools/manage",
	} {
		if !strings.Contains(s, marker) {
			t.Fatalf("provider sources module missing marker %q", marker)
		}
	}

	bootstrap, err := webFS.ReadFile("web/preview_quick_v86.js")
	if err != nil {
		t.Fatalf("read compatibility bootstrap: %v", err)
	}
	if !strings.Contains(string(bootstrap), "/provider_sources.js") {
		t.Fatal("provider_sources.js is not loaded by compatibility bootstrap")
	}
}
