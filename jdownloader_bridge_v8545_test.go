package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestJDownloaderGoFileUsesStablePerFileURLV8545(t *testing.T) {
	res := Result{Remote: RemoteItem{
		Source: "GOFILE", URL: "https://gofile.io/d/lNtYpg", ProviderID: "abc123",
		DirectURL: "https://store9.gofile.io/signed/temporary.mp4",
	}}
	got := jdownloaderURLForResultV8545(res)
	if got != "https://gofile.io/?c=lNtYpg#file=abc123" {
		t.Fatalf("unexpected GoFile JD URL: %s", got)
	}
}

func TestJDownloaderDedicatedProvidersUseStablePerFileURLsV85130(t *testing.T) {
	cases := []struct {
		remote RemoteItem
		want   string
	}{
		{RemoteItem{Source: "BUNKR", URL: "https://bunkr.example/a/album", Handle: "file-handle", DirectURL: "https://cdn.example/temporary.mp4"}, "https://bunkr.example/f/file-handle"},
		{RemoteItem{Source: "CYBERDROP", URL: "https://cyberdrop.example/a/album", ProviderID: "file-id", DirectURL: "https://cdn.example/temporary.jpg"}, "https://cyberdrop.example/f/file-id"},
	}
	for _, tc := range cases {
		if got := jdownloaderURLForResultV8545(Result{Remote: tc.remote}); got != tc.want {
			t.Fatalf("source=%s URL=%q want=%q", tc.remote.Source, got, tc.want)
		}
	}
}

func TestJDownloaderBackendFailSafeAllowsOnlyMissingV85130(t *testing.T) {
	b, err := os.ReadFile("jdownloader_direct_v8550.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, marker := range []string{
		"if decision.Verdict == guardDownload",
		"form.Set(\"descriptions\"",
		"form.Set(\"fnames\"",
		"form.Set(\"package\"",
	} {
		if !strings.Contains(s, marker) {
			t.Fatalf("JD backend fail-safe missing %q", marker)
		}
	}
	if strings.Contains(s, "decision.Verdict == guardReview") {
		t.Fatal("JD backend still allows REVIEW rows to bypass the missing-only rule")
	}
}

func TestWriteJDownloaderCrawlJobIsValidJSONV8545(t *testing.T) {
	path := filepath.Join(t.TempDir(), "DDG.crawljob")
	if err := writeJDownloaderCrawlJobV8545(path, []string{"https://example.com/a", "https://example.com/b"}, `H:\\Downloads`); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var jobs []jdCrawlJobV8545
	if err := json.Unmarshal(b, &jobs); err != nil {
		t.Fatalf("invalid crawljob JSON: %v\n%s", err, b)
	}
	if len(jobs) != 2 || jobs[0].AutoStart != "FALSE" || jobs[0].AutoConfirm != "FALSE" || !strings.Contains(jobs[0].DownloadFolder, "Downloads") {
		t.Fatalf("unexpected crawljob: %#v", jobs)
	}
}
