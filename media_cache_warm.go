package main

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"
)

// Media warmup is intentionally conservative. It prepares cheap local evidence
// after the first Smart Media Guard use, but never launches a farm of FFmpeg
// processes or computes full seven-frame video fingerprints in the background.
// Image signatures and video metadata are enough to make later candidate
// discovery dramatically cheaper; expensive frame fingerprints stay on-demand.
type mediaWarmStateV85 struct {
	sync.Mutex
	Running bool
	Pending bool
	Entries []FileEntry
}

var mediaWarmRegistryV85 sync.Map // *App -> *mediaWarmStateV85

func mediaWarmStateForV85(a *App) *mediaWarmStateV85 {
	if raw, ok := mediaWarmRegistryV85.Load(a); ok {
		return raw.(*mediaWarmStateV85)
	}
	st := &mediaWarmStateV85{}
	actual, _ := mediaWarmRegistryV85.LoadOrStore(a, st)
	return actual.(*mediaWarmStateV85)
}

func mergeWarmEntriesV85(dst []FileEntry, src []FileEntry) []FileEntry {
	byPath := make(map[string]FileEntry, len(dst)+len(src))
	for _, e := range dst {
		byPath[pathKey(e.Path)] = e
	}
	for _, e := range src {
		kind := remoteMediaKind(e.Name)
		if kind != "image" && kind != "video" {
			continue
		}
		byPath[pathKey(e.Path)] = e
	}
	out := make([]FileEntry, 0, len(byPath))
	for _, e := range byPath {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Path) < strings.ToLower(out[j].Path) })
	return out
}

// scheduleMediaCacheWarmV85 coalesces requests and permits only one worker per
// App. New requests arriving while a worker is active are merged into one
// follow-up pass rather than spawning more goroutines.
func scheduleMediaCacheWarmV85(a *App, entries []FileEntry) {
	if a == nil || len(entries) == 0 {
		return
	}
	st := mediaWarmStateForV85(a)
	st.Lock()
	st.Entries = mergeWarmEntriesV85(st.Entries, entries)
	if st.Running {
		st.Pending = true
		st.Unlock()
		return
	}
	st.Running = true
	st.Pending = false
	st.Unlock()
	go runMediaCacheWarmLoopV85(a, st)
}

func foregroundIdleForWarmV85(a *App) bool {
	if a == nil || a.opRunning.Load() {
		return false
	}
	if !a.guardMu.TryLock() {
		return false
	}
	a.guardMu.Unlock()
	return true
}

func runMediaCacheWarmLoopV85(a *App, st *mediaWarmStateV85) {
	defer func() {
		st.Lock()
		st.Running = false
		st.Unlock()
	}()

	for {
		st.Lock()
		entries := append([]FileEntry(nil), st.Entries...)
		st.Entries = nil
		st.Pending = false
		st.Unlock()

		// Let foreground work win. If DDG stays busy for ~10 seconds, retain the
		// request for the next foreground-triggered opportunity instead of
		// competing with scan/guard for CPU and disk.
		idle := false
		for tries := 0; tries < 20; tries++ {
			if foregroundIdleForWarmV85(a) {
				idle = true
				break
			}
			time.Sleep(500 * time.Millisecond)
		}
		if !idle {
			st.Lock()
			st.Entries = mergeWarmEntriesV85(st.Entries, entries)
			st.Unlock()
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
		warmMediaCachePassV85(ctx, a, entries)
		cancel()

		st.Lock()
		more := st.Pending || len(st.Entries) > 0
		st.Unlock()
		if !more {
			return
		}
	}
}

func warmMediaCachePassV85(ctx context.Context, a *App, entries []FileEntry) {
	if len(entries) == 0 || ctx.Err() != nil {
		return
	}
	// Small budgets keep the worker cheap even on huge collections. Repeated
	// foreground use progressively fills the cache without a large one-time hit.
	const imageBudget = 24
	const videoMetaBudget = 16
	imagesDone, videosDone := 0, 0
	imageChanged, mediaChanged := false, false
	ffprobe := a.detectFFprobe()

	for _, e := range entries {
		if ctx.Err() != nil || !foregroundIdleForWarmV85(a) {
			break
		}
		switch remoteMediaKind(e.Name) {
		case "image":
			if imagesDone >= imageBudget {
				continue
			}
			if _, ok := cachedLocalImageSignatureV85(a, e); ok || cachedLocalImageFailureV85(a, e) {
				continue
			}
			imagesDone++
			sig, err := readLocalImageSignatureV85(e.Path)
			if err != nil {
				cacheLocalImageFailureV85(a, e, err.Error())
			} else {
				cacheLocalImageSignatureV85(a, e, sig)
			}
			imageChanged = true
		case "video":
			if videosDone >= videoMetaBudget || ffprobe == "" {
				continue
			}
			if _, ok := cachedLocalMediaInfo(a, e); ok || cachedLocalMediaFailureV85(a, e) {
				continue
			}
			videosDone++
			info := probeMedia(ctx, ffprobe, e.Path, "LOCAL")
			if info.OK {
				cacheLocalMediaInfo(a, e, info)
			} else if ctx.Err() == nil {
				cacheLocalMediaFailureV85(a, e, info.Error)
			}
			mediaChanged = true
		}
	}
	if imageChanged {
		if err := saveLocalImageSignatureCacheV85(a); err != nil {
			a.logf("Media pre-index: salvarea cache-ului imagine a eșuat: %v", err)
		}
	}
	if mediaChanged {
		if err := saveLocalMediaMetaCache(a); err != nil {
			a.logf("Media pre-index: salvarea metadatelor video a eșuat: %v", err)
		}
	}
	if imagesDone > 0 || videosDone > 0 {
		a.logf("Media pre-index: pregătite %d imagini și %d videoclipuri (metadate)", imagesDone, videosDone)
	}
}
