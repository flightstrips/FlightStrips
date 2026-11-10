package sat

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommittedRegionalStandFilesResolveAllBlockingReferences(t *testing.T) {
	for _, airport := range []struct {
		directory string
		icao      string
		count     int
	}{{"ekbi", "EKBI", 29}, {"ekyt", "EKYT", 9}} {
		t.Run(airport.icao, func(t *testing.T) {
			registry, err := LoadStandCapabilityFile(filepath.Join("..", "..", "config", airport.directory, "GRpluginStands.txt"))
			require.NoError(t, err)
			require.Len(t, registry.Stands(airport.icao), airport.count)
			if airport.icao != "EKBI" {
				return
			}
			stand, ok := registry.Lookup("EKBI", "36")
			require.True(t, ok)
			assert.InDelta(t, 55+44.0/60+42.78/3600, stand.Latitude, 1e-10)
			assert.InDelta(t, 9+8.0/60+57.20/3600, stand.Longitude, 1e-10)
			require.Len(t, stand.Variants, 1)
			assert.True(t, stand.Variants[0].Manual, "published coordinates do not establish automatic aircraft compatibility")
			for _, match := range registry.EvaluateCompatibility("EKBI", FlightCompatibilityFacts{}).Matches {
				assert.NotEqual(t, "36", match.Stand.Name, "stand 36 must not become an unrestricted automatic candidate")
			}
			manualMatch := false
			for _, match := range registry.EvaluateManualCompatibility("EKBI", FlightCompatibilityFacts{}).Matches {
				manualMatch = manualMatch || match.Stand.Name == "36"
			}
			assert.True(t, manualMatch, "stand 36 remains available for explicit selection")
			adjacent, ok := registry.Lookup("EKBI", "37")
			require.True(t, ok)
			assert.Contains(t, adjacent.Blocks, "36", "preserve the sector blocking relationship")
		})
	}
}
