package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"
)

// megaPreviewMediaServerV8523 follows the same basic model used by established
// media servers: one long-lived local HTTP server, one logical media URL per
// selection, Range passthrough, and explicit cancellation of the previous
// stream when the selection changes. MEGAcmd remains the upstream streaming
// server; DDG only owns the browser-facing connection lifecycle.
type megaPreviewMediaServerV8523 struct {
	mu         sync.Mutex
	listener   net.Listener
	server     *http.Server
	transport  *http.Transport
	baseURL    string
	generation uint64
	upstream   *url.URL
	active     map[uint64]context.CancelFunc
	nextReq    uint64
}

func newMegaPreviewMediaServerV8523() *megaPreviewMediaServerV8523 {
	return &megaPreviewMediaServerV8523{active: map[uint64]context.CancelFunc{}}
}

var ddgMegaPreviewMediaServerV8523 = newMegaPreviewMediaServerV8523()

func megaPreviewLoopbackURLV8523(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return nil, errors.New("URL MEGA WebDAV invalid")
	}
	host := strings.TrimSpace(strings.Trim(u.Hostname(), "[]"))
	if strings.EqualFold(host, "localhost") {
		return u, nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return nil, errors.New("upstream-ul MEGA preview nu este loopback")
	}
	return u, nil
}

func (s *megaPreviewMediaServerV8523) ensureStarted() error {
	if s == nil {
		return errors.New("server media MEGA lipsa")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.server != nil && s.listener != nil && s.baseURL != "" {
		return nil
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	transport := &http.Transport{
		Proxy:              nil,
		DisableKeepAlives:  true,
		DisableCompression: true,
		IdleConnTimeout:    5 * time.Second,
	}
	srv := &http.Server{
		Handler:           http.HandlerFunc(s.handle),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       5 * time.Second,
	}
	// The dedicated media server never needs HTTP keep-alive. Each browser Range
	// request gets a clean local connection, which avoids old video requests
	// occupying Chromium's connection pool after the row changes.
	srv.SetKeepAlivesEnabled(false)

	s.listener = ln
	s.server = srv
	s.transport = transport
	s.baseURL = "http://" + ln.Addr().String()
	go func() {
		_ = srv.Serve(ln)
	}()
	return nil
}

func (s *megaPreviewMediaServerV8523) activate(upstream string) (string, error) {
	u, err := megaPreviewLoopbackURLV8523(upstream)
	if err != nil {
		return "", err
	}
	if err := s.ensureStarted(); err != nil {
		return "", err
	}

	s.mu.Lock()
	for id, cancel := range s.active {
		cancel()
		delete(s.active, id)
	}
	if s.transport != nil {
		s.transport.CloseIdleConnections()
	}
	s.generation++
	gen := s.generation
	copyURL := *u
	s.upstream = &copyURL
	base := s.baseURL
	s.mu.Unlock()

	name := path.Base(u.Path)
	if name == "" || name == "." || name == "/" {
		name = "media"
	}
	return fmt.Sprintf("%s/media/%d/%s", base, gen, url.PathEscape(name)), nil
}

func (s *megaPreviewMediaServerV8523) stopCurrent() {
	if s == nil {
		return
	}
	s.mu.Lock()
	for id, cancel := range s.active {
		cancel()
		delete(s.active, id)
	}
	s.generation++
	s.upstream = nil
	if s.transport != nil {
		s.transport.CloseIdleConnections()
	}
	s.mu.Unlock()
}

func (s *megaPreviewMediaServerV8523) handle(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Private-Network", "true")
	w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Range, If-Range, If-Modified-Since, If-None-Match, Content-Type")
	w.Header().Set("X-DDG-MEGA-Media", "v8523")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	rest := strings.TrimPrefix(r.URL.Path, "/media/")
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		http.Error(w, "media generation missing", http.StatusBadRequest)
		return
	}
	gen, err := strconv.ParseUint(parts[0], 10, 64)
	if err != nil {
		http.Error(w, "media generation invalid", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	if s.upstream == nil || gen != s.generation {
		s.mu.Unlock()
		http.Error(w, "preview superseded", http.StatusGone)
		return
	}
	upstreamCopy := *s.upstream
	ctx, cancel := context.WithCancel(r.Context())
	s.nextReq++
	reqID := s.nextReq
	s.active[reqID] = cancel
	transport := s.transport
	s.mu.Unlock()

	defer func() {
		cancel()
		s.mu.Lock()
		delete(s.active, reqID)
		s.mu.Unlock()
	}()

	upReq, err := http.NewRequestWithContext(ctx, r.Method, upstreamCopy.String(), nil)
	if err != nil {
		http.Error(w, "preview request invalid", http.StatusBadGateway)
		return
	}
	for _, h := range []string{"Range", "If-Range", "If-Modified-Since", "If-None-Match", "Accept"} {
		if v := r.Header.Get(h); v != "" {
			upReq.Header.Set(h, v)
		}
	}
	upReq.Header.Set("User-Agent", "DuplicateDownloadGuard/8.5.23-media-server")
	upReq.Header.Set("Connection", "close")

	client := &http.Client{
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Do(upReq)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		http.Error(w, "MEGA stream indisponibil: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	for _, h := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "ETag", "Last-Modified", "Content-Disposition"} {
		if v := resp.Header.Get(h); v != "" {
			w.Header().Set(h, v)
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "close")
	w.WriteHeader(resp.StatusCode)
	if r.Method == http.MethodHead {
		return
	}
	buf := make([]byte, 256*1024)
	_, _ = io.CopyBuffer(w, resp.Body, buf)
}

func (a *App) browserReadyMegaPreviewV8523(streamURL, mode string, started time.Time) (string, string, time.Duration, error) {
	mediaURL, err := ddgMegaPreviewMediaServerV8523.activate(streamURL)
	if err != nil {
		// The new browser-facing layer must never remove the known-good 8.5.9
		// direct WebDAV fallback.
		if a != nil {
			a.logf("MEGA media server v8523 indisponibil; folosesc WebDAV direct: %v", err)
		}
		return streamURL, mode, time.Since(started), nil
	}
	if a != nil {
		a.logf("MEGA media server v8523: stream browser activat")
	}
	return mediaURL, mode, time.Since(started), nil
}
