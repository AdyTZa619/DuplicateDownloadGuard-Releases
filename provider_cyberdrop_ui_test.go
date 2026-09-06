package main

import (
	"strings"
	"testing"
)

func TestCyberdropStableMediaRoutingIsEmbedded(t *testing.T) {
	jd, err := webFS.ReadFile("web/jdownloader_final_v8551.js")
	if err != nil {
		t.Fatalf("read JDownloader final module: %v", err)
	}
	jds := string(jd)
	for _, marker := range []string{
		"function cyberdropURL(row)",
		"CYBERDROP",
		"providerId",
		"/f/${encodeURIComponent(String(r.providerId))}",
		"gofileURL(row) || bunkrURL(row) || cyberdropURL(row)",
	} {
		if !strings.Contains(jds, marker) {
			t.Fatalf("JDownloader Cyberdrop routing missing marker %q", marker)
		}
	}

	ui, err := webFS.ReadFile("web/provider_compare_ui_v8558.js")
	if err != nil {
		t.Fatalf("read provider compare module: %v", err)
	}
	uis := string(ui)
	for _, marker := range []string{
		"function cyberdropPublicMediaURL(row)",
		"CYBERDROP",
		"row?.remote?.providerId",
		"bunkrPublicMediaURL(row) || cyberdropPublicMediaURL(row)",
	} {
		if !strings.Contains(uis, marker) {
			t.Fatalf("Compare Studio Cyberdrop routing missing marker %q", marker)
		}
	}
}

func TestCyberdropProviderParserKeepsIdentityAndSize(t *testing.T) {
	output := `[3,"https://k1-cd.cdn.gigachad-cdn.ru/api/file/d/abc?token=x",{"name":"clip.mp4","size":987654,"id":"file-id-123","album_name":"Album C"}]` + "\n"
	items := parseGalleryRemoteItemsV86([]byte(output), "https://cyberdrop.cr/a/ALBUM")
	if len(items) != 1 {
		t.Fatalf("items=%d want=1", len(items))
	}
	it := items[0]
	if it.Source != "CYBERDROP" || it.Name != "clip.mp4" || it.Size != 987654 || it.ProviderID != "file-id-123" {
		t.Fatalf("unexpected Cyberdrop item: %+v", it)
	}
	if got := providerSourceLabelV86("https://cyberdrop.me/a/ALBUM"); got != "CYBERDROP" {
		t.Fatalf("cyberdrop.me detected as %q", got)
	}
	if got := providerSourceLabelV86("https://cyberdrop.to/f/FILE"); got != "CYBERDROP" {
		t.Fatalf("cyberdrop.to detected as %q", got)
	}
}
