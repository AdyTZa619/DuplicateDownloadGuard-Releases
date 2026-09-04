package main

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

type localMediaMetaCacheEntry struct {
	Size  int64     `json:"size"`
	MTime int64     `json:"mtime"`
	Info  MediaInfo `json:"info"`
}

var localMediaMetaCacheState = struct {
	sync.Mutex
	SaveMu     sync.Mutex
	AppDir     string
	Loaded     bool
	Dirty      bool
	Generation uint64
	Entries    map[string]localMediaMetaCacheEntry
}{}

func localMediaMetaCacheFile(a *App) string {
	return filepath.Join(a.appDir, "media_meta_cache.json")
}

func ensureLocalMediaMetaCacheLoaded(a *App) {
	localMediaMetaCacheState.Lock()
	defer localMediaMetaCacheState.Unlock()
	appDir := filepath.Clean(a.appDir)
	if localMediaMetaCacheState.Loaded && localMediaMetaCacheState.AppDir == appDir {
		return
	}
	localMediaMetaCacheState.AppDir = appDir
	localMediaMetaCacheState.Loaded = true
	localMediaMetaCacheState.Dirty = false
	localMediaMetaCacheState.Generation = 0
	localMediaMetaCacheState.Entries = map[string]localMediaMetaCacheEntry{}
	b, err := os.ReadFile(localMediaMetaCacheFile(a))
	if err != nil {
		return
	}
	var rows map[string]localMediaMetaCacheEntry
	if json.Unmarshal(b, &rows) == nil && rows != nil {
		localMediaMetaCacheState.Entries = rows
	}
}

func cachedLocalMediaInfo(a *App, e FileEntry) (MediaInfo, bool) {
	ensureLocalMediaMetaCacheLoaded(a)
	key := pathKey(e.Path)
	localMediaMetaCacheState.Lock()
	defer localMediaMetaCacheState.Unlock()
	row, ok := localMediaMetaCacheState.Entries[key]
	if !ok || row.Size != e.Size || row.MTime != e.MTime || !row.Info.OK {
		return MediaInfo{}, false
	}
	return row.Info, true
}

func cacheLocalMediaInfo(a *App, e FileEntry, info MediaInfo) {
	if !info.OK {
		return
	}
	ensureLocalMediaMetaCacheLoaded(a)
	localMediaMetaCacheState.Lock()
	localMediaMetaCacheState.Entries[pathKey(e.Path)] = localMediaMetaCacheEntry{Size: e.Size, MTime: e.MTime, Info: info}
	localMediaMetaCacheState.Dirty = true
	localMediaMetaCacheState.Generation++
	localMediaMetaCacheState.Unlock()
}

func pruneLocalMediaMetaCache(a *App, entries []FileEntry) bool {
	ensureLocalMediaMetaCacheLoaded(a)
	valid := make(map[string]FileEntry, len(entries))
	for _, e := range entries {
		if remoteMediaKind(e.Name) == "video" {
			valid[pathKey(e.Path)] = e
		}
	}
	changed := false
	localMediaMetaCacheState.Lock()
	for key, cached := range localMediaMetaCacheState.Entries {
		e, ok := valid[key]
		if !ok || e.Size != cached.Size || e.MTime != cached.MTime {
			delete(localMediaMetaCacheState.Entries, key)
			changed = true
		}
	}
	if changed {
		localMediaMetaCacheState.Dirty = true
		localMediaMetaCacheState.Generation++
	}
	localMediaMetaCacheState.Unlock()
	return changed
}

// replaceCacheFileV85 keeps cache writes reliable on Windows, where renaming a
// temporary file directly over an existing destination is not consistently
// supported. If replacement fails, the previous cache is restored.
func replaceCacheFileV85(tmp, path string) error {
	if err := os.Rename(tmp, path); err == nil {
		return nil
	}
	backup := path + ".old"
	_ = os.Remove(backup)
	if _, err := os.Stat(path); err == nil {
		if err := os.Rename(path, backup); err != nil {
			return err
		}
		if err := os.Rename(tmp, path); err != nil {
			_ = os.Rename(backup, path)
			return err
		}
		_ = os.Remove(backup)
		return nil
	}
	return os.Rename(tmp, path)
}

func saveLocalMediaMetaCache(a *App) error {
	ensureLocalMediaMetaCacheLoaded(a)
	localMediaMetaCacheState.SaveMu.Lock()
	defer localMediaMetaCacheState.SaveMu.Unlock()

	localMediaMetaCacheState.Lock()
	if !localMediaMetaCacheState.Dirty {
		localMediaMetaCacheState.Unlock()
		return nil
	}
	generation := localMediaMetaCacheState.Generation
	rows := make(map[string]localMediaMetaCacheEntry, len(localMediaMetaCacheState.Entries))
	for k, v := range localMediaMetaCacheState.Entries {
		rows[k] = v
	}
	localMediaMetaCacheState.Unlock()
	b, err := json.Marshal(rows)
	if err != nil {
		return err
	}
	path := localMediaMetaCacheFile(a)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0644); err != nil {
		return err
	}
	if err := replaceCacheFileV85(tmp, path); err != nil {
		return err
	}
	localMediaMetaCacheState.Lock()
	if localMediaMetaCacheState.Generation == generation {
		localMediaMetaCacheState.Dirty = false
	}
	localMediaMetaCacheState.Unlock()
	return nil
}

func durationCompatibleV85(remoteInfo, localInfo MediaInfo) (float64, bool) {
	if !remoteInfo.OK || !localInfo.OK || remoteInfo.Duration <= 0 || localInfo.Duration <= 0 {
		return 1, false
	}
	maxD := math.Max(remoteInfo.Duration, localInfo.Duration)
	delta := math.Abs(remoteInfo.Duration - localInfo.Duration)
	ratio := delta / maxD
	// Candidate discovery must be broader than the final verdict. A recode with
	// a short intro/outro can legitimately differ by tens of seconds; frame
	// fingerprinting decides whether it is really the same material.
	if ratio > .12 || delta > 90 {
		return ratio, false
	}
	if remoteInfo.Width > 0 && remoteInfo.Height > 0 && localInfo.Width > 0 && localInfo.Height > 0 {
		ra := float64(remoteInfo.Width) / float64(remoteInfo.Height)
		la := float64(localInfo.Width) / float64(localInfo.Height)
		if math.Abs(ra-la)/math.Max(ra, la) > .08 {
			return ratio, false
		}
	}
	return ratio, true
}

type cachedDurationCandidateV85 struct {
	Entry         FileEntry
	DurationRatio float64
	NameScore     int
	SizeRatio     float64
}

func appendDurationCandidateV85(rows []cachedDurationCandidateV85, remote RemoteItem, e FileEntry, ratio float64) []cachedDurationCandidateV85 {
	sizeRatio := 1.0
	if remote.Size > 0 {
		sizeRatio = float64(abs64(e.Size-remote.Size)) / float64(remote.Size)
	}
	return append(rows, cachedDurationCandidateV85{
		Entry:         e,
		DurationRatio: ratio,
		NameScore:     nameSimilarity(remote.Name, e.Name),
		SizeRatio:     sizeRatio,
	})
}

type videoDurationSearchV85 struct {
	Candidates []FileEntry
	Pending    int
	Probed     int
	Cached     int
}

// videoDurationCandidatesCached makes renamed-video discovery scalable without
// ever treating a partial cache as proof that a video is missing. Cached
// metadata is searched across the whole collection. A bounded number of
// uncached files is probed per call, and Pending reports everything that still
// cannot be ruled out so Download Guard can fail closed instead of returning a
// false MISSING verdict.
func (a *App) videoDurationCandidatesCached(ctx context.Context, remoteInfo MediaInfo, remote RemoteItem, entries, existing []FileEntry, limit int) videoDurationSearchV85 {
	if limit <= 0 {
		limit = 7
	}
	result := videoDurationSearchV85{Candidates: append([]FileEntry(nil), existing...)}
	fp := a.detectFFprobe()
	if fp == "" || !remoteInfo.OK || remoteInfo.Duration <= 0 {
		for _, e := range entries {
			if remoteMediaKind(e.Name) == "video" && !hasEntryPath(existing, e.Path) {
				result.Pending++
			}
		}
		return result
	}
	ensureLocalMediaMetaCacheLoaded(a)
	cacheChanged := pruneLocalMediaMetaCache(a, entries)

	matched := make([]cachedDurationCandidateV85, 0, 16)
	type rough struct {
		Entry FileEntry
		Rank  int
	}
	uncached := make([]rough, 0, 128)

	for _, e := range entries {
		if remoteMediaKind(e.Name) != "video" || hasEntryPath(existing, e.Path) {
			continue
		}
		if info, ok := cachedLocalMediaInfo(a, e); ok {
			result.Cached++
			if ratio, compatible := durationCompatibleV85(remoteInfo, info); compatible {
				matched = appendDurationCandidateV85(matched, remote, e, ratio)
			}
			continue
		}
		nameScore := nameSimilarity(remote.Name, e.Name)
		sizeRatio := 1.0
		if remote.Size > 0 {
			sizeRatio = float64(abs64(e.Size-remote.Size)) / float64(remote.Size)
		}
		closeness := int(math.Round(1000 / (1 + sizeRatio*8)))
		rank := closeness + nameScore*8
		if strings.EqualFold(filepathExt(remote.Name), filepathExt(e.Name)) {
			rank += 80
		}
		uncached = append(uncached, rough{Entry: e, Rank: rank})
	}

	sort.SliceStable(uncached, func(i, j int) bool {
		if uncached[i].Rank != uncached[j].Rank {
			return uncached[i].Rank > uncached[j].Rank
		}
		return strings.ToLower(uncached[i].Entry.Path) < strings.ToLower(uncached[j].Entry.Path)
	})

	const probeLimit = 48
	successful := 0
	for i, row := range uncached {
		if i >= probeLimit || ctx.Err() != nil {
			break
		}
		result.Probed++
		info := probeMedia(ctx, fp, row.Entry.Path, "LOCAL")
		if !info.OK {
			continue
		}
		successful++
		cacheLocalMediaInfo(a, row.Entry, info)
		cacheChanged = true
		if ratio, compatible := durationCompatibleV85(remoteInfo, info); compatible {
			matched = appendDurationCandidateV85(matched, remote, row.Entry, ratio)
		}
	}
	result.Pending = len(uncached) - successful
	if result.Pending < 0 {
		result.Pending = 0
	}
	if cacheChanged {
		if err := saveLocalMediaMetaCache(a); err != nil {
			a.logf("Smart Media Guard: nu am putut salva cache-ul media: %v", err)
		}
	}

	sort.SliceStable(matched, func(i, j int) bool {
		if matched[i].DurationRatio != matched[j].DurationRatio {
			return matched[i].DurationRatio < matched[j].DurationRatio
		}
		if matched[i].NameScore != matched[j].NameScore {
			return matched[i].NameScore > matched[j].NameScore
		}
		if matched[i].SizeRatio != matched[j].SizeRatio {
			return matched[i].SizeRatio < matched[j].SizeRatio
		}
		return strings.ToLower(matched[i].Entry.Path) < strings.ToLower(matched[j].Entry.Path)
	})

	for _, row := range matched {
		if len(result.Candidates) >= limit {
			break
		}
		if !hasEntryPath(result.Candidates, row.Entry.Path) {
			result.Candidates = append(result.Candidates, row.Entry)
		}
	}
	return result
}
