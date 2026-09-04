package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"sync/atomic"
	"time"
)

// v8.5.11-test.1 replaces the layered root/per-file preview paths with one
// controller. MEGAcmd already owns one background server; DDG owns exactly one
// public-folder session, one root WebDAV location and, only when unavoidable,
// one temporary per-file compatibility location.

var megaPreviewTraceCounterV8511 atomic.Uint64

type megaPreviewUIResultV8511 struct {
	URL          string
	Mode         string
	Prepare      time.Duration
	TraceID      string
	TransportOK  bool
	FallbackUsed bool
	Note         string
}

type megaHTTPProbeV8511 struct {
	Reachable bool
	Status    int
	Method    string
	Duration  time.Duration
	Body      string
	Err       error
}

func planMegaFallbackV8511(currentRemotePath, currentURL, requestedRemotePath string) (removeRemotePath string, reuseURL string) {
	currentRemotePath = strings.TrimSpace(currentRemotePath)
	requestedRemotePath = strings.TrimSpace(requestedRemotePath)
	if currentRemotePath == requestedRemotePath && strings.TrimSpace(currentURL) != "" {
		return "", strings.TrimSpace(currentURL)
	}
	if currentRemotePath != "" {
		return currentRemotePath, ""
	}
	return "", ""
}

func nextMegaPreviewTraceV8511() string {
	return fmt.Sprintf("MP-%06d", megaPreviewTraceCounterV8511.Add(1))
}

func rootURLFromStateV8511(st MegaPreviewState) string {
	if strings.TrimSpace(st.RootURL) != "" {
		return strings.TrimSpace(st.RootURL)
	}
	if st.RemotePath == megaWarmRootRefV86 {
		return strings.TrimSpace(st.StreamURL)
	}
	return ""
}

func (a *App) megaPreviewSameSourceRootV8511(sourceURL string) bool {
	if a == nil {
		return false
	}
	a.previewMu.Lock()
	defer a.previewMu.Unlock()
	return a.preview.Active && strings.TrimSpace(a.preview.SourceURL) == strings.TrimSpace(sourceURL) && rootURLFromStateV8511(a.preview) != ""
}

func redactMegaDiagnosticV8511(s string) string {
	s = sessionRE.ReplaceAllString(s, "[SESSION_REDACTED]")
	for _, prefix := range []string{"https://mega.nz/", "https://mega.co.nz/"} {
		for {
			i := strings.Index(strings.ToLower(s), prefix)
			if i < 0 {
				break
			}
			end := i
			for end < len(s) && !strings.ContainsRune(" \t\r\n\"'", rune(s[end])) {
				end++
			}
			s = s[:i] + "[MEGA_LINK_REDACTED]" + s[end:]
		}
	}
	return sanitizeMega(s)
}

func redactMegaArgsV8511(args []string) string {
	redacted := append([]string(nil), args...)
	for i, arg := range redacted {
		low := strings.ToLower(arg)
		if strings.HasPrefix(low, "https://mega.nz/") || strings.HasPrefix(low, "https://mega.co.nz/") {
			redacted[i] = "[MEGA_LINK_REDACTED]"
		} else if i > 0 && redacted[0] == "login" && len(arg) >= 40 {
			redacted[i] = "[SESSION_REDACTED]"
		}
	}
	return strings.Join(redacted, " ")
}

func (a *App) runMegaPreviewCommandV8511(parent context.Context, traceID, stage string, timeout time.Duration, exe string, args ...string) (string, error) {
	started := time.Now()
	a.logf("MEGA PREVIEW [%s] CMD START stage=%s timeout=%s command=%s", traceID, stage, timeout, redactMegaArgsV8511(args))
	out, err := runMegaTimed(parent, timeout, exe, args...)
	duration := time.Since(started)
	if err != nil {
		a.logf("MEGA PREVIEW [%s] CMD END stage=%s duration=%s error=%v output=%s", traceID, stage, duration.Round(time.Millisecond), err, redactMegaDiagnosticV8511(out))
	} else {
		a.logf("MEGA PREVIEW [%s] CMD END stage=%s duration=%s ok", traceID, stage, duration.Round(time.Millisecond))
	}
	return out, err
}

func probeMegaHTTPV8511(parent context.Context, target string, timeout time.Duration) megaHTTPProbeV8511 {
	result := megaHTTPProbeV8511{}
	started := time.Now()
	defer func() { result.Duration = time.Since(started) }()
	if timeout <= 0 {
		timeout = 4 * time.Second
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	client := &http.Client{Timeout: timeout}
	request := func(method string, withRange bool) (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, method, target, nil)
		if err != nil {
			return nil, err
		}
		if withRange {
			req.Header.Set("Range", "bytes=0-0")
		}
		return client.Do(req)
	}
	resp, err := request(http.MethodHead, false)
	result.Method = http.MethodHead
	if err == nil && resp != nil {
		result.Status = resp.StatusCode
		_ = resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 400 {
			result.Reachable = true
			return result
		}
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusLoopDetected || resp.StatusCode == 509 {
			result.Err = fmt.Errorf("HTTP %d", resp.StatusCode)
			return result
		}
	}
	// Some WebDAV implementations do not implement HEAD reliably. A one-byte
	// Range GET distinguishes transport failure from an unsupported browser codec.
	resp, err = request(http.MethodGet, true)
	result.Method = "GET Range"
	if err != nil {
		result.Err = err
		return result
	}
	defer resp.Body.Close()
	result.Status = resp.StatusCode
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	result.Body = string(body)
	result.Reachable = resp.StatusCode >= 200 && resp.StatusCode < 400
	if !result.Reachable {
		result.Err = fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return result
}

func (a *App) startMegaRootWithRunnerV8511(ctx context.Context, traceID, exe string) (string, error) {
	out, err := a.runMegaPreviewCommandV8511(ctx, traceID, "webdav-root-start", 20*time.Second, exe, "webdav", megaWarmRootRefV86)
	if err != nil {
		return "", err
	}
	rootURL := extractWebDAVURL(out, megaWarmRootRefV86)
	if rootURL == "" {
		listing, listErr := a.runMegaPreviewCommandV8511(ctx, traceID, "webdav-root-list", 8*time.Second, exe, "webdav")
		if listErr != nil {
			return "", listErr
		}
		rootURL = extractWebDAVURL(listing, megaWarmRootRefV86)
	}
	if rootURL == "" {
		return "", errors.New("MEGAcmd nu a returnat URL-ul root WebDAV")
	}
	return rootURL, nil
}

func webDAVListingContainsRootV8511(listing, expected string) (string, bool) {
	listed := extractWebDAVURL(listing, megaWarmRootRefV86)
	if listed == "" {
		return "", false
	}
	if strings.TrimRight(listed, "/") != strings.TrimRight(strings.TrimSpace(expected), "/") {
		return listed, false
	}
	return listed, true
}

func (a *App) setMegaRootStateV8511(item RemoteItem, exe, rootURL, oldSession string) (string, error) {
	child, err := megaWebDAVChildURL(rootURL, item.Path)
	if err != nil || child == "" {
		return "", fmt.Errorf("cale copil WebDAV invalidă: %w", err)
	}
	a.preview = MegaPreviewState{
		Active:          true,
		SourceURL:       item.URL,
		RemotePath:      megaWarmRootRefV86,
		StreamURL:       rootURL,
		RootURL:         rootURL,
		PreviousSession: oldSession,
		Exe:             exe,
	}
	a.resetPreviewTTLLocked()
	return child, nil
}

func (a *App) ensureMegaPreviewRootV8511(item RemoteItem, traceID string) (string, string, error) {
	if a == nil || !strings.EqualFold(item.Source, "MEGA") {
		return "", "", errors.New("sursa nu este MEGA")
	}
	if strings.TrimSpace(item.URL) == "" {
		return "", "", errors.New("link MEGA lipsă")
	}
	gateCtx, gateCancel := context.WithTimeout(context.Background(), 65*time.Second)
	defer gateCancel()
	if err := acquireMegaSession(gateCtx); err != nil {
		return "", "", fmt.Errorf("MEGA este ocupat cu altă operație: %w", err)
	}
	defer releaseMegaSession()

	a.previewMu.Lock()
	defer a.previewMu.Unlock()
	if a.preview.Active && a.preview.SourceURL == item.URL {
		if rootURL := rootURLFromStateV8511(a.preview); rootURL != "" {
			child, err := megaWebDAVChildURL(rootURL, item.Path)
			if err == nil && child != "" {
				a.resetPreviewTTLLocked()
				a.logf("MEGA PREVIEW [%s] ROUTE root-cache root=true fallback=%t", traceID, a.preview.FallbackRemotePath != "")
				return child, "MEGA ROOT SERVICE", nil
			}
		}
	}

	if a.preview.Active && a.preview.SourceURL != item.URL {
		if err := a.stopMegaPreviewLocked("controller: schimbare sursă"); err != nil {
			a.logf("MEGA PREVIEW [%s] cleanup sursă anterioară: %v", traceID, err)
		}
	}
	exe := a.detectMegaClient()
	if exe == "" {
		return "", "", errors.New("MEGAcmd nu a fost găsit")
	}
	ctx := context.Background()

	// A persisted root is accepted only when MEGAcmd itself still lists exactly
	// that root and the HTTP endpoint answers. A listening TCP port alone is not
	// evidence that the same session/location survived.
	if hint, ok := a.loadMegaPreviewRestartHintV8510(item.URL); ok && hint.RootURL != "" {
		listing, listErr := a.runMegaPreviewCommandV8511(ctx, traceID, "persisted-root-verify", 20*time.Second, exe, "webdav")
		listedRoot, listedOK := webDAVListingContainsRootV8511(listing, hint.RootURL)
		probe := probeMegaHTTPV8511(ctx, hint.RootURL, 4*time.Second)
		a.logf("MEGA PREVIEW [%s] PERSISTED rootListed=%t listed=%s http=%d method=%s duration=%s error=%v", traceID, listedOK, listedRoot, probe.Status, probe.Method, probe.Duration.Round(time.Millisecond), probe.Err)
		if listErr == nil && listedOK && probe.Reachable {
			child, stateErr := a.setMegaRootStateV8511(item, exe, hint.RootURL, "")
			if stateErr == nil {
				return child, "MEGA VERIFIED PERSISTENT ROOT", nil
			}
		}
		a.clearMegaPreviewRestartHintV859()
	}

	oldSession := ""
	if out, err := a.runMegaPreviewCommandV8511(ctx, traceID, "session-snapshot", 20*time.Second, exe, "session"); err == nil {
		oldSession = extractSession(out)
	}
	if oldSession != "" {
		_, _ = a.runMegaPreviewCommandV8511(ctx, traceID, "session-detach", 12*time.Second, exe, "logout", "--keep-session")
	} else {
		_, _ = a.runMegaPreviewCommandV8511(ctx, traceID, "session-reset", 12*time.Second, exe, "logout")
	}
	loginOut, err := a.runMegaPreviewCommandV8511(ctx, traceID, "public-folder-resume", 50*time.Second, exe, megaPublicLoginArgsV856(item.URL)...)
	if err != nil {
		a.restoreMegaSessionSilent(exe, oldSession)
		problem := classifyMegaProblem(loginOut, err)
		return "", "", newMegaProblemError(problem, loginOut)
	}
	rootURL, err := a.startMegaRootWithRunnerV8511(ctx, traceID, exe)
	if err != nil {
		a.restoreMegaSessionSilent(exe, oldSession)
		return "", "", fmt.Errorf("sesiunea MEGA s-a deschis, dar root-ul WebDAV nu a pornit: %w", err)
	}
	child, err := a.setMegaRootStateV8511(item, exe, rootURL, oldSession)
	if err != nil {
		return "", "", err
	}
	if oldSession == "" {
		a.saveMegaPreviewRestartRootV8510(item.URL, rootURL)
	} else {
		a.clearMegaPreviewRestartHintV859()
	}
	a.logf("MEGA PREVIEW [%s] ROUTE cold-root root=%s previousSession=%t", traceID, rootURL, oldSession != "")
	return child, "MEGA ROOT SERVICE COLD", nil
}

func (a *App) diagnoseMegaPreviewFailureV8511(item RemoteItem, traceID string) (megaPreviewUIResultV8511, error) {
	started := time.Now()
	a.previewMu.Lock()
	st := a.preview
	a.previewMu.Unlock()
	rootURL := rootURLFromStateV8511(st)
	if st.Active && st.SourceURL == item.URL && rootURL != "" {
		child, _ := megaWebDAVChildURL(rootURL, item.Path)
		probe := probeMegaHTTPV8511(context.Background(), child, 5*time.Second)
		a.logf("MEGA PREVIEW [%s] BROWSER ERROR route=root probe=%s status=%d method=%s duration=%s error=%v", traceID, item.Path, probe.Status, probe.Method, probe.Duration.Round(time.Millisecond), probe.Err)
		if probe.Reachable {
			return megaPreviewUIResultV8511{URL: child, Mode: "MEGA ROOT TRANSPORT OK", Prepare: time.Since(started), TraceID: traceID, TransportOK: true, Note: "WebDAV răspunde; eroarea browserului indică format/codec neacceptat. Folosește playerul extern."}, nil
		}
		if probe.Status == 509 || strings.Contains(strings.ToLower(probe.Body), "overquota") {
			problem := classifyMegaProblem(fmt.Sprintf("HTTP %d %s", probe.Status, probe.Body), probe.Err)
			return megaPreviewUIResultV8511{}, newMegaProblemError(problem, probe.Body)
		}
	}

	// Repair the single root first. Invalidate only the in-memory URL; preserve
	// the session ownership metadata and never perform another login here.
	gateCtx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	if err := acquireMegaSession(gateCtx); err != nil {
		return megaPreviewUIResultV8511{}, err
	}
	defer releaseMegaSession()
	a.previewMu.Lock()
	defer a.previewMu.Unlock()
	st = a.preview
	if !st.Active || st.SourceURL != item.URL || st.Exe == "" {
		return megaPreviewUIResultV8511{}, errors.New("sesiunea MEGA preview nu mai este activă; selectează din nou fișierul")
	}
	rootURL, rootErr := a.startMegaRootWithRunnerV8511(context.Background(), traceID, st.Exe)
	if rootErr == nil {
		child, childErr := megaWebDAVChildURL(rootURL, item.Path)
		if childErr == nil {
			probe := probeMegaHTTPV8511(context.Background(), child, 5*time.Second)
			a.logf("MEGA PREVIEW [%s] ROOT REPAIR status=%d duration=%s error=%v", traceID, probe.Status, probe.Duration.Round(time.Millisecond), probe.Err)
			if probe.Reachable {
				st.RemotePath, st.StreamURL, st.RootURL = megaWarmRootRefV86, rootURL, rootURL
				a.preview = st
				a.resetPreviewTTLLocked()
				return megaPreviewUIResultV8511{URL: child, Mode: "MEGA ROOT REPAIRED", Prepare: time.Since(started), TraceID: traceID, TransportOK: true, Note: "Root-ul WebDAV a fost reverificat și reparat."}, nil
			}
		}
	}
	a.logf("MEGA PREVIEW [%s] FALLBACK reason=root-transport-failed rootError=%v oldFallback=%s", traceID, rootErr, st.FallbackRemotePath)

	remoteRef := megaRemoteRef(item)
	if remoteRef == "" {
		return megaPreviewUIResultV8511{}, errors.New("fișierul MEGA nu are handle/cale remote")
	}
	// Bound MEGAcmd to root + at most one temporary per-file location.
	removeFallback, reuseFallback := planMegaFallbackV8511(st.FallbackRemotePath, st.FallbackStreamURL, remoteRef)
	if removeFallback != "" {
		_, _ = a.runMegaPreviewCommandV8511(context.Background(), traceID, "fallback-remove-old", 8*time.Second, st.Exe, "webdav", "-d", removeFallback)
		st.FallbackRemotePath, st.FallbackStreamURL = "", ""
	}
	if reuseFallback != "" {
		return megaPreviewUIResultV8511{URL: reuseFallback, Mode: "MEGA BOUNDED FALLBACK", Prepare: time.Since(started), TraceID: traceID, FallbackUsed: true}, nil
	}
	out, err := a.runMegaPreviewCommandV8511(context.Background(), traceID, "fallback-add-one", 20*time.Second, st.Exe, "webdav", remoteRef)
	if err != nil {
		problem := classifyMegaProblem(out, err)
		return megaPreviewUIResultV8511{}, newMegaProblemError(problem, out)
	}
	fallbackURL := extractWebDAVURL(out, remoteRef)
	if fallbackURL == "" {
		listing, _ := a.runMegaPreviewCommandV8511(context.Background(), traceID, "fallback-list", 8*time.Second, st.Exe, "webdav")
		fallbackURL = extractWebDAVURL(listing, remoteRef)
	}
	if fallbackURL == "" {
		return megaPreviewUIResultV8511{}, errors.New("MEGAcmd nu a returnat URL-ul fallback")
	}
	st.FallbackRemotePath, st.FallbackStreamURL = remoteRef, fallbackURL
	a.preview = st
	a.resetPreviewTTLLocked()
	return megaPreviewUIResultV8511{URL: fallbackURL, Mode: "MEGA BOUNDED FALLBACK", Prepare: time.Since(started), TraceID: traceID, FallbackUsed: true, Note: "Root-ul nu a răspuns; este activ un singur endpoint temporar."}, nil
}

func (a *App) prepareMegaPreviewUIV8511(item RemoteItem, diagnoseBrowserError bool) (megaPreviewUIResultV8511, error) {
	started := time.Now()
	traceID := nextMegaPreviewTraceV8511()
	a.logf("MEGA PREVIEW [%s] CLICK file=%s diagnose=%t", traceID, item.Path, diagnoseBrowserError)
	if diagnoseBrowserError {
		return a.diagnoseMegaPreviewFailureV8511(item, traceID)
	}
	if streamURL, mode, ok := a.tryMegaPreviewUICacheV854(item); ok {
		return megaPreviewUIResultV8511{URL: streamURL, Mode: mode, Prepare: time.Since(started), TraceID: traceID, TransportOK: true, Note: "URL derivat din root-ul unic; nicio comandă MEGAcmd."}, nil
	}
	streamURL, mode, err := a.ensureMegaPreviewRootV8511(item, traceID)
	if err != nil {
		return megaPreviewUIResultV8511{}, err
	}
	a.logMegaRuntimeSnapshotV8511(traceID, streamURL)
	return megaPreviewUIResultV8511{URL: streamURL, Mode: mode, Prepare: time.Since(started), TraceID: traceID, TransportOK: true, Note: "Controller MEGA: o sesiune și un root WebDAV."}, nil
}

func (a *App) logMegaRuntimeSnapshotV8511(traceID, streamURL string) {
	if runtime.GOOS != "windows" {
		return
	}
	u, _ := url.Parse(streamURL)
	port := u.Port()
	if port == "" {
		port = "4443"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	serverOut, serverErr := exec.CommandContext(ctx, "tasklist.exe", "/FI", "IMAGENAME eq MEGAcmdServer.exe", "/FO", "CSV", "/NH").CombinedOutput()
	listenerOut, listenerErr := exec.CommandContext(ctx, "netstat.exe", "-ano", "-p", "tcp").CombinedOutput()
	listenerLines := make([]string, 0, 3)
	for _, line := range strings.Split(string(listenerOut), "\n") {
		if strings.Contains(line, ":"+port) && strings.Contains(strings.ToUpper(line), "LISTEN") {
			listenerLines = append(listenerLines, strings.TrimSpace(line))
		}
	}
	a.logf("MEGA PREVIEW [%s] RUNTIME server=%s serverError=%v listener=%s listenerError=%v", traceID, strings.TrimSpace(string(serverOut)), serverErr, strings.Join(listenerLines, " | "), listenerErr)
}
