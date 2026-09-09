package main

import (
	"os"
	"strings"
	"testing"
)

func TestBunkrPreviewDoesNotUseAutoPreloadV85118(t *testing.T) {
	b, err := os.ReadFile("web/provider_buffer_v8561.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if strings.Contains(s, `preload="auto"`) {
		t.Fatal("Bunkr preview must not use preload=auto; it can make Edge aggressively buffer large remote videos")
	}
	if !strings.Contains(s, `preload="metadata"`) {
		t.Fatal("Bunkr preview must use metadata-only preload")
	}
	if !strings.Contains(s, "streaming la cerere") {
		t.Fatal("Bunkr UI should describe the bounded on-demand buffering policy")
	}
}
