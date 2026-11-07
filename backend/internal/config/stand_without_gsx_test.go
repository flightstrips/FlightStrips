package config

import (
	"os"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRegionalStandInputsDoNotRequireGSXScenery(t *testing.T) {
	data, err := os.ReadFile("testdata/radis-services/ekyt.airport.yaml")
	require.NoError(t, err)
	without := regexp.MustCompile(`(?m)^    scenery:\r?\n      repo_path:[^\r\n]+\r?\n`).ReplaceAll(data, nil)
	require.NotEqual(t, data, without)
	for _, gsx := range []bool{false, true} {
		registry, err := LoadAirportFiles(ProductionAirportCatalog(), []string{selectedAirportFile(t, without)}, AirportLoadOptions{RepositoryRoot: testAirportRoot(t), GSXEnabled: gsx})
		require.NoError(t, err, "GSX enabled elsewhere must not require it for regionals")
		d, _ := registry.Lookup("EKYT")
		require.True(t, d.ServiceEnabled("stand_assignment"))
		_, exists := d.Resource("services.stand_assignment.stands")
		require.True(t, exists)
		_, exists = d.Resource("services.stand_assignment.scenery")
		require.False(t, exists)
	}
}
