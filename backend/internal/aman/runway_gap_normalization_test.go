package aman

import (
	"errors"
	"math"
	"testing"
	"time"
)

func TestNormalizeRunwayGapIntervalAbsoluteBounds(t *testing.T) {
	start := gapNormalizationTime()
	end := start.Add(6 * time.Minute)

	interval, err := NormalizeRunwayGapInterval(RunwayGapIntervalInput{Start: start, End: &end}, 20)
	if err != nil {
		t.Fatalf("normalize absolute interval: %v", err)
	}
	if interval.Start() != start || interval.End() != end {
		t.Fatalf("interval = [%s, %s), want [%s, %s)", interval.Start(), interval.End(), start, end)
	}
	if interval.End().Sub(interval.Start()) != 6*time.Minute {
		t.Fatalf("duration = %s, want 6m", interval.End().Sub(interval.Start()))
	}
}

func TestNormalizeRunwayGapIntervalSlotCountUsesAcceptanceRate(t *testing.T) {
	start := gapNormalizationTime()
	slots := uint32(2)

	accepted, err := NormalizeRunwayGapInterval(RunwayGapIntervalInput{Start: start, SlotCount: &slots}, 20)
	if err != nil {
		t.Fatalf("normalize slot interval: %v", err)
	}
	wantEnd := time.Date(2026, time.September, 11, 12, 6, 0, 0, time.UTC)
	if accepted.Start() != start || accepted.End() != wantEnd {
		t.Fatalf("interval = [%s, %s), want [%s, %s)", accepted.Start(), accepted.End(), start, wantEnd)
	}

	changedRate, err := NormalizeRunwayGapInterval(RunwayGapIntervalInput{Start: start, SlotCount: &slots}, 40)
	if err != nil {
		t.Fatalf("normalize comparison interval: %v", err)
	}
	if changedRate.End() == accepted.End() {
		t.Fatal("comparison rate did not demonstrate a different derived end")
	}
	if accepted.End() != wantEnd {
		t.Fatalf("accepted interval changed after later normalization: %s", accepted.End())
	}
}

func TestNormalizeRunwayGapIntervalPreservesFractionalDuration(t *testing.T) {
	start := gapNormalizationTime()
	slots := uint32(2)

	first, err := NormalizeRunwayGapInterval(RunwayGapIntervalInput{Start: start, SlotCount: &slots}, 7)
	if err != nil {
		t.Fatalf("normalize fractional interval: %v", err)
	}
	second, err := NormalizeRunwayGapInterval(RunwayGapIntervalInput{Start: start, SlotCount: &slots}, 7)
	if err != nil {
		t.Fatalf("repeat fractional normalization: %v", err)
	}
	wantDuration := time.Hour * time.Duration(slots) / 7
	if got := first.End().Sub(first.Start()); got != wantDuration {
		t.Fatalf("duration = %s, want exact %s", got, wantDuration)
	}
	if first != second {
		t.Fatalf("normalization is not deterministic: first=%#v second=%#v", first, second)
	}
}

func TestNormalizeRunwayGapIntervalRejectsInvalidInput(t *testing.T) {
	start := gapNormalizationTime()
	end := start.Add(time.Minute)
	zeroSlots := uint32(0)
	oneSlot := uint32(1)
	nonUTCStart := start.In(time.FixedZone("CEST", 2*60*60))
	nonUTCEnd := end.In(time.FixedZone("CEST", 2*60*60))

	tests := map[string]struct {
		input RunwayGapIntervalInput
		rate  uint32
	}{
		"missing mode":       {input: RunwayGapIntervalInput{Start: start}, rate: 20},
		"both modes":         {input: RunwayGapIntervalInput{Start: start, End: &end, SlotCount: &oneSlot}, rate: 20},
		"zero slots":         {input: RunwayGapIntervalInput{Start: start, SlotCount: &zeroSlots}, rate: 20},
		"zero rate absolute": {input: RunwayGapIntervalInput{Start: start, End: &end}, rate: 0},
		"zero rate slots":    {input: RunwayGapIntervalInput{Start: start, SlotCount: &oneSlot}, rate: 0},
		"zero start":         {input: RunwayGapIntervalInput{Start: time.Time{}, End: &end}, rate: 20},
		"non-UTC start":      {input: RunwayGapIntervalInput{Start: nonUTCStart, End: &end}, rate: 20},
		"non-UTC end":        {input: RunwayGapIntervalInput{Start: start, End: &nonUTCEnd}, rate: 20},
		"equal bounds":       {input: RunwayGapIntervalInput{Start: start, End: &start}, rate: 20},
		"reversed bounds":    {input: RunwayGapIntervalInput{Start: start, End: timePointer(start.Add(-time.Second))}, rate: 20},
		"duration overflow":  {input: RunwayGapIntervalInput{Start: start, SlotCount: uint32Pointer(math.MaxUint32)}, rate: 1},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := NormalizeRunwayGapInterval(test.input, test.rate)
			var domain *DomainError
			if !errors.As(err, &domain) || domain.Class != ErrorInvalidArgument {
				t.Fatalf("error = %v, want invalid_argument domain error", err)
			}
		})
	}
}

func gapNormalizationTime() time.Time {
	return time.Date(2026, time.September, 11, 12, 0, 0, 0, time.UTC)
}

func timePointer(value time.Time) *time.Time { return &value }

func uint32Pointer(value uint32) *uint32 { return &value }
