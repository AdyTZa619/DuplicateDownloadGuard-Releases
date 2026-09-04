package main

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestPreviewDirectPathDoesNotHoldPreviewMutexAcrossMegaCommandsV8516(t *testing.T) {
	b, err := os.ReadFile("mega_preview_resume_direct_v858.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	start := strings.Index(s, "func (a *App) startMegaPreviewResumeDirectV858")
	if start < 0 {
		t.Fatal("startMegaPreviewResumeDirectV858 lipsă")
	}
	body := s[start:]
	if strings.Contains(body, "a.previewMu.Lock()\n\tdefer a.previewMu.Unlock()") {
		t.Fatal("previewMu nu trebuie ținut pe întreaga operație MEGAcmd")
	}
	if !strings.Contains(body, "old := a.snapshotMegaPreviewV8516()") {
		t.Fatal("traseul direct trebuie să folosească snapshot scurt al stării preview")
	}
	if !strings.Contains(body, "a.commitMegaPreviewV8516(") {
		t.Fatal("traseul direct trebuie să facă commit scurt al stării preview")
	}
}

func TestPreviewDiagnosticCallIsNonBlockingV8516(t *testing.T) {
	start := time.Now()
	for i := 0; i < 1000; i++ {
		megaPreviewDiagfV8514("NONBLOCK TEST %d", i)
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("diagnosticul preview a blocat apelantul prea mult: %s", elapsed)
	}
}
