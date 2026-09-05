package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// v8.5.26 separates two lifecycles which older builds accidentally coupled:
//   1. one MEGAcmd/WebDAV source service, expensive and shared by row changes;
//   2. one browser media request, cheap and cancelled on every newer selection.
//
// A -> B -> C therefore cancels only the obsolete HTTP transfers. It never
// queues B/C behind teardown or repeats login/webdav while the same source is
// already being prepared.

type megaPreviewPointV8526 struct {
	AtMS   int64  `json:"atMs"`
	FromT0 int64  `json:"fromT0Ms"`
	Detail string `json:"detail,omitempty"`
}

type megaPreviewCommandV8526 struct {
	Name       string `json:"name"`
	StartedMS  int64  `json:"startedMs"`
	DurationMS int64  `json:"durationMs"`
	Result     string `json:"result"`
}

type megaPreviewTraceV8526 struct {
	Generation uint64                           `json:"generation"`
	ItemPath   string                           `json:"itemPath"`
	Kind       string                           `json:"kind"`
	Route      string                           `json:"route"`
	State      string                           `json:"state"`
	Error      string                           `json:"error,omitempty"`
	Problem    *MegaProblem                     `json:"problem,omitempty"`
	Points     map[string]megaPreviewPointV8526 `json:"points"`
	Commands   []megaPreviewCommandV8526        `json:"commands,omitempty"`
	HTTPStatus int                              `json:"httpStatus,omitempty"`
	Range      string                           `json:"range,omitempty"`
	Bytes      int64                            `json:"bytes,omitempty"`
	CreatedMS  int64                            `json:"createdMs"`
}

type megaPreviewSourceV8526 struct {
	sourceURL       string
	ready           chan struct{}
	ctx             context.Context
	cancel          context.CancelFunc
	rootURL         string
	mode            string
	exe             string
	previousSession string
	err             error
	started         time.Time
}

type megaPreviewJobV8526 struct {
	generation uint64
	item       RemoteItem
	kind       string
	remoteRef  string
	force      bool
	ctx        context.Context
	cancel     context.CancelFunc
	source     *megaPreviewSourceV8526
	ready      chan struct{}
	target     string
	mode       string
	err        error
	streaming  bool
}

type megaPreviewOpsV8526 struct {
	detectExe func() string
	run       func(context.Context, time.Duration, string, ...string) (string, error)
	acquire   func(context.Context) error
	release   func()
}

type megaPreviewControllerV8526 struct {
	mu         sync.Mutex
	a          *App
	ops        megaPreviewOpsV8526
	generation uint64
	selection  *megaPreviewJobV8526
	source     *megaPreviewSourceV8526
	traces     map[uint64]*megaPreviewTraceV8526
	traceOrder []uint64
	closed     bool
	transport  *http.Transport
}

type megaPreviewDiagLineV8526 struct {
	path string
	line string
}

var megaPreviewDiagV8526 = struct {
	once sync.Once
	q    chan megaPreviewDiagLineV8526
}{q: make(chan megaPreviewDiagLineV8526, 4096)}

func startMegaPreviewDiagV8526() {
	go func() {
		for entry := range megaPreviewDiagV8526.q {
			if entry.path == "" || entry.line == "" {
				continue
			}
			_ = os.MkdirAll(filepath.Dir(entry.path), 0755)
			if st, err := os.Stat(entry.path); err == nil && st.Size() > 4<<20 {
				_ = os.Remove(entry.path + ".old")
				_ = os.Rename(entry.path, entry.path+".old")
			}
			if f, err := os.OpenFile(entry.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); err == nil {
				_, _ = fmt.Fprintln(f, entry.line)
				_ = f.Close()
			}
		}
	}()
}

func (c *megaPreviewControllerV8526) diagf(format string, args ...any) {
	if c == nil || c.a == nil {
		return
	}
	megaPreviewDiagV8526.once.Do(startMegaPreviewDiagV8526)
	path := filepath.Join(c.a.appDir, "data", "MEGA_Preview_Diagnostic.log")
	line := time.Now().Format("2006-01-02 15:04:05.000") + "  " + fmt.Sprintf(format, args...)
	select {
	case megaPreviewDiagV8526.q <- megaPreviewDiagLineV8526{path: path, line: line}:
	default:
	}
}

func newMegaPreviewControllerV8526(a *App, ops megaPreviewOpsV8526) *megaPreviewControllerV8526 {
	if ops.detectExe == nil {
		ops.detectExe = a.detectMegaClient
	}
	if ops.run == nil {
		ops.run = runMegaTimed
	}
	if ops.acquire == nil {
		ops.acquire = acquireMegaSession
	}
	if ops.release == nil {
		ops.release = releaseMegaSession
	}
	return &megaPreviewControllerV8526{
		a:      a,
		ops:    ops,
		traces: make(map[uint64]*megaPreviewTraceV8526),
		transport: &http.Transport{
			Proxy:                 nil,
			DisableKeepAlives:     true,
			DisableCompression:    true,
			MaxConnsPerHost:       4,
			ResponseHeaderTimeout: 20 * time.Second,
		},
	}
}

func (a *App) previewControllerV8526() *megaPreviewControllerV8526 {
	if a == nil {
		return nil
	}
	a.previewV8526Once.Do(func() {
		a.previewV8526 = newMegaPreviewControllerV8526(a, megaPreviewOpsV8526{})
	})
	return a.previewV8526
}

func (a *App) snapshotMegaPreviewV8526() MegaPreviewState {
	if a == nil {
		return MegaPreviewState{}
	}
	a.previewMu.Lock()
	st := a.preview
	a.previewMu.Unlock()
	return st
}

func (c *megaPreviewControllerV8526) markLocked(generation uint64, label, detail string) {
	tr := c.traces[generation]
	if tr == nil {
		return
	}
	now := time.Now().UnixMilli()
	t0 := tr.CreatedMS
	if p, ok := tr.Points["T0"]; ok && p.AtMS > 0 {
		t0 = p.AtMS
	}
	if _, exists := tr.Points[label]; !exists {
		tr.Points[label] = megaPreviewPointV8526{AtMS: now, FromT0: now - t0, Detail: detail}
	}
	c.diagf("GEN=%d %s +%dms %s", generation, label, now-t0, detail)
}

func (c *megaPreviewControllerV8526) mark(generation uint64, label, detail string) {
	c.mu.Lock()
	c.markLocked(generation, label, detail)
	c.mu.Unlock()
}

func (c *megaPreviewControllerV8526) failLocked(generation uint64, err error) {
	tr := c.traces[generation]
	if tr == nil || err == nil {
		return
	}
	tr.State = "error"
	tr.Error = err.Error()
	problem := megaProblemFromError(err)
	tr.Problem = &problem
	c.diagf("GEN=%d ERROR code=%s message=%s", generation, problem.Code, err.Error())
}

func (c *megaPreviewControllerV8526) begin(item RemoteItem, kind string, force bool, clientT0 int64) (uint64, string, string, error) {
	if strings.TrimSpace(item.URL) == "" {
		return 0, "", "", errors.New("link MEGA lipsă")
	}
	remoteRef := megaRemoteRef(item)
	if remoteRef == "" {
		return 0, "", "", errors.New("fișierul MEGA nu are handle sau cale; rescanează folderul")
	}
	warm := c.a.snapshotMegaPreviewV8526()

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return 0, "", "", errors.New("serviciul de preview este oprit")
	}
	if old := c.selection; old != nil {
		c.markLocked(old.generation, "T11", "selecție nouă; anulare transfer vechi")
		old.cancel()
		if !old.streaming {
			c.markLocked(old.generation, "T12", "cererea veche nu ajunsese la upstream")
		}
	}
	c.generation++
	generation := c.generation
	ctx, cancel := context.WithCancel(context.Background())
	job := &megaPreviewJobV8526{
		generation: generation,
		item:       item,
		kind:       kind,
		remoteRef:  remoteRef,
		force:      force,
		ctx:        ctx,
		cancel:     cancel,
		ready:      make(chan struct{}),
	}
	t0 := time.Now().UnixMilli()
	if clientT0 > 0 && clientT0 <= t0+5000 && clientT0 >= t0-60000 {
		t0 = clientT0
	}
	trace := &megaPreviewTraceV8526{
		Generation: generation,
		ItemPath:   item.Path,
		Kind:       kind,
		Route:      "pending",
		State:      "preparing",
		Points:     make(map[string]megaPreviewPointV8526),
		CreatedMS:  t0,
	}
	trace.Points["T0"] = megaPreviewPointV8526{AtMS: t0, FromT0: 0, Detail: "user click"}
	c.traces[generation] = trace
	c.traceOrder = append(c.traceOrder, generation)
	for len(c.traceOrder) > 100 {
		delete(c.traces, c.traceOrder[0])
		c.traceOrder = c.traceOrder[1:]
	}
	c.selection = job
	c.markLocked(generation, "T1", "rezultat remote rezolvat")
	c.markLocked(generation, "T2", "node="+redactMegaRefV8526(remoteRef))

	if force {
		trace.Route = "explicit-per-file-fallback"
		trace.State = "preparing"
		c.mu.Unlock()
		go c.preparePerFile(job)
	} else if rootURL, childURL, ok := usableWarmRootV8526(warm, item); ok {
		job.target = childURL
		job.mode = "MEGA ROOT CACHE"
		trace.Route = "warm-root"
		trace.State = "ready"
		c.markLocked(generation, "T3", "zero comenzi MEGAcmd")
		c.markLocked(generation, "T4", "root runtime reutilizat: "+safePreviewHostV8526(rootURL))
		close(job.ready)
		c.mu.Unlock()
	} else {
		svc := c.source
		if svc == nil || svc.sourceURL != item.URL || (isClosedV8526(svc.ready) && svc.err != nil) {
			if svc != nil && svc.sourceURL != item.URL {
				svc.cancel()
			}
			svcCtx, svcCancel := context.WithCancel(context.Background())
			svc = &megaPreviewSourceV8526{
				sourceURL: item.URL,
				ready:     make(chan struct{}),
				ctx:       svcCtx,
				cancel:    svcCancel,
				started:   time.Now(),
			}
			c.source = svc
			trace.Route = "new-source-service"
			job.source = svc
			c.mu.Unlock()
			go c.prepareSource(svc, warm)
		} else {
			job.source = svc
			trace.Route = "shared-source-service"
			c.markLocked(generation, "T3", "atașat la inițializarea MEGA deja pornită")
			c.mu.Unlock()
		}
	}

	if generation == 1 || generation == 10 || generation == 20 {
		go c.resourceSnapshot(generation)
	}
	return generation, fmt.Sprintf("/api/remote-preview/media?generation=%d", generation), "MEGA MANAGED PREVIEW", nil
}

func usableWarmRootV8526(st MegaPreviewState, item RemoteItem) (string, string, bool) {
	if !st.Active || st.SourceURL != item.URL || st.RemotePath != megaWarmRootRefV86 || strings.TrimSpace(st.StreamURL) == "" {
		return "", "", false
	}
	child, err := megaWebDAVChildURL(st.StreamURL, item.Path)
	if err != nil || child == "" {
		return "", "", false
	}
	return st.StreamURL, child, true
}

func isClosedV8526(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

func redactMegaRefV8526(ref string) string {
	if strings.HasPrefix(ref, "H:") && len(ref) > 8 {
		return ref[:6] + "…"
	}
	if len(ref) > 100 {
		return ref[:100] + "…"
	}
	return ref
}

func safePreviewHostV8526(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "invalid"
	}
	return u.Scheme + "://" + u.Host
}

func (c *megaPreviewControllerV8526) command(svc *megaPreviewSourceV8526, label string, timeout time.Duration, exe string, args ...string) (string, error) {
	started := time.Now()
	c.mu.Lock()
	for generation, tr := range c.traces {
		if job := c.selection; job != nil && job.generation == generation && job.source == svc {
			c.markLocked(generation, "T3", "MEGAcmd "+label+" start")
		}
		_ = tr
	}
	c.mu.Unlock()
	out, err := c.ops.run(svc.ctx, timeout, exe, args...)
	duration := time.Since(started)
	result := "OK"
	if err != nil {
		result = err.Error()
	}
	cmd := megaPreviewCommandV8526{Name: label + " " + safeMegaArgsV8526(args), StartedMS: started.UnixMilli(), DurationMS: duration.Milliseconds(), Result: result}
	c.mu.Lock()
	for _, tr := range c.traces {
		if job := c.selection; job != nil && job.generation == tr.Generation && job.source == svc {
			tr.Commands = append(tr.Commands, cmd)
		}
	}
	c.mu.Unlock()
	c.diagf("SOURCE=%p CMD=%s duration=%s result=%s output=%s", svc, cmd.Name, duration.Round(time.Millisecond), result, shortMegaOutputV8526(out))
	return out, err
}

func safeMegaArgsV8526(args []string) string {
	safe := append([]string(nil), args...)
	if len(safe) > 1 && safe[0] == "login" {
		safe[1] = "<MEGA_LINK>"
	}
	for i := range safe {
		safe[i] = redactMegaRefV8526(safe[i])
	}
	return strings.Join(safe, " ")
}

func shortMegaOutputV8526(out string) string {
	out = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(out, "\r", ""), "\n", " | "))
	if len(out) > 240 {
		out = out[:240] + "…"
	}
	return out
}

func exactRootURLV8526(out string) string {
	for _, line := range strings.Split(strings.ReplaceAll(out, "\r", ""), "\n") {
		u := webdavURLRE.FindString(line)
		if u == "" {
			continue
		}
		left := strings.TrimSpace(line[:strings.Index(line, u)])
		left = strings.TrimSpace(strings.TrimSuffix(left, ":"))
		const prefix = "serving via webdav "
		if strings.HasPrefix(strings.ToLower(left), prefix) {
			left = strings.TrimSpace(left[len(prefix):])
		}
		if left == megaWarmRootRefV86 {
			return strings.TrimSpace(u)
		}
	}
	return ""
}

func (c *megaPreviewControllerV8526) openRoot(svc *megaPreviewSourceV8526, exe, label string) (string, error) {
	out, err := c.command(svc, label, 60*time.Second, exe, "webdav", megaWarmRootRefV86)
	if err != nil {
		return "", err
	}
	rootURL := exactRootURLV8526(out)
	if rootURL == "" {
		listing, listErr := c.command(svc, label+"-list", 20*time.Second, exe, "webdav")
		if listErr != nil {
			return "", listErr
		}
		rootURL = exactRootURLV8526(listing)
	}
	if rootURL == "" {
		return "", errors.New("MEGAcmd nu a confirmat WebDAV root pentru /; nu reutilizez un URL ambiguu")
	}
	if !loopbackPreviewURLV8526(rootURL) {
		return "", errors.New("MEGAcmd a returnat un URL WebDAV care nu este loopback")
	}
	return rootURL, nil
}

func loopbackPreviewURLV8526(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	if strings.EqualFold(u.Hostname(), "localhost") {
		return true
	}
	ip := net.ParseIP(u.Hostname())
	return ip != nil && ip.IsLoopback()
}

func (c *megaPreviewControllerV8526) prepareSource(svc *megaPreviewSourceV8526, initial MegaPreviewState) {
	var finalErr error
	defer func() {
		c.mu.Lock()
		if finalErr != nil {
			svc.err = finalErr
		}
		close(svc.ready)
		for generation, tr := range c.traces {
			job := c.selection
			if job == nil || job.generation != generation || job.source != svc {
				continue
			}
			if finalErr != nil {
				c.failLocked(generation, finalErr)
			} else {
				tr.State = "ready"
				c.markLocked(generation, "T4", "WebDAV root gata: "+safePreviewHostV8526(svc.rootURL))
			}
		}
		c.mu.Unlock()
	}()

	if rootURL, _, ok := usableWarmRootV8526(initial, RemoteItem{URL: svc.sourceURL, Path: "/"}); ok {
		svc.rootURL = rootURL
		svc.mode = "runtime-root"
		return
	}
	exe := strings.TrimSpace(initial.Exe)
	if exe == "" {
		exe = strings.TrimSpace(c.ops.detectExe())
	}
	if exe == "" {
		finalErr = errors.New("MEGAcmd nu a fost găsit")
		return
	}
	svc.exe = exe

	gateCtx, gateCancel := context.WithTimeout(svc.ctx, 20*time.Second)
	defer gateCancel()
	if err := c.ops.acquire(gateCtx); err != nil {
		finalErr = fmt.Errorf("MEGA este ocupat cu scanare/download: %w", err)
		return
	}
	defer c.ops.release()

	attemptedCurrentSession := false
	if current := c.a.snapshotMegaPreviewV8526(); current.Active && current.SourceURL == svc.sourceURL {
		if rootURL, _, ok := usableWarmRootV8526(current, RemoteItem{URL: svc.sourceURL, Path: "/"}); ok {
			svc.rootURL = rootURL
			svc.mode = "runtime-root-after-wait"
			return
		}
		if current.Exe != "" {
			svc.exe = current.Exe
		}
		attemptedCurrentSession = true
		rootURL, err := c.openRoot(svc, svc.exe, "same-session-root")
		if err == nil {
			svc.rootURL = rootURL
			svc.previousSession = current.PreviousSession
			svc.mode = "same-session-root"
			c.commitSource(svc)
			return
		}
		c.diagf("SOURCE=%p same-session root failed: %v", svc, err)
	}

	// A restart hint proves only that DDG intentionally left this public-folder
	// session active. It does NOT prove that a remembered port/path still maps to
	// this folder. Ask MEGAcmd to confirm/start exactly '/' once.
	if !attemptedCurrentSession && c.a.matchesMegaPreviewRestartHintV859(svc.sourceURL) {
		rootURL, err := c.openRoot(svc, svc.exe, "restart-session-root")
		if err == nil {
			svc.rootURL = rootURL
			svc.mode = "restart-session-root"
			c.commitSource(svc)
			return
		}
		c.a.clearMegaPreviewRestartHintV859()
		c.diagf("SOURCE=%p restart hint stale: %v", svc, err)
	}

	oldSession := ""
	if out, err := c.command(svc, "session-snapshot", 30*time.Second, svc.exe, "session"); err == nil {
		oldSession = extractSession(out)
	}
	svc.previousSession = oldSession
	_, _ = c.command(svc, "session-detach", 20*time.Second, svc.exe, "logout", "--keep-session")
	loginOut, loginErr := c.command(svc, "public-folder-resume", 90*time.Second, svc.exe, megaPublicLoginArgsV856(svc.sourceURL)...)
	if loginErr != nil {
		finalErr = newMegaProblemError(classifyMegaProblem(loginOut, loginErr), loginOut)
		return
	}
	rootURL, rootErr := c.openRoot(svc, svc.exe, "cold-root")
	if rootErr != nil {
		finalErr = newMegaProblemError(classifyMegaProblem("", rootErr), "")
		return
	}
	svc.rootURL = rootURL
	svc.mode = "cold-root"
	c.commitSource(svc)
}

func (c *megaPreviewControllerV8526) commitSource(svc *megaPreviewSourceV8526) {
	c.mu.Lock()
	if c.closed || c.source != svc || svc.ctx.Err() != nil {
		c.mu.Unlock()
		return
	}
	c.a.previewMu.Lock()
	c.a.preview = MegaPreviewState{
		Active:          true,
		SourceURL:       svc.sourceURL,
		RemotePath:      megaWarmRootRefV86,
		StreamURL:       svc.rootURL,
		PreviousSession: svc.previousSession,
		Exe:             svc.exe,
	}
	c.a.resetPreviewTTLLocked()
	c.a.previewMu.Unlock()
	c.mu.Unlock()
	c.diagf("SOURCE=%p COMMIT mode=%s root=%s", svc, svc.mode, safePreviewHostV8526(svc.rootURL))
}

func (c *megaPreviewControllerV8526) preparePerFile(job *megaPreviewJobV8526) {
	defer close(job.ready)
	exe := c.ops.detectExe()
	if exe == "" {
		job.err = errors.New("MEGAcmd nu a fost găsit")
		c.mu.Lock()
		c.failLocked(job.generation, job.err)
		c.mu.Unlock()
		return
	}
	gateCtx, gateCancel := context.WithTimeout(job.ctx, 20*time.Second)
	defer gateCancel()
	if err := c.ops.acquire(gateCtx); err != nil {
		job.err = fmt.Errorf("MEGA este ocupat: %w", err)
		c.mu.Lock()
		c.failLocked(job.generation, job.err)
		c.mu.Unlock()
		return
	}
	defer c.ops.release()
	started := time.Now()
	c.mark(job.generation, "T3", "fallback per-file start "+redactMegaRefV8526(job.remoteRef))
	out, err := c.ops.run(job.ctx, 60*time.Second, exe, "webdav", job.remoteRef)
	duration := time.Since(started)
	c.mu.Lock()
	if tr := c.traces[job.generation]; tr != nil {
		tr.Commands = append(tr.Commands, megaPreviewCommandV8526{Name: "per-file webdav " + redactMegaRefV8526(job.remoteRef), StartedMS: started.UnixMilli(), DurationMS: duration.Milliseconds(), Result: errorTextV8526(err)})
	}
	c.mu.Unlock()
	if err != nil {
		job.err = newMegaProblemError(classifyMegaProblem(out, err), out)
		c.mu.Lock()
		c.failLocked(job.generation, job.err)
		c.mu.Unlock()
		return
	}
	streamURL := extractWebDAVURL(out, job.remoteRef)
	if streamURL == "" || !loopbackPreviewURLV8526(streamURL) {
		job.err = errors.New("MEGAcmd nu a confirmat URL-ul fallback pentru fișier")
		c.mu.Lock()
		c.failLocked(job.generation, job.err)
		c.mu.Unlock()
		return
	}
	job.target = streamURL
	job.mode = "explicit-per-file-fallback"
	c.mu.Lock()
	if tr := c.traces[job.generation]; tr != nil {
		tr.State = "ready"
	}
	c.markLocked(job.generation, "T4", "fallback per-file gata")
	c.mu.Unlock()
}

func errorTextV8526(err error) string {
	if err == nil {
		return "OK"
	}
	return err.Error()
}

func (c *megaPreviewControllerV8526) targetFor(rctx context.Context, job *megaPreviewJobV8526) (string, error) {
	if job.force {
		select {
		case <-rctx.Done():
			return "", rctx.Err()
		case <-job.ctx.Done():
			return "", context.Canceled
		case <-job.ready:
		}
		return job.target, job.err
	}
	if job.target != "" {
		return job.target, nil
	}
	svc := job.source
	if svc == nil {
		return "", errors.New("serviciul MEGA lipsește")
	}
	select {
	case <-rctx.Done():
		return "", rctx.Err()
	case <-job.ctx.Done():
		return "", context.Canceled
	case <-svc.ready:
	}
	if svc.err != nil {
		return "", svc.err
	}
	child, err := megaWebDAVChildURL(svc.rootURL, job.item.Path)
	if err != nil || child == "" {
		if err == nil {
			err = errors.New("cale goală")
		}
		return "", fmt.Errorf("WebDAV root este gata, dar calea fișierului este invalidă: %w", err)
	}
	return child, nil
}

func (c *megaPreviewControllerV8526) currentJob(generation uint64) (*megaPreviewJobV8526, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.selection == nil || c.selection.generation != generation {
		return nil, errors.New("preview depășit de o selecție mai nouă")
	}
	return c.selection, nil
}

func (c *megaPreviewControllerV8526) serveMedia(w http.ResponseWriter, r *http.Request, generation uint64) {
	job, err := c.currentJob(generation)
	if err != nil {
		http.Error(w, err.Error(), http.StatusGone)
		return
	}
	c.mu.Lock()
	job.streaming = true
	if tr := c.traces[generation]; tr != nil {
		tr.State = "streaming"
		tr.Range = r.Header.Get("Range")
	}
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		job.streaming = false
		if job.ctx.Err() != nil {
			c.markLocked(generation, "T12", "transfer upstream închis după anulare")
		}
		c.mu.Unlock()
	}()

	target, err := c.targetFor(r.Context(), job)
	if err != nil {
		c.mu.Lock()
		c.failLocked(generation, err)
		c.mu.Unlock()
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if !loopbackPreviewURLV8526(target) {
		http.Error(w, "upstream MEGA invalid", http.StatusBadGateway)
		return
	}

	requestCtx, requestCancel := context.WithCancel(r.Context())
	stopJobCancel := context.AfterFunc(job.ctx, requestCancel)
	defer func() {
		stopJobCancel()
		requestCancel()
	}()
	upReq, err := http.NewRequestWithContext(requestCtx, r.Method, target, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	for _, h := range []string{"Range", "If-Range", "If-Modified-Since", "If-None-Match"} {
		if v := r.Header.Get(h); v != "" {
			upReq.Header.Set(h, v)
		}
	}
	upReq.Header.Set("Connection", "close")
	resp, err := c.transport.RoundTrip(upReq)
	if err != nil {
		if requestCtx.Err() == nil {
			c.mu.Lock()
			c.failLocked(generation, fmt.Errorf("WebDAV nu a răspuns: %w", err))
			c.mu.Unlock()
		}
		return
	}
	defer resp.Body.Close()
	c.mark(generation, "T5", fmt.Sprintf("HTTP upstream %d range=%q", resp.StatusCode, r.Header.Get("Range")))
	for _, h := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "ETag", "Last-Modified", "Cache-Control"} {
		if v := resp.Header.Get(h); v != "" {
			w.Header().Set(h, v)
		}
	}
	w.Header().Set("X-DDG-MEGA-Preview", "v8526")
	w.Header().Set("Connection", "close")
	w.WriteHeader(resp.StatusCode)
	c.mu.Lock()
	if tr := c.traces[generation]; tr != nil {
		tr.HTTPStatus = resp.StatusCode
	}
	c.mu.Unlock()
	if r.Method == http.MethodHead {
		return
	}
	buf := make([]byte, 256*1024)
	n, readErr := resp.Body.Read(buf)
	if n > 0 {
		c.mark(generation, "T8", fmt.Sprintf("primii %d bytes", n))
		written, writeErr := w.Write(buf[:n])
		c.mu.Lock()
		if tr := c.traces[generation]; tr != nil {
			tr.Bytes += int64(written)
		}
		c.mu.Unlock()
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		if writeErr != nil {
			return
		}
	}
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return
	}
	copied, _ := io.CopyBuffer(w, resp.Body, buf)
	c.mu.Lock()
	if tr := c.traces[generation]; tr != nil {
		tr.Bytes += copied
		if tr.State != "error" {
			tr.State = "served"
		}
	}
	c.mu.Unlock()
}

func (c *megaPreviewControllerV8526) event(generation uint64, label, detail string, clientAt int64) {
	allowed := map[string]bool{"T6": true, "T7": true, "T9": true, "T10": true, "T11": true, "T12": true}
	if !allowed[label] {
		return
	}
	c.mu.Lock()
	tr := c.traces[generation]
	if tr != nil {
		if clientAt > 0 {
			t0 := tr.Points["T0"].AtMS
			if _, exists := tr.Points[label]; !exists {
				tr.Points[label] = megaPreviewPointV8526{AtMS: clientAt, FromT0: clientAt - t0, Detail: detail}
				c.diagf("GEN=%d %s +%dms %s", generation, label, clientAt-t0, detail)
			}
		} else {
			c.markLocked(generation, label, detail)
		}
		if label == "T10" && tr.State != "error" {
			tr.State = "rendered"
		}
	}
	c.mu.Unlock()
}

func cloneTraceV8526(tr *megaPreviewTraceV8526) megaPreviewTraceV8526 {
	copyTrace := *tr
	copyTrace.Points = make(map[string]megaPreviewPointV8526, len(tr.Points))
	for k, v := range tr.Points {
		copyTrace.Points[k] = v
	}
	copyTrace.Commands = append([]megaPreviewCommandV8526(nil), tr.Commands...)
	if tr.Problem != nil {
		problem := *tr.Problem
		copyTrace.Problem = &problem
	}
	return copyTrace
}

func (c *megaPreviewControllerV8526) trace(generation uint64) (megaPreviewTraceV8526, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	tr := c.traces[generation]
	if tr == nil {
		return megaPreviewTraceV8526{}, false
	}
	return cloneTraceV8526(tr), true
}

func (c *megaPreviewControllerV8526) allTraces() []megaPreviewTraceV8526 {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]megaPreviewTraceV8526, 0, len(c.traceOrder))
	for _, generation := range c.traceOrder {
		if tr := c.traces[generation]; tr != nil {
			out = append(out, cloneTraceV8526(tr))
		}
	}
	return out
}

func (c *megaPreviewControllerV8526) cancelCurrent(reason string) {
	c.mu.Lock()
	if job := c.selection; job != nil {
		c.markLocked(job.generation, "T11", reason)
		job.cancel()
		if !job.streaming {
			c.markLocked(job.generation, "T12", "transfer inactiv")
		}
	}
	c.mu.Unlock()
}

func (c *megaPreviewControllerV8526) invalidate(reason string) {
	c.mu.Lock()
	if job := c.selection; job != nil {
		c.markLocked(job.generation, "T11", reason)
		job.cancel()
		c.selection = nil
	}
	if svc := c.source; svc != nil {
		svc.cancel()
		c.source = nil
	}
	c.mu.Unlock()
}

func (c *megaPreviewControllerV8526) close(reason string) {
	c.invalidate(reason)
	c.mu.Lock()
	c.closed = true
	c.transport.CloseIdleConnections()
	c.mu.Unlock()
}

func (c *megaPreviewControllerV8526) resourceSnapshot(generation uint64) {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	c.diagf("GEN=%d RESOURCE goroutines=%d heapMiB=%.1f sysMiB=%.1f", generation, runtime.NumGoroutine(), float64(mem.HeapAlloc)/(1<<20), float64(mem.Sys)/(1<<20))
}

func (a *App) beginMegaPreviewV8526(item RemoteItem, kind string, force bool, clientT0 int64) (uint64, string, string, error) {
	c := a.previewControllerV8526()
	if c == nil {
		return 0, "", "", errors.New("controler MEGA indisponibil")
	}
	return c.begin(item, kind, force, clientT0)
}

func (a *App) cancelCurrentMegaPreviewV8526(reason string) {
	if a != nil && a.previewV8526 != nil {
		a.previewV8526.cancelCurrent(reason)
	}
}

func (a *App) invalidateMegaPreviewControllerV8526(reason string) {
	if a != nil && a.previewV8526 != nil {
		a.previewV8526.invalidate(reason)
	}
}

func (a *App) closeMegaPreviewControllerV8526(reason string) {
	if a != nil && a.previewV8526 != nil {
		a.previewV8526.close(reason)
	}
}

func previewGenerationV8526(r *http.Request) (uint64, error) {
	generation, err := strconv.ParseUint(strings.TrimSpace(r.URL.Query().Get("generation")), 10, 64)
	if err != nil || generation == 0 {
		return 0, errors.New("generation invalid")
	}
	return generation, nil
}

func (a *App) handleMegaPreviewMediaV8526(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	generation, err := previewGenerationV8526(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	a.previewControllerV8526().serveMedia(w, r, generation)
}

func (a *App) handleMegaPreviewStatusV8526(w http.ResponseWriter, r *http.Request) {
	generation, err := previewGenerationV8526(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	trace, ok := a.previewControllerV8526().trace(generation)
	if !ok {
		http.Error(w, "preview necunoscut", http.StatusNotFound)
		return
	}
	jsonOut(w, trace)
}

func (a *App) handleMegaPreviewEventV8526(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Generation uint64 `json:"generation"`
		Event      string `json:"event"`
		Detail     string `json:"detail,omitempty"`
		ClientAt   int64  `json:"clientAt,omitempty"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&req); err != nil || req.Generation == 0 {
		http.Error(w, "eveniment invalid", http.StatusBadRequest)
		return
	}
	a.previewControllerV8526().event(req.Generation, req.Event, req.Detail, req.ClientAt)
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleMegaPreviewTimingsV8526(w http.ResponseWriter, r *http.Request) {
	jsonOut(w, map[string]any{
		"version": appVersion,
		"traces":  a.previewControllerV8526().allTraces(),
	})
}
