package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// v8: portable storage, managed external tools, local AI review via Ollama,
// a persistent download queue and an app-update handoff that can replace the
// executable after the current process exits. Everything here stays in the
// standard library so the Windows build remains a single native x64 executable.

func executableDir() string {
	p, err := os.Executable()
	if err != nil || p == "" {
		wd, _ := os.Getwd()
		return wd
	}
	p, _ = filepath.Abs(p)
	return filepath.Dir(p)
}

func portableDataDir() (string, error) {
	dir := filepath.Join(executableDir(), "data")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("nu pot crea folderul portabil %s: %w", dir, err)
	}
	_ = os.MkdirAll(filepath.Join(dir, "cache"), 0755)
	_ = os.MkdirAll(filepath.Join(dir, "updates"), 0755)
	_ = os.MkdirAll(filepath.Join(executableDir(), "tools"), 0755)
	_ = os.MkdirAll(filepath.Join(executableDir(), "downloads"), 0755)
	return dir, nil
}

func portableToolsDir() string     { return filepath.Join(executableDir(), "tools") }
func portableDownloadsDir() string { return filepath.Join(executableDir(), "downloads") }

func copyFileSimple(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	_, cpErr := io.Copy(out, in)
	syncErr := out.Sync()
	closeErr := out.Close()
	if cpErr != nil {
		return cpErr
	}
	if syncErr != nil {
		return syncErr
	}
	return closeErr
}

func migrateLegacyPortableData(newDir string) error {
	marker := filepath.Join(newDir, ".legacy_migrated")
	if _, err := os.Stat(marker); err == nil {
		return nil
	}
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		_ = os.WriteFile(marker, []byte("no legacy base"), 0644)
		return nil
	}
	old := filepath.Join(base, "DuplicateDownloadGuard")
	if filepath.Clean(old) == filepath.Clean(newDir) {
		return nil
	}
	files := []string{"config.json", "index.gob.gz", "last_results.json.gz", "manual_decisions.json", "DuplicateDownloadGuard.log", "yt-dlp.archive.txt", "download_queue.json"}
	for _, n := range files {
		src, dst := filepath.Join(old, n), filepath.Join(newDir, n)
		if _, e := os.Stat(dst); e == nil {
			continue
		}
		if st, e := os.Stat(src); e == nil && !st.IsDir() {
			_ = copyFileSimple(src, dst)
		}
	}
	return os.WriteFile(marker, []byte(time.Now().Format(time.RFC3339)), 0644)
}

// ---------- Managed tools ----------

type ghRelease struct {
	TagName string `json:"tag_name"`
	Name    string `json:"name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

type managedTool struct {
	Name        string `json:"name"`
	Found       bool   `json:"found"`
	Path        string `json:"path,omitempty"`
	Version     string `json:"version,omitempty"`
	Portable    bool   `json:"portable"`
	Role        string `json:"role"`
	Installable bool   `json:"installable"`
	Large       bool   `json:"large,omitempty"`
}

func githubLatestRelease(ctx context.Context, repo string) (ghRelease, error) {
	var rel ghRelease
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/repos/"+repo+"/releases/latest", nil)
	req.Header.Set("User-Agent", "DuplicateDownloadGuard/8.0")
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return rel, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return rel, fmt.Errorf("GitHub API HTTP %d", resp.StatusCode)
	}
	err = json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&rel)
	return rel, err
}

func releaseAsset(rel ghRelease, rx string) (string, string, error) {
	re, err := regexp.Compile(rx)
	if err != nil {
		return "", "", err
	}
	for _, a := range rel.Assets {
		if re.MatchString(a.Name) {
			return a.Name, a.BrowserDownloadURL, nil
		}
	}
	return "", "", fmt.Errorf("asset compatibil lipsă în release %s", rel.TagName)
}

func downloadToFile(ctx context.Context, url, dst string, progress func(done, total int64)) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	req.Header.Set("User-Agent", "DuplicateDownloadGuard/8.0")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	tmp := dst + ".download"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	total := resp.ContentLength
	var done int64
	buf := make([]byte, 1<<20)
	last := time.Now()
	for {
		n, er := resp.Body.Read(buf)
		if n > 0 {
			if _, err := f.Write(buf[:n]); err != nil {
				return err
			}
			done += int64(n)
			if time.Since(last) >= 250*time.Millisecond {
				progress(done, total)
				last = time.Now()
			}
		}
		if er == io.EOF {
			break
		}
		if er != nil {
			return er
		}
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	progress(done, total)
	_ = os.Remove(dst)
	return os.Rename(tmp, dst)
}

func unzipSelected(zipPath, dest string, wanted map[string]string) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer zr.Close()
	found := map[string]bool{}
	for _, f := range zr.File {
		base := strings.ToLower(filepath.Base(f.Name))
		outName, ok := wanted[base]
		if !ok {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		outPath := filepath.Join(dest, outName)
		if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
			rc.Close()
			return err
		}
		out, err := os.Create(outPath)
		if err != nil {
			rc.Close()
			return err
		}
		_, cp := io.Copy(out, rc)
		rc.Close()
		cl := out.Close()
		if cp != nil {
			return cp
		}
		if cl != nil {
			return cl
		}
		found[base] = true
	}
	for k := range wanted {
		if !found[k] {
			return fmt.Errorf("%s nu a fost găsit în arhivă", k)
		}
	}
	return nil
}

func (a *App) installManagedTool(ctx context.Context, tool string) error {
	tool = strings.ToLower(strings.TrimSpace(tool))
	tools := portableToolsDir()
	setProg := func(name string, done, total int64) {
		a.updateProgress(func(p *Progress) {
			p.Message = "Instalez " + name
			p.Bytes = done
			p.Total = total
			if total > 0 {
				p.Detail = fmt.Sprintf("%s / %s", human(done), human(total))
			}
		})
	}
	switch tool {
	case "yt-dlp", "ytdlp":
		dir := filepath.Join(tools, "yt-dlp")
		dst := filepath.Join(dir, "yt-dlp.exe")
		return downloadToFile(ctx, "https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp.exe", dst, func(d, t int64) { setProg("yt-dlp", d, t) })
	case "gallery-dl", "gallerydl":
		rel, err := githubLatestRelease(ctx, "gdl-org/builds")
		if err != nil {
			return err
		}
		_, url, err := releaseAsset(rel, `(?i)^gallery-dl_windows\.exe$`)
		if err != nil {
			return err
		}
		dir := filepath.Join(tools, "gallery-dl")
		dst := filepath.Join(dir, "gallery-dl.exe")
		return downloadToFile(ctx, url, dst, func(d, t int64) { setProg("gallery-dl", d, t) })
	case "aria2", "aria2c":
		rel, err := githubLatestRelease(ctx, "aria2/aria2")
		if err != nil {
			return err
		}
		_, url, err := releaseAsset(rel, `(?i)win-64bit.*\.zip$`)
		if err != nil {
			return err
		}
		dir := filepath.Join(tools, "aria2")
		zipPath := filepath.Join(a.appDir, "cache", "aria2.zip")
		if err = downloadToFile(ctx, url, zipPath, func(d, t int64) { setProg("aria2", d, t) }); err != nil {
			return err
		}
		err = unzipSelected(zipPath, dir, map[string]string{"aria2c.exe": "aria2c.exe"})
		_ = os.Remove(zipPath)
		return err
	case "ffmpeg":
		rel, err := githubLatestRelease(ctx, "BtbN/FFmpeg-Builds")
		if err != nil {
			return err
		}
		_, url, err := releaseAsset(rel, `(?i)^ffmpeg-master-latest-win64-gpl\.zip$`)
		if err != nil {
			return err
		}
		dir := filepath.Join(tools, "ffmpeg")
		zipPath := filepath.Join(a.appDir, "cache", "ffmpeg.zip")
		if err = downloadToFile(ctx, url, zipPath, func(d, t int64) { setProg("FFmpeg", d, t) }); err != nil {
			return err
		}
		err = unzipSelected(zipPath, dir, map[string]string{"ffmpeg.exe": "ffmpeg.exe", "ffprobe.exe": "ffprobe.exe"})
		_ = os.Remove(zipPath)
		return err
	default:
		return fmt.Errorf("motor necunoscut: %s", tool)
	}
}

func (a *App) managedToolsSnapshot() []managedTool {
	toolDir := filepath.Clean(portableToolsDir())
	inPortable := func(p string) bool {
		if p == "" {
			return false
		}
		q, _ := filepath.Abs(p)
		return strings.HasPrefix(strings.ToLower(filepath.Clean(q)), strings.ToLower(toolDir)+string(os.PathSeparator)) || strings.EqualFold(filepath.Clean(q), toolDir)
	}
	rows := []managedTool{
		{Name: "MEGAcmd", Path: a.detectMegaClient(), Role: "MEGA: listare / preview / verificare / download", Installable: false},
		{Name: "yt-dlp", Path: a.detectYtDlp(), Role: "site-uri video + download", Installable: true},
		{Name: "gallery-dl", Path: a.detectGalleryDL(), Role: "galerii / imagini", Installable: true},
		{Name: "aria2", Path: a.detectAria2(), Role: "HTTP rapid, segmentat, resume", Installable: true},
		{Name: "FFmpeg", Path: a.detectFFmpeg(), Role: "fingerprint video + media", Installable: true, Large: true},
		{Name: "ffprobe", Path: a.detectFFprobe(), Role: "metadate media", Installable: true, Large: true},
	}
	for i := range rows {
		rows[i].Found = rows[i].Path != ""
		rows[i].Portable = inPortable(rows[i].Path)
		if rows[i].Found {
			rows[i].Version = toolVersion(rows[i].Path)
		}
	}
	return rows
}

func (a *App) handleManagedTools(w http.ResponseWriter, r *http.Request) {
	jsonOut(w, map[string]any{"tools": a.managedToolsSnapshot(), "toolsDir": portableToolsDir(), "portable": true})
}

func (a *App) handleToolManage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST necesar", 405)
		return
	}
	var req struct{ Tool, Action string }
	if err := decodeJSON(r, &req); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if a.opRunning.Load() {
		http.Error(w, "există deja o operație în curs", 409)
		return
	}
	list := []string{req.Tool}
	if strings.EqualFold(req.Tool, "recommended") || strings.EqualFold(req.Tool, "all") {
		list = []string{"yt-dlp", "gallery-dl", "aria2", "ffmpeg"}
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.mu.Lock()
	a.cancel = cancel
	a.mu.Unlock()
	a.opRunning.Store(true)
	a.setProgress(Progress{Active: true, Phase: "tools", State: "running", Message: "Pregătesc Tool Manager", Step: 1, StepTotal: len(list), StartedAt: time.Now().Unix(), CanCancel: true})
	go func() {
		defer func() {
			a.opRunning.Store(false)
			a.mu.Lock()
			a.cancel = nil
			a.progress.Active = false
			a.progress.CanCancel = false
			a.mu.Unlock()
		}()
		for i, t := range list {
			if ctx.Err() != nil {
				a.failOp("Instalare anulată", ctx.Err().Error())
				return
			}
			a.updateProgress(func(p *Progress) {
				p.Step = i + 1
				p.StepTotal = len(list)
				p.Message = "Instalez / actualizez " + t
				p.Detail = "Surse oficiale / release-uri publice"
			})
			if err := a.installManagedTool(ctx, t); err != nil {
				a.failOp("Tool Manager: "+t, err.Error())
				return
			}
			a.logf("Tool Manager: %s instalat/actualizat", t)
		}
		a.endOp("Motoarele portabile sunt gata ✓")
	}()
	jsonOut(w, map[string]any{"started": true, "tools": list})
}

// ---------- Batch discovery ----------

func (a *App) discoverOne(ctx context.Context, u, adapter string) ([]RemoteItem, string, error) {
	adapter = strings.ToLower(strings.TrimSpace(adapter))
	if adapter == "" {
		adapter = "auto"
	}
	var errs []string
	if adapter == "auto" || adapter == "http" {
		if x, e := probeHTTPMeta(u); e == nil && !isLikelyHTML(x) {
			return []RemoteItem{x}, "http", nil
		} else if e != nil {
			errs = append(errs, e.Error())
		}
	}
	if adapter == "auto" || adapter == "yt-dlp" {
		if x, e := a.probeYtDlp(ctx, u); e == nil && len(x) > 0 {
			return x, "yt-dlp", nil
		} else if e != nil {
			errs = append(errs, e.Error())
		}
	}
	if adapter == "auto" || adapter == "gallery-dl" {
		if x, e := a.probeGalleryDL(ctx, u); e == nil && len(x) > 0 {
			return x, "gallery-dl", nil
		} else if e != nil {
			errs = append(errs, e.Error())
		}
	}
	if adapter == "http" {
		if x, e := probeHTTPMeta(u); e == nil {
			return []RemoteItem{x}, "http", nil
		} else {
			errs = append(errs, e.Error())
		}
	}
	return nil, "", errors.New(strings.Join(errs, " | "))
}

func (a *App) handleBatchSourceScan(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URLs                []string `json:"urls"`
		Text, Adapter, Mode string
	}
	if err := decodeJSON(r, &req); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if len(req.URLs) == 0 {
		for _, x := range strings.FieldsFunc(req.Text, func(r rune) bool { return r == '\n' || r == '\r' || r == '\t' }) {
			x = strings.TrimSpace(x)
			if x != "" {
				req.URLs = append(req.URLs, x)
			}
		}
	}
	if len(req.URLs) == 0 {
		http.Error(w, "nu ai introdus linkuri", 400)
		return
	}
	if len(req.URLs) > 500 {
		http.Error(w, "maxim 500 linkuri per lot", 400)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Minute)
	defer cancel()
	var all []RemoteItem
	var failures []string
	used := map[string]int{}
	for _, u := range req.URLs {
		items, ad, e := a.discoverOne(ctx, strings.TrimSpace(u), req.Adapter)
		if e != nil {
			failures = append(failures, u+" — "+e.Error())
			continue
		}
		used[ad] += len(items)
		all = append(all, items...)
	}
	if len(all) == 0 {
		http.Error(w, "niciun link nu a produs fișiere: "+strings.Join(failures, " | "), 422)
		return
	}
	a.compareRemote(context.Background(), all, req.Mode)
	jsonOut(w, map[string]any{"ok": true, "links": len(req.URLs), "items": len(all), "adapters": used, "failures": failures})
}

// ---------- Local AI (Ollama) ----------

type ollamaTags struct {
	Models []struct {
		Name  string `json:"name"`
		Model string `json:"model"`
	} `json:"models"`
}
type aiAnswer struct {
	Verdict      string   `json:"verdict"`
	Confidence   int      `json:"confidence"`
	Reason       string   `json:"reason"`
	Observations []string `json:"observations"`
}

func normalizeEndpoint(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		s = "http://127.0.0.1:11434"
	}
	return strings.TrimRight(s, "/")
}

func (a *App) ollamaTags(ctx context.Context) (ollamaTags, error) {
	a.mu.RLock()
	ep := normalizeEndpoint(a.cfg.AIEndpoint)
	a.mu.RUnlock()
	var out ollamaTags
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, ep+"/api/tags", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return out, fmt.Errorf("AI HTTP %d", resp.StatusCode)
	}
	err = json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&out)
	return out, err
}

func (a *App) handleAIStatus(w http.ResponseWriter, r *http.Request) {
	ctx, c := context.WithTimeout(r.Context(), 2*time.Second)
	defer c()
	tags, err := a.ollamaTags(ctx)
	a.mu.RLock()
	ep, model, enabled, vision := normalizeEndpoint(a.cfg.AIEndpoint), a.cfg.AIModel, a.cfg.AIEnabled, a.cfg.AIVision
	a.mu.RUnlock()
	names := []string{}
	for _, m := range tags.Models {
		n := m.Name
		if n == "" {
			n = m.Model
		}
		if n != "" {
			names = append(names, n)
		}
	}
	jsonOut(w, map[string]any{"available": err == nil, "error": func() string {
		if err != nil {
			return err.Error()
		}
		return ""
	}(), "endpoint": ep, "model": model, "enabled": enabled, "vision": vision, "models": names, "note": "AI este consultativ; nu produce verdict 100% fără dovadă byte/hash."})
}
func (a *App) handleAIModels(w http.ResponseWriter, r *http.Request) {
	ctx, c := context.WithTimeout(r.Context(), 3*time.Second)
	defer c()
	t, e := a.ollamaTags(ctx)
	if e != nil {
		http.Error(w, e.Error(), 503)
		return
	}
	jsonOut(w, t)
}

func jpegFrame(ctx context.Context, ff, target string, sec float64) ([]byte, error) {
	if sec < 0 {
		sec = 0
	}
	args := []string{"-v", "error", "-ss", fmt.Sprintf("%.3f", sec), "-i", target, "-frames:v", "1", "-vf", "scale='min(768,iw)':-2:flags=lanczos", "-q:v", "5", "-f", "image2pipe", "-vcodec", "mjpeg", "pipe:1"}
	cmd := exec.CommandContext(ctx, ff, args...)
	hideChildWindow(cmd)
	b, e := cmd.Output()
	if e != nil {
		return nil, e
	}
	if len(b) < 256 {
		return nil, errors.New("cadru AI gol")
	}
	if len(b) > 4<<20 {
		return nil, errors.New("cadru AI prea mare")
	}
	return b, nil
}

func readImageForAI(ctx context.Context, target string, max int64) ([]byte, error) {
	if strings.HasPrefix(strings.ToLower(target), "http://") || strings.HasPrefix(strings.ToLower(target), "https://") {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
		resp, e := http.DefaultClient.Do(req)
		if e != nil {
			return nil, e
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 400 {
			return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
		}
		return io.ReadAll(io.LimitReader(resp.Body, max+1))
	}
	f, e := os.Open(target)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, max+1))
}

func (a *App) aiImages(ctx context.Context, res Result, local string) ([][]byte, error) {
	kind := remoteMediaKind(res.Remote.Name)
	if kind != "image" && kind != "video" {
		return nil, nil
	}
	target, e := remoteTarget(a, res)
	if e != nil {
		return nil, e
	}
	if kind == "image" {
		rb, e := readImageForAI(ctx, target, 4<<20)
		if e != nil {
			return nil, e
		}
		lb, e := readImageForAI(ctx, local, 4<<20)
		if e != nil {
			return nil, e
		}
		if len(rb) > 4<<20 || len(lb) > 4<<20 {
			return nil, errors.New("imagini prea mari pentru AI")
		}
		return [][]byte{rb, lb}, nil
	}
	ff, fp := a.detectFFmpeg(), a.detectFFprobe()
	if ff == "" || fp == "" {
		return nil, errors.New("ffmpeg/ffprobe lipsesc pentru AI video")
	}
	ri := probeMedia(ctx, fp, target, "REMOTE")
	li := probeMedia(ctx, fp, local, "LOCAL")
	sec := 0.0
	if ri.OK && li.OK {
		d := ri.Duration
		if li.Duration < d || d <= 0 {
			d = li.Duration
		}
		if d > 0 {
			sec = d * .5
		}
	}
	rb, e := jpegFrame(ctx, ff, target, sec)
	if e != nil {
		return nil, e
	}
	lb, e := jpegFrame(ctx, ff, local, sec)
	if e != nil {
		return nil, e
	}
	return [][]byte{rb, lb}, nil
}

func (a *App) callOllama(ctx context.Context, res Result, local string, useVision bool) (aiAnswer, error) {
	var ans aiAnswer
	a.mu.RLock()
	ep, model := normalizeEndpoint(a.cfg.AIEndpoint), strings.TrimSpace(a.cfg.AIModel)
	a.mu.RUnlock()
	if model == "" {
		tags, e := a.ollamaTags(ctx)
		if e != nil {
			return ans, e
		}
		if len(tags.Models) == 0 {
			return ans, errors.New("Ollama rulează, dar nu există niciun model instalat")
		}
		model = tags.Models[0].Name
		if model == "" {
			model = tags.Models[0].Model
		}
	}
	st, _ := os.Stat(local)
	localSize := int64(-1)
	if st != nil {
		localSize = st.Size()
	}
	prompt := fmt.Sprintf(`Ești un asistent de verificare duplicate. Analizează strict dovezile și răspunde JSON: {"verdict":"same|probably_same|different|uncertain","confidence":0-100,"reason":"...","observations":["..."]}. Nu spune same doar din nume. Remote: name=%q size=%d source=%s duration=%.3f hashType=%s. Local: path=%q size=%d. Scor algoritm=%d, nume=%d, sameSize=%v, sameExt=%v, motiv=%q, fingerprint=%d%% (%s). Dacă primești 2 imagini: prima este REMOTE, a doua LOCAL.`, res.Remote.Name, res.Remote.Size, res.Remote.Source, res.Remote.Duration, res.Remote.HashType, local, localSize, res.MatchScore, res.NameScore, res.SameSize, res.SameExt, res.AutoReason, res.VisualScore, res.VisualMethod)
	msg := map[string]any{"role": "user", "content": prompt}
	if useVision {
		imgs, e := a.aiImages(ctx, res, local)
		if e == nil && len(imgs) > 0 {
			arr := []string{}
			for _, b := range imgs {
				arr = append(arr, base64.StdEncoding.EncodeToString(b))
			}
			msg["images"] = arr
		}
	}
	body := map[string]any{"model": model, "stream": false, "format": "json", "options": map[string]any{"temperature": 0.1}, "messages": []any{msg}}
	bb, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, ep+"/api/chat", bytes.NewReader(bb))
	req.Header.Set("Content-Type", "application/json")
	resp, e := http.DefaultClient.Do(req)
	if e != nil {
		return ans, e
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return ans, fmt.Errorf("AI HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	var raw struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}
	if e = json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&raw); e != nil {
		return ans, e
	}
	content := strings.TrimSpace(raw.Message.Content)
	start, end := strings.Index(content, "{"), strings.LastIndex(content, "}")
	if start >= 0 && end > start {
		content = content[start : end+1]
	}
	if e = json.Unmarshal([]byte(content), &ans); e != nil {
		return ans, fmt.Errorf("răspuns AI invalid: %w", e)
	}
	if ans.Confidence < 0 {
		ans.Confidence = 0
	}
	if ans.Confidence > 100 {
		ans.Confidence = 100
	}
	ans.Verdict = strings.ToLower(strings.TrimSpace(ans.Verdict))
	if ans.Verdict == "" {
		ans.Verdict = "uncertain"
	}
	a.mu.Lock()
	for i := range a.results {
		if a.results[i].ID == res.ID {
			a.results[i].AIVerdict = ans.Verdict
			a.results[i].AIConfidence = ans.Confidence
			a.results[i].AIReason = ans.Reason
			a.results[i].AIModel = model
			a.results[i].AIAt = time.Now().Unix()
			break
		}
	}
	a.mu.Unlock()
	a.revision.Add(1)
	_ = a.saveResults()
	return ans, nil
}

func (a *App) handleAIAnalyze(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID     int    `json:"id"`
		Path   string `json:"path"`
		Vision bool   `json:"vision"`
	}
	if e := decodeJSON(r, &req); e != nil {
		http.Error(w, e.Error(), 400)
		return
	}
	res, ok := a.resultByID(req.ID)
	if !ok {
		http.Error(w, "rezultat inexistent", 404)
		return
	}
	local := strings.TrimSpace(req.Path)
	if local == "" {
		local = res.LocalPath
	}
	if local == "" || !a.localPathAllowed(local) {
		http.Error(w, "candidat local invalid", 400)
		return
	}
	ctx, c := context.WithTimeout(r.Context(), 3*time.Minute)
	defer c()
	ans, e := a.callOllama(ctx, res, local, req.Vision)
	if e != nil {
		http.Error(w, e.Error(), 502)
		return
	}
	rr, _ := a.resultByID(req.ID)
	jsonOut(w, map[string]any{"answer": ans, "result": rr, "advisory": true})
}

// ---------- Persistent download queue ----------

type DownloadJob struct {
	ID            string `json:"id"`
	ResultID      int    `json:"resultId"`
	Name          string `json:"name"`
	Source        string `json:"source"`
	URL           string `json:"url,omitempty"`
	Destination   string `json:"destination"`
	Engine        string `json:"engine"`
	GID           string `json:"gid,omitempty"`
	Status        string `json:"status"` // queued/running/paused/completed/failed/cancelled/blocked
	Priority      int    `json:"priority"`
	BytesDone     int64  `json:"bytesDone"`
	BytesTotal    int64  `json:"bytesTotal"`
	SpeedBps      int64  `json:"speedBps"`
	ETA           int64  `json:"etaSeconds"`
	Attempts      int    `json:"attempts"`
	MaxRetries    int    `json:"maxRetries"`
	Error         string `json:"error,omitempty"`
	OutputPath    string `json:"outputPath,omitempty"`
	Verification  string `json:"verification,omitempty"`
	GuardMode     string `json:"guardMode,omitempty"`
	GuardVerdict  string `json:"guardVerdict,omitempty"`
	GuardReason   string `json:"guardReason,omitempty"`
	GuardMethod   string `json:"guardMethod,omitempty"`
	GuardVersion  int    `json:"guardVersion,omitempty"`
	GuardAt       int64  `json:"guardAt,omitempty"`
	GuardOverride bool   `json:"guardOverride,omitempty"`
	AddedAt       int64  `json:"addedAt"`
	StartedAt     int64  `json:"startedAt,omitempty"`
	FinishedAt    int64  `json:"finishedAt,omitempty"`
	UpdatedAt     int64  `json:"updatedAt"`
}

type DownloadQueue struct {
	mu      sync.Mutex
	Jobs    []*DownloadJob
	Cancels map[string]context.CancelFunc
	Started bool
	Seq     atomic.Uint64
}

var queueRegistry sync.Map
var megaQueueMu sync.Mutex

func queueFor(a *App) *DownloadQueue {
	if q, ok := queueRegistry.Load(a); ok {
		return q.(*DownloadQueue)
	}
	q := &DownloadQueue{Cancels: map[string]context.CancelFunc{}}
	recoveredPaused := false
	if b, e := os.ReadFile(filepath.Join(a.appDir, "download_queue.json")); e == nil {
		_ = json.Unmarshal(b, &q.Jobs)
		for _, j := range q.Jobs {
			// Never restart network transfers automatically after an app restart/crash.
			// The user must explicitly resume them from Download Studio.
			if j.Status == "running" || j.Status == "queued" {
				j.Status = "paused"
				j.Error = "Pus pe pauză după repornirea aplicației; apasă Resume pentru continuare."
				j.GuardVersion = 0
				recoveredPaused = true
			}
		}
	}
	actual, _ := queueRegistry.LoadOrStore(a, q)
	q = actual.(*DownloadQueue)
	if recoveredPaused {
		q.save(a)
	}
	q.mu.Lock()
	if !q.Started {
		q.Started = true
		go q.scheduler(a)
	}
	q.mu.Unlock()
	return q
}
func (q *DownloadQueue) save(a *App) {
	q.mu.Lock()
	b, _ := json.MarshalIndent(q.Jobs, "", "  ")
	q.mu.Unlock()
	tmp := filepath.Join(a.appDir, "download_queue.json.tmp")
	dst := filepath.Join(a.appDir, "download_queue.json")
	if os.WriteFile(tmp, b, 0644) == nil {
		_ = os.Rename(tmp, dst)
	}
}
func (q *DownloadQueue) snapshot() []DownloadJob {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := make([]DownloadJob, 0, len(q.Jobs))
	for _, j := range q.Jobs {
		out = append(out, *j)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Status == "running" && out[j].Status != "running" {
			return true
		}
		if out[j].Status == "running" && out[i].Status != "running" {
			return false
		}
		if out[i].Priority != out[j].Priority {
			return out[i].Priority > out[j].Priority
		}
		return out[i].AddedAt < out[j].AddedAt
	})
	return out
}
func (q *DownloadQueue) scheduler(a *App) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for range ticker.C {
		a.mu.RLock()
		limit := a.cfg.DownloadConcurrency
		a.mu.RUnlock()
		if limit <= 0 {
			limit = 2
		}
		if limit > 8 {
			limit = 8
		}
		q.mu.Lock()
		launched := false
		running := 0
		megaRunning := false
		for _, j := range q.Jobs {
			if j.Status == "running" {
				running++
				if strings.EqualFold(j.Source, "MEGA") || strings.EqualFold(j.Engine, "mega") {
					megaRunning = true
				}
			}
		}
		slots := limit - running
		if slots > 0 {
			cands := []*DownloadJob{}
			for _, j := range q.Jobs {
				if j.Status == "queued" {
					cands = append(cands, j)
				}
			}
			sort.SliceStable(cands, func(i, j int) bool {
				if cands[i].Priority != cands[j].Priority {
					return cands[i].Priority > cands[j].Priority
				}
				return cands[i].AddedAt < cands[j].AddedAt
			})
			launchedCount := 0
			for _, j := range cands {
				if launchedCount >= slots {
					break
				}
				isMega := strings.EqualFold(j.Source, "MEGA") || strings.EqualFold(j.Engine, "mega")
				if isMega && megaRunning {
					continue
				}
				j.Status = "running"
				j.StartedAt = time.Now().Unix()
				j.UpdatedAt = j.StartedAt
				ctx, cancel := context.WithCancel(context.Background())
				q.Cancels[j.ID] = cancel
				launched = true
				launchedCount++
				if isMega {
					megaRunning = true
				}
				go q.runJob(a, j.ID, ctx)
			}
		}
		q.mu.Unlock()
		if launched {
			q.save(a)
		}
	}
}
func (q *DownloadQueue) findLocked(id string) *DownloadJob {
	for _, j := range q.Jobs {
		if j.ID == id {
			return j
		}
	}
	return nil
}
func (q *DownloadQueue) update(a *App, id string, fn func(*DownloadJob)) {
	q.mu.Lock()
	if j := q.findLocked(id); j != nil {
		fn(j)
		j.UpdatedAt = time.Now().Unix()
	}
	q.mu.Unlock()
	q.save(a)
}

func chooseQueueEngine(a *App, res Result, requested string) string {
	e := strings.ToLower(strings.TrimSpace(requested))
	if e != "" && e != "auto" {
		return e
	}
	if strings.EqualFold(res.Remote.Source, "MEGA") {
		return "mega"
	}
	if strings.EqualFold(res.Remote.Source, "YT-DLP") && a.detectYtDlp() != "" {
		return "yt-dlp"
	}
	if a.detectAria2() != "" && resultDownloadURL(res) != "" {
		return "aria2"
	}
	return "internal"
}

func (q *DownloadQueue) runJob(a *App, id string, ctx context.Context) {
	defer func() { q.mu.Lock(); delete(q.Cancels, id); q.mu.Unlock(); q.save(a) }()
	q.mu.Lock()
	j := q.findLocked(id)
	if j == nil {
		q.mu.Unlock()
		return
	}
	rid, engine, dest := j.ResultID, j.Engine, j.Destination
	q.mu.Unlock()
	res, ok := a.resultByID(rid)
	if !ok {
		q.update(a, id, func(x *DownloadJob) { x.Status = "failed"; x.Error = "rezultatul sursă nu mai există" })
		return
	}
	q.mu.Lock()
	guardVersion, guardOverride, guardMode := 0, false, ""
	if current := q.findLocked(id); current != nil {
		guardVersion, guardOverride, guardMode = current.GuardVersion, current.GuardOverride, current.GuardMode
	}
	q.mu.Unlock()
	// Jobs created by an older version, recovered after a crash, or resumed
	// later are checked again by the backend before any downloader is started.
	if guardVersion != downloadGuardVersion {
		report, guardErr := a.runDownloadGuard(ctx, []Result{res}, dest, guardMode)
		if guardErr != nil {
			if ctx.Err() == nil {
				q.update(a, id, func(x *DownloadJob) {
					x.Status = "paused"
					x.Error = "ExactGuard nu a putut verifica în siguranță: " + guardErr.Error()
				})
			}
			return
		}
		decision := report.Decisions[0]
		q.update(a, id, func(x *DownloadJob) {
			x.GuardMode = report.Mode
			x.GuardVerdict = decision.Verdict
			x.GuardReason = decision.Reason
			x.GuardMethod = decision.Method
			x.GuardVersion = downloadGuardVersion
			x.GuardAt = time.Now().Unix()
		})
		switch decision.Verdict {
		case guardDuplicate:
			q.update(a, id, func(x *DownloadJob) {
				x.Status = "blocked"
				x.Error = "ExactGuard: duplicat exact blocat — " + decision.Reason
				x.Verification = "duplicat exact; 0 bytes descărcați"
				x.FinishedAt = time.Now().Unix()
			})
			return
		case guardReview:
			if !guardOverride {
				q.update(a, id, func(x *DownloadJob) {
					x.Status = "paused"
					x.Error = "ExactGuard REVIEW: " + decision.Reason
					x.Verification = "necesită confirmare înainte de download"
				})
				return
			}
		}
		if refreshed, exists := a.resultByID(rid); exists {
			res = refreshed
		}
	}
	engine = chooseQueueEngine(a, res, engine)
	q.update(a, id, func(x *DownloadJob) {
		x.Engine = engine
		x.BytesTotal = res.Remote.Size
		if x.MaxRetries <= 0 {
			x.MaxRetries = 3
		}
	})
	a.mu.RLock()
	cfgRetries := a.cfg.DownloadRetries
	a.mu.RUnlock()
	if cfgRetries <= 0 {
		cfgRetries = 3
	}
	attempts := 0
	for {
		if ctx.Err() != nil {
			return
		}
		attempts++
		q.update(a, id, func(x *DownloadJob) { x.Attempts = attempts; x.Error = "" })
		var path string
		var err error
		start := time.Now()
		lastDone := int64(0)
		lastAt := start
		progress := func(done, total int64) {
			now := time.Now()
			dt := now.Sub(lastAt).Seconds()
			speed := int64(0)
			if dt > .15 {
				speed = int64(float64(done-lastDone) / dt)
				lastDone = done
				lastAt = now
			}
			eta := int64(-1)
			if speed > 0 && total > done {
				eta = (total - done) / speed
			}
			q.mu.Lock()
			if x := q.findLocked(id); x != nil {
				x.BytesDone = done
				if total > 0 {
					x.BytesTotal = total
				}
				if speed > 0 {
					x.SpeedBps = speed
				}
				x.ETA = eta
				x.UpdatedAt = time.Now().Unix()
			}
			q.mu.Unlock()
		}
		switch engine {
		case "mega":
			megaQueueMu.Lock()
			if a.opRunning.Load() {
				megaQueueMu.Unlock()
				err = errors.New("MEGA este ocupat cu scanare/preview; retry automat")
			} else {
				err = a.downloadMegaResults(ctx, []Result{res}, dest)
				if err == nil {
					path = filepath.Join(dest, sanitizeFilename(res.Remote.Name))
				}
				megaQueueMu.Unlock()
			}
		case "yt-dlp":
			exe := a.detectYtDlp()
			if exe == "" {
				err = errors.New("yt-dlp lipsește")
			} else {
				path, err = a.runYtDlpDownload(ctx, exe, res.Remote.URL, dest)
			}
		case "aria2":
			if a.detectAria2() == "" {
				err = errors.New("aria2 lipsește")
			} else if resultDownloadURL(res) == "" {
				err = errors.New("URL direct lipsă")
			} else {
				path, err = runAriaRPCQueueJob(ctx, a, q, id, res, dest)
			}
		default:
			u := resultDownloadURL(res)
			if u == "" {
				err = errors.New("URL direct lipsă")
			} else {
				path, err = internalDownload(ctx, u, dest, res.Remote.Name, progress)
			}
		}
		if ctx.Err() != nil {
			return
		}
		if err == nil && path != "" {
			if _, e := os.Stat(path); e != nil {
				err = e
			} else if e = verifyDownloadedAgainstRemote(path, res.Remote); e != nil {
				err = e
			} else {
				a.markDownloaded(res.ID, path)
				st, _ := os.Stat(path)
				done := res.Remote.Size
				if st != nil {
					done = st.Size()
				}
				q.update(a, id, func(x *DownloadJob) {
					x.Status = "completed"
					x.OutputPath = path
					x.BytesDone = done
					if x.BytesTotal <= 0 {
						x.BytesTotal = done
					}
					x.SpeedBps = 0
					x.ETA = 0
					x.FinishedAt = time.Now().Unix()
					if res.Remote.Hash != "" {
						x.Verification = "checksum OK"
					} else {
						x.Verification = "download finalizat"
					}
				})
				return
			}
		}
		if err == nil && engine == "mega" {
			q.update(a, id, func(x *DownloadJob) {
				x.Status = "completed"
				x.OutputPath = path
				x.FinishedAt = time.Now().Unix()
				x.Verification = "MEGAcmd finalizat"
			})
			return
		}
		if attempts > cfgRetries {
			q.update(a, id, func(x *DownloadJob) {
				x.Status = "failed"
				x.Error = err.Error()
				x.FinishedAt = time.Now().Unix()
				x.SpeedBps = 0
				x.ETA = 0
			})
			return
		}
		q.update(a, id, func(x *DownloadJob) { x.Error = fmt.Sprintf("încercarea %d: %v", attempts, err) })
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Duration(attempts) * 2 * time.Second):
		}
	}
}

func (a *App) handleQueueAdd(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs                 []int `json:"ids"`
		Destination, Engine string
		GuardMode           string `json:"guardMode"`
		AllowReview         bool   `json:"allowReview"`
	}
	if e := decodeJSON(r, &req); e != nil {
		http.Error(w, e.Error(), 400)
		return
	}
	if len(req.IDs) == 0 {
		http.Error(w, "selecție goală", 400)
		return
	}
	a.mu.RLock()
	dest := strings.TrimSpace(req.Destination)
	if dest == "" {
		dest = a.cfg.DownloadDir
	}
	retries := a.cfg.DownloadRetries
	rows := append([]Result(nil), a.results...)
	a.mu.RUnlock()
	if dest == "" {
		dest = portableDownloadsDir()
	}
	if err := os.MkdirAll(dest, 0755); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	selectedRows := selectedResults(rows, req.IDs)
	if len(selectedRows) == 0 {
		http.Error(w, "rezultatele selectate nu mai există", 404)
		return
	}
	report, err := a.runDownloadGuard(r.Context(), selectedRows, dest, req.GuardMode)
	if err != nil {
		http.Error(w, err.Error(), 409)
		return
	}
	decisions := map[int]DownloadGuardDecision{}
	for _, decision := range report.Decisions {
		decisions[decision.ResultID] = decision
	}
	wanted := map[int]bool{}
	for _, decision := range report.Decisions {
		if decision.Verdict == guardDownload || (decision.Verdict == guardReview && req.AllowReview) {
			wanted[decision.ResultID] = true
		}
	}
	q := queueFor(a)
	added := 0
	ariaRemove := []string{}
	q.mu.Lock()
	// If the same result was left in an old queue, apply the new deterministic
	// guard verdict immediately so a later Resume cannot bypass the protection.
	for _, job := range q.Jobs {
		decision, exists := decisions[job.ResultID]
		if !exists || job.Status == "completed" || job.Status == "cancelled" || job.Status == "blocked" {
			continue
		}
		if decision.Verdict == guardDuplicate {
			if strings.EqualFold(job.Engine, "aria2") && job.GID != "" {
				ariaRemove = append(ariaRemove, job.GID)
				job.GID = ""
			}
			job.Status = "blocked"
			job.Error = "ExactGuard: duplicat exact blocat — " + decision.Reason
			job.Verification = "duplicat exact; 0 bytes descărcați"
			job.FinishedAt = time.Now().Unix()
			job.GuardVerdict, job.GuardMethod, job.GuardReason = decision.Verdict, decision.Method, decision.Reason
			job.GuardVersion, job.GuardAt = downloadGuardVersion, time.Now().Unix()
			if cancel := q.Cancels[job.ID]; cancel != nil {
				cancel()
			}
		} else if decision.Verdict == guardReview && !req.AllowReview {
			job.Status = "paused"
			job.Error = "ExactGuard REVIEW: " + decision.Reason
			job.GuardVerdict, job.GuardMethod, job.GuardReason = decision.Verdict, decision.Method, decision.Reason
			job.GuardVersion, job.GuardAt = downloadGuardVersion, time.Now().Unix()
			if cancel := q.Cancels[job.ID]; cancel != nil {
				cancel()
			}
		}
	}
	for _, res := range rows {
		if !wanted[res.ID] {
			continue
		}
		dup := false
		for _, j := range q.Jobs {
			if j.ResultID == res.ID && (j.Status == "queued" || j.Status == "running" || j.Status == "paused") {
				dup = true
				break
			}
		}
		if dup {
			continue
		}
		n := q.Seq.Add(1)
		jid := fmt.Sprintf("%d-%d", time.Now().UnixNano(), n)
		decision := decisions[res.ID]
		now := time.Now().Unix()
		q.Jobs = append(q.Jobs, &DownloadJob{ID: jid, ResultID: res.ID, Name: res.Remote.Name, Source: res.Remote.Source, URL: resultDownloadURL(res), Destination: dest, Engine: chooseQueueEngine(a, res, req.Engine), Status: "queued", Priority: 0, BytesTotal: res.Remote.Size, MaxRetries: retries, GuardMode: report.Mode, GuardVerdict: decision.Verdict, GuardReason: decision.Reason, GuardMethod: decision.Method, GuardVersion: downloadGuardVersion, GuardAt: now, GuardOverride: decision.Verdict == guardReview && req.AllowReview, AddedAt: now, UpdatedAt: now})
		added++
	}
	q.mu.Unlock()
	q.save(a)
	removeAriaQueueJobsAsync(a, ariaRemove)
	jsonOut(w, map[string]any{"ok": true, "added": added, "destination": dest, "guard": report, "reviewOverride": req.AllowReview})
}
func queueSummary(rows []DownloadJob) map[string]any {
	m := map[string]int{"queued": 0, "running": 0, "paused": 0, "completed": 0, "failed": 0, "cancelled": 0, "blocked": 0}
	var done, total int64
	for _, j := range rows {
		m[j.Status]++
		done += j.BytesDone
		if j.BytesTotal > 0 {
			total += j.BytesTotal
		}
	}
	return map[string]any{"counts": m, "bytesDone": done, "bytesTotal": total, "bytesDoneText": human(done), "bytesTotalText": human(total)}
}
func (a *App) handleQueueList(w http.ResponseWriter, r *http.Request) {
	q := queueFor(a)
	rows := q.snapshot()
	jsonOut(w, map[string]any{"jobs": rows, "summary": queueSummary(rows), "downloadDir": func() string { a.mu.RLock(); defer a.mu.RUnlock(); return a.cfg.DownloadDir }()})
}

func removeAriaQueueJobsAsync(a *App, gids []string) {
	if len(gids) == 0 {
		return
	}
	go func(gids []string) {
		time.Sleep(150 * time.Millisecond)
		m, err := ariaRPCFor(a)
		if err != nil {
			return
		}
		for _, gid := range gids {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			_ = m.remove(ctx, gid)
			cancel()
		}
	}(append([]string(nil), gids...))
}

func (a *App) handleQueueAction(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs    []string `json:"ids"`
		Action string   `json:"action"`
	}
	if e := decodeJSON(r, &req); e != nil {
		http.Error(w, e.Error(), 400)
		return
	}
	q := queueFor(a)
	set := map[string]bool{}
	ariaRemove := []string{}
	for _, id := range req.IDs {
		set[id] = true
	}
	q.mu.Lock()
	now := time.Now().Unix()
	switch strings.ToLower(req.Action) {
	case "pause", "pause-all":
		all := strings.EqualFold(req.Action, "pause-all")
		for _, j := range q.Jobs {
			if (all || set[j.ID]) && (j.Status == "running" || j.Status == "queued") {
				j.Status = "paused"
				j.UpdatedAt = now
				j.GuardVersion = 0
				if c := q.Cancels[j.ID]; c != nil {
					c()
				}
			}
		}
	case "resume", "retry":
		for _, j := range q.Jobs {
			if set[j.ID] && (j.Status == "paused" || j.Status == "failed" || j.Status == "cancelled") {
				j.Status = "queued"
				j.Error = ""
				j.FinishedAt = 0
				j.UpdatedAt = now
				j.GuardVersion = 0
			}
		}
	case "cancel", "stop-all":
		all := strings.EqualFold(req.Action, "stop-all")
		for _, j := range q.Jobs {
			if (all || set[j.ID]) && (j.Status == "running" || j.Status == "queued" || j.Status == "paused") {
				j.Status = "cancelled"
				j.UpdatedAt = now
				j.FinishedAt = now
				if strings.EqualFold(j.Engine, "aria2") && j.GID != "" {
					ariaRemove = append(ariaRemove, j.GID)
					j.GID = ""
				}
				if c := q.Cancels[j.ID]; c != nil {
					c()
				}
			}
		}
	case "remove":
		out := q.Jobs[:0]
		for _, j := range q.Jobs {
			if set[j.ID] && j.Status != "running" {
				if strings.EqualFold(j.Engine, "aria2") && j.GID != "" {
					ariaRemove = append(ariaRemove, j.GID)
				}
				continue
			}
			out = append(out, j)
		}
		q.Jobs = out
	case "clear-completed":
		out := q.Jobs[:0]
		for _, j := range q.Jobs {
			if j.Status == "completed" || j.Status == "cancelled" || j.Status == "blocked" {
				if strings.EqualFold(j.Engine, "aria2") && j.GID != "" {
					ariaRemove = append(ariaRemove, j.GID)
				}
				continue
			}
			out = append(out, j)
		}
		q.Jobs = out
	case "priority-up":
		for _, j := range q.Jobs {
			if set[j.ID] {
				j.Priority++
				j.UpdatedAt = now
			}
		}
	case "priority-down":
		for _, j := range q.Jobs {
			if set[j.ID] {
				j.Priority--
				j.UpdatedAt = now
			}
		}
	default:
		q.mu.Unlock()
		http.Error(w, "acțiune necunoscută", 400)
		return
	}
	q.mu.Unlock()
	q.save(a)
	removeAriaQueueJobsAsync(a, ariaRemove)
	jsonOut(w, map[string]any{"ok": true})
}

// ---------- Application updater ----------

type updateManifest struct {
	Version string `json:"version"`
	URL     string `json:"url"`
	SHA256  string `json:"sha256"`
	Notes   string `json:"notes"`
}

func versionParts(s string) []int {
	re := regexp.MustCompile(`\d+`)
	ms := re.FindAllString(s, -1)
	out := []int{}
	for _, m := range ms {
		n, _ := strconv.Atoi(m)
		out = append(out, n)
	}
	return out
}
func versionNewer(remote, local string) bool {
	a, b := versionParts(remote), versionParts(local)
	n := len(a)
	if len(b) > n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		x, y := 0, 0
		if i < len(a) {
			x = a[i]
		}
		if i < len(b) {
			y = b[i]
		}
		if x != y {
			return x > y
		}
	}
	return false
}
func (a *App) handleUpdateStatus(w http.ResponseWriter, r *http.Request) {
	a.mu.RLock()
	u := a.cfg.UpdateManifestURL
	a.mu.RUnlock()
	exe, _ := os.Executable()
	jsonOut(w, map[string]any{"version": appVersion, "portable": true, "root": executableDir(), "data": a.appDir, "tools": portableToolsDir(), "exe": exe, "manifestUrl": u})
}
func (a *App) handleUpdateCheck(w http.ResponseWriter, r *http.Request) {
	a.mu.RLock()
	u := strings.TrimSpace(a.cfg.UpdateManifestURL)
	a.mu.RUnlock()
	if u == "" {
		jsonOut(w, map[string]any{"configured": false, "message": "Nu există manifest online configurat. Poți aplica direct următorul EXE din interfață."})
		return
	}
	ctx, c := context.WithTimeout(r.Context(), 15*time.Second)
	defer c()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	resp, e := http.DefaultClient.Do(req)
	if e != nil {
		http.Error(w, e.Error(), 502)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		http.Error(w, fmt.Sprintf("manifest HTTP %d", resp.StatusCode), 502)
		return
	}
	var m updateManifest
	if e = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&m); e != nil {
		http.Error(w, e.Error(), 502)
		return
	}
	jsonOut(w, map[string]any{"configured": true, "manifest": m, "newer": versionNewer(m.Version, appVersion)})
}
func validPE64(path string) error {
	f, e := os.Open(path)
	if e != nil {
		return e
	}
	defer f.Close()
	head := make([]byte, 64)
	if _, e = io.ReadFull(f, head); e != nil {
		return e
	}
	if string(head[:2]) != "MZ" {
		return errors.New("fișierul nu este executabil PE")
	}
	off := int64(binary.LittleEndian.Uint32(head[0x3c:0x40]))
	if off < 64 || off > 64<<20 {
		return errors.New("header PE invalid")
	}
	sig := make([]byte, 6)
	if _, e = f.ReadAt(sig, off); e != nil {
		return e
	}
	if string(sig[:4]) != "PE\x00\x00" {
		return errors.New("semnătură PE invalidă")
	}
	machine := binary.LittleEndian.Uint16(sig[4:6])
	if machine != 0x8664 {
		return fmt.Errorf("executabilul nu este x64 (machine=0x%x)", machine)
	}
	if st, err := f.Stat(); err == nil {
		max := st.Size()
		if max > 32<<20 {
			max = 32 << 20
		}
		b := make([]byte, max)
		_, _ = f.ReadAt(b, 0)
		if !bytes.Contains(b, []byte("Duplicate Download Guard")) && !bytes.Contains(b, []byte("DuplicateDownloadGuard")) {
			return errors.New("executabilul x64 nu pare a fi o versiune Duplicate Download Guard")
		}
	}
	return nil
}
func sha256File(path string) (string, error) {
	f, e := os.Open(path)
	if e != nil {
		return "", e
	}
	defer f.Close()
	h := sha256.New()
	_, e = io.Copy(h, f)
	if e != nil {
		return "", e
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func psSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func updateHealthPath(appDir string) string {
	return filepath.Join(appDir, "updates", "health.ok")
}

func markUpdateHealthyLater(appDir string) {
	time.Sleep(2200 * time.Millisecond)
	_ = os.MkdirAll(filepath.Join(appDir, "updates"), 0755)
	_ = os.WriteFile(updateHealthPath(appDir), []byte(appVersion+"\n"+time.Now().Format(time.RFC3339)), 0644)
}

func cleanOldUpdateBackups(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	type item struct {
		path string
		mod  time.Time
	}
	var items []item
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".exe") {
			continue
		}
		if st, err := e.Info(); err == nil {
			items = append(items, item{filepath.Join(dir, e.Name()), st.ModTime()})
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].mod.After(items[j].mod) })
	for i := 3; i < len(items); i++ {
		_ = os.Remove(items[i].path)
	}
}

func (a *App) stageUpdateAndRestart(pending, version string) error {
	if runtime.GOOS != "windows" {
		return errors.New("updaterul EXE este disponibil pe Windows")
	}
	current, err := os.Executable()
	if err != nil {
		return err
	}
	updatesDir := filepath.Join(a.appDir, "updates")
	backupDir := filepath.Join(updatesDir, "backup")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return err
	}
	cleanOldUpdateBackups(backupDir)
	stamp := time.Now().Format("20060102_150405")
	backup := filepath.Join(backupDir, "DuplicateDownloadGuard_"+stamp+".exe")
	health := updateHealthPath(a.appDir)
	_ = os.Remove(health)
	if strings.EqualFold(strings.TrimSpace(version), "local") {
		// A manually supplied update has no trustworthy semantic version in a
		// manifest. The freshly recreated health file is still required.
		version = ""
	}
	pendingSHA, err := sha256File(pending)
	if err != nil {
		return err
	}
	helper := filepath.Join(updatesDir, "DuplicateDownloadGuard.Updater_"+stamp+".exe")
	if err := copyFileDurable(current, helper); err != nil {
		return fmt.Errorf("nu pot pregăti updaterul nativ: %w", err)
	}
	reqPath := filepath.Join(updatesDir, "apply_update.json")
	req := nativeUpdateRequest{
		ParentPID:       os.Getpid(),
		Current:         current,
		Pending:         pending,
		Backup:          backup,
		Health:          health,
		Log:             filepath.Join(updatesDir, "updater.log"),
		ExpectedVersion: version,
		ExpectedSHA256:  pendingSHA,
	}
	b, err := json.MarshalIndent(req, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(reqPath, b, 0600); err != nil {
		return err
	}
	cmd := exec.Command(helper, nativeUpdaterModeArg, reqPath)
	detachUpdaterProcess(cmd)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("nu pot porni updaterul nativ: %w", err)
	}
	_ = cmd.Process.Release()
	go func() { time.Sleep(900 * time.Millisecond); os.Exit(0) }()
	return nil
}

func saveUploadedFile(f multipart.File, h *multipart.FileHeader, dst string) error {
	defer f.Close()
	if h.Size > 100<<20 {
		return errors.New("update prea mare (>100 MB)")
	}
	out, e := os.Create(dst)
	if e != nil {
		return e
	}
	_, cp := io.Copy(out, io.LimitReader(f, (100<<20)+1))
	cl := out.Close()
	if cp != nil {
		return cp
	}
	return cl
}
func (a *App) handleUpdateApply(w http.ResponseWriter, r *http.Request) {
	if runtime.GOOS != "windows" {
		http.Error(w, "updaterul EXE este disponibil pe Windows", 501)
		return
	}
	if e := r.ParseMultipartForm(110 << 20); e != nil {
		http.Error(w, e.Error(), 400)
		return
	}
	f, h, e := r.FormFile("file")
	if e != nil {
		http.Error(w, "alege noul .exe", 400)
		return
	}
	pending := filepath.Join(a.appDir, "updates", "DuplicateDownloadGuard.pending.exe")
	if e = saveUploadedFile(f, h, pending); e != nil {
		http.Error(w, e.Error(), 400)
		return
	}
	if e = validPE64(pending); e != nil {
		_ = os.Remove(pending)
		http.Error(w, e.Error(), 400)
		return
	}
	sum, _ := sha256File(pending)
	if e = a.stageUpdateAndRestart(pending, "local"); e != nil {
		http.Error(w, e.Error(), 500)
		return
	}
	jsonOut(w, map[string]any{"ok": true, "sha256": sum, "message": "Update valid x64. Backup automat + health-check + rollback sunt pregătite; aplicația se va reporni."})
}

// handleAIPull installs an Ollama model through the local Ollama API.
// It is intentionally explicit because model downloads may be several GB.
func (a *App) handleAIPull(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST necesar", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Model string `json:"model"`
	}
	if e := decodeJSON(r, &req); e != nil {
		http.Error(w, e.Error(), 400)
		return
	}
	model := strings.TrimSpace(req.Model)
	if model == "" {
		http.Error(w, "scrie numele modelului Ollama", 400)
		return
	}
	a.mu.RLock()
	ep := normalizeEndpoint(a.cfg.AIEndpoint)
	a.mu.RUnlock()
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Minute)
	defer cancel()
	body, _ := json.Marshal(map[string]any{"name": model, "stream": false})
	hreq, _ := http.NewRequestWithContext(ctx, http.MethodPost, ep+"/api/pull", bytes.NewReader(body))
	hreq.Header.Set("Content-Type", "application/json")
	resp, e := http.DefaultClient.Do(hreq)
	if e != nil {
		http.Error(w, e.Error(), 502)
		return
	}
	defer resp.Body.Close()
	b, e := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if e != nil {
		http.Error(w, e.Error(), 502)
		return
	}
	if resp.StatusCode != 200 {
		http.Error(w, fmt.Sprintf("Ollama HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b))), 502)
		return
	}
	jsonOut(w, map[string]any{"ok": true, "model": model, "message": "Model instalat în Ollama."})
}

// handleUpdateInstallOnline downloads and verifies an update referenced by the
// configured manifest. SHA-256 is mandatory before any replacement is staged.
func (a *App) handleUpdateInstallOnline(w http.ResponseWriter, r *http.Request) {
	if runtime.GOOS != "windows" {
		http.Error(w, "updaterul EXE este disponibil pe Windows", 501)
		return
	}
	a.mu.RLock()
	manifestURL := strings.TrimSpace(a.cfg.UpdateManifestURL)
	a.mu.RUnlock()
	if manifestURL == "" {
		http.Error(w, "nu există manifest online configurat", 400)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()
	mreq, _ := http.NewRequestWithContext(ctx, http.MethodGet, manifestURL, nil)
	mresp, e := http.DefaultClient.Do(mreq)
	if e != nil {
		http.Error(w, e.Error(), 502)
		return
	}
	defer mresp.Body.Close()
	if mresp.StatusCode != 200 {
		http.Error(w, fmt.Sprintf("manifest HTTP %d", mresp.StatusCode), 502)
		return
	}
	var m updateManifest
	if e = json.NewDecoder(io.LimitReader(mresp.Body, 1<<20)).Decode(&m); e != nil {
		http.Error(w, e.Error(), 502)
		return
	}
	if !versionNewer(m.Version, appVersion) {
		jsonOut(w, map[string]any{"ok": true, "newer": false, "message": "Nu există versiune mai nouă."})
		return
	}
	if strings.TrimSpace(m.URL) == "" || strings.TrimSpace(m.SHA256) == "" {
		http.Error(w, "manifestul trebuie să conțină url și sha256", 400)
		return
	}
	dreq, _ := http.NewRequestWithContext(ctx, http.MethodGet, m.URL, nil)
	dresp, e := http.DefaultClient.Do(dreq)
	if e != nil {
		http.Error(w, e.Error(), 502)
		return
	}
	defer dresp.Body.Close()
	if dresp.StatusCode != 200 {
		http.Error(w, fmt.Sprintf("download update HTTP %d", dresp.StatusCode), 502)
		return
	}
	pending := filepath.Join(a.appDir, "updates", "DuplicateDownloadGuard.pending.exe")
	out, e := os.Create(pending)
	if e != nil {
		http.Error(w, e.Error(), 500)
		return
	}
	n, cp := io.Copy(out, io.LimitReader(dresp.Body, (100<<20)+1))
	cl := out.Close()
	if cp != nil || cl != nil || n > 100<<20 {
		_ = os.Remove(pending)
		if cp != nil {
			http.Error(w, cp.Error(), 502)
		} else if cl != nil {
			http.Error(w, cl.Error(), 500)
		} else {
			http.Error(w, "update prea mare (>100 MB)", 400)
		}
		return
	}
	if e = validPE64(pending); e != nil {
		_ = os.Remove(pending)
		http.Error(w, e.Error(), 400)
		return
	}
	sum, e := sha256File(pending)
	if e != nil {
		_ = os.Remove(pending)
		http.Error(w, e.Error(), 500)
		return
	}
	if !strings.EqualFold(sum, strings.TrimSpace(m.SHA256)) {
		_ = os.Remove(pending)
		http.Error(w, "SHA-256 nu corespunde manifestului; update refuzat", 400)
		return
	}
	if e = a.stageUpdateAndRestart(pending, m.Version); e != nil {
		http.Error(w, e.Error(), 500)
		return
	}
	jsonOut(w, map[string]any{"ok": true, "newer": true, "version": m.Version, "sha256": sum, "message": "Update verificat SHA-256. Updaterul nativ a pornit; aplicația se închide acum și versiunea nouă va deschide o fereastră separată."})
}
