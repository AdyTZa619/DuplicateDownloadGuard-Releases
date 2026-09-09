package main

import (
	"sort"
	"strings"
	"sync"
	"time"
)

// The source comparison already performs a live HDD walk. A download started
// shortly afterwards should reuse that exact snapshot instead of walking the
// same roots again. Five minutes is long enough to inspect a result set and
// decide what to download, while still keeping the snapshot intentionally
// short-lived. A later preflight falls back to a new live scan.
const guardFreshIndexTTLV8545 = 5 * time.Minute

type guardRefreshSnapshotV8545 struct {
	At       time.Time
	RootsKey string
	Scan     guardScan
	Entries  []FileEntry
}

var guardRefreshSnapshotsV8545 sync.Map

func guardRootsKeyV8545(roots []string) string {
	keys := make([]string, 0, len(roots))
	for _, root := range roots {
		keys = append(keys, pathKey(root))
	}
	sort.Strings(keys)
	return strings.Join(keys, "\n")
}

func (a *App) rememberGuardRefreshV8545(entries []FileEntry, scan guardScan) {
	copyEntries := append([]FileEntry(nil), entries...)
	copyScan := scan
	copyScan.Roots = append([]string(nil), scan.Roots...)
	guardRefreshSnapshotsV8545.Store(a, guardRefreshSnapshotV8545{
		At:       time.Now(),
		RootsKey: guardRootsKeyV8545(scan.Roots),
		Scan:     copyScan,
		Entries:  copyEntries,
	})
}

func (a *App) reuseFreshGuardIndexV8545(destination string) ([]FileEntry, guardScan, time.Duration, bool) {
	raw, ok := guardRefreshSnapshotsV8545.Load(a)
	if !ok {
		return nil, guardScan{}, 0, false
	}
	snapshot, ok := raw.(guardRefreshSnapshotV8545)
	if !ok {
		return nil, guardScan{}, 0, false
	}
	age := time.Since(snapshot.At)
	if age < 0 || age > guardFreshIndexTTLV8545 {
		return nil, guardScan{}, age, false
	}
	roots := a.guardRoots(destination)
	if guardRootsKeyV8545(roots) != snapshot.RootsKey {
		return nil, guardScan{}, age, false
	}
	entries := append([]FileEntry(nil), snapshot.Entries...)
	scan := snapshot.Scan
	scan.Roots = append([]string(nil), snapshot.Scan.Roots...)
	return entries, scan, age, true
}
