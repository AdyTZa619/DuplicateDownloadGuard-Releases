package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDownloadHistoryDecisionUsesOnlyExistingUnchangedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "already.mp4")
	if err := os.WriteFile(path, []byte("ok"), 0600); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	res := Result{ID: 7, Remote: RemoteItem{Name: "already.mp4", Size: st.Size()}, DownloadedAt: time.Now().Unix(), DownloadPath: path}
	got, ok := downloadHistoryDecision(res)
	if !ok {
		t.Fatal("completed unchanged download was not recognized")
	}
	if got.Method != "download-history" || got.UserStatus != userDownloaded || got.Action != actionDontDownload {
		t.Fatalf("unexpected history decision: %#v", got)
	}

	replacedSize := res
	replacedSize.Remote.Size++
	if _, ok := downloadHistoryDecision(replacedSize); ok {
		t.Fatal("different current size must invalidate download history")
	}

	future := time.Unix(res.DownloadedAt+30, 0)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatal(err)
	}
	if _, ok := downloadHistoryDecision(res); ok {
		t.Fatal("file modified after the recorded download must not be trusted as downloaded history")
	}

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, ok := downloadHistoryDecision(res); ok {
		t.Fatal("missing historical file must not block a new download")
	}
}

func TestDownloadHistoryKeyV85IsStableAndSpecific(t *testing.T) {
	a := downloadHistoryKeyV85("mega", "https://mega.nz/folder/abc#secret/file/node", "clip.mp4", 12345)
	b := downloadHistoryKeyV85(" MEGA ", "https://mega.nz/folder/abc#secret/file/node", "RENAMED.mp4", 12345)
	if a != b {
		t.Fatalf("URL-backed identity should be independent of display filename: %q != %q", a, b)
	}
	if a == downloadHistoryKeyV85("MEGA", "https://mega.nz/folder/abc#secret/file/other", "clip.mp4", 12345) {
		t.Fatal("different remote node must produce a different history key")
	}
	if a == downloadHistoryKeyV85("MEGA", "https://mega.nz/folder/abc#secret/file/node", "clip.mp4", 12346) {
		t.Fatal("different remote size must produce a different history key")
	}
	if len(a) != 64 {
		t.Fatalf("history key should be SHA-256 hex, got length %d", len(a))
	}
}
