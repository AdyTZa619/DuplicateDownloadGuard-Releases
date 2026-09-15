package main

import "testing"

func TestPendingDetectorRowsAreNotFinalReviewV901(t *testing.T) {
	pending := Result{
		ID: 1, Status: "POSSIBLE", AutoStatus: "POSSIBLE",
		Detector: &DuplicateEvidenceV90{Classification: "DE VERIFICAT"},
		Remote:   RemoteItem{Name: "pending.mp4", Size: 100},
	}
	completed := Result{
		ID: 2, Status: "POSSIBLE", AutoStatus: "POSSIBLE", GuardAt: 123, GuardVerdict: guardReview,
		Detector: &DuplicateEvidenceV90{Classification: "DE VERIFICAT"},
		Remote:   RemoteItem{Name: "review.mp4", Size: 200},
	}
	rows := make([]Result, 0, 76)
	for i := 0; i < 75; i++ {
		row := pending
		row.ID = i + 1
		rows = append(rows, row)
	}
	rows = append(rows, completed)

	if !resultAnalysisPendingV901(pending) || resultPendingReview(pending) {
		t.Fatalf("queued detector row was presented as final review: %#v", pending)
	}
	if resultAnalysisPendingV901(completed) || !resultPendingReview(completed) {
		t.Fatalf("completed ambiguous row did not become final review: %#v", completed)
	}
	if !resultMatchesFilter(pending, "", "ANALYZING") || resultMatchesFilter(pending, "", "REVIEW") {
		t.Fatal("pending detector filters overlap")
	}

	summary := buildResultSummary(rows)
	workflow := summary["workflow"].(map[string]int)
	decision := summary["decision"].(map[string]int)
	if workflow["ANALYZING"] != 75 || workflow["REVIEW"] != 1 {
		t.Fatalf("1/76 detector progress was summarized incorrectly: %#v", workflow)
	}
	if decision["ANALYZING"] != 75 || decision["REVIEW"] != 1 {
		t.Fatalf("pending/final decision buckets overlap: %#v", decision)
	}
}

func TestMissingFilterRequiresCompletedFinalDetectorVerdictV901(t *testing.T) {
	provisional := Result{
		Status: "MISSING", AutoStatus: "MISSING",
		Detector: &DuplicateEvidenceV90{Classification: "DE VERIFICAT"},
	}
	confirmed := Result{
		Status: "MISSING", AutoStatus: "MISSING", GuardAt: 123, GuardVerdict: guardDownload,
		Detector: &DuplicateEvidenceV90{Classification: "LIPSĂ"},
	}
	if resultMatchesFilter(provisional, "", "MISSING") || resultConfirmedMissingV901(provisional) {
		t.Fatal("provisional MISSING leaked into the final missing filter")
	}
	if smartRuleMatch(provisional, "missing") {
		t.Fatal("smart selection treated provisional MISSING as final")
	}
	if !resultMatchesFilter(confirmed, "", "MISSING") || !resultConfirmedMissingV901(confirmed) {
		t.Fatal("completed LIPSĂ was not exposed by the final missing filter")
	}
	if !smartRuleMatch(confirmed, "missing") {
		t.Fatal("smart selection omitted completed LIPSĂ")
	}
}
