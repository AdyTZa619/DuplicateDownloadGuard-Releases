package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type RichMediaStreamV860 struct {
	Type          string  `json:"type"`
	Codec         string  `json:"codec,omitempty"`
	Profile       string  `json:"profile,omitempty"`
	Width         int     `json:"width,omitempty"`
	Height        int     `json:"height,omitempty"`
	FPS           float64 `json:"fps,omitempty"`
	BitRate       int64   `json:"bitRate,omitempty"`
	PixelFormat   string  `json:"pixelFormat,omitempty"`
	BitDepth      int     `json:"bitDepth,omitempty"`
	ColorSpace    string  `json:"colorSpace,omitempty"`
	ColorTransfer string  `json:"colorTransfer,omitempty"`
	ColorPrimaries string `json:"colorPrimaries,omitempty"`
	HDR           bool    `json:"hdr,omitempty"`
	Channels      int     `json:"channels,omitempty"`
	ChannelLayout string  `json:"channelLayout,omitempty"`
	SampleRate    int     `json:"sampleRate,omitempty"`
	Language      string  `json:"language,omitempty"`
	Default       bool    `json:"default,omitempty"`
}

type RichMediaInfoV860 struct {
	OK             bool                 `json:"ok"`
	Source         string               `json:"source"`
	Format         string               `json:"format,omitempty"`
	Duration       float64              `json:"duration,omitempty"`
	BitRate        int64                `json:"bitRate,omitempty"`
	Size           int64                `json:"size,omitempty"`
	Video          *RichMediaStreamV860 `json:"video,omitempty"`
	Audio          []RichMediaStreamV860 `json:"audio,omitempty"`
	SubtitleTracks int                  `json:"subtitleTracks,omitempty"`
	VideoTracks    int                  `json:"videoTracks,omitempty"`
	AudioTracks    int                  `json:"audioTracks,omitempty"`
	Error          string               `json:"error,omitempty"`
}

type QualityFactorV860 struct {
	Factor string `json:"factor"`
	Remote string `json:"remote,omitempty"`
	Local  string `json:"local,omitempty"`
	Winner string `json:"winner,omitempty"` // remote|local|neutral|warning
	Weight int    `json:"weight,omitempty"`
	Reason string `json:"reason"`
}

type QualityDecisionV860 struct {
	Verdict     string              `json:"verdict"` // remote-better|local-better|comparable|incomplete
	UserVerdict string              `json:"userVerdict"`
	RemoteScore int                 `json:"remoteScore"`
	LocalScore  int                 `json:"localScore"`
	Confidence  string              `json:"confidence"`
	Factors     []QualityFactorV860 `json:"factors"`
	Caution     string              `json:"caution,omitempty"`
}

type ffprobeQualityRootV860 struct {
	Streams []struct {
		CodecType      string            `json:"codec_type"`
		CodecName      string            `json:"codec_name"`
		Profile        string            `json:"profile"`
		Width          int               `json:"width"`
		Height         int               `json:"height"`
		RFrameRate     string            `json:"r_frame_rate"`
		AvgFrameRate   string            `json:"avg_frame_rate"`
		BitRate        string            `json:"bit_rate"`
		PixFmt         string            `json:"pix_fmt"`
		BitsPerRawSample string          `json:"bits_per_raw_sample"`
		ColorSpace     string            `json:"color_space"`
		ColorTransfer  string            `json:"color_transfer"`
		ColorPrimaries string            `json:"color_primaries"`
		Channels       int               `json:"channels"`
		ChannelLayout  string            `json:"channel_layout"`
		SampleRate     string            `json:"sample_rate"`
		Tags           map[string]string `json:"tags"`
		Disposition    struct {
			Default int `json:"default"`
		} `json:"disposition"`
	} `json:"streams"`
	Format struct {
		FormatName string `json:"format_name"`
		Duration   string `json:"duration"`
		BitRate    string `json:"bit_rate"`
		Size       string `json:"size"`
	} `json:"format"`
}

func parseInt64V860(s string) int64 {
	n, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	return n
}
func parseIntV860(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}
func parseFloatV860(s string) float64 {
	f, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return f
}
func parseFPSV860(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" || s == "0/0" {
		return 0
	}
	if strings.Contains(s, "/") {
		parts := strings.SplitN(s, "/", 2)
		a := parseFloatV860(parts[0])
		b := parseFloatV860(parts[1])
		if b != 0 {
			return a / b
		}
		return 0
	}
	return parseFloatV860(s)
}

func inferBitDepthV860(pixFmt, raw string) int {
	if n := parseIntV860(raw); n > 0 {
		return n
	}
	p := strings.ToLower(pixFmt)
	for _, d := range []int{16, 14, 12, 10, 9} {
		if strings.Contains(p, strconv.Itoa(d)) {
			return d
		}
	}
	if p != "" {
		return 8
	}
	return 0
}

func isHDRV860(transfer, primaries string) bool {
	t := strings.ToLower(transfer)
	p := strings.ToLower(primaries)
	return strings.Contains(t, "smpte2084") || strings.Contains(t, "arib-std-b67") || strings.Contains(t, "hlg") || strings.Contains(p, "bt2020")
}

func parseRichMediaInfoV860(source string, b []byte) (RichMediaInfoV860, error) {
	var raw ffprobeQualityRootV860
	dec := json.NewDecoder(bytes.NewReader(b))
	if err := dec.Decode(&raw); err != nil {
		return RichMediaInfoV860{Source: source, Error: err.Error()}, err
	}
	info := RichMediaInfoV860{OK: true, Source: source, Format: raw.Format.FormatName, Duration: parseFloatV860(raw.Format.Duration), BitRate: parseInt64V860(raw.Format.BitRate), Size: parseInt64V860(raw.Format.Size)}
	for _, s := range raw.Streams {
		fps := parseFPSV860(s.AvgFrameRate)
		if fps <= 0 {
			fps = parseFPSV860(s.RFrameRate)
		}
		stream := RichMediaStreamV860{
			Type: s.CodecType, Codec: s.CodecName, Profile: s.Profile, Width: s.Width, Height: s.Height, FPS: fps,
			BitRate: parseInt64V860(s.BitRate), PixelFormat: s.PixFmt, BitDepth: inferBitDepthV860(s.PixFmt, s.BitsPerRawSample),
			ColorSpace: s.ColorSpace, ColorTransfer: s.ColorTransfer, ColorPrimaries: s.ColorPrimaries,
			HDR: isHDRV860(s.ColorTransfer, s.ColorPrimaries), Channels: s.Channels, ChannelLayout: s.ChannelLayout,
			SampleRate: parseIntV860(s.SampleRate), Language: strings.TrimSpace(s.Tags["language"]), Default: s.Disposition.Default != 0,
		}
		switch s.CodecType {
		case "video":
			info.VideoTracks++
			if info.Video == nil || (stream.Default && !info.Video.Default) {
				copy := stream
				info.Video = &copy
			}
		case "audio":
			info.AudioTracks++
			info.Audio = append(info.Audio, stream)
		case "subtitle":
			info.SubtitleTracks++
		}
	}
	return info, nil
}

func probeRichMediaInfoV860(ctx context.Context, ffprobe, target, referer, source string) RichMediaInfoV860 {
	if strings.TrimSpace(ffprobe) == "" {
		return RichMediaInfoV860{Source: source, Error: "ffprobe lipsește"}
	}
	if strings.TrimSpace(target) == "" {
		return RichMediaInfoV860{Source: source, Error: "sursă media lipsă"}
	}
	ctx, cancel := context.WithTimeout(ctx, 35*time.Second)
	defer cancel()
	args := []string{"-v", "error"}
	if strings.HasPrefix(strings.ToLower(target), "http") && strings.TrimSpace(referer) != "" {
		headers := "Referer: " + strings.TrimSpace(referer) + "\r\nUser-Agent: DuplicateDownloadGuard/8.6\r\n"
		args = append(args, "-headers", headers)
	}
	args = append(args, "-show_format", "-show_streams", "-of", "json", target)
	cmd := exec.CommandContext(ctx, ffprobe, args...)
	hideChildWindow(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		if len(msg) > 600 {
			msg = msg[len(msg)-600:]
		}
		return RichMediaInfoV860{Source: source, Error: msg}
	}
	info, err := parseRichMediaInfoV860(source, out)
	if err != nil {
		return RichMediaInfoV860{Source: source, Error: err.Error()}
	}
	return info
}

func qualityTextBitrateV860(v int64) string {
	if v <= 0 {
		return "?"
	}
	return fmt.Sprintf("%.2f Mb/s", float64(v)/1_000_000)
}

func qualityDecisionV860(remote, local RichMediaInfoV860) QualityDecisionV860 {
	d := QualityDecisionV860{Verdict: "comparable", UserVerdict: "CALITATE APROPIATĂ", Confidence: "medie"}
	if !remote.OK || !local.OK {
		d.Verdict = "incomplete"
		d.UserVerdict = "NU S-A PUTUT COMPARA COMPLET"
		d.Confidence = "scăzută"
		d.Caution = "Lipsesc metadate pentru una dintre versiuni."
		return d
	}
	add := func(f QualityFactorV860) {
		d.Factors = append(d.Factors, f)
		if f.Winner == "remote" {
			d.RemoteScore += f.Weight
		} else if f.Winner == "local" {
			d.LocalScore += f.Weight
		}
	}

	if remote.Duration > 0 && local.Duration > 0 {
		diff := math.Abs(remote.Duration - local.Duration)
		limit := math.Max(2.0, math.Min(remote.Duration, local.Duration)*0.01)
		if diff > limit {
			winner := "warning"
			reason := fmt.Sprintf("Duratele diferă cu %.1f sec; poate fi alt cut, intro/outro sau fișier incomplet.", diff)
			add(QualityFactorV860{Factor: "Durată / completitudine", Remote: fmt.Sprintf("%.1fs", remote.Duration), Local: fmt.Sprintf("%.1fs", local.Duration), Winner: winner, Reason: reason})
			d.Caution = reason
		} else {
			add(QualityFactorV860{Factor: "Durată", Remote: fmt.Sprintf("%.1fs", remote.Duration), Local: fmt.Sprintf("%.1fs", local.Duration), Winner: "neutral", Reason: "Duratele sunt suficient de apropiate."})
		}
	}

	if remote.Video != nil && local.Video != nil {
		rp := remote.Video.Width * remote.Video.Height
		lp := local.Video.Width * local.Video.Height
		if rp > 0 && lp > 0 {
			winner, weight := "neutral", 0
			if float64(rp) >= float64(lp)*1.30 {
				winner, weight = "remote", 4
			} else if float64(lp) >= float64(rp)*1.30 {
				winner, weight = "local", 4
			}
			add(QualityFactorV860{Factor: "Rezoluție", Remote: fmt.Sprintf("%d×%d", remote.Video.Width, remote.Video.Height), Local: fmt.Sprintf("%d×%d", local.Video.Width, local.Video.Height), Winner: winner, Weight: weight, Reason: "Rezoluția contează ca nivel de detaliu, dar singură nu dovedește calitatea perceptuală."})
		}
		if remote.Video.BitDepth > 0 && local.Video.BitDepth > 0 && remote.Video.BitDepth != local.Video.BitDepth {
			winner := "remote"
			if local.Video.BitDepth > remote.Video.BitDepth {
				winner = "local"
			}
			add(QualityFactorV860{Factor: "Bit depth", Remote: fmt.Sprintf("%d-bit", remote.Video.BitDepth), Local: fmt.Sprintf("%d-bit", local.Video.BitDepth), Winner: winner, Weight: 2, Reason: "Bit depth mai mare poate reduce banding-ul și păstra gradații mai fine."})
		}
		if remote.Video.HDR != local.Video.HDR {
			winner := "remote"
			if local.Video.HDR {
				winner = "local"
			}
			add(QualityFactorV860{Factor: "HDR", Remote: fmt.Sprint(remote.Video.HDR), Local: fmt.Sprint(local.Video.HDR), Winner: winner, Weight: 2, Reason: "HDR este tratat ca o caracteristică de versiune/masters, nu ca dovadă absolută de calitate."})
		}
		if remote.Video.FPS > 0 && local.Video.FPS > 0 && math.Abs(remote.Video.FPS-local.Video.FPS) >= 5 {
			winner, weight := "remote", 1
			if local.Video.FPS > remote.Video.FPS {
				winner = "local"
			}
			add(QualityFactorV860{Factor: "FPS", Remote: fmt.Sprintf("%.2f", remote.Video.FPS), Local: fmt.Sprintf("%.2f", local.Video.FPS), Winner: winner, Weight: weight, Reason: "FPS mai mare poate păstra mai multă mișcare, dar poate reprezenta și o conversie; influență redusă."})
		}
		if strings.EqualFold(remote.Video.Codec, local.Video.Codec) && remote.Video.BitRate > 0 && local.Video.BitRate > 0 {
			winner, weight := "neutral", 0
			if float64(remote.Video.BitRate) > float64(local.Video.BitRate)*1.25 {
				winner, weight = "remote", 1
			} else if float64(local.Video.BitRate) > float64(remote.Video.BitRate)*1.25 {
				winner, weight = "local", 1
			}
			add(QualityFactorV860{Factor: "Bitrate video (același codec)", Remote: qualityTextBitrateV860(remote.Video.BitRate), Local: qualityTextBitrateV860(local.Video.BitRate), Winner: winner, Weight: weight, Reason: "Bitrate-ul este folosit doar când codec-ul este același și are pondere mică."})
		} else if remote.Video.Codec != "" || local.Video.Codec != "" {
			add(QualityFactorV860{Factor: "Codec video", Remote: remote.Video.Codec, Local: local.Video.Codec, Winner: "neutral", Reason: "Codecurile diferite au eficiențe diferite; nu declarăm un câștigător numai din numele codec-ului."})
		}
	}

	if remote.AudioTracks != local.AudioTracks {
		winner, weight := "remote", 1
		if local.AudioTracks > remote.AudioTracks {
			winner = "local"
		}
		add(QualityFactorV860{Factor: "Piste audio", Remote: strconv.Itoa(remote.AudioTracks), Local: strconv.Itoa(local.AudioTracks), Winner: winner, Weight: weight, Reason: "Mai multe piste pot însemna limbi/comentarii/opțiuni suplimentare."})
	}
	maxChannels := func(x RichMediaInfoV860) int {
		m := 0
		for _, a := range x.Audio {
			if a.Channels > m {
				m = a.Channels
			}
		}
		return m
	}
	rc, lc := maxChannels(remote), maxChannels(local)
	if rc > 0 && lc > 0 && rc != lc {
		winner, weight := "remote", 2
		if lc > rc {
			winner = "local"
		}
		add(QualityFactorV860{Factor: "Canale audio", Remote: strconv.Itoa(rc), Local: strconv.Itoa(lc), Winner: winner, Weight: weight, Reason: "O pistă multicanal oferă mai multe informații audio decât stereo, dacă este sursă reală."})
	}
	if remote.SubtitleTracks != local.SubtitleTracks {
		winner := "remote"
		if local.SubtitleTracks > remote.SubtitleTracks {
			winner = "local"
		}
		add(QualityFactorV860{Factor: "Subtitrări incluse", Remote: strconv.Itoa(remote.SubtitleTracks), Local: strconv.Itoa(local.SubtitleTracks), Winner: winner, Weight: 1, Reason: "Pistele de subtitrare sunt un avantaj de completitudine, nu de imagine."})
	}

	delta := d.RemoteScore - d.LocalScore
	if delta >= 3 {
		d.Verdict = "remote-better"
		d.UserVerdict = "REMOTE PARE MAI BUN"
	} else if delta <= -3 {
		d.Verdict = "local-better"
		d.UserVerdict = "VERSIUNEA LOCALĂ PARE MAI BUNĂ"
	} else {
		d.Verdict = "comparable"
		d.UserVerdict = "CALITATE APROPIATĂ / ALTĂ VERSIUNE"
	}
	if len(d.Factors) >= 4 && d.Caution == "" {
		d.Confidence = "ridicată"
	}
	return d
}

func (a *App) qualityForResultV860(ctx context.Context, res Result) (RichMediaInfoV860, RichMediaInfoV860, QualityDecisionV860) {
	ff := a.detectFFprobe()
	local := RichMediaInfoV860{Source: "local", Error: "copie locală lipsă"}
	if strings.TrimSpace(res.LocalPath) != "" {
		local = probeRichMediaInfoV860(ctx, ff, res.LocalPath, "", "local")
	}
	remoteTarget := strings.TrimSpace(res.Remote.DirectURL)
	if remoteTarget == "" {
		remoteTarget = strings.TrimSpace(res.Remote.URL)
	}
	if strings.EqualFold(res.Remote.Source, "MEGA") {
		if stream, _, _, err := a.startMegaPreviewForUIV854(res.Remote, false); err == nil {
			remoteTarget = stream
		}
	}
	remote := probeRichMediaInfoV860(ctx, ff, remoteTarget, downloadRefererV855(res), "remote")
	return remote, local, qualityDecisionV860(remote, local)
}

func (a *App) handleMediaQualityV860(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(r.URL.Query().Get("id"))
	if id <= 0 {
		http.Error(w, "id invalid", 400)
		return
	}
	res, ok := a.resultByID(id)
	if !ok {
		http.Error(w, "rezultat inexistent", 404)
		return
	}
	remote, local, decision := a.qualityForResultV860(r.Context(), res)
	jsonOut(w, map[string]any{"remote": remote, "local": local, "decision": decision})
}
