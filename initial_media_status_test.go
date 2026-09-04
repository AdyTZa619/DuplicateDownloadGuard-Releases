package main

import "testing"

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
		Status:    "DIFFERENT",
		LocalPath: `D:\docs\file.zip`,
		NameScore: 100,
		Remote:    RemoteItem{Name: "file.zip", Size: 2000, Source: "HTTP"},
	}
	normalizeInitialMediaResultV85(&r)
	if r.Status != "DIFFERENT" {
		t.Fatalf("status=%q, want DIFFERENT", r.Status)
	}
}
