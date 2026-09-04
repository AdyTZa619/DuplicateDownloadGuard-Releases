package main

import "testing"

func TestResolveVideoEvidenceKeepsStrongAudioMatch(t *testing.T) {
	ri := MediaInfo{OK: true, AudioCodec: "aac"}
	li := MediaInfo{OK: true, AudioCodec: "aac"}
	method, _ := resolveVideoEvidenceV85(99, 90, ri, li, audioFingerprintResultV85{Available: true, Score: 91, Note: "audio ok"})
	if method != "media-same-content" {
		t.Fatalf("method=%q, want media-same-content", method)
	}
}

func TestResolveVideoEvidenceDowngradesDifferentAudio(t *testing.T) {
	ri := MediaInfo{OK: true, AudioCodec: "aac"}
	li := MediaInfo{OK: true, AudioCodec: "aac"}
	method, _ := resolveVideoEvidenceV85(99, 90, ri, li, audioFingerprintResultV85{Available: true, Score: 55, Note: "audio different"})
	if method != "media-version" {
		t.Fatalf("method=%q, want media-version", method)
	}
}

func TestResolveVideoEvidenceDoesNotAutoblockWhenAudioCannotBeChecked(t *testing.T) {
	ri := MediaInfo{OK: true, AudioCodec: "aac"}
	li := MediaInfo{OK: true, AudioCodec: "aac"}
	method, _ := resolveVideoEvidenceV85(99, 90, ri, li, audioFingerprintResultV85{})
	if method != "media-looks-same" {
		t.Fatalf("method=%q, want media-looks-same", method)
	}
}

func TestResolveVideoEvidenceDetectsMissingAudioTrack(t *testing.T) {
	ri := MediaInfo{OK: true, AudioCodec: "aac"}
	li := MediaInfo{OK: true}
	method, _ := resolveVideoEvidenceV85(100, 90, ri, li, audioFingerprintResultV85{Available: true, Score: 0})
	if method != "media-version" {
		t.Fatalf("method=%q, want media-version", method)
	}
}

func TestResolveVideoEvidenceDowngradesAmbiguousRunnerUp(t *testing.T) {
	ri := MediaInfo{OK: true}
	li := MediaInfo{OK: true}
	method, _ := resolveVideoEvidenceV85(98, 98, ri, li, audioFingerprintResultV85{})
	if method != "media-looks-same" {
		t.Fatalf("method=%q, want media-looks-same", method)
	}
}

func TestResolveVideoEvidenceNeverPromotesWeakVisualMatch(t *testing.T) {
	ri := MediaInfo{OK: true, AudioCodec: "aac"}
	li := MediaInfo{OK: true, AudioCodec: "aac"}
	method, _ := resolveVideoEvidenceV85(90, 20, ri, li, audioFingerprintResultV85{Available: true, Score: 100, Note: "same audio"})
	if method != "media-looks-same" {
		t.Fatalf("method=%q, want media-looks-same", method)
	}
}

func TestResolveVideoEvidenceSilentVideoNeedsPerfectVisualForAutoblock(t *testing.T) {
	ri := MediaInfo{OK: true, Duration: 120}
	li := MediaInfo{OK: true, Duration: 120}
	method, _ := resolveVideoEvidenceV85(99, 80, ri, li, audioFingerprintResultV85{})
	if method != "media-version" {
		t.Fatalf("silent 99%% match must remain review-only, got %q", method)
	}
	method, _ = resolveVideoEvidenceV85(100, 80, ri, li, audioFingerprintResultV85{})
	if method != "media-same-content" {
		t.Fatalf("silent unambiguous 100%% match should remain same-content, got %q", method)
	}
}

func TestResolveVideoEvidenceDurationDifferenceBecomesVersion(t *testing.T) {
	ri := MediaInfo{OK: true, Duration: 600, AudioCodec: "aac"}
	li := MediaInfo{OK: true, Duration: 612, AudioCodec: "aac"}
	method, note := resolveVideoEvidenceV85(100, 80, ri, li, audioFingerprintResultV85{Available: true, Score: 96, Note: "audio same"})
	if method != "media-version" {
		t.Fatalf("meaningful duration delta must remain a version, got %q", method)
	}
	if note == "" {
		t.Fatal("duration downgrade should explain why")
	}
}

func TestMeaningfulVideoDurationDeltaIgnoresTinyContainerDrift(t *testing.T) {
	ri := MediaInfo{OK: true, Duration: 3600}
	li := MediaInfo{OK: true, Duration: 3601.2}
	if _, _, meaningful := meaningfulVideoDurationDeltaV85(ri, li); meaningful {
		t.Fatal("small timestamp/container drift must not create a version")
	}
}
