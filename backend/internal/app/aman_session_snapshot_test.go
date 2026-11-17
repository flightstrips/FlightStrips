package app

import (
	"context"
	"testing"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/models"
	"FlightStrips/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestAMANPublicationReceiptIsBoundToSessionAndAirport(t *testing.T) {
	valid := true
	ctx := aman.WithSessionSnapshot(aman.WithSession(t.Context(), 1), func() (aman.SessionSnapshot, bool) { return aman.SessionSnapshot{ID: 1, Airport: "EKCH"}, valid })
	reads := 0
	sessions := &testutil.MockSessionRepository{GetByIDFn: func(_ context.Context, id int32) (*models.Session, error) {
		reads++
		return &models.Session{ID: id, Airport: "EKCH"}, nil
	}}
	scope := amanAirportScope(Config{AMAN: aman.RuntimeConfig{EnabledAirports: []string{"EKCH", "ENGM"}}}, sessions)
	require.NoError(t, scope(ctx, "EKCH"))
	require.Zero(t, reads)
	require.ErrorContains(t, scope(ctx, "ENGM"), "does not match session")
	require.Zero(t, reads)
	require.NoError(t, scope(aman.WithSession(ctx, 2), "EKCH"))
	require.Equal(t, 1, reads, "another session must perform its own database check")
	valid = false
	require.NoError(t, scope(ctx, "EKCH"))
	require.Equal(t, 2, reads, "an invalidated receipt must perform a fresh database check")
}

func TestAMANRunwaySelectionReusesValidatedReceiptWithoutListingSessions(t *testing.T) {
	for _, runways := range [][]string{{"22l", "22L"}, {"22L", "04L"}, {}} {
		ctx := aman.WithSessionSnapshot(aman.WithSession(t.Context(), 1), func() (aman.SessionSnapshot, bool) {
			return aman.SessionSnapshot{ID: 1, Airport: "EKCH", ArrivalRunways: runways}, true
		})
		// A nil session dependency proves there is no database read on this path.
		source := sessionArrivalRunwaySource{}
		runway, err := source.ActiveArrivalRunway(ctx, "EKCH")
		if len(runways) == 0 {
			require.ErrorContains(t, err, "no active arrival runway")
		} else if runways[0] == "22l" {
			require.NoError(t, err)
			require.Equal(t, "22L", runway)
		} else {
			require.ErrorContains(t, err, "multiple active arrival runways")
		}
	}
}
