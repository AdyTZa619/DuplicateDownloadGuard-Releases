package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLocalFolderSignatureIgnoresOrderAndDuplicatesV857(t *testing.T) {
	a := localFolderSignatureV857([]string{`D:\Media`, `E:\Video`, `D:\Media`})
	b := localFolderSignatureV857([]string{`E:\Video`, `D:\Media`})
	if a != b {
		t.Fatalf("equivalent folder sets produced different signatures: %q != %q", a, b)
	}
}

func waitLocalFolderRefreshV857(t *testing.T, a *App) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		st := localFolderRefreshStateForV857(a)
		st.mu.Lock()
		done := st.initialized && !st.queued && st.appliedSig == st.observedSig
		st.mu.Unlock()
		if done && !a.opRunning.Load() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("local-folder auto-refresh did not finish")
}

func testHeartbeatFolderChangeRefreshesCurrentResultsV857(t *testing.T, liveRefresh bool) {
	t.Helper()
	first := t.TempDir()
	second := t.TempDir()
	download := t.TempDir()
	data := []byte("same-size-content-added-in-new-folder")
	local := filepath.Join(second, "original-D3558.jpg")
	if err := os.WriteFile(local, data, 0644); err != nil {
		t.Fatal(err)
	}

	remote := RemoteItem{
		ID:     1,
		Name:   "original.jpg",
		Path:   "/pack/original.jpg",
		Size:   int64(len(data)),
		Source: "MEGA",
	}
	a := &App{
		appDir:    t.TempDir(),
		index:     map[string]FileEntry{},
		bySize:    map[int64][]string{},
		byName:    map[string][]string{},
		decisions: map[string]Decision{},
		results: []Result{{
			ID:         1,
			Remote:     remote,
			Status:     "MISSING",
			AutoStatus: "MISSING",
		}},
	}
	a.cfg = Config{
		LocalPaths:         []string{first},
		DownloadDir:        download,
		Mode:               "balanced",
		LiveRefreshCompare: liveRefresh,
	}

	// First heartbeat only establishes the baseline configuration.
	noteLocalFolderConfigHeartbeatV857(a)

	// Simulate Dashboard -> Add folder -> saveCfg(). The next heartbeat must
	// pick up the new location and recalculate the already visible result.
	a.mu.Lock()
	a.cfg.LocalPaths = []string{first, second}
	a.mu.Unlock()
	noteLocalFolderConfigHeartbeatV857(a)
	waitLocalFolderRefreshV857(t, a)

	a.mu.RLock()
	defer a.mu.RUnlock()
	if len(a.results) != 1 {
		t.Fatalf("got %d results", len(a.results))
	}
	got := a.results[0]
	if got.Status != "POSSIBLE" || got.LocalPath != local || got.Candidates != 1 {
		t.Fatalf("new folder was not reflected in existing result (live=%v): %#v", liveRefresh, got)
	}
}

func TestHeartbeatFolderChangeRefreshesCurrentResultsV857(t *testing.T) {
	testHeartbeatFolderChangeRefreshesCurrentResultsV857(t, true)
}

func TestHeartbeatFolderChangeRefreshesWhenLiveCompareDisabledV857(t *testing.T) {
	testHeartbeatFolderChangeRefreshesCurrentResultsV857(t, false)
}

func TestHeartbeatFolderRemovalPrunesStaleIndexAndResultV857(t *testing.T) {
	keep := t.TempDir()
	removed := t.TempDir()
	download := t.TempDir()
	data := []byte("content-only-in-removed-folder")
	local := filepath.Join(removed, "original-D3558.jpg")
	if err := os.WriteFile(local, data, 0644); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(local)
	if err != nil {
		t.Fatal(err)
	}

	remote := RemoteItem{ID: 1, Name: "original.jpg", Path: "/pack/original.jpg", Size: int64(len(data)), Source: "MEGA"}
	entry := FileEntry{Path: local, Name: filepath.Base(local), Size: st.Size(), MTime: st.ModTime().UnixNano()}
	a := &App{
		appDir:    t.TempDir(),
		index:     map[string]FileEntry{local: entry},
		bySize:    map[int64][]string{entry.Size: {local}},
		byName:    map[string][]string{normalizeName(entry.Name): {local}},
		decisions: map[string]Decision{},
		results: []Result{{
			ID:         1,
			Remote:     remote,
			Status:     "POSSIBLE",
			AutoStatus: "POSSIBLE",
			LocalPath:  local,
			Candidates: 1,
		}},
	}
	a.cfg = Config{
		LocalPaths:         []string{keep, removed},
		DownloadDir:        download,
		Mode:               "balanced",
		LiveRefreshCompare: true,
	}

	noteLocalFolderConfigHeartbeatV857(a)
	a.mu.Lock()
	a.cfg.LocalPaths = []string{keep}
	a.mu.Unlock()
	noteLocalFolderConfigHeartbeatV857(a)
	waitLocalFolderRefreshV857(t, a)

	a.mu.RLock()
	defer a.mu.RUnlock()
	if _, ok := a.index[local]; ok {
		t.Fatalf("removed local root still contributes to index: %s", local)
	}
	if len(a.results) != 1 {
		t.Fatalf("got %d results", len(a.results))
	}
	got := a.results[0]
	if got.Status != "MISSING" || got.LocalPath != "" {
		t.Fatalf("result still uses removed local folder: %#v", got)
	}
}

func TestRemovedParentKeepsFileCoveredByActiveNestedRootV857(t *testing.T) {
	parent := t.TempDir()
	nested := filepath.Join(parent, "keep")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatal(err)
	}
	local := filepath.Join(nested, "keep.jpg")
	if err := os.WriteFile(local, []byte("keep"), 0644); err != nil {
		t.Fatal(err)
	}
	st, _ := os.Stat(local)
	entry := FileEntry{Path: local, Name: filepath.Base(local), Size: st.Size(), MTime: st.ModTime().UnixNano()}
	a := &App{
		index:  map[string]FileEntry{local: entry},
		bySize: map[int64][]string{},
		byName: map[string][]string{},
	}
	a.cfg = Config{LocalPaths: []string{nested}}
	if changed := pruneRemovedLocalRootsV857(a, normalizedLocalFoldersV857([]string{parent, nested}), normalizedLocalFoldersV857([]string{nested})); changed {
		t.Fatal("file under still-active nested root was pruned")
	}
	if _, ok := a.index[local]; !ok {
		t.Fatal("file under active nested root disappeared")
	}
}
