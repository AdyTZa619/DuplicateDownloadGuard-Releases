package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func guardTestApp(t *testing.T, collection, download string, remote RemoteItem) *App {
	t.Helper()
	a := &App{
		appDir:    t.TempDir(),
		index:     map[string]FileEntry{},
		bySize:    map[int64][]string{},
		byName:    map[string][]string{},
		decisions: map[string]Decision{},
		cfg: Config{
			LocalPaths:          []string{collection},
			DownloadDir:         download,
			DownloadGuardMode:   guardModeSmart,
			FullVerifyMaxMB:     12,
			SampleBlocks:        5,
			SampleBlockKB:       64,
			DownloadRetries:     1,
			DownloadConcurrency: 1,
		},
	}
	a.results = []Result{{ID: 1, Status: "MISSING", AutoStatus: "MISSING", Remote: remote}}
	return a
}

func contentServer(data []byte) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "remote.bin", fixedTime, bytes.NewReader(data))
	}))
}

func TestDownloadGuardBlocksRenamedExactDuplicateFromLiveDisk(t *testing.T) {
	data := bytes.Repeat([]byte("exact-content-"), 4000)
	server := contentServer(data)
	defer server.Close()
	collection, download := t.TempDir(), t.TempDir()
	local := filepath.Join(collection, "3756x6654_035cc4db82a2493862302a02a13f9024-D3558.jpg")
	if err := os.WriteFile(local, data, 0644); err != nil {
		t.Fatal(err)
	}
	remote := RemoteItem{Name: "3756x6654_035cc4db82a2493862302a02a13f9024.jpg", Path: "/pack/original.jpg", Size: int64(len(data)), Source: "HTTP", DirectURL: server.URL}
	a := guardTestApp(t, collection, download, remote)
	report, err := a.runDownloadGuard(context.Background(), a.results, download, guardModeSmart)
	if err != nil {
		t.Fatal(err)
	}
	decision := report.Decisions[0]
	if decision.Verdict != guardDuplicate || !decision.Exact || decision.LocalPath != local || decision.Method != "full-sha256" {
		t.Fatalf("unexpected decision: %#v", decision)
	}
	if a.results[0].Status != "VERIFIED" || a.results[0].MatchScore != 100 {
		t.Fatalf("result was not promoted to exact: %#v", a.results[0])
	}
}

func TestCompareRefreshesLiveIndexBeforeMissingVerdict(t *testing.T) {
	collection, download := t.TempDir(), t.TempDir()
	data := []byte("same-size-content-added-after-old-index")
	local := filepath.Join(collection, "original-D3558.jpg")
	if err := os.WriteFile(local, data, 0644); err != nil {
		t.Fatal(err)
	}
	remote := RemoteItem{Name: "original.jpg", Path: "/pack/original.jpg", Size: int64(len(data)), Source: "MEGA"}
	a := guardTestApp(t, collection, download, remote)
	a.cfg.LiveRefreshCompare = true
	a.compareRemote(context.Background(), []RemoteItem{remote}, "balanced")
	if len(a.results) != 1 {
		t.Fatalf("got %d results", len(a.results))
	}
	if got := a.results[0]; got.Status != "POSSIBLE" || got.LocalPath != local || got.Candidates != 1 {
		t.Fatalf("live file was not promoted out of MISSING: %#v", got)
	}
}

func TestSameSizeUnrelatedNameIsNotShownAsPossible(t *testing.T) {
	collection, download := t.TempDir(), t.TempDir()
	data := []byte("same-size-but-unrelated-content")
	if err := os.WriteFile(filepath.Join(collection, "taxe_companie_2021.bin"), data, 0644); err != nil {
		t.Fatal(err)
	}
	remote := RemoteItem{Name: "vacanta_mare_familie.jpg", Size: int64(len(data)), Source: "MEGA"}
	a := guardTestApp(t, collection, download, remote)
	a.cfg.LiveRefreshCompare = true
	a.compareRemote(context.Background(), []RemoteItem{remote}, "balanced")
	if got := a.results[0]; got.Status != "MISSING" || got.LocalPath != "" || got.Candidates != 1 {
		t.Fatalf("unrelated size-only candidate was presented as possible: %#v", got)
	}
}

func TestNameSimilarityRecognizesCollisionSuffixes(t *testing.T) {
	tests := [][2]string{
		{"original.jpg", "original-D3558.jpg"},
		{"3756x6654_035cc4db82a2493862302a02a13f9024.jpg", "3756x6654_035cc4db82a2493862302a02a13f9024-71A13.jpg"},
		{"holiday photo.png", "holiday photo (1).png"},
	}
	for _, pair := range tests {
		if score := nameSimilarity(pair[0], pair[1]); score < 95 {
			t.Errorf("nameSimilarity(%q, %q)=%d, want >=95", pair[0], pair[1], score)
		}
	}
	if score := nameSimilarity("taxe_companie_2021.bin", "vacanta_mare_familie.jpg"); score >= 55 {
		t.Fatalf("unrelated names scored %d, want <55", score)
	}
}

func TestDownloadGuardAllowsSameSizeWhenFullHashDiffers(t *testing.T) {
	remoteData := bytes.Repeat([]byte("A"), 256<<10)
	localData := bytes.Repeat([]byte("B"), len(remoteData))
	server := contentServer(remoteData)
	defer server.Close()
	collection, download := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(collection, "renamed.bin"), localData, 0644); err != nil {
		t.Fatal(err)
	}
	remote := RemoteItem{Name: "remote.bin", Size: int64(len(remoteData)), Source: "HTTP", DirectURL: server.URL}
	a := guardTestApp(t, collection, download, remote)
	report, err := a.runDownloadGuard(context.Background(), a.results, download, guardModeSmart)
	if err != nil {
		t.Fatal(err)
	}
	if got := report.Decisions[0]; got.Verdict != guardDownload || got.Method != "full-sha256" {
		t.Fatalf("unexpected decision: %#v", got)
	}
}

func TestDownloadGuardLargeMatchingSamplesStayReview(t *testing.T) {
	data := bytes.Repeat([]byte("0123456789abcdef"), 160000)
	server := contentServer(data)
	defer server.Close()
	collection, download := t.TempDir(), t.TempDir()
	local := filepath.Join(collection, "same-size-renamed.mp4")
	if err := os.WriteFile(local, data, 0644); err != nil {
		t.Fatal(err)
	}
	remote := RemoteItem{Name: "remote.mp4", Size: int64(len(data)), Source: "HTTP", DirectURL: server.URL}
	a := guardTestApp(t, collection, download, remote)
	a.cfg.FullVerifyMaxMB = 1
	report, err := a.runDownloadGuard(context.Background(), a.results, download, guardModeSmart)
	if err != nil {
		t.Fatal(err)
	}
	decision := report.Decisions[0]
	if decision.Verdict != guardReview || decision.Exact || decision.Method != "deterministic-samples" {
		t.Fatalf("unexpected decision: %#v", decision)
	}
	if a.results[0].Status != "SAMPLED" {
		t.Fatalf("sampled result should be review, got %#v", a.results[0])
	}
}

func TestDownloadGuardExactModeHashesLargeCandidate(t *testing.T) {
	data := bytes.Repeat([]byte("large-exact-content"), 140000)
	server := contentServer(data)
	defer server.Close()
	collection, download := t.TempDir(), t.TempDir()
	local := filepath.Join(collection, "large-renamed-video.mp4")
	if err := os.WriteFile(local, data, 0644); err != nil {
		t.Fatal(err)
	}
	remote := RemoteItem{Name: "large-original-video.mp4", Size: int64(len(data)), Source: "HTTP", DirectURL: server.URL}
	a := guardTestApp(t, collection, download, remote)
	a.cfg.FullVerifyMaxMB = 1
	report, err := a.runDownloadGuard(context.Background(), a.results, download, guardModeExact)
	if err != nil {
		t.Fatal(err)
	}
	if got := report.Decisions[0]; got.Verdict != guardDuplicate || !got.Exact || got.Method != "full-sha256" || got.LocalPath != local {
		t.Fatalf("exact mode did not prove the duplicate: %#v", got)
	}
}

func TestDownloadGuardLargeSampleMismatchConfirmsDifferent(t *testing.T) {
	remoteData := bytes.Repeat([]byte("R"), 2<<20)
	localData := append([]byte(nil), remoteData...)
	localData[0] = 'L'
	server := contentServer(remoteData)
	defer server.Close()
	collection, download := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(collection, "candidate.bin"), localData, 0644); err != nil {
		t.Fatal(err)
	}
	remote := RemoteItem{Name: "remote.bin", Size: int64(len(remoteData)), Source: "HTTP", DirectURL: server.URL}
	a := guardTestApp(t, collection, download, remote)
	a.cfg.FullVerifyMaxMB = 1
	report, err := a.runDownloadGuard(context.Background(), a.results, download, guardModeSmart)
	if err != nil {
		t.Fatal(err)
	}
	if got := report.Decisions[0]; got.Verdict != guardDownload || got.Method != "deterministic-samples" {
		t.Fatalf("unexpected decision: %#v", got)
	}
}

func TestDownloadGuardUsesPublishedHashWithoutRemoteTransfer(t *testing.T) {
	data := []byte("same bytes under another filename")
	sum := sha256.Sum256(data)
	collection, download := t.TempDir(), t.TempDir()
	local := filepath.Join(collection, "another-name.dat")
	if err := os.WriteFile(local, data, 0644); err != nil {
		t.Fatal(err)
	}
	remote := RemoteItem{Name: "remote.dat", Size: int64(len(data)), Source: "HTTP", DirectURL: "http://127.0.0.1:1/unreachable", HashType: "sha256", Hash: hex.EncodeToString(sum[:])}
	a := guardTestApp(t, collection, download, remote)
	report, err := a.runDownloadGuard(context.Background(), a.results, download, guardModeSmart)
	if err != nil {
		t.Fatal(err)
	}
	if got := report.Decisions[0]; got.Verdict != guardDuplicate || got.Method != "remote-hash" || got.LocalPath != local {
		t.Fatalf("unexpected decision: %#v", got)
	}
}

func TestDownloadGuardRespectsManualHaveWhenBytesDiffer(t *testing.T) {
	remoteData := bytes.Repeat([]byte("R"), 128<<10)
	localData := bytes.Repeat([]byte("L"), len(remoteData))
	server := contentServer(remoteData)
	defer server.Close()
	collection, download := t.TempDir(), t.TempDir()
	local := filepath.Join(collection, "manual-candidate.bin")
	if err := os.WriteFile(local, localData, 0644); err != nil {
		t.Fatal(err)
	}
	remote := RemoteItem{Name: "remote.bin", Size: int64(len(remoteData)), Source: "HTTP", DirectURL: server.URL}
	a := guardTestApp(t, collection, download, remote)
	a.results[0].Manual = true
	a.results[0].Status = "HAVE"
	a.results[0].LocalPath = local
	report, err := a.runDownloadGuard(context.Background(), a.results, download, guardModeSmart)
	if err != nil {
		t.Fatal(err)
	}
	if got := report.Decisions[0]; got.Verdict != guardReview || got.Method != "manual-have" {
		t.Fatalf("manual HAVE was bypassed: %#v", got)
	}
}

func TestQueueAddNeverQueuesExactDuplicate(t *testing.T) {
	data := bytes.Repeat([]byte("duplicate"), 1000)
	server := contentServer(data)
	defer server.Close()
	collection, download := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(collection, "different-name.jpg"), data, 0644); err != nil {
		t.Fatal(err)
	}
	remote := RemoteItem{Name: "remote.jpg", Size: int64(len(data)), Source: "HTTP", DirectURL: server.URL}
	a := guardTestApp(t, collection, download, remote)
	body := strings.NewReader(`{"ids":[1],"destination":"` + strings.ReplaceAll(download, `\`, `\\`) + `","engine":"internal"}`)
	recorder := httptest.NewRecorder()
	a.handleQueueAdd(recorder, httptest.NewRequest(http.MethodPost, "/api/queue/add", body))
	if recorder.Code != http.StatusOK {
		t.Fatalf("queue add failed: %d %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Added int                 `json:"added"`
		Guard DownloadGuardReport `json:"guard"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Added != 0 || response.Guard.Counts[guardDuplicate] != 1 {
		t.Fatalf("duplicate reached queue: %s", recorder.Body.String())
	}
	if jobs := queueFor(a).snapshot(); len(jobs) != 0 {
		t.Fatalf("unexpected jobs: %#v", jobs)
	}
}

func TestRecoveredQueueJobIsRecheckedBeforeDownloaderStarts(t *testing.T) {
	data := bytes.Repeat([]byte("recovered-duplicate"), 4000)
	server := contentServer(data)
	defer server.Close()
	collection, download := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(collection, "local-renamed.bin"), data, 0644); err != nil {
		t.Fatal(err)
	}
	remote := RemoteItem{Name: "remote-original.bin", Size: int64(len(data)), Source: "HTTP", DirectURL: server.URL}
	a := guardTestApp(t, collection, download, remote)
	q := &DownloadQueue{
		Jobs: []*DownloadJob{{
			ID: "old-job", ResultID: 1, Name: remote.Name, Source: remote.Source,
			URL: server.URL, Destination: download, Engine: "internal", Status: "running",
			GuardVersion: 0, AddedAt: 1, UpdatedAt: 1,
		}},
		Cancels: map[string]context.CancelFunc{},
		Started: true,
	}
	q.runJob(a, "old-job", context.Background())
	jobs := q.snapshot()
	if len(jobs) != 1 || jobs[0].Status != "blocked" || jobs[0].GuardVerdict != guardDuplicate || jobs[0].BytesDone != 0 {
		t.Fatalf("recovered job bypassed ExactGuard: %#v", jobs)
	}
	if _, err := os.Stat(filepath.Join(download, remote.Name)); !os.IsNotExist(err) {
		t.Fatalf("downloader wrote a file before the guard: %v", err)
	}
}

func TestQueueAddKeepsSampleMatchOutOfQueueUntilOverride(t *testing.T) {
	data := bytes.Repeat([]byte("sample-match-content"), 120000)
	server := contentServer(data)
	defer server.Close()
	collection, download := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(collection, "renamed-large.bin"), data, 0644); err != nil {
		t.Fatal(err)
	}
	remote := RemoteItem{Name: "remote-large.bin", Size: int64(len(data)), Source: "HTTP", DirectURL: server.URL}
	a := guardTestApp(t, collection, download, remote)
	a.cfg.FullVerifyMaxMB = 1
	body := strings.NewReader(`{"ids":[1],"destination":"` + strings.ReplaceAll(download, `\`, `\\`) + `","engine":"internal"}`)
	recorder := httptest.NewRecorder()
	a.handleQueueAdd(recorder, httptest.NewRequest(http.MethodPost, "/api/queue/add", body))
	if recorder.Code != http.StatusOK {
		t.Fatalf("queue add failed: %d %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Added int                 `json:"added"`
		Guard DownloadGuardReport `json:"guard"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Added != 0 || response.Guard.Counts[guardReview] != 1 {
		t.Fatalf("ambiguous sample match reached queue: %s", recorder.Body.String())
	}
	if jobs := queueFor(a).snapshot(); len(jobs) != 0 {
		t.Fatalf("unexpected jobs: %#v", jobs)
	}
}

func TestQueuePauseAllWorksWithoutSelection(t *testing.T) {
	a := &App{appDir: t.TempDir(), index: map[string]FileEntry{}, bySize: map[int64][]string{}, byName: map[string][]string{}, decisions: map[string]Decision{}}
	q := &DownloadQueue{Jobs: []*DownloadJob{{ID: "1", Status: "queued"}, {ID: "2", Status: "running"}}, Cancels: map[string]context.CancelFunc{}, Started: true}
	queueRegistry.Store(a, q)
	defer queueRegistry.Delete(a)
	recorder := httptest.NewRecorder()
	a.handleQueueAction(recorder, httptest.NewRequest(http.MethodPost, "/api/queue/action", strings.NewReader(`{"action":"pause-all","ids":[]}`)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("pause-all failed: %d %s", recorder.Code, recorder.Body.String())
	}
	for _, job := range q.snapshot() {
		if job.Status != "paused" || job.GuardVersion != 0 {
			t.Fatalf("job was not safely paused: %#v", job)
		}
	}
}

func TestQueueStopAllWorksWithoutSelection(t *testing.T) {
	a := &App{appDir: t.TempDir(), index: map[string]FileEntry{}, bySize: map[int64][]string{}, byName: map[string][]string{}, decisions: map[string]Decision{}}
	q := &DownloadQueue{Jobs: []*DownloadJob{{ID: "1", Status: "queued"}, {ID: "2", Status: "running"}, {ID: "3", Status: "paused"}}, Cancels: map[string]context.CancelFunc{}, Started: true}
	queueRegistry.Store(a, q)
	defer queueRegistry.Delete(a)
	recorder := httptest.NewRecorder()
	a.handleQueueAction(recorder, httptest.NewRequest(http.MethodPost, "/api/queue/action", strings.NewReader(`{"action":"stop-all","ids":[]}`)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("stop-all failed: %d %s", recorder.Code, recorder.Body.String())
	}
	for _, job := range q.snapshot() {
		if job.Status != "cancelled" {
			t.Fatalf("job was not stopped: %#v", job)
		}
	}
}
