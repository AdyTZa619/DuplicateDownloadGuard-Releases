package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type localVideoFingerprintCacheEntry struct {
	Size  int64               `json:"size"`
	MTime int64               `json:"mtime"`
	FP    videoFingerprintV85 `json:"fingerprint"`
}

var localVideoFingerprintCacheState = struct {
	sync.Mutex
	AppDir  string
	Loaded  bool
	Dirty   bool
	Entries map[string]localVideoFingerprintCacheEntry
}{}

func localVideoFingerprintCacheFile(a *App) string {
	return filepath.Join(a.appDir, "video_fingerprint_cache.json")
}

func ensureLocalVideoFingerprintCacheLoaded(a *App) {
	localVideoFingerprintCacheState.Lock()
	defer localVideoFingerprintCacheState.Unlock()
	appDir := filepath.Clean(a.appDir)
	if localVideoFingerprintCacheState.Loaded && localVideoFingerprintCacheState.AppDir == appDir {
		return
	}
	localVideoFingerprintCacheState.AppDir = appDir
	localVideoFingerprintCacheState.Loaded = true
	localVideoFingerprintCacheState.Dirty = false
	localVideoFingerprintCacheState.Entries = map[string]localVideoFingerprintCacheEntry{}
	b, err := os.ReadFile(localVideoFingerprintCacheFile(a))
	if err != nil {
		return
	}
	var rows map[string]localVideoFingerprintCacheEntry
	if json.Unmarshal(b, &rows) == nil && rows != nil {
		localVideoFingerprintCacheState.Entries = rows
	}
}

func videoFingerprintUsableV85(fp videoFingerprintV85) bool {
	if !fp.Info.OK || fp.Info.Duration <= 0 || len(fp.Hashes) != len(v85FramePoints) || len(fp.Valid) != len(v85FramePoints) {
		return false
	}
	valid := 0
	for _, ok := range fp.Valid {
		if ok {
			valid++
		}
	}
	return valid >= 4
}

func cachedLocalVideoFingerprintV85(a *App, e FileEntry) (videoFingerprintV85, bool) {
	ensureLocalVideoFingerprintCacheLoaded(a)
	localVideoFingerprintCacheState.Lock()
	defer localVideoFingerprintCacheState.Unlock()
	row, ok := localVideoFingerprintCacheState.Entries[pathKey(e.Path)]
	if !ok || row.Size != e.Size || row.MTime != e.MTime || !videoFingerprintUsableV85(row.FP) {
		return videoFingerprintV85{}, false
	}
	return row.FP, true
}

func cacheLocalVideoFingerprintV85(a *App, e FileEntry, fp videoFingerprintV85) {
	if !videoFingerprintUsableV85(fp) {
		return
	}
	ensureLocalVideoFingerprintCacheLoaded(a)
	localVideoFingerprintCacheState.Lock()
	localVideoFingerprintCacheState.Entries[pathKey(e.Path)] = localVideoFingerprintCacheEntry{Size: e.Size, MTime: e.MTime, FP: fp}
	localVideoFingerprintCacheState.Dirty = true
	localVideoFingerprintCacheState.Unlock()
}

func pruneLocalVideoFingerprintCacheV85(a *App, entries []FileEntry) bool {
	ensureLocalVideoFingerprintCacheLoaded(a)
	valid := make(map[string]FileEntry, len(entries))
	for _, e := range entries {
		if remoteMediaKind(e.Name) == "video" {
			valid[pathKey(e.Path)] = e
		}
	}
	changed := false
	localVideoFingerprintCacheState.Lock()
	for key, cached := range localVideoFingerprintCacheState.Entries {
		e, ok := valid[key]
		if !ok || e.Size != cached.Size || e.MTime != cached.MTime || !videoFingerprintUsableV85(cached.FP) {
			delete(localVideoFingerprintCacheState.Entries, key)
			changed = true
		}
	}
	if changed {
		localVideoFingerprintCacheState.Dirty = true
	}
	localVideoFingerprintCacheState.Unlock()
	return changed
}

func flushLocalVideoFingerprintCacheV85(a *App) error {
	ensureLocalVideoFingerprintCacheLoaded(a)
	localVideoFingerprintCacheState.Lock()
	if !localVideoFingerprintCacheState.Dirty {
		localVideoFingerprintCacheState.Unlock()
		return nil
	}
	rows := make(map[string]localVideoFingerprintCacheEntry, len(localVideoFingerprintCacheState.Entries))
	for k, v := range localVideoFingerprintCacheState.Entries {
		rows[k] = v
	}
	localVideoFingerprintCacheState.Unlock()

	b, err := json.Marshal(rows)
	if err != nil {
		return err
	}
	path := localVideoFingerprintCacheFile(a)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0644); err != nil {
		return err
	}
	if err := replaceCacheFileV85(tmp, path); err != nil {
		return err
	}
	localVideoFingerprintCacheState.Lock()
	localVideoFingerprintCacheState.Dirty = false
	localVideoFingerprintCacheState.Unlock()
	return nil
}

func (a *App) buildLocalVideoFingerprintV85(ctx context.Context, candidate FileEntry) (videoFingerprintV85, error) {
	if cached, ok := cachedLocalVideoFingerprintV85(a, candidate); ok {
		return cached, nil
	}
	ff := a.detectFFmpeg()
	fpExe := a.detectFFprobe()
	if ff == "" || fpExe == "" {
		return videoFingerprintV85{}, fmt.Errorf("ffmpeg + ffprobe lipsesc")
	}
	info, ok := cachedLocalMediaInfo(a, candidate)
	if !ok {
		info = probeMedia(ctx, fpExe, candidate.Path, "LOCAL")
		if info.OK {
			cacheLocalMediaInfo(a, candidate, info)
		}
	}
	if !info.OK || info.Duration <= 0 {
		return videoFingerprintV85{}, fmt.Errorf("nu pot citi durata videoclipului local")
	}
	out := videoFingerprintV85{Info: info, Hashes: make([]uint64, len(v85FramePoints)), Valid: make([]bool, len(v85FramePoints))}
	valid := 0
	for i, p := range v85FramePoints {
		h, informative, err := frameSignatureV85(ctx, ff, candidate.Path, info.Duration*p)
		if err != nil || !informative {
			continue
		}
		out.Hashes[i] = h
		out.Valid[i] = true
		valid++
	}
	if valid < 4 {
		return videoFingerprintV85{}, fmt.Errorf("prea puține cadre informative locale: %d/7", valid)
	}
	cacheLocalVideoFingerprintV85(a, candidate, out)
	return out, nil
}
