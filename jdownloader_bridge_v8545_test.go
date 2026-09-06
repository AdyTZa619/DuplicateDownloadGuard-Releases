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
