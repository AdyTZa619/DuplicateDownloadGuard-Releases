package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func testPreviewControllerV8526(a *App, run func(context.Context, time.Duration, string, ...string) (string, error)) *megaPreviewControllerV8526 {
	return newMegaPreviewControllerV8526(a, megaPreviewOpsV8526{
		detectExe: func() string { return "MegaClient.exe" },
		run:       run,
		acquire:   func(context.Context) error { return nil },
		release:   func() {},
	})
}

func TestMegaPreviewTwentyPerFileSwitchesNeverUseRootOrReloginV8527(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Accept-Ranges", "bytes")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = io.WriteString(w, "preview")
	}))
	defer server.Close()
	a := &App{preview: MegaPreviewState{Active: true, SourceURL: "mega-source", Exe: "MegaClient.exe"}}
	c := testPreviewControllerV8526(a, func(_ context.Context, _ time.Duration, _ string, args ...string) (string, error) {
		calls.Add(1)
		if len(args) != 2 || args[0] != "webdav" || !strings.HasPrefix(args[1], "H:") {
			t.Fatalf("comandă neașteptată la preview-ul cald: %v", args)
		}
		return "Serving via webdav " + args[1] + ": " + server.URL + "/" + strings.TrimPrefix(args[1], "H:"), nil
	})
	defer c.close("test")

	var last uint64
	for i := 1; i <= 20; i++ {
		generation, localURL, _, err := c.begin(RemoteItem{Source: "MEGA", URL: "mega-source", Path: fmt.Sprintf("album/clip-%02d.mp4", i), Handle: fmt.Sprintf("HANDLE%08d", i)}, "video", false, 0)
		if err != nil {
			t.Fatalf("switch %d: %v", i, err)
		}
		if !strings.Contains(localURL, fmt.Sprintf("generation=%d", generation)) {
			t.Fatalf("switch %d returned wrong local URL: %s", i, localURL)
		}
		job, currentErr := c.currentJob(generation)
		if currentErr != nil {
			t.Fatal(currentErr)
		}
		select {
		case <-job.ready:
		case <-time.After(time.Second):
			t.Fatalf("switch %d nu a pregătit ruta per-fișier", i)
		}
		if job.err != nil {
			t.Fatalf("switch %d: %v", i, job.err)
		}
		last = generation
	}
	if calls.Load() != 20 {
		t.Fatalf("20 fișiere au executat %d comenzi, vreau exact una per fișier", calls.Load())
	}
	if last != 20 {
		t.Fatalf("last generation=%d, want 20", last)
	}
	for generation := uint64(1); generation < last; generation++ {
		trace, ok := c.trace(generation)
		if !ok || trace.Points["T11"].AtMS == 0 || trace.Points["T12"].AtMS == 0 {
			t.Fatalf("generation %d was not cancelled/cleaned: %#v", generation, trace.Points)
		}
	}
	trace, _ := c.trace(last)
	if trace.Route != "managed-per-file" || trace.Points["T4"].AtMS == 0 {
		t.Fatalf("ultimul trace per-fișier este invalid: %#v", trace)
	}
}

func TestMegaPreviewABCDoesNotCancelMegaControlAndLatestWinsV8528(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Accept-Ranges", "bytes")
		_, _ = io.WriteString(w, "stream")
	}))
	defer server.Close()
	started := make(chan struct{})
	release := make(chan struct{})
	commandContexts := make(chan context.Context, 3)
	var once sync.Once
	var calls atomic.Int32
	a := &App{preview: MegaPreviewState{Active: true, SourceURL: "same-source", Exe: "MegaClient.exe"}}
	c := testPreviewControllerV8526(a, func(ctx context.Context, _ time.Duration, _ string, args ...string) (string, error) {
		calls.Add(1)
		if len(args) != 2 || args[0] != "webdav" || !strings.HasPrefix(args[1], "H:") {
			t.Fatalf("unexpected command: %v", args)
		}
		commandContexts <- ctx
		once.Do(func() { close(started) })
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-release:
			return "Serving via webdav " + args[1] + ": " + server.URL + "/" + strings.TrimPrefix(args[1], "H:"), nil
		}
	})

	genA, _, _, err := c.begin(RemoteItem{Source: "MEGA", URL: "same-source", Path: "A.mp4", Handle: "HANDLEAAAA"}, "video", false, 0)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("source initialization did not start")
	}
	genB, _, _, _ := c.begin(RemoteItem{Source: "MEGA", URL: "same-source", Path: "B.mp4", Handle: "HANDLEBBBB"}, "video", false, 0)
	genC, _, _, _ := c.begin(RemoteItem{Source: "MEGA", URL: "same-source", Path: "C.mp4", Handle: "HANDLECCCC"}, "video", false, 0)
	if genA != 1 || genB != 2 || genC != 3 {
		t.Fatalf("unexpected generations: %d %d %d", genA, genB, genC)
	}
	firstCommandCtx := <-commandContexts
	select {
	case <-firstCommandCtx.Done():
		t.Fatal("schimbarea selecției a anulat procesul de control MegaClient deja pornit")
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	job, err := c.currentJob(genC)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	target, err := c.targetFor(ctx, job)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(target, "/HANDLECCCC") {
		t.Fatalf("last click did not own target: %s", target)
	}
	if calls.Load() < 2 || calls.Load() > 3 {
		t.Fatalf("A/B/C started an unexpected number of short per-file commands: %d", calls.Load())
	}
	if _, err := c.currentJob(genA); err == nil {
		t.Fatal("obsolete A still owns the player")
	}
}

func TestMegaPreviewControllerContainsNoRuntimeWebDAVDeleteV8528(t *testing.T) {
	b, err := os.ReadFile("mega_preview_controller_v8526.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(b)
	if strings.Contains(source, `"webdav", "-d"`) || strings.Contains(source, "cleanupIdleRoutes") || strings.Contains(source, "cleanupTTL") {
		t.Fatal("controllerul de preview încă poate porni cleanup WebDAV în timpul utilizării")
	}
}

func TestMegaPreviewRecoversWindowsPipeOnceAndRetriesCurrentFileV8529(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	defer server.Close()
	var commandCalls atomic.Int32
	var recoveryCalls atomic.Int32
	a := &App{preview: MegaPreviewState{Active: true, SourceURL: "source", Exe: "MegaClient.exe"}}
	c := newMegaPreviewControllerV8526(a, megaPreviewOpsV8526{
		detectExe: func() string { return "MegaClient.exe" },
		run: func(_ context.Context, _ time.Duration, _ string, args ...string) (string, error) {
			n := commandCalls.Add(1)
			if n == 1 {
				return "Failed to access server: 231", errors.New("exit status 0xfffffffe")
			}
			return "Serving via webdav " + args[1] + ": " + server.URL + "/file", nil
		},
		acquire: func(context.Context) error { return nil },
		release: func() {},
		recover: func(string) (string, error) {
			recoveryCalls.Add(1)
			return "MEGAcmdServer restarted", nil
		},
	})
	defer c.close("test")
	generation, _, _, err := c.begin(RemoteItem{Source: "MEGA", URL: "source", Path: "clip.mp4", Handle: "HANDLE-PIPE"}, "video", false, 0)
	if err != nil {
		t.Fatal(err)
	}
	job, _ := c.currentJob(generation)
	select {
	case <-job.ready:
	case <-time.After(time.Second):
		t.Fatal("recuperarea pipe-ului nu s-a terminat")
	}
	if job.err != nil {
		t.Fatal(job.err)
	}
	if recoveryCalls.Load() != 1 || commandCalls.Load() != 2 {
		t.Fatalf("recovery=%d commands=%d, vreau o recuperare și o singură repetare", recoveryCalls.Load(), commandCalls.Load())
	}
	trace, _ := c.trace(generation)
	foundRecovery := false
	for _, command := range trace.Commands {
		if command.Name == "MEGAcmdServer recovery" {
			foundRecovery = true
		}
	}
	if !foundRecovery {
		t.Fatal("recuperarea serviciului nu apare în diagnosticul preview-ului")
	}
}

func TestMegaPreviewDoesNotLoopPipeRecoveryV8529(t *testing.T) {
	var recoveryCalls atomic.Int32
	a := &App{preview: MegaPreviewState{Active: true, SourceURL: "source", Exe: "MegaClient.exe"}}
	c := newMegaPreviewControllerV8526(a, megaPreviewOpsV8526{
		detectExe: func() string { return "MegaClient.exe" },
		run: func(_ context.Context, _ time.Duration, _ string, _ ...string) (string, error) {
			return "Failed to access server: 231", errors.New("exit status 0xfffffffe")
		},
		acquire: func(context.Context) error { return nil },
		release: func() {},
		recover: func(string) (string, error) {
			recoveryCalls.Add(1)
			return "MEGAcmdServer restarted", nil
		},
	})
	defer c.close("test")
	for i := 0; i < 2; i++ {
		generation, _, _, err := c.begin(RemoteItem{Source: "MEGA", URL: "source", Path: fmt.Sprintf("clip-%d.mp4", i), Handle: fmt.Sprintf("HANDLE-PIPE-%d", i)}, "video", false, 0)
		if err != nil {
			t.Fatal(err)
		}
		job, _ := c.currentJob(generation)
		select {
		case <-job.ready:
		case <-time.After(time.Second):
			t.Fatal("preview-ul cu pipe blocat nu s-a încheiat")
		}
		if job.err == nil {
			t.Fatal("pipe-ul rămas blocat trebuia raportat")
		}
	}
	if recoveryCalls.Load() != 1 {
		t.Fatalf("recuperarea a intrat în buclă: %d încercări", recoveryCalls.Load())
	}
}

func TestMegaPreviewRange206AndFirstByteTimingV8526(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Range"); got != "bytes=5-9" {
			t.Errorf("upstream Range=%q", got)
		}
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Content-Range", "bytes 5-9/20")
		w.Header().Set("Content-Length", "5")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = io.WriteString(w, "56789")
	}))
	defer server.Close()
	a := &App{preview: MegaPreviewState{Active: true, SourceURL: "range-source", Exe: "MegaClient.exe"}}
	c := testPreviewControllerV8526(a, func(_ context.Context, _ time.Duration, _ string, args ...string) (string, error) {
		return "Serving via webdav " + args[1] + ": " + server.URL + "/movie", nil
	})
	generation, _, _, err := c.begin(RemoteItem{Source: "MEGA", URL: "range-source", Path: "movie.mp4", Handle: "HANDLEMOVIE"}, "video", false, 0)
	if err != nil {
		t.Fatal(err)
	}
	job, _ := c.currentJob(generation)
	<-job.ready
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/remote-preview/media?generation=%d", generation), nil)
	req.Header.Set("Range", "bytes=5-9")
	recorder := httptest.NewRecorder()
	c.serveMedia(recorder, req, generation)
	if recorder.Code != http.StatusPartialContent || recorder.Body.String() != "56789" {
		t.Fatalf("proxy response=%d %q", recorder.Code, recorder.Body.String())
	}
	if recorder.Header().Get("Content-Range") != "bytes 5-9/20" || recorder.Header().Get("Accept-Ranges") != "bytes" {
		t.Fatalf("range headers missing: %#v", recorder.Header())
	}
	trace, _ := c.trace(generation)
	if trace.Points["T5"].AtMS == 0 || trace.Points["T8"].AtMS == 0 || trace.HTTPStatus != http.StatusPartialContent {
		t.Fatalf("HTTP timing incomplete: %#v", trace)
	}
}

func TestMegaPreviewObsoleteGenerationReturnsGoneV8526(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "ok") }))
	defer server.Close()
	a := &App{preview: MegaPreviewState{Active: true, SourceURL: "source", Exe: "MegaClient.exe"}}
	c := testPreviewControllerV8526(a, func(_ context.Context, _ time.Duration, _ string, args ...string) (string, error) {
		return "Serving via webdav " + args[1] + ": " + server.URL + "/file", nil
	})
	oldGeneration, _, _, _ := c.begin(RemoteItem{Source: "MEGA", URL: "source", Path: "old.mp4", Handle: "HANDLEOLD1"}, "video", false, 0)
	_, _, _, _ = c.begin(RemoteItem{Source: "MEGA", URL: "source", Path: "new.mp4", Handle: "HANDLENEW1"}, "video", false, 0)
	recorder := httptest.NewRecorder()
	c.serveMedia(recorder, httptest.NewRequest(http.MethodGet, "/", nil), oldGeneration)
	if recorder.Code != http.StatusGone {
		t.Fatalf("obsolete response=%d, want 410", recorder.Code)
	}
}

func TestMegaPreviewTwentyStreamingSwitchesDoNotLeakRequestsV8526(t *testing.T) {
	var active atomic.Int32
	var maximum atomic.Int32
	started := make(chan struct{}, 32)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current := active.Add(1)
		defer active.Add(-1)
		for {
			old := maximum.Load()
			if current <= old || maximum.CompareAndSwap(old, current) {
				break
			}
		}
		w.Header().Set("Accept-Ranges", "bytes")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = io.WriteString(w, "frame")
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		started <- struct{}{}
		<-r.Context().Done()
	}))
	defer server.Close()
	a := &App{preview: MegaPreviewState{Active: true, SourceURL: "stream-source", Exe: "MegaClient.exe"}}
	c := testPreviewControllerV8526(a, func(_ context.Context, _ time.Duration, _ string, args ...string) (string, error) {
		return "Serving via webdav " + args[1] + ": " + server.URL + "/stream", nil
	})
	defer c.close("test")
	var wg sync.WaitGroup
	for i := 1; i <= 20; i++ {
		generation, _, _, err := c.begin(RemoteItem{Source: "MEGA", URL: "stream-source", Path: fmt.Sprintf("clip-%02d.mp4", i), Handle: fmt.Sprintf("HANDLE%08d", i)}, "video", false, 0)
		if err != nil {
			t.Fatal(err)
		}
		wg.Add(1)
		go func(generation uint64) {
			defer wg.Done()
			recorder := httptest.NewRecorder()
			c.serveMedia(recorder, httptest.NewRequest(http.MethodGet, "/", nil), generation)
		}(generation)
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatalf("stream %d did not start", i)
		}
	}
	c.cancelCurrent("test complete")
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("cancelled streams did not exit")
	}
	deadline := time.Now().Add(time.Second)
	for active.Load() != 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := active.Load(); got != 0 {
		t.Fatalf("active upstream requests did not drain: %d", got)
	}
	if got := maximum.Load(); got > 2 {
		t.Fatalf("upstream requests grew to %d instead of remaining bounded", got)
	}
}

func TestMegaPreviewUsesHandleForExplicitFallbackV8526(t *testing.T) {
	var command string
	a := &App{}
	c := testPreviewControllerV8526(a, func(_ context.Context, _ time.Duration, _ string, args ...string) (string, error) {
		command = strings.Join(args, " ")
		return "Serving via webdav H:HANDLE-UNIQUE: http://127.0.0.1:4443/file", nil
	})
	generation, _, _, err := c.begin(RemoteItem{Source: "MEGA", URL: "source", Path: "renamed path.mp4", Handle: "HANDLE-UNIQUE"}, "video", true, 0)
	if err != nil {
		t.Fatal(err)
	}
	job, _ := c.currentJob(generation)
	select {
	case <-job.ready:
	case <-time.After(time.Second):
		t.Fatal("fallback did not finish")
	}
	if command != "webdav H:HANDLE-UNIQUE" {
		t.Fatalf("fallback command=%q", command)
	}
}

func TestMegaPreviewResumesSourceOnceThenKeepsSessionV8527(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	defer server.Close()
	var mu sync.Mutex
	loggedIn := false
	loginCalls := 0
	rootCalls := 0
	a := &App{}
	c := testPreviewControllerV8526(a, func(_ context.Context, _ time.Duration, _ string, args ...string) (string, error) {
		mu.Lock()
		defer mu.Unlock()
		command := strings.Join(args, " ")
		switch {
		case command == "session":
			return "Your (secret) session is: PREVIOUS", nil
		case command == "logout --keep-session":
			return "Logging out", nil
		case len(args) >= 1 && args[0] == "login":
			loginCalls++
			loggedIn = true
			return "", nil
		case len(args) == 2 && args[0] == "webdav" && args[1] == "/":
			rootCalls++
			return "", errors.New("root must never be used")
		case len(args) == 2 && args[0] == "webdav" && strings.HasPrefix(args[1], "H:"):
			if !loggedIn {
				return "API_ENOENT", errors.New("not found")
			}
			return "Serving via webdav " + args[1] + ": " + server.URL + "/" + strings.TrimPrefix(args[1], "H:"), nil
		default:
			return "", fmt.Errorf("unexpected command: %s", command)
		}
	})

	for i, handle := range []string{"HANDLE-FIRST", "HANDLE-SECOND"} {
		generation, _, _, err := c.begin(RemoteItem{Source: "MEGA", URL: "public-source", Path: fmt.Sprintf("clip-%d.mp4", i), Handle: handle}, "video", false, 0)
		if err != nil {
			t.Fatal(err)
		}
		job, _ := c.currentJob(generation)
		select {
		case <-job.ready:
		case <-time.After(time.Second):
			t.Fatal("preview preparation timed out")
		}
		if job.err != nil {
			t.Fatal(job.err)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if loginCalls != 1 {
		t.Fatalf("source login calls=%d, want 1", loginCalls)
	}
	if rootCalls != 0 {
		t.Fatalf("webdav / calls=%d, want 0", rootCalls)
	}
}

func TestMegaPreviewRetryBypassesStalePerFileCacheV8527(t *testing.T) {
	var calls atomic.Int32
	a := &App{preview: MegaPreviewState{Active: true, SourceURL: "source", Exe: "MegaClient.exe"}}
	c := testPreviewControllerV8526(a, func(_ context.Context, _ time.Duration, _ string, args ...string) (string, error) {
		n := calls.Add(1)
		return fmt.Sprintf("Serving via webdav %s: http://127.0.0.1:4443/file-%d", args[1], n), nil
	})
	item := RemoteItem{Source: "MEGA", URL: "source", Path: "clip.mp4", Handle: "HANDLE-RETRY"}
	first, _, _, _ := c.begin(item, "video", false, 0)
	firstJob, _ := c.currentJob(first)
	<-firstJob.ready
	second, _, _, _ := c.begin(item, "video", true, 0)
	secondJob, _ := c.currentJob(second)
	<-secondJob.ready
	if calls.Load() != 2 {
		t.Fatalf("retry used stale cache; MEGAcmd calls=%d, want 2", calls.Load())
	}
	if firstJob.target == secondJob.target {
		t.Fatalf("retry did not replace stale target: %s", secondJob.target)
	}
}

func TestMegaPreviewHTTP509ReportsQuotaV8527(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(509)
		_, _ = io.WriteString(w, "transfer quota exceeded; retry after 2 hours")
	}))
	defer server.Close()
	a := &App{preview: MegaPreviewState{Active: true, SourceURL: "source", Exe: "MegaClient.exe"}}
	c := testPreviewControllerV8526(a, func(_ context.Context, _ time.Duration, _ string, args ...string) (string, error) {
		return "Serving via webdav " + args[1] + ": " + server.URL + "/quota", nil
	})
	generation, _, _, _ := c.begin(RemoteItem{Source: "MEGA", URL: "source", Path: "clip.mp4", Handle: "HANDLE-QUOTA"}, "video", false, 0)
	job, _ := c.currentJob(generation)
	<-job.ready
	recorder := httptest.NewRecorder()
	c.serveMedia(recorder, httptest.NewRequest(http.MethodGet, "/", nil), generation)
	trace, _ := c.trace(generation)
	if trace.Problem == nil || trace.Problem.Code != "MEGA_QUOTA" {
		t.Fatalf("HTTP 509 problem=%#v", trace.Problem)
	}
	if !strings.Contains(trace.Problem.Action, "~2 ore") {
		t.Fatalf("quota retry time missing: %s", trace.Problem.Action)
	}
}

func TestMegaPreviewDiagnosticRedactsSessionTokenV8527(t *testing.T) {
	got := shortMegaOutputV8526("Your (secret) session is: VERY-SECRET-TOKEN\nnext line")
	if strings.Contains(got, "VERY-SECRET-TOKEN") || !strings.Contains(got, "<redacted>") {
		t.Fatalf("session output was not redacted: %q", got)
	}
}

func TestMegaPreviewUIOwnsT0ToT12AndNoAutomaticFallbackV8526(t *testing.T) {
	b, err := os.ReadFile("web/mega_preview_v8526.js")
	if err != nil {
		t.Fatal(err)
	}
	source := string(b)
	for _, marker := range []string{"'T6'", "'T7'", "'T9'", "'T10'", "'T11'", "'T12'", "clientT0", "requestVideoFrameCallback", "megaPreviewFallbackV8526"} {
		if !strings.Contains(source, marker) {
			t.Fatalf("UI timing/control marker missing: %s", marker)
		}
	}
	if strings.Contains(source, "forceFallback:true") || strings.Contains(source, "forceFallback: true") {
		t.Fatal("UI must not trigger an automatic fallback loop")
	}
	index, err := os.ReadFile("web/index.html")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(index), `<script defer src="/mega_preview_v8526.js"></script>`) {
		t.Fatal("v8.5.27 preview UI is not embedded")
	}
}

func TestMegaPreviewUIDisconnectsOldNativeMediaBeforeNextStartV8530(t *testing.T) {
	b, err := os.ReadFile("web/mega_preview_v8526.js")
	if err != nil {
		t.Fatal(err)
	}
	source := string(b)
	videoReset := strings.Index(source, "media.removeAttribute('src')")
	videoLoad := strings.Index(source, "media.load()")
	videoRemove := strings.Index(source, "media.remove()")
	imageReset := strings.Index(source, "image.src = 'data:image/gif;base64,")
	imageRemove := strings.Index(source, "image.remove()")
	requestStart := strings.Index(source, "const data = await api('/api/remote-preview/start'")
	cleanupCall := strings.Index(source, "removeRemoteMediaV8526(forceFallback ? 'explicit fallback' : 'new selection')")
	if videoReset < 0 || videoLoad < videoReset || videoRemove < videoLoad {
		t.Fatal("video/audio must clear src, call load, then detach the DOM node")
	}
	if imageReset < 0 || imageRemove < imageReset {
		t.Fatal("progressive images must replace src before detaching the DOM node")
	}
	if cleanupCall < 0 || requestStart < cleanupCall {
		t.Fatal("old native media must be disconnected before the next preview start request")
	}
	if !strings.Contains(source, "activeV8526 = null;") || !strings.Contains(source, "activeV8526?.generation === generation") {
		t.Fatal("events emitted by a reset stale element must not update the active preview")
	}
}
