package opskip

import (
	"testing"
)

func TestDetectOpeningsContinuesAfterEarlyEpisodesWithoutOP(t *testing.T) {
	common := commonFingerprint(220)
	episodes := []mediaEpisode{
		{MediaJobID: 1, Fingerprint: baseFingerprint(900, 11)},
		{MediaJobID: 2, Fingerprint: baseFingerprint(900, 22)},
		{MediaJobID: 3, Fingerprint: withCommonFingerprint(900, 33, 120, common)},
		{MediaJobID: 4, Fingerprint: withCommonFingerprint(900, 44, 180, common)},
	}

	detections := detectOpenings(episodes)
	if _, ok := detections[1]; ok {
		t.Fatal("episode 1 unexpectedly detected an opening")
	}
	if _, ok := detections[2]; ok {
		t.Fatal("episode 2 unexpectedly detected an opening")
	}
	for _, id := range []int64{3, 4} {
		detection, ok := detections[id]
		if !ok {
			t.Fatalf("episode %d should have detected an opening", id)
		}
		if !validOpeningRange(detection.Start, detection.End) {
			t.Fatalf("episode %d detected invalid range: %+v", id, detection)
		}
	}
}

func TestCompareFingerprintsFindsShiftedOpening(t *testing.T) {
	common := commonFingerprint(180)
	left := withCommonFingerprint(800, 101, 80, common)
	right := withCommonFingerprint(800, 202, 240, common)

	leftRange, rightRange, ok := compareFingerprints(left, right)
	if !ok {
		t.Fatal("expected shifted common fingerprint to be detected")
	}
	if leftRange.Start < 9 || leftRange.Start > 12 {
		t.Fatalf("unexpected left range: %+v", leftRange)
	}
	if rightRange.Start < 29 || rightRange.Start > 32 {
		t.Fatalf("unexpected right range: %+v", rightRange)
	}
	if !validOpeningRange(leftRange.Start, leftRange.End) || !validOpeningRange(rightRange.Start, rightRange.End) {
		t.Fatalf("expected valid ranges, got left=%+v right=%+v", leftRange, rightRange)
	}
}

func TestParseSilenceRanges(t *testing.T) {
	ranges := parseSilenceRanges(`[silencedetect @ 000] silence_start: 12.34
[silencedetect @ 000] silence_end: 12.91 | silence_duration: 0.57
[silencedetect @ 000] silence_start: 33
[silencedetect @ 000] silence_end: 34.25 | silence_duration: 1.25`)
	if len(ranges) != 2 {
		t.Fatalf("expected two silence ranges, got %+v", ranges)
	}
	if ranges[0].Start != 12.34 || ranges[0].End != 12.91 {
		t.Fatalf("unexpected first range: %+v", ranges[0])
	}
	if ranges[1].Start != 33 || ranges[1].End != 34.25 {
		t.Fatalf("unexpected second range: %+v", ranges[1])
	}
}

func TestFingerprintEncodingRoundTrip(t *testing.T) {
	points := []uint32{1, 2, 3, 0xfeedbeef}
	decoded, err := decodeFingerprintPoints(encodeFingerprintPoints(points))
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded) != len(points) {
		t.Fatalf("decoded length = %d, want %d", len(decoded), len(points))
	}
	for index := range points {
		if decoded[index] != points[index] {
			t.Fatalf("decoded[%d] = %d, want %d", index, decoded[index], points[index])
		}
	}
}

func baseFingerprint(length int, seed uint32) []uint32 {
	points := make([]uint32, length)
	value := seed
	for index := range points {
		value = value*1664525 + 1013904223
		points[index] = value
	}
	return points
}

func commonFingerprint(length int) []uint32 {
	points := make([]uint32, length)
	value := uint32(0x12345678)
	for index := range points {
		value = value*1103515245 + 12345
		points[index] = value
	}
	return points
}

func withCommonFingerprint(length int, seed uint32, offset int, common []uint32) []uint32 {
	points := baseFingerprint(length, seed)
	copy(points[offset:], common)
	return points
}
