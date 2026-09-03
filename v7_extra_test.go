package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestHeaderHashDigestSHA256(t *testing.T) {
	sum := sha256.Sum256([]byte("abc"))
	h := http.Header{}
	h.Set("Digest", "sha-256="+base64.StdEncoding.EncodeToString(sum[:]))
	typ, got := headerHash(h)
	if typ != "sha256" || got != hex.EncodeToString(sum[:]) {
		t.Fatalf("%s %s", typ, got)
	}
}

func TestProbeHTTPMetaRangeFallback(t *testing.T) {
	data := []byte("hello-world")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		http.ServeContent(w, r, "clip.mp4", fixedTime, bytes.NewReader(data))
	}))
	defer srv.Close()
	it, err := probeHTTPMeta(srv.URL + "/clip.mp4")
	if err != nil {
		t.Fatal(err)
	}
	if it.Size != int64(len(data)) || !it.AcceptRanges {
		t.Fatalf("bad meta %#v", it)
	}
}

func TestInternalDownloadResume(t *testing.T) {
	data := bytes.Repeat([]byte("abcdef"), 20000)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "x.bin", fixedTime, bytes.NewReader(data))
	}))
	defer srv.Close()
	dir := t.TempDir()
	part := filepath.Join(dir, "x.bin.part")
	if err := os.WriteFile(part, data[:12345], 0644); err != nil {
		t.Fatal(err)
	}
	p, err := internalDownload(context.Background(), srv.URL, dir, "x.bin", func(int64, int64) {})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(p)
	if !bytes.Equal(got, data) {
		t.Fatalf("download mismatch %d/%d", len(got), len(data))
	}
}

func TestImageDHashStableForSamePixels(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 90, 80))
	for y := 0; y < 80; y++ {
		for x := 0; x < 90; x++ {
			img.Set(x, y, color.RGBA{uint8(x * 2), uint8(y * 3), uint8((x + y) % 255), 255})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	decoded, _, err := image.Decode(bytes.NewReader(b.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if dhashImage(img) != dhashImage(decoded) {
		t.Fatal("dHash changed for same pixels")
	}
}

func TestResultDownloadURLMegaNode(t *testing.T) {
	r := Result{Remote: RemoteItem{Source: "MEGA", URL: "https://mega.nz/folder/abc#key", Handle: "NODE1234"}}
	got := resultDownloadURL(r)
	if got != "https://mega.nz/folder/abc#key/file/NODE1234" {
		t.Fatalf("%q", got)
	}
}

func TestFindDownloadedMegaFileRequiresUnambiguousOutput(t *testing.T) {
	dir := t.TempDir()
	started := time.Now()
	res := Result{Remote: RemoteItem{Name: "remote?.jpg", Size: 4}}
	one := filepath.Join(dir, "remote_.jpg")
	if err := os.WriteFile(one, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}
	if got := findDownloadedMegaFile(dir, res, started); got != one {
		t.Fatalf("sanitized result=%q, want %q", got, one)
	}

	res.Remote.Name = "different.jpg"
	two := filepath.Join(dir, "another.jpg")
	if err := os.WriteFile(two, []byte("more"), 0644); err != nil {
		t.Fatal(err)
	}
	if got := findDownloadedMegaFile(dir, res, started); got != "" {
		t.Fatalf("ambiguous same-size outputs must not be guessed: %q", got)
	}
}

var fixedTime = time.Unix(0, 0)
