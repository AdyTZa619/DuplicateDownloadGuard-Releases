package main

import (
	"os"
	"strings"
	"testing"
)

func TestJDownloaderFastV8566UsesOneShotOfficialFlashGotShape(t *testing.T) {
	b, err := os.ReadFile("web/jdownloader_fast_v8566.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{
		"params.set('urls', urls.join('\\n'))",
		"params.set('description', descriptions.join('\\n'))",
		"params.set('package', packageName)",
		"form.submit()",
		"Trimite TOATE",
		"sendExactIDs",
		"event.stopImmediatePropagation()",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing deterministic JD behavior %q", want)
		}
	}
	if strings.Contains(s, "params.set('descriptions'") {
		t.Fatal("JDExternInterface parameter must be singular description")
	}
	if strings.Contains(s, "window.api('/api/download/preflight'") {
		t.Fatal("fast JD flow must never run the HDD preflight")
	}
	if strings.Contains(s, "fetch(`${JD_BASE}/flashgot`") {
		t.Fatal("fast JD flow must not POST once via fetch and retry via form after CORS")
	}
}

func TestJDownloaderFastV8566LoadsBeforeLegacyCaptureHandlers(t *testing.T) {
	b, err := os.ReadFile("web/preview_quick_v86.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	fast := strings.Index(s, "/jdownloader_fast_v8566.js")
	batch := strings.Index(s, "/jdownloader_batch_confirm_v8564.js")
	final := strings.Index(s, "/jdownloader_final_v8551.js")
	if fast < 0 || batch < 0 || final < 0 || !(fast < batch && batch < final) {
		t.Fatalf("JD fast router must load first: fast=%d batch=%d final=%d", fast, batch, final)
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
