package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestDownloadGuardReusesFreshCompareIndexV8545(t *testing.T) {
	collection, download := t.TempDir(), t.TempDir()
	data := []byte("fresh-index-content-v8545")
	local := filepath.Join(collection, "renamed-local.bin")
	if err := os.WriteFile(local, data, 0644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	remote := RemoteItem{
		Name: "remote.bin", Size: int64(len(data)), Source: "HTTP",
		HashType: "sha256", Hash: hex.EncodeToString(sum[:]),
	}
	a := guardTestApp(t, collection, download, remote)
	a.cfg.LiveRefreshCompare = true
	a.compareRemote(context.Background(), []RemoteItem{remote}, "balanced")

	report, err := a.runDownloadGuard(context.Background(), a.results, download, guardModeSmart)
	if err != nil {
		t.Fatal(err)
	}
	if !report.ReusedFreshIndex {
		t.Fatalf("expected fresh index reuse: %#v", report)
	}
	if report.IndexAgeMS < 0 || report.IndexAgeMS > guardFreshIndexTTLV8545.Milliseconds() {
		t.Fatalf("unexpected index age: %dms", report.IndexAgeMS)
	}
	if got := report.Decisions[0]; got.Verdict != guardDuplicate || got.LocalPath != local {
		t.Fatalf("unexpected decision: %#v", got)
	}
}
