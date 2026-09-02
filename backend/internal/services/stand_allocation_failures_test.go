package services

import (
	"FlightStrips/internal/pdc/testdata"
	"FlightStrips/internal/sat"
	"FlightStrips/internal/standdiagnostics"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTierImprovementMissIsNotRecordedAsFailure(t *testing.T) {
	pool, queries := testdata.SetupTestDB(t)
	service, session, _ := standAllocationFixture(t, pool, queries, "", "")
	testdata.SeedTestStrip(t, queries, session, "SASUPGRADE")
	failures := standdiagnostics.NewAllocationFailureLog(10)
	service.failures = failures
	request := standAllocationRequest(session, "SASUPGRADE")
	request.Stage = StageAssigned
	request.ImproveTierBelow = 1

	_, err := service.Reallocate(context.Background(), request)
	require.ErrorIs(t, err, ErrNoTierImprovement)
	require.Empty(t, failures.List(), "an optional tier-upgrade miss must not appear as an allocation error")
}

func TestAllocationFailureSeverityFollowsLifecycleStage(t *testing.T) {
	failures := standdiagnostics.NewAllocationFailureLog(10)
	service := &StandAllocationService{now: time.Now, failures: failures}
	service.recordAllocationFailure(AutomaticStandAllocation, StandAllocationRequest{Stage: StageEstimated}, "no_policy_stand", ErrNoPolicyStand, 1)
	service.recordAllocationFailure(AutomaticStandAllocation, StandAllocationRequest{Stage: StageAssigned}, "no_policy_stand", ErrNoPolicyStand, 1)
	service.recordAllocationFailure(AutomaticStandAllocation, StandAllocationRequest{Stage: StageConfirmed}, "no_policy_stand", ErrNoPolicyStand, 1)

	recorded := failures.List()
	require.Len(t, recorded, 3)
	require.Equal(t, standdiagnostics.SeverityError, recorded[0].Severity)
	require.Equal(t, standdiagnostics.SeverityError, recorded[1].Severity)
	require.Equal(t, standdiagnostics.SeverityWarning, recorded[2].Severity)
}

func TestStandAllocationRecordsRejectedRequest(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.July, 17, 20, 0, 0, 0, time.UTC)
	eta := now.Add(20 * time.Minute)
	tobt := now.Add(5 * time.Minute)
	tsat := now.Add(10 * time.Minute)
	etaSource := "EAT"
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
		ETA:             &eta, ETASource: &etaSource, DepartureTOBT: &tobt, DepartureTSAT: &tsat, DepartureReady: true,
	})
	require.Error(t, err)

	recorded := failures.List()
	require.Len(t, recorded, 1)
	require.Equal(t, "SAS123", recorded[0].Callsign)
	require.Equal(t, "EKCH", recorded[0].Airport)
	require.Equal(t, "invalid_request", recorded[0].Outcome)
	require.Equal(t, "A320", recorded[0].AircraftType)
	require.Equal(t, now, recorded[0].OccurredAt)
	require.Equal(t, eta, *recorded[0].ETA)
	require.Equal(t, "EAT", recorded[0].ETASource)
	require.Equal(t, tobt, *recorded[0].DepartureTOBT)
	require.Equal(t, tsat, *recorded[0].DepartureTSAT)
	require.True(t, recorded[0].DepartureReady)
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

	require.False(t, service.automaticAllocationSuppressed(request))
	service.noteAutomaticTerminalFailure(request, "no_compatible_stand")
	require.True(t, service.automaticAllocationSuppressed(request))

	request.FlightFacts.AircraftKnown = true
	request.FlightFacts.Aircraft.Type = "MD82"
	request.FlightFacts.WTC = "M"
	require.False(t, service.automaticAllocationSuppressed(request), "new aircraft facts must allow a fresh allocation attempt")
}

func TestAutomaticAvailabilityFailureRetriesSilentlyUntilStateChanges(t *testing.T) {
	service := &StandAllocationService{now: time.Now}
	request := StandAllocationRequest{SessionID: 7, Callsign: "sas123", Airport: "ekch", Direction: sat.AssignmentDirectionArrival, Stage: StageEstimated}

	service.noteAutomaticTerminalFailure(request, "no_available_stand")
	skipAttempt, previousOutcome := service.automaticAllocationSuppression(request)
	require.False(t, skipAttempt, "availability must still be probed so a freed stand is claimed")
	require.Equal(t, "no_available_stand", previousOutcome, "the repeated outcome is suppressed from logs, metrics, and diagnostics")

	request.Stage = StageAssigned
	skipAttempt, previousOutcome = service.automaticAllocationSuppression(request)
	require.False(t, skipAttempt)
	require.Empty(t, previousOutcome, "stage escalation must produce one fresh operational diagnostic")

	require.True(t, isTerminalAutomaticStandShortage(ErrNoAvailableStand))
	require.True(t, isTerminalAutomaticStandShortage(ErrNoPolicyStand))
	require.True(t, isTerminalAutomaticStandShortage(ErrNoCompatibleStand))
	require.Equal(t, "no_policy_stand", standAllocationFailureOutcome(ErrNoPolicyStand))
	require.Equal(t, "no_tier_improvement", standAllocationFailureOutcome(ErrNoTierImprovement))
}

func TestObservedConflictSuppressionResetsAfterPhysicalStandChanges(t *testing.T) {
	service := &StandAllocationService{now: time.Now}
	observed := "A1"
	request := StandAllocationRequest{
		SessionID: 7, Callsign: "sas123", Airport: "ekch", Direction: sat.AssignmentDirectionDeparture,
		Stage: StageDepartureBlock, Stand: observed, ObservedStand: &observed,
	}

	service.noteAutomaticTerminalFailure(request, "manual_stand_unavailable")
	_, previousOutcome := service.automaticAllocationSuppression(request)
	require.Equal(t, "manual_stand_unavailable", previousOutcome, "an unchanged physical conflict is emitted only once")

	newObserved := "A2"
	request.Stand, request.ObservedStand = newObserved, &newObserved
	_, previousOutcome = service.automaticAllocationSuppression(request)
	require.Empty(t, previousOutcome, "a new observed stand must produce a fresh conflict event")
}

func TestExpectedAutomaticAllocationOutcomesDoNotAbortReconciliation(t *testing.T) {
	for _, err := range []error{
		ErrNoCompatibleStand,
		ErrNoAvailableStand,
		ErrNoPolicyStand,
		ErrAutomaticAllocationSuppressed,
		ErrNoTierImprovement,
	} {
		require.NoError(t, suppressAutomaticAllocationError(err))
	}

	wanted := errors.New("database unavailable")
	require.ErrorIs(t, suppressAutomaticAllocationError(wanted), wanted)
}
