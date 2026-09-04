package main

import (
	"image"
	"image/color"
	"testing"
)

func solidImageV85(c color.RGBA) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

func gradientImageV85(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, color.RGBA{R: uint8((x * 255) / max(1, w-1)), G: uint8((y * 255) / max(1, h-1)), B: uint8(((x + y) * 255) / max(1, w+h-2)), A: 255})
		}
	}
	return img
}

func TestImageSignatureRejectsDifferentFlatImages(t *testing.T) {
	black := makeImageSignatureV85(solidImageV85(color.RGBA{A: 255}))
	white := makeImageSignatureV85(solidImageV85(color.RGBA{R: 255, G: 255, B: 255, A: 255}))
	if black.Hash != white.Hash {
		t.Log("dHash happened to differ; combined signature is still expected to reject the pair")
	}
	if score := imageSignatureSimilarityV85(black, white); score >= 70 {
		t.Fatalf("flat black/white images scored too high: %d", score)
	}
}

func TestImageSignatureRecognizesSameGradientAtDifferentResolution(t *testing.T) {
	a := makeImageSignatureV85(gradientImageV85(96, 64))
	b := makeImageSignatureV85(gradientImageV85(192, 128))
	if score := imageSignatureSimilarityV85(a, b); score < 94 {
		t.Fatalf("same visual gradient after resize scored too low: %d", score)
	}
}
