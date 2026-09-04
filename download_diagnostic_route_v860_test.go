package main

import (
	"os"
	"strings"
	"testing"
)

func TestDownloadDiagnosticV860RegisteredAndLoaded(t *testing.T) {
	mainGo, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(mainGo), `mux.HandleFunc("/api/download/diagnostic", a.handleDownloadDiagnosticV860)`) {
		t.Fatal("download diagnostic API route missing")
	}
	index, err := os.ReadFile("web/index.html")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(index), `src="/download_diagnostic_v860.js"`) {
		t.Fatal("download diagnostic UI script not loaded")
	}
}
