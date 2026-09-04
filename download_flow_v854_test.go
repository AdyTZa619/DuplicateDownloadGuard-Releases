package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestDownloadPlanAutoUsesNativeHTTPForDirectFiles(t *testing.T) {
	r := RemoteItem{Source: "HTTP", Name: "a.mp4", URL: "https://example.test/a.mp4", DirectURL: "https://example.test/a.mp4"}
	p, err := chooseDownloadPlanCoreV854(r, "auto", downloadCapsV854{Aria2: true})
	if err != nil {
		t.Fatal(err)
	}
	if p.Engine != "internal" {
		t.Fatalf("expected internal for stable Auto direct download, got %q", p.Engine)
	}
}

func TestDownloadPlanManifestRequiresYtDlp(t *testing.T) {
	r := RemoteItem{Source: "YT-DLP", Name: "stream.mp4", URL: "https://example.test/watch/1", DirectURL: "https://cdn.test/master.m3u8", ContentType: "stream/manifest"}
	if _, err := chooseDownloadPlanCoreV854(r, "auto", downloadCapsV854{}); err == nil {
		t.Fatal("expected missing yt-dlp to reject HLS/DASH")
	}
	p, err := chooseDownloadPlanCoreV854(r, "auto", downloadCapsV854{YtDlp: true, Aria2: true})
	if err != nil {
		t.Fatal(err)
	}
	if p.Engine != "yt-dlp" {
		t.Fatalf("expected yt-dlp, got %q", p.Engine)
	}
	if _, err := chooseDownloadPlanCoreV854(r, "aria2", downloadCapsV854{Aria2: true}); err == nil {
		t.Fatal("explicit aria2 must reject HLS/DASH")
	}
}

func TestDownloadPlanMegaNeverFallsThroughHTTP(t *testing.T) {
	r := RemoteItem{Source: "MEGA", Name: "x.mp4", URL: "https://mega.nz/folder/test", Handle: "abc"}
	if _, err := chooseDownloadPlanCoreV854(r, "auto", downloadCapsV854{}); err == nil {
		t.Fatal("expected MEGAcmd requirement")
	}
	p, err := chooseDownloadPlanCoreV854(r, "auto", downloadCapsV854{Mega: true, Aria2: true})
	if err != nil || p.Engine != "mega" {
		t.Fatalf("expected MEGA engine, plan=%+v err=%v", p, err)
	}
}

func TestResultFromDownloadJobUsesPersistentRemoteSnapshot(t *testing.T) {
	job := DownloadJob{ResultID: 42, Remote: RemoteItem{Source: "YT-DLP", Name: "clip.mp4", URL: "https://example.test/watch/42", DirectURL: "https://cdn.test/expiring.mp4", ProviderID: "42"}}
	res, err := resultFromDownloadJobV854(job, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.ID != 42 || res.Remote.URL != "https://example.test/watch/42" || res.Remote.ProviderID != "42" {
		t.Fatalf("snapshot not preserved: %+v", res)
	}
}

func TestLegacyComplexQueueJobFailsInsteadOfDownloadingWrongURL(t *testing.T) {
	job := DownloadJob{ResultID: 7, Source: "YT-DLP", Name: "clip.mp4", URL: "https://cdn.test/temporary.mp4", BytesTotal: 10}
	if _, err := resultFromDownloadJobV854(job, nil); err == nil {
		t.Fatal("legacy yt-dlp job must require re-add instead of guessing source identity")
	}
}

func TestInternalDownloadSendsRefererAndDownloadsMedia(t *testing.T) {
	const page = "/gallery/1"
	payload := []byte("real-media-payload")
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/media/photo.jpg" {
			http.NotFound(w, r)
			return
		}
		if r.Referer() != server.URL+page {
			http.Error(w, "referer required", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Length", "18")
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	dest := t.TempDir()
	remote := RemoteItem{Source: "GALLERY-DL", Name: "photo.jpg", URL: server.URL + page, DirectURL: server.URL + "/media/photo.jpg"}
	path, err := internalDownloadV854(context.Background(), remote, dest, remote.Name, func(int64, int64) {})
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Fatalf("payload mismatch: %q", got)
	}
	if filepath.Base(path) != "photo.jpg" {
		t.Fatalf("unexpected output name: %s", path)
	}
}

func TestInternalDownloadAutoRenamesCollision(t *testing.T) {
	payload := []byte("new")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, string(payload))
	}))
	defer srv.Close()
	dest := t.TempDir()
	if err := os.WriteFile(filepath.Join(dest, "same.bin"), []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	remote := RemoteItem{Source: "HTTP", Name: "same.bin", URL: srv.URL + "/same.bin", DirectURL: srv.URL + "/same.bin"}
	path, err := internalDownloadV854(context.Background(), remote, dest, remote.Name, func(int64, int64) {})
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != "same (1).bin" {
		t.Fatalf("expected collision-safe name, got %s", path)
	}
}
