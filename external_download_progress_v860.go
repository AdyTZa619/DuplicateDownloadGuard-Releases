package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type ytDlpProgressV860 struct {
	Done  int64
	Total int64
	Speed int64
	ETA   int64
}

func parseYtDlpNumberV860(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" || strings.EqualFold(s, "NA") || strings.EqualFold(s, "none") {
		return 0
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return n
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return int64(f)
	}
	return 0
}

func parseYtDlpProgressLineV860(line string) (ytDlpProgressV860, bool) {
	line = strings.TrimSpace(line)
	const prefix = "DDG_PROGRESS|"
	if !strings.HasPrefix(line, prefix) {
		return ytDlpProgressV860{}, false
	}
	parts := strings.Split(strings.TrimPrefix(line, prefix), "|")
	if len(parts) < 5 {
		return ytDlpProgressV860{}, false
	}
	done := parseYtDlpNumberV860(parts[0])
	total := parseYtDlpNumberV860(parts[1])
	if total <= 0 {
		total = parseYtDlpNumberV860(parts[2])
	}
	return ytDlpProgressV860{Done: done, Total: total, Speed: parseYtDlpNumberV860(parts[3]), ETA: parseYtDlpNumberV860(parts[4])}, true
}

func (a *App) runYtDlpDownloadProgressV860(ctx context.Context, exe, u, dest string, progress func(int64, int64)) (string, error) {
	if strings.TrimSpace(exe) == "" {
		return "", errors.New("yt-dlp lipsește")
	}
	a.mu.RLock()
	cookies, limit := strings.TrimSpace(a.cfg.YtCookiesBrowser), a.cfg.SpeedLimitKB
	a.mu.RUnlock()
	args := []string{
		"--no-playlist", "--continue", "--no-overwrites", "--windows-filenames", "--newline", "--progress",
		"-P", dest,
		"--progress-template", "download:DDG_PROGRESS|%(progress.downloaded_bytes)s|%(progress.total_bytes)s|%(progress.total_bytes_estimate)s|%(progress.speed)s|%(progress.eta)s",
		"--print", "after_move:DDG_PATH|%(filepath)s",
	}
	if cookies != "" {
		args = append(args, "--cookies-from-browser", cookies)
	}
	if limit > 0 {
		args = append(args, "--limit-rate", fmt.Sprintf("%dK", limit))
	}
	args = append(args, u)

	cmd := exec.CommandContext(ctx, exe, args...)
	hideChildWindow(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", err
	}
	if err := cmd.Start(); err != nil {
		return "", err
	}

	lines := make(chan string, 64)
	var wg sync.WaitGroup
	scan := func(r *bufio.Scanner) {
		defer wg.Done()
		r.Buffer(make([]byte, 64*1024), 2*1024*1024)
		for r.Scan() {
			lines <- r.Text()
		}
	}
	outScanner := bufio.NewScanner(stdout)
	errScanner := bufio.NewScanner(stderr)
	wg.Add(2)
	go scan(outScanner)
	go scan(errScanner)
	go func() {
		wg.Wait()
		close(lines)
	}()

	var finalPath string
	var recent []string
	for line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "DDG_PATH|") {
			finalPath = strings.TrimSpace(strings.TrimPrefix(trimmed, "DDG_PATH|"))
			continue
		}
		if p, ok := parseYtDlpProgressLineV860(trimmed); ok {
			if progress != nil {
				progress(p.Done, p.Total)
			}
			continue
		}
		if trimmed != "" {
			recent = append(recent, trimmed)
			if len(recent) > 12 {
				recent = recent[len(recent)-12:]
			}
		}
	}
	waitErr := cmd.Wait()
	if waitErr != nil {
		msg := strings.Join(recent, " • ")
		if len(msg) > 1000 {
			msg = msg[len(msg)-1000:]
		}
		if msg != "" {
			return "", fmt.Errorf("yt-dlp: %v • %s", waitErr, msg)
		}
		return "", fmt.Errorf("yt-dlp: %w", waitErr)
	}
	if strings.TrimSpace(finalPath) == "" {
		return "", errors.New("yt-dlp a terminat fără să raporteze calea fișierului final")
	}
	if st, err := os.Stat(finalPath); err != nil || st.IsDir() {
		if err == nil {
			err = errors.New("calea finală yt-dlp este folder")
		}
		return "", fmt.Errorf("fișierul final yt-dlp nu există: %w", err)
	}
	return finalPath, nil
}

type directoryProgressSnapshotV860 map[string]int64

func snapshotDirectoryProgressV860(dest string) directoryProgressSnapshotV860 {
	out := directoryProgressSnapshotV860{}
	_ = filepath.WalkDir(dest, func(path string, d os.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return nil
		}
		if st, e := d.Info(); e == nil {
			out[path] = st.Size()
		}
		return nil
	})
	return out
}

func observedDownloadBytesV860(dest, wanted string, baseline directoryProgressSnapshotV860, started time.Time) int64 {
	wanted = strings.ToLower(strings.TrimSpace(filepath.Base(wanted)))
	wantedBase := strings.TrimSuffix(wanted, filepath.Ext(wanted))
	var best int64
	_ = filepath.WalkDir(dest, func(path string, d os.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return nil
		}
		st, e := d.Info()
		if e != nil || st.ModTime().Before(started.Add(-2*time.Second)) {
			return nil
		}
		name := strings.ToLower(d.Name())
		if wanted != "" && name != wanted && !strings.Contains(name, wantedBase) {
			return nil
		}
		old := baseline[path]
		growth := st.Size() - old
		if old == 0 {
			growth = st.Size()
		}
		if growth > best {
			best = growth
		}
		return nil
	})
	return best
}

func watchExternalDownloadDirectoryV860(ctx context.Context, dest, wanted string, total int64, progress func(int64, int64)) {
	if progress == nil {
		return
	}
	baseline := snapshotDirectoryProgressV860(dest)
	started := time.Now()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	var last int64
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			done := observedDownloadBytesV860(dest, wanted, baseline, started)
			if done > last {
				last = done
				progress(done, total)
			}
		}
	}
}
