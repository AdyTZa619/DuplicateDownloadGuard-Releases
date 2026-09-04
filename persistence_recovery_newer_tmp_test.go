package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPersistenceRecoveryPromotesNewerValidTmpOverValidPrimary(t *testing.T) {
	dir := t.TempDir()
	primary := filepath.Join(dir, "manual_decisions.json")
	pending := primary + ".tmp"
	if err := os.WriteFile(primary, []byte(`{"state":"old"}`), 0600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-5 * time.Second)
	if err := os.Chtimes(primary, old, old); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pending, []byte(`{"state":"new"}`), 0600); err != nil {
		t.Fatal(err)
	}
	newer := time.Now()
	if err := os.Chtimes(pending, newer, newer); err != nil {
		t.Fatal(err)
	}

	recovered := recoverPersistenceDirV85(dir)
	if len(recovered) != 1 || recovered[0] != "manual_decisions.json" {
		t.Fatalf("newer valid temp was not promoted: %#v", recovered)
	}
	b, err := os.ReadFile(primary)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"state":"new"}` {
		t.Fatalf("wrong recovered state: %s", b)
	}
	if _, err := os.Stat(pending); !os.IsNotExist(err) {
		t.Fatalf("promoted temp should be removed, stat err=%v", err)
	}
}

func TestPersistenceRecoveryDoesNotReplaceNewerPrimaryWithOlderTmp(t *testing.T) {
	dir := t.TempDir()
	primary := filepath.Join(dir, "manual_decisions.json")
	pending := primary + ".tmp"
	if err := os.WriteFile(pending, []byte(`{"state":"stale"}`), 0600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-5 * time.Second)
	if err := os.Chtimes(pending, old, old); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(primary, []byte(`{"state":"current"}`), 0600); err != nil {
		t.Fatal(err)
	}

	if recovered := recoverPersistenceDirV85(dir); len(recovered) != 0 {
		t.Fatalf("older temp must not replace valid primary: %#v", recovered)
	}
	b, _ := os.ReadFile(primary)
	if string(b) != `{"state":"current"}` {
		t.Fatalf("primary changed unexpectedly: %s", b)
	}
}
