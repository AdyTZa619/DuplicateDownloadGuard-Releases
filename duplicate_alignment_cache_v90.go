package main

func cachedAlignedFingerprintV90(a *App, e FileEntry, key string) (videoFingerprintV85, bool) {
	ensureLocalVideoFingerprintCacheLoaded(a)
	localVideoFingerprintCacheState.Lock()
	defer localVideoFingerprintCacheState.Unlock()
	row, ok := localVideoFingerprintCacheState.Entries[pathKey(e.Path)]
	if !ok || row.Size != e.Size || row.MTime != e.MTime {
		return videoFingerprintV85{}, false
	}
	fp, ok := row.Aligned[key]
	return fp, ok && richVideoFingerprintUsableV90(fp)
}
func cacheAlignedFingerprintV90(a *App, e FileEntry, key string, fp videoFingerprintV85) {
	if !richVideoFingerprintUsableV90(fp) {
		return
	}
	ensureLocalVideoFingerprintCacheLoaded(a)
	localVideoFingerprintCacheState.Lock()
	defer localVideoFingerprintCacheState.Unlock()
	row, ok := localVideoFingerprintCacheState.Entries[pathKey(e.Path)]
	if !ok || row.Size != e.Size || row.MTime != e.MTime {
		return
	}
	// Copy-on-write keeps the JSON save snapshot race-free.
	align := map[string]videoFingerprintV85{}
	if len(row.Aligned) < 8 {
		for k, v := range row.Aligned {
			align[k] = v
		}
	}
	align[key] = fp
	row.Aligned = align
	localVideoFingerprintCacheState.Entries[pathKey(e.Path)] = row
	localVideoFingerprintCacheState.Dirty = true
	localVideoFingerprintCacheState.Generation++
}
