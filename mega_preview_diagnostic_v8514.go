package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type megaPreviewDiagEntryV8516 struct {
	line string
}

var (
	megaPreviewDiagOnceV8516 sync.Once
	megaPreviewDiagQueueV8516 = make(chan megaPreviewDiagEntryV8516, 4096)
)

func megaPreviewDiagnosticPathV8514() string {
	dir, err := portableDataDir()
	if err != nil || strings.TrimSpace(dir) == "" {
		return filepath.Join(executableDir(), "MEGA_Preview_Diagnostic.log")
	}
	return filepath.Join(dir, "MEGA_Preview_Diagnostic.log")
}

func startMegaPreviewDiagnosticWorkerV8516() {
	go func() {
		for entry := range megaPreviewDiagQueueV8516 {
			if entry.line == "" {
				continue
			}
			path := megaPreviewDiagnosticPathV8514()
			if st, err := os.Stat(path); err == nil && st.Size() > 2<<20 {
				_ = os.Remove(path + ".old")
				_ = os.Rename(path, path+".old")
			}
			if f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); err == nil {
				_, _ = fmt.Fprintln(f, entry.line)
				_ = f.Close()
			}
		}
	}()
}

// megaPreviewDiagfV8514 is intentionally non-blocking. The timestamp and
// message are captured on the caller goroutine, but disk I/O is delegated to a
// worker. Diagnostics must never delay /api/remote-preview/start or the player.
func megaPreviewDiagfV8514(format string, args ...any) {
	megaPreviewDiagOnceV8516.Do(startMegaPreviewDiagnosticWorkerV8516)
	stamp := time.Now().Format("2006-01-02 15:04:05.000")
	entry := megaPreviewDiagEntryV8516{line: stamp + "  " + fmt.Sprintf(format, args...)}
	select {
	case megaPreviewDiagQueueV8516 <- entry:
	default:
		// Never block preview merely to preserve a diagnostic line.
	}
}

func megaPreviewSafeArgsV8514(args []string) string {
	if len(args) == 0 {
		return "<no-args>"
	}
	safe := append([]string(nil), args...)
	if strings.EqualFold(safe[0], "login") && len(safe) > 1 {
		safe[1] = "<MEGA_LINK_REDACTED>"
	}
	for i := range safe {
		if strings.HasPrefix(safe[i], "H:") && len(safe[i]) > 8 {
			safe[i] = safe[i][:6] + "…"
		}
	}
	return strings.Join(safe, " ")
}

func megaPreviewShortOutputV8514(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\r", ""))
	s = strings.ReplaceAll(s, "\n", " | ")
	if len(s) > 240 {
		s = s[:240] + "…"
	}
	return s
}
