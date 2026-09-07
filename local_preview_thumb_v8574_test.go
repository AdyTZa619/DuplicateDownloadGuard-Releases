package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeFileInfoV8574 struct {
	name string
	size int64
	when time.Time
}

func (f fakeFileInfoV8574) Name() string       { return f.name }
func (f fakeFileInfoV8574) Size() int64        { return f.size }
func (f fakeFileInfoV8574) Mode() os.FileMode  { return 0 }
func (f fakeFileInfoV8574) ModTime() time.Time { return f.when }
func (f fakeFileInfoV8574) IsDir() bool        { return false }
func (f fakeFileInfoV8574) Sys() any           { return nil }

func TestLocalPreviewKindV8574(t *testing.T) {
	cases := map[string]string{
		"a.jpg":  "image",
		"a.webp": "image",
		"a.heic": "image",
		"a.mp4":  "video",
		"a.mkv":  "video",
		"a.mp3":  "other",
	}
	for name, want := range cases {
		if got := localPreviewKindV8574(name); got != want {
			t.Fatalf("kind %s: got %s want %s", name, got, want)
		}
	}
}

func TestLocalThumbKeyV8574ChangesWithCurrentEvidence(t *testing.T) {
	when := time.Unix(1700000000, 0)
	one := fakeFileInfoV8574{name: "x.jpg", size: 100, when: when}
	two := fakeFileInfoV8574{name: "x.jpg", size: 101, when: when}
	three := fakeFileInfoV8574{name: "x.jpg", size: 100, when: when.Add(time.Second)}
	k1 := localThumbKeyV8574(filepath.Join("H:\\media", "x.jpg"), one)
	if len(k1) != 64 || strings.Trim(k1, "0123456789abcdef") != "" {
		t.Fatalf("unexpected key format: %q", k1)
	}
	if k1 == localThumbKeyV8574(filepath.Join("H:\\media", "x.jpg"), two) {
		t.Fatal("cache key must change when file size changes")
	}
	if k1 == localThumbKeyV8574(filepath.Join("H:\\media", "x.jpg"), three) {
		t.Fatal("cache key must change when mtime changes")
	}
}

func TestLocalThumbCacheIsBounded(t *testing.T) {
	if localThumbMaxBytesV8574 <= 0 || localThumbMaxBytesV8574 > 512<<20 {
		t.Fatalf("unexpected cache cap: %d", localThumbMaxBytesV8574)
	}
}
