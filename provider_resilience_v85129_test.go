package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDedicatedProviderLabelsV85129(t *testing.T) {
	cases := map[string]string{
		"https://gofile.io/d/ABC":           "GOFILE",
		"https://www.erome.com/a/ABC":       "EROME",
		"https://bunkr.cr/a/ABC":            "BUNKR",
		"https://app.bunkrrr.example/a/ABC": "BUNKR",
		"https://cyberdrop.me/a/ABC":        "CYBERDROP",
		"https://fs-01.cyberdrop.to/f/ABC":  "CYBERDROP",
		"https://example.org/gallery/ABC":   "GALLERY-DL",
	}
	for raw, want := range cases {
		if got := providerSourceLabelV86(raw); got != want {
			t.Fatalf("providerSourceLabelV86(%q)=%q want=%q", raw, got, want)
		}
	}
}

func TestDedicatedProviderInitialEnrichmentPolicyV85129(t *testing.T) {
	for _, source := range []string{"GOFILE", "CYBERDROP", "EROME"} {
		if shouldEnrichGalleryHTTPV86(source) {
			t.Fatalf("%s must remain metadata-only during initial album scan", source)
		}
	}
	if !shouldEnrichGalleryHTTPV86("BUNKR") {
		t.Fatal("BUNKR must retain concurrent HTTP metadata enrichment for missing sizes")
	}
}

func TestDedicatedProviderExpiredLinksAreRefreshableV85129(t *testing.T) {
	for _, source := range []string{"GOFILE", "BUNKR", "CYBERDROP", "EROME"} {
		if !providerRefreshableSourceV86(source) {
			t.Fatalf("%s must re-extract an expired/forbidden direct media URL lazily", source)
		}
	}
	if providerRefreshableSourceV86("MEGA") {
		t.Fatal("MEGA must continue using its dedicated preview engine")
	}
}

func TestProviderPreviewRefreshStatusesV85129(t *testing.T) {
	for _, code := range []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusGone} {
		if !providerPreviewNeedsRefreshV86(code) {
			t.Fatalf("HTTP %d should trigger provider URL refresh", code)
		}
	}
	for _, code := range []int{http.StatusOK, http.StatusPartialContent, http.StatusTooManyRequests, http.StatusInternalServerError} {
		if providerPreviewNeedsRefreshV86(code) {
			t.Fatalf("HTTP %d must not blindly trigger re-extraction", code)
		}
	}
}

func TestProviderPreviewRequestPreservesRangeAndRefererV85129(t *testing.T) {
	incoming := &http.Request{Method: http.MethodGet, Header: make(http.Header)}
	incoming.Header.Set("Range", "bytes=1048576-2097151")
	incoming.Header.Set("If-Range", `"abc"`)
	item := RemoteItem{
		Source:    "EROME",
		URL:       "https://www.erome.com/a/album",
		DirectURL: "https://cdn.example.test/video.mp4?token=short-lived",
	}
	req, err := buildProviderPreviewRequestV86(context.Background(), incoming, item)
	if err != nil {
		t.Fatal(err)
	}
	if req.URL.String() != item.DirectURL {
		t.Fatalf("preview target=%q want=%q", req.URL.String(), item.DirectURL)
	}
	if got := req.Header.Get("Referer"); got != item.URL {
		t.Fatalf("Referer=%q want=%q", got, item.URL)
	}
	if got := req.Header.Get("Range"); got != "bytes=1048576-2097151" {
		t.Fatalf("Range not forwarded: %q", got)
	}
	if got := req.Header.Get("If-Range"); got != `"abc"` {
		t.Fatalf("If-Range not forwarded: %q", got)
	}
}

func TestProviderPreviewDiagnosticClassifiesFailuresV85129(t *testing.T) {
	item := RemoteItem{Source: "BUNKR"}
	resp := func(status int, contentType, final string) *http.Response {
		u, _ := url.Parse(final)
		return &http.Response{
			StatusCode: status,
			Header:     http.Header{"Content-Type": []string{contentType}},
			Request:    &http.Request{URL: u},
		}
	}
	cases := []struct {
		response *http.Response
		code     string
		ok       bool
	}{
		{resp(200, "video/mp4", "https://cdn.example/file.mp4"), "READY", true},
		{resp(200, "text/html; charset=utf-8", "https://cdn.example/error"), "HTML_INSTEAD_OF_MEDIA", false},
		{resp(403, "text/html", "https://cdn.example/file"), "ACCESS_DENIED", false},
		{resp(404, "text/html", "https://cdn.example/file"), "FILE_UNAVAILABLE", false},
		{resp(429, "text/html", "https://cdn.example/file"), "RATE_LIMITED", false},
		{resp(503, "text/html", "https://cdn.example/file"), "REMOTE_SERVER_ERROR", false},
		{resp(200, "video/mp4", "https://cdn.example/maint.mp4"), "BUNKR_MAINTENANCE", false},
	}
	for _, tc := range cases {
		got := providerPreviewDiagnosticV8559(tc.response, item, false, "")
		if got["code"] != tc.code || got["ok"] != tc.ok {
			t.Fatalf("diagnostic status=%d final=%q => %#v want code=%s ok=%v", tc.response.StatusCode, tc.response.Request.URL, got, tc.code, tc.ok)
		}
	}
}

func TestProviderPreviewDiagnosticIncludesRetryAfterV85130(t *testing.T) {
	u, _ := url.Parse("https://cdn.example/file")
	resp := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     http.Header{"Retry-After": []string{"45"}},
		Request:    &http.Request{URL: u},
	}
	got := providerPreviewDiagnosticV8559(resp, RemoteItem{Source: "GOFILE"}, false, "")
	if got["code"] != "RATE_LIMITED" || !strings.Contains(got["detail"].(string), "Retry-After: 45") {
		t.Fatalf("rate-limit diagnostic incomplete: %#v", got)
	}
}

func TestProviderPreviewTransportBoundsResponseHeadersV85130(t *testing.T) {
	contextFirst, ok := newProviderPreviewTransportV85130().(*providerContextFirstTransportV8558)
	if !ok {
		t.Fatal("provider preview transport must apply resolved per-file context first")
	}
	wrapped, ok := contextFirst.base.(*providerAwareTransportV86)
	if !ok {
		t.Fatal("provider preview transport must retain provider-aware authentication")
	}
	transport, ok := wrapped.base.(*http.Transport)
	if !ok {
		t.Fatal("provider preview base transport must be configurable")
	}
	if transport.ResponseHeaderTimeout != providerPreviewResponseHeaderTimeoutV85130 {
		t.Fatalf("ResponseHeaderTimeout=%s want=%s", transport.ResponseHeaderTimeout, providerPreviewResponseHeaderTimeoutV85130)
	}
}

func TestProviderRefreshPersistsNewURLAndPreservesIdentityV85130(t *testing.T) {
	app := &App{
		appDir: t.TempDir(),
		results: []Result{{ID: 7, Remote: RemoteItem{
			ID: 17, Path: "album/clip.mp4", Name: "clip.mp4", Size: 1234,
			URL: "https://www.erome.com/a/album", DirectURL: "https://old.example/clip.mp4",
			Source: "EROME", Extractor: "erome", ProviderID: "clip-42", ContentType: "video/mp4",
		}}},
	}
	app.revision.Store(10)
	fresh := RemoteItem{DirectURL: "https://new.example/clip.mp4?token=fresh"}
	if !app.replaceResultRemoteV86(7, fresh) {
		t.Fatal("existing result was not replaced")
	}
	if got := app.revision.Load(); got != 11 {
		t.Fatalf("revision=%d want=11", got)
	}

	app.mu.RLock()
	got := app.results[0].Remote
	app.mu.RUnlock()
	if got.ID != 17 || got.Name != "clip.mp4" || got.Path != "album/clip.mp4" || got.Size != 1234 ||
		got.URL != "https://www.erome.com/a/album" || got.Source != "EROME" || got.Extractor != "erome" ||
		got.ProviderID != "clip-42" || got.ContentType != "video/mp4" || got.DirectURL != fresh.DirectURL {
		t.Fatalf("refreshed remote lost stable metadata: %+v", got)
	}

	reloaded := &App{appDir: app.appDir}
	if err := reloaded.loadResults(); err != nil {
		t.Fatalf("load persisted refreshed result: %v", err)
	}
	if len(reloaded.results) != 1 || reloaded.results[0].Remote.DirectURL != fresh.DirectURL {
		t.Fatalf("refreshed URL was not persisted: %+v", reloaded.results)
	}
	if _, err := os.Stat(filepath.Join(app.appDir, "last_results.json.gz")); err != nil {
		t.Fatalf("persisted results missing: %v", err)
	}
}

func TestProviderPreviewProxyForwardsRangeRefererAndPartialResponseV85130(t *testing.T) {
	var seenRange, seenReferer string
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenRange = r.Header.Get("Range")
		seenReferer = r.Header.Get("Referer")
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Content-Range", "bytes 5-8/20")
		w.Header().Set("Accept-Ranges", "bytes")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte("DATA"))
	}))
	defer remote.Close()

	pageURL := "https://www.erome.com/a/album"
	app := &App{results: []Result{{ID: 1, Remote: RemoteItem{
		ID: 1, Name: "clip.mp4", Source: "EROME", URL: pageURL, DirectURL: remote.URL + "/clip.mp4",
	}}}}
	req := httptest.NewRequest(http.MethodGet, "/api/provider/preview?id=1", nil)
	req.Header.Set("Range", "bytes=5-8")
	recorder := httptest.NewRecorder()
	app.handleProviderPreviewMediaV86(recorder, req)

	if recorder.Code != http.StatusPartialContent || !bytes.Equal(recorder.Body.Bytes(), []byte("DATA")) {
		t.Fatalf("proxy response status=%d body=%q", recorder.Code, recorder.Body.String())
	}
	if seenRange != "bytes=5-8" || seenReferer != pageURL {
		t.Fatalf("provider headers Range=%q Referer=%q", seenRange, seenReferer)
	}
	if recorder.Header().Get("Content-Disposition") != "" || recorder.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("unsafe preview response headers: %#v", recorder.Header())
	}
}

func TestEromeGalleryMetadataAndIdentityV85129(t *testing.T) {
	output := `[3,"https://cdn.erome.com/media/clip.mp4?token=x",{"name":"clip.mp4","size":987654,"id":"clip-42","album_name":"Album E"}]` + "\n" +
		`[3,"https://cdn.erome.com/media/photo.jpg?token=y",{"name":"photo.jpg","size":1234,"id":"pic-7","album_name":"Album E"}]` + "\n"
	items := parseGalleryRemoteItemsV86([]byte(output), "https://www.erome.com/a/ABC")
	if len(items) != 2 {
		t.Fatalf("Erome items=%d want=2", len(items))
	}
	for _, item := range items {
		if item.Source != "EROME" || item.Extractor != "erome" {
			t.Fatalf("Erome identity lost: %+v", item)
		}
		if item.ProviderID == "" || item.DirectURL == "" || item.URL != "https://www.erome.com/a/ABC" {
			t.Fatalf("Erome source/direct/provider identity incomplete: %+v", item)
		}
	}
}

func TestDedicatedProvidersStayOutOfGenericPickerV85129(t *testing.T) {
	for _, raw := range []string{
		"https://erome.com/a/test",
		"https://gofile.io/d/test",
		"https://bunkr.cr/a/test",
		"https://cyberdrop.me/a/test",
		"https://mega.nz/folder/test#key",
	} {
		if genericMediaAllowedRootV85127(raw) {
			t.Fatalf("dedicated provider leaked into generic Media Picker: %s", raw)
		}
	}
	if !genericMediaAllowedRootV85127("https://filmepornonline.org/test") {
		t.Fatal("generic HTTP page should remain eligible for Media Picker")
	}
}

func TestProviderUIKeepsDedicatedAndGenericFlowsSeparateV85129(t *testing.T) {
	bridge, err := os.ReadFile("web/feature_generic_media_picker_v85114.js")
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ToLower(string(bridge))
	for _, marker := range []string{"return 'erome'", "return 'bunkr'", "return 'cyberdrop'", "return 'gofile'", "return 'mega'", "return 'web'"} {
		if !strings.Contains(text, marker) {
			t.Fatalf("generic provider guard missing %q", marker)
		}
	}

	providerUI, err := os.ReadFile("web/provider_sources.js")
	if err != nil {
		t.Fatal(err)
	}
	providerText := strings.ToLower(string(providerUI))
	for _, marker := range []string{"id:'erome'", "id:'bunkr'", "id:'cyberdrop'", "id:'gofile'", "id:'mega'", "provider?.id === 'web'"} {
		if !strings.Contains(providerText, marker) {
			t.Fatalf("dedicated provider routing missing %q", marker)
		}
	}
}
