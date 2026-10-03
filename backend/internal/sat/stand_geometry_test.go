package sat

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStandAtPosition(t *testing.T) {
	registry, err := LoadStandCapabilities(strings.NewReader(`
STAND:EKCH:A1:N055.37.42.710:E012.38.33.450:30
STAND:EKCH:A2:N055.37.42.710:E012.38.36.450:30
`))
	require.NoError(t, err)

	stand, found := registry.StandAtPosition("ekch", 55.6285306, 12.642625)
	require.True(t, found)
	assert.Equal(t, "A1", stand.Name)

	_, found = registry.StandAtPosition("EKCH", 55.7, 12.7)
	assert.False(t, found)

	assert.True(t, registry.PositionNearAirport("EKCH", 55.6285306, 12.644625, 200))
	assert.False(t, registry.PositionNearAirport("EKCH", 55.7, 12.7, 200))
}

// Coincident centres must not select according to randomized map traversal.
// Adding a strictly closer stand still takes precedence over lexical names.
func TestStandAtPositionResolvesExactDistanceTiesByName(t *testing.T) {
	registry := &StandCapabilityRegistry{byAirport: map[string]map[string]Stand{"TEST": {
		"Z9": {Name: "Z9", Latitude: 55, Longitude: 12, Radius: 1000},
		"A1": {Name: "A1", Latitude: 55, Longitude: 12, Radius: 1000},
	}}}
	for i := 0; i < 200; i++ {
		stand, found := registry.StandAtPosition("TEST", 55, 12.001)
		require.True(t, found)
		require.Equal(t, "A1", stand.Name)
	}
	registry.byAirport["TEST"]["Z0"] = Stand{Name: "Z0", Latitude: 55, Longitude: 12.001, Radius: 1000}
	stand, found := registry.StandAtPosition("TEST", 55, 12.001)
	require.True(t, found)
	require.Equal(t, "Z0", stand.Name)
}
