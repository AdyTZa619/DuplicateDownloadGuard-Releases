package main

import (
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSwitchSameSourceWebDAVReturnsNewURLWithoutCleanup(t *testing.T) {
	old := MegaPreviewState{Exe: "MegaClient.exe", RemotePath: "H:OLD"}
	calls := []string{}
	run := func(_ time.Duration, args ...string) (string, error) {
		calls = append(calls, strings.Join(args, " "))
		if len(args) == 2 && args[0] == "webdav" && args[1] == "H:NEW" {
			return "H:NEW http://127.0.0.1:4443/new.mp4", nil
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
		t.Fatalf("switch should return immediately after new URL is known: %s", time.Since(start))
	}
	if !reflect.DeepEqual(calls, []string{"webdav H:NEW"}) {
		t.Fatalf("switch itself must not wait for old cleanup; calls=%v", calls)
	}
}

func TestSchedulePreviousMegaPreviewCleanupRunsInBackground(t *testing.T) {
	old := MegaPreviewState{Exe: "MegaClient.exe", RemotePath: "H:OLD"}
	var mu sync.Mutex
	calls := []string{}
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	run := func(_ time.Duration, args ...string) (string, error) {
		call := strings.Join(args, " ")
		mu.Lock()
		calls = append(calls, call)
		mu.Unlock()
		if call == "webdav -d H:OLD" {
			started <- struct{}{}
			<-release
		}
		return "", nil
	}
	start := time.Now()
	if !schedulePreviousMegaPreviewCleanupV86(old, "H:NEW", 0, run) {
		t.Fatal("cleanup should be scheduled")
	}
	if time.Since(start) > 100*time.Millisecond {
		t.Fatal("cleanup scheduling blocked caller")
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("background cleanup did not start")
	}
	close(release)
	time.Sleep(20 * time.Millisecond)
	mu.Lock()
	got := append([]string(nil), calls...)
	mu.Unlock()
	if !reflect.DeepEqual(got, []string{"webdav -d H:OLD"}) {
		t.Fatalf("calls=%v", got)
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
