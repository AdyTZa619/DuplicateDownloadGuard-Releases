package main

import (
	"strings"
	"testing"
)

func TestPreviewQuickV86IsEmbeddedAndLoaded(t *testing.T) {
	js, err := webFS.ReadFile("web/preview_quick_v86.js")
	if err != nil {
		t.Fatalf("read preview quick script: %v", err)
	}
	s := string(js)
	for _, required := range []string{
		"remoteQuickV86",
		"localQuickV86",
		"loadedmetadata",
		"visualScore",
		"matchScore",
		"ACELAȘI CONȚINUT",
		"Downloader intern (Auto); aria2 opțional",
	} {
		if !strings.Contains(s, required) {
			t.Fatalf("quick preview summary missing marker %q", required)
		}
	}

	page, err := webFS.ReadFile("web/index.html")
	if err != nil {
		t.Fatalf("read embedded index: %v", err)
	}
	if !strings.Contains(string(page), `src="/preview_quick_v86.js"`) {
		t.Fatal("preview_quick_v86.js is not loaded by index.html")
	}
}
