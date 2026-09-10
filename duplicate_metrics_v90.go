package main

import (
	"context"
	"sync/atomic"
)

type detectorMetricsKeyV90 struct{}
type detectorMetricsV90 struct {
	Deep, LocalHits, RemoteBytes atomic.Int64
	RemoteHit                    atomic.Bool
}

func detectorMetricsFromV90(ctx context.Context) *detectorMetricsV90 {
	m, _ := ctx.Value(detectorMetricsKeyV90{}).(*detectorMetricsV90)
	return m
}
