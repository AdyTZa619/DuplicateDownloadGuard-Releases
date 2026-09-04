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

func (a *App) megaPreviewSameSourceRootV8511(sourceURL string) bool {
	if a == nil {
		return false
	}
	a.previewMu.Lock()
	defer a.previewMu.Unlock()
	return a.preview.Active && a.preview.SourceURL == strings.TrimSpace(sourceURL) && a.preview.RemotePath == megaWarmRootRefV86 && a.preview.StreamURL != ""
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
	ctx, cancel := context.WithTimeout(parent, 6*time.Second)
	defer cancel()
	client := &http.Client{Timeout: 6 * time.Second}
	request := func(method string, ranged bool) (*http.Response, error) {
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

func (a *App) diagnoseMegaBrowserErrorV8511(item RemoteItem, traceID string) (megaPreviewResponseV8511, bool, error) {
	started := time.Now()
	a.previewMu.Lock()
	state := a.preview
	a.previewMu.Unlock()
	if !state.Active || state.SourceURL != item.URL || state.StreamURL == "" {
		a.logf("MEGA PREVIEW [%s] BROWSER ERROR no-active-stream; fallback justified", traceID)
		return megaPreviewResponseV8511{}, false, nil
	}
	target := state.StreamURL
	if state.RemotePath == megaWarmRootRefV86 {
		child, err := megaWebDAVChildURL(state.StreamURL, item.Path)
		if err != nil {
			return megaPreviewResponseV8511{}, false, err
		}
		target = child
	}
	probe := probeMegaPreviewURLV8511(context.Background(), target)
	a.logf("MEGA PREVIEW [%s] BROWSER ERROR route=%s status=%d method=%s duration=%s error=%v", traceID, state.RemotePath, probe.Status, probe.Method, probe.Duration.Round(time.Millisecond), probe.Err)
	if probe.Reachable {
		return megaPreviewResponseV8511{
			URL:         target,
			Mode:        "MEGA TRANSPORT OK",
			Prepare:     time.Since(started),
			TraceID:     traceID,
			TransportOK: true,
			Note:        "WebDAV livrează bytes; browserul nu acceptă formatul sau codecul. Nu s-a creat fallback.",
		}, true, nil
	}
	if probe.Status == 509 || probe.Status == http.StatusTooManyRequests || strings.Contains(strings.ToLower(probe.Body), "overquota") {
		problem := classifyMegaProblem(fmt.Sprintf("HTTP %d %s", probe.Status, probe.Body), probe.Err)
		return megaPreviewResponseV8511{}, true, newMegaProblemError(problem, probe.Body)
	}
	return megaPreviewResponseV8511{}, false, nil
}

func (a *App) startMegaPreviewControlledV8511(item RemoteItem, forceFallback bool, traceID string) (megaPreviewResponseV8511, error) {
	started := time.Now()
	a.logf("MEGA PREVIEW [%s] CLICK file=%s browserError=%t", traceID, item.Path, forceFallback)
	if forceFallback {
		if response, handled, err := a.diagnoseMegaBrowserErrorV8511(item, traceID); handled || err != nil {
			return response, err
		}
	}
	streamURL, mode, prepare, err := a.startMegaPreviewForUIV854(item, forceFallback, traceID)
	if err != nil {
		a.logf("MEGA PREVIEW [%s] RESULT duration=%s route=%s error=%v", traceID, time.Since(started).Round(time.Millisecond), mode, err)
		return megaPreviewResponseV8511{}, err
	}
	response := megaPreviewResponseV8511{
		URL:          streamURL,
		Mode:         mode,
		Prepare:      prepare,
		TraceID:      traceID,
		TransportOK:  !forceFallback,
		FallbackUsed: forceFallback,
	}
	a.logf("MEGA PREVIEW [%s] RESULT duration=%s route=%s fallback=%t", traceID, time.Since(started).Round(time.Millisecond), mode, response.FallbackUsed)
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
