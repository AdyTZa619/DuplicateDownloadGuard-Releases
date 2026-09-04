package main

import (
	"context"
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

// This controller is deliberately layered on top of the validated 8.5.9
// session behavior. It does not keep an unverified root URL across DDG
// restarts. Its responsibilities are diagnostics, transport verification and
// ensuring that browser codec failures never create MEGAcmd WebDAV locations.

var megaPreviewTraceV8511 atomic.Uint64

type megaPreviewResponseV8511 struct {
	URL          string
	Mode         string
	Prepare      time.Duration
	TraceID      string
	TransportOK  bool
	FallbackUsed bool
	Note         string
}

type megaPreviewProbeV8511 struct {
	Reachable bool
	Status    int
	Method    string
	Duration  time.Duration
	Body      string
	Err       error
}

func nextMegaPreviewTraceV8511() string {
	return fmt.Sprintf("MP-%06d", megaPreviewTraceV8511.Add(1))
}

func firstMegaPreviewTraceV8511(traceIDs []string) string {
	if len(traceIDs) > 0 && strings.TrimSpace(traceIDs[0]) != "" {
		return strings.TrimSpace(traceIDs[0])
	}
	return nextMegaPreviewTraceV8511()
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
	return a.preview.Active && a.preview.SourceURL == strings.TrimSpace(sourceURL) && rootURLFromStateV8511(a.preview) != ""
}

func redactMegaArgsV8511(args []string) string {
	redacted := append([]string(nil), args...)
	for i, arg := range redacted {
		lower := strings.ToLower(arg)
		if strings.HasPrefix(lower, "https://mega.nz/") || strings.HasPrefix(lower, "https://mega.co.nz/") {
			redacted[i] = "[MEGA_LINK_REDACTED]"
		} else if len(redacted) > 0 && redacted[0] == "login" && i > 0 && len(arg) >= 40 {
			redacted[i] = "[SESSION_REDACTED]"
		}
	}
	return strings.Join(redacted, " ")
}

func redactMegaOutputV8511(output string) string {
	output = sessionRE.ReplaceAllString(output, "[SESSION_REDACTED]")
	for _, prefix := range []string{"https://mega.nz/", "https://mega.co.nz/"} {
		for {
			lower := strings.ToLower(output)
			start := strings.Index(lower, prefix)
			if start < 0 {
				break
			}
			end := start
			for end < len(output) && !strings.ContainsRune(" \t\r\n\"'", rune(output[end])) {
				end++
			}
			output = output[:start] + "[MEGA_LINK_REDACTED]" + output[end:]
		}
	}
	return sanitizeMega(output)
}

func (a *App) runMegaPreviewCommandV8511(ctx context.Context, traceID, stage string, timeout time.Duration, exe string, args ...string) (string, error) {
	started := time.Now()
	a.logf("MEGA PREVIEW [%s] CMD START stage=%s timeout=%s command=%s", traceID, stage, timeout, redactMegaArgsV8511(args))
	out, err := runMegaTimed(ctx, timeout, exe, args...)
	if err != nil {
		a.logf("MEGA PREVIEW [%s] CMD END stage=%s duration=%s error=%v output=%s", traceID, stage, time.Since(started).Round(time.Millisecond), err, redactMegaOutputV8511(out))
	} else {
		a.logf("MEGA PREVIEW [%s] CMD END stage=%s duration=%s ok", traceID, stage, time.Since(started).Round(time.Millisecond))
	}
	return out, err
}

func probeMegaPreviewURLV8511(parent context.Context, target string) megaPreviewProbeV8511 {
	result := megaPreviewProbeV8511{}
	started := time.Now()
	defer func() { result.Duration = time.Since(started) }()
	client := &http.Client{Timeout: 4 * time.Second}
	request := func(method string, ranged bool) (*http.Response, error) {
		ctx, cancel := context.WithTimeout(parent, 4*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, method, target, nil)
		if err != nil {
			return nil, err
		}
		if ranged {
			req.Header.Set("Range", "bytes=0-0")
		}
		return client.Do(req)
	}
	resp, err := request(http.MethodHead, false)
	result.Method = http.MethodHead
	if err == nil {
		result.Status = resp.StatusCode
		_ = resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 400 {
			result.Reachable = true
			return result
		}
		if resp.StatusCode == 509 || resp.StatusCode == http.StatusTooManyRequests {
			result.Err = fmt.Errorf("HTTP %d", resp.StatusCode)
			return result
		}
	}
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

// startMegaRootTracedV8511 is the only normal WebDAV service constructor.
// The controller serves the public-folder root once and derives child URLs in
// memory for every later click. It never creates a per-file endpoint here.
func (a *App) startMegaRootTracedV8511(ctx context.Context, traceID, exe, stage string) (string, string, error) {
	out, err := a.runMegaPreviewCommandV8511(ctx, traceID, stage+"-start", 20*time.Second, exe, "webdav", megaWarmRootRefV86)
	if err != nil {
		return "", out, err
	}
	rootURL := extractWebDAVURL(out, "")
	if rootURL == "" {
		listing, listErr := a.runMegaPreviewCommandV8511(ctx, traceID, stage+"-list", 6*time.Second, exe, "webdav")
		out += "\n" + listing
		if listErr != nil {
			return "", out, listErr
		}
		rootURL = extractWebDAVURL(listing, megaWarmRootRefV86)
	}
	if rootURL == "" {
		return "", out, errors.New("MEGAcmd nu a returnat URL-ul root WebDAV")
	}
	return rootURL, out, nil
}

func setMegaRootStateV8511(st MegaPreviewState, item RemoteItem, exe, rootURL string) (MegaPreviewState, string, error) {
	child, err := megaWebDAVChildURL(rootURL, item.Path)
	if err != nil || child == "" {
		return st, "", fmt.Errorf("cale copil WebDAV invalidă: %w", err)
	}
	st.Active = true
	st.SourceURL = item.URL
	st.RemotePath = megaWarmRootRefV86
	st.StreamURL = rootURL
	st.RootURL = rootURL
	st.Exe = exe
	return st, child, nil
}

func megaMayReplaceSessionV8511(problem MegaProblem) bool {
	return problem.Code == "MEGA_AUTH" || problem.Code == "MEGA_NOT_FOUND"
}

func planMegaFallbackV8511(currentRemotePath, currentURL, requestedRemotePath string) (removeRemotePath, reuseURL string) {
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

func (a *App) startBoundedMegaFallbackLockedV8511(ctx context.Context, traceID string, st MegaPreviewState, item RemoteItem) (MegaPreviewState, string, error) {
	remoteRef := megaRemoteRef(item)
	if remoteRef == "" {
		return st, "", errors.New("fișierul MEGA nu are handle/cale remote")
	}
	oldFallback := st.FallbackRemotePath
	if oldFallback == "" && st.RemotePath != "" && st.RemotePath != megaWarmRootRefV86 {
		oldFallback = st.RemotePath
	}
	removeFallback, reuseFallback := planMegaFallbackV8511(oldFallback, st.FallbackStreamURL, remoteRef)
	if reuseFallback != "" {
		return st, reuseFallback, nil
	}
	if removeFallback != "" {
		_, _ = a.runMegaPreviewCommandV8511(ctx, traceID, "fallback-remove-old", 6*time.Second, st.Exe, "webdav", "-d", removeFallback)
		st.FallbackRemotePath, st.FallbackStreamURL = "", ""
	}
	out, err := a.runMegaPreviewCommandV8511(ctx, traceID, "fallback-add-one", 20*time.Second, st.Exe, "webdav", remoteRef)
	if err != nil {
		return st, "", newMegaProblemError(classifyMegaProblem(out, err), out)
	}
	streamURL := extractWebDAVURL(out, remoteRef)
	if streamURL == "" {
		listing, _ := a.runMegaPreviewCommandV8511(ctx, traceID, "fallback-list", 6*time.Second, st.Exe, "webdav")
		streamURL = extractWebDAVURL(listing, remoteRef)
	}
	if streamURL == "" {
		_, _ = a.runMegaPreviewCommandV8511(ctx, traceID, "fallback-remove-invalid", 5*time.Second, st.Exe, "webdav", "-d", remoteRef)
		return st, "", errors.New(megaWebDAVURLMissingV85)
	}
	st.Active = true
	st.SourceURL = item.URL
	st.FallbackRemotePath = remoteRef
	st.FallbackStreamURL = streamURL
	if rootURLFromStateV8511(st) == "" {
		st.RemotePath = remoteRef
		st.StreamURL = streamURL
	}
	return st, streamURL, nil
}

// ensureMegaPreviewRootV8511 owns the production route. A scan creates one
// root service; a cold restart first validates the retained MEGAcmd public
// session with the requested handle and immediately promotes it to one root.
// A timeout/network failure is returned instead of stacking logout/login work
// behind an already slow command.
func (a *App) ensureMegaPreviewRootV8511(item RemoteItem, traceID string) (string, string, error) {
	if a == nil || !strings.EqualFold(item.Source, "MEGA") {
		return "", "", errors.New("sursa nu este MEGA")
	}
	if strings.TrimSpace(item.URL) == "" {
		return "", "", errors.New("link MEGA lipsă")
	}
	if streamURL, mode, ok := a.tryMegaPreviewUICacheV854(item); ok {
		return streamURL, mode, nil
	}

	gateCtx, gateCancel := context.WithTimeout(context.Background(), 65*time.Second)
	defer gateCancel()
	if err := acquireMegaSession(gateCtx); err != nil {
		return "", "", fmt.Errorf("MEGA este ocupat cu altă operație: %w", err)
	}
	defer releaseMegaSession()

	a.previewMu.Lock()
	defer a.previewMu.Unlock()
	if rootURL := rootURLFromStateV8511(a.preview); a.preview.Active && a.preview.SourceURL == item.URL && rootURL != "" {
		child, err := megaWebDAVChildURL(rootURL, item.Path)
		if err == nil && child != "" {
			a.resetPreviewTTLLocked()
			return child, "MEGA ROOT SERVICE", nil
		}
	}

	ctx := context.Background()
	if a.preview.Active && a.preview.SourceURL == item.URL && a.preview.Exe != "" {
		st := a.preview
		rootURL, raw, err := a.startMegaRootTracedV8511(ctx, traceID, st.Exe, "same-session-root")
		if err == nil {
			oldPerFile := st.RemotePath
			st, child, stateErr := setMegaRootStateV8511(st, item, st.Exe, rootURL)
			if stateErr != nil {
				return "", "", stateErr
			}
			if oldPerFile != "" && oldPerFile != megaWarmRootRefV86 {
				_, _ = a.runMegaPreviewCommandV8511(ctx, traceID, "promote-remove-per-file", 6*time.Second, st.Exe, "webdav", "-d", oldPerFile)
			}
			st.FallbackRemotePath, st.FallbackStreamURL = "", ""
			a.preview = st
			a.resetPreviewTTLLocked()
			return child, "MEGA ROOT SERVICE REPAIRED", nil
		}
		a.logf("MEGA PREVIEW [%s] ROOT unavailable in active session; bounded fallback only: %v", traceID, err)
		st, fallbackURL, fallbackErr := a.startBoundedMegaFallbackLockedV8511(ctx, traceID, st, item)
		if fallbackErr != nil {
			problem := classifyMegaProblem(raw, err)
			if problem.Code != "MEGA_UNKNOWN" {
				return "", "", newMegaProblemError(problem, raw)
			}
			return "", "", fallbackErr
		}
		a.preview = st
		a.resetPreviewTTLLocked()
		return fallbackURL, "MEGA BOUNDED FALLBACK", nil
	}

	if a.preview.Active {
		_ = a.stopMegaPreviewLocked("controller: schimbare sursă")
	}
	exe := a.detectMegaClient()
	if exe == "" {
		return "", "", errors.New("MEGAcmd nu a fost găsit")
	}
	remoteRef := megaRemoteRef(item)
	if remoteRef == "" {
		return "", "", errors.New("fișierul MEGA nu are handle/cale remote")
	}

	// 8.5.10 incorrectly trusted a saved root URL. The test candidate stores no
	// root URL. It verifies the exact requested handle in MEGAcmd's live session,
	// promotes that session to a root, then removes the one validation endpoint.
	if a.matchesMegaPreviewRestartHintV859(item.URL) {
		run := func(timeout time.Duration, args ...string) (string, error) {
			return a.runMegaPreviewCommandV8511(ctx, traceID, "restart-session-validate", timeout, exe, args...)
		}
		validation, validationErr := tryMegaCurrentSessionWebDAVV859(remoteRef, run)
		if validationErr == nil && validation.StreamURL != "" {
			rootURL, rootRaw, rootErr := a.startMegaRootTracedV8511(ctx, traceID, exe, "restart-root")
			if rootErr == nil {
				_, _ = a.runMegaPreviewCommandV8511(ctx, traceID, "restart-remove-validator", 6*time.Second, exe, "webdav", "-d", remoteRef)
				st, child, stateErr := setMegaRootStateV8511(MegaPreviewState{}, item, exe, rootURL)
				if stateErr != nil {
					return "", "", stateErr
				}
				a.preview = st
				a.resetPreviewTTLLocked()
				return child, "MEGA ROOT SERVICE RESUMED", nil
			}
			st := MegaPreviewState{Active: true, SourceURL: item.URL, RemotePath: remoteRef, StreamURL: validation.StreamURL, FallbackRemotePath: remoteRef, FallbackStreamURL: validation.StreamURL, Exe: exe}
			a.preview = st
			a.resetPreviewTTLLocked()
			a.logf("MEGA PREVIEW [%s] root promotion failed; one validated endpoint retained: %v • %s", traceID, rootErr, redactMegaOutputV8511(rootRaw))
			return validation.StreamURL, "MEGA BOUNDED FALLBACK", nil
		}
		problem := classifyMegaProblem(validation.StartOutput, validationErr)
		a.logf("MEGA PREVIEW [%s] restart session validation failed code=%s; replace=%t", traceID, problem.Code, megaMayReplaceSessionV8511(problem))
		if !megaMayReplaceSessionV8511(problem) {
			return "", "", newMegaProblemError(problem, validation.StartOutput)
		}
		a.clearMegaPreviewRestartHintV859()
	}

	oldSession := ""
	sessionOut, sessionErr := a.runMegaPreviewCommandV8511(ctx, traceID, "session-snapshot", 20*time.Second, exe, "session")
	if sessionErr == nil {
		oldSession = extractSession(sessionOut)
	} else {
		problem := classifyMegaProblem(sessionOut, sessionErr)
		if problem.Code == "MEGA_TIMEOUT" || problem.Code == "MEGA_NETWORK" || problem.Code == "MEGA_QUOTA" || problem.Code == "MEGA_RATE_LIMIT" {
			return "", "", newMegaProblemError(problem, sessionOut)
		}
	}
	if oldSession != "" {
		_, _ = a.runMegaPreviewCommandV8511(ctx, traceID, "session-detach", 12*time.Second, exe, "logout", "--keep-session")
	} else {
		_, _ = a.runMegaPreviewCommandV8511(ctx, traceID, "session-reset", 12*time.Second, exe, "logout")
	}
	loginOut, loginErr := a.runMegaPreviewCommandV8511(ctx, traceID, "public-folder-resume", 50*time.Second, exe, megaPublicLoginArgsV856(item.URL)...)
	if loginErr != nil {
		a.restoreMegaSessionSilent(exe, oldSession)
		return "", "", newMegaProblemError(classifyMegaProblem(loginOut, loginErr), loginOut)
	}
	rootURL, rootRaw, rootErr := a.startMegaRootTracedV8511(ctx, traceID, exe, "cold-root")
	st := MegaPreviewState{Active: true, SourceURL: item.URL, PreviousSession: oldSession, Exe: exe}
	if rootErr == nil {
		var child string
		var stateErr error
		st, child, stateErr = setMegaRootStateV8511(st, item, exe, rootURL)
		if stateErr != nil {
			return "", "", stateErr
		}
		a.preview = st
		a.resetPreviewTTLLocked()
		if oldSession == "" {
			a.saveMegaPreviewRestartHintV859(item.URL)
		}
		return child, "MEGA ROOT SERVICE COLD", nil
	}
	a.logf("MEGA PREVIEW [%s] root start failed; using one bounded endpoint: %v • %s", traceID, rootErr, redactMegaOutputV8511(rootRaw))
	st, fallbackURL, fallbackErr := a.startBoundedMegaFallbackLockedV8511(ctx, traceID, st, item)
	if fallbackErr != nil {
		a.restoreMegaSessionSilent(exe, oldSession)
		return "", "", fallbackErr
	}
	a.preview = st
	a.resetPreviewTTLLocked()
	return fallbackURL, "MEGA BOUNDED FALLBACK", nil
}

func (a *App) diagnoseMegaBrowserErrorV8511(item RemoteItem, traceID string) (megaPreviewResponseV8511, bool, error) {
	started := time.Now()
	a.previewMu.Lock()
	state := a.preview
	a.previewMu.Unlock()
	if !state.Active || state.SourceURL != item.URL {
		a.logf("MEGA PREVIEW [%s] BROWSER ERROR no-active-stream", traceID)
		return megaPreviewResponseV8511{}, false, nil
	}
	target := state.StreamURL
	if rootURL := rootURLFromStateV8511(state); rootURL != "" {
		child, err := megaWebDAVChildURL(rootURL, item.Path)
		if err != nil {
			return megaPreviewResponseV8511{}, true, err
		}
		target = child
	} else if state.FallbackRemotePath == megaRemoteRef(item) && state.FallbackStreamURL != "" {
		target = state.FallbackStreamURL
	}
	probe := probeMegaPreviewURLV8511(context.Background(), target)
	a.logf("MEGA PREVIEW [%s] BROWSER ERROR route=%s status=%d method=%s duration=%s error=%v", traceID, state.RemotePath, probe.Status, probe.Method, probe.Duration.Round(time.Millisecond), probe.Err)
	if probe.Reachable {
		return megaPreviewResponseV8511{URL: target, Mode: "MEGA TRANSPORT OK", Prepare: time.Since(started), TraceID: traceID, TransportOK: true, Note: "WebDAV livrează bytes; browserul nu acceptă formatul sau codecul. Nu s-a creat fallback."}, true, nil
	}
	if probe.Status == 509 || probe.Status == http.StatusTooManyRequests || strings.Contains(strings.ToLower(probe.Body), "overquota") {
		problem := classifyMegaProblem(fmt.Sprintf("HTTP %d %s", probe.Status, probe.Body), probe.Err)
		return megaPreviewResponseV8511{}, true, newMegaProblemError(problem, probe.Body)
	}

	gateCtx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	if err := acquireMegaSession(gateCtx); err != nil {
		return megaPreviewResponseV8511{}, true, err
	}
	defer releaseMegaSession()
	a.previewMu.Lock()
	defer a.previewMu.Unlock()
	state = a.preview
	if !state.Active || state.SourceURL != item.URL || state.Exe == "" {
		return megaPreviewResponseV8511{}, false, nil
	}
	rootURL, raw, rootErr := a.startMegaRootTracedV8511(context.Background(), traceID, state.Exe, "browser-root-repair")
	if rootErr == nil {
		state, target, rootErr = setMegaRootStateV8511(state, item, state.Exe, rootURL)
		if rootErr == nil {
			recheck := probeMegaPreviewURLV8511(context.Background(), target)
			a.logf("MEGA PREVIEW [%s] ROOT RECHECK status=%d method=%s duration=%s error=%v", traceID, recheck.Status, recheck.Method, recheck.Duration.Round(time.Millisecond), recheck.Err)
			if recheck.Reachable {
				if state.FallbackRemotePath != "" {
					_, _ = a.runMegaPreviewCommandV8511(context.Background(), traceID, "repair-remove-fallback", 6*time.Second, state.Exe, "webdav", "-d", state.FallbackRemotePath)
				}
				state.FallbackRemotePath, state.FallbackStreamURL = "", ""
				a.preview = state
				a.resetPreviewTTLLocked()
				return megaPreviewResponseV8511{URL: target, Mode: "MEGA ROOT SERVICE REPAIRED", Prepare: time.Since(started), TraceID: traceID, TransportOK: true, Note: "Root-ul unic a fost refăcut; nu s-a creat endpoint per-fișier."}, true, nil
			}
			if recheck.Status == 509 || recheck.Status == http.StatusTooManyRequests {
				problem := classifyMegaProblem(fmt.Sprintf("HTTP %d %s", recheck.Status, recheck.Body), recheck.Err)
				return megaPreviewResponseV8511{}, true, newMegaProblemError(problem, recheck.Body)
			}
		}
	}
	a.logf("MEGA PREVIEW [%s] FALLBACK reason=root-transport-failed rootError=%v output=%s", traceID, rootErr, redactMegaOutputV8511(raw))
	state, fallbackURL, fallbackErr := a.startBoundedMegaFallbackLockedV8511(context.Background(), traceID, state, item)
	if fallbackErr != nil {
		return megaPreviewResponseV8511{}, true, fallbackErr
	}
	a.preview = state
	a.resetPreviewTTLLocked()
	return megaPreviewResponseV8511{URL: fallbackURL, Mode: "MEGA BOUNDED FALLBACK", Prepare: time.Since(started), TraceID: traceID, FallbackUsed: true, Note: "Root-ul nu a livrat bytes; este activ un singur endpoint temporar."}, true, nil
}

func (a *App) startMegaPreviewControlledV8511(item RemoteItem, forceFallback bool, traceID string) (megaPreviewResponseV8511, error) {
	started := time.Now()
	a.logf("MEGA PREVIEW [%s] CLICK file=%s browserError=%t", traceID, item.Path, forceFallback)
	if forceFallback {
		if response, handled, err := a.diagnoseMegaBrowserErrorV8511(item, traceID); handled || err != nil {
			return response, err
		}
	}
	streamURL, mode, err := a.ensureMegaPreviewRootV8511(item, traceID)
	if err != nil {
		a.logf("MEGA PREVIEW [%s] RESULT duration=%s route=%s error=%v", traceID, time.Since(started).Round(time.Millisecond), mode, err)
		return megaPreviewResponseV8511{}, err
	}
	response := megaPreviewResponseV8511{URL: streamURL, Mode: mode, Prepare: time.Since(started), TraceID: traceID, TransportOK: true, FallbackUsed: strings.Contains(mode, "FALLBACK")}
	a.logf("MEGA PREVIEW [%s] RESULT duration=%s route=%s root=%t fallback=%t", traceID, response.Prepare.Round(time.Millisecond), mode, strings.Contains(mode, "ROOT"), response.FallbackUsed)
	a.logMegaRuntimeV8511(traceID, streamURL)
	return response, nil
}

func (a *App) logMegaRuntimeV8511(traceID, streamURL string) {
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
	serverOutput, serverErr := exec.CommandContext(ctx, "tasklist.exe", "/FI", "IMAGENAME eq MEGAcmdServer.exe", "/FO", "CSV", "/NH").CombinedOutput()
	listenerOutput, listenerErr := exec.CommandContext(ctx, "netstat.exe", "-ano", "-p", "tcp").CombinedOutput()
	listeners := make([]string, 0, 2)
	for _, line := range strings.Split(string(listenerOutput), "\n") {
		upper := strings.ToUpper(line)
		if strings.Contains(line, ":"+port) && strings.Contains(upper, "LISTEN") {
			listeners = append(listeners, strings.TrimSpace(line))
		}
	}
	a.logf("MEGA PREVIEW [%s] RUNTIME server=%s serverError=%v listener=%s listenerError=%v", traceID, strings.TrimSpace(string(serverOutput)), serverErr, strings.Join(listeners, " | "), listenerErr)
}
