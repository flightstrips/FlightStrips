package euroscope

import (
	"FlightStrips/internal/config"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBoardDefinitionUsesSelectedAirportTransitionAltitude(t *testing.T) {
	data, err := os.ReadFile("../config/testdata/radis-ekbi.yaml")
	require.NoError(t, err)
	text := strings.Replace(string(data), "transition_altitude_ft: 5000", "transition_altitude_ft: 3000", 1)
	definitions, err := config.NewAirportRegistry(config.ProductionAirportCatalog(), []config.AirportInput{{Source: "synthetic-only", Reader: strings.NewReader(text)}})
	require.NoError(t, err)
	definition, ok := definitions.Lookup("EKBI")
	require.True(t, ok)
	client := &Client{definition: definition}
	require.Equal(t, int32(3000), client.boardDefinition().TransitionAltitudeFT)
}
