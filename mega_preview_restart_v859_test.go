package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMegaPreviewRestartHintRoundTripV859(t *testing.T) {
	a := &App{appDir: t.TempDir()}
	url := "https://mega.nz/folder/example#key"
	a.saveMegaPreviewRestartHintV859(url)
	if !a.matchesMegaPreviewRestartHintV859(url) {
		t.Fatal("saved restart hint was not matched")
	}
	if a.matchesMegaPreviewRestartHintV859("https://mega.nz/folder/other#key") {
		t.Fatal("restart hint matched a different MEGA folder")
	}
	a.clearMegaPreviewRestartHintV859()
	if a.matchesMegaPreviewRestartHintV859(url) {
		t.Fatal("restart hint survived clear")
	}
}

func TestPersistentMegaRootRoundTripV8510(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	a := &App{appDir: t.TempDir()}
	source := "https://mega.nz/folder/example#key"
	a.saveMegaPreviewRestartRootV8510(source, ts.URL+"/root")
	item := RemoteItem{Source: "MEGA", URL: source, Path: "sub/video.mp4", Name: "video.mp4"}
	root, child, ok := a.tryPersistedMegaRootV8510(item)
	if !ok {
		t.Fatal("persisted loopback root was not reused")
	}
	if root != ts.URL+"/root" {
		t.Fatalf("root=%q", root)
	}
	if child != ts.URL+"/root/sub/video.mp4" {
		t.Fatalf("child=%q", child)
	}
}

func TestPersistentMegaRootRejectsNonLoopbackV8510(t *testing.T) {
	a := &App{appDir: t.TempDir()}
	source := "https://mega.nz/folder/example#key"
	a.saveMegaPreviewRestartRootV8510(source, "https://example.com/root")
	item := RemoteItem{Source: "MEGA", URL: source, Path: "video.mp4"}
	if _, _, ok := a.tryPersistedMegaRootV8510(item); ok {
		t.Fatal("persistent root must only trust a loopback listener")
	}
}

func TestTryMegaCurrentSessionWebDAVV859UsesOnlyWebDAV(t *testing.T) {
	var calls [][]string
	run := func(timeout time.Duration, args ...string) (string, error) {
		copyArgs := append([]string(nil), args...)
		calls = append(calls, copyArgs)
		if len(args) == 2 && args[0] == "webdav" && args[1] == "H:ABCDEF123456" {
			return "WebDAV server is running at http://127.0.0.1:4443/ABCDEF123456/video.mp4", nil
		}
		return "", errors.New("unexpected command")
	}
	result, err := tryMegaCurrentSessionWebDAVV859("H:ABCDEF123456", run)
	if err != nil {
		t.Fatalf("direct current-session WebDAV failed: %v", err)
	}
	if result.StreamURL == "" {
		t.Fatal("stream URL missing")
	}
	if len(calls) != 1 {
		t.Fatalf("expected one direct WebDAV command, got %#v", calls)
	}
	joined := strings.Join(calls[0], " ")
	for _, forbidden := range []string{"session", "logout", "login"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("cold fast path must not run %s: %#v", forbidden, calls)
		}
	}
}

func TestTryMegaCurrentSessionWebDAVV859FailsFastToCaller(t *testing.T) {
	calls := 0
	run := func(timeout time.Duration, args ...string) (string, error) {
		calls++
		if timeout > 4*time.Second {
			t.Fatalf("restart fast path timeout is too large: %v", timeout)
		}
		return "not logged in", errors.New("not logged in")
	}
	if _, err := tryMegaCurrentSessionWebDAVV859("H:ABCDEF123456", run); err == nil {
		t.Fatal("expected current-session failure")
	}
	if calls != 1 {
		t.Fatalf("failed direct attempt should not fan out commands: %d", calls)
	}
}
