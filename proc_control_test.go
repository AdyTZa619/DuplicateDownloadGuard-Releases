package main

import (
	"os"
	"strings"
	"testing"
)

func TestMegaControlWindowNeverKillsProcessTreeV8528(t *testing.T) {
	b, err := os.ReadFile("proc_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(b)
	start := strings.Index(source, "func hideControlWindow")
	if start < 0 {
		t.Fatal("hideControlWindow lipsește din implementarea Windows")
	}
	end := strings.Index(source[start:], "\n}\n")
	if end < 0 {
		t.Fatal("corpul hideControlWindow nu poate fi delimitat")
	}
	body := source[start : start+end]
	if strings.Contains(body, "terminateProcessTree") || strings.Contains(body, "taskkill") || strings.Contains(body, ".Cancel") {
		t.Fatal("comanda de control MegaClient încă poate omorî arborele serverului MEGAcmd")
	}
}
