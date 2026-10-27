package operational

import (
	"context"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"github.com/stretchr/testify/require"
)

func TestOneFlightPredictionErrorDoesNotDegradeSharedPredictor(t *testing.T) {
	now := time.Date(2026, 10, 26, 18, 4, 0, 0, time.UTC)
	service, state, observation, _ := arrivalChangeFixture(now)
	service.deps.Now = func() time.Time { return now }
	ready := componentHealth(aman.HealthReady, "", now)
	service.health = serviceHealth{vatsim: ready, euroScope: ready, navigation: ready, weather: ready, repository: ready, predictor: ready, replay: ready}
	observation.WakeCategory = stringPointer("M")
	flight, err := service.reconcileFlight(context.Background(), state, newFlight(observation, now), observation, now)
	require.NoError(t, err)
	require.NotNil(t, flight.Prediction)

	// A future-dated report passes source validation but the performance model
	// correctly refuses to evaluate weather before the observation. This is a
	// flight input error, not evidence that the shared predictor is unavailable.
	bad := observation
	bad.Callsign = "BAD123"
	position := *observation.Surveillance
	future := now.Add(time.Second)
	position.ObservedAt = &future
	bad.Surveillance = &position
	require.NoError(t, bad.Validate())
	_, err = service.reconcileFlight(context.Background(), state, newFlight(bad, now), bad, now)
	require.ErrorContains(t, err, "performance/wind predictor input")
	health := service.TechnicalHealth(context.Background())
	require.Equal(t, aman.HealthReady, health.Predictor.Status)
	require.True(t, health.Ready)
	require.True(t, health.AuthorityAllowed)
	require.Empty(t, health.BlockedReasons)
}
