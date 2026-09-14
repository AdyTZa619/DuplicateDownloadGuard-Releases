package main

import "sort"

func (x *detectorCandidatesV90) imagePool(a *App, sig imageSignatureV85, existing []FileEntry) ([]FileEntry, int) {
	byPath := map[string]FileEntry{}
	votes := map[string]int{}
	for _, e := range existing {
		byPath[pathKey(e.Path)] = e
	}
	// Two-bit multi-probe across eight bands guarantees retrieval when no more
	// than 23 pHash bits differ. The final structural gates are much stricter.
	for band := 0; band < 8; band++ {
		v := byte(sig.PHash >> uint(band*8))
		probe := func(value byte) {
			for _, entryID := range x.ImageBands[uint16(band*256)+uint16(value)] {
				e := x.Entries[entryID]
				key := pathKey(e.Path)
				byPath[key] = e
				votes[key]++
			}
		}
		probe(v)
		for i := 0; i < 8; i++ {
			probe(v ^ (1 << uint(i)))
			for j := i + 1; j < 8; j++ {
				probe(v ^ (1 << uint(i)) ^ (1 << uint(j)))
			}
		}
	}
	// Entries that were uncached when the per-source index was built may have
	// gained a signature during an earlier progressive pass. Compare them from
	// memory now so the next uncached batch advances instead of repeating the
	// same first 64 files.
	for _, e := range x.UnknownImages {
		if local, ok := cachedLocalImageSignatureV85(a, e); ok {
			key := pathKey(e.Path)
			byPath[key] = e
			votes[key] += max(1, imageSignatureSimilarityV85(sig, local)/10)
		}
	}
	out := make([]FileEntry, 0, len(byPath))
	for _, e := range byPath {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := votes[pathKey(out[i].Path)], votes[pathKey(out[j].Path)]
		if a != b {
			return a > b
		}
		return out[i].Path < out[j].Path
	})
	pending := 0
	uncachedAdded := 0
	for _, e := range x.UnknownImages {
		if _, ok := cachedLocalImageSignatureV85(a, e); ok || cachedLocalImageFailureV85(a, e) {
			continue
		}
		if uncachedAdded >= 64 {
			pending++
			continue
		}
		if !hasEntryPath(out, e.Path) {
			out = append(out, e)
			uncachedAdded++
		}
	}
	return out, pending
}
