package main

import (
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSwitchSameSourceWebDAVReturnsBeforeOldCleanupCompletes(t *testing.T) {
	old := MegaPreviewState{RemotePath: "H:OLD"}
	var mu sync.Mutex
	calls := []string{}
	cleanupStarted := make(chan struct{}, 1)
	cleanupRelease := make(chan struct{})
	run := func(_ time.Duration, args ...string) (string, error) {
		call := strings.Join(args, " ")
		mu.Lock()
		calls = append(calls, call)
		mu.Unlock()
		if len(args) == 2 && args[0] == "webdav" && args[1] == "H:NEW" {
			return "H:NEW http://127.0.0.1:4443/new.mp4", nil
		}
		if call == "webdav -d H:OLD" {
			select {
			case cleanupStarted <- struct{}{}:
			default:
			}
			<-cleanupRelease
		}
		return "", nil
	}

	start := time.Now()
	got, err := switchSameSourceWebDAVV85(old, "H:NEW", run)
	if err != nil {
		t.Fatal(err)
	}
	if got.StreamURL == "" {
		t.Fatal("new stream URL missing")
	}
	if time.Since(start) > 250*time.Millisecond {
		t.Fatalf("switch waited for old cleanup instead of returning new URL quickly: %s", time.Since(start))
	}
	select {
	case <-cleanupStarted:
	case <-time.After(time.Second):
		t.Fatal("old WebDAV cleanup did not start asynchronously")
	}
	close(cleanupRelease)
	time.Sleep(20 * time.Millisecond)

	mu.Lock()
	gotCalls := append([]string(nil), calls...)
	mu.Unlock()
	want := []string{"webdav H:NEW", "webdav -d H:OLD"}
	if !reflect.DeepEqual(gotCalls, want) {
		t.Fatalf("calls=%v want=%v", gotCalls, want)
	}
}

func TestSwitchSameSourceWebDAVKeepsOldWhenNewStartFails(t *testing.T) {
	old := MegaPreviewState{RemotePath: "H:OLD"}
	calls := []string{}
	run := func(_ time.Duration, args ...string) (string, error) {
		calls = append(calls, strings.Join(args, " "))
		return "quota exceeded", errors.New("start failed")
	}
	_, err := switchSameSourceWebDAVV85(old, "H:NEW", run)
	if err == nil {
		t.Fatal("expected start failure")
	}
	if len(calls) != 1 || calls[0] != "webdav H:NEW" {
		t.Fatalf("old preview must not be stopped after failed start; calls=%v", calls)
	}
}

func TestSwitchSameSourceWebDAVRemovesOnlyUnusableNewNode(t *testing.T) {
	old := MegaPreviewState{RemotePath: "H:OLD"}
	calls := []string{}
	run := func(_ time.Duration, args ...string) (string, error) {
		calls = append(calls, strings.Join(args, " "))
		return "", nil
	}
	_, err := switchSameSourceWebDAVV85(old, "H:NEW", run)
	if err == nil {
		t.Fatal("expected missing URL error")
	}
	want := []string{"webdav H:NEW", "webdav", "webdav -d H:NEW"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls=%v want=%v", calls, want)
	}
}
