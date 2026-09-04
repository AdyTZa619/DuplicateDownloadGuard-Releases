package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestShouldReconcileDownloadedRemoteV860StableSource(t *testing.T) {
	a := RemoteItem{Source: "MEGA", Handle: "h1", Name: "old.mp4"}
	b := RemoteItem{Source: "MEGA", Handle: "h1", Name: "renamed.mp4"}
	ok, evidence := shouldReconcileDownloadedRemoteV860(a, b)
	if !ok || evidence != "same-source" {
		t.Fatalf("ok=%v evidence=%q", ok, evidence)
	}
}

func TestShouldReconcileDownloadedRemoteV860ExactHash(t *testing.T) {
	h := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	a := RemoteItem{Source: "HTTP", URL: "https://a/x", Name: "a.mp4", Size: 123, HashType: "sha256", Hash: h}
	b := RemoteItem{Source: "HTTP", URL: "https://b/y", Name: "b.mp4", Size: 123, HashType: "sha256", Hash: h}
	ok, evidence := shouldReconcileDownloadedRemoteV860(a, b)
	if !ok || evidence != "exact-hash" {
		t.Fatalf("ok=%v evidence=%q", ok, evidence)
	}
	b.Size = 124
	if ok, _ := shouldReconcileDownloadedRemoteV860(a, b); ok {
		t.Fatal("different known sizes must not reconcile even with same remote hash")
	}
}

func TestPostDownloadReconcileV860DoesNotTouchUnrelated(t *testing.T) {
	dir := t.TempDir()
	local := filepath.Join(dir, "downloaded.mp4")
	if err := os.WriteFile(local, []byte("downloaded"), 0644); err != nil {
		t.Fatal(err)
	}
	a := &App{
		appDir:    dir,
		index:     map[string]FileEntry{},
		bySize:    map[int64][]string{},
		byName:    map[string][]string{},
		decisions: map[string]Decision{},
		results: []Result{
			{ID: 1, Status: "MISSING", Remote: RemoteItem{Source: "MEGA", Handle: "same", Name: "one.mp4"}},
			{ID: 2, Status: "MISSING", Remote: RemoteItem{Source: "MEGA", Handle: "same", Name: "renamed.mp4"}},
			{ID: 3, Status: "MISSING", Remote: RemoteItem{Source: "MEGA", Handle: "other", Name: "other.mp4"}},
		},
	}
	changed := a.postDownloadReconcileV860(a.results[0], local)
	if changed != 2 {
		t.Fatalf("changed=%d results=%#v", changed, a.results)
	}
	if a.results[0].Status != "HAVE" || a.results[1].Status != "HAVE" {
		t.Fatalf("related rows not reconciled: %#v", a.results)
	}
	if a.results[2].Status != "MISSING" || a.results[2].LocalPath != "" {
		t.Fatalf("unrelated row changed: %#v", a.results[2])
	}
	if _, err := os.Stat(contentGraphPathV860(a)); err != nil {
		t.Fatalf("content graph not persisted: %v", err)
	}
}

func TestApplyDownloadedResultFieldsPreservesManualDecisionV860(t *testing.T) {
	r := Result{Manual: true, ManualStatus: "DIFFERENT", ManualAt: 123}
	applyDownloadedResultFieldsV860(&r, `C:\x.mp4`, "same-source", 999)
	if !r.Manual || r.ManualStatus != "DIFFERENT" || r.ManualAt != 123 {
		t.Fatalf("manual decision was overwritten: %#v", r)
	}
	if r.Status != "HAVE" || r.GuardMethod != "post-download-reconcile" {
		t.Fatalf("download evidence not applied: %#v", r)
	}
}
