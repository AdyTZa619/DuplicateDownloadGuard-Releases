package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDownloadHistoryDecisionUsesOnlyExistingCompletedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "already.mp4")
	if err := os.WriteFile(path, []byte("ok"), 0600); err != nil {
		t.Fatal(err)
	}
	res := Result{ID: 7, Remote: RemoteItem{Name: "already.mp4"}, DownloadedAt: time.Now().Unix(), DownloadPath: path}
	got, ok := downloadHistoryDecision(res)
	if !ok {
		t.Fatal("completed download was not recognized")
	}
	if got.Method != "download-history" || got.UserStatus != userDownloaded || got.Action != actionDontDownload {
		t.Fatalf("unexpected history decision: %#v", got)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, ok := downloadHistoryDecision(res); ok {
		t.Fatal("missing historical file must not block a new download")
	}
}
