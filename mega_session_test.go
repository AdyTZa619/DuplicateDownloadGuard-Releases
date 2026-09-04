package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestMegaSessionGateHonorsCancellation(t *testing.T) {
	if err := acquireMegaSession(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer releaseMegaSession()

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	if err := acquireMegaSession(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("second MEGA owner should time out, got %v", err)
	}
}

func TestStopWarmMegaPreviewWhileSessionOwned(t *testing.T) {
	a := &App{preview: MegaPreviewState{Active: true, SourceURL: "https://mega.nz/folder/test"}}
	if err := acquireMegaSession(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := a.stopMegaPreviewWhileSessionOwned("test"); err != nil {
		releaseMegaSession()
		t.Fatal(err)
	}
	releaseMegaSession()

	if a.preview.Active || a.preview.SourceURL != "" {
		t.Fatalf("preview state was not cleared: %#v", a.preview)
	}
}
