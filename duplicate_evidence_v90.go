package main

import (
	"fmt"
	"math"
	"strings"
)

type DuplicateEvidenceV90 struct {
	Classification string     `json:"classification"`
	Score          *int       `json:"score"`
	Basis          string     `json:"basis"`
	Signals        []string   `json:"signals"`
	Remote         *MediaInfo `json:"remote,omitempty"`
	Local          *MediaInfo `json:"local,omitempty"`
	Quality        string     `json:"quality,omitempty"`
	RemoteBytes    int64      `json:"remoteBytes"`
	RemoteCacheHit bool       `json:"remoteCacheHit"`
	DeepAnalyzed   int        `json:"deepAnalyzed"`
	LocalCacheHits int        `json:"localCacheHits"`
	Pending        int        `json:"pending"`
}

func detectorClassificationV90(d DownloadGuardDecision) string {
	if d.Exact {
		return "EXACT"
	}
	if d.Similarity >= 95 {
		return "FOARTE PROBABIL ACELAȘI CONȚINUT"
	}
	if d.Similarity >= 85 {
		return "PROBABIL"
	}
	if d.Similarity >= 60 {
		return "SIMILAR"
	}
	if d.Verdict == guardDownload {
		return "DIFERIT"
	}
	return "NECUNOSCUT / DATE INSUFICIENTE"
}

func decorateDetectorEvidenceV90(d DownloadGuardDecision) DownloadGuardDecision {
	if d.Detector == nil {
		d.Detector = &DuplicateEvidenceV90{}
	}
	e := d.Detector
	e.Classification = detectorClassificationV90(d)
	if d.Exact {
		score := 100
		e.Score = &score
		e.Basis = "Hash criptografic complet; numele și folderul nu sunt criterii de confirmare."
	} else if d.Similarity > 0 {
		score := min(99, d.Similarity)
		e.Score = &score
		e.Basis = "Scor de similaritate măsurat, nu probabilitate statistică; 100 este rezervat identității exacte."
	} else {
		e.Score = nil
		e.Basis = "Nu există suficiente semnale pentru un scor de similaritate."
	}
	if len(e.Signals) == 0 && d.Reason != "" {
		e.Signals = []string{d.Reason}
	}
	return d
}

func mediaTechnicalEvidenceV90(remote, local MediaInfo) (string, []string) {
	if !remote.OK || !local.OK {
		return "DATE INSUFICIENTE", nil
	}
	signals := []string{
		fmt.Sprintf("Durată: online %.3fs / local %.3fs; diferență %.3fs", remote.Duration, local.Duration, math.Abs(remote.Duration-local.Duration)),
		fmt.Sprintf("Rezoluție: online %d×%d / local %d×%d", remote.Width, remote.Height, local.Width, local.Height),
		fmt.Sprintf("Codec: online %s / local %s; bitrate: %d / %d bit/s", remote.VideoCodec, local.VideoCodec, remote.BitRate, local.BitRate),
		fmt.Sprintf("Cadre/s: online %s / local %s; audio: %s / %s", remote.FPS, local.FPS, remote.AudioCodec, local.AudioCodec),
	}
	quality := "CALITATE NEDETERMINATĂ"
	switch mediaQualityHint(remote, local) {
	case "local":
		quality = "LOCAL MAI BUN TEHNIC"
	case "remote":
		quality = "ONLINE MAI BUN TEHNIC"
	}
	if remote.Width == local.Width && remote.Height == local.Height && remote.FPS == local.FPS &&
		strings.EqualFold(remote.VideoCodec, local.VideoCodec) && strings.EqualFold(remote.AudioCodec, local.AudioCodec) &&
		remote.BitRate > 0 && local.BitRate > 0 && math.Abs(float64(remote.BitRate-local.BitRate))/float64(max(remote.BitRate, local.BitRate)) < .1 && math.Abs(remote.Duration-local.Duration) < .5 {
		quality = "ECHIVALENTE TEHNIC"
	}
	return quality, signals
}
