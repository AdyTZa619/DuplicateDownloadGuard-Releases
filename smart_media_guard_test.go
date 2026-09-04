package main

import "testing"

func TestDecorateGuardDecisionUsesIntuitiveLabels(t *testing.T) {
	tests := []struct {
		in     DownloadGuardDecision
		status string
		action string
	}{
		{DownloadGuardDecision{Verdict: guardDuplicate}, userHaveExact, actionDontDownload},
		{DownloadGuardDecision{Verdict: guardDownload}, userMissing, actionDownload},
		{DownloadGuardDecision{Verdict: guardDuplicate, Method: "download-history"}, userDownloaded, actionDontDownload},
		{DownloadGuardDecision{Verdict: guardReview, Method: "media-same-content"}, userSameContent, actionDontDownload},
		{DownloadGuardDecision{Verdict: guardReview, Method: "media-version", QualityHint: "remote"}, userOtherVersion, actionRemoteBetter},
		{DownloadGuardDecision{Verdict: guardReview, Method: "media-version", QualityHint: "local"}, userOtherVersion, actionLocalBetter},
		{DownloadGuardDecision{Verdict: guardReview, Method: "media-looks-same"}, userLooksSame, actionReview},
		{DownloadGuardDecision{Verdict: guardReview, Method: "metadata-incomplete"}, userUnverified, actionRetry},
	}
	for _, tc := range tests {
		got := decorateGuardDecision(tc.in)
		if got.UserStatus != tc.status || got.Action != tc.action {
			t.Fatalf("decorate(%#v) => status=%q action=%q, want %q / %q", tc.in, got.UserStatus, got.Action, tc.status, tc.action)
		}
	}
}

func TestMediaGuardCandidatesAcceptsReencodedVideoWithDifferentSize(t *testing.T) {
	remote := RemoteItem{Name: "vacanta_2026_final.mp4", Size: 1_000_000_000}
	entries := []FileEntry{
		{Path: `D:\video\vacanta_2026.mp4`, Name: "vacanta_2026.mp4", Size: 620_000_000},
		{Path: `D:\video\facturi_2024.mp4`, Name: "facturi_2024.mp4", Size: 1_000_000_000},
		{Path: `D:\poze\vacanta_2026.jpg`, Name: "vacanta_2026.jpg", Size: 8_000_000},
	}
	got := mediaGuardCandidates(remote, entries, 5)
	if len(got) == 0 {
		t.Fatal("re-encoded video candidate was discarded")
	}
	if got[0].Name != "vacanta_2026.mp4" {
		t.Fatalf("best candidate=%q, want vacanta_2026.mp4", got[0].Name)
	}
}

func TestMediaQualityHintPrefersMaterialResolutionDifference(t *testing.T) {
	remote := MediaInfo{OK: true, Width: 3840, Height: 2160, BitRate: 12_000_000}
	local := MediaInfo{OK: true, Width: 1920, Height: 1080, BitRate: 8_000_000}
	if got := mediaQualityHint(remote, local); got != "remote" {
		t.Fatalf("quality hint=%q, want remote", got)
	}
	if got := mediaQualityHint(local, remote); got != "local" {
		t.Fatalf("reverse quality hint=%q, want local", got)
	}
}
