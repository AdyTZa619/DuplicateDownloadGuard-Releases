package main

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestJDownloaderBatchConfirmationV8564(t *testing.T) {
	b, err := os.ReadFile("web/jdownloader_batch_confirm_v8564.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{
		"params.set('package', packageName)",
		"Trimite TOATE oricum în JDownloader",
		"window.confirm(",
		"guardJDSafeOnly",
		"Deliberately omit dir + autostart",
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing JD batch behavior %q", want)
		}
	}
	if strings.Contains(s, "params.set('dir'") || strings.Contains(s, "params.set('autostart'") {
		t.Fatal("JD batch must not force destination or autostart")
	}
}

func TestJDBatchModuleLoadsBeforeLegacyFinalV8564(t *testing.T) {
	b, err := os.ReadFile("web/preview_quick_v86.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	batch := strings.Index(s, "/jdownloader_batch_confirm_v8564.js")
	legacy := strings.Index(s, "/jdownloader_final_v8551.js")
	if batch < 0 || legacy < 0 || batch > legacy {
		t.Fatalf("batch override must load before legacy JD handler: batch=%d legacy=%d", batch, legacy)
	}
}

func TestGuardFreshIndexAllowsInspectionWindowV8564(t *testing.T) {
	if guardFreshIndexTTLV8545 < 5*time.Minute {
		t.Fatalf("fresh guard index TTL too short: %s", guardFreshIndexTTLV8545)
	}
}
