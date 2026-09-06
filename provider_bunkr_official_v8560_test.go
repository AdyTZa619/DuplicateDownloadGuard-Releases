package main

import "testing"

func TestBunkrMediaPageURLV8560(t *testing.T) {
	res := Result{Remote: RemoteItem{
		Source: "BUNKR",
		URL:    "https://bunkr.si/a/Album123",
		Handle: "wYGCKbGhSvuAW",
	}}
	got := bunkrMediaPageURLV8560(res)
	want := "https://bunkr.si/f/wYGCKbGhSvuAW"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestBunkrMediaPageURLRequiresSlugV8560(t *testing.T) {
	res := Result{Remote: RemoteItem{Source: "BUNKR", URL: "https://bunkr.si/a/Album123"}}
	if got := bunkrMediaPageURLV8560(res); got != "" {
		t.Fatalf("expected empty URL without slug, got %q", got)
	}
}

func TestGalleryDLFilenameFormatEscapesBracesV8560(t *testing.T) {
	got := galleryDLFilenameFormatV8560("a{b}.jpg")
	if got != "a{{b}}.jpg" {
		t.Fatalf("unexpected escaped filename: %q", got)
	}
}
