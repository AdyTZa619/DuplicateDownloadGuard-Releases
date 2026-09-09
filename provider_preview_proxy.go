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

const providerPreviewResponseHeaderTimeoutV85130 = 30 * time.Second

var providerPreviewClientV85130 = &http.Client{
	Transport: newProviderPreviewTransportV85130(),
}

func newProviderPreviewTransportV85130() http.RoundTripper {
	base := http.DefaultTransport
	for {
		switch wrapped := base.(type) {
		case *providerContextFirstTransportV8558:
			if wrapped.base == nil {
				break
			}
			base = wrapped.base
			continue
		case *providerAwareTransportV86:
			if wrapped.base == nil {
				break
			}
			base = wrapped.base
			continue
		}
		break
	}
	if transport, ok := base.(*http.Transport); ok {
		bounded := transport.Clone()
		bounded.ResponseHeaderTimeout = providerPreviewResponseHeaderTimeoutV85130
		base = bounded
	}
	providerAware := &providerAwareTransportV86{base: base}
	return &providerContextFirstTransportV8558{base: providerAware}
}

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
	case "GOFILE", "BUNKR", "CYBERDROP", "EROME", "GALLERY-DL":
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
	if strings.TrimSpace(best.DirectURL) == "" {
		return RemoteItem{}, fmt.Errorf("providerul nu a întors un URL media direct nou")
	}
	if _, err := providerPreviewTargetV86(best); err != nil {
		return RemoteItem{}, fmt.Errorf("URL-ul media reîmprospătat nu este utilizabil: %w", err)
	}
	return best, nil
}

func mergeProviderRemoteV85130(old, fresh RemoteItem) RemoteItem {
	fresh.ID = old.ID
	if strings.TrimSpace(fresh.Path) == "" {
		fresh.Path = old.Path
	}
	if strings.TrimSpace(fresh.Name) == "" {
		fresh.Name = old.Name
	}
	if fresh.Size <= 0 {
		fresh.Size = old.Size
	}
	if strings.TrimSpace(fresh.URL) == "" {
		fresh.URL = old.URL
	}
	if strings.TrimSpace(fresh.Source) == "" {
		fresh.Source = old.Source
	}
	if strings.TrimSpace(fresh.Extractor) == "" {
		fresh.Extractor = old.Extractor
	}
	if strings.TrimSpace(fresh.ProviderID) == "" {
		fresh.ProviderID = old.ProviderID
	}
	if strings.TrimSpace(fresh.ContentType) == "" {
		fresh.ContentType = old.ContentType
	}
	return fresh
}

func (a *App) replaceResultRemoteV86(resultID int, fresh RemoteItem) bool {
	a.mu.Lock()
	replaced := false
	for i := range a.results {
		if a.results[i].ID != resultID {
			continue
		}
		a.results[i].Remote = mergeProviderRemoteV85130(a.results[i].Remote, fresh)
		replaced = true
		break
	}
	a.mu.Unlock()
	if !replaced {
		return false
	}
	a.revision.Add(1)
	if err := a.saveResults(); err != nil {
		a.logf("Atenție: URL-ul providerului a fost reîmprospătat, dar nu a putut fi salvat: %v", err)
	}
	return true
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
	return providerPreviewClientV85130.Do(req)
}

// gallery-dl's maintained Bunkr extractor rejects these exact redirects as
// maintenance placeholders instead of treating them as the requested media.
func providerPreviewMaintenanceURLV8559(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u == nil {
		return false
	}
	path := strings.ToLower(strings.TrimSpace(u.Path))
	return strings.HasSuffix(path, "/maint.mp4") || strings.HasSuffix(path, "/maintenance-vid.mp4")
}

func providerPreviewFinalURLV8559(resp *http.Response) string {
	if resp == nil || resp.Request == nil || resp.Request.URL == nil {
		return ""
	}
	return resp.Request.URL.String()
}

func providerPreviewContentTypeV8559(resp *http.Response) string {
	if resp == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0]))
}

func providerPreviewDiagnosticV8559(resp *http.Response, item RemoteItem, refreshed bool, refreshNote string) map[string]any {
	status := 0
	if resp != nil {
		status = resp.StatusCode
	}
	finalURL := providerPreviewFinalURLV8559(resp)
	contentType := providerPreviewContentTypeV8559(resp)
	source := strings.ToUpper(strings.TrimSpace(item.Source))
	if source == "" {
		source = "REMOTE"
	}
	out := map[string]any{
		"ok":          false,
		"code":        "REMOTE_ERROR",
		"title":       "Fișierul remote nu poate fi redat",
		"detail":      "Sursa remote nu a putut fi validată.",
		"httpStatus":  status,
		"contentType": contentType,
		"finalUrl":    finalURL,
		"refreshed":   refreshed,
		"source":      source,
	}
	appendRefresh := func(detail string) string {
		if strings.TrimSpace(refreshNote) == "" {
			return detail
		}
		return detail + " Reîmprospătare: " + strings.TrimSpace(refreshNote)
	}

	switch {
	case providerPreviewMaintenanceURLV8559(finalURL):
		out["code"] = "BUNKR_MAINTENANCE"
		out["title"] = "Bunkr: serverul fișierului este în mentenanță"
		out["detail"] = appendRefresh("Bunkr a redirecționat fișierul către videoclipul său de mentenanță; conținutul original nu este disponibil acum.")
	case status == http.StatusNotFound || status == http.StatusGone:
		out["code"] = "FILE_UNAVAILABLE"
		out["title"] = "Fișier indisponibil / șters"
		out["detail"] = appendRefresh(fmt.Sprintf("Serverul a răspuns HTTP %d. Fișierul nu mai este disponibil la adresa furnizată de provider.", status))
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		out["code"] = "ACCESS_DENIED"
		out["title"] = "Acces refuzat de server"
		out["detail"] = appendRefresh(fmt.Sprintf("Serverul a răspuns HTTP %d. Linkul temporar poate fi expirat sau providerul cere un context nou de acces.", status))
	case status == http.StatusTooManyRequests:
		out["code"] = "RATE_LIMITED"
		out["title"] = "Providerul limitează temporar cererile"
		detail := "Serverul a răspuns HTTP 429. Așteaptă puțin înainte de o nouă încercare; DDG nu reextrage repetat albumul în această situație."
		if resp != nil {
			if retryAfter := strings.TrimSpace(resp.Header.Get("Retry-After")); retryAfter != "" {
				detail += " Retry-After: " + retryAfter + "."
			}
		}
		out["detail"] = appendRefresh(detail)
	case status >= 500:
		out["code"] = "REMOTE_SERVER_ERROR"
		out["title"] = "Problemă pe serverul providerului"
		out["detail"] = appendRefresh(fmt.Sprintf("Serverul remote a răspuns HTTP %d. Problema este la sursă, nu la playerul DDG.", status))
	case status >= 400:
		out["code"] = "HTTP_ERROR"
		out["title"] = "Sursa remote a refuzat fișierul"
		out["detail"] = appendRefresh(fmt.Sprintf("Serverul remote a răspuns HTTP %d.", status))
	case strings.HasPrefix(contentType, "text/html"):
		out["code"] = "HTML_INSTEAD_OF_MEDIA"
		out["title"] = "Providerul a întors o pagină, nu fișierul media"
		out["detail"] = appendRefresh("URL-ul media a răspuns cu HTML. De regulă înseamnă fișier indisponibil, link expirat sau pagină de eroare/mentenanță.")
	default:
		out["ok"] = true
		out["code"] = "READY"
		out["title"] = "Fișierul răspunde de la provider"
		detail := fmt.Sprintf("Remote HTTP %d", status)
		if contentType != "" {
			detail += " • " + contentType
		}
		out["detail"] = appendRefresh(detail + ". Dacă playerul integrat tot nu pornește, cauza probabilă este formatul/codec-ul neacceptat de playerul WebView.")
	}
	return out
}

func (a *App) handleProviderPreviewDiagnosticV8559(w http.ResponseWriter, r *http.Request, id int, res Result) {
	if _, err := providerPreviewTargetV86(res.Remote); err != nil {
		jsonOut(w, map[string]any{
			"ok": false, "code": "URL_MISSING", "title": "URL remote lipsă",
			"detail": err.Error(), "source": strings.ToUpper(strings.TrimSpace(res.Remote.Source)),
		})
		return
	}
	if remoteMediaKind(res.Remote.Name) == "other" {
		jsonOut(w, map[string]any{
			"ok": false, "code": "FORMAT_UNSUPPORTED", "title": "Format fără preview integrat",
			"detail": "Fișierul există în listă, dar extensia lui nu are player integrat în DDG.",
			"source": strings.ToUpper(strings.TrimSpace(res.Remote.Source)),
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 35*time.Second)
	defer cancel()
	probeReq := &http.Request{Method: http.MethodGet, Header: make(http.Header)}
	probeReq.Header.Set("Range", "bytes=0-0")
	item := res.Remote
	resp, err := doProviderPreviewRequestV86(ctx, probeReq, item)
	if err != nil {
		jsonOut(w, map[string]any{
			"ok": false, "code": "REMOTE_UNREACHABLE", "title": "Sursa remote nu răspunde",
			"detail": err.Error(), "source": strings.ToUpper(strings.TrimSpace(item.Source)),
		})
		return
	}

	refreshed := false
	refreshNote := ""
	if providerPreviewNeedsRefreshV86(resp.StatusCode) && providerRefreshableSourceV86(item.Source) {
		if strings.EqualFold(item.Source, "GOFILE") {
			invalidateGoFileGuestTokenV86()
		}
		if fresh, refreshErr := a.refreshProviderRemoteV86(ctx, item); refreshErr == nil {
			_ = resp.Body.Close()
			item = fresh
			a.replaceResultRemoteV86(id, fresh)
			refreshed = true
			resp, err = doProviderPreviewRequestV86(ctx, probeReq, item)
			if err != nil {
				jsonOut(w, map[string]any{
					"ok": false, "code": "REMOTE_UNREACHABLE", "title": "Sursa remote nu răspunde după reîmprospătare",
					"detail": err.Error(), "refreshed": true, "source": strings.ToUpper(strings.TrimSpace(item.Source)),
				})
				return
			}
		} else {
			refreshNote = refreshErr.Error()
		}
	}
	defer resp.Body.Close()
	jsonOut(w, providerPreviewDiagnosticV8559(resp, item, refreshed, refreshNote))
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
	if strings.TrimSpace(r.URL.Query().Get("diagnose")) == "1" {
		a.handleProviderPreviewDiagnosticV8559(w, r, id, res)
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
	if providerPreviewMaintenanceURLV8559(providerPreviewFinalURLV8559(resp)) {
		http.Error(w, "Bunkr: serverul fișierului este în mentenanță; conținutul original nu este disponibil acum", http.StatusServiceUnavailable)
		return
	}
	if strings.HasPrefix(providerPreviewContentTypeV8559(resp), "text/html") {
		http.Error(w, "Providerul a returnat HTML în locul fișierului media", http.StatusBadGateway)
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
