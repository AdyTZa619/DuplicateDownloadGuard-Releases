package main

import (
	"context"
	"path/filepath"
	"testing"
)

func testResultTruthAppV85130(t *testing.T, entries []FileEntry) *App {
	t.Helper()
	a := &App{
		appDir:     t.TempDir(),
		index:      map[string]FileEntry{},
		bySize:     map[int64][]string{},
		byName:     map[string][]string{},
		byIdentity: map[string][]string{},
		decisions:  map[string]Decision{},
		cfg:        Config{Mode: "balanced"},
	}
	for _, entry := range entries {
		a.index[entry.Path] = entry
	}
	a.rebuildMaps()
	return a
}

func TestMegaExampleManifestExactFilesAreAllLocalV85130(t *testing.T) {
	// Representative entries from the user's 149-file MEGA folder
	// dIBTRCIT#nh95yntp5ABeMBcS83O0eg (145 JPG + 4 MP4).
	manifest := []RemoteItem{
		{Path: "pics/2591736107.jpg", Name: "2591736107.jpg", Size: 125966, Source: "MEGA", Handle: "gQ52DISD"},
		{Path: "pics/2429792353.jpg", Name: "2429792353.jpg", Size: 1839055, Source: "MEGA", Handle: "RNwGlQgD"},
		{Path: "pics/avatar.jpg", Name: "avatar.jpg", Size: 2512012, Source: "MEGA", Handle: "YBo0DIaT"},
		{Path: "vids/2426082582.mp4", Name: "2426082582.mp4", Size: 5821982, Source: "MEGA", Handle: "FNoAACoJ"},
		{Path: "vids/2593083284.mp4", Name: "2593083284.mp4", Size: 14193585, Source: "MEGA", Handle: "dFoW3YAZ"},
	}
	entries := make([]FileEntry, 0, len(manifest))
	for _, remote := range manifest {
		path := filepath.Join(t.TempDir(), remote.Name)
		entries = append(entries, FileEntry{Path: path, Name: remote.Name, Size: remote.Size})
	}
	a := testResultTruthAppV85130(t, entries)
	a.compareRemote(context.Background(), manifest, "balanced")
	for _, result := range a.results {
		if result.Status != "HAVE" || result.LocalPath == "" {
			t.Fatalf("exact local file %q classified as %#v", result.Remote.Path, result)
		}
	}
}

func TestSameByteSizeRenamedFileIsReviewNotMissingV85130(t *testing.T) {
	entry := FileEntry{Path: filepath.Join(t.TempDir(), "renamed-completely.jpg"), Name: "renamed-completely.jpg", Size: 125966}
	a := testResultTruthAppV85130(t, []FileEntry{entry})
	remote := RemoteItem{Path: "pics/2591736107.jpg", Name: "2591736107.jpg", Size: 125966, Source: "MEGA", Handle: "gQ52DISD"}
	a.compareRemote(context.Background(), []RemoteItem{remote}, "balanced")
	got := a.results[0]
	if got.Status != "POSSIBLE" || got.LocalPath != entry.Path || !got.SameSize {
		t.Fatalf("same-byte renamed file must be review, got %#v", got)
	}
}

func TestStableNumericIdentityFindsReencodedLocalVersionV85130(t *testing.T) {
	entry := FileEntry{Path: filepath.Join(t.TempDir(), "twitter_2591736107_saved.jpg"), Name: "twitter_2591736107_saved.jpg", Size: 124000}
	a := testResultTruthAppV85130(t, []FileEntry{entry})
	remote := RemoteItem{Path: "pics/2591736107.jpg", Name: "2591736107.jpg", Size: 125966, Source: "MEGA", Handle: "gQ52DISD"}
	a.compareRemote(context.Background(), []RemoteItem{remote}, "balanced")
	got := a.results[0]
	if got.Status != "POSSIBLE" || got.LocalPath != entry.Path || got.SameSize {
		t.Fatalf("stable-ID media version must be review, got %#v", got)
	}
}

func TestStaleManualMissingCannotOverrideNewExactLocalEvidenceV85130(t *testing.T) {
	remote := RemoteItem{Path: "pics/2591736107.jpg", Name: "2591736107.jpg", Size: 125966, Source: "MEGA", URL: "https://mega.nz/folder/dIBTRCIT#nh95yntp5ABeMBcS83O0eg", Handle: "gQ52DISD"}
	entry := FileEntry{Path: filepath.Join(t.TempDir(), remote.Name), Name: remote.Name, Size: remote.Size}
	a := testResultTruthAppV85130(t, []FileEntry{entry})
	key := decisionKey(remote)
	a.decisions[key] = Decision{Status: "MISSING", UpdatedAt: 1}
	a.compareRemote(context.Background(), []RemoteItem{remote}, "balanced")
	got := a.results[0]
	if got.Status != "HAVE" || got.Manual {
		t.Fatalf("stale MISSING overruled exact current evidence: %#v", got)
	}
	if _, exists := a.decisions[key]; exists {
		t.Fatal("stale MISSING decision was not removed")
	}
}

func TestNoCandidateRemainsMissingV85130(t *testing.T) {
	a := testResultTruthAppV85130(t, nil)
	remote := RemoteItem{Name: "2591736107.jpg", Size: 125966, Source: "MEGA"}
	a.compareRemote(context.Background(), []RemoteItem{remote}, "balanced")
	if got := a.results[0]; got.Status != "MISSING" || got.LocalPath != "" {
		t.Fatalf("absent file should remain missing, got %#v", got)
	}
}
