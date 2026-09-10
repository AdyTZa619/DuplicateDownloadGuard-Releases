package main

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"math"
	"math/bits"
	"os/exec"
	"sort"
)

const signatureVersionV90 = 4

var detectorProcessesV90 = make(chan struct{}, 2)

func detectorProcessSlotV90(ctx context.Context) (func(), error) {
	select {
	case detectorProcessesV90 <- struct{}{}:
		return func() { <-detectorProcessesV90 }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Area sampling avoids the unstable single-pixel probes in the old dHash.
// A compact luminance grid also supplies structural evidence independent of
// the 64-bit hashes and permits small centered crops and brightness changes.
func signatureGridV90(img image.Image, crop float64) []byte {
	b := img.Bounds()
	grid := make([]byte, 32*32)
	if b.Empty() {
		return grid
	}
	w, h := float64(b.Dx()), float64(b.Dy())
	for gy := 0; gy < 32; gy++ {
		for gx := 0; gx < 32; gx++ {
			var sum float64
			for sy := 0; sy < 4; sy++ {
				for sx := 0; sx < 4; sx++ {
					x := b.Min.X + min(b.Dx()-1, int(w*(crop+(float64(gx)+(float64(sx)+.5)/4)/32*(1-2*crop))))
					y := b.Min.Y + min(b.Dy()-1, int(h*(crop+(float64(gy)+(float64(sy)+.5)/4)/32*(1-2*crop))))
					r, g, bl, _ := img.At(x, y).RGBA()
					sum += .2126*float64(r>>8) + .7152*float64(g>>8) + .0722*float64(bl>>8)
				}
			}
			grid[gy*32+gx] = byte(math.Round(sum / 16))
		}
	}
	return grid
}

var dctWeightsV90 = func() [8][32]float64 {
	var out [8][32]float64
	for k := range out {
		for x := range out[k] {
			out[k][x] = math.Cos(math.Pi * float64((2*x+1)*k) / 64)
		}
	}
	return out
}()

func perceptualHashV90(grid []byte) uint64 {
	if len(grid) != 1024 {
		return 0
	}
	var coeff [64]float64
	for v := 0; v < 8; v++ {
		for u := 0; u < 8; u++ {
			for y := 0; y < 32; y++ {
				for x := 0; x < 32; x++ {
					coeff[v*8+u] += float64(grid[y*32+x]) * dctWeightsV90[u][x] * dctWeightsV90[v][y]
				}
			}
		}
	}
	var peak float64
	for _, c := range coeff[1:] {
		peak = math.Max(peak, math.Abs(c))
	}
	for i := 1; i < 64; i++ {
		if math.Abs(coeff[i]) < peak*.01 {
			coeff[i] = 0
		}
	}
	values := append([]float64(nil), coeff[1:]...)
	sort.Float64s(values)
	median := values[len(values)/2]
	var h uint64
	for i := 1; i < 64; i++ {
		if coeff[i] > median {
			h |= uint64(1) << i
		}
	}
	return h
}

func gridCorrelationV90(a, b []byte) float64 {
	if len(a) != 1024 || len(b) != 1024 {
		return 0
	}
	var sa, sb, saa, sbb, sab float64
	for i := range a {
		x, y := float64(a[i]), float64(b[i])
		sa += x
		sb += y
		saa += x * x
		sbb += y * y
		sab += x * y
	}
	va, vb := saa-sa*sa/1024, sbb-sb*sb/1024
	if va < 1024*16 || vb < 1024*16 {
		return 0
	}
	return math.Max(0, math.Min(1, (sab-sa*sb/1024)/math.Sqrt(va*vb))) * 100
}

func richSignatureSimilarityV90(a, b imageSignatureV85) int {
	if a.Version != signatureVersionV90 || b.Version != signatureVersionV90 {
		return 0
	}
	p := float64(63-bits.OnesCount64(a.PHash^b.PHash)) * 100 / 63
	structure := gridCorrelationV90(a.Grid, b.Grid)
	// Compare a small crop on either side, never unrelated unordered patches.
	structure = math.Max(structure, math.Max(gridCorrelationV90(a.Center, b.Grid), gridCorrelationV90(a.Grid, b.Center)))
	if a.LumaStd < 6 || b.LumaStd < 6 {
		return min(59, int(structure))
	}
	d := float64(imageHashSimilarityV85(a.Hash, b.Hash))
	score := int(math.Round(.20*d + .30*p + .50*structure))
	if structure < 75 || p < 70 {
		score = min(score, 79)
	}
	if structure < 50 || p < 55 {
		score = min(score, 59)
	}
	return max(0, min(100, score))
}

func videoFrameSignatureV90(ctx context.Context, ff, target string, sec float64) (imageSignatureV85, bool, error) {
	release, err := detectorProcessSlotV90(ctx)
	if err != nil {
		return imageSignatureV85{}, false, err
	}
	defer release()
	args := []string{"-v", "error", "-threads", "1", "-ss", fmt.Sprintf("%.3f", sec), "-i", target, "-frames:v", "1", "-an", "-vf", "scale=64:64:flags=area", "-threads", "1", "-f", "rawvideo", "-pix_fmt", "rgb24", "pipe:1"}
	cmd := exec.CommandContext(ctx, ff, args...)
	hideChildWindow(cmd)
	b, err := cmd.Output()
	if err != nil {
		return imageSignatureV85{}, false, err
	}
	if len(b) != 64*64*3 {
		return imageSignatureV85{}, false, fmt.Errorf("incomplete frame: %d bytes", len(b))
	}
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			i := (y*64 + x) * 3
			img.SetRGBA(x, y, color.RGBA{b[i], b[i+1], b[i+2], 255})
		}
	}
	sig := makeImageSignatureV85(img)
	return sig, sig.LumaStd >= 6, nil
}

func scoreRichFrameSetV90(a, b videoFingerprintV85) (score, matched, high, veryHigh int) {
	for i := range a.Frames {
		if i >= len(b.Frames) || i >= len(a.Valid) || i >= len(b.Valid) || !a.Valid[i] || !b.Valid[i] {
			continue
		}
		value := richSignatureSimilarityV90(a.Frames[i], b.Frames[i])
		score += value
		matched++
		if value >= 90 {
			high++
		}
		if value >= 95 {
			veryHigh++
		}
	}
	if matched > 0 {
		score = int(math.Round(float64(score) / float64(matched)))
	}
	return
}
