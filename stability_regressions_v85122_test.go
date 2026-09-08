package main

import (
	"testing"
	"time"
)

func TestEromeUsesDedicatedMetadataOnlyGalleryPathV85122(t *testing.T) {
	for _, raw := range []string{
		"https://www.erome.com/a/G4szqJSJ",
		"https://erome.com/a/G4szqJSJ",
	} {
		if got := providerSourceLabelV86(raw); got != "EROME" {
			t.Fatalf("providerSourceLabelV86(%q)=%q, want EROME", raw, got)
		}
	}
	if shouldEnrichGalleryHTTPV86("EROME") {
		t.Fatal("Erome initial scan must not probe every media URL over HTTP")
	}
	if !shouldEnrichGalleryHTTPV86("BUNKR") {
		t.Fatal("Bunkr enrichment behavior must remain unchanged")
	}
}

func TestNativeCloseFastPathRequiresAllSignalsV85122(t *testing.T) {
	now := time.Now()
	hint := now.Add(-5 * time.Second).UnixNano()
	last := hint
	if !shouldStopNativeCloseV85122(now, last, hint, false, true) {
		t.Fatal("destroyed latched HWND + pagehide + no replacement should stop")
	}
	if shouldStopNativeCloseV85122(now, last, hint, true, true) {
		t.Fatal("a replacement/present DDG window must keep backend alive")
	}
	if shouldStopNativeCloseV85122(now, hint+1, hint, false, true) {
		t.Fatal("heartbeat newer than pagehide must cancel close")
	}
	if shouldStopNativeCloseV85122(now, last, hint, false, false) {
		t.Fatal("uncertain window disappearance must use conservative fallback")
	}
}
