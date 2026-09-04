package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	guardModeSmart = "smart"
	guardModeExact = "exact"
	guardModeAI    = "ai"

	guardDownload  = "DOWNLOAD"
	guardDuplicate = "DUPLICATE"
	guardReview    = "REVIEW"

	downloadGuardVersion = 2
)

type DownloadGuardDecision struct {
	ResultID    int    `json:"resultId"`
	Name        string `json:"name"`
	Verdict     string `json:"verdict"`
	Reason      string `json:"reason"`
	LocalPath   string `json:"localPath,omitempty"`
	Method      string `json:"method"`
	Candidates  int    `json:"candidates"`
	Exact       bool   `json:"exact"`
	UserStatus  string `json:"userStatus,omitempty"`
	Action      string `json:"action,omitempty"`
	Similarity  int    `json:"similarity,omitempty"`
	QualityHint string `json:"qualityHint,omitempty"`
}

type DownloadGuardReport struct {
	Mode         string                  `json:"mode"`
	Decisions    []DownloadGuardDecision `json:"decisions"`
	Counts       map[string]int          `json:"counts"`
	ScannedFiles int                     `json:"scannedFiles"`
	ScannedBytes int64                   `json:"scannedBytes"`
	ScannedRoots []string                `json:"scannedRoots"`
	DurationMS   int64                   `json:"durationMs"`
}

type guardScan struct {
	Files int
	Bytes int64
	Roots []string
}

func normalizeGuardMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case guardModeExact:
		return guardModeExact
	case guardModeAI:
		return guardModeAI
	default:
		return guardModeSmart
	}
}

func isGuardTemporaryFile(path string) bool {
	lower := strings.ToLower(filepath.Base(path))
	for _, suffix := range []string{".part", ".aria2", ".download", ".tmp", ".checksum_failed"} {
		if strings.HasSuffix(lower, suffix) {
			return true
		}
	}
	return false
}

func pathKey(path string) string {
	p := filepath.Clean(path)
	if runtimeIsWindows() {
		return strings.ToLower(p)
	}
	return p
}

func runtimeIsWindows() bool {
	return filepath.Separator == '\\'
}

func compactGuardRoots(paths []string) []string {
	uniq := map[string]string{}
	for _, raw := range paths {
		p := strings.TrimSpace(raw)
		if p == "" {
			continue
		}
		abs, err := filepath.Abs(p)
		if err == nil {
			p = abs
		}
		p = filepath.Clean(p)
		st, err := os.Stat(p)
		if err != nil || !st.IsDir() {
			continue
		}
		uniq[pathKey(p)] = p
	}
	roots := make([]string, 0, len(uniq))
	for _, p := range uniq {
		roots = append(roots, p)
	}
	sort.Slice(roots, func(i, j int) bool {
		if len(roots[i]) != len(roots[j]) {
			return len(roots[i]) < len(roots[j])
		}
		return strings.ToLower(roots[i]) < strings.ToLower(roots[j])
	})
	kept := make([]string, 0, len(roots))
	for _, root := range roots {
		covered := false
		for _, parent := range kept {
			if isUnder(root, parent) {
				covered = true
				break
			}
		}
		if !covered {
			kept = append(kept, root)
		}
	}
	return kept
}

func (a *App) guardRoots(destination string) []string {
	a.mu.RLock()
	paths := append([]string(nil), a.cfg.LocalPaths...)
	configuredDownload := a.cfg.DownloadDir
	a.mu.RUnlock()
	paths = append(paths, configuredDownload, destination)
	return compactGuardRoots(paths)
}

func pathUnderAny(path string, roots []string) bool {
	for _, root := range roots {
		if isUnder(path, root) {
			return true
		}
	}
	return false
}

func (a *App) refreshLiveIndexForGuard(ctx context.Context, destination string) ([]FileEntry, guardScan, error) {
	roots := a.guardRoots(destination)
	a.mu.RLock()
	old := make(map[string]FileEntry, len(a.index))
	for p, e := range a.index {
		old[p] = e
	}
	a.mu.RUnlock()

	updated := make(map[string]FileEntry, len(old))
	for p, e := range old {
		updated[p] = e
	}
	entries := map[string]FileEntry{}
	seen := map[string]bool{}
	scan := guardScan{Roots: append([]string(nil), roots...)}

	add := func(path string, info os.FileInfo) {
		if info == nil || info.IsDir() || isGuardTemporaryFile(path) {
			return
		}
		path = filepath.Clean(path)
		mtimeNano := info.ModTime().UnixNano()
		e := FileEntry{Path: path, Name: filepath.Base(path), Size: info.Size(), MTime: mtimeNano}
		if prev, ok := old[path]; ok && prev.Size == e.Size && (prev.MTime == mtimeNano || prev.MTime == info.ModTime().Unix()) {
			e.SHA256, e.MD5 = prev.SHA256, prev.MD5
		}
		updated[path] = e
		entries[pathKey(path)] = e
		seen[pathKey(path)] = true
		scan.Files++
		scan.Bytes += info.Size()
	}

	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if walkErr != nil {
				if d != nil && d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if d.IsDir() {
				return nil
			}
			info, err := d.Info()
			if err == nil {
				add(path, info)
			}
			return nil
		})
		if err != nil && !errors.Is(err, context.Canceled) {
			a.logf("ExactGuard: scanarea live a ignorat o eroare în %s: %v", root, err)
		}
		if ctx.Err() != nil {
			return nil, scan, ctx.Err()
		}
	}

	for path := range old {
		if seen[pathKey(path)] || pathUnderAny(path, roots) {
			continue
		}
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			delete(updated, path)
			continue
		}
		add(path, info)
	}
	for path := range old {
		if pathUnderAny(path, roots) && !seen[pathKey(path)] {
			delete(updated, path)
		}
	}

	a.mu.Lock()
	a.index = updated
	a.rebuildMaps()
	a.mu.Unlock()

	out := make([]FileEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, e)
	}
	return out, scan, nil
}

func remoteContentSHA256(ctx context.Context, target string, expectedSize int64) (string, int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Accept-Encoding", "identity")
	req.Header.Set("User-Agent", "DuplicateDownloadGuard/8.5 ExactGuard")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", 0, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	h := sha256.New()
	reader := io.Reader(resp.Body)
	if expectedSize > 0 {
		reader = io.LimitReader(resp.Body, expectedSize+1)
	}
	n, err := io.Copy(h, reader)
	if err != nil {
		return "", n, err
	}
	if expectedSize > 0 && n != expectedSize {
		return "", n, fmt.Errorf("mărime remote schimbată: %d/%d bytes", n, expectedSize)
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

func rankGuardCandidates(remote RemoteItem, entries []FileEntry) []FileEntry {
	out := append([]FileEntry(nil), entries...)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := rankCandidate(remote, out[i]), rankCandidate(remote, out[j])
		if a.Rank != b.Rank {
			return a.Rank > b.Rank
		}
		return strings.ToLower(out[i].Path) < strings.ToLower(out[j].Path)
	})
	return out
}

func (a *App) exactLocalHashMatch(remote RemoteItem, candidates []FileEntry) (string, bool, error) {
	kind := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(remote.HashType), "-", ""))
	if kind != "sha256" && kind != "md5" {
		return "", false, errors.New("tipul hash-ului remote nu este suportat")
	}
	for _, candidate := range rankGuardCandidates(remote, candidates) {
		h, err := a.ensureHash(candidate.Path, kind)
		if err != nil {
			continue
		}
		if strings.EqualFold(h, strings.TrimSpace(remote.Hash)) {
			return candidate.Path, true, nil
		}
	}
	return "", false, nil
}

func sampleGuardCandidates(ctx context.Context, target string, size int64, candidates []FileEntry, blocks, blockKB int) ([]FileEntry, int64, error) {
	if size <= 0 {
		return nil, 0, errors.New("mărimea remote este necunoscută")
	}
	ranges := sampleRanges(size, int64(blockKB)<<10, blocks)
	active := append([]FileEntry(nil), candidates...)
	var transferred int64
	for _, rg := range ranges {
		remoteBytes, err := fetchHTTPRange(ctx, target, rg[0], rg[1])
		if err != nil {
			return nil, transferred, err
		}
		transferred += int64(len(remoteBytes))
		next := active[:0]
		for _, candidate := range active {
			f, err := os.Open(candidate.Path)
			if err != nil {
				continue
			}
			localBytes := make([]byte, len(remoteBytes))
			n, readErr := f.ReadAt(localBytes, rg[0])
			_ = f.Close()
			if (readErr == nil || readErr == io.EOF) && n == len(remoteBytes) && bytes.Equal(localBytes, remoteBytes) {
				next = append(next, candidate)
			}
		}
		active = next
		if len(active) == 0 {
			break
		}
	}
	return active, transferred, nil
}

func bestAIAdvisoryCandidate(remote RemoteItem, entries []FileEntry) (FileEntry, bool) {
	bestRank := -1
	var best FileEntry
	remoteKind := remoteMediaKind(remote.Name)
	if remoteKind != "image" && remoteKind != "video" {
		return best, false
	}
	for _, e := range entries {
		if remoteMediaKind(e.Name) != remoteKind {
			continue
		}
		c := rankCandidate(remote, e)
		ratio := 1.0
		if remote.Size > 0 {
			ratio = float64(abs64(e.Size-remote.Size)) / float64(remote.Size)
		}
		if c.NameScore < 60 && ratio > .08 {
			continue
		}
		if c.Rank > bestRank {
			bestRank, best = c.Rank, e
		}
	}
	return best, bestRank >= 0
}

func guardReviewDecision(res Result, method, reason string, candidates int, local string) DownloadGuardDecision {
	return DownloadGuardDecision{ResultID: res.ID, Name: res.Remote.Name, Verdict: guardReview, Reason: reason, LocalPath: local, Method: method, Candidates: candidates}
}

func (a *App) evaluateDownloadGuard(ctx context.Context, res Result, entries []FileEntry, bySize map[int64][]FileEntry, mode string, megaRemoteAvailable bool) DownloadGuardDecision {
	base := DownloadGuardDecision{ResultID: res.ID, Name: res.Remote.Name, Verdict: guardDownload, Method: "live-size-index"}
	if history, ok := downloadHistoryDecision(res); ok {
		return history
	}
	if res.Remote.Size <= 0 || res.Remote.ApproxSize {
		kind := remoteMediaKind(res.Remote.Name)
		if kind == "image" || kind == "video" {
			if mediaDecision, ok := a.mediaNearDuplicateDecision(ctx, res, entries, megaRemoteAvailable); ok {
				return mediaDecision
			}
		}
		return guardReviewDecision(res, "metadata-incomplete", "Mărimea remote nu este exactă. Am încercat verificările de conținut disponibile, dar descărcarea automată rămâne oprită până la un verdict sigur.", 0, res.LocalPath)
	}
	candidates := rankGuardCandidates(res.Remote, bySize[res.Remote.Size])
	base.Candidates = len(candidates)
	if len(candidates) == 0 {
		if mediaDecision, ok := a.mediaNearDuplicateDecision(ctx, res, entries, megaRemoteAvailable); ok {
			return mediaDecision
		}
		base.Reason = "Scanarea live și verificarea candidaților media nu au găsit un corespondent relevant în colecția locală."
		if res.Manual && strings.EqualFold(res.Status, "HAVE") {
			return guardReviewDecision(res, "manual-have", "Rezultatul este marcat manual «Ai deja», dar fișierul local nu mai este disponibil; necesită confirmare.", 0, res.LocalPath)
		}
		return base
	}

	if res.Remote.Hash != "" && res.Remote.HashType != "" {
		path, same, err := a.exactLocalHashMatch(res.Remote, candidates)
		if err == nil && same {
			return DownloadGuardDecision{ResultID: res.ID, Name: res.Remote.Name, Verdict: guardDuplicate, Reason: "Hash-ul remote coincide cu hash-ul fișierului local, indiferent de nume.", LocalPath: path, Method: "remote-hash", Candidates: len(candidates), Exact: true}
		}
		if err == nil {
			if mediaDecision, ok := a.mediaNearDuplicateDecision(ctx, res, entries, megaRemoteAvailable); ok {
				return mediaDecision
			}
			base.Method = "remote-hash"
			base.Reason = fmt.Sprintf("Toți cei %d candidați de aceeași mărime au hash diferit de hash-ul remote.", len(candidates))
			return base
		}
	}

	if strings.EqualFold(res.Remote.Source, "MEGA") && !megaRemoteAvailable {
		return guardReviewDecision(res, "mega-busy", "MEGA este ocupat cu altă scanare/descărcare; candidații rămân pentru verificare manuală și nu sunt descărcați automat.", len(candidates), candidates[0].Path)
	}
	target, err := remoteTarget(a, res)
	if err != nil {
		return guardReviewDecision(res, "remote-unavailable", "Nu am putut deschide conținutul remote pentru verificare: "+err.Error(), len(candidates), candidates[0].Path)
	}

	a.mu.RLock()
	fullMB, blocks, blockKB := a.cfg.FullVerifyMaxMB, a.cfg.SampleBlocks, a.cfg.SampleBlockKB
	a.mu.RUnlock()
	if fullMB <= 0 {
		fullMB = 12
	}
	if blocks < 3 {
		blocks = 3
	}
	if blockKB < 64 {
		blockKB = 64
	}

	if mode == guardModeExact || res.Remote.Size <= int64(fullMB)<<20 {
		remoteHash, transferred, err := remoteContentSHA256(ctx, target, res.Remote.Size)
		if err != nil {
			return guardReviewDecision(res, "full-sha256-error", "Verificarea integrală nu s-a putut încheia: "+err.Error(), len(candidates), candidates[0].Path)
		}
		for _, candidate := range candidates {
			localHash, hashErr := a.ensureHash(candidate.Path, "sha256")
			if hashErr == nil && strings.EqualFold(localHash, remoteHash) {
				return DownloadGuardDecision{ResultID: res.ID, Name: res.Remote.Name, Verdict: guardDuplicate, Reason: fmt.Sprintf("Conținut identic confirmat prin SHA-256 după citirea integrală a %s remote.", human(transferred)), LocalPath: candidate.Path, Method: "full-sha256", Candidates: len(candidates), Exact: true}
			}
		}
		if mediaDecision, ok := a.mediaNearDuplicateDecision(ctx, res, entries, megaRemoteAvailable); ok {
			return mediaDecision
		}
		base.Method = "full-sha256"
		base.Reason = fmt.Sprintf("Verificarea integrală SHA-256 (%s remote) a demonstrat că toți candidații de aceeași mărime au conținut diferit.", human(transferred))
		return base
	}

	survivors, transferred, err := sampleGuardCandidates(ctx, target, res.Remote.Size, candidates, blocks, blockKB)
	if err != nil {
		return guardReviewDecision(res, "sample-error", "Mostrele remote nu au putut fi verificate: "+err.Error(), len(candidates), candidates[0].Path)
	}
	if len(survivors) > 0 {
		return guardReviewDecision(res, "deterministic-samples", fmt.Sprintf("%d candidat(ți) au trecut toate mostrele distribuite (%s trafic). Pot fi identici, dar mostrele nu sunt dovadă integrală.", len(survivors), human(transferred)), len(candidates), survivors[0].Path)
	}
	if mediaDecision, ok := a.mediaNearDuplicateDecision(ctx, res, entries, megaRemoteAvailable); ok {
		return mediaDecision
	}
	base.Method = "deterministic-samples"
	base.Reason = fmt.Sprintf("Fiecare candidat de aceeași mărime diferă în cel puțin o mostră; %s transferați pentru verificare.", human(transferred))
	return base
}

func (a *App) applyAIAdvisory(ctx context.Context, res Result, entries []FileEntry, decision DownloadGuardDecision) DownloadGuardDecision {
	if decision.Verdict != guardDownload {
		return decision
	}
	candidate, ok := bestAIAdvisoryCandidate(res.Remote, entries)
	if !ok {
		return decision
	}
	res.LocalPath = candidate.Path
	c := rankCandidate(res.Remote, candidate)
	res.NameScore, res.MatchScore, res.SameSize, res.SameExt = c.NameScore, c.MatchScore, c.SameSize, c.SameExt

	visual := httptestLikeVisual(a, res, candidate.Path, ctx)
	if visual.err == nil && visual.score >= 90 {
		d := guardReviewDecision(res, "media-looks-same", fmt.Sprintf("Fingerprint-ul media indică %d%% similaritate cu %s. Verifică înainte de download.", visual.score, candidate.Path), max(1, decision.Candidates), candidate.Path)
		d.Similarity = visual.score
		return d
	}
	a.mu.RLock()
	aiEnabled, useVision := a.cfg.AIEnabled, a.cfg.AIVision
	a.mu.RUnlock()
	if !aiEnabled {
		return decision
	}
	answer, err := a.callOllama(ctx, res, candidate.Path, useVision)
	if err == nil && (answer.Verdict == "same" || answer.Verdict == "probably_same") && answer.Confidence >= 65 {
		return guardReviewDecision(res, "ollama-advisory", fmt.Sprintf("AI local: %s (%d%%) — %s. AI este consultativ, deci rezultatul necesită verificare.", answer.Verdict, answer.Confidence, answer.Reason), max(1, decision.Candidates), candidate.Path)
	}
	return decision
}

func (a *App) applyGuardDecisions(decisions []DownloadGuardDecision) {
	now := time.Now().Unix()
	byID := make(map[int]DownloadGuardDecision, len(decisions))
	for _, decision := range decisions {
		byID[decision.ResultID] = decision
	}
	a.mu.Lock()
	for i := range a.results {
		decision, ok := byID[a.results[i].ID]
		if !ok {
			continue
		}
		x := &a.results[i]
		x.GuardVerdict, x.GuardMethod, x.GuardReason, x.GuardAt = decision.Verdict, decision.Method, decision.Reason, now
		x.Candidates = decision.Candidates
		if decision.LocalPath != "" {
			x.LocalPath = decision.LocalPath
		}
		switch decision.Verdict {
		case guardDuplicate:
			x.AutoStatus = "VERIFIED"
			x.AutoConfidence = "ExactGuard • 100% conținut"
			x.AutoReason = decision.Reason
			x.MatchScore, x.SameSize = 100, true
		case guardReview:
			status := "POSSIBLE"
			if decision.Method == "deterministic-samples" {
				status = "SAMPLED"
				x.MatchScore = 99
			}
			if decision.Similarity > 0 {
				x.VisualScore = decision.Similarity
				if x.MatchScore < decision.Similarity {
					x.MatchScore = decision.Similarity
				}
			}
			x.AutoStatus = status
			x.AutoConfidence = "ExactGuard • necesită verificare"
			x.AutoReason = decision.Reason
		case guardDownload:
			x.AutoStatus = "MISSING"
			x.AutoConfidence = "ExactGuard • fără corespondent relevant"
			x.AutoReason = decision.Reason
		}
		if !x.Manual {
			x.Status, x.Confidence, x.Reason = x.AutoStatus, x.AutoConfidence, x.AutoReason
		}
	}
	a.mu.Unlock()
	a.revision.Add(1)
	_ = a.saveResults()
}

func (a *App) runDownloadGuard(ctx context.Context, rows []Result, destination, requestedMode string) (DownloadGuardReport, error) {
	started := time.Now()
	mode := normalizeGuardMode(requestedMode)
	if requestedMode == "" {
		a.mu.RLock()
		mode = normalizeGuardMode(a.cfg.DownloadGuardMode)
		a.mu.RUnlock()
	}
	report := DownloadGuardReport{Mode: mode, Counts: map[string]int{guardDownload: 0, guardDuplicate: 0, guardReview: 0}}
	if len(rows) == 0 {
		return report, errors.New("nu există rezultate selectate")
	}

	a.guardMu.Lock()
	defer a.guardMu.Unlock()
	entries, scan, err := a.refreshLiveIndexForGuard(ctx, destination)
	if err != nil {
		return report, err
	}
	report.ScannedFiles, report.ScannedBytes, report.ScannedRoots = scan.Files, scan.Bytes, scan.Roots
	bySize := map[int64][]FileEntry{}
	for _, entry := range entries {
		bySize[entry.Size] = append(bySize[entry.Size], entry)
	}

	hasMega := false
	for _, row := range rows {
		kind := remoteMediaKind(row.Remote.Name)
		needsMedia := kind == "image" || kind == "video"
		if strings.EqualFold(row.Remote.Source, "MEGA") && (mode == guardModeAI || needsMedia || (row.Remote.Size > 0 && len(bySize[row.Remote.Size]) > 0 && row.Remote.Hash == "")) {
			hasMega = true
			break
		}
	}
	megaReady := true
	megaLocked := false
	if hasMega {
		megaReady = !a.opRunning.Load() && megaQueueMu.TryLock()
		megaLocked = megaReady
	}
	if megaLocked {
		defer func() {
			_ = a.stopMegaPreview("ExactGuard finalizat")
			megaQueueMu.Unlock()
		}()
	}

	for _, row := range rows {
		if ctx.Err() != nil {
			return report, ctx.Err()
		}
		decision := a.evaluateDownloadGuard(ctx, row, entries, bySize, mode, megaReady)
		if mode == guardModeAI && decision.Verdict == guardDownload && (!strings.EqualFold(row.Remote.Source, "MEGA") || megaReady) {
			decision = a.applyAIAdvisory(ctx, row, entries, decision)
		}
		if decision.Verdict == guardDownload && row.Manual && strings.EqualFold(row.Status, "HAVE") {
			decision = guardReviewDecision(row, "manual-have", "Fișierul este marcat manual «Ai deja». Download Guard nu suprascrie această decizie fără confirmare explicită. "+decision.Reason, decision.Candidates, row.LocalPath)
		}
		decision = decorateGuardDecision(decision)
		report.Decisions = append(report.Decisions, decision)
		report.Counts[decision.Verdict]++
	}
	report.DurationMS = time.Since(started).Milliseconds()
	if err := a.saveIndex(); err != nil {
		a.logf("ExactGuard: indexul live nu a putut fi salvat: %v", err)
	}
	a.applyGuardDecisions(report.Decisions)
	a.logf("ExactGuard %s: %d download, %d duplicate blocate, %d review • scan live %d fișiere în %d ms", mode, report.Counts[guardDownload], report.Counts[guardDuplicate], report.Counts[guardReview], report.ScannedFiles, report.DurationMS)
	return report, nil
}

func selectedResults(rows []Result, ids []int) []Result {
	wanted := map[int]bool{}
	for _, id := range ids {
		wanted[id] = true
	}
	out := make([]Result, 0, len(wanted))
	for _, row := range rows {
		if wanted[row.ID] {
			out = append(out, row)
		}
	}
	return out
}

func (a *App) handleDownloadPreflight(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs         []int  `json:"ids"`
		Destination string `json:"destination"`
		Mode        string `json:"mode"`
	}
	if err := decodeJSON(r, &req); err != nil || len(req.IDs) == 0 {
		http.Error(w, "selecție invalidă", http.StatusBadRequest)
		return
	}
	a.mu.RLock()
	rows := selectedResults(append([]Result(nil), a.results...), req.IDs)
	destination := strings.TrimSpace(req.Destination)
	if destination == "" {
		destination = a.cfg.DownloadDir
	}
	a.mu.RUnlock()
	report, err := a.runDownloadGuard(r.Context(), rows, destination, req.Mode)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	jsonOut(w, report)
}
