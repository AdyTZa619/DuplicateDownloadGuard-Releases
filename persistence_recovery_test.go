package main

import (
	"compress/gzip"
	"encoding/gob"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func writeTestResultsV85(t *testing.T, path string, rows []Result) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	if err := json.NewEncoder(gz).Encode(rows); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func writeTestIndexV85(t *testing.T, path string, rows map[string]FileEntry) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	if err := gob.NewEncoder(gz).Encode(rows); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestPersistenceRecoveryPrefersValidTmpOverBackup(t *testing.T) {
	dir := t.TempDir()
	primary := filepath.Join(dir, "config.json")
	if err := os.WriteFile(primary, []byte(`{"broken":`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(primary+".tmp", []byte(`{"mode":"new"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(primary+".good.bak", []byte(`{"mode":"old"}`), 0600); err != nil {
		t.Fatal(err)
	}
	recovered := recoverPersistenceDirV85(dir)
	if len(recovered) != 1 || recovered[0] != "config.json" {
		t.Fatalf("unexpected recovery list: %#v", recovered)
	}
	b, err := os.ReadFile(primary)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"mode":"new"}` {
		t.Fatalf("valid tmp should win over older backup, got %s", b)
	}
}

func TestPersistenceRecoveryUsesBackupWhenPrimaryAndTmpAreBroken(t *testing.T) {
	dir := t.TempDir()
	primary := filepath.Join(dir, "last_results.json.gz")
	if err := os.WriteFile(primary, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(primary+".tmp", []byte("also broken"), 0600); err != nil {
		t.Fatal(err)
	}
	want := []Result{{ID: 9, Remote: RemoteItem{Name: "kept.mp4", Size: 123}}}
	writeTestResultsV85(t, primary+".good.bak", want)

	recovered := recoverPersistenceDirV85(dir)
	if len(recovered) != 1 || recovered[0] != "last_results.json.gz" {
		t.Fatalf("unexpected recovery list: %#v", recovered)
	}
	if err := validateResultsFileV85(primary); err != nil {
		t.Fatalf("recovered results are invalid: %v", err)
	}
}

func TestPersistenceRecoveryLeavesValidPrimaryUntouched(t *testing.T) {
	dir := t.TempDir()
	primary := filepath.Join(dir, "manual_decisions.json")
	if err := os.WriteFile(primary, []byte(`{"remote":"HAVE"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(primary+".good.bak", []byte(`{"remote":"MISSING"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if recovered := recoverPersistenceDirV85(dir); len(recovered) != 0 {
		t.Fatalf("valid primary must not be replaced: %#v", recovered)
	}
	b, _ := os.ReadFile(primary)
	if string(b) != `{"remote":"HAVE"}` {
		t.Fatalf("primary changed unexpectedly: %s", b)
	}
}

func TestPersistenceSnapshotNeverReplacesGoodBackupWithCorruptPrimary(t *testing.T) {
	dir := t.TempDir()
	primary := filepath.Join(dir, "index.gob.gz")
	backup := primary + ".good.bak"
	writeTestIndexV85(t, primary, map[string]FileEntry{"A": {Path: "A", Name: "A.mp4", Size: 10, MTime: 1}})
	stamps := map[string]persistenceStampV85{}
	snapshotPersistenceDirV85(dir, stamps)
	if err := validateIndexFileV85(backup); err != nil {
		t.Fatalf("initial backup invalid: %v", err)
	}
	before, err := os.ReadFile(backup)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(primary, []byte("truncated"), 0600); err != nil {
		t.Fatal(err)
	}
	snapshotPersistenceDirV85(dir, stamps)
	after, err := os.ReadFile(backup)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("corrupt primary must never overwrite known-good backup")
	}
}
