package holdingclearance

import (
	"errors"
	"fmt"
	"time"
)

const eatOccurrenceWindow = 12 * time.Hour

// ErrInvalidEATHHMM identifies controller EAT values that are not exactly four
// decimal digits representing a valid UTC hour and minute.
var ErrInvalidEATHHMM = errors.New("invalid EAT HHMM")

// ResolveEATUTC resolves a time-only controller EAT against the server's
// reference clock. The returned instant is the nearest occurrence no more than
// 12 hours before or after the reference. When the previous and next
// occurrences are equally near, the previous occurrence wins so replay at an
// exact half-day boundary is deterministic.
//
// The reference may use any location. Calendar-day selection and the result
// are always normalized to UTC.
func ResolveEATUTC(hhmm string, reference time.Time) (time.Time, error) {
	hour, minute, err := parseEATHHMM(hhmm)
	if err != nil {
		return time.Time{}, err
	}

	reference = reference.UTC()
	previousDay := time.Date(
		reference.Year(), reference.Month(), reference.Day()-1,
		hour, minute, 0, 0, time.UTC,
	)
	currentDay := time.Date(
		reference.Year(), reference.Month(), reference.Day(),
		hour, minute, 0, 0, time.UTC,
	)
	nextDay := time.Date(
		reference.Year(), reference.Month(), reference.Day()+1,
		hour, minute, 0, 0, time.UTC,
	)

	nearest := previousDay
	for _, candidate := range [...]time.Time{currentDay, nextDay} {
		if durationMagnitude(candidate.Sub(reference)) < durationMagnitude(nearest.Sub(reference)) {
			nearest = candidate
		}
	}
	if durationMagnitude(nearest.Sub(reference)) > eatOccurrenceWindow {
		return time.Time{}, fmt.Errorf("%w: %q has no occurrence within 12 hours of %s", ErrInvalidEATHHMM, hhmm, reference.Format(time.RFC3339Nano))
	}
	return nearest, nil
}

func parseEATHHMM(value string) (int, int, error) {
	if len(value) != 4 {
		return 0, 0, fmt.Errorf("%w: expected four digits, got %q", ErrInvalidEATHHMM, value)
	}
	for index := range value {
		if value[index] < '0' || value[index] > '9' {
			return 0, 0, fmt.Errorf("%w: expected four digits, got %q", ErrInvalidEATHHMM, value)
		}
	}

	hour := int(value[0]-'0')*10 + int(value[1]-'0')
	minute := int(value[2]-'0')*10 + int(value[3]-'0')
	if hour > 23 || minute > 59 {
		return 0, 0, fmt.Errorf("%w: out-of-range time %q", ErrInvalidEATHHMM, value)
	}
	return hour, minute, nil
}

func durationMagnitude(value time.Duration) time.Duration {
	if value < 0 {
		return -value
	}
	return value
}
