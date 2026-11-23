package operational

import (
	"context"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"github.com/stretchr/testify/require"
)

func TestPlaceFlightAtTimeKeepsRunwayChoiceFMPOnly(t *testing.T) {
	now := time.Date(2026, time.September, 12, 12, 0, 0, 0, time.UTC)
	controller := aman.CommandContext{Airport: "EKCH", Actor: "123", Role: "EKDK_D_CTR", ReceivedAt: now, ControllerSequencing: true}
	place := func(t *testing.T, editState func(*aman.AirportState)) (*memoryRepository, error) {
		t.Helper()
		state := manualPlacementState(now)
		editState(&state)
		repository := &memoryRepository{has: true, state: cloneGapState(t, state)}
		_, err := manualPlacementActions(t, repository, now).PlaceFlightAtTime(context.Background(), controller, aman.PlaceFlightAtTimeCommand{
			Metadata: aman.CommandMetadata{CommandID: "controller-place", ExpectedRevision: 7}, Callsign: "TARGET",
			RunwayGroupID: "north", SlotTime: now.Add(5 * time.Minute),
		})
		return repository, err
	}

	repository, err := place(t, func(s *aman.AirportState) { s.Flights[0].SelectedRunwayGroup = nil })
	requireDomainErrorClass(t, err, aman.ErrorUnauthorized)
	require.Empty(t, repository.commits)

	_, err = place(t, func(s *aman.AirportState) {
		north := aman.RunwayGroupID("north")
		s.Flights[0].SelectedRunwayGroup = &north
	})
	if err != nil {
		var domain *aman.DomainError
		require.ErrorAs(t, err, &domain)
		require.NotEqual(t, aman.ErrorUnauthorized, domain.Class, "placing on the flight's own runway is allowed")
	}
}
