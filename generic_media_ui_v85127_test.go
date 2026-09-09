package main

import (
	"os"
	"strings"
	"testing"
)

// This guard is intentionally file-level as well as runtime-level: dedicated
// providers must never be routed through the generic Media Picker discovery.
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
		"/api/generic-media/discover",
		"ddg:generic-media-discovered",
		"discoveryOnly:true",
	} {
		if !strings.Contains(js, token) {
			t.Fatalf("advanced generic UI missing %q", token)
		}
	}
	if strings.Contains(js, "adapter:'http'") || strings.Contains(js, "body:JSON.stringify({urls:discoveredURLs") {
		t.Fatal("discovery stage must not compare raw URLs before the user selects media")
	}

	picker, err := os.ReadFile("web/media_picker_v8566.js")
	if err != nil {
		t.Fatal(err)
	}
	pickerJS := string(picker)
	for _, token := range []string{
		"openDiscovery",
		"Compară selectatele cu PC",
		"/api/generic-media/preview?token=",
		"qualities",
		"Video ${Number(totals.video || 0)",
		"discoveryWarnings",
		"/api/source/batch",
		"adapter:'auto'",
		"ddg:source-scan-complete",
	} {
		if !strings.Contains(pickerJS, token) {
			t.Fatalf("two-stage Media Picker missing %q", token)
		}
	}
}
