//go:build windows

package main

import "testing"

func TestNativeVersionIsNewerV8566(t *testing.T) {
	cases := []struct {
		remote string
		local  string
		want   bool
	}{
		{"8.5.49-test.72", "8.5.49-test.71 Pro Smart Media Guard TEST", true},
		{"8.5.49-test.71", "8.5.49-test.71 Pro Smart Media Guard TEST", false},
		{"8.5.49-test.70", "8.5.49-test.71 Pro Smart Media Guard TEST", false},
		{"8.5.50-test.1", "8.5.49-test.99 Pro Smart Media Guard TEST", true},
		{"8.5.50", "8.5.50-test.99", true},
		{"8.5.49-test.99", "8.5.50", false},
	}
	for _, tc := range cases {
		if got := nativeVersionIsNewerV8566(tc.remote, tc.local); got != tc.want {
			t.Fatalf("remote=%q local=%q: got %v want %v", tc.remote, tc.local, got, tc.want)
		}
	}
}

func TestExtractNativeUpdateVersionV8566(t *testing.T) {
	got := extractNativeUpdateVersionV8566("⬇ TEST 8.5.49-test.72 disponibil")
	if got != "8.5.49-test.72" {
		t.Fatalf("unexpected version %q", got)
	}
}
