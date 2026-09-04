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
		t.Fatalf("MEGA handle identity should survive a display-name change: %q != %q", a, b)
	}
	if a == downloadHistoryKeyV85("MEGA", "https://mega.nz/folder/abc#secret/file/other", "clip.mp4", 12345) {
		t.Fatal("different MEGA node must produce a different history key")
	}
	if a == downloadHistoryKeyV85("MEGA", "https://mega.nz/folder/abc#secret/file/node", "clip.mp4", 12346) {
		t.Fatal("different remote size must produce a different history key")
	}

	pageA := downloadHistoryKeyV85("HTTP", "https://example.test/gallery/42", "first.jpg", 5000)
	pageB := downloadHistoryKeyV85("HTTP", "https://example.test/gallery/42", "second.jpg", 5000)
	if pageA == pageB {
		t.Fatal("same-page same-size files with different names must not collide")
	}
	if len(a) != 64 || len(pageA) != 64 {
		t.Fatal("history keys should be SHA-256 hex")
	}
}

func TestYtDlpHistoryIdentityIgnoresExpiringDirectURLAndTitle(t *testing.T) {
	a := Result{Remote: RemoteItem{
		Source:     "YT-DLP",
		URL:        "https://video.example/watch?v=abc",
		DirectURL:  "https://cdn.example/videoplayback?expire=1&sig=old",
		ProviderID: "abc",
		Extractor:  "ExampleVideo",
		Name:       "Old title.mp4",
		Size:       123456,
	}}
	b := a
	b.Remote.DirectURL = "https://other-cdn.example/videoplayback?expire=2&sig=new"
	b.Remote.Name = "Edited title.mp4"
	identityA := historyRemoteURLV85(a)
	identityB := historyRemoteURLV85(b)
	if identityA != identityB {
		t.Fatalf("stable source identity changed with CDN/title: %q != %q", identityA, identityB)
	}
	keyA := downloadHistoryKeyV85(a.Remote.Source, identityA, a.Remote.Name, a.Remote.Size)
	keyB := downloadHistoryKeyV85(b.Remote.Source, identityB, b.Remote.Name, b.Remote.Size)
	if keyA != keyB {
		t.Fatal("yt-dlp history key should survive direct URL expiry and title edits")
	}
}

func TestStableYtDlpSourceHistoryReviewsDifferentQuality(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old-1080p.mp4")
	if err := os.WriteFile(path, []byte("previous download"), 0600); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	quick, err := quickFileFingerprintV85(path, st.Size())
	if err != nil {
		t.Fatal(err)
	}
	res := Result{ID: 44, Remote: RemoteItem{
		Source:     "YT-DLP",
		URL:        "https://video.example/watch?v=abc",
		ProviderID: "abc",
		Extractor:  "ExampleVideo",
		Name:       "same-video-4k.webm",
		Size:       st.Size() * 4,
	}}
	row := downloadHistoryEntryV85{
		Source:     "YT-DLP",
		Identity:   stableHistoryIdentityV85(res),
		Name:       "same-video-1080p.mp4",
		Bytes:      st.Size(),
		OutputPath: path,
		FinishedAt: time.Now().Add(-time.Hour).Unix(),
		FileSize:   st.Size(),
		FileMTime:  st.ModTime().UnixNano(),
		QuickHash:  quick,
	}
	got, ok := stableSourceHistoryDecisionV85(res, row)
	if !ok {
		t.Fatal("same yt-dlp provider identity should surface prior-download evidence")
	}
	if got.Verdict != guardReview || got.Method != "download-history-source" || got.UserStatus != userDownloaded || got.Action != actionReview {
		t.Fatalf("different quality must be review-only, not auto-blocked: %#v", got)
	}
}

func TestStableYtDlpSourceHistoryRejectsDifferentProviderOrChangedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.mp4")
	if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	quick, err := quickFileFingerprintV85(path, st.Size())
	if err != nil {
		t.Fatal(err)
	}
	original := Result{Remote: RemoteItem{Source: "YT-DLP", URL: "https://video.example/watch?v=abc", ProviderID: "abc", Extractor: "ExampleVideo", Name: "clip.mp4", Size: 999}}
	row := downloadHistoryEntryV85{Source: "YT-DLP", Identity: stableHistoryIdentityV85(original), OutputPath: path, FileSize: st.Size(), FileMTime: st.ModTime().UnixNano(), QuickHash: quick}

	other := original
	other.Remote.ProviderID = "other"
	if _, ok := stableSourceHistoryDecisionV85(other, row); ok {
		t.Fatal("different provider ID must not reuse source history")
	}

	if err := os.WriteFile(path, []byte("modified"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, ok := stableSourceHistoryDecisionV85(original, row); ok {
		t.Fatal("changed historical output must invalidate source-history evidence")
	}
}

func TestQuickFileFingerprintV85DetectsSameSizeReplacement(t *testing.T) {
	path := filepath.Join(t.TempDir(), "same-size.bin")
	if err := os.WriteFile(path, []byte("abcdefgh"), 0600); err != nil {
		t.Fatal(err)
	}
	first, err := quickFileFingerprintV85(path, 8)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("abcdEFGH"), 0600); err != nil {
		t.Fatal(err)
	}
	second, err := quickFileFingerprintV85(path, 8)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("same-size content replacement must change the history fingerprint")
	}
}
