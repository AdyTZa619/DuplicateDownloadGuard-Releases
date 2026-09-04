package main

import (
	"fmt"
	"sync"
)

type megaPreviewLogEntryV8515 struct {
	a   *App
	msg string
}

var (
	megaPreviewLogOnceV8515  sync.Once
	megaPreviewLogQueueV8515 = make(chan megaPreviewLogEntryV8515, 256)
)

func startMegaPreviewLogWorkerV8515() {
	go func() {
		for entry := range megaPreviewLogQueueV8515 {
			if entry.a == nil || entry.msg == "" {
				continue
			}
			entry.a.logf("%s", entry.msg)
		}
	}()
}

// previewLogfAsyncV8515 keeps the interactive preview path independent from
// the general application log mutex and disk writes. The user's player must
// never wait because DuplicateDownloadGuard.log is busy or slow.
func (a *App) previewLogfAsyncV8515(format string, args ...any) {
	if a == nil {
		return
	}
	megaPreviewLogOnceV8515.Do(startMegaPreviewLogWorkerV8515)
	entry := megaPreviewLogEntryV8515{a: a, msg: fmt.Sprintf(format, args...)}
	select {
	case megaPreviewLogQueueV8515 <- entry:
	default:
		megaPreviewDiagfV8514("GENERAL LOG DROP preview queue full: %s", entry.msg)
	}
}
