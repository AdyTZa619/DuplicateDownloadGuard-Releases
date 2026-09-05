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

func TestPipeRecoveryTargetsOnlyMEGAcmdServerV8529(t *testing.T) {
	b, err := os.ReadFile("mega_control_recovery_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(b)
	if !strings.Contains(source, `"taskkill.exe", "/F", "/IM", "MEGAcmdServer.exe"`) {
		t.Fatal("recuperarea nu țintește explicit serviciul MEGAcmdServer")
	}
	if strings.Contains(source, `"/IM", "MegaClient.exe"`) || strings.Contains(source, `"/T"`) {
		t.Fatal("recuperarea nu trebuie să omoare MegaClient sau un arbore de procese")
	}
}
