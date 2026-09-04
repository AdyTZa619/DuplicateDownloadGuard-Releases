package main

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestClassifyMegaProblem(t *testing.T) {
	tests := []struct {
		text string
		code string
	}{
		{"API error: EOVERQUOTA", "MEGA_QUOTA"},
		{"API_EOVERQUOTA", "MEGA_QUOTA"},
		{"Bandwidth overquota", "MEGA_QUOTA"},
		{"HTTP 509 bandwidth limit exceeded", "MEGA_QUOTA"},
		{"You are not logged in", "MEGA_AUTH"},
		{"Invalid decryption key", "MEGA_KEY"},
		{"API_EBLOCKED", "MEGA_BLOCKED"},
		{"API_EBUSINESSPASTDUE", "MEGA_BLOCKED"},
		{"Malformed link: EARGS", "MEGA_LINK"},
		{"API_ENOENT", "MEGA_NOT_FOUND"},
		{"API_EACCESS", "ACCESS_DENIED"},
		{"API_ETOOMANY", "MEGA_RATE_LIMIT"},
		{"HTTP 429 too many requests", "MEGA_RATE_LIMIT"},
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

func TestMegaRetrySecondsV85(t *testing.T) {
	tests := []struct {
		text string
		want int64
	}{
		{"retry after 45 seconds", 45},
		{"Quota exceeded; try again in 12 minutes", 12 * 60},
		{"available again after 2 hours", 2 * 3600},
		{"quota reset 01:30", 90},
		{"retry after 01:02:03", 3723},
	}
	for _, test := range tests {
		if got := megaRetrySecondsV85(test.text); got != test.want {
			t.Errorf("%q => %d, want %d", test.text, got, test.want)
		}
	}
}

func TestMegaQuotaActionIncludesRetryHintWhenAvailable(t *testing.T) {
	got := classifyMegaProblem("API_EOVERQUOTA; retry after 17 minutes", errors.New("exit status 1"))
	if got.Code != "MEGA_QUOTA" {
		t.Fatalf("unexpected code: %#v", got)
	}
	if !strings.Contains(got.Action, "17 minute") {
		t.Fatalf("retry hint missing from action: %q", got.Action)
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
