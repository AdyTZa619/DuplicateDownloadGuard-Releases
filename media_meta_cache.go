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
	AppDir  string
	Loaded  bool
	Entries map[string]localMediaMetaCacheEntry
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
	localMediaMetaCacheState.Unlock()
}

func saveLocalMediaMetaCache(a *App) error {
	ensureLocalMediaMetaCacheLoaded(a)
	localMediaMetaCacheState.Lock()
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
	return os.Rename(tmp, path)
}

func durationCompatibleV85(remoteInfo, localInfo MediaInfo) (float64, bool) {
	if !remoteInfo.OK || !localInfo.OK || remoteInfo.Duration <= 0 || localInfo.Duration <= 0 {
		return 1, false
	}
	maxD := math.Max(remoteInfo.Duration, localInfo.Duration)
	delta := math.Abs(remoteInfo.Duration - localInfo.Duration)
	ratio := delta / maxD
	if ratio > .015 && delta > 1.5 {
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

func appendDurationCandidateV85(rows []cachedDurationCandidateV85, remote RemoteItem, e FileEntry, info MediaInfo, ratio float64) []cachedDurationCandidateV85 {
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

// videoDurationCandidatesCached makes the renamed-video fallback scalable.
// Cached metadata is checked across the whole local collection regardless of
// filename or byte size. Only a bounded number of uncached files are ffprobed
// per verification, and those results persist for later scans.
func (a *App) videoDurationCandidatesCached(ctx context.Context, remoteInfo MediaInfo, remote RemoteItem, entries, existing []FileEntry, limit int) []FileEntry {
	if limit <= 0 {
		limit = 7
	}
	fp := a.detectFFprobe()
	if fp == "" || !remoteInfo.OK || remoteInfo.Duration <= 0 {
		return existing
	}
	ensureLocalMediaMetaCacheLoaded(a)

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
			if ratio, compatible := durationCompatibleV85(remoteInfo, info); compatible {
				matched = appendDurationCandidateV85(matched, remote, e, info, ratio)
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
	if len(uncached) > 48 {
		uncached = uncached[:48]
	}

	cacheChanged := false
	for _, row := range uncached {
		if ctx.Err() != nil {
			break
		}
		info := probeMedia(ctx, fp, row.Entry.Path, "LOCAL")
		if !info.OK {
			continue
		}
		cacheLocalMediaInfo(a, row.Entry, info)
		cacheChanged = true
		if ratio, compatible := durationCompatibleV85(remoteInfo, info); compatible {
			matched = appendDurationCandidateV85(matched, remote, row.Entry, info, ratio)
		}
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

	out := append([]FileEntry(nil), existing...)
	for _, row := range matched {
		if len(out) >= limit {
			break
		}
		if !hasEntryPath(out, row.Entry.Path) {
			out = append(out, row.Entry)
		}
	}
	return out
}
