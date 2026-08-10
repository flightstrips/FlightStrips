package services

import (
	"FlightStrips/internal/sat"
	"FlightStrips/internal/standdiagnostics"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestStandAllocationRecordsRejectedRequest(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.July, 17, 20, 0, 0, 0, time.UTC)
	failures := standdiagnostics.NewAllocationFailureLog(10)
	service := &StandAllocationService{now: func() time.Time { return now }, failures: failures}

	_, err := service.Allocate(context.Background(), StandAllocationRequest{
		SessionID: 7,
		Callsign:  "sas123",
		Airport:   "ekch",
		FlightFacts: sat.FlightCompatibilityFacts{
			EngineType: sat.EngineJet,
			WTC:        "M",
		},
		AssignmentFacts: sat.AssignmentFlightFacts{AircraftType: "A320", BorderStatus: sat.BorderStatusSchengen},
	})
	require.Error(t, err)

	recorded := failures.List()
	require.Len(t, recorded, 1)
	require.Equal(t, "SAS123", recorded[0].Callsign)
	require.Equal(t, "EKCH", recorded[0].Airport)
	require.Equal(t, "invalid_request", recorded[0].Outcome)
	require.Equal(t, "A320", recorded[0].AircraftType)
	require.Equal(t, now, recorded[0].OccurredAt)
}

func TestStandActionRecordsPreAllocationRejection(t *testing.T) {
	t.Parallel()

	failures := standdiagnostics.NewAllocationFailureLog(10)
	actions := &StandActionService{
		allocations: &StandAllocationService{now: time.Now, failures: failures},
	}

	_, err := actions.AssignManually(context.Background(), 7, "EKCH", "", "sas123", "A12", 0)
	require.ErrorIs(t, err, ErrStandActionUnauthorized)

	recorded := failures.List()
	require.Len(t, recorded, 1)
	require.Equal(t, "MANUAL_ASSIGNMENT", recorded[0].Command)
	require.Equal(t, "unauthorized", recorded[0].Outcome)
	require.Equal(t, "A12", recorded[0].AttemptedStand)
}

func TestAutomaticTerminalFailuresAreSuppressedUntilFactsChange(t *testing.T) {
	service := &StandAllocationService{now: time.Now}
	request := StandAllocationRequest{
		SessionID: 7,
		Callsign:  "sas123",
		Airport:   "ekch",
		Direction: sat.AssignmentDirectionDeparture,
		FlightFacts: sat.FlightCompatibilityFacts{
			Origin: "EKCH", Destination: "EGLL", AircraftKnown: false,
			EngineType: sat.EngineJet, WTC: "UNKNOWN", BorderStatus: sat.BorderStatusNonSchengen,
		},
		AssignmentFacts: sat.AssignmentFlightFacts{
			AircraftType: "MD82", BorderStatus: sat.BorderStatusNonSchengen,
			Direction: sat.AssignmentDirectionDeparture,
		},
	}

	for attempt := 0; attempt < automaticTerminalFailureThreshold; attempt++ {
		require.False(t, service.automaticAllocationSuppressed(request))
		service.noteAutomaticTerminalFailure(request, ErrNoCompatibleStand)
	}
	require.True(t, service.automaticAllocationSuppressed(request))

	request.FlightFacts.AircraftKnown = true
	request.FlightFacts.Aircraft.Type = "MD82"
	request.FlightFacts.WTC = "M"
	require.False(t, service.automaticAllocationSuppressed(request), "new aircraft facts must allow a fresh allocation attempt")
}

func TestAutomaticNoAvailableFailureUsesBoundedRetrySuppression(t *testing.T) {
	now := time.Date(2026, 8, 10, 18, 0, 0, 0, time.UTC)
	service := &StandAllocationService{now: func() time.Time { return now }}
	request := StandAllocationRequest{SessionID: 7, Callsign: "sas123", Airport: "ekch", Direction: sat.AssignmentDirectionDeparture}

	service.noteAutomaticTerminalFailure(request, ErrNoAvailableStand)
	require.True(t, service.automaticAllocationSuppressed(request))
	now = now.Add(automaticAvailabilityRetryBase)
	require.False(t, service.automaticAllocationSuppressed(request), "availability is retried after the cooldown")

	service.noteAutomaticTerminalFailure(request, ErrNoAvailableStand)
	now = now.Add(automaticAvailabilityRetryBase)
	require.True(t, service.automaticAllocationSuppressed(request), "a repeated shortage uses a longer cooldown")
	now = now.Add(automaticAvailabilityRetryBase)
	require.False(t, service.automaticAllocationSuppressed(request), "the second cooldown expires after twice the base delay")

	require.Equal(t, automaticAvailabilityRetryMax, automaticAvailabilityRetryDelay(100), "backoff is capped")
	require.True(t, isTerminalAutomaticStandShortage(ErrNoAvailableStand))
	require.True(t, isTerminalAutomaticStandShortage(ErrNoCompatibleStand))
}
