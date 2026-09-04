package main

import (
	"strings"
	"testing"
	"time"
)

func TestMegaPreviewUICacheRootNeedsNoNetworkProbe(t *testing.T) {
	a := &App{}
	a.preview = MegaPreviewState{
		Active:     true,
		SourceURL:  "https://mega.nz/folder/X#KEY",
		RemotePath: megaWarmRootRefV86,
		StreamURL:  "http://127.0.0.1:65534/root",
		Exe:        "mega-cmd.exe",
	}
	item := RemoteItem{
		URL:    a.preview.SourceURL,
		Path:   "/pack/My Video #1.mp4",
		Name:   "My Video #1.mp4",
		Source: "MEGA",
	}
	started := time.Now()
	got, mode, ok := a.tryMegaPreviewUICacheV854(item)
	elapsed := time.Since(started)
	if !ok {
		t.Fatal("expected root cache hit even though the test URL is unreachable")
	}
	if mode != "MEGA FAST ROOT" {
		t.Fatalf("mode=%q", mode)
	}
	if !strings.Contains(got, "/root/pack/My%20Video%20%231.mp4") {
		t.Fatalf("unexpected URL: %s", got)
	}
	if elapsed > 100*time.Millisecond {
		t.Fatalf("cache hit performed blocking work: %v", elapsed)
	}
}

func TestMegaPreviewUICacheReusesExactPerFileNode(t *testing.T) {
	item := RemoteItem{URL: "source", Path: "/a.mp4", Name: "a.mp4", Source: "MEGA", Handle: "AbCdEf12"}
	a := &App{}
	a.preview = MegaPreviewState{
		Active:     true,
		SourceURL:  item.URL,
		RemotePath: megaRemoteRef(item),
		StreamURL:  "http://127.0.0.1:4443/node/a.mp4",
	}
	got, mode, ok := a.tryMegaPreviewUICacheV854(item)
	if !ok || got != a.preview.StreamURL || mode != "MEGA FAST CACHE" {
		t.Fatalf("unexpected cache result: ok=%v mode=%q url=%q", ok, mode, got)
	}
}

func TestMegaPreviewUICacheRejectsDifferentSource(t *testing.T) {
	a := &App{preview: MegaPreviewState{Active: true, SourceURL: "A", RemotePath: megaWarmRootRefV86, StreamURL: "http://127.0.0.1/root"}}
	if _, _, ok := a.tryMegaPreviewUICacheV854(RemoteItem{URL: "B", Path: "/x.mp4", Source: "MEGA"}); ok {
		t.Fatal("different MEGA source must not reuse warm root")
	}
}
