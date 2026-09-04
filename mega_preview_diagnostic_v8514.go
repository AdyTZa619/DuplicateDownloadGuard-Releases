package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var megaPreviewDiagMuV8514 sync.Mutex

func megaPreviewDiagnosticPathV8514() string {
	dir, err := portableDataDir()
	if err != nil || strings.TrimSpace(dir) == "" {
		return filepath.Join(executableDir(), "MEGA_Preview_Diagnostic.log")
	}
	return filepath.Join(dir, "MEGA_Preview_Diagnostic.log")
}

func megaPreviewDiagfV8514(format string, args ...any) {
	megaPreviewDiagMuV8514.Lock()
	defer megaPreviewDiagMuV8514.Unlock()

	path := megaPreviewDiagnosticPathV8514()
	if st, err := os.Stat(path); err == nil && st.Size() > 2<<20 {
		_ = os.Remove(path + ".old")
		_ = os.Rename(path, path+".old")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	stamp := time.Now().Format("2006-01-02 15:04:05.000")
	_, _ = fmt.Fprintf(f, "%s  %s\n", stamp, fmt.Sprintf(format, args...))
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
