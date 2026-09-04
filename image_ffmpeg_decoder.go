package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const maxDirectSignaturePixelsV85 int64 = 80_000_000

// Go's standard image decoders cover JPEG/PNG/GIF, while Smart Media Guard
// also accepts WEBP/BMP/AVIF. Register small FFmpeg-backed decoders for those
// formats so unsupported images do not remain permanently unverifiable.
func init() {
	image.RegisterFormat("ddg-webp", "RIFF????WEBP", decodeImageViaFFmpegV85, decodeImageConfigViaFFmpegV85)
	image.RegisterFormat("ddg-bmp", "BM", decodeImageViaFFmpegV85, decodeImageConfigViaFFmpegV85)
	image.RegisterFormat("ddg-avif", "????ftypavif", decodeImageViaFFmpegV85, decodeImageConfigViaFFmpegV85)
	image.RegisterFormat("ddg-avis", "????ftypavis", decodeImageViaFFmpegV85, decodeImageConfigViaFFmpegV85)
}

func fallbackFFmpegV85() string {
	// Respect the portable/custom configuration first without constructing App.
	var cfg struct {
		FFmpegPath string `json:"ffmpegPath"`
	}
	if b, err := os.ReadFile(filepath.Join(executableDir(), "data", "config.json")); err == nil {
		if json.Unmarshal(b, &cfg) == nil {
			if p := strings.TrimSpace(cfg.FFmpegPath); p != "" {
				if st, err := os.Stat(p); err == nil && !st.IsDir() {
					return p
				}
			}
		}
	}

	candidates := []string{
		filepath.Join(portableToolsDir(), "ffmpeg", "ffmpeg.exe"),
		filepath.Join(portableToolsDir(), "ffmpeg", "bin", "ffmpeg.exe"),
		filepath.Join(portableToolsDir(), "ffmpeg.exe"),
	}
	if runtime.GOOS == "windows" {
		candidates = append(candidates,
			filepath.Join(os.Getenv("ProgramFiles"), "FFmpeg", "bin", "ffmpeg.exe"),
			`C:\ffmpeg\bin\ffmpeg.exe`,
		)
	}
	for _, p := range candidates {
		if p == "" {
			continue
		}
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	for _, name := range []string{"ffmpeg.exe", "ffmpeg"} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	return ""
}

func decodeImageViaFFmpegV85(r io.Reader) (image.Image, error) {
	ff := fallbackFFmpegV85()
	if ff == "" {
		return nil, fmt.Errorf("format imagine necesită FFmpeg")
	}
	// Decode only one frame and bound it to 512x512. Smart Media Guard needs a
	// stable perceptual signature, not the original full-resolution pixels.
	args := []string{
		"-v", "error",
		"-i", "pipe:0",
		"-frames:v", "1",
		"-vf", "scale=512:512:force_original_aspect_ratio=decrease",
		"-f", "image2pipe",
		"-vcodec", "png",
		"pipe:1",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, ff, args...)
	hideChildWindow(cmd)
	cmd.Stdin = io.LimitReader(r, 256<<20)
	out, err := cmd.Output()
	if ctx.Err() != nil {
		return nil, fmt.Errorf("FFmpeg imagine a depășit limita de timp: %w", ctx.Err())
	}
	if err != nil {
		return nil, fmt.Errorf("FFmpeg imagine: %w", err)
	}
	img, _, err := image.Decode(bytes.NewReader(out))
	if err != nil {
		return nil, fmt.Errorf("FFmpeg a produs o imagine invalidă: %w", err)
	}
	return img, nil
}

func decodeImageConfigViaFFmpegV85(r io.Reader) (image.Config, error) {
	img, err := decodeImageViaFFmpegV85(r)
	if err != nil {
		return image.Config{}, err
	}
	b := img.Bounds()
	return image.Config{ColorModel: img.ColorModel(), Width: b.Dx(), Height: b.Dy()}, nil
}

// decodeImageForSignatureV85 checks dimensions before a full Go decode. A very
// compressed JPEG/PNG can expand to hundreds of megabytes even when the remote
// file itself is small. Oversized images are downscaled through FFmpeg first.
func decodeImageForSignatureV85(r io.ReadSeeker) (image.Image, error) {
	cfg, _, cfgErr := image.DecodeConfig(r)
	if _, err := r.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	if cfgErr == nil && cfg.Width > 0 && cfg.Height > 0 {
		pixels := int64(cfg.Width) * int64(cfg.Height)
		if pixels > maxDirectSignaturePixelsV85 {
			return decodeImageViaFFmpegV85(r)
		}
	}
	img, _, err := image.Decode(r)
	return img, err
}
