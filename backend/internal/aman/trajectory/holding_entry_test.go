package trajectory

import (
	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/navdata"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestHoldingEntryUsesOneNMCrossingAndRejectsIncoherentReports(t *testing.T) {
	at := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	next := at.Add(time.Minute)
	fix := navdata.Coordinate{LatitudeDeg: 55, LongitudeDeg: 12}
	previous := aman.SurveillanceFact{LatitudeDegrees: 55, LongitudeDegrees: 11.94, ObservedAt: &at}
	current := aman.SurveillanceFact{LatitudeDegrees: 55, LongitudeDegrees: 12.06, ObservedAt: &next}
	entered, source := HoldingEntry(&previous, current, fix)
	require.NotNil(t, entered)
	require.Equal(t, "fix_crossing", source)
	require.InDelta(t, at.Add(30*time.Second).UnixNano(), entered.UnixNano(), float64(time.Second))
	current.LatitudeDegrees = 55.04
	previous.LatitudeDegrees = 55.04
	entered, _ = HoldingEntry(&previous, current, fix)
	require.Nil(t, entered, "broad holding footprint is not crossing")
	previous.LatitudeDegrees = 55
	current.LatitudeDegrees = 55
	delayed := at.Add(3 * time.Minute)
	current.ObservedAt = &delayed
	entered, _ = HoldingEntry(&previous, current, fix)
	require.Nil(t, entered, "do not interpolate across a surveillance gap")
	current.LongitudeDegrees = 12.01
	entered, source = HoldingEntry(nil, current, fix)
	require.Equal(t, "near_fix", source)
	require.Equal(t, delayed, *entered)
}
