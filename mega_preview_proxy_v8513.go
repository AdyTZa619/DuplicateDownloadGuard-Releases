package main

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

var megaPreviewProxyV8513 struct {
	sync.Mutex
	addr   string
	server *http.Server
	err    error
}

func isLoopbackPreviewHostV8513(host string) bool {
	host = strings.TrimSpace(strings.Trim(host, "[]"))
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func ensureMegaPreviewProxyV8513() (string, error) {
	megaPreviewProxyV8513.Lock()
	defer megaPreviewProxyV8513.Unlock()
	if megaPreviewProxyV8513.addr != "" {
		return megaPreviewProxyV8513.addr, nil
	}
	if megaPreviewProxyV8513.err != nil {
		return "", megaPreviewProxyV8513.err
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		megaPreviewProxyV8513.err = err
		return "", err
	}
	transport := &http.Transport{
		Proxy:                 nil,
		DisableKeepAlives:     true,
		DisableCompression:    true,
		ResponseHeaderTimeout: 12 * time.Second,
	}
	client := &http.Client{
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/stream", func(w http.ResponseWriter, r *http.Request) {
		raw := strings.TrimSpace(r.URL.Query().Get("u"))
		u, err := url.Parse(raw)
		if err != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") || !isLoopbackPreviewHostV8513(u.Hostname()) {
			http.Error(w, "upstream preview invalid", http.StatusBadRequest)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		upReq, err := http.NewRequestWithContext(r.Context(), r.Method, u.String(), nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		for _, h := range []string{"Range", "If-Range", "If-Modified-Since", "If-None-Match"} {
			if v := r.Header.Get(h); v != "" {
				upReq.Header.Set(h, v)
			}
		}
		upReq.Header.Set("User-Agent", "DuplicateDownloadGuard/8.5.13-preview-proxy")
		upReq.Header.Set("Connection", "close")
		resp, err := client.Do(upReq)
		if err != nil {
			if errors.Is(err, contextCanceledErrV8513(r.Context())) {
				return
			}
			http.Error(w, "MEGA proxy: "+err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		for _, h := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "ETag", "Last-Modified", "Content-Disposition", "Cache-Control"} {
			if v := resp.Header.Get(h); v != "" {
				w.Header().Set(h, v)
			}
		}
		w.Header().Set("X-DDG-MEGA-Proxy", "v8513")
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
		IdleTimeout:       15 * time.Second,
	}
	megaPreviewProxyV8513.addr = "http://" + ln.Addr().String()
	megaPreviewProxyV8513.server = srv
	go func() {
		_ = srv.Serve(ln)
	}()
	return megaPreviewProxyV8513.addr, nil
}

func contextCanceledErrV8513(ctx interface{ Err() error }) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}

func wrapMegaPreviewProxyURLV8513(upstream string) (string, error) {
	upstream = strings.TrimSpace(upstream)
	if upstream == "" {
		return "", errors.New("MEGA preview upstream lipsă")
	}
	base, err := ensureMegaPreviewProxyV8513()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/stream?u=%s", base, url.QueryEscape(upstream)), nil
}
