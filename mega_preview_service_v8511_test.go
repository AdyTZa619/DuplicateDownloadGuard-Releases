package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestMegaPreviewServiceRepeatedSwitchesUseOnlyRootV8511(t *testing.T) {
	a := &App{preview: MegaPreviewState{
		Active:     true,
		SourceURL:  "https://mega.nz/folder/test#key",
		RemotePath: megaWarmRootRefV86,
		StreamURL:  "http://127.0.0.1:4443/root",
		RootURL:    "http://127.0.0.1:4443/root",
	}}
	for i := 0; i < 100; i++ {
		item := RemoteItem{Source: "MEGA", URL: a.preview.SourceURL, Path: "/set/video-" + time.Unix(int64(i), 0).Format("150405") + ".mp4"}
		got, mode, ok := a.tryMegaPreviewUICacheV854(item)
		if !ok || mode != "MEGA ROOT SERVICE" || !strings.Contains(got, "/root/set/video-") {
			t.Fatalf("switch %d left root service: ok=%v mode=%q url=%q", i, ok, mode, got)
		}
		if a.preview.FallbackRemotePath != "" {
			t.Fatalf("switch %d created a per-file location: %#v", i, a.preview)
		}
	}
}

func TestMegaPreviewTransportProbeSeparatesCodecErrorV8511(t *testing.T) {
	var heads, ranges int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			heads++
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if r.Header.Get("Range") == "bytes=0-0" {
			ranges++
			w.Header().Set("Content-Range", "bytes 0-0/10")
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write([]byte{0})
			return
		}
		http.Error(w, "unexpected", http.StatusBadRequest)
	}))
	defer ts.Close()
	probe := probeMegaHTTPV8511(context.Background(), ts.URL+"/clip.mkv", 2*time.Second)
	if !probe.Reachable || probe.Method != "GET Range" || heads != 1 || ranges != 1 {
		t.Fatalf("probe=%#v heads=%d ranges=%d", probe, heads, ranges)
	}
}

func TestMegaPreviewFallbackIsBoundedAcrossManyFailuresV8511(t *testing.T) {
	currentPath, currentURL := "", ""
	activeLocations := map[string]bool{"/": true}
	for i := 0; i < 50; i++ {
		requested := "H:" + string(rune('A'+i%20))
		remove, reuse := planMegaFallbackV8511(currentPath, currentURL, requested)
		if remove != "" {
			delete(activeLocations, remove)
			currentPath, currentURL = "", ""
		}
		if reuse == "" {
			currentPath, currentURL = requested, "http://127.0.0.1:4443/"+requested
			activeLocations[currentPath] = true
		}
		if len(activeLocations) > 2 {
			t.Fatalf("switch %d accumulated locations: %#v", i, activeLocations)
		}
	}
}

func TestMegaPreviewPersistedRootRequiresExactListingV8511(t *testing.T) {
	listing := "Served locations:\n/ http://127.0.0.1:4443/right/"
	if _, ok := webDAVListingContainsRootV8511(listing, "http://127.0.0.1:4443/wrong/"); ok {
		t.Fatal("a listener belonging to another WebDAV root was accepted")
	}
	if got, ok := webDAVListingContainsRootV8511(listing, "http://127.0.0.1:4443/right/"); !ok || got == "" {
		t.Fatalf("exact listed root rejected: %q %v", got, ok)
	}
}

func TestMegaPreviewDiagnosticsRedactSecretsV8511(t *testing.T) {
	secret := strings.Repeat("A", 48)
	got := redactMegaDiagnosticV8511("session\n" + secret + "\nhttps://mega.nz/folder/abc#secret")
	if strings.Contains(got, secret) || strings.Contains(got, "#secret") {
		t.Fatalf("diagnostic leaked a secret: %q", got)
	}
}

func TestMegaPreviewHasSingleBrowserFallbackOwnerV8511(t *testing.T) {
	quick, err := os.ReadFile("web/preview_quick_v86.js")
	if err != nil {
		t.Fatal(err)
	}
	exact, err := os.ReadFile("web/exact_guard.js")
	if err != nil {
		t.Fatal(err)
	}
	if count := strings.Count(string(quick), "forceFallback:true") + strings.Count(string(exact), "forceFallback: true"); count != 1 {
		t.Fatalf("browser fallback owners=%d want 1", count)
	}
}

func TestMegaPreviewQuotaHTTPIsClassifiedV8511(t *testing.T) {
	problem := classifyMegaProblem("HTTP 509 transfer quota exceeded", errors.New("HTTP 509"))
	if problem.Code != "MEGA_QUOTA" {
		t.Fatalf("code=%s", problem.Code)
	}
}
