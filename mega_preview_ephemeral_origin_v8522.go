package main

import (
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const megaPreviewOriginIdleV8522 = 2 * time.Minute

type megaPreviewOriginV8522 struct {
	mu        sync.Mutex
	server    *http.Server
	transport *http.Transport
	idle      *time.Timer
	active    int
	generation uint64
	closed    bool
}

func (o *megaPreviewOriginV8522) beginRequest() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closed {
		return false
	}
	o.generation++
	if o.idle != nil {
		o.idle.Stop()
		o.idle = nil
	}
	o.active++
	return true
}

func (o *megaPreviewOriginV8522) endRequest() {
	o.mu.Lock()
	if o.active > 0 {
		o.active--
	}
	if o.active == 0 && !o.closed {
		o.armIdleLocked()
	}
	o.mu.Unlock()
}

func (o *megaPreviewOriginV8522) armIdleLocked() {
	o.generation++
	generation := o.generation
	if o.idle != nil {
		o.idle.Stop()
	}
	o.idle = time.AfterFunc(megaPreviewOriginIdleV8522, func() {
		o.closeIfIdle(generation)
	})
}

func (o *megaPreviewOriginV8522) closeIfIdle(generation uint64) {
	o.mu.Lock()
	if o.closed || o.active != 0 || o.generation != generation {
		o.mu.Unlock()
		return
	}
	o.closed = true
	srv := o.server
	transport := o.transport
	o.mu.Unlock()

	if transport != nil {
		transport.CloseIdleConnections()
	}
	if srv != nil {
		_ = srv.Close()
	}
}

func megaPreviewLoopbackURLV8522(raw string) (*url.URL, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return nil, false
	}
	host := strings.TrimSpace(strings.Trim(u.Hostname(), "[]"))
	if strings.EqualFold(host, "localhost") {
		return u, true
	}
	ip := net.ParseIP(host)
	return u, ip != nil && ip.IsLoopback()
}

// startMegaPreviewEphemeralOriginV8522 gives every selected MEGA item a fresh
// browser origin (a new 127.0.0.1 port). Chromium keeps a small connection pool
// per origin. A video element can leave several Range requests alive after the
// row changes; reusing one local port therefore lets old streams queue the next
// media request. A new port per selection makes those old connections irrelevant
// to the next player without changing MEGAcmd/session/WebDAV behavior.
func startMegaPreviewEphemeralOriginV8522(upstream string) (string, error) {
	u, ok := megaPreviewLoopbackURLV8522(upstream)
	if !ok {
		return "", &url.Error{Op: "preview", URL: "loopback", Err: errMegaPreviewOriginV8522}
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}

	transport := &http.Transport{
		Proxy:                 nil,
		DisableKeepAlives:     true,
		DisableCompression:    true,
		ResponseHeaderTimeout: 12 * time.Second,
		IdleConnTimeout:       5 * time.Second,
	}
	client := &http.Client{
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	origin := &megaPreviewOriginV8522{transport: transport}
	mux := http.NewServeMux()
	mux.HandleFunc("/stream", func(w http.ResponseWriter, r *http.Request) {
		if !origin.beginRequest() {
			http.Error(w, "preview origin closed", http.StatusGone)
			return
		}
		defer origin.endRequest()

		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		upReq, reqErr := http.NewRequestWithContext(r.Context(), r.Method, u.String(), nil)
		if reqErr != nil {
			http.Error(w, "preview request invalid", http.StatusBadGateway)
			return
		}
		for _, h := range []string{"Range", "If-Range", "If-Modified-Since", "If-None-Match"} {
			if v := r.Header.Get(h); v != "" {
				upReq.Header.Set(h, v)
			}
		}
		upReq.Header.Set("User-Agent", "DuplicateDownloadGuard/8.5.22-ephemeral-origin")
		upReq.Header.Set("Connection", "close")

		resp, reqErr := client.Do(upReq)
		if reqErr != nil {
			if r.Context().Err() != nil {
				return
			}
			http.Error(w, "MEGA stream indisponibil: "+reqErr.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		for _, h := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "ETag", "Last-Modified", "Content-Disposition", "Cache-Control"} {
			if v := resp.Header.Get(h); v != "" {
				w.Header().Set(h, v)
			}
		}
		w.Header().Set("X-DDG-MEGA-Origin", "v8522")
		w.Header().Set("Connection", "close")
		w.WriteHeader(resp.StatusCode)
		if r.Method == http.MethodHead {
			return
		}
		buf := make([]byte, 256*1024)
		_, _ = io.CopyBuffer(w, resp.Body, buf)
	})

	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       5 * time.Second,
	}
	origin.mu.Lock()
	origin.server = srv
	origin.armIdleLocked()
	origin.mu.Unlock()

	go func() {
		_ = srv.Serve(ln)
	}()
	return "http://" + ln.Addr().String() + "/stream", nil
}

var errMegaPreviewOriginV8522 = &megaPreviewOriginErrorV8522{}

type megaPreviewOriginErrorV8522 struct{}

func (*megaPreviewOriginErrorV8522) Error() string { return "MEGA preview upstream must be loopback" }

func (a *App) browserReadyMegaPreviewV8522(streamURL, mode string, started time.Time) (string, string, time.Duration, error) {
	isolatedURL, err := startMegaPreviewEphemeralOriginV8522(streamURL)
	if err != nil {
		// Never make the new isolation layer a new point of failure. If Windows
		// cannot allocate a local listener, preserve the exact v8.5.9 URL/path.
		a.logf("MEGA preview: port izolat indisponibil; folosesc URL-ul WebDAV direct: %v", err)
		return streamURL, mode, time.Since(started), nil
	}
	return isolatedURL, mode, time.Since(started), nil
}
