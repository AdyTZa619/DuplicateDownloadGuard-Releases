package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseYtDlpProgressLineV860(t *testing.T) {
	p, ok := parseYtDlpProgressLineV860("DDG_PROGRESS|1048576|2097152|NA|524288|2")
	if !ok || p.Done != 1048576 || p.Total != 2097152 || p.Speed != 524288 || p.ETA != 2 {
		t.Fatalf("progress=%#v ok=%v", p, ok)
	}
	p, ok = parseYtDlpProgressLineV860("DDG_PROGRESS|100|NA|1000|50.7|18")
	if !ok || p.Total != 1000 || p.Speed != 50 {
		t.Fatalf("estimated progress=%#v", p)
	}
	if _, ok := parseYtDlpProgressLineV860("[download] 10%"); ok {
		t.Fatal("ordinary yt-dlp line must not be parsed as structured progress")
	}
}

func TestObservedDownloadBytesV860(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "movie.mp4")
	if err := os.WriteFile(old, make([]byte, 100), 0644); err != nil {
		t.Fatal(err)
	}
	baseline := snapshotDirectoryProgressV860(dir)
	started := time.Now()
	if err := os.WriteFile(old, make([]byte, 350), 0644); err != nil {
		t.Fatal(err)
	}
	if got := observedDownloadBytesV860(dir, "movie.mp4", baseline, started); got != 250 {
		t.Fatalf("growth=%d", got)
	}
	part := filepath.Join(dir, "movie.mp4.mega")
	if err := os.WriteFile(part, make([]byte, 700), 0644); err != nil {
		t.Fatal(err)
	}
	if got := observedDownloadBytesV860(dir, "movie.mp4", baseline, started); got != 700 {
		t.Fatalf("new temp progress=%d", got)
	}
}

func TestSnapshotDirectoryProgressV860(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.part")
	if err := os.WriteFile(p, []byte("12345"), 0644); err != nil {
		t.Fatal(err)
	}
	s := snapshotDirectoryProgressV860(dir)
	if s[p] != 5 {
		t.Fatalf("snapshot=%#v", s)
	}
}
