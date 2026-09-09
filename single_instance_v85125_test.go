package main

import (
	"os"
	"strings"
	"testing"
)

func TestSingleInstanceStartupMigrationV85125(t *testing.T) {
	ui, err := os.ReadFile("ui_window_cleanup.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(ui)
	for _, token := range []string{
		"claimDDGSingleInstanceNative()",
		"activateExistingDDGWindowNative()",
		"terminateOtherDDGProcessesSameImageNative()",
		"runningNativeUpdaterMode(os.Args)",
	} {
		if !strings.Contains(text, token) {
			t.Fatalf("single-instance startup missing %q", token)
		}
	}
}

func TestSingleInstanceWindowsIsExactPathScopedV85125(t *testing.T) {
	b, err := os.ReadFile("single_instance_windows_v85125.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	for _, token := range []string{
		"CreateMutexW",
		"ERROR_ALREADY_EXISTS",
		"QueryFullProcessImageNameW",
		"strings.EqualFold(filepath.Clean(other), current)",
		"terminateProcessTree(int(pid))",
	} {
		if !strings.Contains(text, token) {
			t.Fatalf("single-instance Windows guard missing %q", token)
		}
	}
}

func TestMediaPickerRemainsGenericOnlyV85125(t *testing.T) {
	b, err := os.ReadFile("web/feature_generic_media_picker_v85114.js")
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	for _, token := range []string{
		"providerKind(raw) === 'web'",
		"erome",
		"pickerButton.hidden = !generic",
		"Media Picker generic nu se afișează",
		"!isGenericHTTP(raw)",
	} {
		if !strings.Contains(text, token) {
			t.Fatalf("generic Media Picker boundary missing %q", token)
		}
	}
}
