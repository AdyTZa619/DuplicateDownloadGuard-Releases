package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const detectorRemoteBudgetV90 int64 = 16 << 20
const detectorRemoteBlockV90 int64 = 256 << 10

type detectorRemoteKeyV90 struct{}
type detectorRemoteV90 struct {
	mu                                    sync.Mutex
	URL, Original, Validator, ContentType string
	Size, Bytes                           int64
	Blocks                                map[int64][]byte
	Err                                   error
	CacheHit                              bool
	Cancel                                context.CancelFunc
	Context                               context.Context
	Server                                *http.Server
}

// A private loopback reader bounds ALL ffprobe/FFmpeg requests for one analysis.
// Blocks are reused across frame seeks. A server ignoring Range is refused
// before reading its body; validators prevent mixing different remote versions.
func prepareDetectorRemoteV90(parent context.Context, target string) (context.Context, string, func(), error) {
	if s, ok := parent.Value(detectorRemoteKeyV90{}).(*detectorRemoteV90); ok && (target == s.URL || target == s.Original) {
		return parent, s.URL, func() {}, nil
	}
	u, err := url.Parse(target)
	if err != nil {
		return parent, "", func() {}, err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return parent, target, func() {}, nil
	}
	ctx, cancel := context.WithCancel(parent)
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, target, nil)
	if err != nil {
		cancel()
		return parent, "", func() {}, err
	}
	req.Header.Set("Accept-Encoding", "identity")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		return parent, "", func() {}, err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.ContentLength <= 0 {
		cancel()
		return parent, "", func() {}, errors.New("remote size unavailable; additional verification required")
	}
	s := &detectorRemoteV90{Original: target, Size: resp.ContentLength, ContentType: resp.Header.Get("Content-Type"),
		Blocks: map[int64][]byte{}, Context: ctx, Cancel: cancel}
	if etag := resp.Header.Get("ETag"); etag != "" && !strings.HasPrefix(etag, "W/") {
		s.Validator = etag
	} else {
		s.Validator = resp.Header.Get("Last-Modified")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		cancel()
		return parent, "", func() {}, err
	}
	token := sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%p", target, time.Now().UnixNano(), s)))
	path := "/" + hex.EncodeToString(token[:16]) + "/media"
	s.URL = "http://" + listener.Addr().String() + path
	s.Server = &http.Server{ReadHeaderTimeout: 5 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path || (r.Method != http.MethodGet && r.Method != http.MethodHead) {
			http.NotFound(w, r)
			return
		}
		if s.ContentType != "" {
			w.Header().Set("Content-Type", s.ContentType)
		}
		http.ServeContent(w, r, "media", time.Time{}, &detectorRemoteReaderV90{s: s, ctx: r.Context()})
	})}
	go func() { _ = s.Server.Serve(listener) }()
	close := func() { cancel(); _ = s.Server.Close() }
	return context.WithValue(ctx, detectorRemoteKeyV90{}, s), s.URL, close, nil
}

type detectorRemoteReaderV90 struct {
	s   *detectorRemoteV90
	ctx context.Context
	pos int64
}

func (r *detectorRemoteReaderV90) Seek(offset int64, whence int) (int64, error) {
	var next int64
	switch whence {
	case io.SeekStart:
		next = offset
	case io.SeekCurrent:
		next = r.pos + offset
	case io.SeekEnd:
		next = r.s.Size + offset
	default:
		return 0, errors.New("invalid seek")
	}
	if next < 0 {
		return 0, errors.New("negative seek")
	}
	r.pos = next
	return next, nil
}
func (r *detectorRemoteReaderV90) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	if r.pos >= r.s.Size {
		return 0, io.EOF
	}
	block, err := r.s.block(r.ctx, r.pos/detectorRemoteBlockV90*detectorRemoteBlockV90)
	if err != nil {
		return 0, err
	}
	offset := r.pos % detectorRemoteBlockV90
	n := copy(p, block[offset:])
	r.pos += int64(n)
	return n, nil
}
func (s *detectorRemoteV90) block(ctx context.Context, start int64) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.Context.Err(); err != nil {
		return nil, err
	}
	if b, ok := s.Blocks[start]; ok {
		return b, nil
	}
	if s.Err != nil {
		return nil, s.Err
	}
	end := min(s.Size-1, start+detectorRemoteBlockV90-1)
	if s.Bytes+(end-start+1) > detectorRemoteBudgetV90 {
		s.Err = errors.New("16 MiB analysis traffic limit reached; additional verification required")
		return nil, s.Err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.Original, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))
	req.Header.Set("Accept-Encoding", "identity")
	if s.Validator != "" {
		req.Header.Set("If-Range", s.Validator)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	wanted := fmt.Sprintf("bytes %d-%d/%d", start, end, s.Size)
	if resp.StatusCode != http.StatusPartialContent || resp.Header.Get("Content-Range") != wanted || resp.Header.Get("Content-Encoding") != "" {
		s.Err = errors.New("Range ignored, invalid, or remote changed; full download refused")
		return nil, s.Err
	}
	if s.Validator != "" {
		current := resp.Header.Get("Last-Modified")
		if strings.HasPrefix(s.Validator, "\"") {
			current = resp.Header.Get("ETag")
		}
		if current != s.Validator {
			s.Err = errors.New("remote validator changed during analysis")
			return nil, s.Err
		}
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, end-start+2))
	s.Bytes += int64(len(b))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) != end-start+1 {
		s.Err = io.ErrUnexpectedEOF
		return nil, s.Err
	}
	s.Blocks[start] = b
	return b, nil
}

func remoteFingerprintPathV90(a *App, s *detectorRemoteV90) string {
	key := sha256.Sum256([]byte(s.Original + "\n" + s.Validator + "\n" + strconv.FormatInt(s.Size, 10)))
	return filepath.Join(a.appDir, "remote_fingerprints_v90", hex.EncodeToString(key[:])+".json")
}
func cachedRemoteFingerprintV90(a *App, ctx context.Context) (videoFingerprintV85, bool) {
	s, ok := ctx.Value(detectorRemoteKeyV90{}).(*detectorRemoteV90)
	if !ok || s.Validator == "" {
		return videoFingerprintV85{}, false
	}
	path := remoteFingerprintPathV90(a, s)
	st, err := os.Stat(path)
	if err != nil || time.Since(st.ModTime()) > 7*24*time.Hour {
		return videoFingerprintV85{}, false
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return videoFingerprintV85{}, false
	}
	var fp videoFingerprintV85
	if json.Unmarshal(b, &fp) != nil || !richVideoFingerprintUsableV90(fp) {
		return videoFingerprintV85{}, false
	}
	s.CacheHit = true
	return fp, true
}
func saveRemoteFingerprintV90(a *App, ctx context.Context, fp videoFingerprintV85) {
	s, ok := ctx.Value(detectorRemoteKeyV90{}).(*detectorRemoteV90)
	if !ok || s.Validator == "" || !richVideoFingerprintUsableV90(fp) {
		return
	}
	path := remoteFingerprintPathV90(a, s)
	if os.MkdirAll(filepath.Dir(path), 0755) != nil {
		return
	}
	b, err := json.Marshal(fp)
	if err != nil {
		return
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "fp-*.tmp")
	if err != nil {
		return
	}
	name := tmp.Name()
	defer os.Remove(name)
	_, err = tmp.Write(b)
	closeErr := tmp.Close()
	if err == nil && closeErr == nil {
		_ = replaceCacheFileV85(name, path)
	}
}

func richVideoFingerprintUsableV90(fp videoFingerprintV85) bool {
	if !videoFingerprintUsableV85(fp) || len(fp.Frames) != len(v85FramePoints) {
		return false
	}
	for i, valid := range fp.Valid {
		if valid && fp.Frames[i].Version != signatureVersionV90 {
			return false
		}
	}
	return true
}
