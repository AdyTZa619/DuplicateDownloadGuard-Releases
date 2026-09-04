package main

import (
	"strings"
	"testing"
)

func TestOperationHUDV851IsEmbeddedAndCompatible(t *testing.T) {
	b, err := webFS.ReadFile("web/exact_guard.js")
	if err != nil {
		t.Fatalf("read embedded UI: %v", err)
	}
	s := string(b)
	for _, required := range []string{
		"Operation HUD v8.5.1",
		"operationHudPanel",
		"operationHudProgressBar",
		"/api/status",
		`id=\"topStatus\"`,
		"Monitor operațional live",
	} {
		if !strings.Contains(s, required) {
			t.Fatalf("operation HUD missing required marker %q", required)
		}
	}
}
