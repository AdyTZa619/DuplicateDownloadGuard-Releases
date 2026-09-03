package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math"
	"math/bits"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// v7 adds a source-adapter layer, stronger HTTP metadata probing, optional
// yt-dlp/gallery-dl extraction, exact/full verification, perceptual media
// verification and a resumable download engine. External tools are optional.

type toolInfo struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Found   bool   `json:"found"`
	Version string `json:"version,omitempty"`
	Role    string `json:"role"`
}

func firstExisting(custom string, names []string, candidates []string) string {
	if strings.TrimSpace(custom) != "" {
		if st, err := os.Stat(custom); err == nil && !st.IsDir() {
			return custom
		}
	}
	for _, n := range names {
		if p, err := exec.LookPath(n); err == nil {
			return p
		}
	}
	for _, p := range candidates {
		if p == "" {
			continue
		}
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}
func (a *App) detectYtDlp() string {
	a.mu.RLock()
	custom := a.cfg.YtDlpPath
	a.mu.RUnlock()
	var c []string
	if runtime.GOOS == "windows" {
		c = []string{filepath.Join(portableToolsDir(), "yt-dlp", "yt-dlp.exe"), filepath.Join(portableToolsDir(), "yt-dlp.exe"), filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "yt-dlp", "yt-dlp.exe"), filepath.Join(os.Getenv("USERPROFILE"), "bin", "yt-dlp.exe"), `C:\yt-dlp\yt-dlp.exe`}
	}
	return firstExisting(custom, []string{"yt-dlp.exe", "yt-dlp"}, c)
}
func (a *App) detectGalleryDL() string {
	a.mu.RLock()
	custom := a.cfg.GalleryDLPath
	a.mu.RUnlock()
	var c []string
	if runtime.GOOS == "windows" {
		c = []string{filepath.Join(portableToolsDir(), "gallery-dl", "gallery-dl.exe"), filepath.Join(portableToolsDir(), "gallery-dl.exe"), filepath.Join(os.Getenv("USERPROFILE"), "bin", "gallery-dl.exe"), `C:\gallery-dl\gallery-dl.exe`}
	}
	return firstExisting(custom, []string{"gallery-dl.exe", "gallery-dl"}, c)
}
func (a *App) detectAria2() string {
	a.mu.RLock()
	custom := a.cfg.Aria2Path
	a.mu.RUnlock()
	var c []string
	if runtime.GOOS == "windows" {
		c = []string{filepath.Join(portableToolsDir(), "aria2", "aria2c.exe"), filepath.Join(portableToolsDir(), "aria2c.exe"), filepath.Join(os.Getenv("ProgramFiles"), "aria2", "aria2c.exe"), `C:\aria2\aria2c.exe`}
	}
	return firstExisting(custom, []string{"aria2c.exe", "aria2c"}, c)
}
func (a *App) detectFFmpeg() string {
	a.mu.RLock()
	custom, probe := a.cfg.FFmpegPath, a.cfg.FFprobePath
	a.mu.RUnlock()
	c := []string{}
	if probe != "" {
		c = append(c, filepath.Join(filepath.Dir(probe), "ffmpeg.exe"), filepath.Join(filepath.Dir(probe), "ffmpeg"))
	}
	if runtime.GOOS == "windows" {
		c = append(c, filepath.Join(portableToolsDir(), "ffmpeg", "ffmpeg.exe"), filepath.Join(portableToolsDir(), "ffmpeg", "bin", "ffmpeg.exe"), filepath.Join(os.Getenv("ProgramFiles"), "FFmpeg", "bin", "ffmpeg.exe"), `C:\ffmpeg\bin\ffmpeg.exe`)
	}
	return firstExisting(custom, []string{"ffmpeg.exe", "ffmpeg"}, c)
}
func toolVersion(path string) string {
	if path == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	args := []string{"--version"}
	base := strings.ToLower(filepath.Base(path))
	if strings.HasPrefix(base, "ffmpeg") || strings.HasPrefix(base, "ffprobe") {
		args = []string{"-version"}
	}
	cmd := exec.CommandContext(ctx, path, args...)
	hideChildWindow(cmd)
	b, err := cmd.CombinedOutput()
	if err != nil && len(b) == 0 {
		return ""
	}
	s := strings.TrimSpace(strings.ReplaceAll(string(b), "\r", ""))
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 120 {
		s = s[:120]
	}
	return s
}
func (a *App) handleTools(w http.ResponseWriter, r *http.Request) {
	tools := []toolInfo{
		{Name: "MEGAcmd", Path: a.detectMegaClient(), Role: "MEGA listare / preview / download"},
		{Name: "yt-dlp", Path: a.detectYtDlp(), Role: "video sites / metadata / download"},
		{Name: "gallery-dl", Path: a.detectGalleryDL(), Role: "galerii / imagini / URL extraction"},
		{Name: "aria2c", Path: a.detectAria2(), Role: "download HTTP segmentat + resume"},
		{Name: "ffprobe", Path: a.detectFFprobe(), Role: "metadate media"},
		{Name: "ffmpeg", Path: a.detectFFmpeg(), Role: "fingerprint vizual video"},
	}
	for i := range tools {
		tools[i].Found = tools[i].Path != ""
		if tools[i].Found {
			tools[i].Version = toolVersion(tools[i].Path)
		}
	}
	jsonOut(w, tools)
}

func headerHash(h http.Header) (typ, val string) {
	for _, k := range []string{"X-Checksum-Sha256", "X-Amz-Meta-Sha256"} {
		v := strings.TrimSpace(h.Get(k))
		if validHex(v, 64) {
			return "sha256", strings.ToLower(v)
		}
	}
	for _, k := range []string{"X-Amz-Checksum-Sha256"} {
		v := strings.TrimSpace(h.Get(k))
		if b, e := base64.StdEncoding.DecodeString(v); e == nil && len(b) == 32 {
			return "sha256", hex.EncodeToString(b)
		}
	}
	for _, k := range []string{"Digest", "Content-Digest"} {
		d := h.Get(k)
		for _, part := range strings.Split(d, ",") {
			kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
			if len(kv) != 2 {
				continue
			}
			alg := strings.ToLower(strings.TrimSpace(kv[0]))
			enc := strings.Trim(strings.TrimSpace(kv[1]), ":\"")
			if alg == "sha-256" || alg == "sha256" {
				if b, e := base64.StdEncoding.DecodeString(enc); e == nil && len(b) == 32 {
					return "sha256", hex.EncodeToString(b)
				}
			}
			if alg == "md5" {
				if b, e := base64.StdEncoding.DecodeString(enc); e == nil && len(b) == 16 {
					return "md5", hex.EncodeToString(b)
				}
			}
		}
	}
	if m := strings.TrimSpace(h.Get("Content-MD5")); m != "" {
		if b, e := base64.StdEncoding.DecodeString(m); e == nil && len(b) == 16 {
			return "md5", hex.EncodeToString(b)
		}
	}
	// Google Cloud commonly exposes md5 in X-Goog-Hash.
	for _, part := range strings.Split(h.Get("X-Goog-Hash"), ",") {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) == 2 && strings.EqualFold(kv[0], "md5") {
			if b, e := base64.StdEncoding.DecodeString(kv[1]); e == nil && len(b) == 16 {
				return "md5", hex.EncodeToString(b)
			}
		}
	}
	return "", ""
}
func parseTotalFromContentRange(v string) int64 {
	// bytes 0-0/12345
	if i := strings.LastIndex(v, "/"); i >= 0 {
		n, _ := strconv.ParseInt(strings.TrimSpace(v[i+1:]), 10, 64)
		return n
	}
	return -1
}
func headerFilename(resp *http.Response) string {
	cd := resp.Header.Get("Content-Disposition")
	if cd != "" {
		if _, p, e := mimeParseMediaType(cd); e == nil && p["filename"] != "" {
			return p["filename"]
		}
	}
	if resp.Request != nil && resp.Request.URL != nil {
		n := filepath.Base(resp.Request.URL.Path)
		if n != "" && n != "/" && n != "." {
			if q, e := url.PathUnescape(n); e == nil {
				return q
			}
			return n
		}
	}
	return "download"
}
func probeHTTPMeta(u string) (RemoteItem, error) {
	client := http.Client{Timeout: 25 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > 10 {
			return errors.New("prea multe redirectări")
		}
		return nil
	}}
	do := func(method string, rangeOne bool) (*http.Response, error) {
		req, e := http.NewRequest(method, u, nil)
		if e != nil {
			return nil, e
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 DuplicateDownloadGuard/8.0")
		if rangeOne {
			req.Header.Set("Range", "bytes=0-0")
		}
		return client.Do(req)
	}
	resp, err := do("HEAD", false)
	if err != nil || resp.StatusCode == http.StatusMethodNotAllowed || resp.StatusCode == http.StatusNotImplemented {
		if resp != nil {
			resp.Body.Close()
		}
		resp, err = do("GET", true)
	}
	if err != nil {
		return RemoteItem{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return RemoteItem{}, fmt.Errorf("server HTTP %d", resp.StatusCode)
	}
	size := resp.ContentLength
	if cr := resp.Header.Get("Content-Range"); cr != "" {
		if n := parseTotalFromContentRange(cr); n >= 0 {
			size = n
		}
	}
	ct := strings.ToLower(strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0]))
	name := headerFilename(resp)
	typ, hash := headerHash(resp.Header)
	ar := strings.Contains(strings.ToLower(resp.Header.Get("Accept-Ranges")), "bytes") || resp.StatusCode == http.StatusPartialContent
	it := RemoteItem{ID: 1, Path: name, Name: name, Size: size, Source: "HTTP", URL: u, DirectURL: resp.Request.URL.String(), HashType: typ, Hash: hash, ContentType: ct, ETag: resp.Header.Get("ETag"), AcceptRanges: ar}
	if size < 0 {
		it.Size = -1
	}
	return it, nil
}
func scanURL(u string) (RemoteItem, error) { return probeHTTPMeta(u) }

func number(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case json.Number:
		f, _ := x.Float64()
		return f
	case string:
		f, _ := strconv.ParseFloat(x, 64)
		return f
	}
	return 0
}
func strv(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
func mapv(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return nil
}
func arrv(v any) []any {
	if x, ok := v.([]any); ok {
		return x
	}
	return nil
}
func safeRemoteName(title, ext, id string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		title = id
	}
	if title == "" {
		title = "media"
	}
	repl := strings.NewReplacer("<", "_", ">", "_", ":", "_", "\"", "_", "/", "_", "\\", "_", "|", "_", "?", "_", "*", "_")
	title = repl.Replace(title)
	title = strings.Trim(title, " .")
	if ext != "" && !strings.HasSuffix(strings.ToLower(title), "."+strings.ToLower(ext)) {
		title += "." + ext
	}
	return title
}
func (a *App) probeYtDlp(ctx context.Context, u string) ([]RemoteItem, error) {
	exe := a.detectYtDlp()
	if exe == "" {
		return nil, errors.New("yt-dlp nu este instalat/configurat")
	}
	a.mu.RLock()
	cookies := strings.TrimSpace(a.cfg.YtCookiesBrowser)
	a.mu.RUnlock()
	args := []string{"-J", "--no-warnings", "--skip-download", "--no-playlist"}
	if cookies != "" {
		args = append(args, "--cookies-from-browser", cookies)
	}
	args = append(args, u)
	cmd := exec.CommandContext(ctx, exe, args...)
	hideChildWindow(cmd)
	b, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("yt-dlp: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var raw map[string]any
	if err := dec.Decode(&raw); err != nil {
		return nil, err
	}
	title, ext, id := strv(raw["title"]), strv(raw["ext"]), strv(raw["id"])
	name := safeRemoteName(title, ext, id)
	size := int64(number(raw["filesize"]))
	approx := false
	if size <= 0 {
		size = int64(number(raw["filesize_approx"]))
		approx = size > 0
	}
	if size <= 0 {
		size = -1
	}
	direct := strv(raw["url"])
	protocol := strings.ToLower(strv(raw["protocol"]))
	if rr := arrv(raw["requested_downloads"]); len(rr) > 0 {
		rm := mapv(rr[0])
		if direct == "" {
			direct = strv(rm["url"])
		}
		if protocol == "" {
			protocol = strings.ToLower(strv(rm["protocol"]))
		}
		if size < 0 {
			if n := int64(number(rm["filesize"])); n > 0 {
				size = n
			}
		}
	}
	extractor := strv(raw["extractor_key"])
	if extractor == "" {
		extractor = strv(raw["extractor"])
	}
	it := RemoteItem{ID: 1, Path: name, Name: name, Size: size, Source: "YT-DLP", URL: u, DirectURL: direct, Extractor: extractor, ProviderID: id, Duration: number(raw["duration"]), ApproxSize: approx}
	// Do not mistake an HLS/DASH manifest for the final media file size.
	isManifest := strings.Contains(protocol, "m3u8") || strings.Contains(protocol, "dash") || strings.Contains(protocol, "m3u8_native")
	if direct != "" && !isManifest {
		if hm, e := probeHTTPMeta(direct); e == nil {
			if hm.Size > 0 {
				it.Size = hm.Size
				it.ApproxSize = false
			}
			it.HashType = hm.HashType
			it.Hash = hm.Hash
			it.ContentType = hm.ContentType
			it.ETag = hm.ETag
			it.AcceptRanges = hm.AcceptRanges
		}
	} else if isManifest {
		it.ContentType = "stream/manifest"
	}
	return []RemoteItem{it}, nil
}
func (a *App) probeGalleryDL(ctx context.Context, u string) ([]RemoteItem, error) {
	exe := a.detectGalleryDL()
	if exe == "" {
		return nil, errors.New("gallery-dl nu este instalat/configurat")
	}
	cmd := exec.CommandContext(ctx, exe, "-G", "--no-download", "--no-colors", u)
	hideChildWindow(cmd)
	b, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("gallery-dl: %w", err)
	}
	seen := map[string]bool{}
	urls := []string{}
	sc := bufio.NewScanner(bytes.NewReader(b))
	buf := make([]byte, 64*1024)
	sc.Buffer(buf, 8*1024*1024)
	for sc.Scan() {
		s := strings.TrimSpace(sc.Text())
		if (strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")) && !seen[s] {
			seen[s] = true
			urls = append(urls, s)
			if len(urls) >= 10000 {
				break
			}
		}
	}
	if len(urls) == 0 {
		return nil, errors.New("gallery-dl nu a găsit fișiere media")
	}
	type pair struct {
		i  int
		it RemoteItem
	}
	jobs := make(chan int)
	out := make(chan pair, len(urls))
	workers := 8
	if len(urls) < workers {
		workers = len(urls)
	}
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				du := urls[i]
				hm, e := probeHTTPMeta(du)
				if e != nil {
					pu, _ := url.Parse(du)
					n := filepath.Base(pu.Path)
					if n == "" {
						n = fmt.Sprintf("item_%05d", i+1)
					}
					hm = RemoteItem{Name: n, Path: n, Size: -1, DirectURL: du, ContentType: ""}
				}
				hm.ID = i + 1
				hm.Source = "GALLERY-DL"
				hm.URL = u
				hm.DirectURL = du
				out <- pair{i, hm}
			}
		}()
	}
	go func() {
		for i := range urls {
			jobs <- i
		}
		close(jobs)
		wg.Wait()
		close(out)
	}()
	items := make([]RemoteItem, len(urls))
	for p := range out {
		items[p.i] = p.it
	}
	return items, nil
}
func isLikelyHTML(it RemoteItem) bool {
	return strings.HasPrefix(strings.ToLower(it.ContentType), "text/html") || strings.HasPrefix(strings.ToLower(it.ContentType), "application/xhtml")
}
func (a *App) handleUniversalScan(w http.ResponseWriter, r *http.Request) {
	var req struct{ URL, Mode, Adapter string }
	if e := decodeJSON(r, &req); e != nil {
		http.Error(w, e.Error(), 400)
		return
	}
	req.URL = strings.TrimSpace(req.URL)
	if req.URL == "" {
		http.Error(w, "URL lipsă", 400)
		return
	}
	if strings.Contains(strings.ToLower(req.URL), "mega.nz/") {
		http.Error(w, "Pentru link MEGA folosește scannerul MEGA dedicat; oferă handles și preview sigur.", 409)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()
	adapter := strings.ToLower(strings.TrimSpace(req.Adapter))
	if adapter == "" {
		adapter = "auto"
	}
	var items []RemoteItem
	var used string
	var errs []string
	if adapter == "auto" || adapter == "http" {
		if it, e := probeHTTPMeta(req.URL); e == nil && !isLikelyHTML(it) && it.Size != 0 {
			items = []RemoteItem{it}
			used = "HTTP"
		} else if e != nil {
			errs = append(errs, "HTTP: "+e.Error())
		}
	}
	if len(items) == 0 && (adapter == "auto" || adapter == "yt-dlp") {
		if x, e := a.probeYtDlp(ctx, req.URL); e == nil {
			items = x
			used = "yt-dlp"
		} else {
			errs = append(errs, e.Error())
		}
	}
	if len(items) == 0 && (adapter == "auto" || adapter == "gallery-dl") {
		if x, e := a.probeGalleryDL(ctx, req.URL); e == nil {
			items = x
			used = "gallery-dl"
		} else {
			errs = append(errs, e.Error())
		}
	}
	if len(items) == 0 {
		http.Error(w, "Nicio metodă nu a putut extrage fișiere. "+strings.Join(errs, " | "), 422)
		return
	}
	a.compareRemote(context.Background(), items, req.Mode)
	jsonOut(w, map[string]any{"ok": true, "adapter": used, "items": len(items), "errors": errs})
}

func remoteTarget(a *App, res Result) (string, error) {
	if strings.EqualFold(res.Remote.Source, "MEGA") {
		return a.startMegaPreview(res.Remote)
	}
	if strings.TrimSpace(res.Remote.DirectURL) != "" {
		return res.Remote.DirectURL, nil
	}
	if strings.TrimSpace(res.Remote.URL) != "" {
		return res.Remote.URL, nil
	}
	return "", errors.New("sursa remote nu are URL utilizabil")
}
func (a *App) updateVerification(id int, fn func(*Result)) (Result, bool) {
	a.mu.Lock()
	var out Result
	ok := false
	for i := range a.results {
		if a.results[i].ID == id {
			fn(&a.results[i])
			out = a.results[i]
			ok = true
			break
		}
	}
	a.mu.Unlock()
	if ok {
		a.revision.Add(1)
		_ = a.saveResults()
	}
	return out, ok
}
func exactHashVerify(a *App, res Result, local string) (Result, bool, error) {
	if res.Remote.Hash == "" || res.Remote.HashType == "" {
		return res, false, nil
	}
	h, e := a.ensureHash(local, res.Remote.HashType)
	if e != nil {
		return res, false, e
	}
	match := strings.EqualFold(h, res.Remote.Hash)
	rr, _ := a.updateVerification(res.ID, func(x *Result) {
		x.VerifiedBytes = res.Remote.Size
		if match {
			x.AutoStatus = "VERIFIED"
			x.AutoConfidence = "Hash " + strings.ToUpper(res.Remote.HashType) + " identic"
			x.AutoReason = "Hash-ul publicat de sursă coincide cu hash-ul fișierului local."
			x.MatchScore = 100
		} else {
			x.AutoStatus = "DIFFERENT"
			x.AutoConfidence = "Hash diferit"
			x.AutoReason = "Hash-ul remote diferă de fișierul local."
			x.MatchScore = 0
		}
		if !x.Manual {
			x.Status = x.AutoStatus
			x.Confidence = x.AutoConfidence
			x.Reason = x.AutoReason
		}
	})
	return rr, true, nil
}
func fullByteCompare(ctx context.Context, target, local string) (bool, int64, error) {
	f, e := os.Open(local)
	if e != nil {
		return false, 0, e
	}
	defer f.Close()
	req, e := http.NewRequestWithContext(ctx, "GET", target, nil)
	if e != nil {
		return false, 0, e
	}
	req.Header.Set("User-Agent", "DuplicateDownloadGuard/8.0")
	resp, e := http.DefaultClient.Do(req)
	if e != nil {
		return false, 0, e
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, 0, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	lb := make([]byte, 1024*1024)
	rb := make([]byte, 1024*1024)
	var total int64
	for {
		ln, le := f.Read(lb)
		rn, re := io.ReadFull(resp.Body, rb[:ln])
		if le != nil && le != io.EOF {
			return false, total, le
		}
		if ln == 0 {
			extra := make([]byte, 1)
			n, _ := resp.Body.Read(extra)
			return n == 0, total, nil
		}
		total += int64(rn)
		if rn != ln || !bytes.Equal(lb[:ln], rb[:rn]) {
			return false, total, nil
		}
		if re != nil && re != io.EOF && re != io.ErrUnexpectedEOF {
			return false, total, re
		}
		if le == io.EOF {
			return true, total, nil
		}
	}
}
func (a *App) performSampleVerify(ctx context.Context, res Result, local string, blocks, blockKB int) (Result, error) {
	st, e := os.Stat(local)
	if e != nil {
		return res, e
	}
	if res.Remote.Size > 0 && st.Size() != res.Remote.Size {
		return res, errors.New("mărimea diferă; byte-sampling nu poate demonstra identitatea")
	}
	target, e := remoteTarget(a, res)
	if e != nil {
		return res, e
	}
	if blocks < 3 {
		blocks = 3
	}
	if blocks > 15 {
		blocks = 15
	}
	if blockKB < 64 {
		blockKB = 64
	}
	if blockKB > 2048 {
		blockKB = 2048
	}
	ranges := sampleRanges(st.Size(), int64(blockKB)<<10, blocks)
	f, e := os.Open(local)
	if e != nil {
		return res, e
	}
	defer f.Close()
	matched := 0
	var transferred int64
	mismatch := -1
	for i, rg := range ranges {
		n := rg[1] - rg[0] + 1
		lb := make([]byte, n)
		_, e = f.ReadAt(lb, rg[0])
		if e != nil && e != io.EOF {
			return res, e
		}
		rb, e := fetchHTTPRange(ctx, target, rg[0], rg[1])
		if e != nil {
			return res, e
		}
		transferred += int64(len(rb))
		if !bytes.Equal(lb, rb) {
			mismatch = i
			break
		}
		matched++
	}
	rr, _ := a.updateVerification(res.ID, func(x *Result) {
		x.SampleMatched = matched
		x.SampleTotal = len(ranges)
		x.VerifiedBytes = transferred
		if mismatch >= 0 {
			x.AutoStatus = "DIFFERENT"
			x.AutoConfidence = "Mostră diferită"
			x.AutoReason = fmt.Sprintf("Diferență detectată la blocul %d/%d.", mismatch+1, len(ranges))
			x.MatchScore = 0
		} else {
			x.AutoStatus = "SAMPLED"
			x.AutoConfidence = fmt.Sprintf("%d/%d mostre identice", matched, len(ranges))
			x.AutoReason = fmt.Sprintf("Mostre distribuite în întreg fișierul coincid; %s transferați remote. Nu este verificare integrală.", human(transferred))
			if x.MatchScore < 99 {
				x.MatchScore = 99
			}
		}
		if !x.Manual {
			x.Status = x.AutoStatus
			x.Confidence = x.AutoConfidence
			x.Reason = x.AutoReason
		}
	})
	return rr, nil
}
func (a *App) handleFullVerify(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID   int    `json:"id"`
		Path string `json:"path"`
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
	local := req.Path
	if local == "" {
		local = res.LocalPath
	}
	if local == "" || !a.localPathAllowed(local) {
		http.Error(w, "candidat local invalid", 400)
		return
	}
	if rr, done, e := exactHashVerify(a, res, local); e != nil {
		http.Error(w, e.Error(), 500)
		return
	} else if done {
		jsonOut(w, map[string]any{"result": rr, "method": "hash", "exact": rr.AutoStatus == "VERIFIED"})
		return
	}
	target, e := remoteTarget(a, res)
	if e != nil {
		http.Error(w, e.Error(), 500)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Hour)
	defer cancel()
	same, n, e := fullByteCompare(ctx, target, local)
	if e != nil {
		http.Error(w, e.Error(), 502)
		return
	}
	rr, _ := a.updateVerification(res.ID, func(x *Result) {
		x.VerifiedBytes = n
		if same {
			x.AutoStatus = "VERIFIED"
			x.AutoConfidence = "100% byte-cu-byte"
			x.AutoReason = fmt.Sprintf("Întregul fișier remote (%s) a fost comparat cu localul.", human(n))
			x.MatchScore = 100
		} else {
			x.AutoStatus = "DIFFERENT"
			x.AutoConfidence = "Conținut diferit"
			x.AutoReason = fmt.Sprintf("Comparația integrală a detectat diferență după %s.", human(n))
			x.MatchScore = 0
		}
		if !x.Manual {
			x.Status = x.AutoStatus
			x.Confidence = x.AutoConfidence
			x.Reason = x.AutoReason
		}
	})
	jsonOut(w, map[string]any{"result": rr, "method": "full", "exact": same, "transferred": n})
}

func dhashImage(img image.Image) uint64 {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return 0
	}
	lum := func(x, y int) uint8 {
		c := img.At(b.Min.X+x, b.Min.Y+y)
		r, g, bb, _ := c.RGBA()
		return uint8((299*uint32(r>>8) + 587*uint32(g>>8) + 114*uint32(bb>>8)) / 1000)
	}
	var out uint64
	bit := uint(0)
	for y := 0; y < 8; y++ {
		sy := int((float64(y) + 0.5) * float64(h) / 8)
		if sy >= h {
			sy = h - 1
		}
		for x := 0; x < 8; x++ {
			x1 := int((float64(x) + 0.5) * float64(w) / 9)
			x2 := int((float64(x) + 1.5) * float64(w) / 9)
			if x1 >= w {
				x1 = w - 1
			}
			if x2 >= w {
				x2 = w - 1
			}
			if lum(x1, sy) > lum(x2, sy) {
				out |= 1 << bit
			}
			bit++
		}
	}
	return out
}
func fetchAllLimit(ctx context.Context, target string, max int64) ([]byte, error) {
	req, e := http.NewRequestWithContext(ctx, "GET", target, nil)
	if e != nil {
		return nil, e
	}
	req.Header.Set("User-Agent", "DuplicateDownloadGuard/8.0")
	resp, e := http.DefaultClient.Do(req)
	if e != nil {
		return nil, e
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > max {
		return nil, fmt.Errorf("fișier remote prea mare pentru verificare vizuală (%s)", human(resp.ContentLength))
	}
	b, e := io.ReadAll(io.LimitReader(resp.Body, max+1))
	if int64(len(b)) > max {
		return nil, errors.New("limita de preview vizual a fost depășită")
	}
	return b, e
}
func imageVisualScore(ctx context.Context, target, local string, max int64) (int, error) {
	rb, e := fetchAllLimit(ctx, target, max)
	if e != nil {
		return 0, e
	}
	ri, _, e := image.Decode(bytes.NewReader(rb))
	if e != nil {
		return 0, fmt.Errorf("imagine remote: %w", e)
	}
	f, e := os.Open(local)
	if e != nil {
		return 0, e
	}
	defer f.Close()
	li, _, e := image.Decode(f)
	if e != nil {
		return 0, fmt.Errorf("imagine locală: %w", e)
	}
	d := bits.OnesCount64(dhashImage(ri) ^ dhashImage(li))
	return int(math.Round(float64(64-d) * 100 / 64)), nil
}
func frameHash(ctx context.Context, ff, target string, sec float64) (uint64, error) {
	args := []string{"-v", "error", "-ss", fmt.Sprintf("%.3f", sec), "-i", target, "-frames:v", "1", "-vf", "scale=9:8:flags=fast_bilinear,format=gray", "-f", "rawvideo", "-pix_fmt", "gray", "pipe:1"}
	cmd := exec.CommandContext(ctx, ff, args...)
	hideChildWindow(cmd)
	b, e := cmd.Output()
	if e != nil {
		return 0, e
	}
	if len(b) < 72 {
		return 0, errors.New("ffmpeg nu a produs cadrul necesar")
	}
	var h uint64
	bit := uint(0)
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			if b[y*9+x] > b[y*9+x+1] {
				h |= 1 << bit
			}
			bit++
		}
	}
	return h, nil
}
func (a *App) visualVideoScore(ctx context.Context, target, local string) (int, string, error) {
	ff := a.detectFFmpeg()
	fp := a.detectFFprobe()
	if ff == "" || fp == "" {
		return 0, "", errors.New("ffmpeg + ffprobe sunt necesare pentru fingerprint video")
	}
	ri := probeMedia(ctx, fp, target, "REMOTE")
	li := probeMedia(ctx, fp, local, "LOCAL")
	if !ri.OK || !li.OK || ri.Duration <= 0 || li.Duration <= 0 {
		return 0, "", errors.New("nu am putut citi durata ambelor videoclipuri")
	}
	minD := math.Min(ri.Duration, li.Duration)
	points := []float64{.18, .5, .82}
	sum := 0
	ok := 0
	for _, p := range points {
		sec := minD * p
		rh, e := frameHash(ctx, ff, target, sec)
		if e != nil {
			continue
		}
		lh, e := frameHash(ctx, ff, local, sec)
		if e != nil {
			continue
		}
		d := bits.OnesCount64(rh ^ lh)
		sum += int(math.Round(float64(64-d) * 100 / 64))
		ok++
	}
	if ok == 0 {
		return 0, "", errors.New("nu am putut extrage cadre comparabile")
	}
	score := int(math.Round(float64(sum) / float64(ok)))
	durDelta := math.Abs(ri.Duration - li.Duration)
	note := fmt.Sprintf("%d cadre • durată Δ %.3fs", ok, durDelta)
	if durDelta > math.Max(.5, minD*.01) {
		score = int(float64(score) * .75)
	}
	return score, note, nil
}
func (a *App) handleVisualVerify(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID   int    `json:"id"`
		Path string `json:"path"`
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
	local := req.Path
	if local == "" {
		local = res.LocalPath
	}
	if local == "" || !a.localPathAllowed(local) {
		http.Error(w, "candidat local invalid", 400)
		return
	}
	target, e := remoteTarget(a, res)
	if e != nil {
		http.Error(w, e.Error(), 500)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Minute)
	defer cancel()
	kind := remoteMediaKind(res.Remote.Name)
	score := 0
	method := ""
	note := ""
	switch kind {
	case "image":
		a.mu.RLock()
		mb := a.cfg.VisualImageMaxMB
		a.mu.RUnlock()
		if mb <= 0 {
			mb = 20
		}
		score, e = imageVisualScore(ctx, target, local, int64(mb)<<20)
		method = "dHash imagine"
	case "video":
		score, note, e = a.visualVideoScore(ctx, target, local)
		method = "3 cadre dHash + durată"
	default:
		e = errors.New("verificarea vizuală este disponibilă pentru imagini și video")
	}
	if e != nil {
		http.Error(w, e.Error(), 422)
		return
	}
	rr, _ := a.updateVerification(res.ID, func(x *Result) {
		x.VisualScore = score
		x.VisualMethod = method
		if score >= 96 {
			x.AutoStatus = "POSSIBLE"
			x.AutoConfidence = fmt.Sprintf("Similaritate media %d%%", score)
			x.AutoReason = fmt.Sprintf("Fingerprint vizual %s: %d%%. %s Poate detecta aceeași imagine/video după recomprimare, dar nu dovedește identitate byte-cu-byte.", method, score, note)
			if x.MatchScore < 97 {
				x.MatchScore = 97
			}
		} else if score >= 85 {
			x.AutoStatus = "POSSIBLE"
			x.AutoConfidence = fmt.Sprintf("Similaritate media %d%%", score)
			x.AutoReason = fmt.Sprintf("Fingerprint vizual %s: %d%%. %s Necesită verificare manuală.", method, score, note)
		} else {
			x.AutoConfidence = fmt.Sprintf("Media slab similară %d%%", score)
			x.AutoReason = fmt.Sprintf("Fingerprint vizual %s: %d%%. %s", method, score, note)
		}
		if !x.Manual {
			x.Status = x.AutoStatus
			x.Confidence = x.AutoConfidence
			x.Reason = x.AutoReason
		}
	})
	jsonOut(w, map[string]any{"result": rr, "score": score, "method": method, "note": note})
}
func (a *App) handleSmartVerify(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID   int    `json:"id"`
		Path string `json:"path"`
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
	local := req.Path
	if local == "" {
		local = res.LocalPath
	}
	if local == "" || !a.localPathAllowed(local) {
		http.Error(w, "candidat local invalid", 400)
		return
	}
	if rr, done, e := exactHashVerify(a, res, local); e != nil {
		http.Error(w, e.Error(), 500)
		return
	} else if done {
		jsonOut(w, map[string]any{"result": rr, "method": "remote-hash"})
		return
	}
	st, e := os.Stat(local)
	if e != nil {
		http.Error(w, e.Error(), 404)
		return
	}
	a.mu.RLock()
	fullMB, blocks, kb := a.cfg.FullVerifyMaxMB, a.cfg.SampleBlocks, a.cfg.SampleBlockKB
	a.mu.RUnlock()
	if fullMB <= 0 {
		fullMB = 4
	}
	if res.Remote.Size > 0 && res.Remote.Size == st.Size() && st.Size() <= int64(fullMB)<<20 {
		target, e := remoteTarget(a, res)
		if e == nil {
			ctx, c := context.WithTimeout(r.Context(), 10*time.Minute)
			same, n, er := fullByteCompare(ctx, target, local)
			c()
			if er == nil {
				rr, _ := a.updateVerification(res.ID, func(x *Result) {
					x.VerifiedBytes = n
					if same {
						x.AutoStatus = "VERIFIED"
						x.AutoConfidence = "100% byte-cu-byte"
						x.AutoReason = "Smart Verify a comparat integral fișierul mic."
						x.MatchScore = 100
					} else {
						x.AutoStatus = "DIFFERENT"
						x.AutoConfidence = "Conținut diferit"
						x.AutoReason = "Smart Verify integral a găsit diferențe."
						x.MatchScore = 0
					}
					if !x.Manual {
						x.Status = x.AutoStatus
						x.Confidence = x.AutoConfidence
						x.Reason = x.AutoReason
					}
				})
				jsonOut(w, map[string]any{"result": rr, "method": "full-small"})
				return
			}
		}
	}
	if res.Remote.Size > 0 && res.Remote.Size == st.Size() {
		ctx, c := context.WithTimeout(r.Context(), 3*time.Minute)
		rr, e := a.performSampleVerify(ctx, res, local, blocks, kb)
		c()
		if e == nil {
			jsonOut(w, map[string]any{"result": rr, "method": "adaptive-samples"})
			return
		}
	}
	// If bytes cannot be compared directly (e.g. re-encoded media), fall back to perceptual verification.
	kind := remoteMediaKind(res.Remote.Name)
	if kind == "image" || kind == "video" {
		rr := httptestLikeVisual(a, res, local, r.Context())
		if rr.err == nil {
			jsonOut(w, map[string]any{"result": rr.res, "method": rr.method, "score": rr.score})
			return
		}
	}
	http.Error(w, "Smart Verify nu a găsit o metodă mai puternică pentru acest fișier. Folosește MediaInfo/preview manual.", 422)
}

type visualOutcome struct {
	res    Result
	score  int
	method string
	err    error
}

func httptestLikeVisual(a *App, res Result, local string, parent context.Context) visualOutcome {
	target, e := remoteTarget(a, res)
	if e != nil {
		return visualOutcome{err: e}
	}
	ctx, c := context.WithTimeout(parent, 4*time.Minute)
	defer c()
	kind := remoteMediaKind(res.Remote.Name)
	score := 0
	method := ""
	note := ""
	if kind == "image" {
		a.mu.RLock()
		mb := a.cfg.VisualImageMaxMB
		a.mu.RUnlock()
		if mb <= 0 {
			mb = 20
		}
		score, e = imageVisualScore(ctx, target, local, int64(mb)<<20)
		method = "dHash imagine"
	} else {
		score, note, e = a.visualVideoScore(ctx, target, local)
		method = "3 cadre dHash + durată"
	}
	if e != nil {
		return visualOutcome{err: e}
	}
	rr, _ := a.updateVerification(res.ID, func(x *Result) {
		x.VisualScore = score
		x.VisualMethod = method
		x.AutoStatus = "POSSIBLE"
		x.AutoConfidence = fmt.Sprintf("Similaritate media %d%%", score)
		x.AutoReason = fmt.Sprintf("%s: %d%%. %s Nu este identitate byte-cu-byte.", method, score, note)
		if score >= 96 && x.MatchScore < 97 {
			x.MatchScore = 97
		}
		if !x.Manual {
			x.Status = x.AutoStatus
			x.Confidence = x.AutoConfidence
			x.Reason = x.AutoReason
		}
	})
	return visualOutcome{res: rr, score: score, method: method}
}

func sanitizeFilename(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	r := strings.NewReplacer("<", "_", ">", "_", ":", "_", "\"", "_", "/", "_", "\\", "_", "|", "_", "?", "_", "*", "_")
	name = r.Replace(name)
	name = strings.Trim(name, " .")
	if name == "" {
		name = "download.bin"
	}
	return name
}
func (a *App) addDownloadedToIndex(path string) {
	st, e := os.Stat(path)
	if e != nil || st.IsDir() {
		return
	}
	e1 := FileEntry{Path: path, Name: filepath.Base(path), Size: st.Size(), MTime: st.ModTime().Unix()}
	a.mu.Lock()
	a.index[path] = e1
	a.mu.Unlock()
	a.rebuildMaps()
	_ = a.saveIndex()
}
func internalDownload(ctx context.Context, u, dest, name string, progress func(int64, int64)) (string, error) {
	if err := os.MkdirAll(dest, 0755); err != nil {
		return "", err
	}
	final := filepath.Join(dest, sanitizeFilename(name))
	part := final + ".part"
	var start int64
	if st, e := os.Stat(part); e == nil {
		start = st.Size()
	}
	req, e := http.NewRequestWithContext(ctx, "GET", u, nil)
	if e != nil {
		return "", e
	}
	req.Header.Set("User-Agent", "DuplicateDownloadGuard/8.0")
	if start > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", start))
	}
	resp, e := http.DefaultClient.Do(req)
	if e != nil {
		return "", e
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	flags := os.O_CREATE | os.O_WRONLY
	if start > 0 && resp.StatusCode == http.StatusPartialContent {
		flags |= os.O_APPEND
	} else {
		start = 0
		flags |= os.O_TRUNC
	}
	f, e := os.OpenFile(part, flags, 0644)
	if e != nil {
		return "", e
	}
	defer f.Close()
	total := resp.ContentLength
	if total >= 0 {
		total += start
	}
	buf := make([]byte, 1024*1024)
	done := start
	last := time.Now()
	for {
		n, er := resp.Body.Read(buf)
		if n > 0 {
			if _, e = f.Write(buf[:n]); e != nil {
				return "", e
			}
			done += int64(n)
			if time.Since(last) > 250*time.Millisecond {
				progress(done, total)
				last = time.Now()
			}
		}
		if er == io.EOF {
			break
		}
		if er != nil {
			return "", er
		}
	}
	if e = f.Sync(); e != nil {
		return "", e
	}
	if e = os.Rename(part, final); e != nil {
		return "", e
	}
	progress(done, total)
	return final, nil
}
func (a *App) markDownloaded(id int, path string) {
	st, _ := os.Stat(path)
	a.addDownloadedToIndex(path)
	a.mu.Lock()
	for i := range a.results {
		if a.results[i].ID == id {
			x := &a.results[i]
			x.LocalPath = path
			x.DownloadPath = path
			x.DownloadedAt = time.Now().Unix()
			x.Status = "HAVE"
			x.Manual = true
			x.ManualStatus = "HAVE"
			x.ManualAt = time.Now().Unix()
			x.Confidence = "Descărcat de program"
			x.Reason = "Fișier descărcat cu succes și adăugat în indexul local."
			if st != nil {
				x.SameSize = x.Remote.Size <= 0 || x.Remote.Size == st.Size()
			}
			key := decisionKey(x.Remote)
			a.decisions[key] = Decision{Status: "HAVE", LocalPath: path, Note: "Descărcat de v7", UpdatedAt: time.Now().Unix()}
			break
		}
	}
	a.mu.Unlock()
	a.revision.Add(1)
	_ = a.saveDecisions()
	_ = a.saveResults()
}
func resultDownloadURL(x Result) string {
	if strings.EqualFold(x.Remote.Source, "MEGA") && x.Remote.Handle != "" {
		return megaItemURL(x.Remote.URL, x.Remote.Handle)
	}
	if x.Remote.DirectURL != "" {
		return x.Remote.DirectURL
	}
	return x.Remote.URL
}
func (a *App) runAria2(ctx context.Context, exe, u, dest, name, hashType, hash string) error {
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
	args := []string{"--continue=true", "--auto-file-renaming=false", "--allow-overwrite=false", "--file-allocation=none", fmt.Sprintf("--split=%d", conn), fmt.Sprintf("--max-connection-per-server=%d", conn), "--min-split-size=1M", fmt.Sprintf("--max-tries=%d", retries+1), "--retry-wait=2", "--summary-interval=1", "--console-log-level=warn", "--dir=" + dest, "--out=" + sanitizeFilename(name)}
	if limit > 0 {
		args = append(args, fmt.Sprintf("--max-download-limit=%dK", limit))
	}
	if hash != "" && (strings.EqualFold(hashType, "sha256") || strings.EqualFold(hashType, "md5")) {
		typ := strings.ToLower(hashType)
		if typ == "sha256" {
			typ = "sha-256"
		}
		args = append(args, "--check-integrity=true", "--checksum="+typ+"="+hash)
	}
	args = append(args, u)
	cmd := exec.CommandContext(ctx, exe, args...)
	hideChildWindow(cmd)
	b, e := cmd.CombinedOutput()
	if e != nil {
		return fmt.Errorf("aria2: %v • %s", e, strings.TrimSpace(string(b)))
	}
	return nil
}
func (a *App) runYtDlpDownload(ctx context.Context, exe, u, dest string) (string, error) {
	archive := filepath.Join(a.appDir, "yt-dlp.archive.txt")
	a.mu.RLock()
	cookies, limit := strings.TrimSpace(a.cfg.YtCookiesBrowser), a.cfg.SpeedLimitKB
	a.mu.RUnlock()
	args := []string{"--no-playlist", "--continue", "--no-overwrites", "--windows-filenames", "--download-archive", archive, "-P", dest, "--print", "after_move:filepath"}
	if cookies != "" {
		args = append(args, "--cookies-from-browser", cookies)
	}
	if limit > 0 {
		args = append(args, "--limit-rate", fmt.Sprintf("%dK", limit))
	}
	args = append(args, u)
	cmd := exec.CommandContext(ctx, exe, args...)
	hideChildWindow(cmd)
	b, e := cmd.CombinedOutput()
	if e != nil {
		return "", fmt.Errorf("yt-dlp: %v • %s", e, strings.TrimSpace(string(b)))
	}
	lines := strings.Split(strings.TrimSpace(strings.ReplaceAll(string(b), "\r", "")), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		p := strings.TrimSpace(lines[i])
		if p != "" {
			if _, e := os.Stat(p); e == nil {
				return p, nil
			}
		}
	}
	return "", nil
}
func (a *App) downloadMegaResults(ctx context.Context, rows []Result, dest string) error {
	if len(rows) == 0 {
		return nil
	}
	exe := a.detectMegaClient()
	if exe == "" {
		return errors.New("MEGAcmd nu este disponibil")
	}
	groups := map[string][]Result{}
	for _, x := range rows {
		groups[x.Remote.URL] = append(groups[x.Remote.URL], x)
	}
	var old string
	if s, e := runMegaTimed(ctx, 10*time.Second, exe, "session"); e == nil {
		old = extractSession(s)
	}
	if old != "" {
		_, _ = runMegaTimed(ctx, 10*time.Second, exe, "logout", "--keep-session")
	} else {
		_, _ = runMegaTimed(ctx, 10*time.Second, exe, "logout")
	}
	defer a.restoreMegaSessionSilent(exe, old)
	for link, rs := range groups {
		if _, e := runMegaTimed(ctx, 45*time.Second, exe, "login", link); e != nil {
			return e
		}
		for _, x := range rs {
			ref := megaRemoteRef(x.Remote)
			a.updateProgress(func(p *Progress) { p.Message = "Descarc MEGA: " + x.Remote.Name; p.Detail = ref })
			out, e := runMegaTimed(ctx, 6*time.Hour, exe, "get", ref, dest)
			if e != nil {
				return fmt.Errorf("MEGA get %s: %v • %s", x.Remote.Name, e, sanitizeMega(out))
			}
			candidate := filepath.Join(dest, sanitizeFilename(x.Remote.Name))
			if _, e := os.Stat(candidate); e == nil {
				a.markDownloaded(x.ID, candidate)
			}
			a.updateProgress(func(p *Progress) { p.Current++ })
		}
		_, _ = runMegaTimed(ctx, 10*time.Second, exe, "logout")
	}
	return nil
}
func (a *App) handleDownloadStart(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs                 []int `json:"ids"`
		Method, Destination string
	}
	if e := decodeJSON(r, &req); e != nil {
		http.Error(w, e.Error(), 400)
		return
	}
	if len(req.IDs) == 0 {
		http.Error(w, "selectează fișiere", 400)
		return
	}
	if a.opRunning.Load() {
		http.Error(w, "există deja o operație în curs", 409)
		return
	}
	a.mu.RLock()
	dest := strings.TrimSpace(req.Destination)
	if dest == "" {
		dest = a.cfg.DownloadDir
	}
	method := strings.ToLower(strings.TrimSpace(req.Method))
	if method == "" {
		method = a.cfg.DownloadMethod
	}
	rows := []Result{}
	set := map[int]bool{}
	for _, id := range req.IDs {
		set[id] = true
	}
	for _, x := range a.results {
		if set[x.ID] {
			rows = append(rows, x)
		}
	}
	a.mu.RUnlock()
	if dest == "" {
		http.Error(w, "setează folderul de download", 400)
		return
	}
	if method == "" {
		method = "auto"
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.mu.Lock()
	a.cancel = cancel
	a.mu.Unlock()
	a.opRunning.Store(true)
	a.setProgress(Progress{Active: true, Phase: "download", State: "running", Message: "Pregătesc download-urile", Step: 1, StepTotal: len(rows), Total: int64(len(rows)), StartedAt: time.Now().Unix(), CanCancel: true})
	go a.runDownloadRows(ctx, rows, dest, method)
	jsonOut(w, map[string]any{"started": true, "count": len(rows), "method": method, "destination": dest})
}
func verifyDownloadedAgainstRemote(path string, remote RemoteItem) error {
	if remote.Hash == "" || remote.HashType == "" {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	var got string
	switch strings.ToLower(remote.HashType) {
	case "sha256":
		h := sha256.New()
		if _, err = io.Copy(h, f); err == nil {
			got = hex.EncodeToString(h.Sum(nil))
		}
	case "md5":
		h := md5.New()
		if _, err = io.Copy(h, f); err == nil {
			got = hex.EncodeToString(h.Sum(nil))
		}
	default:
		return nil
	}
	if err != nil {
		return err
	}
	if !strings.EqualFold(got, remote.Hash) {
		return fmt.Errorf("checksum %s diferit după download", remote.HashType)
	}
	return nil
}

func (a *App) runDownloadRows(ctx context.Context, rows []Result, dest, method string) {
	defer func() {
		a.opRunning.Store(false)
		a.mu.Lock()
		a.cancel = nil
		a.progress.Active = false
		a.progress.CanCancel = false
		a.mu.Unlock()
	}()
	if e := os.MkdirAll(dest, 0755); e != nil {
		a.failOp("Download: eroare", e.Error())
		return
	}
	mega := []Result{}
	others := []Result{}
	for _, x := range rows {
		if strings.EqualFold(x.Remote.Source, "MEGA") && (method == "auto" || method == "mega") {
			mega = append(mega, x)
		} else {
			others = append(others, x)
		}
	}
	if len(mega) > 0 {
		if e := a.downloadMegaResults(ctx, mega, dest); e != nil {
			a.failOp("Download MEGA eșuat", e.Error())
			return
		}
	}
	aria := a.detectAria2()
	yd := a.detectYtDlp()
	for _, x := range others {
		if ctx.Err() != nil {
			a.failOp("Download anulat", "Operația a fost anulată.")
			return
		}
		chosen := method
		if chosen == "auto" {
			if strings.EqualFold(x.Remote.Source, "YT-DLP") && yd != "" {
				chosen = "yt-dlp"
			} else if aria != "" {
				chosen = "aria2"
			} else {
				chosen = "internal"
			}
		}
		a.updateProgress(func(p *Progress) { p.Message = fmt.Sprintf("Descarc %s", x.Remote.Name); p.Detail = "Motor: " + chosen })
		var path string
		var e error
		switch chosen {
		case "yt-dlp":
			if yd == "" {
				e = errors.New("yt-dlp nu este instalat")
			} else {
				path, e = a.runYtDlpDownload(ctx, yd, x.Remote.URL, dest)
			}
		case "aria2":
			u := resultDownloadURL(x)
			if aria == "" {
				e = errors.New("aria2c nu este instalat")
			} else if u == "" {
				e = errors.New("URL direct lipsă")
			} else {
				e = a.runAria2(ctx, aria, u, dest, x.Remote.Name, x.Remote.HashType, x.Remote.Hash)
				path = filepath.Join(dest, sanitizeFilename(x.Remote.Name))
			}
		case "internal":
			u := resultDownloadURL(x)
			if u == "" {
				e = errors.New("URL direct lipsă")
			} else {
				path, e = internalDownload(ctx, u, dest, x.Remote.Name, func(done, total int64) {
					a.updateProgress(func(p *Progress) {
						p.Bytes = done
						if total > 0 {
							p.Detail = fmt.Sprintf("Internal • %s / %s", human(done), human(total))
						}
					})
				})
			}
		default:
			e = fmt.Errorf("metodă download necunoscută: %s", chosen)
		}
		if e != nil {
			a.logf("Download eșuat %s: %v", x.Remote.Name, e)
			a.failOp("Download eșuat: "+x.Remote.Name, e.Error())
			return
		}
		if path != "" {
			if _, statErr := os.Stat(path); statErr == nil {
				if verifyErr := verifyDownloadedAgainstRemote(path, x.Remote); verifyErr != nil {
					bad := path + ".checksum_failed"
					_ = os.Rename(path, bad)
					a.failOp("Download invalid: "+x.Remote.Name, verifyErr.Error())
					return
				}
				a.markDownloaded(x.ID, path)
			}
		}
		a.updateProgress(func(p *Progress) { p.Current++; p.Step = int(p.Current) + 1 })
	}
	a.endOp(fmt.Sprintf("Download gata ✓ • %d fișiere", len(rows)))
}
func (a *App) handleDownloadJD2(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs    []int  `json:"ids"`
		Folder string `json:"folder"`
	}
	if e := decodeJSON(r, &req); e != nil {
		http.Error(w, e.Error(), 400)
		return
	}
	a.mu.RLock()
	rows := append([]Result(nil), a.results...)
	folder := req.Folder
	if folder == "" {
		folder = a.cfg.JDFolder
	}
	a.mu.RUnlock()
	set := map[int]bool{}
	for _, id := range req.IDs {
		set[id] = true
	}
	var lines []string
	seen := map[string]bool{}
	for _, x := range rows {
		if len(set) > 0 && !set[x.ID] {
			continue
		}
		u := resultDownloadURL(x)
		if u != "" && !seen[u] {
			seen[u] = true
			lines = append(lines, u)
		}
	}
	if len(lines) == 0 {
		http.Error(w, "nu există linkuri exportabile", 400)
		return
	}
	if folder == "" {
		folder = a.appDir
	}
	if e := os.MkdirAll(folder, 0755); e != nil {
		http.Error(w, e.Error(), 500)
		return
	}
	p := filepath.Join(folder, fmt.Sprintf("DuplicateGuard_%s.crawljob", time.Now().Format("20060102_150405")))
	if e := os.WriteFile(p, []byte(strings.Join(lines, "\r\n")+"\r\n"), 0644); e != nil {
		http.Error(w, e.Error(), 500)
		return
	}
	jsonOut(w, map[string]any{"ok": true, "path": p, "count": len(lines)})
}
