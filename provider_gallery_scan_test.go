package main

import (
	"os"
	"strings"
	"testing"
)

func TestParseGalleryRemoteItemsGoFileV86(t *testing.T) {
	output := `[3,"https://store1.gofile.io/download/web/abc/file.mp4",{"name":"file.mp4","size":123456,"id":"abc"}]` + "\n"
	items := parseGalleryRemoteItemsV86([]byte(output), "https://gofile.io/d/FOLDER")
	if len(items) != 1 {
		t.Fatalf("items=%d", len(items))
	}
	it := items[0]
	if it.Source != "GOFILE" || it.Name != "file.mp4" || it.Size != 123456 || it.ProviderID != "abc" {
		t.Fatalf("unexpected GoFile item: %+v", it)
	}
	if it.URL != "https://gofile.io/d/FOLDER" || !strings.Contains(it.DirectURL, "store1.gofile.io") {
		t.Fatalf("source/direct URL not preserved: %+v", it)
	}
}

func TestParseGalleryRemoteItemsClassicPrettyJSONV86(t *testing.T) {
	// gallery-dl builds without output.jsonl emit one pretty-printed JSON array.
	// DDG must accept that format too, otherwise a valid GoFile extraction becomes
	// an empty result list on older installations.
	output := `[
  [
    2,
    "",
    {"title":"Folder"}
  ],
  [
    3,
    "https://store1.gofile.io/download/web/abc/file.mp4",
    {"name":"file.mp4","size":123456,"id":"abc"}
  ],
  [
    3,
    "https://store2.gofile.io/download/web/def/pic.jpg",
    {"name":"pic.jpg","size":321,"id":"def"}
  ]
]`
	items := parseGalleryRemoteItemsV86([]byte(output), "https://gofile.io/d/FOLDER")
	if len(items) != 2 {
		t.Fatalf("items=%d want=2", len(items))
	}
	if items[0].Name != "file.mp4" || items[0].ProviderID != "abc" || items[1].Name != "pic.jpg" || items[1].ProviderID != "def" {
		t.Fatalf("unexpected classic JSON items: %+v", items)
	}
}

func TestParseGalleryRemoteItemsIgnoresQueueMessagesV86(t *testing.T) {
	output := `[6,"https://gofile.io/d/CHILD",{"id":"folder"}]` + "\n" +
		`[3,"https://store1.gofile.io/download/web/abc/file.mp4",{"name":"file.mp4","size":10,"id":"abc"}]` + "\n"
	items := parseGalleryRemoteItemsV86([]byte(output), "https://gofile.io/d/FOLDER")
	if len(items) != 1 || items[0].Name != "file.mp4" {
		t.Fatalf("queue/control message became a file: %+v", items)
	}
}

func TestParseGalleryRemoteItemsBunkrV86(t *testing.T) {
	output := `[3,"https://cdn.bunkr.si/path/clip.mp4?token=x",{"name":"clip","extension":"mp4","size":999,"id_url":"42","album_name":"Album X"}]` + "\n"
	items := parseGalleryRemoteItemsV86([]byte(output), "https://bunkr.si/a/album")
	if len(items) != 1 {
		t.Fatalf("items=%d", len(items))
	}
	it := items[0]
	if it.Source != "BUNKR" || it.Name != "clip.mp4" || it.ProviderID != "42" || it.Size != 999 {
		t.Fatalf("unexpected Bunkr item: %+v", it)
	}
	if !strings.Contains(filepathSlashForTestV86(it.Path), "Album X/clip.mp4") {
		t.Fatalf("album path missing: %q", it.Path)
	}
}

func TestParseGalleryRemoteItemsCyberdropV86(t *testing.T) {
	output := `[3,"https://fs-01.cyberdrop.to/file.bin",{"name":"asset.bin","size":321,"id":"xyz"}]` + "\n"
	items := parseGalleryRemoteItemsV86([]byte(output), "https://cyberdrop.cr/a/test")
	if len(items) != 1 || items[0].Source != "CYBERDROP" {
		t.Fatalf("unexpected Cyberdrop items: %+v", items)
	}
}

func TestMergeGalleryProbePreservesProviderIdentityV86(t *testing.T) {
	base := RemoteItem{Name: "clip.mp4", Path: "Album/clip.mp4", Size: -1, Source: "BUNKR", URL: "https://bunkr.si/a/a", DirectURL: "https://cdn.bunkr.si/x", Extractor: "bunkr", ProviderID: "42"}
	probe := RemoteItem{Size: 777, Hash: "abcd", HashType: "md5", ContentType: "video/mp4", ETag: `"etag"`, AcceptRanges: true, Name: "wrong-name.mp4", Source: "HTTP"}
	got := mergeGalleryProbeV86(base, probe)
	if got.Name != base.Name || got.Path != base.Path || got.Source != base.Source || got.URL != base.URL || got.DirectURL != base.DirectURL || got.ProviderID != base.ProviderID {
		t.Fatalf("provider identity changed: %+v", got)
	}
	if got.Size != 777 || got.Hash != "abcd" || got.HashType != "md5" || got.ContentType != "video/mp4" || !got.AcceptRanges {
		t.Fatalf("HTTP enrichment missing: %+v", got)
	}
}

func TestLegacyGalleryScannerRoutesToRichProviderV86(t *testing.T) {
	b, err := os.ReadFile("v7_extra.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, "return a.probeGalleryDLRichV86(ctx, u)") {
		t.Fatal("probeGalleryDL is not routed through rich provider scanner")
	}
	if strings.Contains(s, `exec.CommandContext(ctx, exe, "-G", "--no-download", "--no-colors", u)`) {
		t.Fatal("legacy URL-only gallery-dl scanner is still active")
	}

	rich, err := os.ReadFile("provider_gallery_scan.go")
	if err != nil {
		t.Fatal(err)
	}
	rs := string(rich)
	for _, marker := range []string{"output.private=true", "output.jsonl=true", "--cookies-export", "json.NewDecoder", "walkGalleryJSONV86"} {
		if !strings.Contains(rs, marker) {
			t.Fatalf("rich scanner missing gallery-dl compatibility marker %q", marker)
		}
	}
}

func filepathSlashForTestV86(s string) string {
	return strings.ReplaceAll(s, "\\", "/")
}
