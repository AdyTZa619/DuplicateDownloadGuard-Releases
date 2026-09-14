package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

type adaptiveVideoEvidenceV901 struct {
	Score   int
	Matched int
	Total   int
	Note    string
}

type adaptiveVideoPairCacheEntryV901 struct {
	LocalSize     int64                     `json:"localSize"`
	LocalMTime    int64                     `json:"localMtime"`
	LocalIdentity string                    `json:"localIdentity"`
	Evidence      adaptiveVideoEvidenceV901 `json:"evidence"`
}

type adaptiveVideoPairCacheV901 struct {
	sync.Mutex
	Loaded bool
	Rows   map[string]adaptiveVideoPairCacheEntryV901
}

var adaptiveVideoPairRegistryV901 sync.Map // *App -> *adaptiveVideoPairCacheV901

func adaptiveVideoPairStateV901(a *App) *adaptiveVideoPairCacheV901 {
	if raw, ok := adaptiveVideoPairRegistryV901.Load(a); ok {
		return raw.(*adaptiveVideoPairCacheV901)
	}
	state := &adaptiveVideoPairCacheV901{Rows: map[string]adaptiveVideoPairCacheEntryV901{}}
	actual, _ := adaptiveVideoPairRegistryV901.LoadOrStore(a, state)
	return actual.(*adaptiveVideoPairCacheV901)
}

func adaptiveVideoPairPathV901(a *App) string {
	return filepath.Join(a.appDir, "adaptive_video_pairs_v901.json")
}

func adaptiveRemoteKeyV901(ctx context.Context, localPath string) (string, bool) {
	session, ok := ctx.Value(detectorRemoteKeyV90{}).(*detectorRemoteV90)
	if !ok {
		return "", false
	}
	session.mu.Lock()
	original, validator, size := session.Original, session.Validator, session.Size
	session.mu.Unlock()
	if original == "" || validator == "" || size <= 0 {
		return "", false
	}
	sum := sha256.Sum256([]byte(original + "\n" + validator + "\n" + fmt.Sprint(size) + "\n" + filepath.Clean(localPath)))
	return hex.EncodeToString(sum[:]), true
}

func adaptiveLocalIdentityV901(path string) (size, mtime int64, identity string, ok bool) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, "", false
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return 0, 0, "", false
	}
	identity, cacheable := hashFileIdentityV90(f, info)
	return info.Size(), info.ModTime().UnixNano(), identity, cacheable && identity != ""
}

func loadAdaptiveVideoPairCacheV901(a *App, state *adaptiveVideoPairCacheV901) {
	if state.Loaded {
		return
	}
	state.Loaded = true
	var rows map[string]adaptiveVideoPairCacheEntryV901
	if b, err := os.ReadFile(adaptiveVideoPairPathV901(a)); err == nil && json.Unmarshal(b, &rows) == nil && rows != nil {
		state.Rows = rows
	}
}

func cachedAdaptiveVideoEvidenceV901(a *App, ctx context.Context, localPath string) (adaptiveVideoEvidenceV901, bool) {
	key, ok := adaptiveRemoteKeyV901(ctx, localPath)
	if !ok {
		return adaptiveVideoEvidenceV901{}, false
	}
	size, mtime, identity, ok := adaptiveLocalIdentityV901(localPath)
	if !ok {
		return adaptiveVideoEvidenceV901{}, false
	}
	state := adaptiveVideoPairStateV901(a)
	state.Lock()
	defer state.Unlock()
	loadAdaptiveVideoPairCacheV901(a, state)
	entry, ok := state.Rows[key]
	return entry.Evidence, ok && entry.LocalSize == size && entry.LocalMTime == mtime && entry.LocalIdentity == identity
}

func saveAdaptiveVideoEvidenceV901(a *App, ctx context.Context, localPath string, evidence adaptiveVideoEvidenceV901) {
	key, ok := adaptiveRemoteKeyV901(ctx, localPath)
	if !ok || evidence.Total <= 7 {
		return
	}
	size, mtime, identity, ok := adaptiveLocalIdentityV901(localPath)
	if !ok {
		return
	}
	state := adaptiveVideoPairStateV901(a)
	state.Lock()
	defer state.Unlock()
	loadAdaptiveVideoPairCacheV901(a, state)
	state.Rows[key] = adaptiveVideoPairCacheEntryV901{LocalSize: size, LocalMTime: mtime, LocalIdentity: identity, Evidence: evidence}
	b, err := json.Marshal(state.Rows)
	if err != nil {
		return
	}
	path := adaptiveVideoPairPathV901(a)
	tmp := path + ".tmp"
	if os.WriteFile(tmp, b, 0600) == nil {
		_ = replaceCacheFileV85(tmp, path)
	}
}

func adaptiveFramePointsV901() ([]float64, map[int]bool) {
	points := make([]float64, 25)
	for i := range points {
		points[i] = float64(i+1) / 26
	}
	first := map[int]bool{}
	for i := 0; i < 15; i++ {
		// Spread the first stage over the entire timeline. The second stage fills
		// the ten gaps, so 7 -> 15 -> 25 never decodes the same frame twice.
		first[int(math.Round(float64(i)*24/14))] = true
	}
	return points, first
}

func collectAdaptiveFramesV901(ctx context.Context, ff, target string, duration float64, points []float64, wanted map[int]bool, cached map[int]imageSignatureV85) map[int]imageSignatureV85 {
	if cached == nil {
		cached = map[int]imageSignatureV85{}
	}
	keys := make([]int, 0, len(wanted))
	for i := range wanted {
		keys = append(keys, i)
	}
	sort.Ints(keys)
	for _, i := range keys {
		if _, ok := cached[i]; ok || i < 0 || i >= len(points) || ctx.Err() != nil {
			continue
		}
		sig, informative, err := videoFrameSignatureV90(ctx, ff, target, duration*points[i])
		if err == nil && informative {
			cached[i] = sig
		}
	}
	return cached
}

func scoreAdaptiveFramesV901(remote, local map[int]imageSignatureV85, total int) (score, matched, high, veryHigh int) {
	for i, a := range remote {
		b, ok := local[i]
		if !ok {
			continue
		}
		value := richSignatureSimilarityV90(a, b)
		score += value
		matched++
		if value >= 90 {
			high++
		}
		if value >= 95 {
			veryHigh++
		}
	}
	if matched == 0 {
		return 0, 0, 0, 0
	}
	score = int(math.Round(float64(score) / float64(matched)))
	if matched < (total+1)/2 {
		score = min(score, 84)
	}
	if high*2 < matched && score > 88 {
		score = 88
	}
	if veryHigh*2 < matched && score > 93 {
		score = 93
	}
	return score, matched, high, veryHigh
}

// refineVideoEvidenceV901 spends extra decoder work only after the seven-frame
// stage is still ambiguous. It expands to 15 points and, if needed, to 25.
func (a *App) refineVideoEvidenceV901(ctx context.Context, remoteTarget, localPath string, remoteInfo, localInfo MediaInfo, baseScore int) adaptiveVideoEvidenceV901 {
	out := adaptiveVideoEvidenceV901{Score: baseScore, Total: 7}
	if baseScore < 85 || baseScore >= 100 || remoteInfo.Duration <= 0 || localInfo.Duration <= 0 {
		return out
	}
	ff := a.detectFFmpeg()
	if ff == "" {
		return out
	}
	if cached, ok := cachedAdaptiveVideoEvidenceV901(a, ctx, localPath); ok {
		cached.Note += " • cache adaptiv"
		return cached
	}
	points, first := adaptiveFramePointsV901()
	remoteFrames := collectAdaptiveFramesV901(ctx, ff, remoteTarget, remoteInfo.Duration, points, first, nil)
	localFrames := collectAdaptiveFramesV901(ctx, ff, localPath, localInfo.Duration, points, first, nil)
	score, matched, _, _ := scoreAdaptiveFramesV901(remoteFrames, localFrames, 15)
	if matched < 8 || ctx.Err() != nil {
		return out
	}
	out = adaptiveVideoEvidenceV901{Score: score, Matched: matched, Total: 15, Note: fmt.Sprintf("analiză adaptivă: %d/15 cadre compatibile", matched)}
	if score < 85 || score >= 98 {
		saveAdaptiveVideoEvidenceV901(a, ctx, localPath, out)
		return out
	}
	all := make(map[int]bool, 25)
	for i := range points {
		all[i] = true
	}
	remoteFrames = collectAdaptiveFramesV901(ctx, ff, remoteTarget, remoteInfo.Duration, points, all, remoteFrames)
	localFrames = collectAdaptiveFramesV901(ctx, ff, localPath, localInfo.Duration, points, all, localFrames)
	score, matched, _, _ = scoreAdaptiveFramesV901(remoteFrames, localFrames, 25)
	if matched >= 13 && ctx.Err() == nil {
		out = adaptiveVideoEvidenceV901{Score: score, Matched: matched, Total: 25, Note: fmt.Sprintf("analiză adaptivă: %d/25 cadre compatibile", matched)}
	}
	saveAdaptiveVideoEvidenceV901(a, ctx, localPath, out)
	return out
}
