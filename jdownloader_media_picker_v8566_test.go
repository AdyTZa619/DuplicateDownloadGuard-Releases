package main

import (
	"os"
	"strings"
	"testing"
)

func TestJDownloaderFastV8567UsesOneShotCurrentFlashGotShape(t *testing.T) {
	b, err := os.ReadFile("web/jdownloader_fast_v8567.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{
		"params.set('urls', urls.join('\\n'))",
		"params.set('descriptions', descriptions.join('\\n'))",
		"params.set('fnames', filenames.join('\\n'))",
		"params.set('package', packageName)",
		"params.set('referer', referer)",
		"form.submit()",
		"Trimite TOATE",
		"sendExactIDs",
		"event.stopImmediatePropagation()",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing deterministic JD behavior %q", want)
		}
	}
	if strings.Contains(s, "params.set('description',") {
		t.Fatal("current JD flashgot implementation reads plural descriptions")
	}
	if strings.Contains(s, "window.api('/api/download/preflight'") {
		t.Fatal("fast JD flow must never run the HDD preflight")
	}
	if strings.Contains(s, "fetch(`${JD_BASE}/flashgot`") {
		t.Fatal("fast JD flow must not POST once via fetch and retry via form")
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
	for _, want := range []string{"window.addEventListener('click'", "event.stopImmediatePropagation()", "ddgJDownloaderFastV8566"} {
		if !strings.Contains(guard, want) {
			t.Fatalf("window JD guard missing %q", want)
		}
	}
}

func TestMediaPickerV8566UsesEstablishedExtractorsAndExactJDIDs(t *testing.T) {
	b, err := os.ReadFile("web/media_picker_v8566.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{
		"erome\\.com",
		"cyberdrop\\.",
		"select.value = 'gallery-dl'",
		"/api/provider-preview/media?id=",
		"sendExactIDs",
		"Trimite TOATE în JD",
		"Media Picker",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing Media Picker behavior %q", want)
		}
	}
}
