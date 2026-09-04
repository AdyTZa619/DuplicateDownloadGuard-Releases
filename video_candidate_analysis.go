package main

import (
	"context"
	"fmt"
	"math"
	"strings"
)

type videoCandidateAnalysisV85 struct {
	BestScore   int
	SecondScore int
	BestPath    string
	BestNote    string
	BestQuality string
	BestInfo    MediaInfo
}

// scoreVideoCandidatesDetailedV85 keeps the runner-up score as well as the
// winning candidate. The runner-up is useful as a safety signal: a marginal
// winner among several visually similar files should not be treated as a
// definitive same-content match.
func (a *App) scoreVideoCandidatesDetailedV85(ctx context.Context, remoteFP videoFingerprintV85, candidates []FileEntry) videoCandidateAnalysisV85 {
	out := videoCandidateAnalysisV85{BestScore: -1, SecondScore: -1}
	for _, candidate := range candidates {
		if ctx.Err() != nil {
			break
		}
		score, note, li, err := a.scoreLocalVideoFingerprintV85(ctx, remoteFP, candidate)
		if err != nil {
			continue
		}
		quality := mediaQualityHint(remoteFP.Info, li)
		if score > out.BestScore {
			out.SecondScore = out.BestScore
			out.BestScore = score
			out.BestPath = candidate.Path
			out.BestNote = note
			out.BestQuality = quality
			out.BestInfo = li
		} else if score > out.SecondScore {
			out.SecondScore = score
		}
	}
	return out
}

func videoBaseMethodV85(score int) string {
	switch {
	case score >= 98:
		return "media-same-content"
	case score >= 94:
		return "media-version"
	case score >= 89:
		return "media-looks-same"
	default:
		return "media-possible"
	}
}

func meaningfulVideoDurationDeltaV85(remoteInfo, localInfo MediaInfo) (float64, float64, bool) {
	if !remoteInfo.OK || !localInfo.OK || remoteInfo.Duration <= 0 || localInfo.Duration <= 0 {
		return 0, 0, false
	}
	delta := math.Abs(remoteInfo.Duration - localInfo.Duration)
	ratio := delta / math.Max(remoteInfo.Duration, localInfo.Duration)
	// Container/timestamp rounding can differ slightly after a recode. More than
	// three seconds and 0.1% of the material is large enough to treat as a
	// possible intro/outro/cut rather than silently auto-blocking it.
	return delta, ratio, delta > 3 && ratio > .001
}

// resolveVideoEvidenceV85 is deliberately conservative. Visual fingerprints
// are the primary evidence. Audio can only keep or downgrade a strong visual
// match; it can never promote a weaker visual match into a duplicate.
func resolveVideoEvidenceV85(visualScore, secondScore int, remoteInfo, localInfo MediaInfo, audio audioFingerprintResultV85) (method, extra string) {
	method = videoBaseMethodV85(visualScore)
	if visualScore < 98 {
		if audio.Available {
			extra = " • " + audio.Note
		}
		return method, extra
	}

	// A 98/98 or 99/98 race can happen with near-static/generic footage. Keep
	// the result review-only unless the winner is perfect or clearly ahead.
	if visualScore < 100 && secondScore >= 96 && visualScore-secondScore <= 1 {
		return "media-looks-same", fmt.Sprintf(" • potrivire ambiguă: al doilea candidat are %d%%", secondScore)
	}

	if delta, _, meaningful := meaningfulVideoDurationDeltaV85(remoteInfo, localInfo); meaningful {
		return "media-version", fmt.Sprintf(" • durată diferită cu %.1fs; poate exista intro/outro sau o tăietură", delta)
	}

	remoteHasAudio := strings.TrimSpace(remoteInfo.AudioCodec) != ""
	localHasAudio := strings.TrimSpace(localInfo.AudioCodec) != ""
	if remoteHasAudio != localHasAudio {
		return "media-version", " • audio diferit: numai una dintre versiuni are pistă audio"
	}
	if !remoteHasAudio {
		// With no soundtrack available as a second independent signal, 98–99%
		// visual similarity is not enough for an automatic block. A perfect,
		// non-ambiguous frame match can still identify a silent re-encode.
		if visualScore < 100 {
			return "media-version", " • ambele versiuni sunt fără audio; 98–99% vizual rămâne review pentru siguranță"
		}
		return "media-same-content", " • ambele versiuni sunt fără audio, iar cadrele informative coincid 100%"
	}

	// Both have audio. If Chromaprint is unavailable, do not silently assume the
	// soundtrack is the same and do not auto-block the download.
	if !audio.Available {
		return "media-looks-same", " • video foarte apropiat, dar audio nu a putut fi verificat perceptual"
	}
	if audio.Score >= 82 {
		return "media-same-content", " • " + audio.Note
	}
	if audio.Score >= 68 {
		return "media-version", " • " + audio.Note + " • pista audio pare o variantă diferită"
	}
	return "media-version", " • " + audio.Note + " • pista audio este diferită"
}
