package main

import (
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var megaStreamProxyTransportV8512 = &http.Transport{
	Proxy:                 nil,
	DisableKeepAlives:     true,
	ForceAttemptHTTP2:     false,
	MaxIdleConns:          0,
	MaxIdleConnsPerHost:   0,
	IdleConnTimeout:       time.Second,
	ResponseHeaderTimeout: 12 * time.Second,
	ExpectContinueTimeout: time.Second,
	DialContext: (&net.Dialer{
		Timeout:   3 * time.Second,
		KeepAlive: -1,
	}).DialContext,
}

var megaStreamProxyClientV8512 = &http.Client{Transport: megaStreamProxyTransportV8512}

func validateMegaStreamTargetV8512(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u == nil {
		return nil, errors.New("URL MEGA stream invalid")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, errors.New("schema MEGA stream invalidă")
	}
	host := strings.ToLower(strings.TrimSpace(u.Hostname()))
	if host != "127.0.0.1" && host != "localhost" && host != "::1" {
		return nil, errors.New("proxy-ul acceptă numai WebDAV local MEGAcmd")
	}
	return u, nil
}

func copyMegaProxyHeaderV8512(dst, src http.Header, key string) {
	for _, v := range src.Values(key) {
		dst.Add(key, v)
	}
}

func (a *App) handleMegaStreamProxyV8512(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "GET/HEAD necesar", http.StatusMethodNotAllowed)
		return
	}
	target, err := validateMegaStreamTargetV8512(r.URL.Query().Get("u"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Do not turn this helper into a generic localhost proxy. The target must be
	// on the same local MEGAcmd WebDAV listener currently tracked by DDG.
	a.previewMu.Lock()
	st := a.preview
	a.previewMu.Unlock()
	if !st.Active || strings.TrimSpace(st.StreamURL) == "" {
		http.Error(w, "MEGA preview nu este activ", http.StatusConflict)
		return
	}
	root, err := validateMegaStreamTargetV8512(st.StreamURL)
	if err != nil || !strings.EqualFold(root.Host, target.Host) {
		http.Error(w, "ținta nu aparține WebDAV-ului MEGA activ", http.StatusForbidden)
		return
	}

	started := time.Now()
	up, err := http.NewRequestWithContext(r.Context(), r.Method, target.String(), nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	up.Header.Set("Accept-Encoding", "identity")
	up.Header.Set("User-Agent", "DuplicateDownloadGuard/8.5 MEGA proxy")
	for _, key := range []string{"Range", "If-Range", "If-None-Match", "If-Modified-Since"} {
		copyMegaProxyHeaderV8512(up.Header, r.Header, key)
	}

	resp, err := megaStreamProxyClientV8512.Do(up)
	if err != nil {
		if r.Context().Err() == nil {
			a.logf("MEGA PROXY: upstream eșuat după %d ms: %v", time.Since(started).Milliseconds(), err)
			http.Error(w, "MEGA WebDAV nu a răspuns: "+err.Error(), http.StatusBadGateway)
		}
		return
	}
	defer resp.Body.Close()

	for _, key := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "ETag", "Last-Modified", "Content-Disposition"} {
		copyMegaProxyHeaderV8512(w.Header(), resp.Header, key)
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-DDG-Mega-Proxy", "1")
	w.WriteHeader(resp.StatusCode)
	if r.Method == http.MethodHead {
		a.logf("MEGA PROXY HEAD: %d în %d ms", resp.StatusCode, time.Since(started).Milliseconds())
		return
	}

	buf := make([]byte, 256*1024)
	_, copyErr := io.CopyBuffer(w, resp.Body, buf)
	if copyErr != nil && r.Context().Err() == nil {
		a.logf("MEGA PROXY: stream întrerupt după %d ms: %v", time.Since(started).Milliseconds(), copyErr)
	}
}
