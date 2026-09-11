package navdata

// HoldingSequencePolicy selects whether confirmed physical holding-stack
// position may influence sequence order for one STAR family. It is persisted
// with terminal policy so later sequence preparation can consume the exact
// airport-versioned operator choice.
type HoldingSequencePolicy string

const (
	// HoldingSequenceDisabled preserves ordinary landing-time and slot order.
	HoldingSequenceDisabled HoldingSequencePolicy = "disabled"
	// HoldingSequenceLowestAltitudeFirst allows the lowest confirmed aircraft
	// in one holding stack to be preferred by the sequence engine.
	HoldingSequenceLowestAltitudeFirst HoldingSequencePolicy = "lowest_altitude_first"
)

// Valid reports whether the policy is an explicit supported wire value.
func (p HoldingSequencePolicy) Valid() bool {
	return p == HoldingSequenceDisabled || p == HoldingSequenceLowestAltitudeFirst
}

// Effective returns the migration-safe behavior for a persisted legacy value.
// An empty value means the additive field was absent before #556-A.
func (p HoldingSequencePolicy) Effective() HoldingSequencePolicy {
	if p == "" {
		return HoldingSequenceDisabled
	}
	return p
}
