package main

import (
	"os"
	"path/filepath"
	"strings"
)

const smartMediaCacheSchemaV85 = "v8.5-media-schema-3"

func init() {
	// Tests use isolated temporary App directories; never mutate files next to
	// the Go test executable merely because the package was imported.
	base := strings.ToLower(filepath.Base(os.Args[0]))
	if strings.HasSuffix(base, ".test") || strings.HasSuffix(base, ".test.exe") {
		return
	}
	ensureSmartMediaCacheSchemaV85()
}

func ensureSmartMediaCacheSchemaV85() {
	dataDir := filepath.Join(executableDir(), "data")
	marker := filepath.Join(dataDir, "smart_media_cache_schema.txt")
	if b, err := os.ReadFile(marker); err == nil && strings.TrimSpace(string(b)) == smartMediaCacheSchemaV85 {
		return
	}

	// The fingerprint/signature meaning is algorithm-specific. Reusing an older
	// cache after frame positions, filtering or image signature logic changes is
	// worse than rebuilding it once, because it can create false verdicts.
	for _, name := range []string{
		"video_fingerprint_cache.json",
		"image_signature_cache.json",
		"media_meta_cache.json",
	} {
		_ = os.Remove(filepath.Join(dataDir, name))
	}
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return
	}
	tmp := marker + ".tmp"
	if os.WriteFile(tmp, []byte(smartMediaCacheSchemaV85+"\n"), 0644) == nil {
		_ = replaceCacheFileV85(tmp, marker)
	}
}
