package main

// TEST90 validates the browser-safe image fallback independently of an FFmpeg
// installation, while retaining the existing cache-key and cache-bound checks.

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
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

func TestLocalThumbNativeImageV8575WorksWithoutFFmpeg(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sample.png")
	src := image.NewRGBA(image.Rect(0, 0, 24, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 24; x++ {
			src.Set(x, y, color.RGBA{R: uint8(x * 7), G: uint8(y * 11), B: 120, A: 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, src); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	data, err := localThumbNativeImageV8575(path)
	if err != nil {
		t.Fatalf("native image fallback failed: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("native image fallback returned empty JPEG")
	}
	decoded, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("native image fallback emitted invalid image: %v", err)
	}
	if format != "jpeg" {
		t.Fatalf("native image fallback format = %q, want jpeg", format)
	}
	if decoded.Bounds().Dx() <= 0 || decoded.Bounds().Dy() <= 0 {
		t.Fatal("native image fallback emitted invalid dimensions")
	}
}

func TestLocalThumbCacheIsBounded(t *testing.T) {
	if localThumbMaxBytesV8574 <= 0 || localThumbMaxBytesV8574 > 512<<20 {
		t.Fatalf("unexpected cache cap: %d", localThumbMaxBytesV8574)
	}
}
