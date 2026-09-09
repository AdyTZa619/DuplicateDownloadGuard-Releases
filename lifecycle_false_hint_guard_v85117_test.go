package main

import (
	"testing"
	"time"
)

func TestFalseExitHintClearsOnlyWhenWindowStillAliveV85117(t *testing.T) {
	now := time.Now()
	oldHint := now.Add(-uiFalseExitHintGraceV85117 - time.Second).UnixNano()
	freshHint := now.Add(-time.Second).UnixNano()

	if !clearFalseUIExitHintV85117(now, oldHint, true) {
		t.Fatal("old pagehide hint must be cleared when DDG window is still alive")
	}
	if clearFalseUIExitHintV85117(now, oldHint, false) {
		t.Fatal("real close candidate must keep its hint after native window disappears")
	}
	if clearFalseUIExitHintV85117(now, freshHint, true) {
		t.Fatal("fresh hint must receive the grace period before being classified as false")
	}
	if clearFalseUIExitHintV85117(now, 0, true) {
		t.Fatal("missing hint cannot be cleared")
	}
}
