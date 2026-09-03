package main

import (
	"context"
	"errors"
	"testing"
)

func TestClassifyMegaProblem(t *testing.T) {
	tests := []struct {
		text string
		code string
	}{
		{"API error: EOVERQUOTA", "MEGA_QUOTA"},
		{"Bandwidth overquota", "MEGA_QUOTA"},
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
		{"dimensiune diferită după download: local 10 bytes, remote 11 bytes", "DOWNLOAD_VERIFY_FAILED"},
		{"checksum sha256 diferit după download", "DOWNLOAD_VERIFY_FAILED"},
	}
	for _, test := range tests {
		if got := classifyMegaProblem(test.text, errors.New("exit status 1")); got.Code != test.code {
			t.Errorf("%q => %s, want %s", test.text, got.Code, test.code)
		}
	}
}

func TestMegaCancellationIsNotRetryable(t *testing.T) {
	got := classifyMegaProblem("", context.Canceled)
	if got.Code != "CANCELLED" || got.Retryable {
		t.Fatalf("unexpected cancellation classification: %#v", got)
	}
}

func TestMegaVerificationFailureIsNotRetryable(t *testing.T) {
	got := classifyMegaProblem("", errors.New("checksum sha256 diferit după download"))
	if got.Code != "DOWNLOAD_VERIFY_FAILED" || got.Retryable {
		t.Fatalf("unexpected verification classification: %#v", got)
	}
}

func TestUnknownMegaProblemIsNeverBlank(t *testing.T) {
	got := classifyMegaProblem("", nil)
	if got.Code == "" || got.Message == "" || got.Action == "" {
		t.Fatalf("incomplete problem: %#v", got)
	}
}
