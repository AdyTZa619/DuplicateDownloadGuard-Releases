package main

import (
	"context"
	"net/http"
	"net/url"
	"os"
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
