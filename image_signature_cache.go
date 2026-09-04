package main

import (
	"context"
	"encoding/json"
	"image"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

type imageSignatureV85 struct {
	Hash    uint64  `json:"hash"`
	AvgR    uint8   `json:"avgR"`
	AvgG    uint8   `json:"avgG"`
	AvgB    uint8   `json:"avgB"`
	LumaStd float64 `json:"lumaStd"`
}

func makeImageSignatureV85(img image.Image) imageSignatureV85 {
	b := img.Bounds()
	if b.Dx() <= 0 || b.Dy() <= 0 {
		return imageSignatureV85{Hash: dhashImage(img)}
	}
	const grid = 16
	var sumR, sumG, sumB float64
	lumas := make([]float64, 0, grid*grid)
	for gy := 0; gy < grid; gy++ {
		y := b.Min.Y + (2*gy+1)*b.Dy()/(2*grid)
		if y >= b.Max.Y {
			y = b.Max.Y - 1
		}
		for gx := 0; gx < grid; gx++ {
			x := b.Min.X + (2*gx+1)*b.Dx()/(2*grid)
			if x >= b.Max.X {
				x = b.Max.X - 1
			}
			r16, g16, b16, _ := img.At(x, y).RGBA()
			r := float64(r16 >> 8)
			g := float64(g16 >> 8)
			bl := float64(b16 >> 8)
			sumR += r
			sumG += g
			sumB += bl
			lumas = append(lumas, .2126*r+.7152*g+.0722*bl)
		}
	}
	n := float64(len(lumas))
	meanL := 0.0
	for _, v := range lumas {
		meanL += v
	}
	meanL /= n
	variance := 0.0
	for _, v := range lumas {
		d := v - meanL
		variance += d * d
	}
	variance /= n
	return imageSignatureV85{
		Hash:    dhashImage(img),
		AvgR:    uint8(math.Round(sumR / n)),
		AvgG:    uint8(math.Round(sumG / n)),
		AvgB:    uint8(math.Round(sumB / n)),
		LumaStd: math.Sqrt(variance),
	}
}

func imageSignatureSimilarityV85(a, b imageSignatureV85) int {
	hashScore := imageHashSimilarityV85(a.Hash, b.Hash)
	dr := float64(int(a.AvgR) - int(b.AvgR))
	dg := float64(int(a.AvgG) - int(b.AvgG))
	db := float64(int(a.AvgB) - int(b.AvgB))
	colorDistance := math.Sqrt(dr*dr + dg*dg + db*db)
	colorScore := 100 - colorDistance*100/math.Sqrt(3*255*255)
	if colorScore < 0 {
		colorScore = 0
	}
	textureScore := 100 - math.Min(100, math.Abs(a.LumaStd-b.LumaStd)*4)
	var score float64
	if a.LumaStd < 6 || b.LumaStd < 6 {
		// dHash carries very little information on flat/near-black frames.
		score = .25*float64(hashScore) + .65*colorScore + .10*textureScore
	} else {
		score = .72*float64(hashScore) + .18*colorScore + .10*textureScore
	}
	if score < 0 {
		return 0
	}
	if score > 100 {
		return 100
	}
	return int(math.Round(score))
}

type localImageSignatureCacheEntry struct {
	Size      int64             `json:"size"`
	MTime     int64             `json:"mtime"`
	Signature imageSignatureV85 `json:"signature"`
	Unusable  bool              `json:"unusable,omitempty"`
	Error     string            `json:"error,omitempty"`
}

var localImageSignatureCacheState = struct {
	sync.Mutex
	SaveMu     sync.Mutex
	AppDir     string
	Loaded     bool
	Dirty      bool
	Generation uint64
	Entries    map[string]localImageSignatureCacheEntry
}{}

func localImageSignatureCacheFile(a *App) string {
	return filepath.Join(a.appDir, "image_signature_cache.json")
}

func ensureLocalImageSignatureCacheLoaded(a *App) {
	localImageSignatureCacheState.Lock()
	defer localImageSignatureCacheState.Unlock()
	appDir := filepath.Clean(a.appDir)
	if localImageSignatureCacheState.Loaded && localImageSignatureCacheState.AppDir == appDir {
		return
	}
	localImageSignatureCacheState.AppDir = appDir
	localImageSignatureCacheState.Loaded = true
	localImageSignatureCacheState.Dirty = false
	localImageSignatureCacheState.Generation = 0
	localImageSignatureCacheState.Entries = map[string]localImageSignatureCacheEntry{}
	b, err := os.ReadFile(localImageSignatureCacheFile(a))
	if err != nil {
		return
	}
	var rows map[string]localImageSignatureCacheEntry
	if json.Unmarshal(b, &rows) == nil && rows != nil {
		localImageSignatureCacheState.Entries = rows
	}
}

func cachedLocalImageSignatureV85(a *App, e FileEntry) (imageSignatureV85, bool) {
	ensureLocalImageSignatureCacheLoaded(a)
	localImageSignatureCacheState.Lock()
	defer localImageSignatureCacheState.Unlock()
	row, ok := localImageSignatureCacheState.Entries[pathKey(e.Path)]
	if !ok || row.Size != e.Size || row.MTime != e.MTime || row.Unusable {
		return imageSignatureV85{}, false
	}
	return row.Signature, true
}

func cachedLocalImageFailureV85(a *App, e FileEntry) bool {
	ensureLocalImageSignatureCacheLoaded(a)
	localImageSignatureCacheState.Lock()
	defer localImageSignatureCacheState.Unlock()
	row, ok := localImageSignatureCacheState.Entries[pathKey(e.Path)]
	return ok && row.Size == e.Size && row.MTime == e.MTime && row.Unusable
}

func cacheLocalImageSignatureV85(a *App, e FileEntry, sig imageSignatureV85) {
	ensureLocalImageSignatureCacheLoaded(a)
	localImageSignatureCacheState.Lock()
	localImageSignatureCacheState.Entries[pathKey(e.Path)] = localImageSignatureCacheEntry{Size: e.Size, MTime: e.MTime, Signature: sig}
	localImageSignatureCacheState.Dirty = true
	localImageSignatureCacheState.Generation++
	localImageSignatureCacheState.Unlock()
}

func cacheLocalImageFailureV85(a *App, e FileEntry, reason string) {
	ensureLocalImageSignatureCacheLoaded(a)
	reason = strings.TrimSpace(reason)
	if len(reason) > 300 {
		reason = reason[:300]
	}
	localImageSignatureCacheState.Lock()
	localImageSignatureCacheState.Entries[pathKey(e.Path)] = localImageSignatureCacheEntry{Size: e.Size, MTime: e.MTime, Unusable: true, Error: reason}
	localImageSignatureCacheState.Dirty = true
	localImageSignatureCacheState.Generation++
	localImageSignatureCacheState.Unlock()
}

func pruneLocalImageSignatureCacheV85(a *App, entries []FileEntry) bool {
	ensureLocalImageSignatureCacheLoaded(a)
	valid := make(map[string]FileEntry, len(entries))
	for _, e := range entries {
		if remoteMediaKind(e.Name) == "image" {
			valid[pathKey(e.Path)] = e
		}
	}
	changed := false
	localImageSignatureCacheState.Lock()
	for key, cached := range localImageSignatureCacheState.Entries {
		e, ok := valid[key]
		if !ok || e.Size != cached.Size || e.MTime != cached.MTime {
			delete(localImageSignatureCacheState.Entries, key)
			changed = true
		}
	}
	if changed {
		localImageSignatureCacheState.Dirty = true
		localImageSignatureCacheState.Generation++
	}
	localImageSignatureCacheState.Unlock()
	return changed
}

func saveLocalImageSignatureCacheV85(a *App) error {
	ensureLocalImageSignatureCacheLoaded(a)
	localImageSignatureCacheState.SaveMu.Lock()
	defer localImageSignatureCacheState.SaveMu.Unlock()

	localImageSignatureCacheState.Lock()
	if !localImageSignatureCacheState.Dirty {
		localImageSignatureCacheState.Unlock()
		return nil
	}
	generation := localImageSignatureCacheState.Generation
	rows := make(map[string]localImageSignatureCacheEntry, len(localImageSignatureCacheState.Entries))
	for k, v := range localImageSignatureCacheState.Entries {
		rows[k] = v
	}
	localImageSignatureCacheState.Unlock()
	b, err := json.Marshal(rows)
	if err != nil {
		return err
	}
	path := localImageSignatureCacheFile(a)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0644); err != nil {
		return err
	}
	if err := replaceCacheFileV85(tmp, path); err != nil {
		return err
	}
	localImageSignatureCacheState.Lock()
	if localImageSignatureCacheState.Generation == generation {
		localImageSignatureCacheState.Dirty = false
	}
	localImageSignatureCacheState.Unlock()
	return nil
}

func readLocalImageSignatureV85(path string) (imageSignatureV85, error) {
	f, err := os.Open(path)
	if err != nil {
		return imageSignatureV85{}, err
	}
	defer f.Close()
	img, err := decodeImageForSignatureV85(f)
	if err != nil {
		return imageSignatureV85{}, err
	}
	return makeImageSignatureV85(img), nil
}

type imageCandidateSearchV85 struct {
	Candidates  []FileEntry
	BestPath    string
	BestScore   int
	SecondScore int
	Ambiguous   bool
	Pending     int
	Probed      int
	Cached      int
	Excluded    int
}

func updateImageBestV85(result *imageCandidateSearchV85, score int, path string) {
	if result == nil || path == "" {
		return
	}
	if score > result.BestScore {
		result.SecondScore = result.BestScore
		result.BestScore = score
		result.BestPath = path
		return
	}
	if pathKey(path) != pathKey(result.BestPath) && score > result.SecondScore {
		result.SecondScore = score
	}
}

func finalizeImageAmbiguityV85(result *imageCandidateSearchV85) {
	if result == nil {
		return
	}
	// Near-identical scores for two different local images are not enough to
	// choose a unique duplicate automatically. Keep the best candidate for UI,
	// but cap the confidence below the auto-block threshold.
	if result.BestScore >= 98 && result.SecondScore >= 96 && result.BestScore-result.SecondScore <= 1 {
		result.Ambiguous = true
		result.BestScore = 93
	}
}

type scoredImageCandidateV85 struct {
	Entry FileEntry
	Score int
}

func imageCandidateRoughRankV85(remote RemoteItem, e FileEntry) int {
	nameScore := nameSimilarity(remote.Name, e.Name)
	sizeRatio := 1.0
	if remote.Size > 0 {
		sizeRatio = float64(abs64(e.Size-remote.Size)) / float64(remote.Size)
	}
	closeness := int(math.Round(1200 / (1 + sizeRatio*5)))
	rank := closeness + nameScore*9
	if strings.EqualFold(filepathExt(remote.Name), filepathExt(e.Name)) {
		rank += 80
	}
	return rank
}

// imageCandidatesCachedV85 searches cached perceptual signatures across the
// whole image collection, not only similarly named/sized files. Files that are
// proven unreadable are cached as unusable until size/mtime changes, so one
// corrupt local image cannot keep unrelated remote images permanently blocked.
func (a *App) imageCandidatesCachedV85(ctx context.Context, remoteSig imageSignatureV85, remote RemoteItem, entries, existing []FileEntry, limit int) imageCandidateSearchV85 {
	if limit <= 0 {
		limit = 7
	}
	result := imageCandidateSearchV85{Candidates: append([]FileEntry(nil), existing...), BestScore: -1, SecondScore: -1}
	cacheChanged := pruneLocalImageSignatureCacheV85(a, entries)
	seen := map[string]bool{}
	for _, e := range existing {
		seen[pathKey(e.Path)] = true
		if cachedLocalImageFailureV85(a, e) {
			result.Cached++
			result.Excluded++
			continue
		}
		sig, ok := cachedLocalImageSignatureV85(a, e)
		if !ok {
			result.Probed++
			var err error
			sig, err = readLocalImageSignatureV85(e.Path)
			if ctx.Err() != nil {
				result.Pending++
				continue
			}
			if err != nil {
				cacheLocalImageFailureV85(a, e, err.Error())
				cacheChanged = true
				result.Excluded++
				continue
			}
			cacheLocalImageSignatureV85(a, e, sig)
			cacheChanged = true
		}
		score := imageSignatureSimilarityV85(remoteSig, sig)
		updateImageBestV85(&result, score, e.Path)
	}

	scored := make([]scoredImageCandidateV85, 0, 32)
	type rough struct {
		Entry FileEntry
		Rank  int
	}
	uncached := make([]rough, 0, 128)
	for _, e := range entries {
		if remoteMediaKind(e.Name) != "image" || seen[pathKey(e.Path)] {
			continue
		}
		if sig, ok := cachedLocalImageSignatureV85(a, e); ok {
			result.Cached++
			score := imageSignatureSimilarityV85(remoteSig, sig)
			updateImageBestV85(&result, score, e.Path)
			if score >= 80 {
				scored = append(scored, scoredImageCandidateV85{Entry: e, Score: score})
			}
			continue
		}
		if cachedLocalImageFailureV85(a, e) {
			result.Cached++
			result.Excluded++
			continue
		}
		uncached = append(uncached, rough{Entry: e, Rank: imageCandidateRoughRankV85(remote, e)})
	}
	sort.SliceStable(uncached, func(i, j int) bool {
		if uncached[i].Rank != uncached[j].Rank {
			return uncached[i].Rank > uncached[j].Rank
		}
		return strings.ToLower(uncached[i].Entry.Path) < strings.ToLower(uncached[j].Entry.Path)
	})

	const probeLimit = 64
	resolved := 0
	for i, row := range uncached {
		if i >= probeLimit || ctx.Err() != nil {
			break
		}
		result.Probed++
		sig, err := readLocalImageSignatureV85(row.Entry.Path)
		if ctx.Err() != nil {
			break
		}
		resolved++
		if err != nil {
			cacheLocalImageFailureV85(a, row.Entry, err.Error())
			cacheChanged = true
			result.Excluded++
			continue
		}
		cacheLocalImageSignatureV85(a, row.Entry, sig)
		cacheChanged = true
		score := imageSignatureSimilarityV85(remoteSig, sig)
		updateImageBestV85(&result, score, row.Entry.Path)
		if score >= 80 {
			scored = append(scored, scoredImageCandidateV85{Entry: row.Entry, Score: score})
		}
	}
	result.Pending += len(uncached) - resolved
	if result.Pending < 0 {
		result.Pending = 0
	}
	if cacheChanged {
		if err := saveLocalImageSignatureCacheV85(a); err != nil {
			a.logf("Smart Media Guard: nu am putut salva cache-ul de imagini: %v", err)
		}
	}

	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].Score != scored[j].Score {
			return scored[i].Score > scored[j].Score
		}
		return strings.ToLower(scored[i].Entry.Path) < strings.ToLower(scored[j].Entry.Path)
	})
	for _, row := range scored {
		if len(result.Candidates) >= limit {
			break
		}
		if !hasEntryPath(result.Candidates, row.Entry.Path) {
			result.Candidates = append(result.Candidates, row.Entry)
		}
	}
	finalizeImageAmbiguityV85(&result)
	return result
}
