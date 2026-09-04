package main

import (
	"context"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// localFolderRefreshState tracks the configuration seen and applied by one App
// instance. It deliberately lives outside Config so no persistent format change
// is required for this reliability fix.
type localFolderRefreshStateV857 struct {
	mu          sync.Mutex
	initialized bool
	observedSig string
	appliedSig  string
	queued      bool
}

var localFolderRefreshStatesV857 sync.Map // map[*App]*localFolderRefreshStateV857

func localFolderSignatureV857(paths []string) string {
	clean := make([]string, 0, len(paths))
	seen := map[string]bool{}
	for _, raw := range paths {
		p := strings.TrimSpace(raw)
		if p == "" {
			continue
		}
		p = filepath.Clean(p)
		if runtimeIsWindows() {
			p = strings.ToLower(p)
		}
		if seen[p] {
			continue
		}
		seen[p] = true
		clean = append(clean, p)
	}
	sort.Strings(clean)
	return strings.Join(clean, "\x00")
}

func (a *App) currentLocalFolderSignatureV857() string {
	if a == nil {
		return ""
	}
	a.mu.RLock()
	paths := append([]string(nil), a.cfg.LocalPaths...)
	a.mu.RUnlock()
	return localFolderSignatureV857(paths)
}

func localFolderRefreshStateForV857(a *App) *localFolderRefreshStateV857 {
	if raw, ok := localFolderRefreshStatesV857.Load(a); ok {
		return raw.(*localFolderRefreshStateV857)
	}
	st := &localFolderRefreshStateV857{}
	actual, _ := localFolderRefreshStatesV857.LoadOrStore(a, st)
	return actual.(*localFolderRefreshStateV857)
}

// noteLocalFolderConfigHeartbeatV857 is intentionally cheap: it is called from
// the existing UI heartbeat and only compares a short normalized signature.
// The first heartbeat establishes a baseline. A later add/remove/reorder of
// local folders is coalesced into one refresh worker.
func noteLocalFolderConfigHeartbeatV857(a *App) {
	if a == nil {
		return
	}
	st := localFolderRefreshStateForV857(a)
	sig := a.currentLocalFolderSignatureV857()

	st.mu.Lock()
	if !st.initialized {
		st.initialized = true
		st.observedSig = sig
		st.appliedSig = sig
		st.mu.Unlock()
		return
	}
	if st.observedSig != sig {
		st.observedSig = sig
	}
	if st.observedSig == st.appliedSig || st.queued {
		st.mu.Unlock()
		return
	}
	st.queued = true
	st.mu.Unlock()

	go runLocalFolderRefreshWorkerV857(a, st)
}

func runLocalFolderRefreshWorkerV857(a *App, st *localFolderRefreshStateV857) {
	for {
		st.mu.Lock()
		targetSig := st.observedSig
		st.mu.Unlock()

		// Do not race a MEGA scan, explicit index build or another foreground
		// operation. Download Guard is serialized separately by guardMu below.
		for a.opRunning.Load() {
			time.Sleep(350 * time.Millisecond)
		}

		a.guardMu.Lock()
		if !a.opRunning.CompareAndSwap(false, true) {
			a.guardMu.Unlock()
			time.Sleep(350 * time.Millisecond)
			continue
		}

		a.mu.Lock()
		a.progress = Progress{
			Active:    true,
			Phase:     "local-refresh",
			State:     "running",
			Message:   "Actualizez automat folderele locale…",
			Detail:    "Reindexez locațiile configurate și recalculez rezultatele curente ca să nu rămână fișiere noi pe dinafară.",
			StartedAt: time.Now().Unix(),
			CanCancel: false,
		}
		rows := append([]Result(nil), a.results...)
		mode := a.cfg.Mode
		liveRefresh := a.cfg.LiveRefreshCompare
		a.mu.Unlock()

		ctx := context.Background()
		var err error
		if len(rows) > 0 && liveRefresh {
			items := make([]RemoteItem, 0, len(rows))
			for _, row := range rows {
				items = append(items, row.Remote)
			}
			a.compareRemote(ctx, items, mode)
		} else {
			_, _, err = a.refreshLiveIndexForGuard(ctx, "")
			if err == nil {
				err = a.saveIndex()
			}
			if err == nil && len(rows) > 0 {
				items := make([]RemoteItem, 0, len(rows))
				for _, row := range rows {
					items = append(items, row.Remote)
				}
				a.compareRemote(ctx, items, mode)
			}
		}

		if err != nil {
			a.failOp("Actualizarea automată a folderelor a eșuat", err.Error())
			a.logf("Auto-refresh foldere locale: %v", err)
		} else {
			a.endOp("Foldere locale actualizate • rezultate recalculate")
			a.logf("Auto-refresh foldere locale: index și rezultate actualizate")
		}
		a.guardMu.Unlock()

		st.mu.Lock()
		st.appliedSig = targetSig
		if st.observedSig == st.appliedSig {
			st.queued = false
			st.mu.Unlock()
			return
		}
		// Configurația s-a schimbat încă o dată în timp ce scanam. Păstrăm
		// workerul și repetăm pentru ultima stare observată, fără a pierde update-ul.
		st.mu.Unlock()
	}
}
