package main

import (
	"strings"
	"testing"
)

func TestMediaQualityReasonPersistsRecommendation(t *testing.T) {
	remote := mediaQualityReason("remote")
	if !strings.Contains(remote, "Versiunea remote pare mai bună") || !strings.Contains(remote, actionRemoteBetter) {
		t.Fatalf("remote recommendation not persisted: %q", remote)
	}
	local := mediaQualityReason("local")
	if !strings.Contains(local, "Versiunea locală pare mai bună") || !strings.Contains(local, actionLocalBetter) {
		t.Fatalf("local recommendation not persisted: %q", local)
	}
	if got := mediaQualityReason(""); got != "" {
		t.Fatalf("empty quality hint should not invent a recommendation: %q", got)
	}
}
