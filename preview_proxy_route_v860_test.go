package main

import (
	"os"
	"strings"
	"testing"
)

func TestMainRegistersMegaPreviewProxyV860(t *testing.T) {
	b, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	const route = `mux.HandleFunc("/api/remote-preview/proxy", a.handleRemotePreviewProxyV860)`
	if !strings.Contains(string(b), route) {
		t.Fatalf("MEGA preview proxy route missing from main.go")
	}
}
