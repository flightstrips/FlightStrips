package config

import (
	"github.com/stretchr/testify/require"
	"testing"
)

// All operating values in these fixtures are synthetic, never rollout defaults.
func coverageDefinition() *AirportDefinition {
	return &AirportDefinition{document: AirportDocument{Airport: "ZZZZ", Positions: []Position{
		{Name: "ZZZZ_TWR", Frequency: "118.000"}, {Name: "ZZZZ_APP", Frequency: "119.000"}, {Name: "ZZXX_CTR", Frequency: "120.000"},
	}, Coverage: AirportCoverage{
		Matching:         CoverageMatching{PrimaryFrequency: true, TunedFrequencies: true, CallsignFallback: true},
		Membership:       CoverageMembership{Policy: "remaining-responsibilities/v1"},
		Responsibilities: []AirportResponsibility{{ID: "tower", Providers: []string{"ZZZZ_TWR", "ZZZZ_APP", "ZZXX_CTR"}}, {ID: "approach", Providers: []string{"ZZZZ_APP", "ZZXX_CTR"}}},
	}}}
}

func TestCoverageRetainsRemainingResponsibilitiesWithoutOwnership(t *testing.T) {
	d := coverageDefinition()
	area := CoverageParticipant{Key: "area", Callsign: "ZZXX_CTR", PrimaryFrequency: "120000000"}
	tower := CoverageParticipant{Key: "tower", Callsign: "ZZZZ_TWR", PrimaryFrequency: "118.000"}
	approach := CoverageParticipant{Key: "app", Callsign: "ZZZZ_APP", PrimaryFrequency: "119.000"}
	require.Equal(t, []string{"tower", "approach"}, d.CoverageResponsibilities([]CoverageParticipant{area})["area"])
	require.Equal(t, []string{"approach"}, d.CoverageResponsibilities([]CoverageParticipant{tower, area})["area"])
	require.NotContains(t, d.CoverageResponsibilities([]CoverageParticipant{approach, tower, area}), "area")
}

func TestCoverageRadioCarriageAndConfiguredFallback(t *testing.T) {
	d := coverageDefinition()
	area := CoverageParticipant{Key: "area", Callsign: "ZZXX_CTR", PrimaryFrequency: "120.000", TunedFrequencies: []string{"118000000"}}
	app := CoverageParticipant{Key: "app", Callsign: "ZZZZ_APP", PrimaryFrequency: "119.000"}
	require.Equal(t, []string{"tower"}, d.CoverageResponsibilities([]CoverageParticipant{app, area})["area"])
	area.TunedFrequencies = nil
	require.NotContains(t, d.CoverageResponsibilities([]CoverageParticipant{app, area}), "area")
	area.Callsign = "UNCONFIGURED_CTR"
	area.PrimaryFrequency = "121.000"
	area.TunedFrequencies = []string{"118.000"}
	require.Equal(t, []string{"tower"}, d.CoverageResponsibilities([]CoverageParticipant{app, area})["area"])
	area.Observer = true
	require.NotContains(t, d.CoverageResponsibilities([]CoverageParticipant{app, area}), "area")
	area.Observer = false
	area.TunedFrequencies = nil
	area.Callsign = "ZZXX_CTR"
	require.NotEmpty(t, d.CoverageResponsibilities([]CoverageParticipant{area})["area"])
	d.document.Coverage.Matching.CallsignFallback = false
	require.Empty(t, d.CoverageResponsibilities([]CoverageParticipant{area}))
}

func TestCoverageDeclarationRequiresEnabledDefinitionAndServerAuthority(t *testing.T) {
	d := coverageDefinition()
	r := &AirportRegistry{definitions: map[string]*AirportDefinition{"ZZZZ": d}}
	require.Empty(t, r.CoverageAirports([]string{"ZZZZ"}, nil))
	require.Empty(t, r.CoverageAirports([]string{"ZZZZ"}, func(string) bool { return false }))
	require.Empty(t, r.CoverageAirports([]string{"DISABLED"}, func(string) bool { return true }))
	require.Equal(t, []*AirportDefinition{d}, r.CoverageAirports([]string{"ZZZZ", "ZZZZ", "DISABLED"}, func(string) bool { return true }))
	d.document.Coverage.Membership.Policy = "legacy-single-airport/v1"
	require.Empty(t, r.CoverageAirports([]string{"ZZZZ"}, func(string) bool { return true }))
}
