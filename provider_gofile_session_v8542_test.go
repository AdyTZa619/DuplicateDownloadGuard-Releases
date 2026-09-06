package main

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type gofileRoundTripV8542 func(*http.Request) (*http.Response, error)

func (f gofileRoundTripV8542) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func setupGofileStateTestV8542(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "gofile-state.json")
	t.Setenv("DDG_GOFILE_STATE_PATH", path)
	t.Setenv("GOFILE_TOKEN", "")
	t.Setenv("GOFILE_API_TOKEN", "")
	t.Setenv("GF_TOKEN", "")
	t.Setenv("GOFILE_WT_SALT", "")
	invalidateGoFileGuestTokenV86()
	return path
}

func TestGofileGuestTokenPersistsAcrossMemoryResetV8542(t *testing.T) {
	setupGofileStateTestV8542(t)
	if err := persistGofileGuestTokenV8542("persisted-guest"); err != nil {
		t.Fatal(err)
	}
	invalidateGoFileGuestTokenV86()
	calls := 0
	tr := gofileRoundTripV8542(func(r *http.Request) (*http.Response, error) {
		calls++
		return nil, context.DeadlineExceeded
	})
	token, err := gofileGuestTokenV86(context.Background(), tr)
	if err != nil {
		t.Fatal(err)
	}
	if token != "persisted-guest" || calls != 0 {
		t.Fatalf("token=%q calls=%d", token, calls)
	}
}

func TestGofileConfiguredTokenIsNotPersistedV8542(t *testing.T) {
	path := setupGofileStateTestV8542(t)
	t.Setenv("GOFILE_TOKEN", "configured-account")
	token, err := gofileGuestTokenV86(context.Background(), gofileRoundTripV8542(func(r *http.Request) (*http.Response, error) {
		t.Fatal("network must not be used for configured token")
		return nil, nil
	}))
	if err != nil || token != "configured-account" {
		t.Fatalf("token=%q err=%v", token, err)
	}
	if data, err := os.ReadFile(path); err == nil && strings.Contains(string(data), "configured-account") {
		t.Fatal("configured token was persisted")
	}
}

func TestGofileGuest429CreatesCooldownAndDoesNotHammerV8542(t *testing.T) {
	setupGofileStateTestV8542(t)
	calls := 0
	tr := gofileRoundTripV8542(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Header:     http.Header{"Retry-After": []string{"60"}},
			Body:       io.NopCloser(strings.NewReader(`{"status":"error-rateLimit"}`)),
			Request:    r,
		}, nil
	})
	_, err := gofileGuestTokenV86(context.Background(), tr)
	if err == nil || calls != 1 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
	if remaining := gofileGuestCooldownRemainingV8542(); remaining <= 0 {
		t.Fatal("rate-limit cooldown was not persisted")
	}
	invalidateGoFileGuestTokenV86()
	_, err = gofileGuestTokenV86(context.Background(), tr)
	if err == nil || calls != 1 {
		t.Fatalf("cooldown did not stop second POST: err=%v calls=%d", err, calls)
	}
}

func TestGofileCurrentSaltAndCandidateFallbackV8542(t *testing.T) {
	setupGofileStateTestV8542(t)
	if gofileWebsiteSaltDefaultV86 != "12af056dacea0b" {
		t.Fatalf("current salt=%q", gofileWebsiteSaltDefaultV86)
	}
	calls := 0
	client := &http.Client{Transport: gofileRoundTripV8542(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return &http.Response{StatusCode: http.StatusUnauthorized, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"status":"error-notPremium"}`)), Request: r}, nil
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"status":"ok","data":{"id":"folder","type":"folder","name":"root","children":{}}}`)), Request: r}, nil
	})}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := fetchGofileContentV86(ctx, client, "same-account-token", "folder")
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("expected website-token fallback with same account, calls=%d", calls)
	}
}

func TestGofileExplicitWrongTokenIsDistinguishedV8542(t *testing.T) {
	err := &gofileAPIErrorV8542{HTTPStatus: http.StatusUnauthorized, APIStatus: "error-wrongToken"}
	if !gofileAccountTokenRejectedV8542(err) {
		t.Fatal("wrongToken not classified as account token rejection")
	}
	if gofileWebsiteTokenCandidateErrorV8542(err) {
		t.Fatal("wrongToken incorrectly classified as website-token rotation")
	}
	generic := &gofileAPIErrorV8542{HTTPStatus: http.StatusUnauthorized, APIStatus: "error-notPremium"}
	if gofileAccountTokenRejectedV8542(generic) {
		t.Fatal("notPremium must not immediately destroy guest cache")
	}
	if !gofileWebsiteTokenCandidateErrorV8542(generic) {
		t.Fatal("notPremium should try website-token candidates first")
	}
	maskedNotFound := &gofileAPIErrorV8542{HTTPStatus: http.StatusOK, APIStatus: "error-notFound"}
	if !gofileWebsiteTokenCandidateErrorV8542(maskedNotFound) {
		t.Fatal("notFound must remain ambiguous until website-token candidates are exhausted")
	}
}

func TestGofileNotFoundFallsThroughToNextSaltV8544(t *testing.T) {
	setupGofileStateTestV8542(t)
	calls := 0
	client := &http.Client{Transport: gofileRoundTripV8542(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"status":"error-notFound"}`)), Request: r}, nil
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"status":"ok","data":{"id":"folder","type":"folder","name":"root","children":{}}}`)), Request: r}, nil
	})}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	data, err := fetchGofileContentV86(ctx, client, "same-account-token", "folder")
	if err != nil {
		t.Fatal(err)
	}
	if data.ID != "folder" || calls != 2 {
		t.Fatalf("id=%q calls=%d", data.ID, calls)
	}
}

func TestGofileRealNotFoundSurvivesCandidateExhaustionV8544(t *testing.T) {
	setupGofileStateTestV8542(t)
	calls := 0
	client := &http.Client{Transport: gofileRoundTripV8542(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"status":"error-notFound"}`)), Request: r}, nil
	})}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := fetchGofileContentV86(ctx, client, "same-account-token", "missing-folder")
	if err == nil {
		t.Fatal("expected notFound after candidate exhaustion")
	}
	_, status, ok := gofileAPIErrorInfoV8542(err)
	if !ok || status != "error-notfound" {
		t.Fatalf("unexpected final error: %v", err)
	}
	if calls != len(gofileWebsiteSaltCandidatesV8542()) {
		t.Fatalf("calls=%d candidates=%d", calls, len(gofileWebsiteSaltCandidatesV8542()))
	}
}
