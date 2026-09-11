package aman

// SequenceDisposition records whether a flight participates in AMAN
// sequencing. It is deliberately independent of FlightState: desequencing a
// flight does not change where it is in the arrival lifecycle.
type SequenceDisposition string

const (
	SequenceDispositionActive      SequenceDisposition = "active"
	SequenceDispositionDesequenced SequenceDisposition = "desequenced"
)

func (d SequenceDisposition) Valid() bool {
	return d == SequenceDispositionActive || d == SequenceDispositionDesequenced
}

// OrDefault preserves source compatibility for aggregate values created before
// the field existed. Persistence adapters should write the returned explicit
// value back to the aggregate after decoding legacy state.
func (d SequenceDisposition) OrDefault() SequenceDisposition {
	if d == "" {
		return SequenceDispositionActive
	}
	return d
}
