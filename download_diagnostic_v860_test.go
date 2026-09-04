package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunLocalDownloadSelfTestV860(t *testing.T) {
	dir := t.TempDir()
	a := &App{appDir: dir}
	detail, err := runLocalDownloadSelfTestV860(a)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(detail, "Referer OK") || !strings.Contains(detail, "integritate OK") {
		t.Fatalf("unexpected detail: %s", detail)
	}
}

func TestProbeDirectDownloadV860SendsReferer(t *testing.T) {
	var gotRef string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRef = r.Header.Get("Referer")
		if r.Header.Get("Range") != "bytes=0-0" {
			t.Fatalf("missing range: %q", r.Header.Get("Range"))
		}
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Accept-Ranges", "bytes")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte{0})
	}))
	defer s.Close()

	res := Result{Remote: RemoteItem{Source: "HTML", URL: "https://site.example/page", DirectURL: s.URL + "/a.mp4", Name: "a.mp4"}}
	if _, err := probeDirectDownloadV860(context.Background(), res); err != nil {
		t.Fatal(err)
	}
	if gotRef != "https://site.example/page" {
		t.Fatalf("referer=%q", gotRef)
	}
}

func TestProbeDirectDownloadV860RejectsHTML(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html>login</html>"))
	}))
	defer s.Close()
	res := Result{Remote: RemoteItem{Source: "HTTP", URL: s.URL, DirectURL: s.URL, Name: "video.mp4"}}
	if _, err := probeDirectDownloadV860(context.Background(), res); err == nil {
		t.Fatal("expected HTML rejection")
	}
}

func TestDownloadDiagnosticReportV860(t *testing.T) {
	dir := t.TempDir()
	a := &App{appDir: dir, results: []Result{{ID: 11, Remote: RemoteItem{Source: "HTTP", URL: "http://127.0.0.1:1/a.bin", DirectURL: "http://127.0.0.1:1/a.bin", Name: "a.bin"}}}}
	report := a.runDownloadDiagnosticV860(context.Background(), 11, false)
	if report.ResultID != 11 || report.Engine != "internal" {
		t.Fatalf("report=%#v", report)
	}
	foundCore := false
	for _, c := range report.Checks {
		if strings.Contains(c.Name, "Referer + resume") {
			foundCore = true
			if c.Status != "pass" {
				t.Fatalf("core check failed: %#v", c)
			}
		}
	}
	if !foundCore {
		t.Fatal("core self-test missing")
	}
}

func TestDiagnosticTempFilesCleanedV860(t *testing.T) {
	dir := t.TempDir()
	a := &App{appDir: dir}
	if _, err := runLocalDownloadSelfTestV860(a); err != nil {
		t.Fatal(err)
	}
	matches, err := filepath.Glob(filepath.Join(dir, "download-diagnostic-*"))
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range matches {
		if _, err := os.Stat(m); err == nil {
			t.Fatalf("diagnostic temp dir left behind: %s", m)
		}
	}
}
