package main

import (
	"os"
	"strings"
	"testing"
)

func TestSmartStateEngineEvidenceFirstAndNoJDRescan(t *testing.T) {
	b, err := os.ReadFile("web/smart_state_engine_v8568.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	markers := []string{
		"function strongLocalEvidence(row)",
		"function exactCurrentEvidence(row)",
		"function shouldReleaseManual(row)",
		"manual === 'MISSING'",
		"manual === 'DIFFERENT'",
		"status: 'AUTO'",
		"/api/results/mark",
		"function finalState(row, store = loadTransfers())",
		"if (strongLocalEvidence(row)) return 'HAVE'",
		"if (pendingTransfer(row, store)) return 'IN_JD'",
		"AI DEJA",
		"DE VERIFICAT",
		"LIPSEȘTE",
		"ÎN JD",
	}
	for _, marker := range markers {
		if !strings.Contains(s, marker) {
			t.Fatalf("Smart State Engine missing marker %q", marker)
		}
	}
	for _, forbidden := range []string{"/api/download/preflight", "/api/index/start", "/api/mega/scan"} {
		if strings.Contains(s, forbidden) {
			t.Fatalf("Smart State Engine must not trigger rescan/preflight path %q", forbidden)
		}
	}
}

func TestSmartStateEngineTracksActualFlashGotSubmission(t *testing.T) {
	b, err := os.ReadFile("web/smart_state_engine_v8568.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	markers := []string{
		"HTMLFormElement",
		"proto.submit = function()",
		"127.0.0.1:9666/flashgot",
		"rememberJDSubmission",
		"state: 'PENDING'",
		"state = 'COMPLETED'",
		"localStorage",
	}
	for _, marker := range markers {
		if !strings.Contains(s, marker) {
			t.Fatalf("JD transfer tracking missing marker %q", marker)
		}
	}
}

func TestSmartStateEngineLoadOrderPreservesJDOneShot(t *testing.T) {
	b, err := os.ReadFile("web/preview_quick_v86.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	fast := strings.Index(s, "/jdownloader_fast_v8567.js")
	state := strings.Index(s, "/smart_state_engine_v8569.js")
	picker := strings.Index(s, "/media_picker_v8566.js")
	legacy := strings.Index(s, "/jdownloader_batch_confirm_v8564.js")
	if fast < 0 || state < 0 || picker < 0 || legacy < 0 {
		t.Fatalf("expected JD/state/picker modules in bootstrap")
	}
	if !(fast < state && state < picker && picker < legacy) {
		t.Fatalf("unsafe module order: fast=%d state=%d picker=%d legacy=%d", fast, state, picker, legacy)
	}
}
