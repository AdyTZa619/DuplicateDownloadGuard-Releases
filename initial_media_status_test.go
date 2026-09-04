package main

import (
	"strings"
	"testing"
)

func TestNormalizeInitialMediaResultDowngradesUnprovenDifferent(t *testing.T) {
	r := Result{
		Status:    "DIFFERENT",
		LocalPath: `D:\media\clip.mp4`,
		NameScore: 100,
		Remote:    RemoteItem{Name: "clip.mp4", Size: 2000, Source: "MEGA"},
	}
	normalizeInitialMediaResultV85(&r)
	if r.Status != "POSSIBLE" {
		t.Fatalf("status=%q, want POSSIBLE", r.Status)
	}
}

func TestNormalizeInitialMediaResultSameSizeRenamedImageIsNotMissing(t *testing.T) {
	r := Result{
		Status:     "MISSING",
		Candidates: 1,
		Remote: RemoteItem{
			Name:   "3756x6654_035cc4db82a2493862302a02a13f9024.jpg",
			Size:   5_600_000,
			Source: "MEGA",
		},
	}
	normalizeInitialMediaResultV85(&r)
	if r.Status != "POSSIBLE" {
		t.Fatalf("status=%q, want POSSIBLE", r.Status)
	}
	if !strings.Contains(r.Reason, "exact aceeași dimensiune") {
		t.Fatalf("reason=%q, expected same-size explanation", r.Reason)
	}
}

func TestNormalizeInitialMediaResultSameSizeRenamedVideoIsNotMissing(t *testing.T) {
	r := Result{
		Status:     "MISSING",
		Candidates: 3,
		Remote:    RemoteItem{Name: "remote-source.mp4", Size: 25_000_000, Source: "HTTP"},
	}
	normalizeInitialMediaResultV85(&r)
	if r.Status != "POSSIBLE" {
		t.Fatalf("status=%q, want POSSIBLE", r.Status)
	}
}

func TestNormalizeInitialMediaResultNoSameSizeCandidateStaysMissing(t *testing.T) {
	r := Result{
		Status:     "MISSING",
		Candidates: 0,
		Remote:    RemoteItem{Name: "photo.jpg", Size: 2000, Source: "MEGA"},
	}
	normalizeInitialMediaResultV85(&r)
	if r.Status != "MISSING" {
		t.Fatalf("status=%q, want MISSING", r.Status)
	}
}

func TestNormalizeInitialMediaResultKeepsHashProvenDifferent(t *testing.T) {
	r := Result{
		Status:    "DIFFERENT",
		LocalPath: `D:\media\clip.mp4`,
		NameScore: 100,
		Remote:    RemoteItem{Name: "clip.mp4", Size: 2000, Source: "HTTP", HashType: "sha256", Hash: "abc"},
	}
	normalizeInitialMediaResultV85(&r)
	if r.Status != "DIFFERENT" {
		t.Fatalf("status=%q, want DIFFERENT", r.Status)
	}
}

func TestNormalizeInitialMediaResultDoesNotChangeDocuments(t *testing.T) {
	r := Result{
		Status:     "MISSING",
		Candidates: 2,
		Remote:    RemoteItem{Name: "file.zip", Size: 2000, Source: "HTTP"},
	}
	normalizeInitialMediaResultV85(&r)
	if r.Status != "MISSING" {
		t.Fatalf("status=%q, want MISSING", r.Status)
	}
}
