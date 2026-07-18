package vatsim

import (
	"FlightStrips/internal/aman"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAMANBaselineInputUsesNormalizedFactsAndExistingTakeoffDetection(t *testing.T) {
	now := time.Date(2026, time.July, 18, 12, 0, 0, 0, time.UTC)
	eobt := now.Add(time.Hour)
	eet := 90 * time.Minute
	observation := aman.FlightObservation{
		Destination: "EKCH", TakeoffDetected: &now,
		PlannedTiming: &aman.PlannedTiming{EstimatedOffBlockTime: &eobt, EstimatedEnrouteTime: &eet},
		FlightPlan:    aman.FlightPlanFact{ObservedAt: &now},
	}

	input := AMANBaselineInput(now, "EKCH", observation, true, false)
	require.Equal(t, eobt, *input.Timing.EOBT)
	require.Equal(t, eet, *input.Timing.FiledEET)
	require.Equal(t, now, *input.Airborne.SensedAt)
	require.True(t, input.Airborne.PreviouslyObserved)
	require.Nil(t, input.Timing.APIEstimatedFlightTime, "VATSIM does not manufacture an API estimate")
	require.Nil(t, input.GreatCircle, "coordinates/route geometry are not part of this source adapter")
}
