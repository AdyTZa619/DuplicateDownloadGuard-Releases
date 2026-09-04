package main

import (
	"context"
	"fmt"
	"math"
	"math/bits"
	"os"
	"sort"
	"strings"
)

const (
	userHaveExact    = "AI DEJA"
	userSameContent  = "ACELAȘI CONȚINUT"
	userOtherVersion = "ALTĂ VERSIUNE"
	userLooksSame    = "PARE ACELAȘI"
	userPossible     = "POSIBIL DUPLICAT"
	userMissing      = "NU ÎL AI"
	userDownloaded   = "DESCĂRCAT DEJA"
	userUnverified   = "NU S-A PUTUT VERIFICA"
	userUnavailable  = "INDISPONIBIL"
	userQuota        = "LIMITĂ / COTĂ"
	userError        = "EROARE"

	actionDontDownload = "NU DESCĂRCA"
	actionDownload     = "DESCARCĂ"
	actionReview       = "VERIFICĂ MANUAL"
	actionRemoteBetter = "REMOTE E MAI BUN"
	actionLocalBetter  = "AI DEJA VERSIUNEA MAI BUNĂ"
	actionRetry        = "REÎNCEARCĂ"
)

// decorateGuardDecision translates internal safety verdicts into short labels
// that describe what the user actually needs to know. Internal verdicts stay
// unchanged so queue/update compatibility is preserved.
func decorateGuardDecision(d DownloadGuardDecision) DownloadGuardDecision {
	if d.UserStatus != "" {
		return d
	}
	switch d.Verdict {
	case guardDuplicate:
		d.UserStatus = userHaveExact
		d.Action = actionDontDownload
	case guardDownload:
		d.UserStatus = userMissing
		d.Action = actionDownload
	case guardReview:
		d.UserStatus = userPossible
		d.Action = actionReview
	}

	switch d.Method {
	case "media-same-content":
		d.UserStatus = userSameContent
		d.Action = actionDontDownload
	case "media-version":
		d.UserStatus = userOtherVersion
		if d.QualityHint == "remote" {
			d.Action = actionRemoteBetter
		} else if d.QualityHint == "local" {
			d.Action = actionLocalBetter
		} else {
			d.Action = actionReview
		}
	case "media-looks-same", "deterministic-samples":
		d.UserStatus = userLooksSame
		d.Action = actionReview
	case "metadata-incomplete", "mega-busy", "remote-unavailable", "full-sha256-error", "sample-error":
		d.UserStatus = userUnverified
		d.Action = actionRetry
	}
	return d
}

type mediaGuardCandidate struct {
	Entry FileEntry
	Rank  int
}

func mediaGuardCandidates(remote RemoteItem, entries []FileEntry, limit int) []FileEntry {
	kind := remoteMediaKind(remote.Name)
	if kind != "image" && kind != "video" {
		return nil
	}
	if limit <= 0 {
		limit = 5
	}
	rows := make([]mediaGuardCandidate, 0, 32)
	for _, e := range entries {
		if remoteMediaKind(e.Name) != kind {
			continue
		}
		name := nameSimilarity(remote.Name, e.Name)
		ratio := 1.0
		if remote.Size > 0 {
			ratio = float64(abs64(e.Size-remote.Size)) / float64(remote.Size)
		}
		// Near-duplicate media may have very different byte sizes after re-encode,
		// but completely unrelated names should not trigger expensive probes.
		if name < 45 && ratio > .35 {
			continue
		}
		rank := name * 100
		switch {
		case ratio <= .01:
			rank += 3000
		case ratio <= .05:
			rank += 2200
		case ratio <= .15:
			rank += 1400
		case ratio <= .35:
			rank += 700
		}
		if strings.EqualFold(filepathExt(remote.Name), filepathExt(e.Name)) {
			rank += 250
		}
		rows = append(rows, mediaGuardCandidate{Entry: e, Rank: rank})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Rank != rows[j].Rank {
			return rows[i].Rank > rows[j].Rank
		}
		return strings.ToLower(rows[i].Entry.Path) < strings.ToLower(rows[j].Entry.Path)
	})
	if len(rows) > limit {
		rows = rows[:limit]
	}
	out := make([]FileEntry, len(rows))
	for i := range rows {
		out[i] = rows[i].Entry
	}
	return out
}

func filepathExt(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if i := strings.LastIndex(name, "."); i >= 0 {
		return name[i:]
	}
	return ""
}

// visualVideoScoreV85 samples seven distributed frames instead of three. It
// tolerates re-encoding while still penalizing meaningful duration differences.
func (a *App) visualVideoScoreV85(ctx context.Context, target, local string) (int, string, MediaInfo, MediaInfo, error) {
	ff := a.detectFFmpeg()
	fp := a.detectFFprobe()
	if ff == "" || fp == "" {
		return 0, "", MediaInfo{}, MediaInfo{}, fmt.Errorf("ffmpeg + ffprobe lipsesc")
	}
	ri := probeMedia(ctx, fp, target, "REMOTE")
	li := probeMedia(ctx, fp, local, "LOCAL")
	if !ri.OK || !li.OK || ri.Duration <= 0 || li.Duration <= 0 {
		return 0, "", ri, li, fmt.Errorf("nu pot citi durata ambelor videoclipuri")
	}
	minD := math.Min(ri.Duration, li.Duration)
	points := []float64{.08, .20, .35, .50, .65, .80, .92}
	sum := 0
	matched := 0
	for _, p := range points {
		sec := minD * p
		rh, err := frameHash(ctx, ff, target, sec)
		if err != nil {
			continue
		}
		lh, err := frameHash(ctx, ff, local, sec)
		if err != nil {
			continue
		}
		d := bits.OnesCount64(rh ^ lh)
		sum += int(math.Round(float64(64-d) * 100 / 64))
		matched++
	}
	if matched < 4 {
		return 0, "", ri, li, fmt.Errorf("prea puține cadre comparabile: %d/7", matched)
	}
	score := int(math.Round(float64(sum) / float64(matched)))
	delta := math.Abs(ri.Duration - li.Duration)
	tolerance := math.Max(1.0, minD*.012)
	if delta > tolerance {
		score = int(math.Round(float64(score) * .78))
	}
	return score, fmt.Sprintf("%d/7 cadre • durată Δ %.2fs", matched, delta), ri, li, nil
}

func mediaQualityHint(remote, local MediaInfo) string {
	if !remote.OK || !local.OK {
		return ""
	}
	remotePixels := int64(remote.Width) * int64(remote.Height)
	localPixels := int64(local.Width) * int64(local.Height)
	if remotePixels > 0 && localPixels > 0 {
		if remotePixels >= localPixels*13/10 {
			return "remote"
		}
		if localPixels >= remotePixels*13/10 {
			return "local"
		}
	}
	if remote.BitRate > 0 && local.BitRate > 0 {
		if remote.BitRate >= local.BitRate*3/2 {
			return "remote"
		}
		if local.BitRate >= remote.BitRate*3/2 {
			return "local"
		}
	}
	return ""
}

func (a *App) mediaNearDuplicateDecision(ctx context.Context, res Result, entries []FileEntry, megaRemoteAvailable bool) (DownloadGuardDecision, bool) {
	kind := remoteMediaKind(res.Remote.Name)
	if kind != "image" && kind != "video" {
		return DownloadGuardDecision{}, false
	}
	if strings.EqualFold(res.Remote.Source, "MEGA") && !megaRemoteAvailable {
		return DownloadGuardDecision{}, false
	}
	candidates := mediaGuardCandidates(res.Remote, entries, 5)
	if len(candidates) == 0 {
		return DownloadGuardDecision{}, false
	}
	target, err := remoteTarget(a, res)
	if err != nil {
		return DownloadGuardDecision{}, false
	}
	bestScore := -1
	bestPath := ""
	bestNote := ""
	bestQuality := ""
	for _, candidate := range candidates {
		if ctx.Err() != nil {
			break
		}
		score := 0
		note := ""
		quality := ""
		if kind == "image" {
			a.mu.RLock()
			mb := a.cfg.VisualImageMaxMB
			a.mu.RUnlock()
			if mb <= 0 {
				mb = 25
			}
			score, err = imageVisualScore(ctx, target, candidate.Path, int64(mb)<<20)
			note = "fingerprint perceptual imagine"
		} else {
			var ri, li MediaInfo
			score, note, ri, li, err = a.visualVideoScoreV85(ctx, target, candidate.Path)
			quality = mediaQualityHint(ri, li)
		}
		if err != nil {
			continue
		}
		if score > bestScore {
			bestScore, bestPath, bestNote, bestQuality = score, candidate.Path, note, quality
		}
	}
	if bestScore < 85 || bestPath == "" {
		return DownloadGuardDecision{}, false
	}

	d := DownloadGuardDecision{ResultID: res.ID, Name: res.Remote.Name, Verdict: guardReview, LocalPath: bestPath, Candidates: len(candidates), Similarity: bestScore, QualityHint: bestQuality}
	switch {
	case bestScore >= 98:
		d.Method = "media-same-content"
		d.Reason = fmt.Sprintf("Aceeași imagine/video este indicată foarte puternic de fingerprint-ul media: %d%% (%s). Fișierul poate fi recodat, redimensionat sau recomprimat.", bestScore, bestNote)
	case bestScore >= 94:
		d.Method = "media-version"
		d.Reason = fmt.Sprintf("Pare o altă versiune a aceluiași material: %d%% similaritate (%s).", bestScore, bestNote)
	case bestScore >= 89:
		d.Method = "media-looks-same"
		d.Reason = fmt.Sprintf("Pare același material, dar nu există suficiente dovezi pentru blocare automată: %d%% (%s).", bestScore, bestNote)
	default:
		d.Method = "media-possible"
		d.Reason = fmt.Sprintf("Există o asemănare media relevantă de %d%% (%s); verifică manual.", bestScore, bestNote)
	}
	return decorateGuardDecision(d), true
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}
