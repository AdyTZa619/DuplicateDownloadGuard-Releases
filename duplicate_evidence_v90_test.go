package main

import "testing"

func TestDetectorNeverTurnsPerceptualScoreIntoExactV90(t *testing.T) {
	d := decorateGuardDecision(DownloadGuardDecision{Verdict: guardReview, Similarity: 100, Method: "media-same-content"})
	if d.Detector.Classification == "EXACT" || d.Detector.Score == nil || *d.Detector.Score != 99 || d.Exact {
		t.Fatalf("invalid perceptual evidence: %#v", d.Detector)
	}
	d = decorateGuardDecision(DownloadGuardDecision{Verdict: guardDuplicate, Exact: true, Method: "full-sha256"})
	if d.Detector.Classification != "EXACT" || *d.Detector.Score != 100 {
		t.Fatal("exact evidence lost")
	}
	d = decorateGuardDecision(DownloadGuardDecision{Verdict: guardReview, Method: "media-unverified"})
	if d.Detector.Score != nil || d.Detector.Classification != "NECUNOSCUT / DATE INSUFICIENTE" {
		t.Fatal("invented score for missing evidence")
	}
}

func TestIncompleteDetectorPreservesMeasuredScoreV90(t *testing.T) {
	d, _ := incompleteMediaDecisionV90(Result{ID: 1}, 72, 10, "media-index-incomplete", "remaining candidates", 12, "local.mp4")
	if d.Detector.Classification != "NECUNOSCUT / DATE INSUFICIENTE" || d.Detector.Score == nil || *d.Detector.Score != 72 || d.Detector.Pending != 10 {
		t.Fatalf("lost incomplete evidence: %#v", d.Detector)
	}
	d = decorateDetectorEvidenceV90(DownloadGuardDecision{Verdict: guardReview, Similarity: 72})
	if d.Detector.Classification != "SIMILAR" {
		t.Fatal("discarded measured similarity")
	}
}
