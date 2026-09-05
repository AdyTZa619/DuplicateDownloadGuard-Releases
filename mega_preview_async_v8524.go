package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
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

// tryMegaPreviewUICacheAsyncV8525 is the only cache lookup used by the async
// browser path. It never waits behind legacy preview teardown/start code.
func (a *App) tryMegaPreviewUICacheAsyncV8525(item RemoteItem) (string, string, bool) {
	if a == nil || !strings.EqualFold(item.Source, "MEGA") {
		return "", "", false
	}
	if !a.previewMu.TryLock() {
		return "", "", false
	}
	defer a.previewMu.Unlock()

	st := a.preview
	if !st.Active || st.SourceURL != item.URL || strings.TrimSpace(st.StreamURL) == "" {
		return "", "", false
	}
	if st.RemotePath == megaWarmRootRefV86 {
		child, err := megaWebDAVChildURL(st.StreamURL, item.Path)
		if err == nil && child != "" {
			a.resetPreviewTTLLocked()
			return child, "MEGA ASYNC FAST ROOT", true
		}
	}
	remoteRef := megaRemoteRef(item)
	if remoteRef != "" && st.RemotePath == remoteRef {
		a.resetPreviewTTLLocked()
		return st.StreamURL, "MEGA ASYNC FAST CACHE", true
	}
	return "", "", false
}

// tryCommitMegaPreviewStateV8525 makes cache persistence best-effort. A valid
// WebDAV URL must be returned to the player even if some legacy path currently
// owns previewMu for cleanup.
func (a *App) tryCommitMegaPreviewStateV8525(st MegaPreviewState) bool {
	if a == nil || !a.previewMu.TryLock() {
		return false
	}
	a.preview = st
	a.resetPreviewTTLLocked()
	a.previewMu.Unlock()
	return true
}

// runMegaTimedAsyncV8525 bounds both process runtime and the inherited-pipe
// problem seen with MegaClient/MEGAcmdServer on Windows. ErrWaitDelay means the
// command process already exited successfully but a descendant kept stdout or
// stderr open; the captured command output is still usable and must not be
// treated as a failed MEGA operation.
func runMegaTimedAsyncV8525(parent context.Context, timeout time.Duration, exe string, args ...string) (string, error) {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, exe, args...)
	hideChildWindow(cmd)
	cmd.Env = os.Environ()
	cmd.WaitDelay = 750 * time.Millisecond

	b, err := cmd.CombinedOutput()
	out := strings.TrimSpace(string(b))
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return out, fmt.Errorf("timeout preview MEGA după %s", timeout.Round(time.Millisecond))
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return out, context.Canceled
	}
	if errors.Is(err, exec.ErrWaitDelay) {
		return out, nil
	}
	if err != nil {
		return out, fmt.Errorf("%w: %s", err, out)
	}
	return out, nil
}

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

	if !forceFallback {
		if u, mode, ok := a.tryMegaPreviewUICacheAsyncV8525(item); ok {
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
		if u, mode, ok := a.tryMegaPreviewUICacheAsyncV8525(item); ok {
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
		return runMegaTimedAsyncV8525(ctx, timeout, exe, args...)
	}

	// Cheapest path: current MEGA session already knows the public folder.
	if out, err := run(4*time.Second, "webdav", remoteRef); err == nil {
		if u := extractWebDAVURL(out, remoteRef); u != "" {
			_ = a.tryCommitMegaPreviewStateV8525(MegaPreviewState{
				Active:     true,
				SourceURL:  item.URL,
				RemotePath: remoteRef,
				StreamURL:  u,
				Exe:        exe,
			})
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

	_ = a.tryCommitMegaPreviewStateV8525(MegaPreviewState{
		Active:          true,
		SourceURL:       item.URL,
		RemotePath:      remoteRef,
		StreamURL:       u,
		PreviousSession: oldSession,
		Exe:             exe,
	})
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

	transport := http.RoundTripper(http.DefaultTransport)
	if base, ok := http.DefaultTransport.(*http.Transport); ok {
		tr := base.Clone()
		tr.DisableKeepAlives = true
		tr.ResponseHeaderTimeout = 8 * time.Second
		transport = tr
	}
	resp, err := (&http.Client{Transport: transport}).Do(req)
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
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(resp.StatusCode)
	if r.Method == http.MethodHead {
		return
	}
	_, _ = io.Copy(w, resp.Body)
}
