package main

import (
	"os"
	"strings"
	"testing"
)

func TestLocalPreviewV8570HasBrowserSafeImageFallback(t *testing.T) {
	b, err := os.ReadFile("web/local_preview_resilience_v8570.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, marker := range []string{
		"/api/local-preview?path=",
		"fetch(directURL(path)",
		"response.arrayBuffer()",
		"URL.createObjectURL(new Blob",
		"data-ddg-local-stage=\"direct\"",
		"onload=\"ddgLocalPreviewV8570.onImageLoad(this)\"",
		"onerror=\"ddgLocalPreviewV8570.onImageError(this)\"",
		"object-fit:contain",
		"IMAGE • FALLBACK OK",
		"Imagine locală indisponibilă",
	} {
		if !strings.Contains(s, marker) {
			t.Fatalf("resilient local preview missing marker %q", marker)
		}
	}
	for _, forbidden := range []string{"/api/index/start", "/api/mega/scan", "/api/download/preflight"} {
		if strings.Contains(s, forbidden) {
			t.Fatalf("local preview must never trigger scan/preflight path %q", forbidden)
		}
	}
}

func TestLocalPreviewV8570KeepsVideoAudioStreaming(t *testing.T) {
	b, err := os.ReadFile("web/local_preview_resilience_v8570.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, marker := range []string{
		"if (kind === 'video')",
		"preload=\"metadata\"",
		"if (kind === 'audio')",
		"ddgLocalPreviewV8570.onMediaError(this)",
		"playerul extern",
	} {
		if !strings.Contains(s, marker) {
			t.Fatalf("streaming local preview missing marker %q", marker)
		}
	}
}

func TestLocalPreviewV8571LoadsBeforeProviderCompare(t *testing.T) {
	b, err := os.ReadFile("web/preview_quick_v86.js")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	core := strings.Index(s, "/preview_quick_core.js")
	local := strings.Index(s, "/local_preview_resilience_v8571.js")
	provider := strings.Index(s, "/provider_compare_ui_v8558.js")
	if core < 0 || local < 0 || provider < 0 {
		t.Fatal("expected preview core, resilient local preview v8571 and provider compare modules")
	}
	if !(core < local && local < provider) {
		t.Fatalf("unsafe preview module order: core=%d local=%d provider=%d", core, local, provider)
	}
}
