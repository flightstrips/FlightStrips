package sat

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadAirlineAssignmentNormalizesCallsingsStandsAndConditions(t *testing.T) {
	config, err := LoadAirlineAssignment(strings.NewReader(`{
  "rules": [{
    "id": "sas-schengen",
    "callsigns": [" sas123 ", "sk*"],
    "conditions": {
      "border_status": "schengen",
      "aircraft_types": ["a32*"],
      "aircraft_use": ["a"],
      "direction": "arrival",
      "special": "  priority  "
    },
    "stands": {"tier1": {" a18 ": 25}}
  }],
  "stand_groups": {"EchoHigh": ["e70", "e72"]},
  "fallback_rules": {
    "airliner_default": {"stands": {"tier1": {"EchoHigh": 100}}},
    "business_vip": {"stands": {"tier1": {"E70": 100}}},
    "cargo": {"stands": {"tier1": {"E70": 100}}},
    "military": {"stands": {"tier1": {"E70": 100}}},
    "military_helicopter": {"stands": {"tier1": {"E70": 100}}},
    "helicopter": {"stands": {"tier1": {"E70": 100}}},
    "ga_private": {"stands": {"tier1": {"E70": 100}}},
    "unknown": {"stands": {"tier1": {"E70": 100}}}
  }
	}`), testAssignmentRegistry(t))
	require.NoError(t, err)
	assert.Equal(t, []string{"SAS123", "SK*"}, config.Rules[0].Callsigns)
	assert.Equal(t, "SCHENGEN", string(config.Rules[0].Conditions.BorderStatus))
	assert.Equal(t, "A32*", config.Rules[0].Conditions.AircraftTypes[0])
	assert.Equal(t, "A18", config.Rules[0].Tiers[0].Entries[0].Stand)
}

func TestLoadAirlineAssignmentRejectsLegacyAndUnknownJSONFields(t *testing.T) {
	_, err := LoadAirlineAssignment(strings.NewReader(`{
  "rules": [{"callsign": "SAS", "stands": {"tier1": {"A1": 1}}}],
  "stand_groups": {},
  "fallback_rules": {}
}`), testAssignmentRegistry(t))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "callsign")
}

func TestLoadAirlineAssignmentRequiresStandRegistry(t *testing.T) {
	_, err := LoadAirlineAssignment(strings.NewReader(`{
  "rules": [], "stand_groups": {}, "fallback_rules": {}
}`), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stand registry is nil")
}

func TestLoadCommittedAirlineAssignment(t *testing.T) {
	standRegistry, err := LoadStandCapabilityFile(filepath.Join("..", "..", "config", "ekch", "GRpluginStands.txt"))
	require.NoError(t, err)
	config, err := LoadAirlineAssignmentFile(filepath.Join("..", "..", "config", "ekch", "airline_assignment.json"), standRegistry)
	require.NoError(t, err)

	assert.Len(t, config.Rules, 62)
	assert.Equal(t, []string{"JTD"}, config.RulesByID("JTD_NON-SCHENGEN")[0].Callsigns)
	assert.NotEmpty(t, config.RulesByID("MEA"))
	assert.NotEmpty(t, config.RulesByID("MGH"))
	group, err := config.ResolveStandGroup("echoHigh")
	require.NoError(t, err)
	assert.Equal(t, []string{"E70", "E72", "E76", "E77", "E78"}, group)
	for _, fallback := range requiredFallbacks {
		_, ok := config.GetFallbackRule(fallback)
		assert.True(t, ok, fallback)
	}
	roundTripped, err := json.Marshal(config)
	require.NoError(t, err)
	_, err = LoadAirlineAssignment(strings.NewReader(string(roundTripped)), standRegistry)
	require.NoError(t, err)
}

func TestAirlineAssignmentRulePrecedenceAndFallbacks(t *testing.T) {
	config := testAssignmentConfig(t, `
    {"id":"prefix","callsigns":["SAS"],"stands":{"tier1":{"A1":1}}},
    {"id":"wildcard","callsigns":["SAS12*"],"stands":{"tier1":{"A2":1}}},
    {"id":"exact","callsigns":["SAS123"],"stands":{"tier1":{"A3":1}}},
    {"id":"special","callsigns":[],"conditions":{"special":"VIP"},"stands":{"tier1":{"A4":1}}}`)

	match, err := config.MatchRule(AssignmentFlightFacts{Callsign: "sas123", Special: "vip"})
	require.NoError(t, err)
	assert.Equal(t, "special", match.Rule.ID)

	match, err = config.MatchRule(AssignmentFlightFacts{Callsign: "SAS123"})
	require.NoError(t, err)
	assert.Equal(t, RulePrecedenceExactCallsign, match.Precedence)
	assert.Equal(t, "exact", match.Rule.ID)

	match, err = config.MatchRule(AssignmentFlightFacts{Callsign: "SAS129"})
	require.NoError(t, err)
	assert.Equal(t, "wildcard", match.Rule.ID)

	match, err = config.MatchRule(AssignmentFlightFacts{Callsign: "SAS999"})
	require.NoError(t, err)
	assert.Equal(t, "prefix", match.Rule.ID)

	match, err = config.MatchRule(AssignmentFlightFacts{Callsign: "ZZZ", AircraftUse: AircraftUseCodeC})
	require.NoError(t, err)
	assert.Equal(t, FallbackCargo, match.Fallback)
	assert.Equal(t, RulePrecedenceUseFallback, match.Precedence)
}

func TestAirlineAssignmentRejectsAmbiguityAndInvalidPreferences(t *testing.T) {
	for name, document := range map[string]string{
		"invalid border":  `{"rules":[{"callsigns":["SAS"],"conditions":{"border_status":"BOTH"},"stands":{"tier1":{"A1":1}}}],"stand_groups":{},"fallback_rules":{}}`,
		"negative weight": `{"rules":[{"callsigns":["SAS"],"stands":{"tier1":{"A1":-1}}}],"stand_groups":{},"fallback_rules":{}}`,
		"empty tier":      `{"rules":[{"callsigns":["SAS"],"stands":{"tier1":{}}}],"stand_groups":{},"fallback_rules":{}}`,
		"missing default": `{"rules":[],"stand_groups":{},"fallback_rules":{"cargo":{"stands":{"tier1":{"A1":1}}}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := LoadAirlineAssignment(strings.NewReader(document), testAssignmentRegistry(t))
			require.Error(t, err)
		})
	}

	config := testAssignmentConfig(t, `
    {"id":"one","callsigns":["SAS*"],"stands":{"tier1":{"A1":1}}},
    {"id":"two","callsigns":["SAS*"],"stands":{"tier1":{"A2":1}}}`)
	_, err := config.MatchRule(AssignmentFlightFacts{Callsign: "SAS123"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ambiguous")
}

func TestAirlineAssignmentRejectsCircularGroups(t *testing.T) {
	_, err := LoadAirlineAssignment(strings.NewReader(`{
  "rules": [],
  "stand_groups": {"A": ["B"], "B": ["A"]},
  "fallback_rules": {
    "airliner_default": {"stands": {"tier1": {"A": 1}}},
    "business_vip": {"stands": {"tier1": {"A": 1}}},
    "cargo": {"stands": {"tier1": {"A": 1}}},
    "military": {"stands": {"tier1": {"A": 1}}},
    "military_helicopter": {"stands": {"tier1": {"A": 1}}},
    "helicopter": {"stands": {"tier1": {"A": 1}}},
    "ga_private": {"stands": {"tier1": {"A": 1}}},
    "unknown": {"stands": {"tier1": {"A": 1}}}
  }
}`), testAssignmentRegistry(t))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "circular")
}

func (c *AirlineAssignmentConfig) RulesByID(id string) []AirlineAssignmentRule {
	var result []AirlineAssignmentRule
	for _, rule := range c.Rules {
		if rule.ID == id {
			result = append(result, rule)
		}
	}
	return result
}

func testAssignmentConfig(t *testing.T, rules string) *AirlineAssignmentConfig {
	t.Helper()
	document := `{"rules":[` + strings.TrimSpace(rules) + `],"stand_groups":{},"fallback_rules":{` + fallbackJSON("A1") + `}}`
	config, err := LoadAirlineAssignment(strings.NewReader(document), testAssignmentRegistry(t))
	require.NoError(t, err)
	return config
}

func testAssignmentRegistry(t *testing.T) *StandCapabilityRegistry {
	t.Helper()
	registry, err := LoadStandCapabilities(strings.NewReader(`
STAND:EKCH:A1:N055.37.42.710:E012.38.33.450:30
STAND:EKCH:A2:N055.37.42.710:E012.38.33.451:30
STAND:EKCH:A3:N055.37.42.710:E012.38.33.452:30
STAND:EKCH:A4:N055.37.42.710:E012.38.33.453:30
STAND:EKCH:A18:N055.37.42.710:E012.38.33.454:30
STAND:EKCH:E70:N055.37.42.710:E012.38.33.455:30
STAND:EKCH:E72:N055.37.42.710:E012.38.33.456:30
`))
	require.NoError(t, err)
	return registry
}

func fallbackJSON(stand string) string {
	result := make([]string, 0, len(requiredFallbacks))
	for _, name := range requiredFallbacks {
		result = append(result, `"`+name+`":{"stands":{"tier1":{"`+stand+`":100}}}`)
	}
	return strings.Join(result, ",")
}
