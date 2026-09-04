package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type AriaRPCManager struct {
	mu       sync.Mutex
	exe      string
	cmd      *exec.Cmd
	endpoint string
	secret   string
	port     int
	started  bool
}

var ariaRPCRegistry sync.Map

type ariaRPCEnvelope struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      string          `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type ariaStatus struct {
	GID             string `json:"gid"`
	Status          string `json:"status"`
	TotalLength     string `json:"totalLength"`
	CompletedLength string `json:"completedLength"`
	DownloadSpeed   string `json:"downloadSpeed"`
	ErrorCode       string `json:"errorCode"`
	ErrorMessage    string `json:"errorMessage"`
	Dir             string `json:"dir"`
	Files           []struct {
		Path            string `json:"path"`
		Length          string `json:"length"`
		CompletedLength string `json:"completedLength"`
	} `json:"files"`
}

func randomSecret() string { b := make([]byte, 16); _, _ = rand.Read(b); return hex.EncodeToString(b) }
func freeLocalPort() (int, error) {
	ln, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		return 0, e
	}
	p := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return p, nil
}

func ariaRPCFor(a *App) (*AriaRPCManager, error) {
	if x, ok := ariaRPCRegistry.Load(a); ok {
		m := x.(*AriaRPCManager)
		if err := m.ensure(a); err == nil {
			return m, nil
		}
	}
	m := &AriaRPCManager{}
	actual, _ := ariaRPCRegistry.LoadOrStore(a, m)
	m = actual.(*AriaRPCManager)
	if err := m.ensure(a); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *AriaRPCManager) ensure(a *App) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	exe := a.detectAria2()
	if exe == "" {
		return errors.New("aria2c nu este disponibil")
	}
	if m.started && m.exe == exe {
		ctx, c := context.WithTimeout(context.Background(), 700*time.Millisecond)
		var v map[string]any
		err := m.call(ctx, "aria2.getVersion", nil, &v)
		c()
		if err == nil {
			return nil
		}
		if m.cmd != nil && m.cmd.Process != nil {
			_ = m.cmd.Process.Kill()
			_, _ = m.cmd.Process.Wait()
		}
		m.cmd = nil
		m.started = false
	}
	port, e := freeLocalPort()
	if e != nil {
		return e
	}
	secret := randomSecret()
	endpoint := fmt.Sprintf("http://127.0.0.1:%d/jsonrpc", port)
	args := []string{"--enable-rpc=true", "--rpc-listen-all=false", fmt.Sprintf("--rpc-listen-port=%d", port), "--rpc-secret=" + secret, fmt.Sprintf("--stop-with-process=%d", os.Getpid()), "--file-allocation=none", "--continue=true", "--max-concurrent-downloads=16", "--max-download-result=200", "--console-log-level=warn", "--summary-interval=0", "--quiet=true"}
	cmd := exec.Command(exe, args...)
	hideChildWindow(cmd)
	if e = cmd.Start(); e != nil {
		return e
	}
	m.exe, m.cmd, m.endpoint, m.secret, m.port, m.started = exe, cmd, endpoint, secret, port, true
	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		ctx, c := context.WithTimeout(context.Background(), 500*time.Millisecond)
		var v map[string]any
		e = m.call(ctx, "aria2.getVersion", nil, &v)
		c()
		if e == nil {
			return nil
		}
		time.Sleep(120 * time.Millisecond)
	}
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}
	m.cmd = nil
	m.started = false
	return errors.New("aria2 RPC nu a pornit în 6 secunde")
}

func (m *AriaRPCManager) call(ctx context.Context, method string, params []any, out any) error {
	if !m.started {
		return errors.New("aria2 RPC oprit")
	}
	ps := []any{"token:" + m.secret}
	ps = append(ps, params...)
	body := map[string]any{"jsonrpc": "2.0", "id": "ddg", "method": method, "params": ps}
	bb, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, m.endpoint, bytes.NewReader(bb))
	req.Header.Set("Content-Type", "application/json")
	resp, e := http.DefaultClient.Do(req)
	if e != nil {
		return e
	}
	defer resp.Body.Close()
	b, e := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if e != nil {
		return e
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("aria2 RPC HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	var env ariaRPCEnvelope
	if e = json.Unmarshal(b, &env); e != nil {
		return e
	}
	if env.Error != nil {
		return fmt.Errorf("aria2 RPC %d: %s", env.Error.Code, env.Error.Message)
	}
	if out != nil {
		return json.Unmarshal(env.Result, out)
	}
	return nil
}

func (m *AriaRPCManager) add(ctx context.Context, a *App, res Result, dest string) (string, error) {
	a.mu.RLock()
	conn, retries, limit := a.cfg.AriaConnections, a.cfg.DownloadRetries, a.cfg.SpeedLimitKB
	a.mu.RUnlock()
	if conn <= 0 {
		conn = 8
	}
	if conn > 16 {
		conn = 16
	}
	if retries <= 0 {
		retries = 3
	}
	u := resultDownloadURL(res)
	if u == "" {
		return "", errors.New("URL direct lipsă")
	}
	opt := map[string]string{"dir": dest, "out": sanitizeFilename(res.Remote.Name), "continue": "true", "auto-file-renaming": "false", "allow-overwrite": "false", "file-allocation": "none", "split": strconv.Itoa(conn), "max-connection-per-server": strconv.Itoa(conn), "min-split-size": "1M", "max-tries": strconv.Itoa(retries + 1), "retry-wait": "2", "user-agent": browserUserAgentV854()}
	if ref := downloadRefererV854(res.Remote); ref != "" {
		opt["referer"] = ref
	}
	if limit > 0 {
		opt["max-download-limit"] = fmt.Sprintf("%dK", limit)
	}
	if res.Remote.Hash != "" && (strings.EqualFold(res.Remote.HashType, "sha256") || strings.EqualFold(res.Remote.HashType, "md5")) {
		typ := strings.ToLower(res.Remote.HashType)
		if typ == "sha256" {
			typ = "sha-256"
		}
		opt["checksum"] = typ + "=" + res.Remote.Hash
		opt["check-integrity"] = "true"
	}
	var gid string
	e := m.call(ctx, "aria2.addUri", []any{[]string{u}, opt}, &gid)
	return gid, e
}
func (m *AriaRPCManager) tell(ctx context.Context, gid string) (ariaStatus, error) {
	var s ariaStatus
	e := m.call(ctx, "aria2.tellStatus", []any{gid, []string{"gid", "status", "totalLength", "completedLength", "downloadSpeed", "errorCode", "errorMessage", "dir", "files"}}, &s)
	return s, e
}
func (m *AriaRPCManager) pause(ctx context.Context, gid string) error {
	var x string
	return m.call(ctx, "aria2.forcePause", []any{gid}, &x)
}
func (m *AriaRPCManager) unpause(ctx context.Context, gid string) error {
	var x string
	return m.call(ctx, "aria2.unpause", []any{gid}, &x)
}
func (m *AriaRPCManager) remove(ctx context.Context, gid string) error {
	var x string
	return m.call(ctx, "aria2.forceRemove", []any{gid}, &x)
}

func (m *AriaRPCManager) shutdown() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.started {
		return
	}
	if m.endpoint != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
		var result string
		_ = m.call(ctx, "aria2.shutdown", nil, &result)
		cancel()
	}
	if m.cmd != nil && m.cmd.Process != nil {
		done := make(chan struct{})
		go func(p *os.Process) {
			_, _ = p.Wait()
			close(done)
		}(m.cmd.Process)
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			_ = m.cmd.Process.Kill()
			<-done
		}
	}
	m.cmd = nil
	m.started = false
}

func shutdownAriaRPC(a *App) {
	if a == nil {
		return
	}
	if x, ok := ariaRPCRegistry.LoadAndDelete(a); ok {
		x.(*AriaRPCManager).shutdown()
	}
}

func parseAriaInt(s string) int64 { n, _ := strconv.ParseInt(s, 10, 64); return n }

func runAriaRPCQueueJob(ctx context.Context, a *App, q *DownloadQueue, id string, res Result, dest string) (string, error) {
	m, e := ariaRPCFor(a)
	if e != nil {
		return "", e
	}
	q.mu.Lock()
	j := q.findLocked(id)
	gid := ""
	if j != nil {
		gid = j.GID
	}
	q.mu.Unlock()
	if gid != "" {
		c, cc := context.WithTimeout(context.Background(), 2*time.Second)
		e = m.unpause(c, gid)
		cc()
		if e != nil {
			gid = ""
		}
	}
	if gid == "" {
		c, cc := context.WithTimeout(ctx, 10*time.Second)
		gid, e = m.add(c, a, res, dest)
		cc()
		if e != nil {
			return "", e
		}
		q.mu.Lock()
		if j := q.findLocked(id); j != nil {
			j.GID = gid
			j.UpdatedAt = time.Now().Unix()
		}
		q.mu.Unlock()
		q.save(a)
	}
	tick := time.NewTicker(400 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			q.mu.Lock()
			state := ""
			if j := q.findLocked(id); j != nil {
				state = j.Status
			}
			q.mu.Unlock()
			c, cc := context.WithTimeout(context.Background(), 1500*time.Millisecond)
			if state == "cancelled" || state == "blocked" {
				_ = m.remove(c, gid)
			} else {
				_ = m.pause(c, gid)
			}
			cc()
			return "", ctx.Err()
		case <-tick.C:
			c, cc := context.WithTimeout(context.Background(), 2*time.Second)
			st, er := m.tell(c, gid)
			cc()
			if er != nil {
				return "", er
			}
			total, done, speed := parseAriaInt(st.TotalLength), parseAriaInt(st.CompletedLength), parseAriaInt(st.DownloadSpeed)
			eta := int64(-1)
			if speed > 0 && total > done {
				eta = (total - done) / speed
			}
			q.mu.Lock()
			if j := q.findLocked(id); j != nil {
				j.GID = gid
				j.BytesTotal = total
				j.BytesDone = done
				j.SpeedBps = speed
				j.ETA = eta
				j.UpdatedAt = time.Now().Unix()
			}
			q.mu.Unlock()
			switch st.Status {
			case "complete":
				path := ""
				if len(st.Files) > 0 {
					path = st.Files[0].Path
				}
				if path == "" {
					path = filepath.Join(dest, sanitizeFilename(res.Remote.Name))
				}
				return path, nil
			case "error":
				return "", fmt.Errorf("aria2 [%s]: %s", st.ErrorCode, st.ErrorMessage)
			case "removed":
				return "", errors.New("aria2 job eliminat")
			case "paused":
				q.mu.Lock()
				paused := false
				if j := q.findLocked(id); j != nil {
					paused = j.Status == "paused" || j.Status == "cancelled"
				}
				q.mu.Unlock()
				if paused {
					return "", context.Canceled
				}
			}
		}
	}
}
