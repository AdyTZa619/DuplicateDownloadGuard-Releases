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
