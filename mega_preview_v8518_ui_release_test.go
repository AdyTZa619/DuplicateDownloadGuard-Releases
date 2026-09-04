package main

import (
	"os"
	"strings"
	"testing"
)

func TestRemotePreviewReleaseDoesNotForceMediaLoadV8518(t *testing.T) {
	b, err := os.ReadFile("web/preview_quick_v86.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	start := strings.Index(s, "function releaseRemoteMediaV8512()")
	end := strings.Index(s, "window.releaseRemoteMediaV8512 = releaseRemoteMediaV8512")
	if start < 0 || end <= start {
		t.Fatal("releaseRemoteMediaV8512 lipsă")
	}
	body := s[start:end]
	if strings.Contains(body, "media.load()") {
		t.Fatal("schimbarea rândului nu trebuie să apeleze media.load(); poate bloca Chromium sincron")
	}
	if strings.Contains(body, "media.removeAttribute('src')") {
		t.Fatal("nu rupe src sincron înainte de detașarea playerului")
	}
	if !strings.Contains(body, "media.pause()") || !strings.Contains(body, "media.remove()") {
		t.Fatal("playerul vechi trebuie oprit și detașat fără reload")
	}
}
