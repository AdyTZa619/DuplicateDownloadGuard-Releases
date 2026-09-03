package main

import (
	"errors"
	"testing"
)

func TestClassifyMegaProblem(t *testing.T) {
	tests := []struct {
		text string
		code string
	}{
		{"API error: EOVERQUOTA", "MEGA_QUOTA"},
		{"You are not logged in", "MEGA_AUTH"},
		{"Invalid decryption key", "MEGA_KEY"},
		{"API_EBLOCKED", "MEGA_BLOCKED"},
		{"Malformed link: EARGS", "MEGA_LINK"},
		{"API_ENOENT", "MEGA_NOT_FOUND"},
		{"API_EACCESS", "ACCESS_DENIED"},
		{"API_ETOOMANY", "MEGA_RATE_LIMIT"},
		{"No space left on device", "DISK_FULL"},
		{"ETEMPUNAVAIL, try again later", "MEGA_TEMPORARY"},
		{"connection timed out", "MEGA_TIMEOUT"},
	}
	for _, test := range tests {
		if got := classifyMegaProblem(test.text, errors.New("exit status 1")); got.Code != test.code {
			t.Errorf("%q => %s, want %s", test.text, got.Code, test.code)
		}
	}
}

func TestUnknownMegaProblemIsNeverBlank(t *testing.T) {
	got := classifyMegaProblem("", nil)
	if got.Code == "" || got.Message == "" || got.Action == "" {
		t.Fatalf("incomplete problem: %#v", got)
	}
}
