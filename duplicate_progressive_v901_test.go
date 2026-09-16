package main

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func patternedImageV901(seed, width, height int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.SetRGBA(x, y, color.RGBA{uint8((x*3 + y + seed*19) % 256), uint8((x + y*5 + seed*31) % 256), uint8((x*7 + y*2 + seed*11) % 256), 255})
		}
	}
	return img
}

func TestProgressiveImageFindsRenamedWinnerOutsideFirstShortlistV901(t *testing.T) {
	collection := t.TempDir()
	for i := 0; i < 130; i++ {
		path := filepath.Join(collection, "likely", "remote-name-candidate-"+fmtIntV901(i)+".jpg")
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		if err = jpeg.Encode(f, patternedImageV901(i+100, 96, 54), &jpeg.Options{Quality: 82}); err != nil {
			f.Close()
			t.Fatal(err)
		}
		f.Close()
	}
	remoteImage := patternedImageV901(7, 160, 90)
	localWinner := filepath.Join(collection, "unrelated-folder", "totally-different-name.jpg")
	if err := os.MkdirAll(filepath.Dir(localWinner), 0700); err != nil {
		t.Fatal(err)
	}
	winner, _ := os.Create(localWinner)
	if err := jpeg.Encode(winner, remoteImage, &jpeg.Options{Quality: 68}); err != nil {
		winner.Close()
		t.Fatal(err)
	}
	winner.Close()
	var remoteBytes bytes.Buffer
	if err := png.Encode(&remoteBytes, remoteImage); err != nil {
		t.Fatal(err)
	}
	server := contentServer(remoteBytes.Bytes())
	defer server.Close()
	a := guardTestApp(t, collection, t.TempDir(), RemoteItem{})
	a.runIndex(context.Background(), []string{collection}, "", 0)
	entries := make([]FileEntry, 0, len(a.index))
	for _, e := range a.index {
		entries = append(entries, e)
	}
	ctx := detectorCandidateContextV90(context.Background(), a, entries)
	result := Result{ID: 1, Remote: RemoteItem{Name: "remote-name.png", Size: int64(remoteBytes.Len()), Source: "HTTP", DirectURL: server.URL}}
	d, ok := a.mediaNearDuplicateDecision(ctx, result, entries, true)
	if !ok || d.LocalPath != localWinner || d.Detector == nil || d.Detector.Pending != 0 || d.Similarity < 94 {
		t.Fatalf("winner outside shortlist was not found: %#v", d)
	}
}

func TestExtensionlessRemoteImageUsesProviderContentTypeV901(t *testing.T) {
	collection := t.TempDir()
	img := patternedImageV901(17, 160, 90)
	local := filepath.Join(collection, "unrelated-local-name.jpg")
	f, err := os.Create(local)
	if err != nil {
		t.Fatal(err)
	}
	if err = jpeg.Encode(f, img, &jpeg.Options{Quality: 72}); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()
	var remoteBytes bytes.Buffer
	if err = png.Encode(&remoteBytes, img); err != nil {
		t.Fatal(err)
	}
	server := contentServer(remoteBytes.Bytes())
	defer server.Close()
	a := guardTestApp(t, collection, t.TempDir(), RemoteItem{})
	a.runIndex(context.Background(), []string{collection}, "", 0)
	entries := make([]FileEntry, 0, len(a.index))
	for _, entry := range a.index {
		entries = append(entries, entry)
	}
	result := Result{ID: 1, Remote: RemoteItem{Name: "opaque-provider-id", ContentType: "image/png", Size: int64(remoteBytes.Len()), Source: "BUNKR", DirectURL: server.URL}}
	d, ok := a.mediaNearDuplicateDecision(detectorCandidateContextV90(context.Background(), a, entries), result, entries, true)
	if !ok || d.LocalPath != local || d.Verdict != guardDuplicate {
		t.Fatalf("content type did not activate image detector: %#v", d)
	}
}

func TestBunkrUsesSameAutomaticDetectorAsOtherSourcesV901(t *testing.T) {
	data := bytes.Repeat([]byte("shared-engine"), 1000)
	a, _, local := sourceDetectorFixtureV90(t, data)
	mux := httpNewServeMuxV901(a)
	server := contentServer(data)
	defer server.Close()
	a.compareRemote(context.Background(), []RemoteItem{{Name: "renamed-bunkr.bin", Size: int64(len(data)), Source: "BUNKR", DirectURL: server.URL, URL: "https://bunkr.example/a/album", Handle: "remote-id"}}, "balanced")
	waitSourceDetectorV90(t, a)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/api/duplicates/status", nil))
	row, _ := a.resultByID(1)
	if row.GuardVerdict != guardDuplicate || row.LocalPath != local || row.Detector == nil || row.Detector.Classification != "IDENTIC" {
		t.Fatalf("Bunkr did not use common detector: %#v status=%s", row, w.Body.String())
	}
}

func httpNewServeMuxV901(a *App) *http.ServeMux {
	mux := http.NewServeMux()
	a.registerDuplicateRoutesV90(mux)
	return mux
}

func fmtIntV901(value int) string {
	return fmt.Sprintf("%03d", value)
}

func TestAdaptiveFrameScheduleV901(t *testing.T) {
	points, first := adaptiveFramePointsV901()
	if len(points) != 25 || len(first) != 15 {
		t.Fatalf("adaptive schedule is %d/%d, want 15 then 25", len(first), len(points))
	}
	deadline, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	cancel()
	if got := collectAdaptiveFramesV901(deadline, "missing", "missing", 10, points, first, nil); len(got) != 0 {
		t.Fatal("cancelled adaptive stage produced evidence")
	}
}

func TestAdaptivePairCacheInvalidatesChangedMovedAndDeletedLocalV901(t *testing.T) {
	a := &App{appDir: t.TempDir()}
	local := filepath.Join(t.TempDir(), "same-name.mp4")
	if err := os.WriteFile(local, []byte("first-content"), 0600); err != nil {
		t.Fatal(err)
	}
	// Windows deliberately refuses to cache files changed in the last two
	// seconds because FAT-compatible timestamp resolution can hide rewrites.
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, _, _, ok := adaptiveLocalIdentityV901(local); ok {
			break
		}
		if time.Now().After(deadline) {
			t.Skip("filesystem identity is not safely cacheable on this platform")
		}
		time.Sleep(50 * time.Millisecond)
	}
	ctx := context.WithValue(context.Background(), detectorRemoteKeyV90{}, &detectorRemoteV90{
		Original: "https://remote.example/file.mp4", Validator: `"remote-v1"`, Size: 12345,
	})
	want := adaptiveVideoEvidenceV901{Score: 96, Matched: 15, Total: 15}
	saveAdaptiveVideoEvidenceV901(a, ctx, local, want)
	if got, ok := cachedAdaptiveVideoEvidenceV901(a, ctx, local); !ok || got.Score != want.Score {
		t.Fatalf("saved adaptive pair was not reusable: %#v ok=%v", got, ok)
	}

	before, err := os.Stat(local)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(local, []byte("other-content"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Chtimes(local, before.ModTime(), before.ModTime()); err != nil {
		t.Fatal(err)
	}
	if _, ok := cachedAdaptiveVideoEvidenceV901(a, ctx, local); ok {
		t.Fatal("adaptive pair cache reused a replaced same-name/same-size local file")
	}
	moved := local + ".moved"
	if err = os.Rename(local, moved); err != nil {
		t.Fatal(err)
	}
	if _, ok := cachedAdaptiveVideoEvidenceV901(a, ctx, local); ok {
		t.Fatal("adaptive pair cache reused a moved local file at its old path")
	}
	if err = os.Remove(moved); err != nil {
		t.Fatal(err)
	}
	if _, ok := cachedAdaptiveVideoEvidenceV901(a, ctx, moved); ok {
		t.Fatal("adaptive pair cache reused a deleted local file")
	}
}
