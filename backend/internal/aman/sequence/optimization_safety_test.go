package sequence

import (
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"github.com/stretchr/testify/require"
)

func TestCorrectingAnInversionCannotPayForAnUnprovenCrossing(t *testing.T) {
	at := time.Date(2026, time.November, 13, 18, 0, 0, 0, time.UTC)
	later := at.Add(time.Minute)
	back := allocatedEntry{flight: preparedFlight{Flight: Flight{Callsign: "BACK", SelectedSTARFamily: "TESPI",
		STARProgress: &STARProgress{Fix: "ROSBI", ETA: &later, TerminalDigest: "terminal"}}}}
	front := allocatedEntry{flight: preparedFlight{Flight: Flight{Callsign: "FRONT", SelectedSTARFamily: "TESPI",
		STARProgress: &STARProgress{Fix: "ROSBI", ETA: &at, TerminalDigest: "terminal"}}}}
	unknown := allocatedEntry{flight: preparedFlight{Flight: Flight{Callsign: aman.Callsign("UNKNOWN"), SelectedSTARFamily: "TESPI"}}}
	before := []allocatedEntry{back, front, unknown}
	after := []allocatedEntry{front, unknown, back}
	require.Less(t, starInversions(after), starInversions(before), "aggregate inversion scoring misses the unknown pair")
	require.False(t, retainsSTAROrder(before, after))
	require.True(t, retainsSTAROrder(before, []allocatedEntry{front, back, unknown}))
}
