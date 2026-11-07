package config

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRegionalSectorCoverageNeedsNoAdditionalOperatingInputs(t *testing.T) {
	for _, airport := range []string{"EKBI", "EKYT"} {
		t.Run(airport, func(t *testing.T) {
			registry, err := LoadAirportFiles(ProductionAirportCatalog(), []string{"backend/config/airports/" + strings.ToLower(airport) + ".airport.yaml"}, AirportLoadOptions{RepositoryRoot: testAirportRoot(t)})
			require.NoError(t, err)
			d, _ := registry.Lookup(airport)
			frequency := func(name string) string {
				for _, p := range d.Document().Positions {
					if p.Name == name {
						return p.Frequency
					}
				}
				t.Fatalf("missing sector position %s", name)
				return ""
			}
			participant := func(key, name string) CoverageParticipant {
				return CoverageParticipant{Key: key, Callsign: name, PrimaryFrequency: frequency(name)}
			}
			area := participant("area", "EKDK_L_CTR")
			tower := participant("tower", airport+"_TWR")
			approach := participant("approach", airport+"_APP")
			final := participant("final", airport+"_F_APP")
			require.Contains(t, d.CoverageResponsibilities([]CoverageParticipant{area})[area.Key], "approach")
			require.Contains(t, d.CoverageResponsibilities([]CoverageParticipant{area, tower})[area.Key], "approach", "TWR leaves EKDK's approach responsibility intact")
			require.NotContains(t, d.CoverageResponsibilities([]CoverageParticipant{area, tower, approach}), area.Key, "APP replaces EKDK's remaining duties")
			require.Equal(t, []string{"final"}, d.CoverageResponsibilities([]CoverageParticipant{area, tower, approach, final})[final.Key])
			require.Contains(t, d.CoverageResponsibilities([]CoverageParticipant{area, tower})[area.Key], "approach", "EKDK returns when APP leaves")
			for _, fallback := range []string{"EKDK_V_CTR", "EKDK_UV_CTR", "EKDK_UN_CTR", "EKDK_N_CTR"} {
				require.Contains(t, d.CoverageResponsibilities([]CoverageParticipant{participant("fallback", fallback)})["fallback"], "approach")
			}
			area.TunedFrequencies = []string{frequency(airport + "_TWR")}
			require.Contains(t, d.CoverageResponsibilities([]CoverageParticipant{area, approach})[area.Key], "tower", "tuned local frequency retains its supplied duty")
			area.Observer = true
			require.NotContains(t, d.CoverageResponsibilities([]CoverageParticipant{area}), area.Key)
			require.Zero(t, d.RelinquishGrace())
			require.Less(t, d.MasterPriority(tower.Callsign, tower.PrimaryFrequency), d.MasterPriority(approach.Callsign, approach.PrimaryFrequency))
			require.Less(t, d.MasterPriority(approach.Callsign, approach.PrimaryFrequency), d.MasterPriority("EKDK_L_CTR", frequency("EKDK_L_CTR")))
		})
	}
}
