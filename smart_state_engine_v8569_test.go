package main

import (
	"os"
	"strings"
	"testing"
)

func TestSmartStateV8569RebindsBestCurrentCandidate(t *testing.T) {
	b, err := os.ReadFile("web/smart_state_engine_v8569.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, marker := range []string{
		"/api/results/candidates?id=",
		"/api/results/candidate",
		"/api/results/smart-verify",
		"function candidatePriority",
		"function candidateIsStrong",
		"function rejectPair",
		"rejectedPairs",
		"sameSize",
		"exactCandidateName",
		"Reconciliază după JD",
		"găsit după JD • candidat nou confirmat",
	} {
		if !strings.Contains(s, marker) {
			t.Fatalf("Smart State v8569 missing marker %q", marker)
		}
	}
}

func TestSmartStateV8569IsEvidenceFirstAndConservative(t *testing.T) {
	b, err := os.ReadFile("web/smart_state_engine_v8569.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, marker := range []string{
		"if (!candidate?.path || !candidate.sameSize)",
		"if (!candidate?.sameSize) return false",
		"beforeManualStatus === 'DIFFERENT' && exactCurrentEvidence(updated)",
		"beforeManualStatus === 'MISSING' && candidateIsStrong",
		"candidateRows(row)",
		"candidatePriority(row,b,store) - candidatePriority(row,a,store)",
	} {
		if !strings.Contains(s, marker) {
			t.Fatalf("evidence-first guard missing marker %q", marker)
		}
	}
	for _, forbidden := range []string{
		"/api/download/preflight",
		"/api/index/start",
		"/api/mega/scan",
	} {
		if strings.Contains(s, forbidden) {
			t.Fatalf("Smart State v8569 must not trigger %s", forbidden)
		}
	}
}

func TestSmartStateV8569ReplacesV8568InBootstrap(t *testing.T) {
	b, err := os.ReadFile("web/preview_quick_v86.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, "/smart_state_engine_v8569.js") || !strings.Contains(s, "ddgSmartStateEngineV8569Script") {
		t.Fatal("v8569 Smart State engine is not loaded by preview bootstrap")
	}
	if strings.Contains(s, "/smart_state_engine_v8568.js") {
		t.Fatal("obsolete v8568 Smart State engine is still loaded")
	}
}

func TestSmartStateV8569DoesNotModifyVerifiedJDHandoff(t *testing.T) {
	b, err := os.ReadFile("web/jdownloader_fast_v8567.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, marker := range []string{
		"form.submit()",
		"params.set('urls'",
		"params.set('package'",
		"JDownloader — fără rescanare HDD",
	} {
		if !strings.Contains(s, marker) {
			t.Fatalf("verified JD handoff unexpectedly changed/missing marker %q", marker)
		}
	}
	for _, actualCall := range []string{
		"window.api('/api/download/preflight",
		"fetch('/api/download/preflight",
		"fetch(\"/api/download/preflight",
	} {
		if strings.Contains(s, actualCall) {
			t.Fatalf("verified JD handoff regressed to an actual preflight call: %q", actualCall)
		}
	}
}
