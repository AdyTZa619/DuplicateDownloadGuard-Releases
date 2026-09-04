package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"math/bits"
	"os/exec"
	"strings"
	"sync"
	"time"
)

var chromaprintSupportV85 = struct {
	sync.Mutex
	ByFFmpeg map[string]bool
}{ByFFmpeg: map[string]bool{}}

func ffmpegHasChromaprintV85(ff string) bool {
	ff = strings.TrimSpace(ff)
	if ff == "" {
		return false
	}
	chromaprintSupportV85.Lock()
	if value, ok := chromaprintSupportV85.ByFFmpeg[ff]; ok {
		chromaprintSupportV85.Unlock()
		return value
	}
	chromaprintSupportV85.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, ff, "-hide_banner", "-muxers")
	hideChildWindow(cmd)
	out, err := cmd.CombinedOutput()
	available := err == nil && strings.Contains(strings.ToLower(string(out)), "chromaprint")
	chromaprintSupportV85.Lock()
	chromaprintSupportV85.ByFFmpeg[ff] = available
	chromaprintSupportV85.Unlock()
	return available
}

func chromaprintSegmentV85(parent context.Context, ff, target string, start, seconds float64) ([]uint32, error) {
	if start < 0 {
		start = 0
	}
	if seconds <= 0 {
		seconds = 12
	}
	ctx, cancel := context.WithTimeout(parent, 25*time.Second)
	defer cancel()
	args := []string{
		"-v", "error",
		"-ss", fmt.Sprintf("%.3f", start),
		"-i", target,
		"-t", fmt.Sprintf("%.3f", seconds),
		"-map", "0:a:0",
		"-ac", "2",
		"-ar", "11025",
		"-c:a", "pcm_s16le",
		"-f", "chromaprint",
		"-fp_format", "raw",
		"pipe:1",
	}
	cmd := exec.CommandContext(ctx, ff, args...)
	hideChildWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	if len(out) < 32 || len(out)%4 != 0 {
		return nil, fmt.Errorf("fingerprint audio prea scurt: %d bytes", len(out))
	}
	rows := make([]uint32, len(out)/4)
	for i := range rows {
		rows[i] = binary.NativeEndian.Uint32(out[i*4 : i*4+4])
	}
	return rows, nil
}

func chromaprintSimilarityV85(a, b []uint32) int {
	if len(a) < 8 || len(b) < 8 {
		return -1
	}
	best := -1
	for shift := -8; shift <= 8; shift++ {
		a0, b0 := 0, 0
		if shift > 0 {
			b0 = shift
		} else if shift < 0 {
			a0 = -shift
		}
		n := len(a) - a0
		if m := len(b) - b0; m < n {
			n = m
		}
		if n < 8 {
			continue
		}
		matchingBits := 0
		for i := 0; i < n; i++ {
			matchingBits += 32 - bits.OnesCount32(a[a0+i]^b[b0+i])
		}
		score := int(math.Round(float64(matchingBits) * 100 / float64(n*32)))
		if score > best {
			best = score
		}
	}
	return best
}

type audioFingerprintResultV85 struct {
	Available bool
	Score     int
	Note      string
}

// audioVariantScoreV85 is intentionally advisory. It is only called after a
// strong visual video match and can downgrade ACELAȘI CONȚINUT to ALTĂ
// VERSIUNE; it never promotes an unrelated video into a duplicate.
func (a *App) audioVariantScoreV85(ctx context.Context, remoteTarget string, remoteInfo MediaInfo, localPath string, localInfo MediaInfo) audioFingerprintResultV85 {
	remoteAudio := strings.TrimSpace(remoteInfo.AudioCodec) != ""
	localAudio := strings.TrimSpace(localInfo.AudioCodec) != ""
	if remoteAudio != localAudio {
		return audioFingerprintResultV85{Available: true, Score: 0, Note: "o versiune are pistă audio, cealaltă nu"}
	}
	if !remoteAudio {
		return audioFingerprintResultV85{}
	}
	ff := a.detectFFmpeg()
	if ff == "" || !ffmpegHasChromaprintV85(ff) || remoteInfo.Duration <= 0 || localInfo.Duration <= 0 {
		return audioFingerprintResultV85{}
	}
	defer func() {
		if err := flushLocalAudioSegmentCacheV85(a); err != nil {
			a.logf("Smart Media Guard: nu am putut salva cache-ul audio: %v", err)
		}
	}()

	const segment = 12.0
	points := []float64{.22, .50, .78}
	remoteSegments := make([][]uint32, len(points))
	validRemote := make([]bool, len(points))
	for i, p := range points {
		center := remoteInfo.Duration * p
		start := math.Max(0, math.Min(center-segment/2, remoteInfo.Duration-segment))
		fp, err := chromaprintSegmentV85(ctx, ff, remoteTarget, start, segment)
		if err == nil {
			remoteSegments[i] = fp
			validRemote[i] = true
		}
	}

	delta := localInfo.Duration - remoteInfo.Duration
	offsets := []float64{0}
	if math.Abs(delta) >= 1.5 && math.Abs(delta) <= 90 {
		offsets = append(offsets, delta/2, delta)
	}
	bestScore := -1
	bestSegments := 0
	bestOffset := 0.0
	for _, offset := range offsets {
		total, matched := 0, 0
		for i, p := range points {
			if !validRemote[i] {
				continue
			}
			center := remoteInfo.Duration*p + offset
			start := math.Max(0, math.Min(center-segment/2, localInfo.Duration-segment))
			localFP, err := a.cachedLocalChromaprintSegmentV85(ctx, ff, localPath, start, segment)
			if err != nil {
				continue
			}
			score := chromaprintSimilarityV85(remoteSegments[i], localFP)
			if score < 0 {
				continue
			}
			total += score
			matched++
		}
		if matched < 2 {
			continue
		}
		score := int(math.Round(float64(total) / float64(matched)))
		if score > bestScore {
			bestScore, bestSegments, bestOffset = score, matched, offset
		}
	}
	if bestScore < 0 {
		return audioFingerprintResultV85{}
	}
	return audioFingerprintResultV85{
		Available: true,
		Score:     bestScore,
		Note:      fmt.Sprintf("audio Chromaprint %d%% • %d segmente • offset %+0.1fs", bestScore, bestSegments, bestOffset),
	}
}
