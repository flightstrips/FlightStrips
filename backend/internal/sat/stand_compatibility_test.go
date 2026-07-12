package sat

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvaluateStandCompatibilityMatchesCompleteVariantWithoutMixingDuplicates(t *testing.T) {
	registry := compatibilityRegistry(t, `
STAND:EKCH:A1:N055.37.42.710:E012.38.33.450:30
SCHENGEN
WTC:H
ENGINETYPE:J
WINGSPAN:40
BLOCKS:A2
STAND:EKCH:A1:N055.37.42.710:E012.38.33.450:30
NON-SCHENGEN
WTC:M
ENGINETYPE:T
WINGSPAN:30
BLOCKS:A3
STAND:EKCH:A2:N055.37.42.000:E012.38.33.000:30
MANUAL
STAND:EKCH:A3:N055.37.43.000:E012.38.34.000:30
MANUAL
`)

	result := registry.EvaluateCompatibility("EKCH", compatibilityFacts())
	assert.Empty(t, result.Matches, "no one variant contains the required border, WTC, engine, and wingspan facts")
	assert.Contains(t, rejectionCapabilities(result, "A1"), StandCapabilityBorder)
	assert.Contains(t, rejectionCapabilities(result, "A1"), StandCapabilityWTC)
	assert.Contains(t, rejectionCapabilities(result, "A1"), StandCapabilityEngineType)
	assert.Contains(t, rejectionCapabilities(result, "A1"), StandCapabilityWingspan)
}

func TestEvaluateStandCompatibilityCapabilityMatrix(t *testing.T) {
	base := compatibilityFacts()
	for _, test := range []struct {
		name       string
		directive  string
		facts      FlightCompatibilityFacts
		capability StandCompatibilityCapability
	}{
		{"schengen rejects non-schengen", "SCHENGEN", withBorder(base, BorderStatusNonSchengen), StandCapabilityBorder},
		{"non-schengen rejects schengen", "NON-SCHENGEN", withBorder(base, BorderStatusSchengen), StandCapabilityBorder},
		{"WTC allow", "WTC:H", base, StandCapabilityWTC},
		{"WTC deny", "NOTWTC:M", base, StandCapabilityWTC},
		{"engine allow", "ENGINETYPE:T", base, StandCapabilityEngineType},
		{"engine deny", "NOTENGINETYPE:J", base, StandCapabilityEngineType},
		{"wingspan", "WINGSPAN:35", base, StandCapabilityWingspan},
		{"length", "LENGTH:35", base, StandCapabilityLength},
		{"height", "HEIGHT:10", base, StandCapabilityHeight},
		{"MTOW", "MTOW:70000", base, StandCapabilityMTOW},
		{"code", "CODE:B", base, StandCapabilityCode},
		{"aircraft allow GR wildcard", "ATYP:B7*", base, StandCapabilityAircraftType},
		{"aircraft deny GR wildcard", "NOTATYP:A?0N", base, StandCapabilityAircraftType},
	} {
		t.Run(test.name, func(t *testing.T) {
			registry := compatibilityRegistry(t, standRecord("A1", test.directive))
			result := registry.EvaluateCompatibility("EKCH", test.facts)
			assert.Empty(t, result.Matches)
			assert.Contains(t, rejectionCapabilities(result, "A1"), test.capability)
		})
	}
}

func TestEvaluateStandCompatibilitySupportsGRWildcardPatterns(t *testing.T) {
	registry := compatibilityRegistry(t, standRecord("A1", "ATYP:A*0?"))
	result := registry.EvaluateCompatibility("EKCH", compatibilityFacts())
	require.Len(t, result.Matches, 1)
	assert.Empty(t, result.Rejections)
}

func TestEvaluateStandCompatibilityIgnoresGRPreferenceDirectives(t *testing.T) {
	registry := compatibilityRegistry(t, `
STAND:EKCH:A1:N055.37.42.710:E012.38.33.450:30
WTC:M
PRIORITY:-99
USE:ARR
CALLSIGN:ZZZ
NOTCALLSIGN:A20
ROUTE:NEVER
`)

	result := registry.EvaluateCompatibility("EKCH", compatibilityFacts())
	require.Len(t, result.Matches, 1)
	assert.Equal(t, "A1", result.Matches[0].Stand.Name)
	assert.Empty(t, result.Rejections)
}

func TestEvaluateStandCompatibilityRejectsUnknownFactsOnlyWhenRestricted(t *testing.T) {
	unknown := FlightCompatibilityFacts{
		EngineType:   EngineUnknown,
		WTC:          "UNKNOWN",
		BorderStatus: BorderStatusUnknown,
	}

	unrestricted := compatibilityRegistry(t, standRecord("A1", ""))
	assert.Len(t, unrestricted.EvaluateCompatibility("EKCH", unknown).Matches, 1)

	for _, directive := range []string{
		"SCHENGEN", "WTC:M", "NOTWTC:H", "ENGINETYPE:J", "NOTENGINETYPE:T",
		"ATYP:A20N", "NOTATYP:B738", "WINGSPAN:40", "LENGTH:40", "HEIGHT:20", "MTOW:90000", "CODE:A",
	} {
		registry := compatibilityRegistry(t, standRecord("A1", directive))
		result := registry.EvaluateCompatibility("EKCH", unknown)
		assert.Emptyf(t, result.Matches, "directive %s must not be bypassed by unknown facts", directive)
		assert.NotEmpty(t, result.Rejections)
	}
}

func TestEvaluateStandCompatibilityExcludesManualVariantsButManualSelectionRetainsThem(t *testing.T) {
	registry := compatibilityRegistry(t, standRecord("M1", "MANUAL"))
	facts := compatibilityFacts()

	automatic := registry.EvaluateCompatibility("EKCH", facts)
	assert.Empty(t, automatic.Matches)
	assert.Equal(t, []StandCompatibilityCapability{StandCapabilityManual}, rejectionCapabilities(automatic, "M1"))

	manual := registry.EvaluateManualCompatibility("EKCH", facts)
	require.Len(t, manual.Matches, 1)
	assert.True(t, manual.Matches[0].Variant.Manual)
}

func TestEvaluateStandCompatibilityReturnsVariantBlocksWithoutUsingThemForFit(t *testing.T) {
	registry := compatibilityRegistry(t, `
STAND:EKCH:A1:N055.37.42.710:E012.38.33.450:30
BLOCKS:A2
STAND:EKCH:A1:N055.37.42.710:E012.38.33.450:30
WTC:H
BLOCKS:A3
STAND:EKCH:A2:N055.37.42.000:E012.38.33.000:30
MANUAL
STAND:EKCH:A3:N055.37.43.000:E012.38.34.000:30
MANUAL
`)

	result := registry.EvaluateCompatibility("EKCH", compatibilityFacts())
	require.Len(t, result.Matches, 1)
	assert.Equal(t, []string{"A2"}, result.Matches[0].Blocks)
	assert.NotContains(t, rejectionCapabilities(result, "A1"), StandCapabilityManual)
}

func compatibilityRegistry(t *testing.T, data string) *StandCapabilityRegistry {
	t.Helper()
	registry, err := LoadStandCapabilities(strings.NewReader(data))
	require.NoError(t, err)
	return registry
}

func compatibilityFacts() FlightCompatibilityFacts {
	return FlightCompatibilityFacts{
		Aircraft: Aircraft{
			Type:           "A20N",
			WingspanMetres: 35.8,
			LengthMetres:   37.57,
			HeightMetres:   11.76,
			MTOWKilograms:  79000,
			UseCode:        AircraftUseCodeA,
		},
		AircraftKnown: true,
		EngineType:    EngineJet,
		WTC:           "M",
		BorderStatus:  BorderStatusSchengen,
	}
}

func withBorder(facts FlightCompatibilityFacts, status BorderStatus) FlightCompatibilityFacts {
	facts.BorderStatus = status
	return facts
}

func standRecord(name, directive string) string {
	if directive != "" {
		directive += "\n"
	}
	return "STAND:EKCH:" + name + ":N055.37.42.710:E012.38.33.450:30\n" + directive
}

func rejectionCapabilities(result StandCompatibilityEvaluation, stand string) []StandCompatibilityCapability {
	resultCapabilities := make([]StandCompatibilityCapability, 0)
	for _, rejection := range result.Rejections {
		if rejection.Stand == stand {
			resultCapabilities = append(resultCapabilities, rejection.Capability)
		}
	}
	return resultCapabilities
}
