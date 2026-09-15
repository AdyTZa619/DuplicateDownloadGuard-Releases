package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestJDownloaderBackendRechecksAndSendsOnlyFinalMissingV901(t *testing.T) {
	localData := bytes.Repeat([]byte("already-local"), 100)
	collection := t.TempDir()
	local := filepath.Join(collection, "renamed-local.bin")
	if err := os.WriteFile(local, localData, 0600); err != nil {
		t.Fatal(err)
	}
	remote := contentServer(localData)
	defer remote.Close()
	a := guardTestApp(t, collection, t.TempDir(), RemoteItem{})
	a.runIndex(context.Background(), []string{collection}, "", 0)
	a.results = []Result{
		{ID: 1, Remote: RemoteItem{Name: "unchecked-name.bin", Size: int64(len(localData)), Source: "BUNKR", DirectURL: remote.URL, URL: "https://bunkr.example/a/a", Handle: "have"}},
		{ID: 2, Remote: RemoteItem{Name: "actually-missing.bin", Size: 987654, Source: "BUNKR", URL: "https://bunkr.example/a/a", Handle: "missing"}},
		{ID: 3, Remote: RemoteItem{Name: "metadata-incomplete.mp4", Size: 0, ApproxSize: true, Source: "BUNKR", URL: "https://bunkr.example/a/a", Handle: "review"}},
	}
	var form url.Values
	jd := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		form, _ = url.ParseQuery(string(body))
		_, _ = w.Write([]byte("success"))
	}))
	defer jd.Close()
	oldBase := jdownloaderDirectBaseV8550
	jdownloaderDirectBaseV8550 = jd.URL
	defer func() { jdownloaderDirectBaseV8550 = oldBase }()
	body, _ := json.Marshal(map[string]any{"ids": []int{1, 2, 3}, "destination": t.TempDir(), "guardMode": "smart", "allowReview": true})
	w := httptest.NewRecorder()
	a.handleJDownloaderDirectEndpointV8551(w, httptest.NewRequest(http.MethodPost, "/api/download/jdownloader-direct", bytes.NewReader(body)))
	if w.Code != http.StatusOK {
		t.Fatalf("handoff failed: %d %s", w.Code, w.Body.String())
	}
	if got := form.Get("urls"); got != "https://bunkr.example/d/missing" {
		t.Fatalf("unchecked duplicate crossed JD boundary: %q", got)
	}
	var response struct {
		ExternalItems []jdownloaderExternalItemV901 `json:"externalItems"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.ExternalItems) != 1 || response.ExternalItems[0].ResultID != 2 || response.ExternalItems[0].URL != "https://bunkr.example/d/missing" {
		t.Fatalf("backend handoff journal is not the exact sent set: %#v", response.ExternalItems)
	}
	if row, _ := a.resultByID(1); row.GuardVerdict != guardDuplicate || row.LocalPath != local {
		t.Fatalf("duplicate not blocked by common detector: %#v", row)
	}
	if row, _ := a.resultByID(3); row.GuardVerdict != guardReview {
		t.Fatalf("incomplete remote metadata did not remain blocked for review: %#v", row)
	}
}

func TestJDownloaderFinalUIUsesGuardedBackendV901(t *testing.T) {
	b, err := os.ReadFile("web/jdownloader_window_capture_v8566.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, required := range []string{"/api/download/jdownloader-direct", "ddgJDownloaderGuardV901", "JSON.stringify(request)", "event.stopImmediatePropagation()"} {
		if !bytes.Contains(b, []byte(required)) {
			t.Fatalf("guarded JD route missing %q", required)
		}
	}
	for _, forbidden := range []string{"ddgJDownloaderFastV8566", "submitOneForm(", "currentReport(rows)", "allowReview: true", "/flashgot"} {
		if bytes.Contains(b, []byte(forbidden)) {
			t.Fatalf("JD UI retained bypass %q\n%s", forbidden, fmt.Sprintf("%.200s", s))
		}
	}
}

func TestEveryJDownloaderFrontendSenderDelegatesToCanonicalGuardV901(t *testing.T) {
	files := []string{
		"web/jdownloader_window_capture_v8566.js",
		"web/jdownloader_fast_v8567.js",
		"web/jdownloader_batch_confirm_v8564.js",
		"web/jdownloader_final_v8551.js",
		"web/download_actions_v8545.js",
		"web/exact_guard.js",
	}
	for _, name := range files {
		b, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		s := string(b)
		if !strings.Contains(s, "ddgJDownloaderGuardV901") {
			t.Fatalf("%s does not delegate to the canonical JD guard", name)
		}
		for _, forbidden := range []string{"/flashgot", "form.submit()", "submitOneForm(", "submitHiddenForm("} {
			if strings.Contains(s, forbidden) {
				t.Fatalf("%s retained direct JD bypass %q", name, forbidden)
			}
		}
	}
}

func TestJDownloaderBoundaryRejectsUndecoratedOrReviewDecisionV901(t *testing.T) {
	missing := decorateGuardDecision(DownloadGuardDecision{Verdict: guardDownload})
	if !jdownloaderDecisionAllowedV901(missing) {
		t.Fatal("fully decorated LIPSĂ decision was rejected")
	}
	for _, blocked := range []DownloadGuardDecision{
		{Verdict: guardDownload},
		decorateGuardDecision(DownloadGuardDecision{Verdict: guardReview}),
		decorateGuardDecision(DownloadGuardDecision{Verdict: guardDuplicate, Exact: true}),
	} {
		if jdownloaderDecisionAllowedV901(blocked) {
			t.Fatalf("non-LIPSĂ decision crossed JD boundary: %#v", blocked)
		}
	}
}

func TestJDownloaderBlocksMissingWhenConfiguredRootIsOfflineV901(t *testing.T) {
	collection, download := t.TempDir(), t.TempDir()
	a := guardTestApp(t, collection, download, RemoteItem{})
	a.cfg.LocalPaths = append(a.cfg.LocalPaths, filepath.Join(t.TempDir(), "offline-drive"))
	a.results = []Result{{ID: 1, Remote: RemoteItem{Name: "apparently-missing.bin", Size: 12345, Source: "BUNKR", URL: "https://bunkr.example/a/a", Handle: "missing"}}}
	var calls int
	jd := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_, _ = w.Write([]byte("success"))
	}))
	defer jd.Close()
	oldBase := jdownloaderDirectBaseV8550
	jdownloaderDirectBaseV8550 = jd.URL
	defer func() { jdownloaderDirectBaseV8550 = oldBase }()
	body, _ := json.Marshal(map[string]any{"ids": []int{1}, "destination": download, "guardMode": "smart"})
	w := httptest.NewRecorder()
	a.handleJDownloaderDirectEndpointV8551(w, httptest.NewRequest(http.MethodPost, "/api/download/jdownloader-direct", bytes.NewReader(body)))
	if w.Code != http.StatusOK || calls != 0 {
		t.Fatalf("offline collection crossed JD boundary: code=%d calls=%d body=%s", w.Code, calls, w.Body.String())
	}
	var response struct {
		ExternalAdded int                 `json:"externalAdded"`
		Guard         DownloadGuardReport `json:"guard"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.ExternalAdded != 0 || len(response.Guard.Decisions) != 1 || response.Guard.Decisions[0].Verdict != guardReview {
		t.Fatalf("offline root was not fail-closed: %#v", response)
	}
}

func TestReviewOverrideCannotEnterAnyDownloadQueueV901(t *testing.T) {
	a := guardTestApp(t, t.TempDir(), t.TempDir(), RemoteItem{})
	a.results = []Result{{ID: 1, Remote: RemoteItem{Name: "metadata-missing.mp4", Size: 0, ApproxSize: true, Source: "BUNKR"}}}
	body, _ := json.Marshal(map[string]any{"ids": []int{1}, "destination": t.TempDir(), "engine": "auto", "guardMode": "smart", "allowReview": true})
	w := httptest.NewRecorder()
	a.handleQueueAdd(w, httptest.NewRequest(http.MethodPost, "/api/queue/add", bytes.NewReader(body)))
	if w.Code != http.StatusOK {
		t.Fatalf("queue response: %d %s", w.Code, w.Body.String())
	}
	q := queueFor(a)
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.Jobs) != 0 {
		t.Fatalf("DE VERIFICAT entered download queue with override: %#v", q.Jobs)
	}
	if !bytes.Contains(w.Body.Bytes(), []byte(`"reviewOverride":false`)) {
		t.Fatalf("override was not rejected explicitly: %s", w.Body.String())
	}
}
