package main

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestProviderHeadersPreserveRangeAndOverrideRequiredContextV86(t *testing.T) {
	dst := make(http.Header)
	dst.Set("Range", "bytes=100-199")
	dst.Set("Referer", "https://bunkr.si/a/source")
	src := make(http.Header)
	src.Set("Referer", "https://get.bunkrr.su/file/123")
	src.Set("Origin", "https://get.bunkrr.su")
	src.Set("Cookie", "session=abc")
	src.Set("Range", "bytes=0-0")
	applyProviderHeadersV86(dst, src)
	if got := dst.Get("Range"); got != "bytes=100-199" {
		t.Fatalf("Range was overwritten: %q", got)
	}
	if got := dst.Get("Referer"); got != "https://get.bunkrr.su/file/123" {
		t.Fatalf("provider Referer not applied: %q", got)
	}
	if got := dst.Get("Origin"); got != "https://get.bunkrr.su" {
		t.Fatalf("provider Origin not applied: %q", got)
	}
	if got := dst.Get("Cookie"); got != "session=abc" {
		t.Fatalf("provider cookie not applied: %q", got)
	}
}

func TestProviderContextIsMemoryOnlyAndExpiresV86(t *testing.T) {
	direct := "https://cdn.example.invalid/file.mp4?sig=123"
	headers := make(http.Header)
	headers.Set("Referer", "https://source.example.invalid/file/1")
	rememberProviderContextV86(direct, "https://source.example.invalid/a/1", headers, time.Minute)
	ctx, ok := providerContextForURLV86(direct)
	if !ok || ctx.Headers.Get("Referer") == "" {
		t.Fatal("in-memory provider context missing")
	}
	providerContextMuV86.Lock()
	v := providerContextV86[providerContextKeyV86(direct)]
	v.ExpiresAt = time.Now().Add(-time.Second)
	providerContextV86[providerContextKeyV86(direct)] = v
	providerContextMuV86.Unlock()
	if _, ok := providerContextForURLV86(direct); ok {
		t.Fatal("expired provider context remained usable")
	}
}

func TestNetscapeCookiesMatchDomainPathSecureAndHttpOnlyV86(t *testing.T) {
	data := strings.Join([]string{
		"# Netscape HTTP Cookie File",
		".gofile.io\tTRUE\t/\tTRUE\t4102444800\taccountToken\ttoken123",
		"#HttpOnly_.bunkr.si\tTRUE\t/\tTRUE\t4102444800\tsession\tsecret456",
		"example.com\tFALSE\t/private\tFALSE\t4102444800\tprivate\tvalue",
	}, "\n")
	cookies := parseNetscapeCookiesV86([]byte(data))
	if got := cookiesForURLV86("https://store1.gofile.io/path/file", cookies); got != "accountToken=token123" {
		t.Fatalf("GoFile cookie mismatch: %q", got)
	}
	if got := cookiesForURLV86("https://cdn.bunkr.si/file", cookies); got != "session=secret456" {
		t.Fatalf("HttpOnly cookie mismatch: %q", got)
	}
	if got := cookiesForURLV86("http://store1.gofile.io/path/file", cookies); got != "" {
		t.Fatalf("secure cookie leaked to HTTP: %q", got)
	}
	if got := cookiesForURLV86("https://example.com/public", cookies); got != "" {
		t.Fatalf("path-scoped cookie leaked: %q", got)
	}
}

func TestGalleryPrivateJSONLContextV86(t *testing.T) {
	direct := "https://cdn.bunkr.si/media/video.mp4?token=abc"
	source := "https://bunkr.si/a/album"
	jsonl := `[3,"` + direct + `",{"_http_headers":{"Referer":"https://get.bunkrr.su/file/987","Origin":"https://get.bunkrr.su"},"id_url":"987"}]` + "\n"
	if n := parseGalleryResolvedContextV86([]byte(jsonl), source, nil); n != 1 {
		t.Fatalf("resolved context count=%d", n)
	}
	ctx, ok := providerContextForURLV86(direct)
	if !ok {
		t.Fatal("private gallery context was not cached")
	}
	if got := ctx.Headers.Get("Referer"); got != "https://get.bunkrr.su/file/987" {
		t.Fatalf("private Referer mismatch: %q", got)
	}
	if got := ctx.Headers.Get("Origin"); got != "https://get.bunkrr.su" {
		t.Fatalf("private Origin mismatch: %q", got)
	}
}

func TestProviderHostDetectionV86(t *testing.T) {
	for _, host := range []string{"store1.gofile.io", "cdn.bunkr.si", "media.bunkrr.su", "fs-01.cyberdrop.to"} {
		if !providerSpecialHostV86(host) {
			t.Fatalf("provider host not detected: %s", host)
		}
	}
	if providerSpecialHostV86("example.com") {
		t.Fatal("unrelated host classified as provider host")
	}
}

type guestRoundTripFuncV8541 func(*http.Request) (*http.Response, error)

func (f guestRoundTripFuncV8541) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestGofileGuestTokenRequestBrowserLikeV8543(t *testing.T) {
	setupGofileStateTestV8542(t)
	defer invalidateGoFileGuestTokenV86()
	calls := 0
	tr := guestRoundTripFuncV8541(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != http.MethodPost || r.URL.String() != "https://api.gofile.io/accounts" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL)
		}
		if got := r.Header.Get("X-BL"); got != "" {
			t.Fatalf("X-BL must not be sent on guest account creation: %q", got)
		}
		if got := r.Header.Get("X-Website-Token"); got != "" {
			t.Fatalf("website token must not be sent on guest account creation: %q", got)
		}
		if got := r.Header.Get("Content-Type"); got != "" {
			t.Fatalf("content type must stay empty for bodyless POST: %q", got)
		}
		if r.Body != nil {
			body, _ := io.ReadAll(r.Body)
			if len(body) != 0 {
				t.Fatalf("guest request must be bodyless: %q", string(body))
			}
		}
		if r.Header.Get("Origin") != "https://gofile.io" || r.Header.Get("Referer") != "https://gofile.io/" {
			t.Fatalf("browser origin/referer missing: origin=%q referer=%q", r.Header.Get("Origin"), r.Header.Get("Referer"))
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"status":"ok","data":{"token":"guest-8543"}}`)),
			Request:    r,
		}, nil
	})
	token, err := gofileGuestTokenV86(context.Background(), tr)
	if err != nil {
		t.Fatal(err)
	}
	if token != "guest-8543" || calls != 1 {
		t.Fatalf("token=%q calls=%d", token, calls)
	}
}

func TestGofileGuestTokenTransportFailureDoesNotRetryV8543(t *testing.T) {
	setupGofileStateTestV8542(t)
	defer invalidateGoFileGuestTokenV86()
	calls := 0
	tr := guestRoundTripFuncV8541(func(r *http.Request) (*http.Response, error) {
		calls++
		return nil, context.DeadlineExceeded
	})
	_, err := gofileGuestTokenV86(context.Background(), tr)
	if err == nil {
		t.Fatal("expected transport failure")
	}
	if calls != 1 {
		t.Fatalf("stalled guest creation must not be retried, calls=%d", calls)
	}
	if gofileGuestAttemptTimeoutV8543 > 10*time.Second {
		t.Fatalf("guest timeout too long: %s", gofileGuestAttemptTimeoutV8543)
	}
	if !strings.Contains(err.Error(), "trec la fallback") {
		t.Fatalf("fallback intent missing from error: %v", err)
	}
}
