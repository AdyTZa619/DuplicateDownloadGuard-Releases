package main

import "net/http"

// providerContextFirstTransportV8558 applies an already-resolved per-file
// provider context before host classification. Bunkr can return media from CDN
// hostnames that do not contain "bunkr" at all; the older transport returned
// early for those hosts and silently dropped gallery-dl's required Referer.
// Exact URL context is safe to apply regardless of hostname because it was
// captured for that exact resolved media URL and remains in memory only.
type providerContextFirstTransportV8558 struct {
	base http.RoundTripper
}

func (t *providerContextFirstTransportV8558) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil || req.URL == nil {
		return t.base.RoundTrip(req)
	}
	if ctx, ok := providerContextForURLV86(req.URL.String()); ok {
		clone := req.Clone(req.Context())
		clone.Header = req.Header.Clone()
		applyProviderHeadersV86(clone.Header, ctx.Headers)
		return t.base.RoundTrip(clone)
	}
	return t.base.RoundTrip(req)
}

func init() {
	if _, ok := http.DefaultTransport.(*providerContextFirstTransportV8558); ok {
		return
	}
	http.DefaultTransport = &providerContextFirstTransportV8558{base: http.DefaultTransport}
}
