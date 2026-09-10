package main

import (
	"context"
	"math"
	"sort"
)

type detectorCandidateKeyV90 struct{}
type detectorCandidateViewV90 struct{}
type durationEntryV90 struct {
	Entry    FileEntry
	Duration float64
}
type detectorCandidatesV90 struct {
	Entries       []FileEntry
	Videos        []durationEntryV90
	Unknown       []FileEntry
	Bands         map[uint16][]int
	ImageBands    map[uint16][]int
	UnknownImages []FileEntry
}

// Build once per source/preflight, not once for each remote row. Size buckets
// remain the exact stage; sorted durations and pHash bands drive the media stage.
func detectorCandidateContextV90(ctx context.Context, a *App, entries []FileEntry) context.Context {
	x := &detectorCandidatesV90{Entries: entries, Bands: map[uint16][]int{}, ImageBands: map[uint16][]int{}}
	pruneLocalMediaMetaCache(a, entries)
	pruneLocalImageSignatureCacheV85(a, entries)
	for entryID, e := range entries {
		if remoteMediaKind(e.Name) == "image" {
			if sig, ok := cachedLocalImageSignatureV85(a, e); ok && sig.Version == signatureVersionV90 {
				for band := 0; band < 8; band++ {
					key := uint16(band*256) + uint16(byte(sig.PHash>>uint(band*8)))
					x.ImageBands[key] = append(x.ImageBands[key], entryID)
				}
			} else if !cachedLocalImageFailureV85(a, e) {
				x.UnknownImages = append(x.UnknownImages, e)
			}
			continue
		}
		if remoteMediaKind(e.Name) != "video" {
			continue
		}
		if info, ok := cachedLocalMediaInfo(a, e); ok {
			x.Videos = append(x.Videos, durationEntryV90{e, info.Duration})
		} else if !cachedLocalMediaFailureV85(a, e) {
			x.Unknown = append(x.Unknown, e)
		}
		if fp, ok := cachedLocalVideoFingerprintV85(a, e); ok && richVideoFingerprintUsableV90(fp) {
			seen := map[uint16]bool{}
			for i, sig := range fp.Frames {
				if !fp.Valid[i] {
					continue
				}
				for band := 0; band < 8; band++ {
					key := uint16(band*256) + uint16(byte(sig.PHash>>uint(band*8)))
					if !seen[key] {
						x.Bands[key] = append(x.Bands[key], entryID)
						seen[key] = true
					}
				}
			}
		}
	}
	sort.Slice(x.Videos, func(i, j int) bool { return x.Videos[i].Duration < x.Videos[j].Duration })
	return context.WithValue(ctx, detectorCandidateKeyV90{}, x)
}

func (a *App) videoCandidatesV90(ctx context.Context, fp videoFingerprintV85, remote RemoteItem, entries []FileEntry) videoDurationSearchV85 {
	x, ok := ctx.Value(detectorCandidateKeyV90{}).(*detectorCandidatesV90)
	if !ok {
		ctx = detectorCandidateContextV90(ctx, a, entries)
		x = ctx.Value(detectorCandidateKeyV90{}).(*detectorCandidatesV90)
	}
	// Permit small cuts on short clips as well as 12% duration changes on longer
	// material. Duration/codec/size are only filters, never a duplicate verdict.
	d := fp.Info.Duration
	lo := sort.Search(len(x.Videos), func(i int) bool { return x.Videos[i].Duration >= math.Min(d*.88, d-3) })
	hi := sort.Search(len(x.Videos), func(i int) bool { return x.Videos[i].Duration > math.Max(d/.88, d+3) })
	pool := append([]FileEntry(nil), x.Unknown...)
	for _, row := range x.Videos[lo:hi] {
		pool = append(pool, row.Entry)
	}
	search := a.videoDurationCandidatesCached(context.WithValue(ctx, detectorCandidateViewV90{}, true), fp.Info, remote, pool, nil, 64)
	votes := map[string]int{}
	byPath := map[string]FileEntry{}
	// Multi-probe 8-bit bands retain pHashes with up to 15 differing bits without
	// comparing every fingerprint to every other fingerprint.
	for i, sig := range fp.Frames {
		if !fp.Valid[i] {
			continue
		}
		for band := 0; band < 8; band++ {
			value := byte(sig.PHash >> uint(band*8))
			for bit := -1; bit < 8; bit++ {
				v := value
				if bit >= 0 {
					v ^= 1 << uint(bit)
				}
				for _, entryID := range x.Bands[uint16(band*256)+uint16(v)] {
					e := x.Entries[entryID]
					info, ok := cachedLocalMediaInfo(a, e)
					if !ok {
						continue
					}
					if _, compatible := durationCompatibleV85(fp.Info, info); !compatible {
						continue
					}
					key := pathKey(e.Path)
					votes[key]++
					byPath[key] = e
				}
			}
		}
	}
	for _, e := range search.Candidates {
		byPath[pathKey(e.Path)] = e
	}
	all := make([]FileEntry, 0, len(byPath))
	for _, e := range byPath {
		all = append(all, e)
	}
	sort.Slice(all, func(i, j int) bool {
		vi, vj := votes[pathKey(all[i].Path)], votes[pathKey(all[j].Path)]
		if vi != vj {
			return vi > vj
		}
		return all[i].Path < all[j].Path
	})
	// Cheap fingerprint comparison ranks already indexed content before the
	// costly decode budget. The name can never hide a cached renamed match.
	type ranked struct {
		e     FileEntry
		score int
	}
	scored := []ranked{}
	uncached := []FileEntry{}
	for i, e := range all {
		if i >= 512 {
			search.Pending += len(all) - i
			break
		}
		if local, ok := cachedLocalVideoFingerprintV85(a, e); ok && richVideoFingerprintUsableV90(local) {
			score, _, _, _ := scoreRichFrameSetV90(fp, local)
			scored = append(scored, ranked{e, score})
		} else {
			uncached = append(uncached, e)
		}
	}
	sort.SliceStable(scored, func(i, j int) bool { return scored[i].score > scored[j].score })
	search.Candidates = nil
	for _, row := range scored {
		if len(search.Candidates) < 8 {
			search.Candidates = append(search.Candidates, row.e)
		} else if row.score >= 85 {
			search.Pending++
		}
	}
	for i, e := range uncached {
		if len(search.Candidates) >= 12 {
			search.Pending += len(uncached) - i
			break
		}
		search.Candidates = append(search.Candidates, e)
	}
	return search
}
