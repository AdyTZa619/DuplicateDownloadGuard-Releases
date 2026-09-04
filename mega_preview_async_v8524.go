package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type megaAsyncPreviewJobV8524 struct {
	generation uint64
	item       RemoteItem
	kind       string
	ctx        context.Context
	cancel     context.CancelFunc
	done       chan struct{}
	upstream   string
	mode       string
	err        error
}

var megaAsyncPreviewStateV8524 struct {
	sync.Mutex
	next    uint64
	current *megaAsyncPreviewJobV8524
}

var megaAsyncPreviewWorkerV8524 sync.Mutex

func (a *App) beginMegaAsyncPreviewV8524(item RemoteItem, forceFallback bool) (*megaAsyncPreviewJobV8524, string, error) {
	if a == nil || !strings.EqualFold(item.Source, "MEGA") {
		return nil, "", errors.New("sursa nu este MEGA")
	}
	kind := remoteMediaKind(item.Name)
	if kind == "other" {
		return nil, "", errors.New("formatul nu are preview media integrat")
	}

	ctx, cancel := context.WithCancel(context.Background())
	megaAsyncPreviewStateV8524.Lock()
	if old := megaAsyncPreviewStateV8524.current; old != nil && old.cancel != nil {
		old.cancel()
	}
	megaAsyncPreviewStateV8524.next++
	job := &megaAsyncPreviewJobV8524{
		generation: megaAsyncPreviewStateV8524.next,
		item:       item,
		kind:       kind,
		ctx:        ctx,
		cancel:     cancel,
		done:       make(chan struct{}),
	}
	megaAsyncPreviewStateV8524.current = job
	megaAsyncPreviewStateV8524.Unlock()

	// Cache hits are intentionally network-free and can be published immediately.
	if !forceFallback {
		if u, mode, ok := a.tryMegaPreviewUICacheV854(item); ok {
			job.upstream = u
			job.mode = mode
			close(job.done)
			return job, fmt.Sprintf("/api/remote-preview/media?v=%d", job.generation), nil
		}
	}

	go a.runMegaAsyncPreviewV8524(job, forceFallback)
	return job, fmt.Sprintf("/api/remote-preview/media?v=%d", job.generation), nil
}

func (a *App) runMegaAsyncPreviewV8524(job *megaAsyncPreviewJobV8524, forceFallback bool) {
	defer close(job.done)
	megaAsyncPreviewWorkerV8524.Lock()
	defer megaAsyncPreviewWorkerV8524.Unlock()
	if err := job.ctx.Err(); err != nil {
		job.err = err
		return
	}

	ctx, cancel := context.WithTimeout(job.ctx, 25*time.Second)
	defer cancel()
	u, mode, err := a.prepareMegaPreviewV8524(ctx, job.item, forceFallback)
	if err != nil {
		job.err = err
		return
	}
	if ctx.Err() != nil {
		job.err = ctx.Err()
		return
	}
	job.upstream = u
	job.mode = mode
}

func (a *App) prepareMegaPreviewV8524(ctx context.Context, item RemoteItem, forceFallback bool) (string, string, error) {
	if !forceFallback {
		if u, mode, ok := a.tryMegaPreviewUICacheV854(item); ok {
			return u, mode, nil
		}
	}
	if err := acquireMegaSession(ctx); err != nil {
		return "", "", fmt.Errorf("MEGA ocupat: %w", err)
	}
	defer releaseMegaSession()

	remoteRef := megaRemoteRef(item)
	if remoteRef == "" {
		return "", "", errors.New("fișierul MEGA nu are handle/cale remote utilizabilă")
	}
	exe := a.detectMegaClient()
	if exe == "" {
		return "", "", errors.New("MEGAcmd nu a fost găsit")
	}

	run := func(timeout time.Duration, args ...string) (string, error) {
		return runMegaTimed(ctx, timeout, exe, args...)
	}

	// First try the currently active public session. This is the cheapest and
	// safest path after a scan or a preserved --resume session.
	if out, err := run(4*time.Second, "webdav", remoteRef); err == nil {
		if u := extractWebDAVURL(out, remoteRef); u != "" {
			a.previewMu.Lock()
			a.preview = MegaPreviewState{Active: true, SourceURL: item.URL, RemotePath: remoteRef, StreamURL: u, Exe: exe}
			a.resetPreviewTTLLocked()
			a.previewMu.Unlock()
			a.logf("MEGA ASYNC CURRENT SESSION: %s [%s] -> %s", item.Path, remoteRef, u)
			return u, "MEGA ASYNC CURRENT", nil
		}
	}
	if err := ctx.Err(); err != nil {
		return "", "", err
	}

	oldSession := ""
	if out, err := run(2*time.Second, "session"); err == nil {
		oldSession = extractSession(out)
	}
	_, _ = run(3*time.Second, "logout", "--keep-session")
	if err := ctx.Err(); err != nil {
		return "", "", err
	}

	loginOut, err := run(12*time.Second, megaPublicLoginArgsV856(item.URL)...)
	if err != nil {
		problem := classifyMegaProblem(loginOut, err)
		return "", "", newMegaProblemError(problem, loginOut)
	}
	if err := ctx.Err(); err != nil {
		return "", "", err
	}

	out, err := run(6*time.Second, "webdav", remoteRef)
	if err != nil {
		problem := classifyMegaProblem(out, err)
		return "", "", newMegaProblemError(problem, out)
	}
	u := extractWebDAVURL(out, remoteRef)
	if u == "" {
		listing, _ := run(2*time.Second, "webdav")
		u = extractWebDAVURL(listing, remoteRef)
	}
	if u == "" {
		return "", "", errors.New("MEGAcmd a activat WebDAV, dar nu a returnat URL-ul de streaming")
	}

	a.previewMu.Lock()
	a.preview = MegaPreviewState{
		Active:          true,
		SourceURL:       item.URL,
		RemotePath:      remoteRef,
		StreamURL:       u,
		PreviousSession: oldSession,
		Exe:             exe,
	}
	a.resetPreviewTTLLocked()
	a.previewMu.Unlock()
	a.logf("MEGA ASYNC RESUME: %s [%s] -> %s", item.Path, remoteRef, u)
	return u, "MEGA ASYNC RESUME", nil
}

func currentMegaAsyncPreviewV8524(generation uint64) *megaAsyncPreviewJobV8524 {
	megaAsyncPreviewStateV8524.Lock()
	defer megaAsyncPreviewStateV8524.Unlock()
	job := megaAsyncPreviewStateV8524.current
	if job == nil || job.generation != generation {
		return nil
	}
	return job
}

func cancelMegaAsyncPreviewV8524() {
	megaAsyncPreviewStateV8524.Lock()
	if job := megaAsyncPreviewStateV8524.current; job != nil && job.cancel != nil {
		job.cancel()
	}
	megaAsyncPreviewStateV8524.current = nil
	megaAsyncPreviewStateV8524.Unlock()
}

func (a *App) handleMegaAsyncPreviewMediaV8524(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "metodă neacceptată", http.StatusMethodNotAllowed)
		return
	}
	generation, err := strconv.ParseUint(strings.TrimSpace(r.URL.Query().Get("v")), 10, 64)
	if err != nil || generation == 0 {
		http.Error(w, "preview invalid", http.StatusBadRequest)
		return
	}
	job := currentMegaAsyncPreviewV8524(generation)
	if job == nil {
		http.Error(w, "preview înlocuit de o selecție mai nouă", http.StatusGone)
		return
	}

	select {
	case <-job.done:
	case <-r.Context().Done():
		return
	case <-job.ctx.Done():
		http.Error(w, "preview anulat", http.StatusGone)
		return
	}
	if job.err != nil {
		status := http.StatusBadGateway
		if errors.Is(job.err, context.Canceled) {
			status = http.StatusGone
		} else if errors.Is(job.err, context.DeadlineExceeded) {
			status = http.StatusGatewayTimeout
		}
		http.Error(w, job.err.Error(), status)
		return
	}
	if strings.TrimSpace(job.upstream) == "" {
		http.Error(w, "MEGA nu a produs un URL de streaming", http.StatusBadGateway)
		return
	}

	proxyCtx, proxyCancel := context.WithCancel(r.Context())
	defer proxyCancel()
	go func() {
		select {
		case <-job.ctx.Done():
			proxyCancel()
		case <-proxyCtx.Done():
		}
	}()

	req, err := http.NewRequestWithContext(proxyCtx, r.Method, job.upstream, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	for _, h := range []string{"Range", "If-Range", "If-None-Match", "If-Modified-Since", "Accept", "User-Agent"} {
		if v := r.Header.Get(h); v != "" {
			req.Header.Set(h, v)
		}
	}
	resp, err := (&http.Client{Transport: http.DefaultTransport}).Do(req)
	if err != nil {
		if proxyCtx.Err() != nil {
			return
		}
		http.Error(w, "stream MEGA: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	for _, h := range []string{"Accept-Ranges", "Content-Length", "Content-Range", "Content-Type", "ETag", "Last-Modified", "Cache-Control"} {
		if v := resp.Header.Get(h); v != "" {
			w.Header().Set(h, v)
		}
	}
	w.Header().Set("X-DDG-Preview-Mode", job.mode)
	w.Header().Set("Content-Disposition", "inline")
	w.WriteHeader(resp.StatusCode)
	if r.Method == http.MethodHead {
		return
	}
	_, _ = io.Copy(w, resp.Body)
}
