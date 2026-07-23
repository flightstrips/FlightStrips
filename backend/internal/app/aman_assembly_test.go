package app

import (
	"testing"

	"FlightStrips/internal/aman/navdata"
	"FlightStrips/internal/aman/terminal"
	"github.com/stretchr/testify/require"
)

func TestValidateTerminalAirportCoverage(t *testing.T) {
	configuration := terminal.Configuration{Airport: navdata.AirportID("EKCH")}

	require.NoError(t, validateTerminalAirportCoverage(configuration, []string{"ekch"}))
	require.ErrorContains(t, validateTerminalAirportCoverage(configuration, []string{"EKCH", "EGLL"}), "requires exactly that enabled airport")
	require.ErrorContains(t, validateTerminalAirportCoverage(configuration, []string{"EGLL"}), "requires exactly that enabled airport")
}
