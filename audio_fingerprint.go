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

const (
	audioPCMSampleRateV901 = 8000
	audioPCMWindowV901     = 2048
	audioPCMHopV901        = 1024
	audioPCMBandsV901      = 20
	audioPCMStrideV901     = audioPCMBandsV901 + 1
)

// pcmAudioSegmentV901 intentionally uses only the standard PCM output offered
// by every supported FFmpeg build. Some Windows FFmpeg distributions omit the
// optional Chromaprint muxer, which previously made otherwise strong video
// matches review-only solely because the audio evidence could not be built.
func pcmAudioSegmentV901(parent context.Context, ff, target string, start, seconds float64) ([]uint32, error) {
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
		"-ac", "1",
		"-ar", fmt.Sprintf("%d", audioPCMSampleRateV901),
		"-c:a", "pcm_s16le",
		"-f", "s16le",
		"pipe:1",
	}
	cmd := exec.CommandContext(ctx, ff, args...)
	hideChildWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return pcmAudioFingerprintV901(out, audioPCMSampleRateV901)
}

// pcmAudioFingerprintV901 stores a normalized spectral profile for each short
// time window. Amplitude is deliberately removed, so codec gain, bitrate and
// container changes do not alter the signature. The representation stays
// numeric (rather than a byte hash) so comparison can tolerate compression
// noise and a small temporal offset.
func pcmAudioFingerprintV901(pcm []byte, sampleRate int) ([]uint32, error) {
	if sampleRate <= 0 || len(pcm) < audioPCMWindowV901*2 {
		return nil, fmt.Errorf("segment PCM prea scurt: %d bytes", len(pcm))
	}
	samples := make([]float64, len(pcm)/2)
	for i := range samples {
		samples[i] = float64(int16(binary.LittleEndian.Uint16(pcm[i*2 : i*2+2])))
	}

	frequencies := make([]float64, audioPCMBandsV901)
	const low, high = 80.0, 3600.0
	for i := range frequencies {
		frequencies[i] = low * math.Pow(high/low, float64(i)/float64(len(frequencies)-1))
	}
	window := make([]float64, audioPCMWindowV901)
	for i := range window {
		window[i] = .5 - .5*math.Cos(2*math.Pi*float64(i)/float64(len(window)-1))
	}

	frames := 1 + (len(samples)-audioPCMWindowV901)/audioPCMHopV901
	out := make([]uint32, 0, frames*audioPCMStrideV901)
	for frame := 0; frame < frames; frame++ {
		base := frame * audioPCMHopV901
		var squareSum float64
		for i := 0; i < audioPCMWindowV901; i++ {
			v := samples[base+i]
			squareSum += v * v
		}
		rms := math.Sqrt(squareSum / audioPCMWindowV901)
		if rms < 24 { // digital silence / encoder floor
			out = append(out, 0)
			out = append(out, make([]uint32, audioPCMBandsV901)...)
			continue
		}

		bands := make([]float64, audioPCMBandsV901)
		var mean float64
		for band, frequency := range frequencies {
			omega := 2 * math.Pi * frequency / float64(sampleRate)
			coeff := 2 * math.Cos(omega)
			q0, q1, q2 := 0.0, 0.0, 0.0
			for i := 0; i < audioPCMWindowV901; i++ {
				q0 = samples[base+i]*window[i] + coeff*q1 - q2
				q2, q1 = q1, q0
			}
			power := q1*q1 + q2*q2 - coeff*q1*q2
			bands[band] = math.Log1p(math.Max(0, power))
			mean += bands[band]
		}
		mean /= audioPCMBandsV901
		var norm float64
		for i := range bands {
			bands[i] -= mean
			norm += bands[i] * bands[i]
		}
		norm = math.Sqrt(norm)
		if norm < 1e-9 {
			out = append(out, 0)
			out = append(out, make([]uint32, audioPCMBandsV901)...)
			continue
		}
		out = append(out, 1)
		for _, value := range bands {
			quantized := int32(math.Round(value / norm * 1000000))
			out = append(out, uint32(quantized))
		}
	}
	if len(out) < 8*audioPCMStrideV901 {
		return nil, fmt.Errorf("fingerprint audio PCM prea scurt: %d cadre", len(out)/audioPCMStrideV901)
	}
	return out, nil
}

func pcmAudioSimilarityV901(a, b []uint32) int {
	if len(a)%audioPCMStrideV901 != 0 || len(b)%audioPCMStrideV901 != 0 {
		return -1
	}
	aFrames, bFrames := len(a)/audioPCMStrideV901, len(b)/audioPCMStrideV901
	if aFrames < 8 || bFrames < 8 {
		return -1
	}
	best := -1
	for shift := -20; shift <= 20; shift++ {
		a0, b0 := 0, 0
		if shift > 0 {
			b0 = shift
		} else if shift < 0 {
			a0 = -shift
		}
		n := aFrames - a0
		if m := bFrames - b0; m < n {
			n = m
		}
		if n < 8 {
			continue
		}
		var total float64
		matched := 0
		for frame := 0; frame < n; frame++ {
			ai := (a0 + frame) * audioPCMStrideV901
			bi := (b0 + frame) * audioPCMStrideV901
			if a[ai] == 0 || b[bi] == 0 {
				continue
			}
			var dot, an, bn float64
			for band := 1; band < audioPCMStrideV901; band++ {
				av := float64(int32(a[ai+band]))
				bv := float64(int32(b[bi+band]))
				dot += av * bv
				an += av * av
				bn += bv * bv
			}
			if an == 0 || bn == 0 {
				continue
			}
			cosine := math.Max(-1, math.Min(1, dot/math.Sqrt(an*bn)))
			total += (cosine + 1) * 50
			matched++
		}
		if matched < 8 {
			continue
		}
		score := int(math.Round(total / float64(matched)))
		if score > best {
			best = score
		}
	}
	return best
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
	if ff == "" || remoteInfo.Duration <= 0 || localInfo.Duration <= 0 {
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
		fp, err := pcmAudioSegmentV901(ctx, ff, remoteTarget, start, segment)
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
			localFP, err := a.cachedLocalAudioFingerprintSegmentV901(ctx, ff, localPath, start, segment)
			if err != nil {
				continue
			}
			score := pcmAudioSimilarityV901(remoteSegments[i], localFP)
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
		Note:      fmt.Sprintf("fingerprint audio PCM %d%% • %d segmente • offset %+0.1fs", bestScore, bestSegments, bestOffset),
	}
}
