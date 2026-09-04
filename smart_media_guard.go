package main

import (
	"bytes"
	"context"
	"fmt"
	"image"
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

var v85FramePoints = []float64{.08, .20, .35, .50, .65, .80, .92}

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
	case "download-history":
		d.UserStatus = userDownloaded
		d.Action = actionDontDownload
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

func downloadHistoryDecision(res Result) (DownloadGuardDecision, bool) {
	path := strings.TrimSpace(res.DownloadPath)
	if res.DownloadedAt <= 0 || path == "" || !fileExists(path) {
		return DownloadGuardDecision{}, false
	}
	d := DownloadGuardDecision{
		ResultID:   res.ID,
		Name:       res.Remote.Name,
		Verdict:    guardDuplicate,
		Reason:     "Acest rezultat a fost descărcat anterior de aplicație, iar fișierul rezultat există încă pe disc.",
		LocalPath:  path,
		Method:     "download-history",
		Candidates: 1,
		Exact:      true,
	}
	return decorateGuardDecision(d), true
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

func hasEntryPath(rows []FileEntry, path string) bool {
	for _, row := range rows {
		if pathKey(row.Path) == pathKey(path) {
			return true
		}
	}
	return false
}

type durationGuardCandidate struct {
	Entry         FileEntry
	DurationRatio float64
	NameScore     int
	SizeRatio     float64
}

func (a *App) videoDurationCandidates(ctx context.Context, remoteInfo MediaInfo, remote RemoteItem, entries, existing []FileEntry, limit int) []FileEntry {
	if limit <= 0 {
		limit = 7
	}
	fp := a.detectFFprobe()
	if fp == "" || !remoteInfo.OK || remoteInfo.Duration <= 0 {
		return existing
	}

	type rough struct {
		Entry     FileEntry
		NameScore int
		SizeRatio float64
		Rank      int
	}
	roughRows := make([]rough, 0, 128)
	for _, e := range entries {
		if remoteMediaKind(e.Name) != "video" || hasEntryPath(existing, e.Path) {
			continue
		}
		nameScore := nameSimilarity(remote.Name, e.Name)
		sizeRatio := 1.0
		if remote.Size > 0 {
			sizeRatio = float64(abs64(e.Size-remote.Size)) / float64(remote.Size)
		}
		closeness := int(math.Round(1000 / (1 + sizeRatio*8)))
		rank := closeness + nameScore*8
		if strings.EqualFold(filepathExt(remote.Name), filepathExt(e.Name)) {
			rank += 80
		}
		roughRows = append(roughRows, rough{Entry: e, NameScore: nameScore, SizeRatio: sizeRatio, Rank: rank})
	}
	sort.SliceStable(roughRows, func(i, j int) bool {
		if roughRows[i].Rank != roughRows[j].Rank {
			return roughRows[i].Rank > roughRows[j].Rank
		}
		return strings.ToLower(roughRows[i].Entry.Path) < strings.ToLower(roughRows[j].Entry.Path)
	})
	if len(roughRows) > 28 {
		roughRows = roughRows[:28]
	}

	matched := make([]durationGuardCandidate, 0, 8)
	for _, row := range roughRows {
		if ctx.Err() != nil {
			break
		}
		li := probeMedia(ctx, fp, row.Entry.Path, "LOCAL")
		if !li.OK || li.Duration <= 0 {
			continue
		}
		maxD := math.Max(remoteInfo.Duration, li.Duration)
		durationRatio := math.Abs(remoteInfo.Duration-li.Duration) / maxD
		if durationRatio > .015 && math.Abs(remoteInfo.Duration-li.Duration) > 1.5 {
			continue
		}
		if remoteInfo.Width > 0 && remoteInfo.Height > 0 && li.Width > 0 && li.Height > 0 {
			ra := float64(remoteInfo.Width) / float64(remoteInfo.Height)
			la := float64(li.Width) / float64(li.Height)
			aspectDelta := math.Abs(ra-la) / math.Max(ra, la)
			if aspectDelta > .08 {
				continue
			}
		}
		matched = append(matched, durationGuardCandidate{Entry: row.Entry, DurationRatio: durationRatio, NameScore: row.NameScore, SizeRatio: row.SizeRatio})
	}
	sort.SliceStable(matched, func(i, j int) bool {
		if matched[i].DurationRatio != matched[j].DurationRatio {
			return matched[i].DurationRatio < matched[j].DurationRatio
		}
		if matched[i].NameScore != matched[j].NameScore {
			return matched[i].NameScore > matched[j].NameScore
		}
		if matched[i].SizeRatio != matched[j].SizeRatio {
			return matched[i].SizeRatio < matched[j].SizeRatio
		}
		return strings.ToLower(matched[i].Entry.Path) < strings.ToLower(matched[j].Entry.Path)
	})

	out := append([]FileEntry(nil), existing...)
	for _, row := range matched {
		if len(out) >= limit {
			break
		}
		if !hasEntryPath(out, row.Entry.Path) {
			out = append(out, row.Entry)
		}
	}
	return out
}

type videoFingerprintV85 struct {
	Info   MediaInfo
	Hashes []uint64
	Valid  []bool
}

func (a *App) buildRemoteVideoFingerprintV85(ctx context.Context, target string) (videoFingerprintV85, error) {
	ff := a.detectFFmpeg()
	fp := a.detectFFprobe()
	if ff == "" || fp == "" {
		return videoFingerprintV85{}, fmt.Errorf("ffmpeg + ffprobe lipsesc")
	}
	ri := probeMedia(ctx, fp, target, "REMOTE")
	if !ri.OK || ri.Duration <= 0 {
		return videoFingerprintV85{}, fmt.Errorf("nu pot citi durata videoclipului remote")
	}
	out := videoFingerprintV85{Info: ri, Hashes: make([]uint64, len(v85FramePoints)), Valid: make([]bool, len(v85FramePoints))}
	valid := 0
	for i, p := range v85FramePoints {
		h, err := frameHash(ctx, ff, target, ri.Duration*p)
		if err != nil {
			continue
		}
		out.Hashes[i], out.Valid[i] = h, true
		valid++
	}
	if valid < 4 {
		return videoFingerprintV85{}, fmt.Errorf("prea puține cadre remote disponibile: %d/7", valid)
	}
	return out, nil
}

func (a *App) scoreLocalVideoFingerprintV85(ctx context.Context, remoteFP videoFingerprintV85, local string) (int, string, MediaInfo, error) {
	ff := a.detectFFmpeg()
	fp := a.detectFFprobe()
	if ff == "" || fp == "" {
		return 0, "", MediaInfo{}, fmt.Errorf("ffmpeg + ffprobe lipsesc")
	}
	li := probeMedia(ctx, fp, local, "LOCAL")
	if !li.OK || li.Duration <= 0 {
		return 0, "", li, fmt.Errorf("nu pot citi durata videoclipului local")
	}
	ri := remoteFP.Info
	maxD := math.Max(ri.Duration, li.Duration)
	delta := math.Abs(ri.Duration - li.Duration)
	durationRatio := delta / maxD
	if durationRatio > .35 {
		return 0, "", li, fmt.Errorf("duratele diferă prea mult: %.1f%%", durationRatio*100)
	}

	sum := 0
	matched := 0
	highMatches := 0
	veryHighMatches := 0
	for i, p := range v85FramePoints {
		if i >= len(remoteFP.Valid) || !remoteFP.Valid[i] {
			continue
		}
		lh, err := frameHash(ctx, ff, local, li.Duration*p)
		if err != nil {
			continue
		}
		d := bits.OnesCount64(remoteFP.Hashes[i] ^ lh)
		frameScore := int(math.Round(float64(64-d) * 100 / 64))
		sum += frameScore
		matched++
		if frameScore >= 90 {
			highMatches++
		}
		if frameScore >= 95 {
			veryHighMatches++
		}
	}
	if matched < 4 {
		return 0, "", li, fmt.Errorf("prea puține cadre comparabile: %d/7", matched)
	}

	score := int(math.Round(float64(sum) / float64(matched)))
	if highMatches < 4 && score > 88 {
		score = 88
	}
	if veryHighMatches < 4 && score > 93 {
		score = 93
	}
	if durationRatio > .12 && score > 88 {
		score = 88
	} else if durationRatio > .03 && score > 97 {
		score = 97
	}

	note := fmt.Sprintf("%d/7 cadre • %d foarte apropiate • durată Δ %.2fs", matched, highMatches, delta)
	return score, note, li, nil
}

func (a *App) visualVideoScoreV85(ctx context.Context, target, local string) (int, string, MediaInfo, MediaInfo, error) {
	remoteFP, err := a.buildRemoteVideoFingerprintV85(ctx, target)
	if err != nil {
		return 0, "", MediaInfo{}, MediaInfo{}, err
	}
	score, note, li, err := a.scoreLocalVideoFingerprintV85(ctx, remoteFP, local)
	return score, note, remoteFP.Info, li, err
}

func remoteImageDHashV85(ctx context.Context, target string, max int64) (uint64, error) {
	rb, err := fetchAllLimit(ctx, target, max)
	if err != nil {
		return 0, err
	}
	img, _, err := image.Decode(bytes.NewReader(rb))
	if err != nil {
		return 0, fmt.Errorf("imagine remote: %w", err)
	}
	return dhashImage(img), nil
}

func localImageDHashV85(path string) (uint64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return 0, fmt.Errorf("imagine locală: %w", err)
	}
	return dhashImage(img), nil
}

func imageHashSimilarityV85(remoteHash, localHash uint64) int {
	d := bits.OnesCount64(remoteHash ^ localHash)
	return int(math.Round(float64(64-d) * 100 / 64))
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

func (a *App) scoreImageCandidatesV85(ctx context.Context, target string, candidates []FileEntry) (int, string, string) {
	a.mu.RLock()
	mb := a.cfg.VisualImageMaxMB
	a.mu.RUnlock()
	if mb <= 0 {
		mb = 25
	}
	remoteHash, err := remoteImageDHashV85(ctx, target, int64(mb)<<20)
	if err != nil {
		return -1, "", ""
	}
	bestScore := -1
	bestPath := ""
	for _, candidate := range candidates {
		if ctx.Err() != nil {
			break
		}
		localHash, err := localImageDHashV85(candidate.Path)
		if err != nil {
			continue
		}
		score := imageHashSimilarityV85(remoteHash, localHash)
		if score > bestScore {
			bestScore, bestPath = score, candidate.Path
		}
	}
	return bestScore, bestPath, "fingerprint perceptual imagine"
}

func (a *App) scoreVideoCandidatesV85(ctx context.Context, remoteFP videoFingerprintV85, candidates []FileEntry) (int, string, string, string) {
	bestScore := -1
	bestPath := ""
	bestNote := ""
	bestQuality := ""
	for _, candidate := range candidates {
		if ctx.Err() != nil {
			break
		}
		score, note, li, err := a.scoreLocalVideoFingerprintV85(ctx, remoteFP, candidate.Path)
		if err != nil {
			continue
		}
		quality := mediaQualityHint(remoteFP.Info, li)
		if score > bestScore {
			bestScore, bestPath, bestNote, bestQuality = score, candidate.Path, note, quality
		}
	}
	return bestScore, bestPath, bestNote, bestQuality
}

func (a *App) mediaNearDuplicateDecision(ctx context.Context, res Result, entries []FileEntry, megaRemoteAvailable bool) (DownloadGuardDecision, bool) {
	kind := remoteMediaKind(res.Remote.Name)
	if kind != "image" && kind != "video" {
		return DownloadGuardDecision{}, false
	}
	if strings.EqualFold(res.Remote.Source, "MEGA") && !megaRemoteAvailable {
		return DownloadGuardDecision{}, false
	}
	target, err := remoteTarget(a, res)
	if err != nil {
		return DownloadGuardDecision{}, false
	}

	candidates := mediaGuardCandidates(res.Remote, entries, 5)
	bestScore := -1
	bestPath := ""
	bestNote := ""
	bestQuality := ""
	if kind == "image" {
		bestScore, bestPath, bestNote = a.scoreImageCandidatesV85(ctx, target, candidates)
	} else {
		remoteFP, err := a.buildRemoteVideoFingerprintV85(ctx, target)
		if err != nil {
			return DownloadGuardDecision{}, false
		}
		bestScore, bestPath, bestNote, bestQuality = a.scoreVideoCandidatesV85(ctx, remoteFP, candidates)
		if bestScore < 85 && ctx.Err() == nil {
			candidates = a.videoDurationCandidatesCached(ctx, remoteFP.Info, res.Remote, entries, candidates, 7)
			bestScore, bestPath, bestNote, bestQuality = a.scoreVideoCandidatesV85(ctx, remoteFP, candidates)
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
