package main

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestSwitchSameSourceWebDAVStartsNewBeforeStoppingOld(t *testing.T) {
	old := MegaPreviewState{RemotePath: "H:OLD"}
	calls := []string{}
	run := func(_ time.Duration, args ...string) (string, error) {
		calls = append(calls, strings.Join(args, " "))
		if len(args) == 2 && args[0] == "webdav" && args[1] == "H:NEW" {
			return "H:NEW http://127.0.0.1:4443/new.mp4", nil
		}
		return "", nil
	}
	got, err := switchSameSourceWebDAVV85(old, "H:NEW", run)
	if err != nil {
		t.Fatal(err)
	}
	if got.StreamURL == "" {
		t.Fatal("new stream URL missing")
	}
	want := []string{"webdav H:NEW", "webdav -d H:OLD"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls=%v want=%v", calls, want)
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
