package sat

import (
	"fmt"
	"slices"
	"strings"
)

// StandCompatibilityCapability identifies the stand capability that rejected
// a flight. BLOCKS is deliberately absent: occupancy is evaluated by the
// transactional allocator, not by this physical-fit pass.
type StandCompatibilityCapability string

const (
	StandCapabilityManual       StandCompatibilityCapability = "MANUAL"
	StandCapabilityBorder       StandCompatibilityCapability = "BORDER"
	StandCapabilityAircraftType StandCompatibilityCapability = "AIRCRAFT_TYPE"
	StandCapabilityWTC          StandCompatibilityCapability = "WTC"
	StandCapabilityEngineType   StandCompatibilityCapability = "ENGINE_TYPE"
	StandCapabilityWingspan     StandCompatibilityCapability = "WINGSPAN"
	StandCapabilityLength       StandCompatibilityCapability = "LENGTH"
	StandCapabilityHeight       StandCompatibilityCapability = "HEIGHT"
	StandCapabilityMTOW         StandCompatibilityCapability = "MTOW"
	StandCapabilityCode         StandCompatibilityCapability = "CODE"
)

// StandCompatibilityRejection records one failed capability on one complete
// stand variant. Expected is the directive value and Actual is the normalized
// flight fact; this makes diagnostics useful without parsing text messages.
type StandCompatibilityRejection struct {
	Airport     string
	Stand       string
	VariantLine int
	Capability  StandCompatibilityCapability
	Expected    string
	Actual      string
}

// StandCompatibilityMatch is a physical stand and the complete variant that
// passed all physical and border checks. Blocks belongs to that variant and is
// returned for the allocator to consider later; it is never a fit rejection.
type StandCompatibilityMatch struct {
	Stand   Stand
	Variant StandCapability
	Blocks  []string
}

// StandCompatibilityEvaluation separates qualifying stands from structured
// rejections. A physical stand appears at most once in Matches, even when more
// than one of its variants would fit.
type StandCompatibilityEvaluation struct {
	Matches    []StandCompatibilityMatch
	Rejections []StandCompatibilityRejection
}

// EvaluateStandCompatibility evaluates complete variants for automatic stand
// assignment. MANUAL variants are retained in the registry but excluded here;
// use EvaluateManualStandCompatibility for an explicit controller selection.
func EvaluateStandCompatibility(stands []Stand, facts FlightCompatibilityFacts) StandCompatibilityEvaluation {
	return evaluateStandCompatibility(stands, facts, false)
}

// EvaluateManualStandCompatibility evaluates complete variants for an
// explicit manual stand selection. It includes MANUAL variants but still
// applies every physical and border restriction.
func EvaluateManualStandCompatibility(stands []Stand, facts FlightCompatibilityFacts) StandCompatibilityEvaluation {
	return evaluateStandCompatibility(stands, facts, true)
}

// EvaluateCompatibility evaluates all stands at an airport for automatic
// assignment.
func (r *StandCapabilityRegistry) EvaluateCompatibility(airport string, facts FlightCompatibilityFacts) StandCompatibilityEvaluation {
	if r == nil {
		return StandCompatibilityEvaluation{}
	}
	return EvaluateStandCompatibility(r.Stands(airport), facts)
}

// EvaluateManualCompatibility evaluates all stands at an airport for explicit
// manual selection, including MANUAL variants.
func (r *StandCapabilityRegistry) EvaluateManualCompatibility(airport string, facts FlightCompatibilityFacts) StandCompatibilityEvaluation {
	if r == nil {
		return StandCompatibilityEvaluation{}
	}
	return EvaluateManualStandCompatibility(r.Stands(airport), facts)
}

func evaluateStandCompatibility(stands []Stand, facts FlightCompatibilityFacts, includeManual bool) StandCompatibilityEvaluation {
	result := StandCompatibilityEvaluation{}
	for _, stand := range stands {
		for _, variant := range stand.Variants {
			rejections := evaluateStandVariant(stand.Airport, stand.Name, variant, facts, includeManual)
			if len(rejections) != 0 {
				result.Rejections = append(result.Rejections, rejections...)
				continue
			}
			result.Matches = append(result.Matches, StandCompatibilityMatch{
				Stand:   cloneStand(stand),
				Variant: cloneStandCapability(variant),
				Blocks:  slices.Clone(variant.Blocks),
			})
			break
		}
	}
	return result
}

func evaluateStandVariant(airport, stand string, variant StandCapability, facts FlightCompatibilityFacts, includeManual bool) []StandCompatibilityRejection {
	rejections := make([]StandCompatibilityRejection, 0)
	reject := func(capability StandCompatibilityCapability, expected, actual string) {
		rejections = append(rejections, StandCompatibilityRejection{
			Airport:     airport,
			Stand:       stand,
			VariantLine: variant.Line,
			Capability:  capability,
			Expected:    expected,
			Actual:      actual,
		})
	}

	if variant.Manual && !includeManual {
		reject(StandCapabilityManual, "manual selection", "automatic assignment")
	}
	if variant.BorderClass != StandBorderAny && !borderClassMatches(variant.BorderClass, facts.BorderStatus) {
		reject(StandCapabilityBorder, string(variant.BorderClass), string(facts.BorderStatus))
	}

	if len(variant.AircraftTypes) > 0 && (!facts.AircraftKnown || !matchesAnyGRPattern(variant.AircraftTypes, facts.Aircraft.Type)) {
		reject(StandCapabilityAircraftType, strings.Join(variant.AircraftTypes, ","), aircraftTypeActual(facts))
	}
	if len(variant.NotAircraftTypes) > 0 && (!facts.AircraftKnown || matchesAnyGRPattern(variant.NotAircraftTypes, facts.Aircraft.Type)) {
		reject(StandCapabilityAircraftType, "NOT "+strings.Join(variant.NotAircraftTypes, ","), aircraftTypeActual(facts))
	}

	if len(variant.WTC) > 0 && !matchesCapabilityTokens(variant.WTC, facts.WTC) {
		reject(StandCapabilityWTC, strings.Join(variant.WTC, ","), facts.WTC)
	}
	if len(variant.NotWTC) > 0 && (!knownWTC(facts.WTC) || matchesCapabilityTokens(variant.NotWTC, facts.WTC)) {
		reject(StandCapabilityWTC, "NOT "+strings.Join(variant.NotWTC, ","), facts.WTC)
	}
	if len(variant.EngineTypes) > 0 && !matchesCapabilityTokens(variant.EngineTypes, string(facts.EngineType)) {
		reject(StandCapabilityEngineType, strings.Join(variant.EngineTypes, ","), string(facts.EngineType))
	}
	if len(variant.NotEngineTypes) > 0 && (facts.EngineType == EngineUnknown || matchesCapabilityTokens(variant.NotEngineTypes, string(facts.EngineType))) {
		reject(StandCapabilityEngineType, "NOT "+strings.Join(variant.NotEngineTypes, ","), string(facts.EngineType))
	}

	if variant.Wingspan > 0 && (!facts.AircraftKnown || facts.Aircraft.WingspanMetres > variant.Wingspan) {
		reject(StandCapabilityWingspan, formatLimit(variant.Wingspan), aircraftDimensionActual(facts, func(a Aircraft) float64 { return a.WingspanMetres }))
	}
	if variant.Length > 0 && (!facts.AircraftKnown || facts.Aircraft.LengthMetres > variant.Length) {
		reject(StandCapabilityLength, formatLimit(variant.Length), aircraftDimensionActual(facts, func(a Aircraft) float64 { return a.LengthMetres }))
	}
	if variant.Height > 0 && (!facts.AircraftKnown || facts.Aircraft.HeightMetres > variant.Height) {
		reject(StandCapabilityHeight, formatLimit(variant.Height), aircraftDimensionActual(facts, func(a Aircraft) float64 { return a.HeightMetres }))
	}
	if variant.MTOW > 0 && (!facts.AircraftKnown || facts.Aircraft.MTOWKilograms > variant.MTOW) {
		reject(StandCapabilityMTOW, formatLimit(variant.MTOW), aircraftDimensionActual(facts, func(a Aircraft) float64 { return a.MTOWKilograms }))
	}
	if variant.Code != "" && (!facts.AircraftKnown || !strings.Contains(variant.Code, string(facts.Aircraft.UseCode))) {
		reject(StandCapabilityCode, variant.Code, aircraftUseActual(facts))
	}
	return rejections
}

func borderClassMatches(class StandBorderClass, status BorderStatus) bool {
	return (class == StandBorderSchengen && status == BorderStatusSchengen) ||
		(class == StandBorderNonSchengen && status == BorderStatusNonSchengen)
}

func matchesCapabilityTokens(tokens []string, value string) bool {
	if value == "" || value == "UNKNOWN" {
		return false
	}
	for _, token := range tokens {
		if strings.Contains(token, value) {
			return true
		}
	}
	return false
}

func knownWTC(value string) bool {
	return validWTC(value)
}

func matchesAnyGRPattern(patterns []string, value string) bool {
	for _, pattern := range patterns {
		if grWildcardMatch(pattern, value) {
			return true
		}
	}
	return false
}

// grWildcardMatch implements GRPlugin's case-insensitive glob-style matching:
// '*' matches any sequence and '?' matches one character. Stand configuration
// is normalized during load, but normalizing here keeps direct callers safe.
func grWildcardMatch(pattern, value string) bool {
	pattern = normalizeToken(pattern)
	value = normalizeToken(value)
	patternIndex, valueIndex, starIndex, resumeValue := 0, 0, -1, 0
	for valueIndex < len(value) {
		switch {
		case patternIndex < len(pattern) && (pattern[patternIndex] == '?' || pattern[patternIndex] == value[valueIndex]):
			patternIndex++
			valueIndex++
		case patternIndex < len(pattern) && pattern[patternIndex] == '*':
			starIndex = patternIndex
			patternIndex++
			resumeValue = valueIndex
		case starIndex >= 0:
			patternIndex = starIndex + 1
			resumeValue++
			valueIndex = resumeValue
		default:
			return false
		}
	}
	for patternIndex < len(pattern) && pattern[patternIndex] == '*' {
		patternIndex++
	}
	return patternIndex == len(pattern)
}

func aircraftTypeActual(facts FlightCompatibilityFacts) string {
	if !facts.AircraftKnown {
		return "UNKNOWN"
	}
	return facts.Aircraft.Type
}

func aircraftUseActual(facts FlightCompatibilityFacts) string {
	if !facts.AircraftKnown {
		return "UNKNOWN"
	}
	return string(facts.Aircraft.UseCode)
}

func aircraftDimensionActual(facts FlightCompatibilityFacts, value func(Aircraft) float64) string {
	if !facts.AircraftKnown {
		return "UNKNOWN"
	}
	return formatLimit(value(facts.Aircraft))
}

func formatLimit(value float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.3f", value), "0"), ".")
}
