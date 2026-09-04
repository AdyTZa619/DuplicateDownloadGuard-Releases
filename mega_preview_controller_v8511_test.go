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

func TestMegaPreviewRepeatedRootChangesStayCommandFreeV8511(t *testing.T) {
	a := &App{preview: MegaPreviewState{
		Active:     true,
		SourceURL:  "https://mega.nz/folder/test#key",
		RemotePath: megaWarmRootRefV86,
		StreamURL:  "http://127.0.0.1:4443/root",
	}}
	for i := 0; i < 100; i++ {
		item := RemoteItem{Source: "MEGA", URL: a.preview.SourceURL, Path: "/set/clip-" + time.Unix(int64(i), 0).Format("150405") + ".mp4"}
		url, mode, ok := a.tryMegaPreviewUICacheV854(item)
		if !ok || mode != "MEGA ROOT SERVICE" || !strings.Contains(url, "/root/set/clip-") {
			t.Fatalf("switch %d left root path: ok=%v mode=%q url=%q", i, ok, mode, url)
		}
	}
}

func TestMegaPreviewFallbackStaysBoundedAcrossManyFailuresV8511(t *testing.T) {
	currentPath, currentURL := "", ""
	activeLocations := map[string]bool{megaWarmRootRefV86: true}
	for i := 0; i < 100; i++ {
		requested := "H:" + time.Unix(int64(i), 0).Format("150405")
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
			t.Fatalf("switch %d accumulated WebDAV locations: %#v", i, activeLocations)
		}
	}
}

func TestMegaPreviewSlowFailureDoesNotAuthorizeReloginV8511(t *testing.T) {
	for _, problem := range []MegaProblem{
		classifyMegaProblem("", context.DeadlineExceeded),
		classifyMegaProblem("network connection failed", errors.New("failed")),
		classifyMegaProblem("HTTP 509 transfer quota exceeded", errors.New("HTTP 509")),
	} {
		if megaMayReplaceSessionV8511(problem) {
			t.Fatalf("slow/transient failure %s would stack a logout/login chain", problem.Code)
		}
	}
	if !megaMayReplaceSessionV8511(classifyMegaProblem("not logged in", errors.New("not logged in"))) {
		t.Fatal("an explicitly missing session must allow one controlled login")
	}
}

func TestMegaBrowserErrorWithWorkingBytesIsCodecNotFallbackV8511(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if r.Header.Get("Range") == "bytes=0-0" {
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write([]byte{0})
			return
		}
		http.Error(w, "unexpected", http.StatusBadRequest)
	}))
	defer ts.Close()
	item := RemoteItem{Source: "MEGA", URL: "source", Path: "/clip.mkv", Name: "clip.mkv"}
	a := &App{preview: MegaPreviewState{Active: true, SourceURL: item.URL, RemotePath: megaWarmRootRefV86, StreamURL: ts.URL + "/root"}}
	result, handled, err := a.diagnoseMegaBrowserErrorV8511(item, "MP-TEST")
	if err != nil || !handled || !result.TransportOK || result.FallbackUsed {
		t.Fatalf("result=%#v handled=%v err=%v", result, handled, err)
	}
}

func TestMegaBrowserErrorReportsQuotaV8511(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(509)
		_, _ = w.Write([]byte("transfer quota exceeded"))
	}))
	defer ts.Close()
	item := RemoteItem{Source: "MEGA", URL: "source", Path: "/clip.mp4"}
	a := &App{preview: MegaPreviewState{Active: true, SourceURL: item.URL, RemotePath: megaWarmRootRefV86, StreamURL: ts.URL + "/root"}}
	_, handled, err := a.diagnoseMegaBrowserErrorV8511(item, "MP-QUOTA")
	if !handled || err == nil || megaProblemFromError(err).Code != "MEGA_QUOTA" {
		t.Fatalf("handled=%v error=%v problem=%#v", handled, err, megaProblemFromError(err))
	}
}

func TestMegaPreviewProbeUsesRangeWhenHeadUnsupportedV8511(t *testing.T) {
	var heads, ranges int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			heads++
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if r.Header.Get("Range") == "bytes=0-0" {
			ranges++
			w.WriteHeader(http.StatusPartialContent)
			return
		}
		http.Error(w, "unexpected", http.StatusBadRequest)
	}))
	defer ts.Close()
	probe := probeMegaPreviewURLV8511(context.Background(), ts.URL+"/file")
	if !probe.Reachable || heads != 1 || ranges != 1 {
		t.Fatalf("probe=%#v heads=%d ranges=%d", probe, heads, ranges)
	}
}

func TestOnlyOneJavaScriptFallbackOwnerV8511(t *testing.T) {
	quick, err := os.ReadFile("web/preview_quick_v86.js")
	if err != nil {
		t.Fatal(err)
	}
	exact, err := os.ReadFile("web/exact_guard.js")
	if err != nil {
		t.Fatal(err)
	}
	count := strings.Count(string(quick), "forceFallback:true") + strings.Count(string(exact), "forceFallback: true")
	if count != 1 {
		t.Fatalf("fallback owners=%d want 1", count)
	}
}

func TestSameSourceScanCanReuseRootV8511(t *testing.T) {
	a := &App{preview: MegaPreviewState{Active: true, SourceURL: "same", RemotePath: megaWarmRootRefV86, StreamURL: "http://127.0.0.1:4443/root"}}
	if !a.megaPreviewSameSourceRootV8511("same") {
		t.Fatal("same-source scan would destroy its working root")
	}
	if a.megaPreviewSameSourceRootV8511("other") {
		t.Fatal("different source reused the wrong session")
	}
}

func TestMegaDiagnosticsRedactSessionAndLinkV8511(t *testing.T) {
	secret := strings.Repeat("A", 48)
	got := redactMegaOutputV8511(secret + "\nhttps://mega.nz/folder/id#key")
	if strings.Contains(got, secret) || strings.Contains(got, "#key") {
		t.Fatalf("secret leaked in diagnostic: %q", got)
	}
}
