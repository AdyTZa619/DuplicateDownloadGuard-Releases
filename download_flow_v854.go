package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type downloadCapsV854 struct {
	Mega  bool
	YtDlp bool
	Aria2 bool
}

type downloadPlanV854 struct {
	Engine string `json:"engine"`
	Reason string `json:"reason"`
}

type downloadRejectionV854 struct {
	ResultID int    `json:"resultId"`
	Name     string `json:"name"`
	Reason   string `json:"reason"`
}

func remoteDirectURLV854(r RemoteItem) string {
	if strings.TrimSpace(r.DirectURL) != "" {
		return strings.TrimSpace(r.DirectURL)
	}
	if !strings.EqualFold(r.Source, "MEGA") {
		return strings.TrimSpace(r.URL)
	}
	return ""
}

func ytDlpInputV854(r RemoteItem) string {
	if strings.TrimSpace(r.URL) != "" {
		return strings.TrimSpace(r.URL)
	}
	return remoteDirectURLV854(r)
}

func remoteManifestV854(r RemoteItem) bool {
	ct := strings.ToLower(strings.TrimSpace(r.ContentType))
	if strings.Contains(ct, "stream/manifest") || strings.Contains(ct, "mpegurl") || strings.Contains(ct, "dash") {
		return true
	}
	for _, raw := range []string{r.DirectURL, r.URL, r.Name} {
		u := strings.ToLower(raw)
		if strings.Contains(u, ".m3u8") || strings.Contains(u, ".mpd") {
			return true
		}
	}
	return false
}

func downloadRefererV854(r RemoteItem) string {
	source := strings.ToUpper(strings.TrimSpace(r.Source))
	if source != "GALLERY-DL" && source != "HTML" && source != "WEB" && source != "WEB-MEDIA" {
		return ""
	}
	page := strings.TrimSpace(r.URL)
	direct := strings.TrimSpace(r.DirectURL)
	if page == "" || page == direct {
		return ""
	}
	if u, err := url.Parse(page); err == nil && (u.Scheme == "http" || u.Scheme == "https") {
		return page
	}
	return ""
}

func chooseDownloadPlanCoreV854(r RemoteItem, requested string, caps downloadCapsV854) (downloadPlanV854, error) {
	req := strings.ToLower(strings.TrimSpace(requested))
	if req == "" {
		req = "auto"
	}
	source := strings.ToUpper(strings.TrimSpace(r.Source))
	direct := remoteDirectURLV854(r)
	manifest := remoteManifestV854(r)

	validateExplicit := func(engine string) (downloadPlanV854, error) {
		switch engine {
		case "mega":
			if source != "MEGA" {
				return downloadPlanV854{}, errors.New("MEGAcmd poate fi folosit aici numai pentru rezultate MEGA")
			}
			if !caps.Mega {
				return downloadPlanV854{}, errors.New("MEGAcmd nu este disponibil")
			}
			return downloadPlanV854{Engine: "mega", Reason: "sursă MEGA → MEGAcmd"}, nil
		case "yt-dlp":
			if !caps.YtDlp {
				return downloadPlanV854{}, errors.New("yt-dlp nu este instalat")
			}
			if ytDlpInputV854(r) == "" {
				return downloadPlanV854{}, errors.New("yt-dlp nu are un URL sursă utilizabil")
			}
			return downloadPlanV854{Engine: "yt-dlp", Reason: "yt-dlp selectat explicit"}, nil
		case "aria2":
			if manifest {
				return downloadPlanV854{}, errors.New("HLS/DASH nu poate fi descărcat ca fișier simplu cu aria2; folosește yt-dlp")
			}
			if !caps.Aria2 {
				return downloadPlanV854{}, errors.New("aria2c nu este instalat")
			}
			if direct == "" {
				return downloadPlanV854{}, errors.New("aria2 nu are URL direct")
			}
			return downloadPlanV854{Engine: "aria2", Reason: "aria2 selectat explicit"}, nil
		case "internal":
			if manifest {
				return downloadPlanV854{}, errors.New("HLS/DASH necesită yt-dlp; downloaderul HTTP intern ar salva doar manifestul")
			}
			if direct == "" {
				return downloadPlanV854{}, errors.New("downloaderul intern nu are URL direct")
			}
			return downloadPlanV854{Engine: "internal", Reason: "HTTP intern selectat explicit"}, nil
		default:
			return downloadPlanV854{}, fmt.Errorf("motor necunoscut: %s", engine)
		}
	}

	if req != "auto" {
		return validateExplicit(req)
	}
	if source == "MEGA" {
		if !caps.Mega {
			return downloadPlanV854{}, errors.New("MEGAcmd lipsește; rezultatele MEGA nu pot fi descărcate")
		}
		return downloadPlanV854{Engine: "mega", Reason: "Auto: MEGA → MEGAcmd"}, nil
	}
	if source == "YT-DLP" {
		if caps.YtDlp {
			return downloadPlanV854{Engine: "yt-dlp", Reason: "Auto: pagină video → yt-dlp"}, nil
		}
		if manifest {
			return downloadPlanV854{}, errors.New("yt-dlp este necesar pentru acest stream HLS/DASH")
		}
		if direct != "" {
			return downloadPlanV854{Engine: "internal", Reason: "Auto: yt-dlp lipsește, folosesc URL-ul media direct"}, nil
		}
		return downloadPlanV854{}, errors.New("yt-dlp lipsește și nu există URL media direct de rezervă")
	}
	if manifest {
		if caps.YtDlp {
			return downloadPlanV854{Engine: "yt-dlp", Reason: "Auto: stream HLS/DASH → yt-dlp"}, nil
		}
		return downloadPlanV854{}, errors.New("stream HLS/DASH detectat, dar yt-dlp nu este instalat")
	}
	if direct == "" {
		return downloadPlanV854{}, errors.New("sursa nu oferă un URL direct descărcabil")
	}
	if source == "GALLERY-DL" {
		return downloadPlanV854{Engine: "internal", Reason: "Auto: media extrasă din galerie → HTTP intern + Referer"}, nil
	}
	return downloadPlanV854{Engine: "internal", Reason: "Auto: URL direct → HTTP intern cu resume"}, nil
}

func chooseDownloadPlanV854(a *App, res Result, requested string) (downloadPlanV854, error) {
	caps := downloadCapsV854{
		Mega:  a.detectMegaClient() != "",
		YtDlp: a.detectYtDlp() != "",
		Aria2: a.detectAria2() != "",
	}
	return chooseDownloadPlanCoreV854(res.Remote, requested, caps)
}

func resultFromDownloadJobV854(job DownloadJob, live *Result) (Result, error) {
	if strings.TrimSpace(job.Remote.Name) != "" || strings.TrimSpace(job.Remote.Source) != "" {
		return Result{ID: job.ResultID, Remote: job.Remote}, nil
	}
	if live != nil {
		return *live, nil
	}
	// Legacy queue entries stored only the final URL. That is enough for a
	// plain HTTP file, but not for MEGA/yt-dlp/gallery-dl where handles/page
	// identity and other metadata are required for a correct retry.
	if strings.EqualFold(job.Source, "HTTP") && strings.TrimSpace(job.URL) != "" {
		r := RemoteItem{ID: job.ResultID, Name: job.Name, Path: job.Name, Size: job.BytesTotal, Source: "HTTP", URL: job.URL, DirectURL: job.URL}
		return Result{ID: job.ResultID, Remote: r}, nil
	}
	return Result{}, errors.New("job vechi fără snapshot-ul sursei; selectează din nou fișierul și adaugă-l în coadă")
}

func browserUserAgentV854() string {
	return "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0 Safari/537.36"
}

func uniqueDownloadPathV854(dest, name string) (string, string) {
	name = sanitizeFilename(name)
	base := strings.TrimSuffix(name, filepath.Ext(name))
	ext := filepath.Ext(name)
	for i := 0; i < 10000; i++ {
		candidate := name
		if i > 0 {
			candidate = fmt.Sprintf("%s (%d)%s", base, i, ext)
		}
		final := filepath.Join(dest, candidate)
		part := final + ".part"
		if _, err := os.Stat(final); err == nil {
			continue
		}
		// Reuse an existing .part for this candidate so retries resume instead
		// of inventing a new filename on every attempt.
		return final, part
	}
	final := filepath.Join(dest, fmt.Sprintf("download-%d%s", time.Now().UnixNano(), ext))
	return final, final + ".part"
}

func internalDownloadV854(ctx context.Context, remote RemoteItem, dest, name string, progress func(int64, int64)) (string, error) {
	u := remoteDirectURLV854(remote)
	if u == "" {
		return "", errors.New("URL direct lipsă")
	}
	if remoteManifestV854(remote) {
		return "", errors.New("stream HLS/DASH detectat; folosește yt-dlp")
	}
	if err := os.MkdirAll(dest, 0755); err != nil {
		return "", err
	}
	final, part := uniqueDownloadPathV854(dest, name)
	var start int64
	if st, err := os.Stat(part); err == nil && !st.IsDir() {
		start = st.Size()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", browserUserAgentV854())
	req.Header.Set("Accept", "*/*")
	if ref := downloadRefererV854(remote); ref != "" {
		req.Header.Set("Referer", ref)
	}
	if start > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", start))
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusRequestedRangeNotSatisfiable && start > 0 {
		cr := resp.Header.Get("Content-Range")
		if slash := strings.LastIndex(cr, "/"); slash >= 0 {
			if total, parseErr := strconv.ParseInt(strings.TrimSpace(cr[slash+1:]), 10, 64); parseErr == nil && total == start {
				if err = os.Rename(part, final); err == nil {
					progress(start, start)
					return final, nil
				}
			}
		}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
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
	buf := make([]byte, 1024*1024)
	done := start
	last := time.Now()
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, err = f.Write(buf[:n]); err != nil {
				return "", err
			}
			done += int64(n)
			if time.Since(last) >= 250*time.Millisecond {
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
	if err = os.Rename(part, final); err != nil {
		return "", err
	}
	progress(done, total)
	return final, nil
}
