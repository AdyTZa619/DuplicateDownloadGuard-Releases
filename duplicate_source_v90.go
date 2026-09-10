package main

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Independent worker state; never held during network, disk or decoder work.
type duplicateScanStateV90 struct {
	mu         sync.Mutex
	enabled    bool
	generation uint64
	cancel     context.CancelFunc
	done       chan struct{}
	status     duplicateScanStatusV90
}
type duplicateScanStatusV90 struct {
	Active    bool   `json:"active"`
	Completed int    `json:"completed"`
	Total     int    `json:"total"`
	Message   string `json:"message"`
}

func (a *App) registerDuplicateRoutesV90(mux *http.ServeMux) {
	a.duplicateScan.mu.Lock()
	a.duplicateScan.enabled = true
	a.duplicateScan.mu.Unlock()
	mux.HandleFunc("/api/duplicates/status", func(w http.ResponseWriter, r *http.Request) {
		a.duplicateScan.mu.Lock()
		status := a.duplicateScan.status
		a.duplicateScan.mu.Unlock()
		jsonOut(w, status)
	})
	mux.HandleFunc("/api/duplicates/cancel", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", 405)
			return
		}
		a.cancelDuplicateScanV90()
		jsonOut(w, map[string]bool{"ok": true})
	})
	mux.HandleFunc("/api/duplicates/start", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required", 405)
			return
		}
		generation := a.cancelDuplicateScanV90()
		a.mu.RLock()
		rows := append([]Result(nil), a.results...)
		a.mu.RUnlock()
		a.startDuplicateScanV90(generation, rows)
		jsonOut(w, map[string]bool{"ok": true})
	})
}
func (a *App) cancelDuplicateScanV90() uint64 {
	s := &a.duplicateScan
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	s.generation++
	s.status.Active = false
	s.status.Message = "Analiză oprită; rezultatele deja calculate sunt păstrate."
	return s.generation
}
func initialDetectorEvidenceV90(r *Result) {
	d := DownloadGuardDecision{ResultID: r.ID, Verdict: guardReview, Method: "source-candidates", Reason: "Analiza conținutului urmează. Numele și mărimea selectează candidați; nu confirmă identitatea."}
	if r.Status == "VERIFIED" && r.Remote.Hash != "" {
		d.Exact, d.Verdict, d.Method, d.Reason = true, guardDuplicate, "remote-hash", r.Reason
	} else if r.Status == "HAVE" {
		r.Status, r.Confidence = "POSSIBLE", "Conținut încă neverificat"
	}
	r.Detector = decorateDetectorEvidenceV90(d).Detector
}
func (a *App) startDuplicateScanV90(generation uint64, rows []Result) {
	s := &a.duplicateScan
	s.mu.Lock()
	if !s.enabled || s.generation != generation || len(rows) == 0 {
		s.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	s.cancel, s.done = cancel, done
	s.status = duplicateScanStatusV90{Active: true, Total: len(rows), Message: "Analizez automat conținutul online și candidații locali."}
	s.mu.Unlock()
	go func() {
		defer close(done)
		defer cancel()
		defer func() {
			s.mu.Lock()
			if s.generation == generation {
				s.status.Active = false
				s.cancel = nil
				s.status.Message = "Analiză încheiată. Rezultatele cu date insuficiente pot fi reverificate."
			}
			s.mu.Unlock()
		}()
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		wait := func() bool {
			select {
			case <-ctx.Done():
				return false
			case <-ticker.C:
				return true
			}
		}
		// The source scan may still own the MEGA session. Wait until it releases it.
		for a.opRunning.Load() {
			if !wait() {
				return
			}
		}
		a.mu.RLock()
		entries := make([]FileEntry, 0, len(a.index))
		for _, entry := range a.index {
			entries = append(entries, entry)
		}
		a.mu.RUnlock()
		bySize := map[int64][]FileEntry{}
		for _, entry := range entries {
			bySize[entry.Size] = append(bySize[entry.Size], entry)
		}
		ctx = detectorCandidateContextV90(ctx, a, entries)
		for _, row := range rows {
			if ctx.Err() != nil {
				return
			}
			for !a.guardMu.TryLock() {
				if !wait() {
					return
				}
			}
			if ctx.Err() != nil {
				a.guardMu.Unlock()
				return
			}
			megaLocked := false
			ready := true
			if strings.EqualFold(row.Remote.Source, "MEGA") {
				ready = !a.opRunning.Load() && megaQueueMu.TryLock()
				megaLocked = ready
			}
			rowCtx, rowCancel := context.WithTimeout(ctx, 90*time.Second)
			metrics := &detectorMetricsV90{}
			rowCtx = context.WithValue(rowCtx, detectorMetricsKeyV90{}, metrics)
			d := a.evaluateDownloadGuard(rowCtx, row, entries, bySize, guardModeSmart, ready)
			rowCancel()
			// Keep the existing preview session warm; only release our queue lease.
			if megaLocked {
				megaQueueMu.Unlock()
			}
			a.guardMu.Unlock()
			if ctx.Err() != nil {
				return
			}
			d = decorateGuardDecision(d)
			d.Detector.DeepAnalyzed = int(metrics.Deep.Load())
			d.Detector.LocalCacheHits = int(metrics.LocalHits.Load())
			d.Detector.RemoteBytes = metrics.RemoteBytes.Load()
			d.Detector.RemoteCacheHit = metrics.RemoteHit.Load()
			s.mu.Lock()
			if s.generation != generation {
				s.mu.Unlock()
				return
			}
			a.mu.Lock()
			for i := range a.results {
				if a.results[i].ID == row.ID && a.results[i].Remote == row.Remote {
					applyGuardDecisionV90(&a.results[i], d, time.Now().Unix())
					enrichResult(&a.results[i], a.index)
					break
				}
			}
			a.mu.Unlock()
			s.status.Completed++
			s.mu.Unlock()
			a.revision.Add(1)
			_ = a.saveResults()
		}
		_ = a.saveIndex()
	}()
}
