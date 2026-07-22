package aman

// EffectiveRolloutMode is the runtime result after technical and validation
// gates have been applied to a desired rollout mode. Blocked is intentionally
// not a desired configuration value: it makes a failed operational gate
// visible without silently changing the configured mode to disabled or shadow.
type EffectiveRolloutMode string

const (
	EffectiveDisabled      EffectiveRolloutMode = "disabled"
	EffectiveShadow        EffectiveRolloutMode = "shadow"
	EffectiveReadOnly      EffectiveRolloutMode = "read_only"
	EffectiveAuthoritative EffectiveRolloutMode = "authoritative"
	EffectiveBlocked       EffectiveRolloutMode = "blocked"
)

func EffectiveModeFor(mode RolloutMode) EffectiveRolloutMode {
	switch mode {
	case ModeDisabled:
		return EffectiveDisabled
	case ModeShadow:
		return EffectiveShadow
	case ModeReadOnly:
		return EffectiveReadOnly
	case ModeAuthoritative:
		return EffectiveAuthoritative
	default:
		return EffectiveBlocked
	}
}

func operationalMode(mode RolloutMode) bool {
	return mode == ModeReadOnly || mode == ModeAuthoritative
}
