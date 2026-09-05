package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func providerPreviewTargetV86(item RemoteItem) (string, error) {
	target := strings.TrimSpace(item.DirectURL)
	if target == "" {
		target = strings.TrimSpace(item.URL)
	}
	u, err := url.Parse(target)
	if err != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("sursa remote nu are URL HTTP utilizabil")
	}
	return u.String(), nil
}

func buildProviderPreviewRequestV86(ctx context.Context, incoming *http.Request, item RemoteItem) (*http.Request, error) {
	target, err := providerPreviewTargetV86(item)
	if err != nil {
		return nil, err
	}
	method := http.MethodGet
	if incoming != nil && incoming.Method == http.MethodHead {
		method = http.MethodHead
	}
	req, err := http.NewRequestWithContext(ctx, method, target, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 DuplicateDownloadGuard/ProviderPreview")
	req.Header.Set("Accept", "*/*")
	if page := strings.TrimSpace(item.URL); page != "" && page != target {
		if u, parseErr := url.Parse(page); parseErr == nil && (u.Scheme == "http" || u.Scheme == "https") {
			req.Header.Set("Referer", page)
		}
	}
	if incoming != nil {
		for _, key := range []string{"Range", "If-Range", "If-None-Match", "If-Modified-Since"} {
			if value := strings.TrimSpace(incoming.Header.Get(key)); value != "" {
				req.Header.Set(key, value)
			}
		}
	}
	return req, nil
}

func copyProviderPreviewResponseHeadersV86(dst, src http.Header) {
	for _, key := range []string{
		"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges",
		"ETag", "Last-Modified", "Cache-Control",
	} {
		if value := src.Get(key); value != "" {
			dst.Set(key, value)
		}
	}
	// Deliberately do not forward Content-Disposition: attachment. The route is
	// a preview surface; forwarding it would make browsers download media that
	// should be rendered inline.
	dst.Set("X-Content-Type-Options", "nosniff")
}

func providerRefreshableSourceV86(source string) bool {
	switch strings.ToUpper(strings.TrimSpace(source)) {
	case "GOFILE", "BUNKR", "CYBERDROP", "GALLERY-DL":
		return true
	default:
		return false
	}
}

func providerRemoteMatchRankV86(old, fresh RemoteItem) int {
	if old.ProviderID != "" && fresh.ProviderID != "" && strings.EqualFold(strings.TrimSpace(old.ProviderID), strings.TrimSpace(fresh.ProviderID)) {
		return 3
	}
	if old.Path != "" && fresh.Path != "" && strings.EqualFold(filepathSlashV86(old.Path), filepathSlashV86(fresh.Path)) {
		return 2
	}
	if old.Name != "" && fresh.Name != "" && strings.EqualFold(strings.TrimSpace(old.Name), strings.TrimSpace(fresh.Name)) {
		return 1
	}
	return 0
}

func filepathSlashV86(path string) string {
	return strings.ReplaceAll(strings.TrimSpace(path), "\\", "/")
}

func (a *App) refreshProviderRemoteV86(ctx context.Context, old RemoteItem) (RemoteItem, error) {
	if !providerRefreshableSourceV86(old.Source) || strings.TrimSpace(old.URL) == "" {
		return RemoteItem{}, fmt.Errorf("providerul nu poate reîmprospăta URL-ul remote")
	}
	refreshCtx, cancel := context.WithTimeout(ctx, 75*time.Second)
	defer cancel()
	items, err := a.probeGalleryDLRichV86(refreshCtx, old.URL)
	if err != nil {
		return RemoteItem{}, err
	}
	bestRank := 0
	bestCount := 0
	best := RemoteItem{}
	for _, fresh := range items {
		rank := providerRemoteMatchRankV86(old, fresh)
		if rank > bestRank {
			bestRank = rank
			bestCount = 1
			best = fresh
		} else if rank > 0 && rank == bestRank {
			bestCount++
		}
	}
	if bestRank == 0 {
		return RemoteItem{}, fmt.Errorf("fișierul nu a mai fost găsit în sursa remote")
	}
	if bestCount > 1 {
		return RemoteItem{}, fmt.Errorf("reîmprospătarea este ambiguă: %d fișiere corespund aceleiași identități", bestCount)
	}
	return best, nil
}

func (a *App) replaceResultRemoteV86(resultID int, fresh RemoteItem) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for i := range a.results {
		if a.results[i].ID != resultID {
			continue
		}
		fresh.ID = a.results[i].Remote.ID
		a.results[i].Remote = fresh
		return
	}
}

func invalidateGoFileGuestTokenV86() {
	gofileTokenMuV86.Lock()
	gofileTokenV86 = ""
	gofileTokenAtV86 = time.Time{}
	gofileTokenMuV86.Unlock()
}

func providerPreviewNeedsRefreshV86(status int) bool {
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusGone:
		return true
	default:
		return false
	}
}

func doProviderPreviewRequestV86(ctx context.Context, incoming *http.Request, item RemoteItem) (*http.Response, error) {
	req, err := buildProviderPreviewRequestV86(ctx, incoming, item)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 0}
	return client.Do(req)
}

func (a *App) handleProviderPreviewMediaV86(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "metodă neacceptată", http.StatusMethodNotAllowed)
		return
	}
	id, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("id")))
	if err != nil || id <= 0 {
		http.Error(w, "ID rezultat invalid", http.StatusBadRequest)
		return
	}
	res, ok := a.resultByID(id)
	if !ok {
		http.Error(w, "Rezultatul nu mai există", http.StatusNotFound)
		return
	}
	if strings.EqualFold(res.Remote.Source, "MEGA") {
		http.Error(w, "MEGA folosește motorul de preview dedicat", http.StatusBadRequest)
		return
	}
	if remoteMediaKind(res.Remote.Name) == "other" {
		http.Error(w, "Formatul nu are preview media integrat", http.StatusUnsupportedMediaType)
		return
	}
	if _, err := providerPreviewTargetV86(res.Remote); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	item := res.Remote
	resp, err := doProviderPreviewRequestV86(r.Context(), r, item)
	if err != nil {
		http.Error(w, "Sursa remote nu răspunde pentru preview", http.StatusBadGateway)
		return
	}
	if providerPreviewNeedsRefreshV86(resp.StatusCode) && providerRefreshableSourceV86(item.Source) {
		_ = resp.Body.Close()
		if strings.EqualFold(item.Source, "GOFILE") {
			invalidateGoFileGuestTokenV86()
		}
		if fresh, refreshErr := a.refreshProviderRemoteV86(r.Context(), item); refreshErr == nil {
			item = fresh
			a.replaceResultRemoteV86(id, fresh)
			resp, err = doProviderPreviewRequestV86(r.Context(), r, item)
			if err != nil {
				http.Error(w, "Sursa remote nu răspunde după reîmprospătare", http.StatusBadGateway)
				return
			}
		} else {
			http.Error(w, "Linkul remote a expirat și nu a putut fi reîmprospătat", http.StatusBadGateway)
			return
		}
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		http.Error(w, fmt.Sprintf("Sursa remote a răspuns HTTP %d", resp.StatusCode), http.StatusBadGateway)
		return
	}

	copyProviderPreviewResponseHeadersV86(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	if r.Method == http.MethodHead || resp.StatusCode == http.StatusNotModified {
		return
	}
	buf := make([]byte, 1024*1024)
	if _, err := io.CopyBuffer(w, resp.Body, buf); err != nil {
		// Browser/player cancellation is normal during seeking or row changes.
		return
	}
}
