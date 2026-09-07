package main

import (
	"bytes"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// TEST v8.5.99: diagnostic local-preview path.
// The normal preview remains ServeFile-based. This wrapper only measures the
// real phases (authorization, stat/open-to-first-byte, transfer) and writes
// compact evidence into the existing DDG journal. A separate buffered image
// endpoint is available as an explicit alternative; it is never selected
// automatically by the backend and never scans HDD/MEGA/JDownloader.

type localPreviewTimingWriterV8599 struct {
	http.ResponseWriter
	status    int
	bytes     int64
	firstBody time.Time
}

func (w *localPreviewTimingWriterV8599) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *localPreviewTimingWriterV8599) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	if w.firstBody.IsZero() {
		w.firstBody = time.Now()
	}
	n, err := w.ResponseWriter.Write(p)
	w.bytes += int64(n)
	return n, err
}

func previewMSV8599(d time.Duration) int64 {
	return d.Milliseconds()
}

func previewRangeLabelV8599(r *http.Request) string {
	x := strings.TrimSpace(r.Header.Get("Range"))
	if x == "" {
		return "-"
	}
	if len(x) > 96 {
		x = x[:96] + "…"
	}
	return x
}

func (a *App) handleLocalPreviewDiagV8599(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	p := strings.TrimSpace(r.URL.Query().Get("path"))
	if p == "" {
		http.Error(w, "path lipsă", 400)
		return
	}

	authAt := time.Now()
	allowed := a.localPathAllowed(p)
	authMS := previewMSV8599(time.Since(authAt))
	if !allowed {
		a.logf("LOCAL PREVIEW diag DIRECT REFUZAT: auth=%dms path=%s", authMS, p)
		http.Error(w, "Fișier local neautorizat", 403)
		return
	}

	statAt := time.Now()
	st, err := os.Stat(p)
	statMS := previewMSV8599(time.Since(statAt))
	if err != nil || st.IsDir() {
		a.logf("LOCAL PREVIEW diag DIRECT LIPSEȘTE: auth=%dms stat=%dms err=%v path=%s", authMS, statMS, err, p)
		http.Error(w, "Fișierul local nu mai există", 404)
		return
	}

	tw := &localPreviewTimingWriterV8599{ResponseWriter: w}
	tw.Header().Set("Content-Disposition", "inline")
	tw.Header().Set("Cache-Control", "private, max-age=60")
	serveAt := time.Now()
	http.ServeFile(tw, r, p)
	serveMS := previewMSV8599(time.Since(serveAt))
	totalMS := previewMSV8599(time.Since(started))
	firstBodyMS := int64(-1)
	if !tw.firstBody.IsZero() {
		firstBodyMS = previewMSV8599(tw.firstBody.Sub(serveAt))
	}
	status := tw.status
	if status == 0 {
		status = http.StatusOK
	}
	kind := remoteMediaKind(filepath.Base(p))

	// Images are logged on every request so fast and slow files can be compared.
	// Video/audio Range traffic is intentionally quiet unless it is slow/error.
	if kind == "image" || totalMS >= 500 || status >= 400 || authMS >= 100 || statMS >= 100 {
		a.logf("LOCAL PREVIEW diag DIRECT: total=%dms auth=%dms stat=%dms first-byte=%dms serve=%dms status=%d sent=%d size=%d kind=%s range=%s path=%s",
			totalMS, authMS, statMS, firstBodyMS, serveMS, status, tw.bytes, st.Size(), kind, previewRangeLabelV8599(r), p)
	}
}

func (a *App) handleLocalPreviewBufferedV8599(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	p := strings.TrimSpace(r.URL.Query().Get("path"))
	if p == "" {
		http.Error(w, "path lipsă", 400)
		return
	}
	authAt := time.Now()
	allowed := a.localPathAllowed(p)
	authMS := previewMSV8599(time.Since(authAt))
	if !allowed {
		a.logf("LOCAL PREVIEW diag BUFFER REFUZAT: auth=%dms path=%s", authMS, p)
		http.Error(w, "Fișier local neautorizat", 403)
		return
	}
	statAt := time.Now()
	st, err := os.Stat(p)
	statMS := previewMSV8599(time.Since(statAt))
	if err != nil || st.IsDir() {
		a.logf("LOCAL PREVIEW diag BUFFER LIPSEȘTE: auth=%dms stat=%dms err=%v path=%s", authMS, statMS, err, p)
		http.Error(w, "Fișierul local nu mai există", 404)
		return
	}
	if remoteMediaKind(filepath.Base(p)) != "image" {
		http.Error(w, "Metoda BUFFER este disponibilă numai pentru imagini", 415)
		return
	}
	const maxBuffered = int64(64 << 20)
	if st.Size() > maxBuffered {
		a.logf("LOCAL PREVIEW diag BUFFER OMIS: size=%d > limit=%d path=%s", st.Size(), maxBuffered, p)
		http.Error(w, "Imaginea depășește limita metodei alternative (64 MiB)", 413)
		return
	}

	readAt := time.Now()
	data, err := os.ReadFile(p)
	readMS := previewMSV8599(time.Since(readAt))
	if err != nil {
		a.logf("LOCAL PREVIEW diag BUFFER EROARE CITIRE: auth=%dms stat=%dms read=%dms err=%v path=%s", authMS, statMS, readMS, err, p)
		http.Error(w, err.Error(), 500)
		return
	}
	contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(p)))
	if contentType == "" && len(data) > 0 {
		n := len(data)
		if n > 512 {
			n = 512
		}
		contentType = http.DetectContentType(data[:n])
	}
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	w.Header().Set("Content-Disposition", "inline")
	w.Header().Set("Cache-Control", "private, max-age=60")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	serveAt := time.Now()
	http.ServeContent(w, r, filepath.Base(p), st.ModTime(), bytes.NewReader(data))
	serveMS := previewMSV8599(time.Since(serveAt))
	totalMS := previewMSV8599(time.Since(started))
	a.logf("LOCAL PREVIEW diag BUFFER: total=%dms auth=%dms stat=%dms read=%dms memory-serve=%dms bytes=%d size=%d path=%s",
		totalMS, authMS, statMS, readMS, serveMS, len(data), st.Size(), p)
}

type localPreviewClientTraceV8599 struct {
	Event         string  `json:"event"`
	Path          string  `json:"path"`
	Method        string  `json:"method"`
	ElapsedMS     float64 `json:"elapsedMs"`
	NaturalWidth  int     `json:"naturalWidth"`
	NaturalHeight int     `json:"naturalHeight"`
	Detail        string  `json:"detail"`
}

func (a *App) handleLocalPreviewClientTraceV8599(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST necesar", 405)
		return
	}
	var req localPreviewClientTraceV8599
	if err := decodeJSON(r, &req); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	req.Event = strings.ToUpper(strings.TrimSpace(req.Event))
	req.Method = strings.ToUpper(strings.TrimSpace(req.Method))
	if req.Method == "" {
		req.Method = "DIRECT"
	}
	if req.Path != "" && !a.localPathAllowed(req.Path) {
		http.Error(w, "path local invalid", 403)
		return
	}
	if req.Detail != "" && len(req.Detail) > 180 {
		req.Detail = req.Detail[:180]
	}
	if req.Event == "PENDING" || req.Event == "ERROR" || req.Event == "ALT" || req.ElapsedMS >= 700 {
		a.logf("LOCAL PREVIEW client %s/%s: %.0fms %dx%d detail=%s path=%s",
			req.Event, req.Method, req.ElapsedMS, req.NaturalWidth, req.NaturalHeight, req.Detail, req.Path)
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = fmt.Fprint(w, `{"ok":true}`)
}
