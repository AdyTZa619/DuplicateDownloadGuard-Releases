package main

import "context"

// MEGAcmd has one process-wide/account session. Login/logout/webdav/get
// operations must not overlap, otherwise one operation can invalidate another.
// A channel is used instead of sync.Mutex so callers with a cancellable context
// can stop waiting immediately.
var megaSessionGate = func() chan struct{} {
	ch := make(chan struct{}, 1)
	ch <- struct{}{}
	return ch
}()

func acquireMegaSession(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-megaSessionGate:
		return nil
	}
}

func releaseMegaSession() {
	select {
	case megaSessionGate <- struct{}{}:
	default:
		panic("DDG: releaseMegaSession fără acquire")
	}
}

// Caller already owns megaSessionGate. This ordering (MEGA gate -> previewMu)
// is used everywhere to avoid deadlocks while restoring/stopping WebDAV.
func (a *App) stopMegaPreviewWhileSessionOwned(reason string) error {
	a.invalidateMegaPreviewControllerV8526(reason)
	a.previewMu.Lock()
	defer a.previewMu.Unlock()
	return a.stopMegaPreviewLocked(reason)
}
