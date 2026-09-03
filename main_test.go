package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseMegaLong(t *testing.T) {
	sample := `FLAGS VERS    SIZE            DATE       NAME
----    1      734289 2026-09-03T07:10:11 /pack/Images/photo one.jpg
----    1   123456789 2026-09-03T07:10:12 /pack/Videos/video.mp4
`
	got := parseMegaLong(sample, "MEGA", "x")
	if len(got) != 2 {
		t.Fatalf("want 2, got %d: %#v", len(got), got)
	}
	if got[0].Size != 734289 || got[0].Path != "/pack/Images/photo one.jpg" {
		t.Fatalf("bad first: %#v", got[0])
	}
}

func TestCompareBasic(t *testing.T) {
	p := filepath.Join("C:\\", "media", "a.jpg")
	a := &App{appDir: t.TempDir(), index: map[string]FileEntry{p: {Path: p, Name: "a.jpg", Size: 100}}, bySize: map[int64][]string{100: {p}}, byName: map[string][]string{"a.jpg": {p}}, cfg: Config{Mode: "balanced"}}
	a.compareRemote(context.Background(), []RemoteItem{{Path: "/x/a.jpg", Name: "a.jpg", Size: 100, Source: "MEGA"}}, "balanced")
	if len(a.results) != 1 || a.results[0].Status != "HAVE" {
		t.Fatalf("unexpected: %#v", a.results)
	}
}

func TestParseMegaHandleAndDirectURL(t *testing.T) {
	sample := `FLAGS VERS    SIZE            DATE       NAME
----    1   123456789 2026-09-03T07:10:12 H:bnwmwZ7K /pack/Videos/video.mp4
`
	got := parseMegaLong(sample, "MEGA", "https://mega.nz/folder/jjJRGCwS#KEY")
	if len(got) != 1 {
		t.Fatalf("want 1, got %d: %#v", len(got), got)
	}
	if got[0].Handle != "bnwmwZ7K" {
		t.Fatalf("handle not parsed: %#v", got[0])
	}
	u := megaItemURL(got[0].URL, got[0].Handle)
	want := "https://mega.nz/folder/jjJRGCwS#KEY/file/bnwmwZ7K"
	if u != want {
		t.Fatalf("want %q got %q", want, u)
	}
}

func TestExtractWebDAVURL(t *testing.T) {
	out := "Serving via webdav /pack/Videos/video.mp4: http://127.0.0.1:4443/AbCdEf12/video.mp4\n"
	got := extractWebDAVURL(out, "/pack/Videos/video.mp4")
	want := "http://127.0.0.1:4443/AbCdEf12/video.mp4"
	if got != want {
		t.Fatalf("want %q got %q", want, got)
	}
}

func TestRemoteMediaKind(t *testing.T) {
	cases := map[string]string{
		"a.JPG":  "image",
		"b.mp4":  "video",
		"c.MKV":  "video",
		"d.flac": "audio",
		"e.zip":  "other",
	}
	for name, want := range cases {
		if got := remoteMediaKind(name); got != want {
			t.Fatalf("%s: want %s got %s", name, want, got)
		}
	}
}

func TestMegaRemoteRefPrefersHandle(t *testing.T) {
	item := RemoteItem{Path: "video.mp4", Handle: "AbCdEf12"}
	if got := megaRemoteRef(item); got != "H:AbCdEf12" {
		t.Fatalf("want handle ref, got %q", got)
	}
	item.Handle = ""
	item.Path = "/pack/Videos/video.mp4"
	if got := megaRemoteRef(item); got != "/pack/Videos/video.mp4" {
		t.Fatalf("want path fallback, got %q", got)
	}
}

func TestExtractWebDAVURLWithHandleRef(t *testing.T) {
	out := "Serving via webdav H:AbCdEf12: http://127.0.0.1:4443/AbCdEf12/video.mp4\n"
	got := extractWebDAVURL(out, "H:AbCdEf12")
	want := "http://127.0.0.1:4443/AbCdEf12/video.mp4"
	if got != want {
		t.Fatalf("want %q got %q", want, got)
	}
}

func TestNameSimilarityRenamed(t *testing.T) {
	cases := []struct {
		a, b string
		min  int
	}{
		{"1920x3410_0e905198575d63a8d9e7_source.mp4", "2021-11-05_1920x3410_0e905198575d63a8d9e7.mp4", 70},
		{"holiday_clip_final.mp4", "holiday clip.mp4", 75},
		{"completely_different.mp4", "other_file.mp4", 0},
	}
	for _, c := range cases {
		if got := nameSimilarity(c.a, c.b); got < c.min {
			t.Fatalf("similarity %q/%q = %d, want >= %d", c.a, c.b, got, c.min)
		}
	}
}

func TestSampleRanges(t *testing.T) {
	r := sampleRanges(100<<20, 256<<10, 5)
	if len(r) != 5 {
		t.Fatalf("want 5 ranges, got %d", len(r))
	}
	if r[0][0] != 0 {
		t.Fatalf("first range should start at 0: %#v", r[0])
	}
	if r[len(r)-1][1] != (100<<20)-1 {
		t.Fatalf("last range should reach EOF: %#v", r[len(r)-1])
	}
	r = sampleRanges(512<<10, 256<<10, 5)
	if len(r) != 1 || r[0][0] != 0 || r[0][1] != (512<<10)-1 {
		t.Fatalf("small file should be full range: %#v", r)
	}
}

func TestFetchHTTPRange(t *testing.T) {
	data := []byte("0123456789abcdefghijklmnopqrstuvwxyz")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "x.bin", time.Unix(0, 0), strings.NewReader(string(data)))
	}))
	defer srv.Close()
	got, err := fetchHTTPRange(context.Background(), srv.URL, 10, 19)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(data[10:20]) {
		t.Fatalf("got %q want %q", got, data[10:20])
	}
}

func TestCandidateRankingPrefersSameSizeAndName(t *testing.T) {
	r := RemoteItem{Name: "2026-09-03_1920x1080_abcdef123456_source.mp4", Size: 1000}
	a := rankCandidate(r, FileEntry{Path: "A", Name: "abcdef123456.mp4", Size: 1000})
	b := rankCandidate(r, FileEntry{Path: "B", Name: "totally_other.mp4", Size: 1000})
	if a.Rank <= b.Rank || a.NameScore <= b.NameScore {
		t.Fatalf("expected renamed candidate to rank higher: a=%#v b=%#v", a, b)
	}
}

func TestManualDecisionPersistsAndRestoresAuto(t *testing.T) {
	a := &App{
		appDir:    t.TempDir(),
		index:     map[string]FileEntry{},
		bySize:    map[int64][]string{},
		byName:    map[string][]string{},
		decisions: map[string]Decision{},
		cfg:       Config{Mode: "balanced"},
	}
	remote := RemoteItem{Path: "/pack/x.mp4", Name: "x.mp4", Size: 1234, Source: "MEGA", URL: "https://mega.nz/folder/test#key", Handle: "AbCdEf12"}
	a.compareRemote(context.Background(), []RemoteItem{remote}, "balanced")
	if got := a.results[0].Status; got != "MISSING" {
		t.Fatalf("initial status = %s, want MISSING", got)
	}

	body := strings.NewReader(`{"ids":[1],"status":"HAVE","note":"verificat vizual"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/results/mark", body)
	rr := httptest.NewRecorder()
	a.handleMark(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("mark failed: %d %s", rr.Code, rr.Body.String())
	}
	if !a.results[0].Manual || a.results[0].Status != "HAVE" || a.results[0].AutoStatus != "MISSING" {
		t.Fatalf("manual mark not separated from auto: %#v", a.results[0])
	}
	if len(a.decisions) != 1 {
		t.Fatalf("decision not persisted in memory: %#v", a.decisions)
	}

	// A fresh comparison of the same remote node must reuse the user's decision.
	a.compareRemote(context.Background(), []RemoteItem{remote}, "balanced")
	if !a.results[0].Manual || a.results[0].Status != "HAVE" || a.results[0].AutoStatus != "MISSING" {
		t.Fatalf("persistent decision not applied: %#v", a.results[0])
	}

	body = strings.NewReader(`{"ids":[1],"status":"AUTO"}`)
	req = httptest.NewRequest(http.MethodPost, "/api/results/mark", body)
	rr = httptest.NewRecorder()
	a.handleMark(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("restore auto failed: %d %s", rr.Code, rr.Body.String())
	}
	if a.results[0].Manual || a.results[0].Status != "MISSING" || len(a.decisions) != 0 {
		t.Fatalf("auto verdict not restored: result=%#v decisions=%#v", a.results[0], a.decisions)
	}
}

func TestUndoManualMark(t *testing.T) {
	a := &App{
		appDir:    t.TempDir(),
		index:     map[string]FileEntry{},
		bySize:    map[int64][]string{},
		byName:    map[string][]string{},
		decisions: map[string]Decision{},
		cfg:       Config{Mode: "balanced"},
	}
	remote := RemoteItem{Path: "/pack/y.jpg", Name: "y.jpg", Size: 22, Source: "MEGA", URL: "u", Handle: "h"}
	a.compareRemote(context.Background(), []RemoteItem{remote}, "balanced")
	rr := httptest.NewRecorder()
	a.handleMark(rr, httptest.NewRequest(http.MethodPost, "/api/results/mark", strings.NewReader(`{"ids":[1],"status":"HAVE"}`)))
	if a.results[0].Status != "HAVE" {
		t.Fatal("mark did not apply")
	}
	rr = httptest.NewRecorder()
	a.handleUndoMark(rr, httptest.NewRequest(http.MethodPost, "/api/results/undo-mark", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("undo failed: %d %s", rr.Code, rr.Body.String())
	}
	if a.results[0].Status != "MISSING" || a.results[0].Manual || len(a.decisions) != 0 {
		t.Fatalf("undo did not restore result: %#v decisions=%#v", a.results[0], a.decisions)
	}
}

func TestCandidateMatchScoreReserves100ForExact(t *testing.T) {
	r := RemoteItem{Name: "1920x3410_0e905198575d63a8d9e7_source.mp4", Size: 1000}
	e := FileEntry{Path: "A", Name: "2021-11-05_1920x3410_0e905198575d63a8d9e7.mp4", Size: 1000}
	s := candidateMatchScore(r, e)
	if s < 95 || s > 99 {
		t.Fatalf("smart score = %d, want 95..99", s)
	}
}

func TestResultPendingReviewMovesAfterManualDecision(t *testing.T) {
	x := Result{Status: "HAVE", AutoStatus: "HAVE"}
	if !resultPendingReview(x) {
		t.Fatal("unconfirmed HAVE should be pending review")
	}
	x.Manual = true
	if resultPendingReview(x) {
		t.Fatal("manual decision should remove item from review queue")
	}
}

func TestSummarySeparatesWorkflowFromEffectiveStatus(t *testing.T) {
	rows := []Result{
		{Status: "HAVE", AutoStatus: "HAVE", Remote: RemoteItem{Size: 100}},
		{Status: "HAVE", AutoStatus: "HAVE", Manual: true, Remote: RemoteItem{Size: 200}},
		{Status: "MISSING", AutoStatus: "MISSING", Remote: RemoteItem{Size: 300}},
	}
	s := buildResultSummary(rows)
	wf := s["workflow"].(map[string]int)
	eff := s["effective"].(map[string]int)
	if eff["HAVE"] != 2 {
		t.Fatalf("effective HAVE = %d, want 2", eff["HAVE"])
	}
	if wf["AUTO_HAVE"] != 1 || wf["MANUAL"] != 1 || wf["REVIEW"] != 1 || wf["DOWNLOAD"] != 1 {
		t.Fatalf("unexpected workflow summary: %#v", wf)
	}
}

func TestSmartSelectionVeryLikelyAcrossWholeSet(t *testing.T) {
	a := &App{appDir: t.TempDir(), index: map[string]FileEntry{}, decisions: map[string]Decision{}}
	a.results = []Result{
		{ID: 1, Status: "POSSIBLE", AutoStatus: "POSSIBLE", LocalPath: "A", SameSize: true, SameExt: true, MatchScore: 97, Remote: RemoteItem{Name: "a.mp4", Size: 100, Source: "MEGA"}},
		{ID: 2, Status: "POSSIBLE", AutoStatus: "POSSIBLE", LocalPath: "B", SameSize: true, SameExt: true, MatchScore: 88, Remote: RemoteItem{Name: "b.mp4", Size: 200, Source: "MEGA"}},
		{ID: 3, Status: "HAVE", AutoStatus: "HAVE", Manual: true, LocalPath: "C", SameSize: true, SameExt: true, MatchScore: 99, Remote: RemoteItem{Name: "c.mp4", Size: 300, Source: "MEGA"}},
	}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/results/select", strings.NewReader(`{"rule":"very-likely","scope":"all","scoreMin":95,"scoreMax":100}`))
	a.handleSmartSelect(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("smart select failed: %d %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"count":1`) || !strings.Contains(rr.Body.String(), `"ids":[1]`) {
		t.Fatalf("unexpected smart selection: %s", rr.Body.String())
	}
}

func TestResultFilterAutoHaveExcludesManual(t *testing.T) {
	a := Result{Status: "HAVE", AutoStatus: "HAVE", Manual: false}
	b := Result{Status: "HAVE", AutoStatus: "HAVE", Manual: true}
	if !resultMatchesFilter(a, "", "AUTO_HAVE") {
		t.Fatal("automatic HAVE should match AUTO_HAVE")
	}
	if resultMatchesFilter(b, "", "AUTO_HAVE") {
		t.Fatal("manual HAVE should not match AUTO_HAVE")
	}
}

func TestMarkUpdatesReviewAndManualCountsAtomically(t *testing.T) {
	a := &App{
		appDir:    t.TempDir(),
		index:     map[string]FileEntry{},
		bySize:    map[int64][]string{},
		byName:    map[string][]string{},
		decisions: map[string]Decision{},
		cfg:       Config{Mode: "balanced"},
	}
	a.results = []Result{{ID: 1, Status: "HAVE", AutoStatus: "HAVE", Remote: RemoteItem{Path: "/a.mp4", Name: "a.mp4", Size: 10, Source: "MEGA"}}}
	if !resultPendingReview(a.results[0]) {
		t.Fatal("precondition: should be in review")
	}
	rr := httptest.NewRecorder()
	a.handleMark(rr, httptest.NewRequest(http.MethodPost, "/api/results/mark", strings.NewReader(`{"ids":[1],"status":"HAVE"}`)))
	if rr.Code != http.StatusOK {
		t.Fatalf("mark failed: %d %s", rr.Code, rr.Body.String())
	}
	if resultPendingReview(a.results[0]) {
		t.Fatal("marked result should leave review immediately")
	}
	if !strings.Contains(rr.Body.String(), `"MANUAL":1`) || !strings.Contains(rr.Body.String(), `"REVIEW_DONE":1`) {
		t.Fatalf("mark response summary not updated: %s", rr.Body.String())
	}
}
