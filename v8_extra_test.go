package main

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestVersionNewer(t *testing.T) {
	cases := []struct {
		r, l string
		want bool
	}{
		{"8.1.0", "8.0.0", true}, {"8.0.0", "8.0.0", false}, {"7.9.9", "8.0.0", false}, {"8.0.1 Pro", "8.0.0", true},
		{"8.5.33", "8.5.33-test.1 Pro Smart Media Guard", true},
		{"8.5.33-test.2", "8.5.33-test.1 Pro Smart Media Guard", true},
		{"8.5.33-test.1", "8.5.33", false},
		{"8.5.33", "8.5.33 Pro Smart Media Guard", false},
		{"invalid", "8.5.33", false},
	}
	for _, c := range cases {
		if got := versionNewer(c.r, c.l); got != c.want {
			t.Fatalf("versionNewer(%q,%q)=%v want %v", c.r, c.l, got, c.want)
		}
	}
}

func TestReleaseAsset(t *testing.T) {
	var r ghRelease
	r.TagName = "x"
	r.Assets = []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	}{{"a.txt", "u1"}, {"aria2-1.37-win-64bit-build1.zip", "u2"}}
	n, u, e := releaseAsset(r, `(?i)win-64bit.*\.zip$`)
	if e != nil || n == "" || u != "u2" {
		t.Fatalf("asset: %q %q %v", n, u, e)
	}
}

func TestQueueSummary(t *testing.T) {
	x := queueSummary([]DownloadJob{{Status: "queued", BytesDone: 10, BytesTotal: 100}, {Status: "completed", BytesDone: 50, BytesTotal: 50}})
	c := x["counts"].(map[string]int)
	if c["queued"] != 1 || c["completed"] != 1 {
		t.Fatalf("counts=%v", c)
	}
	if x["bytesDone"].(int64) != 60 || x["bytesTotal"].(int64) != 150 {
		t.Fatalf("summary=%v", x)
	}
}

func TestValidPE64Marker(t *testing.T) {
	p := filepath.Join(t.TempDir(), "ddg.exe")
	b := make([]byte, 1024)
	copy(b[:2], []byte("MZ"))
	binary.LittleEndian.PutUint32(b[0x3c:0x40], 0x80)
	copy(b[0x80:0x84], []byte{'P', 'E', 0, 0})
	binary.LittleEndian.PutUint16(b[0x84:0x86], 0x8664)
	copy(b[0x100:], []byte("Duplicate Download Guard"))
	if err := os.WriteFile(p, b, 0644); err != nil {
		t.Fatal(err)
	}
	if err := validPE64(p); err != nil {
		t.Fatal(err)
	}
}

func TestNormalizeEndpoint(t *testing.T) {
	if got := normalizeEndpoint("http://127.0.0.1:11434/"); got != "http://127.0.0.1:11434" {
		t.Fatal(got)
	}
}
