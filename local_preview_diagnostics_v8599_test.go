package main

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testLocalPreviewAppV8599(t *testing.T) (*App, string, []byte) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "probe.jpg")
	body := []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00, 0x01, 0xff, 0xd9}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	app := &App{
		index:  map[string]FileEntry{path: {Path: path, Name: filepath.Base(path), Size: int64(len(body))}},
		appDir: dir,
	}
	t.Cleanup(func() {
		if !flushLocalPreviewLogsV85130(3 * time.Second) {
			t.Error("local preview diagnostic queue did not drain")
		}
		localPreviewRootSnapshotsV85102.Delete(app)
	})
	return app, path, body
}

func TestLocalPreviewDirectDiagnosticServesOriginalV8599(t *testing.T) {
	a, path, want := testLocalPreviewAppV8599(t)
	req := httptest.NewRequest(http.MethodGet, "/api/local-preview?path="+url.QueryEscape(path), nil)
	rr := httptest.NewRecorder()
	a.handleLocalPreviewDiagV8599(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rr.Code, rr.Body.String())
	}
	if !bytes.Equal(rr.Body.Bytes(), want) {
		t.Fatalf("direct preview changed bytes: got=%v want=%v", rr.Body.Bytes(), want)
	}
}

func TestLocalPreviewBufferedAlternativeServesImageV8599(t *testing.T) {
	a, path, want := testLocalPreviewAppV8599(t)
	req := httptest.NewRequest(http.MethodGet, "/api/local-preview-buffered?path="+url.QueryEscape(path), nil)
	rr := httptest.NewRecorder()
	a.handleLocalPreviewBufferedV8599(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%q", rr.Code, rr.Body.String())
	}
	if !bytes.Equal(rr.Body.Bytes(), want) {
		t.Fatalf("buffered preview changed bytes: got=%v want=%v", rr.Body.Bytes(), want)
	}
}

func TestLocalPreviewFastAuthDoesNotWaitForGlobalWriterV85102(t *testing.T) {
	appDir := t.TempDir()
	root := t.TempDir()
	path := filepath.Join(root, "slow-before.jpg")
	if err := os.WriteFile(path, []byte{0xff, 0xd8, 0xff, 0xd9}, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := fmt.Sprintf(`{"localPaths":[%q],"downloadDir":""}`, root)
	if err := os.WriteFile(filepath.Join(appDir, "config.json"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	a := &App{appDir: appDir, index: map[string]FileEntry{}}

	a.mu.Lock()
	started := time.Now()
	allowed := a.localPreviewPathAllowedV85102(path)
	elapsed := time.Since(started)
	a.mu.Unlock()

	if !allowed {
		t.Fatal("configured LOCAL root should authorize without App.mu")
	}
	if elapsed > 250*time.Millisecond {
		t.Fatalf("preview authorization waited behind App.mu: %v", elapsed)
	}
}

func TestLocalPreviewDiagnosticsStayIsolatedV8599(t *testing.T) {
	goSrc, err := os.ReadFile("local_preview_diagnostics_v8599.go")
	if err != nil {
		t.Fatal(err)
	}
	fastSrc, err := os.ReadFile("local_preview_fastpath_v85102.go")
	if err != nil {
		t.Fatal(err)
	}
	js, err := os.ReadFile(filepath.Join("web", "local_preview_diagnostics_v8599.js"))
	if err != nil {
		t.Fatal(err)
	}
	boot, err := os.ReadFile(filepath.Join("web", "preview_quick_v86.js"))
	if err != nil {
		t.Fatal(err)
	}
	workflow, err := os.ReadFile(filepath.Join(".github", "workflows", "build-test.yml"))
	if err != nil {
		t.Fatal(err)
	}

	for _, marker := range []string{"LOCAL PREVIEW diag DIRECT", "first-byte=", "LOCAL PREVIEW diag BUFFER", "read=%dms", "localPreviewPathAllowedV85102"} {
		if !strings.Contains(string(goSrc), marker) {
			t.Fatalf("backend diagnostic marker missing: %q", marker)
		}
	}
	for _, marker := range []string{"TryRLock", "localPreviewLogfV85102", "appendLocalPreviewJournalV85104", "a.mu.TryLock()", "dedicated-direct-local-media"} {
		if !strings.Contains(string(fastSrc), marker) {
			t.Fatalf("fast-path marker missing: %q", marker)
		}
	}
	if strings.Contains(string(fastSrc), "entry.a.logf") {
		t.Fatal("LOCAL diagnostic worker must never queue App.logf/App.mu writer")
	}
	for _, marker := range []string{"PENDING_MS = 1500", "resourceTiming", "queue=", "ALT • citește întâi în memorie", "/api/local-preview-buffered", "button.addEventListener('click'", "BASE_URL", "resolveDedicatedBase", "routeLocalElement", "ddgLocalDedicatedV85104", "DEDICATED"} {
		if !strings.Contains(string(js), marker) {
			t.Fatalf("client diagnostic marker missing: %q", marker)
		}
	}
	if !strings.Contains(string(boot), "/local_preview_diagnostics_v8599.js") {
		t.Fatal("diagnostic JS is not loaded by preview bootstrap")
	}
	if !strings.Contains(string(workflow), "registerLocalPreviewDiagnosticsV8599(mux, a)") {
		t.Fatal("TEST build does not wire the diagnostic local-preview route")
	}

	combined := string(goSrc) + "\n" + string(fastSrc) + "\n" + string(js)
	for _, forbidden := range []string{"/api/index/start", "/api/mega/scan", "/api/download/preflight"} {
		if strings.Contains(combined, forbidden) {
			t.Fatalf("local preview fast path must not invoke %s", forbidden)
		}
	}
	for _, forbidden := range []string{"window.localPreviewHTML", "loadRemotePreview", "remotePreview", "detailSeq", "currentRow", "megaPreview"} {
		if strings.Contains(string(js), forbidden) {
			t.Fatalf("local preview UI must not touch shared/remote selection state: %s", forbidden)
		}
	}
	if strings.Contains(string(js), "setTimeout(() => { img.src") {
		t.Fatal("there must be no timeout-driven competing image request")
	}
}
