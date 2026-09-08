package main

import (
	"os"
	"strings"
	"testing"
)

func TestGenericMediaAdvancedUIIsGenericOnlyV85127(t *testing.T) {
	bridge, err := os.ReadFile("web/feature_generic_media_picker_v85114.js")
	if err != nil {
		t.Fatal(err)
	}
	text := string(bridge)
	for _, token := range []string{
		"/features/generic_media_discovery_v85127.js",
		"HTML/iframe + HLS/DASH + yt-dlp + gallery-dl",
		"erome",
		"bunkr",
		"cyberdrop",
		"gofile",
		"mega",
	} {
		if !strings.Contains(strings.ToLower(text), strings.ToLower(token)) {
			t.Fatalf("generic bridge missing %q", token)
		}
	}

	advanced, err := os.ReadFile("web/features/generic_media_discovery_v85127.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(advanced)
	for _, token := range []string{
		"ddg-generic-media-v1",
		"adapterIsAuto",
		"isGeneric",
		"addEventListener('click', interceptClick, true)",
		"addEventListener('keydown', interceptEnter, true)",
		"/api/source/batch",
		"adapter:'http'",
		"adapter:'auto'",
		"ddg:source-scan-complete",
	} {
		if !strings.Contains(js, token) {
			t.Fatalf("advanced generic UI missing %q", token)
		}
	}
}
