package sat

import (
	"math/rand"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSelectStandFallsThroughTiersAndNormalizesEligibleWeights(t *testing.T) {
	config := testAssignmentConfig(t, `
    {"id":"sas","callsigns":["SAS"],"stands":{"tier1":{"A1":100},"tier2":{"A2":2,"A3":6}}}`)

	selection, err := config.SelectStand(AssignmentFlightFacts{Callsign: "SAS123"}, []string{"A2", "A3"}, func() float64 { return .3 })
	require.NoError(t, err)
	require.NotNil(t, selection)
	assert.Equal(t, "A3", selection.Stand)
	assert.Equal(t, "sas", selection.RuleID)
	assert.Equal(t, 2, selection.Tier)
	assert.Equal(t, "tier2", selection.TierName)
	assert.Equal(t, 6.0, selection.OriginalWeight)
	assert.Equal(t, .75, selection.NormalizedWeight)
	assert.False(t, selection.FallbackUsed)
}

func TestSelectStandExpandsGroupsAndIgnoresZeroWeight(t *testing.T) {
	document := `{
  "rules":[{"id":"sas","callsigns":["SAS"],"stands":{"tier1":{"Remote":10,"A1":0}}}],
  "stand_groups":{"Remote":["A1","A2"]},
  "fallback_rules":{` + fallbackJSON("A3") + `}
}`
	config, err := LoadAirlineAssignment(strings.NewReader(document), testAssignmentRegistry(t))
	require.NoError(t, err)

	selection, err := config.SelectStand(AssignmentFlightFacts{Callsign: "SAS123"}, []string{"A1", "A2"}, func() float64 { return 0 })
	require.NoError(t, err)
	require.NotNil(t, selection)
	assert.Equal(t, "A1", selection.Stand)
	assert.Equal(t, 5.0, selection.OriginalWeight)
	assert.Equal(t, 0.5, selection.NormalizedWeight)
}

func TestSelectStandPreservesGroupWeightAcrossExpansionAndAvailability(t *testing.T) {
	document := `{
  "rules":[{"id":"sas","callsigns":["SAS"],"stands":{"tier1":{"Large":40,"Small":20}}}],
  "stand_groups":{"Large":["A1","A2","A3"],"Small":["A4"]},
  "fallback_rules":{` + fallbackJSON("A3") + `}
}`
	config, err := LoadAirlineAssignment(strings.NewReader(document), testAssignmentRegistry(t))
	require.NoError(t, err)

	selection, err := config.SelectStand(AssignmentFlightFacts{Callsign: "SAS123"}, []string{"A1", "A2", "A3", "A4"}, func() float64 { return .7 })
	require.NoError(t, err)
	require.NotNil(t, selection)
	assert.Equal(t, "A4", selection.Stand)
	assert.Equal(t, 20.0, selection.OriginalWeight)
	assert.InDelta(t, 1.0/3.0, selection.NormalizedWeight, 1e-9)

	selection, err = config.SelectStand(AssignmentFlightFacts{Callsign: "SAS123"}, []string{"A1", "A4"}, func() float64 { return .5 })
	require.NoError(t, err)
	require.NotNil(t, selection)
	assert.Equal(t, "A1", selection.Stand)
	assert.Equal(t, 40.0, selection.OriginalWeight)
	assert.InDelta(t, 2.0/3.0, selection.NormalizedWeight, 1e-9)
}

func TestSelectStandUsesFallbackOnlyAfterAirlineRuleIsExhausted(t *testing.T) {
	document := `{
  "rules":[{"id":"sas","callsigns":["SAS"],"stands":{"tier1":{"A1":100}}}],
  "stand_groups":{},
  "fallback_rules":{
    "airliner_default":{"stands":{"tier1":{"A3":100}}},
    "business_vip":{"stands":{"tier1":{"A3":100}}},
    "cargo":{"stands":{"tier1":{"A2":100}}},
    "military":{"stands":{"tier1":{"A3":100}}},
    "military_helicopter":{"stands":{"tier1":{"A3":100}}},
    "helicopter":{"stands":{"tier1":{"A3":100}}},
    "ga_private":{"stands":{"tier1":{"A3":100}}},
    "unknown":{"stands":{"tier1":{"A3":100}}}
  }
}`
	config, err := LoadAirlineAssignment(strings.NewReader(document), testAssignmentRegistry(t))
	require.NoError(t, err)

	selection, err := config.SelectStand(AssignmentFlightFacts{Callsign: "SAS123", AircraftUse: AircraftUseCodeC}, []string{"A1", "A2"}, func() float64 { return 0 })
	require.NoError(t, err)
	require.NotNil(t, selection)
	assert.Equal(t, "A1", selection.Stand, "an eligible airline rule must win before fallback")
	assert.False(t, selection.FallbackUsed)

	selection, err = config.SelectStand(AssignmentFlightFacts{Callsign: "SAS123", AircraftUse: AircraftUseCodeC}, []string{"A2"}, func() float64 { return 0 })
	require.NoError(t, err)
	require.NotNil(t, selection)
	assert.Equal(t, "A2", selection.Stand)
	assert.Equal(t, FallbackCargo, selection.RuleID)
	assert.True(t, selection.FallbackUsed)
}

func TestSelectStandPrefersMatchingConditionSpecificRule(t *testing.T) {
	config := testAssignmentConfig(t, `
    {"id":"sas","callsigns":["SAS"],"stands":{"tier1":{"A1":100}}},
    {"id":"sas-non-schengen","callsigns":["SAS"],"conditions":{"border_status":"NON_SCHENGEN"},"stands":{"tier1":{"A2":100}}}`)

	selection, err := config.SelectStand(AssignmentFlightFacts{Callsign: "SAS123", BorderStatus: BorderStatusNonSchengen}, []string{"A1", "A2"}, func() float64 { return 0 })
	require.NoError(t, err)
	require.NotNil(t, selection)
	assert.Equal(t, "sas-non-schengen", selection.RuleID)
	assert.Equal(t, "A2", selection.Stand)
}

func TestSelectStandSeededDistributionRespectsWeights(t *testing.T) {
	config := testAssignmentConfig(t, `
    {"id":"sas","callsigns":["SAS"],"stands":{"tier1":{"A1":1,"A2":3}}}`)
	random := rand.New(rand.NewSource(7))
	counts := map[string]int{}
	for range 10_000 {
		selection, err := config.SelectStand(AssignmentFlightFacts{Callsign: "SAS123"}, []string{"A1", "A2"}, random.Float64)
		require.NoError(t, err)
		require.NotNil(t, selection)
		counts[selection.Stand]++
	}
	assert.InDelta(t, 0.25, float64(counts["A1"])/10_000, .02)
	assert.InDelta(t, 0.75, float64(counts["A2"])/10_000, .02)
}
