package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRecordDownloadedContentGraphV860(t *testing.T) {
	dir := t.TempDir()
	a := &App{appDir: dir}
	local := filepath.Join(dir, "movie.mp4")
	if err := os.WriteFile(local, []byte("movie-bytes"), 0644); err != nil {
		t.Fatal(err)
	}
	remote := RemoteItem{Source: "MEGA", Handle: "abc123", Name: "remote.mp4", Path: "folder/remote.mp4", Size: 11, HashType: "sha256", Hash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
	if err := recordDownloadedContentGraphV860(a, remote, local); err != nil {
		t.Fatal(err)
	}
	g := loadContentGraphV860(a)
	if g.Schema != contentGraphSchemaV860 || len(g.Nodes) != 2 {
		t.Fatalf("graph=%#v", g)
	}
	if len(g.Edges) != 2 {
		t.Fatalf("edges=%#v", g.Edges)
	}
	stats := contentGraphStatsV860(a)
	if stats["nodes"] != 2 || stats["edges"] != 2 {
		t.Fatalf("stats=%#v", stats)
	}
}

func TestContentGraphRemoteIdentitySurvivesRenameV860(t *testing.T) {
	a := RemoteItem{Source: "MEGA", Handle: "same-handle", Name: "old.mp4", Path: "old.mp4"}
	b := RemoteItem{Source: "MEGA", Handle: "same-handle", Name: "renamed.mp4", Path: "folder/renamed.mp4"}
	na, err := remoteGraphNodeV860(a)
	if err != nil {
		t.Fatal(err)
	}
	nb, err := remoteGraphNodeV860(b)
	if err != nil {
		t.Fatal(err)
	}
	if na.ID != nb.ID || na.Identity != nb.Identity {
		t.Fatalf("identity changed after rename: %#v %#v", na, nb)
	}
}

func TestContentGraphRejectsUnstableRemoteV860(t *testing.T) {
	_, err := remoteGraphNodeV860(RemoteItem{})
	if err == nil {
		t.Fatal("expected unstable remote identity error")
	}
}
