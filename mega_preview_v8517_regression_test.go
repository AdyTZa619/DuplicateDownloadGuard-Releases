package main

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestPreviewSnapshotDoesNotWaitBehindBusyMutexV8517(t *testing.T) {
	a := &App{}
	a.previewMu.Lock()
	started := time.Now()
	_ = a.snapshotMegaPreviewV8516()
	elapsed := time.Since(started)
	a.previewMu.Unlock()
	if elapsed > 100*time.Millisecond {
		t.Fatalf("snapshot-ul preview a așteptat prea mult după previewMu: %s", elapsed)
	}
	if elapsed < 15*time.Millisecond {
		t.Logf("snapshot busy returned quickly: %s", elapsed)
	}
}

func TestUICacheDoesNotWaitBehindBusyMutexV8517(t *testing.T) {
	a := &App{}
	a.previewMu.Lock()
	started := time.Now()
	_, _, ok := a.tryMegaPreviewUICacheV854(RemoteItem{Source: "MEGA", URL: "https://mega.example/folder", Path: "video.mp4"})
	elapsed := time.Since(started)
	a.previewMu.Unlock()
	if ok {
		t.Fatal("cache lookup nu trebuie să pretindă hit când starea este ocupată")
	}
	if elapsed > 25*time.Millisecond {
		t.Fatalf("cache lookup a blocat clickul pe previewMu: %s", elapsed)
	}
}

func TestIntentionalUIStopKeepsMegaWarmV8517(t *testing.T) {
	b, err := os.ReadFile("web/preview_quick_v86.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	start := strings.Index(s, "function installSoftRemoteStopV8517()")
	end := strings.Index(s, "function installSoftClearResultsV8517()")
	if start < 0 || end <= start {
		t.Fatal("wrapperul de stop soft v8.5.17 lipsește")
	}
	body := s[start:end]
	if strings.Contains(body, "/api/remote-preview/stop") {
		t.Fatal("Stop stream nu trebuie să dărâme sesiunea MEGA/WebDAV din backend")
	}
	if !strings.Contains(body, "markIntentionalRemoteAbortV8517()") {
		t.Fatal("Stop stream trebuie să dezarmeze onerror înainte de eliminarea media")
	}
	if !strings.Contains(s, "trueFallbackIDV858 = 0") || !strings.Contains(s, "trueFallbackAtV858 = 0") {
		t.Fatal("o selecție nouă trebuie să reactiveze fallback-ul real după un stop intenționat")
	}
}

func TestPreviewUIRequestIsLoggedBeforeStateProbeV8517(t *testing.T) {
	b, err := os.ReadFile("mega_preview_ui_fast_v854.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	start := strings.Index(s, "func (a *App) startMegaPreviewForUIV854")
	if start < 0 {
		t.Fatal("startMegaPreviewForUIV854 lipsă")
	}
	body := s[start:]
	request := strings.Index(body, "REQUEST START")
	probe := strings.Index(body, "trySnapshotMegaPreviewUIV8517")
	if request < 0 || probe < 0 || request > probe {
		t.Fatal("diagnosticul trebuie să marcheze cererea înainte de orice citire a stării preview")
	}
}
