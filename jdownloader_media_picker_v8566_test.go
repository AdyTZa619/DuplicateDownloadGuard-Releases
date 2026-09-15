package main

import (
	"os"
	"strings"
	"testing"
)

func TestJDownloaderFastV8567DelegatesEverySendToCanonicalGuard(t *testing.T) {
	b, err := os.ReadFile("web/jdownloader_fast_v8567.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{
		"const guard = window.ddgJDownloaderGuardV901",
		"const result = await guard.sendIDs(ids)",
		"return await submitRows(unique.map(id => ({id})))",
		"sendExactIDs",
		"event.stopImmediatePropagation()",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing guarded JD behavior %q", want)
		}
	}
	for _, forbidden := range []string{"/flashgot", "form.submit()", "submitOneForm(", "window.api('/api/download/preflight'", "Trimite TOATE", "ddgJDFastAllV8567"} {
		if strings.Contains(s, forbidden) {
			t.Fatalf("fast JD flow retained bypass %q", forbidden)
		}
	}
}

func TestJDownloaderPopupRequiresFinalMissingDetectorVerdictV901(t *testing.T) {
	b, err := os.ReadFile("web/jdownloader_fast_v8567.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	finalVerdict := "if (guard === 'DOWNLOAD') return detector === 'LIPSĂ' ? 'DOWNLOAD' : 'REVIEW';"
	if !strings.Contains(s, finalVerdict) || !strings.Contains(s, "if (guard === 'DUPLICATE') return localPresent ? 'DUPLICATE' : 'REVIEW';") {
		t.Fatal("JD popup is not combining final Guard, detector truth, and current local presence")
	}
	if strings.Contains(s, "if (guard === 'DOWNLOAD' && !manual) return 'DOWNLOAD';") {
		t.Fatal("manual flag must not override a final DOWNLOAD guard verdict")
	}
	guardPos := strings.Index(s, "const guard = String(row?.guardVerdict")
	finalPos := strings.Index(s, finalVerdict)
	statusPos := strings.Index(s, "const status = String(row?.status")
	if guardPos < 0 || finalPos < 0 || statusPos < 0 || !(guardPos < finalPos && finalPos < statusPos) {
		t.Fatalf("final guard plus detector truth must take precedence before legacy/manual status fallback: guard=%d final=%d status=%d", guardPos, finalPos, statusPos)
	}
}

func TestJDownloaderFastV8567LoadsBehindWindowGuardAndBeforeLegacyHandlers(t *testing.T) {
	b, err := os.ReadFile("web/preview_quick_v86.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	windowGuard := strings.Index(s, "/jdownloader_window_capture_v8566.js")
	fast := strings.Index(s, "/jdownloader_fast_v8567.js")
	batch := strings.Index(s, "/jdownloader_batch_confirm_v8564.js")
	final := strings.Index(s, "/jdownloader_final_v8551.js")
	if windowGuard < 0 || fast < 0 || batch < 0 || final < 0 || !(windowGuard < fast && fast < batch && batch < final) {
		t.Fatalf("JD routing order invalid: window=%d fast=%d batch=%d final=%d", windowGuard, fast, batch, final)
	}

	guardBytes, err := os.ReadFile("web/jdownloader_window_capture_v8566.js")
	if err != nil {
		t.Fatal(err)
	}
	guard := string(guardBytes)
	for _, want := range []string{"window.addEventListener('click'", "event.stopImmediatePropagation()", "ddgJDownloaderGuardV901", "/api/download/jdownloader-direct"} {
		if !strings.Contains(guard, want) {
			t.Fatalf("window JD guard missing %q", want)
		}
	}
	if strings.Contains(guard, "ddgJDownloaderFastV8566") {
		t.Fatal("earliest window handler still delegates to the unsafe fast module")
	}
}

func TestMediaPickerV8566UsesEstablishedExtractorsAndExactJDIDs(t *testing.T) {
	b, err := os.ReadFile("web/media_picker_v8566.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{
		"/api/provider-preview/media?id=",
		"sendExactIDs",
		"Trimite lipsurile selectate în JD",
		"Selectează lipsurile",
		"function genericPickerAllowed(raw)",
		"if (!genericPickerAllowed(sourceURL))",
		"Media Picker",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing Media Picker behavior %q", want)
		}
	}
	for _, forbidden := range []string{"autoOpenAfterScan", "knownGallery", "select.value = 'gallery-dl'", "Trimite TOATE în JD"} {
		if strings.Contains(s, forbidden) {
			t.Fatalf("generic Media Picker retained dedicated/all-files path %q", forbidden)
		}
	}
}
