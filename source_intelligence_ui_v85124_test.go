package main

import (
	"os"
	"strings"
	"testing"
)

func mustReadSourceIntelligenceFileV85124(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestSourceIntelligenceSlotsV85124(t *testing.T) {
	provider := mustReadSourceIntelligenceFileV85124(t, "web/provider_sources.js")
	for _, token := range []string{
		"ddgSourceIntelligenceV85124",
		"ddgSourceHistorySlotV85124",
		"ddgSourceFolderSlotV85124",
		"ddgSourceMediaSlotV85124",
		"/features/source_history_v85124.js",
		"ddg:source-scan-start",
		"ddg:source-scan-complete",
	} {
		if !strings.Contains(provider, token) {
			t.Fatalf("provider_sources.js missing %q", token)
		}
	}
}

func TestGenericMediaBridgeUsesExplicitScanEventsV85124(t *testing.T) {
	js := mustReadSourceIntelligenceFileV85124(t, "web/feature_generic_media_picker_v85114.js")
	if strings.Contains(js, "new MutationObserver") {
		t.Fatal("generic Media Picker bridge must not watch mutable status text")
	}
	for _, token := range []string{"ddg:source-scan-complete", "ddgSourceMediaInfoV85124", "ddgSourceFolderSlotV85124"} {
		if !strings.Contains(js, token) {
			t.Fatalf("generic Media Picker bridge missing %q", token)
		}
	}
}

func TestSourceHistoryTargetsUnifiedSlotV85124(t *testing.T) {
	js := mustReadSourceIntelligenceFileV85124(t, "web/features/source_history_v85124.js")
	for _, token := range []string{"ddgSourceHistorySlotV85124", "source_history.json", "JDownloader"} {
		if !strings.Contains(js, token) {
			t.Fatalf("source history UI missing %q", token)
		}
	}
}
