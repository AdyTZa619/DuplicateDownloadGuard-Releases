package main

import (
	"context"
	"fmt"
)

func (a *App) finalizeVideoMediaDecisionV85(ctx context.Context, target string, d DownloadGuardDecision, remoteInfo, localInfo MediaInfo, secondScore int, bestNote string) DownloadGuardDecision {
	audio := audioFingerprintResultV85{}
	if d.Similarity >= 98 && d.LocalPath != "" {
		audio = a.audioVariantScoreV85(ctx, target, remoteInfo, d.LocalPath, localInfo)
	}
	method, extra := resolveVideoEvidenceV85(d.Similarity, secondScore, remoteInfo, localInfo, audio)
	d.Method = method
	switch method {
	case "media-same-content":
		d.Reason = fmt.Sprintf("Același material video este indicat foarte puternic de fingerprint-ul cadrelor: %d%% (%s)%s. Fișierul poate fi recodat sau recomprimat.%s", d.Similarity, bestNote, extra, mediaQualityReason(d.QualityHint))
	case "media-version":
		d.Reason = fmt.Sprintf("Pare o altă versiune a aceluiași material video: %d%% similaritate vizuală (%s)%s.%s", d.Similarity, bestNote, extra, mediaQualityReason(d.QualityHint))
	case "media-looks-same":
		d.Reason = fmt.Sprintf("Video-ul pare același material, dar nu există suficiente dovezi pentru blocare automată: %d%% (%s)%s.%s", d.Similarity, bestNote, extra, mediaQualityReason(d.QualityHint))
	default:
		d.Method = "media-possible"
		d.Reason = fmt.Sprintf("Există o asemănare video relevantă de %d%% (%s)%s; verifică manual.%s", d.Similarity, bestNote, extra, mediaQualityReason(d.QualityHint))
	}
	return decorateGuardDecision(d)
}
