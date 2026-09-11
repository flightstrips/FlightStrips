package aman

import (
	"math"
	"time"
)

// RunwayGapIntervalInput is the operator-supplied shape of a runway GAP.
// Exactly one end mode must be present: an absolute End or a SlotCount whose
// duration is derived from the arrival rate at acceptance time.
type RunwayGapIntervalInput struct {
	Start     time.Time
	End       *time.Time
	SlotCount *uint32
}

// RunwayGapInterval is the immutable normalized [Start, End) value retained
// after input acceptance. It deliberately carries neither slot count nor rate,
// so subsequent rate changes cannot resize an accepted interval.
type RunwayGapInterval struct {
	start time.Time
	end   time.Time
}

// Start returns the inclusive UTC boundary.
func (i RunwayGapInterval) Start() time.Time { return i.start }

// End returns the exclusive UTC boundary.
func (i RunwayGapInterval) End() time.Time { return i.end }

// NormalizeRunwayGapInterval validates one input form and returns only its
// absolute UTC interval. arrivalsPerHour is the rate captured at acceptance;
// it must be positive for either form so callers cannot accidentally omit the
// rate required by the normalization contract.
func NormalizeRunwayGapInterval(input RunwayGapIntervalInput, arrivalsPerHour uint32) (RunwayGapInterval, error) {
	if err := requireUTCTime("runway gap start", input.Start); err != nil {
		return RunwayGapInterval{}, err
	}
	if arrivalsPerHour == 0 {
		return RunwayGapInterval{}, invalid("runway gap arrival rate must be positive")
	}
	if (input.End == nil) == (input.SlotCount == nil) {
		return RunwayGapInterval{}, invalid("runway gap requires exactly one end or slot count")
	}

	end := time.Time{}
	if input.End != nil {
		if err := requireUTCTime("runway gap end", *input.End); err != nil {
			return RunwayGapInterval{}, err
		}
		end = *input.End
	} else {
		if *input.SlotCount == 0 {
			return RunwayGapInterval{}, invalid("runway gap slot count must be positive")
		}
		duration, ok := runwayGapSlotDuration(*input.SlotCount, arrivalsPerHour)
		if !ok {
			return RunwayGapInterval{}, invalid("runway gap slot duration exceeds the supported range")
		}
		end = input.Start.Add(duration)
	}

	if !input.Start.Before(end) {
		return RunwayGapInterval{}, invalid("runway gap start must be before end")
	}
	return RunwayGapInterval{start: input.Start, end: end}, nil
}

// runwayGapSlotDuration calculates time.Hour * slotCount / arrivalsPerHour
// without overflowing the intermediate multiplication. Division intentionally
// uses time.Duration's nanosecond precision and therefore preserves fractional
// minute and second results deterministically.
func runwayGapSlotDuration(slotCount, arrivalsPerHour uint32) (time.Duration, bool) {
	if uint64(slotCount) > uint64(math.MaxInt64/time.Hour) {
		return 0, false
	}
	return time.Hour * time.Duration(slotCount) / time.Duration(arrivalsPerHour), true
}
