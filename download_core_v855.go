package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func stableRemoteKeyV855(r RemoteItem) string {
	source := strings.ToUpper(strings.TrimSpace(r.Source))
	if source == "MEGA" && strings.TrimSpace(r.Handle) != "" {
		return "MEGA|H:" + strings.TrimSpace(r.Handle)
	}
	if strings.TrimSpace(r.ProviderID) != "" {
		return source + "|" + strings.ToLower(strings.TrimSpace(r.Extractor)) + "|ID:" + strings.TrimSpace(r.ProviderID)
	}
	page := strings.TrimSpace(r.URL)
	direct := strings.TrimSpace(r.DirectURL)
	name := strings.ToLower(strings.TrimSpace(r.Name))
	if source == "GALLERY-DL" && page != "" {
		return source + "|" + page + "|" + direct + "|" + name
	}
	if direct != "" {
		return source + "|" + direct + "|" + name
	}
	if page != "" {
		return source + "|" + page + "|" + name
	}
	if name != "" {
		return source + "|NAME:" + name + fmt.Sprintf("|SIZE:%d", r.Size)
	}
	return ""
}

func remoteSnapshotFromJobV855(j *DownloadJob) RemoteItem {
	if j == nil {
		return RemoteItem{}
	}
	if stableRemoteKeyV855(j.Remote) != "" {
		return j.Remote
	}
	return RemoteItem{
		Name:      j.Name,
		Path:      j.Name,
		Size:      j.BytesTotal,
		Source:    j.Source,
		URL:       j.URL,
		DirectURL: j.URL,
	}
}

func sameQueueRemoteV855(j *DownloadJob, res Result) bool {
	if j == nil {
		return false
	}
	a := stableRemoteKeyV855(remoteSnapshotFromJobV855(j))
	b := stableRemoteKeyV855(res.Remote)
	return a != "" && b != "" && a == b
}

func (a *App) downloadResultForJobV855(j *DownloadJob) Result {
	if j == nil {
		return Result{}
	}
	snapshot := remoteSnapshotFromJobV855(j)
	fallback := Result{ID: j.ResultID, Remote: snapshot, MediaKind: remoteMediaKind(snapshot.Name)}
	if live, ok := a.resultByID(j.ResultID); ok && sameQueueRemoteV855(j, live) {
		return live
	}
	return fallback
}

func isManifestRemoteV855(r RemoteItem) bool {
	ct := strings.ToLower(strings.TrimSpace(r.ContentType))
	if strings.Contains(ct, "stream/manifest") || strings.Contains(ct, "mpegurl") || strings.Contains(ct, "dash") {
		return true
	}
	u := strings.ToLower(strings.TrimSpace(r.DirectURL))
	if u == "" {
		u = strings.ToLower(strings.TrimSpace(r.URL))
	}
	if q := strings.IndexByte(u, '?'); q >= 0 {
		u = u[:q]
	}
	return strings.HasSuffix(u, ".m3u8") || strings.HasSuffix(u, ".mpd")
}

func chooseQueueEngineV855(res Result, requested string) string {
	e := strings.ToLower(strings.TrimSpace(requested))
	if e != "" && e != "auto" {
		return e
	}
	if strings.EqualFold(res.Remote.Source, "MEGA") {
		return "mega"
	}
	if strings.EqualFold(res.Remote.Source, "YT-DLP") || isManifestRemoteV855(res.Remote) {
		return "yt-dlp"
	}
	// Direct HTTP and gallery-dl results use the built-in downloader by default.
	// aria2 remains an explicit performance option; merely having it installed
	// must not silently change semantics or headers.
	return "internal"
}

func (a *App) validateDownloadEngineV855(res Result, engine string) error {
	engine = strings.ToLower(strings.TrimSpace(engine))
	switch engine {
	case "mega":
		if !strings.EqualFold(res.Remote.Source, "MEGA") {
			return errors.New("motorul MEGA poate descărca numai rezultate MEGA")
		}
		if a.detectMegaClient() == "" {
			return errors.New("MEGAcmd lipsește; instalează MEGAcmd înainte de download")
		}
	case "yt-dlp":
		if a.detectYtDlp() == "" {
			return errors.New("yt-dlp lipsește; instalează-l din AI & Tool Manager")
		}
	case "aria2":
		if isManifestRemoteV855(res.Remote) {
			return errors.New("HLS/DASH nu poate fi descărcat ca fișier direct cu aria2; folosește Auto/yt-dlp")
		}
		if a.detectAria2() == "" {
			return errors.New("aria2 lipsește; instalează-l din AI & Tool Manager sau folosește Auto")
		}
		if resultDownloadURL(res) == "" {
			return errors.New("URL direct lipsă pentru aria2")
		}
	case "internal":
		if isManifestRemoteV855(res.Remote) {
			return errors.New("HLS/DASH necesită yt-dlp; downloaderul intern nu salvează manifestul ca videoclip")
		}
		if resultDownloadURL(res) == "" {
			return errors.New("sursa nu are URL direct utilizabil")
		}
	default:
		return fmt.Errorf("motor de download necunoscut: %s", engine)
	}
	return nil
}

func downloadRefererV855(res Result) string {
	page := strings.TrimSpace(res.Remote.URL)
	direct := strings.TrimSpace(res.Remote.DirectURL)
	if page == "" || direct == "" || page == direct {
		return ""
	}
	pu, err := url.Parse(page)
	if err != nil || (pu.Scheme != "http" && pu.Scheme != "https") {
		return ""
	}
	return page
}

func sourcePartPathV855(dest, name, sourceURL string) string {
	h := sha256.Sum256([]byte(strings.TrimSpace(sourceURL)))
	tag := hex.EncodeToString(h[:4])
	return filepath.Join(dest, sanitizeFilename(name)+"."+tag+".part")
}

func collisionFreeFinalV855(dest, name string) string {
	clean := sanitizeFilename(name)
	base := strings.TrimSuffix(clean, filepath.Ext(clean))
	ext := filepath.Ext(clean)
	for i := 0; i < 10000; i++ {
		candidate := filepath.Join(dest, clean)
		if i > 0 {
			candidate = filepath.Join(dest, fmt.Sprintf("%s (%d)%s", base, i, ext))
		}
		if _, err := os.Stat(candidate); errors.Is(err, os.ErrNotExist) {
			return candidate
		}
	}
	return filepath.Join(dest, fmt.Sprintf("%s-%d%s", base, time.Now().UnixNano(), ext))
}

func internalDownloadV855(ctx context.Context, res Result, dest string, progress func(int64, int64)) (string, error) {
	u := resultDownloadURL(res)
	if strings.TrimSpace(u) == "" {
		return "", errors.New("URL direct lipsă")
	}
	if err := os.MkdirAll(dest, 0755); err != nil {
		return "", err
	}
	part := sourcePartPathV855(dest, res.Remote.Name, u)
	var start int64
	if st, err := os.Stat(part); err == nil && !st.IsDir() {
		start = st.Size()
	}

	request := func(offset int64) (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 DuplicateDownloadGuard/8.5.5")
		req.Header.Set("Accept", "*/*")
		if referer := downloadRefererV855(res); referer != "" {
			req.Header.Set("Referer", referer)
		}
		if offset > 0 {
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
		}
		return http.DefaultClient.Do(req)
	}

	resp, err := request(start)
	if err != nil {
		return "", err
	}
	if start > 0 && resp.StatusCode == http.StatusRequestedRangeNotSatisfiable {
		resp.Body.Close()
		start = 0
		resp, err = request(0)
		if err != nil {
			return "", err
		}
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		if resp.StatusCode == http.StatusForbidden && downloadRefererV855(res) != "" {
			return "", errors.New("HTTP 403: serverul a refuzat downloadul chiar și cu Referer-ul paginii sursă; pot fi necesare cookies/autentificare")
		}
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	flags := os.O_CREATE | os.O_WRONLY
	if start > 0 && resp.StatusCode == http.StatusPartialContent {
		flags |= os.O_APPEND
	} else {
		start = 0
		flags |= os.O_TRUNC
	}
	f, err := os.OpenFile(part, flags, 0644)
	if err != nil {
		return "", err
	}
	closed := false
	defer func() {
		if !closed {
			_ = f.Close()
		}
	}()

	total := resp.ContentLength
	if total >= 0 {
		total += start
	}
	done := start
	buf := make([]byte, 1024*1024)
	last := time.Now()
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, err = f.Write(buf[:n]); err != nil {
				return "", err
			}
			done += int64(n)
			if time.Since(last) >= 200*time.Millisecond {
				progress(done, total)
				last = time.Now()
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return "", readErr
		}
	}
	if err = f.Sync(); err != nil {
		return "", err
	}
	if total >= 0 && done != total {
		return "", fmt.Errorf("download incomplet: %d / %d bytes", done, total)
	}
	if err = f.Close(); err != nil {
		return "", err
	}
	closed = true

	for tries := 0; tries < 10000; tries++ {
		final := collisionFreeFinalV855(dest, res.Remote.Name)
		if err = os.Rename(part, final); err == nil {
			progress(done, total)
			return final, nil
		}
		if !os.IsExist(err) {
			// Windows may report a generic access error when another concurrent
			// job won the final filename race. Retry with another collision name
			// only if the candidate appeared meanwhile.
			if _, statErr := os.Stat(final); statErr != nil {
				return "", err
			}
		}
	}
	return "", errors.New("nu am putut rezerva un nume final unic pentru download")
}

func (a *App) markDownloadedResultV855(res Result, path string) {
	if live, ok := a.resultByID(res.ID); ok {
		fake := &DownloadJob{Remote: res.Remote}
		if sameQueueRemoteV855(fake, live) {
			a.markDownloaded(res.ID, path)
			return
		}
	}
	// The results table may have been replaced while this job was running. Do
	// not mark an unrelated row that reused the same ResultID; update only the
	// durable local index. Queue history is persisted from the completed job.
	a.addDownloadedToIndex(path)
}

func classifyDownloadErrorV855(engine string, err error) (code, title, action string) {
	if err == nil {
		return "", "", ""
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "yt-dlp lipsește"):
		return "TOOL_MISSING", "yt-dlp lipsește", "Instalează yt-dlp din AI & Tool Manager și apasă Reîncearcă."
	case strings.Contains(msg, "aria2 lipsește"):
		return "TOOL_MISSING", "aria2 lipsește", "Instalează aria2 sau schimbă motorul pe Auto."
	case strings.Contains(msg, "megacmd") && strings.Contains(msg, "lipsește"):
		return "TOOL_MISSING", "MEGAcmd lipsește", "Instalează MEGAcmd și apasă Reîncearcă."
	case strings.Contains(msg, "hls/dash"):
		return "ENGINE_INCOMPATIBLE", "Motor incompatibil", "Folosește Auto sau yt-dlp pentru streamuri HLS/DASH."
	case strings.Contains(msg, "http 403"):
		return "HTTP_403", "Acces refuzat de site", "Sursa poate necesita cookies/autentificare. Pentru pagini video încearcă yt-dlp cu cookies din browser."
	case strings.Contains(msg, "http 404"):
		return "HTTP_404", "Fișier indisponibil", "Rescanează sursa; URL-ul direct poate fi expirat."
	case strings.Contains(msg, "url direct lipsă") || strings.Contains(msg, "url direct utilizabil"):
		return "SOURCE_URL_MISSING", "URL de download lipsă", "Rescanează sursa sau folosește motorul specific site-ului."
	default:
		return "DOWNLOAD_ERROR", "Download eșuat", fmt.Sprintf("Motor: %s. Verifică mesajul tehnic și apasă Reîncearcă după corectarea cauzei.", engine)
	}
}
