package main

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"math"
	"math/bits"
	"os"
	"os/exec"
	"path/filepath"
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
	case "metadata-incomplete", "mega-busy", "remote-unavailable", "full-sha256-error", "sample-error", "media-tools-missing", "media-index-incomplete", "image-index-incomplete", "media-unverified":
		d.UserStatus = userUnverified
		d.Action = actionRetry
	}
	return d
}

func mediaReviewDecisionV85(res Result, method, reason string, candidates int, localPath string) (DownloadGuardDecision, bool) {
	return decorateGuardDecision(guardReviewDecision(res, method, reason, candidates, localPath)), true
}

func downloadHistoryDecision(res Result) (DownloadGuardDecision, bool) {
	if persistent, ok := persistentDownloadHistoryDecisionV85(res); ok {
		return persistent, true
	}
	// Backward-compatible fallback for old result files that may already carry
	// DownloadedAt/DownloadPath even before the persistent registry existed.
	path := strings.TrimSpace(res.DownloadPath)
	if res.DownloadedAt <= 0 || path == "" {
		return DownloadGuardDecision{}, false
	}
	st, err := os.Stat(path)
	if err != nil || st.IsDir() {
		return DownloadGuardDecision{}, false
	}
	if res.Remote.Size > 0 && !res.Remote.ApproxSize && st.Size() != res.Remote.Size {
		return DownloadGuardDecision{}, false
	}
	if st.ModTime().Unix() > res.DownloadedAt+5 {
		return DownloadGuardDecision{}, false
	}
	d := DownloadGuardDecision{
		ResultID:   res.ID,
		Name:       res.Remote.Name,
		Verdict:    guardDuplicate,
		Reason:     "Acest rezultat a fost descărcat anterior de aplicație, iar fișierul rezultat există încă neschimbat la calea înregistrată.",
		LocalPath:  path,
		Method:     "download-history",
		Candidates: 1,
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

func mediaEntryCountV85(entries []FileEntry, kind string) int {
	n := 0
	for _, e := range entries {
		if remoteMediaKind(e.Name) == kind {
			n++
		}
	}
	return n
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

type videoFingerprintV85 struct {
	Info   MediaInfo
	Hashes []uint64
	Valid  []bool
}

// frameSignatureV85 filters low-information frames before they enter dHash
// scoring. Flat black/white/fade frames otherwise create deceptively similar
// hashes across unrelated videos.
func frameSignatureV85(ctx context.Context, ff, target string, sec float64) (uint64, bool, error) {
	args := []string{"-v", "error", "-ss", fmt.Sprintf("%.3f", sec), "-i", target, "-frames:v", "1", "-vf", "scale=9:8:flags=fast_bilinear,format=gray", "-f", "rawvideo", "-pix_fmt", "gray", "pipe:1"}
	cmd := exec.CommandContext(ctx, ff, args...)
	hideChildWindow(cmd)
	b, err := cmd.Output()
	if err != nil {
		return 0, false, err
	}
	if len(b) < 72 {
		return 0, false, fmt.Errorf("cadru incomplet: %d/72 bytes", len(b))
	}
	var h uint64
	bit := 0
	minV, maxV := 255, 0
	sum, sumSq := 0.0, 0.0
	for i := 0; i < 72; i++ {
		v := int(b[i])
		if v < minV {
			minV = v
		}
		if v > maxV {
			maxV = v
		}
		sum += float64(v)
		sumSq += float64(v * v)
	}
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			if b[y*9+x] > b[y*9+x+1] {
				h |= uint64(1) << bit
			}
			bit++
		}
	}
	mean := sum / 72
	variance := sumSq/72 - mean*mean
	if variance < 0 {
		variance = 0
	}
	std := math.Sqrt(variance)
	informative := maxV-minV >= 24 || std >= 7
	return h, informative, nil
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
		h, informative, err := frameSignatureV85(ctx, ff, target, ri.Duration*p)
		if err != nil || !informative {
			continue
		}
		out.Hashes[i], out.Valid[i] = h, true
		valid++
	}
	if valid < 4 {
		return videoFingerprintV85{}, fmt.Errorf("prea puține cadre informative remote: %d/7", valid)
	}
	return out, nil
}

func (a *App) scoreLocalVideoFingerprintV85(ctx context.Context, remoteFP videoFingerprintV85, candidate FileEntry) (int, string, MediaInfo, error) {
	localFP, err := a.buildLocalVideoFingerprintV85(ctx, candidate)
	if err != nil {
		return 0, "", localFP.Info, err
	}
	li := localFP.Info
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
	for i := range v85FramePoints {
		if i >= len(remoteFP.Valid) || i >= len(localFP.Valid) || !remoteFP.Valid[i] || !localFP.Valid[i] {
			continue
		}
		d := bits.OnesCount64(remoteFP.Hashes[i] ^ localFP.Hashes[i])
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
		return 0, "", li, fmt.Errorf("prea puține cadre informative comparabile: %d/7", matched)
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

	note := fmt.Sprintf("%d/7 cadre informative • %d foarte apropiate • durată Δ %.2fs", matched, highMatches, delta)
	return score, note, li, nil
}

func (a *App) visualVideoScoreV85(ctx context.Context, target, local string) (int, string, MediaInfo, MediaInfo, error) {
	remoteFP, err := a.buildRemoteVideoFingerprintV85(ctx, target)
	if err != nil {
		return 0, "", MediaInfo{}, MediaInfo{}, err
	}
	st, err := os.Stat(local)
	if err != nil {
		return 0, "", remoteFP.Info, MediaInfo{}, err
	}
	candidate := FileEntry{Path: local, Name: filepath.Base(local), Size: st.Size(), MTime: st.ModTime().UnixNano()}
	score, note, li, err := a.scoreLocalVideoFingerprintV85(ctx, remoteFP, candidate)
	if flushErr := flushLocalVideoFingerprintCacheV85(a); flushErr != nil {
		a.logf("Smart Media Guard: nu am putut salva cache-ul fingerprint video: %v", flushErr)
	}
	return score, note, remoteFP.Info, li, err
}

func remoteImageSignatureV85(ctx context.Context, target string, max int64) (imageSignatureV85, error) {
	rb, err := fetchAllLimit(ctx, target, max)
	if err != nil {
		return imageSignatureV85{}, err
	}
	img, _, err := image.Decode(bytes.NewReader(rb))
	if err != nil {
		return imageSignatureV85{}, fmt.Errorf("imagine remote: %w", err)
	}
	return makeImageSignatureV85(img), nil
}

func remoteImageDHashV85(ctx context.Context, target string, max int64) (uint64, error) {
	sig, err := remoteImageSignatureV85(ctx, target, max)
	return sig.Hash, err
}

func localImageDHashV85(path string) (uint64, error) {
	sig, err := readLocalImageSignatureV85(path)
	return sig.Hash, err
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
	// Bitrate is only a useful quality hint when the video codec is the same.
	// HEVC/AV1 can legitimately look better than H.264 at a lower bitrate.
	if remote.BitRate > 0 && local.BitRate > 0 && remote.VideoCodec != "" && strings.EqualFold(remote.VideoCodec, local.VideoCodec) {
		if remote.BitRate >= local.BitRate*3/2 {
			return "remote"
		}
		if local.BitRate >= remote.BitRate*3/2 {
			return "local"
		}
	}
	return ""
}

func mediaQualityReason(hint string) string {
	switch hint {
	case "remote":
		return " Versiunea remote pare mai bună după rezoluție sau bitrate comparabil în același codec; recomandare: REMOTE E MAI BUN."
	case "local":
		return " Versiunea locală pare mai bună după rezoluție sau bitrate comparabil în același codec; recomandare: AI DEJA VERSIUNEA MAI BUNĂ."
	default:
		return ""
	}
}

func (a *App) scoreImageCandidatesV85(ctx context.Context, target string, remote RemoteItem, entries, candidates []FileEntry) (int, string, string, int, error) {
	a.mu.RLock()
	mb := a.cfg.VisualImageMaxMB
	a.mu.RUnlock()
	if mb <= 0 {
		mb = 25
	}
	remoteSig, err := remoteImageSignatureV85(ctx, target, int64(mb)<<20)
	if err != nil {
		return -1, "", "", 0, err
	}
	search := a.imageCandidatesCachedV85(ctx, remoteSig, remote, entries, candidates, 7)
	note := fmt.Sprintf("semnătură perceptuală imagine • cache %d • analizate %d", search.Cached, search.Probed)
	return search.BestScore, search.BestPath, note, search.Pending, nil
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
		score, note, li, err := a.scoreLocalVideoFingerprintV85(ctx, remoteFP, candidate)
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
	localCount := mediaEntryCountV85(entries, kind)
	if localCount == 0 {
		return DownloadGuardDecision{}, false
	}
	if strings.EqualFold(res.Remote.Source, "MEGA") && !megaRemoteAvailable {
		return mediaReviewDecisionV85(res, "mega-busy", "Există fișiere locale de același tip, dar sesiunea MEGA este ocupată. Nu declar fișierul nou fără verificarea media.", localCount, res.LocalPath)
	}
	target, err := remoteTarget(a, res)
	if err != nil {
		return mediaReviewDecisionV85(res, "remote-unavailable", "Nu pot accesa conținutul remote pentru verificarea media: "+err.Error(), localCount, res.LocalPath)
	}

	candidates := mediaGuardCandidates(res.Remote, entries, 5)
	bestScore := -1
	bestPath := ""
	bestNote := ""
	bestQuality := ""
	pending := 0

	if kind == "image" {
		bestScore, bestPath, bestNote, pending, err = a.scoreImageCandidatesV85(ctx, target, res.Remote, entries, candidates)
		if err != nil {
			return mediaReviewDecisionV85(res, "media-unverified", "Fingerprint-ul imaginii remote nu a putut fi calculat: "+err.Error(), localCount, res.LocalPath)
		}
		if bestScore < 85 && pending > 0 {
			return mediaReviewDecisionV85(res, "image-index-incomplete", fmt.Sprintf("Mai există %d imagini locale fără semnătură perceptuală validată. Cache-ul se completează progresiv; nu declar fișierul nou până nu pot exclude imaginile complet redenumite.", pending), localCount, bestPath)
		}
	} else {
		if a.detectFFmpeg() == "" || a.detectFFprobe() == "" {
			return mediaReviewDecisionV85(res, "media-tools-missing", "Pentru a exclude un video recodat sau complet redenumit sunt necesare FFmpeg și ffprobe. Instalează-le din Tool Manager și reîncearcă.", localCount, res.LocalPath)
		}
		pruneLocalVideoFingerprintCacheV85(a, entries)
		defer func() {
			if flushErr := flushLocalVideoFingerprintCacheV85(a); flushErr != nil {
				a.logf("Smart Media Guard: nu am putut salva cache-ul fingerprint video: %v", flushErr)
			}
		}()
		remoteFP, fpErr := a.buildRemoteVideoFingerprintV85(ctx, target)
		if fpErr != nil {
			return mediaReviewDecisionV85(res, "media-unverified", "Fingerprint-ul video remote nu a putut fi calculat sigur: "+fpErr.Error(), localCount, res.LocalPath)
		}
		bestScore, bestPath, bestNote, bestQuality = a.scoreVideoCandidatesV85(ctx, remoteFP, candidates)
		if bestScore < 85 && ctx.Err() == nil {
			search := a.videoDurationCandidatesCached(ctx, remoteFP.Info, res.Remote, entries, candidates, 7)
			candidates = search.Candidates
			pending = search.Pending
			bestScore, bestPath, bestNote, bestQuality = a.scoreVideoCandidatesV85(ctx, remoteFP, candidates)
			if bestScore < 85 && pending > 0 {
				return mediaReviewDecisionV85(res, "media-index-incomplete", fmt.Sprintf("Mai există %d videoclipuri locale fără metadate media validate. Au fost analizate %d acum; cache-ul se completează progresiv. Nu declar fișierul nou până nu pot exclude un re-encode complet redenumit.", pending, search.Probed), localCount, bestPath)
			}
		}
	}
	if ctx.Err() != nil {
		return mediaReviewDecisionV85(res, "media-unverified", "Verificarea media a fost întreruptă înainte de un verdict sigur.", localCount, bestPath)
	}
	if bestScore < 85 || bestPath == "" {
		return DownloadGuardDecision{}, false
	}

	d := DownloadGuardDecision{ResultID: res.ID, Name: res.Remote.Name, Verdict: guardReview, LocalPath: bestPath, Candidates: len(candidates), Similarity: bestScore, QualityHint: bestQuality}
	switch {
	case bestScore >= 98:
		d.Method = "media-same-content"
		d.Reason = fmt.Sprintf("Aceeași imagine/video este indicată foarte puternic de fingerprint-ul media: %d%% (%s). Fișierul poate fi recodat, redimensionat sau recomprimat.%s", bestScore, bestNote, mediaQualityReason(bestQuality))
	case bestScore >= 94:
		d.Method = "media-version"
		d.Reason = fmt.Sprintf("Pare o altă versiune a aceluiași material: %d%% similaritate (%s).%s", bestScore, bestNote, mediaQualityReason(bestQuality))
	case bestScore >= 89:
		d.Method = "media-looks-same"
		d.Reason = fmt.Sprintf("Pare același material, dar nu există suficiente dovezi pentru blocare automată: %d%% (%s).%s", bestScore, bestNote, mediaQualityReason(bestQuality))
	default:
		d.Method = "media-possible"
		d.Reason = fmt.Sprintf("Există o asemănare media relevantă de %d%% (%s); verifică manual.%s", bestScore, bestNote, mediaQualityReason(bestQuality))
	}
	return decorateGuardDecision(d), true
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}
