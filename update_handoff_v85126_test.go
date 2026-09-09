package main

import (
	"os"
	"strings"
	"testing"
)

func TestUpdateHandoffClosesStaleUIAfterInstanceClaimV85126(t *testing.T) {
	b, err := os.ReadFile("ui_window_cleanup.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	for _, token := range []string{
		"postUpdateHandoffPending()",
		"claimDDGSingleInstanceNative()",
		"closeDDGPresenceWindowsForHandoffNative()",
		"terminateOtherDDGProcessesSameImageNative()",
	} {
		if !strings.Contains(text, token) {
			t.Fatalf("update handoff startup missing %q", token)
		}
	}
	claim := strings.Index(text, "if !claimDDGSingleInstanceNative()")
	handoff := strings.Index(text, "if postUpdateHandoffPending()")
	closeStale := strings.Index(text, "closeDDGPresenceWindowsForHandoffNative()")
	if claim < 0 || handoff < 0 || closeStale < 0 || !(claim < handoff && handoff < closeStale) {
		t.Fatalf("startup order must be claim -> handoff detection -> stale UI close; got claim=%d handoff=%d close=%d", claim, handoff, closeStale)
	}
}

func TestUpdateHandoffUsesTolerantWindowMatcherV85126(t *testing.T) {
	b, err := os.ReadFile("ui_window_cleanup_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	start := strings.Index(text, "func closeDDGPresenceWindowsForHandoffNative()")
	if start < 0 {
		t.Fatal("handoff close function missing")
	}
	body := text[start:]
	for _, token := range []string{"matchingDDGPresenceWindowsV85117()", "wmClose", "swHide"} {
		if !strings.Contains(body, token) {
			t.Fatalf("handoff close must contain %q", token)
		}
	}
}

func TestSingleInstanceMutexIsPortablePathScopedV85126(t *testing.T) {
	b, err := os.ReadFile("single_instance_windows_v85125.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	for _, token := range []string{
		"ddgSingleInstanceMutexNameV85126()",
		"os.Executable()",
		"filepath.Clean(current)",
		"sha256.Sum256",
		"hex.EncodeToString",
	} {
		if !strings.Contains(text, token) {
			t.Fatalf("path-scoped mutex implementation missing %q", token)
		}
	}
	if strings.Contains(text, "ddgSingleInstanceMutexNameV85125 =") {
		t.Fatal("fixed TEST125 global mutex must not remain")
	}
}
