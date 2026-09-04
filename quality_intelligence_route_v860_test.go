package main

import (
	"os"
	"strings"
	"testing"
)

func TestQualityIntelligenceV860RegisteredAndLoaded(t *testing.T) {
	mainGo, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(mainGo), `mux.HandleFunc("/api/media/quality", a.handleMediaQualityV860)`) {
		t.Fatal("quality intelligence API route missing")
	}
	index, err := os.ReadFile("web/index.html")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(index), `src="/quality_intelligence_v860.js"`) {
		t.Fatal("quality intelligence UI script not loaded")
	}
}
