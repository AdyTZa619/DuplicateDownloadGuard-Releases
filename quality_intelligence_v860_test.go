package main

import (
	"testing"
)

func TestParseRichMediaInfoV860(t *testing.T) {
	raw := []byte(`{
  "streams": [
    {"codec_type":"video","codec_name":"hevc","profile":"Main 10","width":1920,"height":1080,"avg_frame_rate":"30000/1001","bit_rate":"5000000","pix_fmt":"yuv420p10le","color_space":"bt2020nc","color_transfer":"smpte2084","color_primaries":"bt2020","disposition":{"default":1}},
    {"codec_type":"audio","codec_name":"aac","bit_rate":"384000","channels":6,"channel_layout":"5.1","sample_rate":"48000","tags":{"language":"eng"},"disposition":{"default":1}},
    {"codec_type":"audio","codec_name":"aac","bit_rate":"192000","channels":2,"channel_layout":"stereo","sample_rate":"48000","tags":{"language":"ron"},"disposition":{"default":0}},
    {"codec_type":"subtitle","codec_name":"subrip","tags":{"language":"ron"},"disposition":{"default":1}}
  ],
  "format":{"format_name":"matroska,webm","duration":"120.25","bit_rate":"5600000","size":"84210000"}
}`)
	info, err := parseRichMediaInfoV860("test", raw)
	if err != nil {
		t.Fatal(err)
	}
	if !info.OK || info.Video == nil {
		t.Fatalf("info=%#v", info)
	}
	if info.Video.BitDepth != 10 || !info.Video.HDR || info.Video.Width != 1920 || info.Video.Height != 1080 {
		t.Fatalf("video=%#v", info.Video)
	}
	if info.AudioTracks != 2 || info.SubtitleTracks != 1 || len(info.Audio) != 2 {
		t.Fatalf("tracks=%#v", info)
	}
	if info.Audio[0].Channels != 6 || info.Audio[0].Language != "eng" {
		t.Fatalf("audio=%#v", info.Audio[0])
	}
}

func TestQualityDecisionRemoteClearlyBetterV860(t *testing.T) {
	remote := RichMediaInfoV860{OK: true, Duration: 100, Video: &RichMediaStreamV860{Codec: "hevc", Width: 3840, Height: 2160, BitDepth: 10, HDR: true, FPS: 60}, AudioTracks: 2, Audio: []RichMediaStreamV860{{Channels: 6}}, SubtitleTracks: 2}
	local := RichMediaInfoV860{OK: true, Duration: 100, Video: &RichMediaStreamV860{Codec: "h264", Width: 1920, Height: 1080, BitDepth: 8, HDR: false, FPS: 30}, AudioTracks: 1, Audio: []RichMediaStreamV860{{Channels: 2}}, SubtitleTracks: 0}
	d := qualityDecisionV860(remote, local)
	if d.Verdict != "remote-better" || d.RemoteScore <= d.LocalScore {
		t.Fatalf("decision=%#v", d)
	}
	if len(d.Factors) < 5 {
		t.Fatalf("expected explainable factors: %#v", d.Factors)
	}
}

func TestQualityDecisionDoesNotUseCrossCodecBitrateAsProofV860(t *testing.T) {
	remote := RichMediaInfoV860{OK: true, Video: &RichMediaStreamV860{Codec: "hevc", Width: 1920, Height: 1080, BitRate: 2_000_000, BitDepth: 8}, AudioTracks: 1, Audio: []RichMediaStreamV860{{Channels: 2}}}
	local := RichMediaInfoV860{OK: true, Video: &RichMediaStreamV860{Codec: "h264", Width: 1920, Height: 1080, BitRate: 8_000_000, BitDepth: 8}, AudioTracks: 1, Audio: []RichMediaStreamV860{{Channels: 2}}}
	d := qualityDecisionV860(remote, local)
	if d.Verdict != "comparable" {
		t.Fatalf("cross-codec bitrate must not choose a winner: %#v", d)
	}
	for _, f := range d.Factors {
		if f.Factor == "Bitrate video (același codec)" {
			t.Fatalf("cross-codec bitrate factor should not be used: %#v", f)
		}
	}
}

func TestQualityDecisionDurationMismatchWarnsV860(t *testing.T) {
	remote := RichMediaInfoV860{OK: true, Duration: 105, Video: &RichMediaStreamV860{Codec: "h264", Width: 1920, Height: 1080}}
	local := RichMediaInfoV860{OK: true, Duration: 90, Video: &RichMediaStreamV860{Codec: "h264", Width: 1920, Height: 1080}}
	d := qualityDecisionV860(remote, local)
	if d.Caution == "" {
		t.Fatalf("expected duration caution: %#v", d)
	}
}

func TestParseFPSV860(t *testing.T) {
	if got := parseFPSV860("30000/1001"); got < 29.96 || got > 29.98 {
		t.Fatalf("fps=%f", got)
	}
}
