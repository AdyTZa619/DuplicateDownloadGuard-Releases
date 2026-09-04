package main

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"
)

const maxLocalAudioSegmentCacheEntriesV85 = 512

type localAudioSegmentCacheEntryV85 struct {
	Path        string   `json:"path"`
	Size        int64    `json:"size"`
	MTime       int64    `json:"mtime"`
	StartMS     int64    `json:"startMs"`
	DurationMS  int64    `json:"durationMs"`
	Fingerprint []uint32 `json:"fingerprint"`
	CreatedAt   int64    `json:"createdAt"`
}

var localAudioSegmentCacheStateV85 = struct {
	sync.Mutex
	SaveMu     sync.Mutex
	AppDir     string
	Loaded     bool
	Dirty      bool
	Generation uint64
	Entries    map[string]localAudioSegmentCacheEntryV85
}{}

func localAudioSegmentCacheFileV85(a *App) string {
	return filepath.Join(a.appDir, "audio_segment_cache.json")
}

func audioSegmentKeyV85(path string, start, seconds float64) string {
	startMS := int64(math.Round(start * 1000))
	durationMS := int64(math.Round(seconds * 1000))
	return pathKey(path) + "\x1f" + strconv.FormatInt(startMS, 10) + "\x1f" + strconv.FormatInt(durationMS, 10)
}

func ensureLocalAudioSegmentCacheLoadedV85(a *App) {
	localAudioSegmentCacheStateV85.Lock()
	defer localAudioSegmentCacheStateV85.Unlock()
	appDir := filepath.Clean(a.appDir)
	if localAudioSegmentCacheStateV85.Loaded && localAudioSegmentCacheStateV85.AppDir == appDir {
		return
	}
	localAudioSegmentCacheStateV85.AppDir = appDir
	localAudioSegmentCacheStateV85.Loaded = true
	localAudioSegmentCacheStateV85.Dirty = false
	localAudioSegmentCacheStateV85.Generation = 0
	localAudioSegmentCacheStateV85.Entries = map[string]localAudioSegmentCacheEntryV85{}
	b, err := os.ReadFile(localAudioSegmentCacheFileV85(a))
	if err != nil {
		return
	}
	var rows map[string]localAudioSegmentCacheEntryV85
	if json.Unmarshal(b, &rows) == nil && rows != nil {
		for key, row := range rows {
			if len(row.Fingerprint) >= 8 && row.Path != "" {
				localAudioSegmentCacheStateV85.Entries[key] = row
			}
		}
	}
}

func cachedLocalAudioSegmentV85(a *App, path string, start, seconds float64) ([]uint32, bool) {
	ensureLocalAudioSegmentCacheLoadedV85(a)
	st, err := os.Stat(path)
	if err != nil || st.IsDir() {
		return nil, false
	}
	key := audioSegmentKeyV85(path, start, seconds)
	localAudioSegmentCacheStateV85.Lock()
	row, ok := localAudioSegmentCacheStateV85.Entries[key]
	if ok && (row.Size != st.Size() || row.MTime != st.ModTime().UnixNano() || len(row.Fingerprint) < 8) {
		delete(localAudioSegmentCacheStateV85.Entries, key)
		localAudioSegmentCacheStateV85.Dirty = true
		localAudioSegmentCacheStateV85.Generation++
		ok = false
	}
	localAudioSegmentCacheStateV85.Unlock()
	if !ok {
		return nil, false
	}
	return append([]uint32(nil), row.Fingerprint...), true
}

func pruneLocalAudioSegmentCacheV85() {
	localAudioSegmentCacheStateV85.Lock()
	defer localAudioSegmentCacheStateV85.Unlock()
	if len(localAudioSegmentCacheStateV85.Entries) <= maxLocalAudioSegmentCacheEntriesV85 {
		return
	}
	type row struct {
		Key string
		At  int64
	}
	rows := make([]row, 0, len(localAudioSegmentCacheStateV85.Entries))
	for key, entry := range localAudioSegmentCacheStateV85.Entries {
		rows = append(rows, row{Key: key, At: entry.CreatedAt})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].At != rows[j].At {
			return rows[i].At < rows[j].At
		}
		return rows[i].Key < rows[j].Key
	})
	remove := len(rows) - maxLocalAudioSegmentCacheEntriesV85
	for i := 0; i < remove; i++ {
		delete(localAudioSegmentCacheStateV85.Entries, rows[i].Key)
	}
	if remove > 0 {
		localAudioSegmentCacheStateV85.Dirty = true
		localAudioSegmentCacheStateV85.Generation++
	}
}

func cacheLocalAudioSegmentV85(a *App, path string, start, seconds float64, fingerprint []uint32) error {
	if len(fingerprint) < 8 {
		return errors.New("fingerprint audio local prea scurt")
	}
	st, err := os.Stat(path)
	if err != nil || st.IsDir() {
		return errors.New("fișier audio local indisponibil")
	}
	ensureLocalAudioSegmentCacheLoadedV85(a)
	key := audioSegmentKeyV85(path, start, seconds)
	localAudioSegmentCacheStateV85.Lock()
	localAudioSegmentCacheStateV85.Entries[key] = localAudioSegmentCacheEntryV85{
		Path:        path,
		Size:        st.Size(),
		MTime:       st.ModTime().UnixNano(),
		StartMS:     int64(math.Round(start * 1000)),
		DurationMS:  int64(math.Round(seconds * 1000)),
		Fingerprint: append([]uint32(nil), fingerprint...),
		CreatedAt:   time.Now().UnixNano(),
	}
	localAudioSegmentCacheStateV85.Dirty = true
	localAudioSegmentCacheStateV85.Generation++
	localAudioSegmentCacheStateV85.Unlock()
	pruneLocalAudioSegmentCacheV85()
	return nil
}

func flushLocalAudioSegmentCacheV85(a *App) error {
	ensureLocalAudioSegmentCacheLoadedV85(a)
	localAudioSegmentCacheStateV85.SaveMu.Lock()
	defer localAudioSegmentCacheStateV85.SaveMu.Unlock()

	localAudioSegmentCacheStateV85.Lock()
	if !localAudioSegmentCacheStateV85.Dirty {
		localAudioSegmentCacheStateV85.Unlock()
		return nil
	}
	generation := localAudioSegmentCacheStateV85.Generation
	rows := make(map[string]localAudioSegmentCacheEntryV85, len(localAudioSegmentCacheStateV85.Entries))
	for key, row := range localAudioSegmentCacheStateV85.Entries {
		row.Fingerprint = append([]uint32(nil), row.Fingerprint...)
		rows[key] = row
	}
	localAudioSegmentCacheStateV85.Unlock()

	b, err := json.Marshal(rows)
	if err != nil {
		return err
	}
	path := localAudioSegmentCacheFileV85(a)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0644); err != nil {
		return err
	}
	if err := replaceCacheFileV85(tmp, path); err != nil {
		return err
	}
	localAudioSegmentCacheStateV85.Lock()
	if localAudioSegmentCacheStateV85.Generation == generation {
		localAudioSegmentCacheStateV85.Dirty = false
	}
	localAudioSegmentCacheStateV85.Unlock()
	return nil
}

func (a *App) cachedLocalChromaprintSegmentV85(ctx context.Context, ff, path string, start, seconds float64) ([]uint32, error) {
	if fp, ok := cachedLocalAudioSegmentV85(a, path, start, seconds); ok {
		return fp, nil
	}
	fp, err := chromaprintSegmentV85(ctx, ff, path, start, seconds)
	if err != nil {
		return nil, err
	}
	_ = cacheLocalAudioSegmentV85(a, path, start, seconds, fp)
	return fp, nil
}
