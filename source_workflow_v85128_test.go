package main

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestSourceFolderNormalizeV85128(t *testing.T) {
	cases := map[string]string{
		`H:/trans/Ana/`:          `H:\trans\Ana`,
		`C:\Downloads\Models`: `C:\Downloads\Models`,
		`\\NAS\Media\Ana\`:  `\\NAS\Media\Ana`,
	}
	for raw, want := range cases {
		got, err := normalizeSourceFolderV85128(raw)
		if err != nil {
			t.Fatalf("normalize %q: %v", raw, err)
		}
		if got != want {
			t.Fatalf("normalize %q = %q, want %q", raw, got, want)
		}
	}
	for _, bad := range []string{"relative\\folder", "", "H:folder", "../folder"} {
		if _, err := normalizeSourceFolderV85128(bad); err == nil {
			t.Fatalf("expected invalid folder for %q", bad)
		}
	}
}

func TestSourceFolderLearningExactWinsV85128(t *testing.T) {
	now := time.Now()
	key, err := canonicalSourceURLV85124("https://example.com/model/ana")
	if err != nil {
		t.Fatal(err)
	}
	host, family := sourceFolderSourceShapeV85128(key)
	otherKey, _ := canonicalSourceURLV85124("https://example.com/model/maria")
	store := sourceFolderLearningStoreV85128{Version: 1, Links: map[string]sourceFolderProfileV85128{
		key: {
			Key: key, Host: host, Family: family,
			Observations: []sourceFolderObservationV85128{{At: now.UnixMilli(), Folder: `H:\trans\Ana`, Confidence: "ridicata", Evidence: 8}},
		},
		otherKey: {
			Key: otherKey, Host: host, Family: family,
			Observations: []sourceFolderObservationV85128{{At: now.UnixMilli(), Folder: `H:\trans\Maria`, Confidence: "ridicata", Evidence: 10}},
		},
	}}
	got := sourceFolderLearningSuggestFromStoreV85128(store, key, host, family, now)
	if len(got) < 2 {
		t.Fatalf("expected at least two suggestions, got %#v", got)
	}
	if got[0].Folder != `H:\trans\Ana` || got[0].Exact < 1 {
		t.Fatalf("exact source should win, got %#v", got)
	}
}

func TestSourceFolderLearningHostFallbackV85128(t *testing.T) {
	now := time.Now()
	queryKey, _ := canonicalSourceURLV85124("https://site.example/gallery/new")
	host, family := sourceFolderSourceShapeV85128(queryKey)
	knownKey, _ := canonicalSourceURLV85124("https://site.example/gallery/old")
	store := sourceFolderLearningStoreV85128{Version: 1, Links: map[string]sourceFolderProfileV85128{
		knownKey: {
			Key: knownKey, Host: host, Family: family,
			Observations: []sourceFolderObservationV85128{{At: now.UnixMilli(), Folder: `H:\Scrape\Site`, Confidence: "medie", Evidence: 4}},
		},
	}}
	got := sourceFolderLearningSuggestFromStoreV85128(store, queryKey, host, family, now)
	if len(got) != 1 || got[0].Folder != `H:\Scrape\Site` || got[0].Family < 1 {
		t.Fatalf("family fallback missing: %#v", got)
	}
}

func TestGenericMediaRemainsGenericOnlyV85128(t *testing.T) {
	for _, raw := range []string{
		"https://erome.com/a/demo",
		"https://gofile.io/d/demo",
		"https://bunkr.cr/a/demo",
		"https://cyberdrop.me/a/demo",
		"https://mega.nz/folder/demo#key",
	} {
		if genericMediaAllowedRootV85127(raw) {
			t.Fatalf("dedicated provider leaked into generic Media Picker: %s", raw)
		}
	}
	if !genericMediaAllowedRootV85127("https://filmepornonline.org/demo") {
		t.Fatal("generic HTTP site should remain eligible for advanced Media Picker")
	}
}

func TestFolderAdvisorV2UIContractV85128(t *testing.T) {
	b, err := os.ReadFile("web/feature_source_folder_hint_v85114.js")
	if err != nil {
		t.Fatal(err)
	}
	js := string(b)
	for _, token := range []string{
		"ddg-source-folder-learning-v1",
		"ddgSourceDecisionV85128",
		"SITUAȚIE CURENTĂ",
		"ddgDecisionTotalV85128",
		"ddgDecisionLocalV85128",
		"ddgDecisionMissingV85128",
		"ddgDecisionSelectedV85128",
		"ddgDecisionFolderV85128",
		"ddg:source-scan-complete",
		"ddg:folder-advisor-updated",
		"lastReport",
	} {
		if !strings.Contains(js, token) {
			t.Fatalf("Folder Advisor v2 missing %q", token)
		}
	}
	if strings.Contains(js, "document.body MutationObserver") || strings.Contains(js, ".observe(document.body") {
		t.Fatal("Folder Advisor must not restore the old self-triggering body observer")
	}
}

func TestProviderRoutingStillDedicatedV85128(t *testing.T) {
	provider, err := os.ReadFile("web/provider_sources.js")
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ToLower(string(provider))
	for _, token := range []string{"id:'mega'", "id:'gofile'", "id:'bunkr'", "id:'cyberdrop'", "id:'erome'", "provider?.id === 'web'"} {
		if !strings.Contains(text, strings.ToLower(token)) {
			t.Fatalf("provider routing missing %q", token)
		}
	}
	bridge, err := os.ReadFile("web/feature_generic_media_picker_v85114.js")
	if err != nil {
		t.Fatal(err)
	}
	bridgeText := strings.ToLower(string(bridge))
	for _, token := range []string{"return 'erome'", "return 'gofile'", "return 'bunkr'", "return 'cyberdrop'", "return 'mega'", "return 'web'"} {
		if !strings.Contains(bridgeText, token) {
			t.Fatalf("generic Media Picker provider guard missing %q", token)
		}
	}
}
