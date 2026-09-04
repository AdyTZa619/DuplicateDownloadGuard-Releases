package main

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var previewProxyTransportV860 = &http.Transport{
	Proxy:                 nil,
	DisableCompression:    true,
	MaxIdleConns:          16,
	MaxIdleConnsPerHost:   8,
	IdleConnTimeout:       45 * time.Second,
	ResponseHeaderTimeout: 15 * time.Second,
}

var previewProxyClientV860 = &http.Client{Transport: previewProxyTransportV860}

func previewProxyURLV860(id int) string {
	return "/api/remote-preview/proxy?id=" + strconv.Itoa(id)
}

func isLoopbackPreviewUpstreamV860(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return false
	}
	host := strings.TrimSpace(u.Hostname())
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (a *App) megaPreviewUpstreamV860(id int) (string, string, error) {
	if a == nil || id <= 0 {
		return "", "", errors.New("ID preview invalid")
	}
	res, ok := a.resultByID(id)
	if !ok {
		return "", "", errors.New("rezultatul nu mai există")
	}
	if !strings.EqualFold(res.Remote.Source, "MEGA") {
		return "", "", errors.New("proxy-ul local este disponibil numai pentru MEGA")
	}
	if streamURL, mode, ok := a.tryMegaPreviewUICacheV854(res.Remote); ok {
		if !isLoopbackPreviewUpstreamV860(streamURL) {
			return "", "", errors.New("WebDAV MEGA nu este pe loopback; proxy refuzat")
		}
		return streamURL, mode, nil
	}
	streamURL, mode, _, err := a.startMegaPreviewForUIV854(res.Remote, false)
	if err != nil {
		return "", mode, err
	}
	if !isLoopbackPreviewUpstreamV860(streamURL) {
		return "", mode, errors.New("WebDAV MEGA nu este pe loopback; proxy refuzat")
	}
	return streamURL, mode, nil
}

func copyPreviewProxyHeadersV860(dst, src http.Header) {
	for _, key := range []string{
		"Accept-Ranges",
		"Cache-Control",
		"Content-Disposition",
		"Content-Length",
		"Content-Range",
		"Content-Type",
		"ETag",
		"Last-Modified",
	} {
		if v := src.Get(key); v != "" {
			dst.Set(key, v)
		}
	}
	dst.Set("X-DDG-Preview-Proxy", "1")
}

func forwardMegaPreviewV860(w http.ResponseWriter, r *http.Request, upstream, mode string) error {
	if !isLoopbackPreviewUpstreamV860(upstream) {
		return errors.New("upstream preview invalid")
	}
	method := http.MethodGet
	if r.Method == http.MethodHead {
		method = http.MethodHead
	}
	req, err := http.NewRequestWithContext(r.Context(), method, upstream, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "DuplicateDownloadGuard/8.6 PreviewProxy")
	req.Header.Set("Accept", "*/*")
	for _, key := range []string{"Range", "If-Range", "If-None-Match", "If-Modified-Since"} {
		if v := r.Header.Get(key); v != "" {
			req.Header.Set(key, v)
		}
	}
	resp, err := previewProxyClientV860.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	copyPreviewProxyHeadersV860(w.Header(), resp.Header)
	w.Header().Set("X-DDG-Preview-Mode", mode)
	if w.Header().Get("Accept-Ranges") == "" {
		w.Header().Set("Accept-Ranges", "bytes")
	}
	w.WriteHeader(resp.StatusCode)
	if method == http.MethodHead || resp.StatusCode == http.StatusNotModified {
		return nil
	}
	buf := make([]byte, 512*1024)
	_, err = io.CopyBuffer(w, resp.Body, buf)
	return err
}

func (a *App) handleRemotePreviewProxyV860(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "GET/HEAD necesar", http.StatusMethodNotAllowed)
		return
	}
	id, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("id")))
	if err != nil || id <= 0 {
		http.Error(w, "ID preview invalid", http.StatusBadRequest)
		return
	}
	upstream, mode, err := a.megaPreviewUpstreamV860(id)
	if err != nil {
		http.Error(w, "Preview MEGA indisponibil: "+err.Error(), http.StatusBadGateway)
		return
	}
	if err := forwardMegaPreviewV860(w, r, upstream, mode); err != nil {
		a.logf("MEGA Preview Proxy: %v", err)
		// If headers are already committed, an HTTP status cannot be changed.
		// The browser will surface the interrupted stream and UI can retry once.
		return
	}
}

func previewProxyDebugV860(id int, upstream, mode string) string {
	return fmt.Sprintf("id=%d mode=%s upstream=%s", id, mode, upstream)
}
