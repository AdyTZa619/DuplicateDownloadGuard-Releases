package main

import (
	"hash/fnv"
	"path/filepath"
	"strings"
	"testing"
)

func TestCanonicalSourceURLV85124(t *testing.T) {
	got, err := canonicalSourceURLV85124("HTTPS://WWW.Example.COM:443/path/?utm_source=x&b=2&a=1#section")
	if err != nil {
		t.Fatal(err)
	}
	want := "https://example.com/path?a=1&b=2"
	if got != want {
		t.Fatalf("canonical URL = %q, want %q", got, want)
	}
}

func TestCanonicalSourceURLV85124KeepsMegaKey(t *testing.T) {
	raw := "https://mega.nz/folder/FJRUSC7a#8eBmnE0x6QVa_ILoTLoBAg"
	got, err := canonicalSourceURLV85124(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got != raw {
		t.Fatalf("MEGA canonical URL = %q, want %q", got, raw)
	}
}

func TestSourceHistoryPortSeedV85124Deterministic(t *testing.T) {
	dir := filepath.Clean(`C:\\Users\\AdY\\DDG\\data`)
	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.ToLower(dir)))
	want := sourceHistoryPortBaseV85124 + int(h.Sum32()%sourceHistoryPortSpanV85124)
	if got := sourceHistoryPortSeedV85124(dir); got != want {
		t.Fatalf("port seed = %d, want %d", got, want)
	}
}

func TestSourceHistoryCountClampV85124(t *testing.T) {
	if got := clampSourceHistoryCountV85124(-2, 10); got != 0 {
		t.Fatalf("negative clamp = %d", got)
	}
	if got := clampSourceHistoryCountV85124(14, 10); got != 10 {
		t.Fatalf("high clamp = %d", got)
	}
	if got := clampSourceHistoryCountV85124(7, 10); got != 7 {
		t.Fatalf("normal clamp = %d", got)
	}
}
