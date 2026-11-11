package sat

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func regionalPolicy(t *testing.T, airport string) (*StandCapabilityRegistry, *AirlineAssignmentConfig) {
	t.Helper()
	dir := filepath.Join("..", "..", "config", strings.ToLower(airport))
	stands, err := LoadStandCapabilityFile(filepath.Join(dir, "GRpluginStands.txt"))
	require.NoError(t, err)
	policy, err := LoadAirlineAssignmentFile(filepath.Join(dir, "airline_assignment.json"), stands.ForAirport(airport))
	require.NoError(t, err)
	return stands, policy
}

func TestRegionalSectorUseAndPriorityApplyToSelectionPreviewAndFallback(t *testing.T) {
	_, policy := regionalPolicy(t, "EKBI")
	for _, test := range []struct {
		name     string
		facts    AssignmentFlightFacts
		eligible []string
		want     string
		tier     int
	}{
		{"preferred airline", AssignmentFlightFacts{AircraftUse: "A"}, []string{"1", "4", "26", "65", "36"}, "26", 1},
		{"airline reserve", AssignmentFlightFacts{AircraftUse: "A"}, []string{"1", "4", "65", "36"}, "65", 3},
		{"business", AssignmentFlightFacts{AircraftUse: "B"}, []string{"1", "4", "26"}, "1", 2},
		{"private", AssignmentFlightFacts{AircraftUse: "P"}, []string{"1", "4", "26"}, "1", 2},
		{"helicopter", AssignmentFlightFacts{AircraftUse: "H"}, []string{"1", "4", "26"}, "1", 2},
		{"cargo", AssignmentFlightFacts{AircraftUse: "C"}, []string{"1", "4", "26"}, "4", 2},
		{"cargo operator", AssignmentFlightFacts{Callsign: "BCS123", AircraftUse: "A"}, []string{"4", "26"}, "4", 2},
		{"cargo remark", AssignmentFlightFacts{AircraftUse: "A", Remarks: "RMK/CARGO"}, []string{"4", "26"}, "4", 2},
		{"unknown defaults to airline", AssignmentFlightFacts{}, []string{"4", "26"}, "26", 1},
		{"cargo cannot escape use", AssignmentFlightFacts{AircraftUse: "C"}, []string{"26", "65"}, "", 0},
		{"military", AssignmentFlightFacts{AircraftUse: "M"}, []string{"1", "4", "26"}, "", 0},
		{"military helicopter", AssignmentFlightFacts{AircraftUse: "I"}, []string{"1", "4", "26"}, "", 0},
		{"military transport", AssignmentFlightFacts{AircraftUse: "T"}, []string{"1", "4", "26"}, "", 0},
		{"manual stand", AssignmentFlightFacts{AircraftUse: "A"}, []string{"36"}, "", 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, selectStand := range []func(AssignmentFlightFacts, []string, func() float64) (*StandSelection, error){policy.SelectStand, policy.SelectFallbackStand} {
				selection, err := selectStand(test.facts, test.eligible, func() float64 { return 0 })
				require.NoError(t, err)
				if test.want == "" {
					require.Nil(t, selection)
				} else {
					require.NotNil(t, selection)
					assert.Equal(t, test.want, selection.Stand)
					assert.Equal(t, test.tier, selection.Tier)
				}
			}
			for _, previewStand := range []func(AssignmentFlightFacts, []string) (StandSelectionPreview, error){policy.PreviewStandSelection, policy.PreviewFallbackStandSelection} {
				preview, err := previewStand(test.facts, test.eligible)
				require.NoError(t, err)
				if test.want == "" {
					assert.Empty(t, preview.Candidates)
				} else {
					require.NotEmpty(t, preview.Candidates)
					assert.Equal(t, test.want, preview.Candidates[0].Stand)
				}
			}
		})
	}
	pool, err := policy.FallbackStandPool(AssignmentFlightFacts{AircraftUse: "C"})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"4", "5", "6", "7"}, pool)
}

func TestRegionalAircraftFitRemainsIndependentOfUsePreference(t *testing.T) {
	stands, policy := regionalPolicy(t, "EKYT")
	facts := FlightCompatibilityFacts{AircraftKnown: true, Aircraft: Aircraft{Type: "A320", WingspanMetres: 34.1}, WTC: "M", EngineType: EngineJet}
	compatible := stands.EvaluateCompatibility("EKYT", facts)
	eligible := []string{}
	for _, match := range compatible.Matches {
		eligible = append(eligible, match.Stand.Name)
	}
	assert.ElementsMatch(t, []string{"4", "5", "6", "7", "8"}, eligible)
	selected, err := policy.SelectStand(AssignmentFlightFacts{AircraftUse: "A"}, eligible, func() float64 { return 0 })
	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.Equal(t, "4", selected.Stand)
	selected, err = policy.SelectStand(AssignmentFlightFacts{AircraftUse: "C"}, eligible, func() float64 { return 0 })
	require.NoError(t, err)
	assert.Nil(t, selected)
}

func TestSectorUseRejectsMalformedCategories(t *testing.T) {
	_, err := LoadStandCapabilities(strings.NewReader("STAND:EKBI:1:N055.44.42.780:E009.08.57.200:25\nUSE:AX\n"))
	require.ErrorContains(t, err, "unsupported category")
}

func TestSectorUseIsOptInAndRejectsAmbiguousVariantUse(t *testing.T) {
	_, policy := regionalPolicy(t, "EKBI")
	legacy := *policy
	legacy.EnforceStandUse = false
	selected, err := legacy.SelectStand(AssignmentFlightFacts{AircraftUse: "C"}, []string{"26"}, func() float64 { return 0 })
	require.NoError(t, err)
	require.NotNil(t, selected, "legacy policies retain their existing general fallback")
	assert.Equal(t, "26", selected.Stand)
	stands, err := LoadStandCapabilities(strings.NewReader("STAND:EKBI:1:N055.44.42.780:E009.08.57.200:25\nUSE:A\nSTAND:EKBI:1:N055.44.42.780:E009.08.57.200:25\nUSE:C\n"))
	require.NoError(t, err)
	document := `{"enforce_stand_use":true,"rules":[],"stand_groups":{},"fallback_rules":{` + fallbackJSON("1") + `}}`
	_, err = LoadAirlineAssignment(strings.NewReader(document), stands.ForAirport("EKBI"))
	require.ErrorContains(t, err, "conflicting variant USE")
	_, err = LoadAirlineAssignment(strings.NewReader(strings.Replace(document, `"enforce_stand_use":true`, `"cargo_callsigns":[""]`, 1)), stands.ForAirport("EKBI"))
	require.ErrorContains(t, err, "invalid airline prefix")
}
