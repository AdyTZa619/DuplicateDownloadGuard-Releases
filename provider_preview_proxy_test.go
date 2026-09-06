package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBuildProviderPreviewRequestPreservesRangeV86(t *testing.T) {
	incoming := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/provider-preview/media?id=1", nil)
	incoming.Header.Set("Range", "bytes=100-199")
	item := RemoteItem{URL: "https://example.test/page", DirectURL: "https://cdn.example.test/file.mp4"}
	req, err := buildProviderPreviewRequestV86(incoming.Context(), incoming, item)
	if err != nil {
		t.Fatal(err)
	}
	if got := req.Header.Get("Range"); got != "bytes=100-199" {
		t.Fatalf("Range=%q", got)
	}
	if got := req.Header.Get("Referer"); got != item.URL {
		t.Fatalf("Referer=%q", got)
	}
}

func TestProviderPreviewProxyStreamsRangeV86(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Range"); got != "bytes=2-5" {
			http.Error(w, "bad range: "+got, http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Content-Range", "bytes 2-5/10")
		w.Header().Set("Content-Disposition", `attachment; filename="clip.mp4"`)
		w.WriteHeader(http.StatusPartialContent)
		_, _ = io.WriteString(w, "2345")
	}))
	defer upstream.Close()

	a := &App{results: []Result{{ID: 7, Remote: RemoteItem{ID: 7, Name: "clip.mp4", Source: "HTTP", DirectURL: upstream.URL + "/clip.mp4"}}}}
	req := httptest.NewRequest(http.MethodGet, "/api/provider-preview/media?id=7", nil)
	req.Header.Set("Range", "bytes=2-5")
	rr := httptest.NewRecorder()
	a.handleProviderPreviewMediaV86(rr, req)

	resp := rr.Result()
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d body=%q", resp.StatusCode, string(body))
	}
	if got := resp.Header.Get("Content-Range"); got != "bytes 2-5/10" {
		t.Fatalf("Content-Range=%q", got)
	}
	if got := resp.Header.Get("Content-Disposition"); got != "" {
		t.Fatalf("preview propagated attachment header: %q", got)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "2345" {
		t.Fatalf("body=%q", string(body))
	}
}

func TestProviderPreviewProxyRejectsMegaV86(t *testing.T) {
	a := &App{results: []Result{{ID: 3, Remote: RemoteItem{ID: 3, Name: "clip.mp4", Source: "MEGA", URL: "https://mega.nz/file/x"}}}}
	req := httptest.NewRequest(http.MethodGet, "/api/provider-preview/media?id=3", nil)
	rr := httptest.NewRecorder()
	a.handleProviderPreviewMediaV86(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%q", rr.Code, rr.Body.String())
	}
}

func TestProviderPreviewURLForRequestV86(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:12345/x", nil)
	got := providerPreviewURLForRequestV86(req, 42)
	if got != "http://127.0.0.1:12345/api/provider-preview/media?id=42" {
		t.Fatalf("url=%q", got)
	}
	if !strings.HasPrefix(providerPreviewPathV86(42), "/api/provider-preview/media?") {
		t.Fatalf("path=%q", providerPreviewPathV86(42))
	}
}

func TestProviderRemoteMatchRankV86(t *testing.T) {
	old := RemoteItem{Name: "clip.mp4", Path: "Album/clip.mp4", ProviderID: "abc"}
	if got := providerRemoteMatchRankV86(old, RemoteItem{Name: "other.mp4", Path: "Else/other.mp4", ProviderID: "abc"}); got != 3 {
		t.Fatalf("provider ID rank=%d", got)
	}
	if got := providerRemoteMatchRankV86(old, RemoteItem{Name: "other.mp4", Path: "Album/clip.mp4"}); got != 2 {
		t.Fatalf("path rank=%d", got)
	}
	if got := providerRemoteMatchRankV86(old, RemoteItem{Name: "clip.mp4", Path: "Else/clip.mp4"}); got != 1 {
		t.Fatalf("name rank=%d", got)
	}
}

func TestProviderPreviewMaintenanceURLV8559(t *testing.T) {
	for _, raw := range []string{
		"https://cdn.example/x/maint.mp4",
		"https://cdn.example/x/maintenance-vid.mp4?token=1",
	} {
		if !providerPreviewMaintenanceURLV8559(raw) {
			t.Fatalf("maintenance URL not detected: %s", raw)
		}
	}
	if providerPreviewMaintenanceURLV8559("https://cdn.example/x/real-video.mp4") {
		t.Fatal("normal media classified as maintenance")
	}
}

func TestProviderPreviewDiagnosticUnavailableV8559(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer upstream.Close()

	a := &App{results: []Result{{ID: 9, Remote: RemoteItem{ID: 9, Name: "missing.mp4", Source: "HTTP", DirectURL: upstream.URL + "/missing.mp4"}}}}
	req := httptest.NewRequest(http.MethodGet, "/api/provider-preview/media?id=9&diagnose=1", nil)
	rr := httptest.NewRecorder()
	a.handleProviderPreviewMediaV86(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rr.Code, rr.Body.String())
	}
	var data map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &data); err != nil {
		t.Fatal(err)
	}
	if data["code"] != "FILE_UNAVAILABLE" {
		t.Fatalf("code=%v body=%s", data["code"], rr.Body.String())
	}
}

func TestProviderPreviewDiagnosticPlayableButPlayerMayFailV8559(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Content-Range", "bytes 0-0/100")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = io.WriteString(w, "x")
	}))
	defer upstream.Close()

	a := &App{results: []Result{{ID: 10, Remote: RemoteItem{ID: 10, Name: "codec.mp4", Source: "BUNKR", DirectURL: upstream.URL + "/codec.mp4"}}}}
	req := httptest.NewRequest(http.MethodGet, "/api/provider-preview/media?id=10&diagnose=1", nil)
	rr := httptest.NewRecorder()
	a.handleProviderPreviewMediaV86(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rr.Code, rr.Body.String())
	}
	var data map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &data); err != nil {
		t.Fatal(err)
	}
	if data["code"] != "READY" || data["ok"] != true {
		t.Fatalf("unexpected diagnostic: %s", rr.Body.String())
	}
}
