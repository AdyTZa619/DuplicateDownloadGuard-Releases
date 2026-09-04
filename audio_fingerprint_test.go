package main

import "testing"

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
