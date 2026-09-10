package main

import "sort"

func (x *detectorCandidatesV90) imagePool(sig imageSignatureV85, existing []FileEntry) ([]FileEntry, int) {
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
	if len(out) > 512 {
		pending += len(out) - 512
		out = out[:512]
	}
	for i, e := range x.UnknownImages {
		if i >= 64 {
			pending += len(x.UnknownImages) - i
			break
		}
		if !hasEntryPath(out, e.Path) {
			out = append(out, e)
		}
	}
	return out, pending
}
