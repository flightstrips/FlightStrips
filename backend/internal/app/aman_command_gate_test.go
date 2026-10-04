package app

import (
	"context"
	"testing"

	"FlightStrips/internal/aman"
	"github.com/stretchr/testify/require"
)

func TestAMANCommandGateUsesCurrentTechnicalAuthority(t *testing.T) {
	ready := aman.ComponentHealth{Status: aman.HealthReady}
	blocked := aman.EvaluateTechnicalHealth(
		aman.ModeAuthoritative,
		ready,
		aman.ComponentHealth{Status: aman.HealthUnavailable, Reason: "navigation_refresh_failed"},
		ready, ready, ready, ready,
	)
	gate := &amanCommandGate{health: func(context.Context) aman.TechnicalHealth { return blocked }}
	err := gate.authorize(context.Background())
	var domain *aman.DomainError
	require.ErrorAs(t, err, &domain)
	require.Equal(t, aman.ErrorReadOnly, domain.Class)
	require.ErrorContains(t, err, "navigation:navigation_refresh_failed")

	allowed := aman.EvaluateTechnicalHealth(aman.ModeAuthoritative, ready, ready, ready, ready, ready, ready)
	gate.health = func(context.Context) aman.TechnicalHealth { return allowed }
	require.NoError(t, gate.authorize(context.Background()))

	allowed = aman.EvaluateTechnicalHealth(aman.ModeAuthoritative, ready, ready,
		aman.ComponentHealth{Status: aman.HealthUnavailable, Reason: "weather_refresh_failed"}, ready, ready, ready)
	require.Equal(t, aman.HealthReady, allowed.Status)
	require.Equal(t, aman.HealthUnavailable, allowed.Weather.Status, "weather failure remains visible on its component")
	require.NoError(t, gate.authorize(context.Background()), "weather fallback must not disable controller commands")
}
