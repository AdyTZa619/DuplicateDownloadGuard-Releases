package main

import (
	"encoding/binary"
	"math"
	"testing"
)

func TestChromaprintSimilarityV85HandlesSmallSequenceShift(t *testing.T) {
	a := []uint32{0x01020304, 0x11223344, 0x55667788, 0x99aabbcc, 0xdeadbeef, 0x13572468, 0x24681357, 0xa5a5a5a5, 0x5a5a5a5a, 0x0f0f0f0f, 0xf0f0f0f0, 0x12345678}
	b := append([]uint32{0xffffffff, 0x00000000}, a...)
	if score := chromaprintSimilarityV85(a, b); score < 99 {
		t.Fatalf("shifted identical audio fingerprint scored too low: %d", score)
	}
}

func TestChromaprintSimilarityV85RejectsDifferentSequences(t *testing.T) {
	a := []uint32{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	b := []uint32{0xffffffff, 0xffffffff, 0xffffffff, 0xffffffff, 0xffffffff, 0xffffffff, 0xffffffff, 0xffffffff, 0xffffffff, 0xffffffff, 0xffffffff, 0xffffffff}
	if score := chromaprintSimilarityV85(a, b); score > 10 {
		t.Fatalf("opposite audio fingerprints scored too high: %d", score)
	}
}

func TestChromaprintSimilarityV85NeedsEnoughData(t *testing.T) {
	if score := chromaprintSimilarityV85([]uint32{1, 2, 3}, []uint32{1, 2, 3}); score != -1 {
		t.Fatalf("short audio fingerprints must be unavailable, got %d", score)
	}
}

func testPCM16V901(seconds float64, frequencies ...float64) []byte {
	samples := int(seconds * audioPCMSampleRateV901)
	out := make([]byte, samples*2)
	for i := 0; i < samples; i++ {
		var value float64
		for _, frequency := range frequencies {
			value += math.Sin(2 * math.Pi * frequency * float64(i) / audioPCMSampleRateV901)
		}
		value /= math.Max(1, float64(len(frequencies)))
		binary.LittleEndian.PutUint16(out[i*2:i*2+2], uint16(int16(value*12000)))
	}
	return out
}

func TestPCMAudioFingerprintV901ToleratesGainAndTemporalShift(t *testing.T) {
	base := testPCM16V901(12, 220, 523, 1170)
	shifted := append(make([]byte, audioPCMSampleRateV901/2*2), base...)
	a, err := pcmAudioFingerprintV901(base, audioPCMSampleRateV901)
	if err != nil {
		t.Fatal(err)
	}
	b, err := pcmAudioFingerprintV901(shifted, audioPCMSampleRateV901)
	if err != nil {
		t.Fatal(err)
	}
	if score := pcmAudioSimilarityV901(a, b); score < 95 {
		t.Fatalf("shifted PCM fingerprint scored too low: %d", score)
	}
}

func TestPCMAudioFingerprintV901RejectsDifferentSpectrum(t *testing.T) {
	a, err := pcmAudioFingerprintV901(testPCM16V901(12, 180, 410), audioPCMSampleRateV901)
	if err != nil {
		t.Fatal(err)
	}
	b, err := pcmAudioFingerprintV901(testPCM16V901(12, 980, 2100), audioPCMSampleRateV901)
	if err != nil {
		t.Fatal(err)
	}
	if score := pcmAudioSimilarityV901(a, b); score >= 68 {
		t.Fatalf("different PCM spectra scored too high: %d", score)
	}
}

func TestPCMAudioFingerprintV901RejectsShortInput(t *testing.T) {
	if _, err := pcmAudioFingerprintV901(make([]byte, 100), audioPCMSampleRateV901); err == nil {
		t.Fatal("short PCM input must be rejected")
	}
}
