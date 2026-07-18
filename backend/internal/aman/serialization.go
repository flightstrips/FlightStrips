package aman

import (
	"time"
)

// RFC3339Millis is the canonical JSON timestamp layout for AMAN owning wire
// packages. Domain fields remain time.Time; adapters call FormatTime before
// putting them in a transport DTO.
const RFC3339Millis = "2006-01-02T15:04:05.000Z07:00"

// FormatTime returns the required UTC RFC3339 value with exactly millisecond
// precision. It rejects non-UTC values so an adapter cannot silently emit a
// local-zone timestamp.
func FormatTime(value time.Time) (string, error) {
	if err := requireUTCTime("timestamp", value); err != nil {
		return "", err
	}
	return value.Format(RFC3339Millis), nil
}

// WholeSeconds returns a duration for an AMAN wire age/duration field. The
// domain keeps time.Duration, while wire packages must not silently discard
// sub-second precision.
func WholeSeconds(value time.Duration) (int64, error) {
	if value%time.Second != 0 {
		return 0, invalid("wire duration must be a whole number of seconds")
	}
	return int64(value / time.Second), nil
}
