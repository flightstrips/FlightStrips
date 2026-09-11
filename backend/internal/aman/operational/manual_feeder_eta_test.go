package operational

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/sequence"
	"github.com/stretchr/testify/require"
)

func TestManualFeederETAOverridesDerivedRecalculationsAndResetRestoresLatest(t *testing.T) {
	now := time.Date(2026, time.September, 11, 20, 0, 0, 0, time.UTC)
	routeETA, manualETA := now.Add(18*time.Minute), now.Add(15*time.Minute)
	service, state := manualFeederETAState(now, &aman.FeederETAState{ETA: &routeETA, Source: aman.FeederETASourceRoute})

	set, err := service.SetManualFeederETA(aman.CommandContext{ReceivedAt: now}, aman.SetManualFeederETACommand{
		Metadata: aman.CommandMetadata{CommandID: "manual-feeder-1"}, FlightID: "flight-1", FeederETA: manualETA,
	})
	require.NoError(t, err)
	changed, err := set(state)
	require.NoError(t, err)
	flight := changed.State.Flights[0]
	require.Equal(t, aman.FeederETASourceManual, flight.FeederETA.Source)
	require.Equal(t, manualETA, *flight.FeederETA.ETA)

	routeRecalculation := now.Add(17 * time.Minute)
	applyDerivedFeederETA(&flight, &aman.FeederETAState{ETA: &routeRecalculation, Source: aman.FeederETASourceRoute})
	runwayRecalculation := now.Add(22 * time.Minute)
	applyDerivedFeederETA(&flight, &aman.FeederETAState{ETA: &runwayRecalculation, Source: aman.FeederETASourceRoute})
	require.Equal(t, manualETA, *flight.FeederETA.ETA, "route and runway recalculation must not replace the override")
	require.Equal(t, runwayRecalculation, *flight.DerivedFeederETA.ETA)

	changed.State.Flights[0] = flight
	reset, err := service.ResetManualFeederETA(aman.CommandContext{ReceivedAt: now.Add(time.Minute)}, aman.ResetManualFeederETACommand{
		Metadata: aman.CommandMetadata{CommandID: "manual-feeder-reset-1"}, FlightID: "flight-1",
	})
	require.NoError(t, err)
	resetResult, err := reset(changed.State)
	require.NoError(t, err)
	require.Equal(t, aman.FeederETASourceRoute, resetResult.State.Flights[0].FeederETA.Source)
	require.Equal(t, runwayRecalculation, *resetResult.State.Flights[0].FeederETA.ETA)
}

func TestManualFeederETAAcceptsPastOnlyAfterAuthoritativePassage(t *testing.T) {
	now := time.Date(2026, time.September, 11, 20, 0, 0, 0, time.UTC)
	past := now.Add(-time.Minute)
	service, ahead := manualFeederETAState(now, nil)
	command := aman.SetManualFeederETACommand{Metadata: aman.CommandMetadata{CommandID: "past-feeder"}, FlightID: "flight-1", FeederETA: past}

	set, err := service.SetManualFeederETA(aman.CommandContext{ReceivedAt: now}, command)
	require.NoError(t, err)
	_, err = set(ahead)
	var domain *aman.DomainError
	require.ErrorAs(t, err, &domain)
	require.Equal(t, aman.ErrorInvalidArgument, domain.Class)

	passed := &aman.FeederETAState{Source: aman.FeederETASourcePassed, Passed: true}
	service, state := manualFeederETAState(now, passed)
	set, err = service.SetManualFeederETA(aman.CommandContext{ReceivedAt: now}, command)
	require.NoError(t, err)
	accepted, err := set(state)
	require.NoError(t, err)
	require.Equal(t, aman.FeederETASourceManual, accepted.State.Flights[0].FeederETA.Source)
	require.Equal(t, passed, accepted.State.Flights[0].DerivedFeederETA)
}

func TestManualFeederETASuppressesHoldingDerivation(t *testing.T) {
	now := time.Date(2026, time.September, 11, 20, 0, 0, 0, time.UTC)
	service, state := holdingFeederETAServiceState(now, "TESPI", "TNO", "EKCH-TESPI-PRIMARY", "ARRIVAL-22L", int64Pointer(195))
	manual := now.Add(19 * time.Minute)
	state.Flights[0].FeederETA = &aman.FeederETAState{ETA: &manual, Source: aman.FeederETASourceManual}

	service.refreshHoldingPlans(&state)

	require.Equal(t, manual, *state.Flights[0].FeederETA.ETA)
	require.Equal(t, aman.FeederETASourceManual, state.Flights[0].FeederETA.Source)
	require.Equal(t, now.Add(23*time.Minute+15*time.Second), *state.Flights[0].DerivedFeederETA.ETA)
	require.Equal(t, aman.FeederETASourceHolding, state.Flights[0].DerivedFeederETA.Source)
}

func TestManualFeederETAPersistsAndReplaysDeterministically(t *testing.T) {
	now := time.Date(2026, time.September, 11, 20, 0, 0, 0, time.UTC)
	derived, manual := now.Add(18*time.Minute), now.Add(16*time.Minute)
	service, state := manualFeederETAState(now, &aman.FeederETAState{ETA: &derived, Source: aman.FeederETASourceRoute})
	set, err := service.SetManualFeederETA(aman.CommandContext{ReceivedAt: now}, aman.SetManualFeederETACommand{
		Metadata: aman.CommandMetadata{CommandID: "replay-manual"}, FlightID: "flight-1", FeederETA: manual,
	})
	require.NoError(t, err)
	result, err := set(state)
	require.NoError(t, err)

	encoded, err := json.Marshal(result.State)
	require.NoError(t, err)
	var restarted aman.AirportState
	require.NoError(t, json.Unmarshal(encoded, &restarted))
	require.NoError(t, restarted.Validate())
	require.Equal(t, result.State.Flights[0].FeederETA, restarted.Flights[0].FeederETA)
	require.Equal(t, result.State.Flights[0].DerivedFeederETA, restarted.Flights[0].DerivedFeederETA)

	nextDerived := now.Add(14 * time.Minute)
	for _, replayed := range []*aman.AMANFlight{&result.State.Flights[0], &restarted.Flights[0]} {
		applyDerivedFeederETA(replayed, &aman.FeederETAState{ETA: &nextDerived, Source: aman.FeederETASourceRoute})
	}
	require.Equal(t, result.State.Flights[0], restarted.Flights[0])
}

func TestManualFeederETACommandRetryUsesPersistedCommandIdentity(t *testing.T) {
	now := time.Date(2026, time.September, 11, 20, 0, 0, 0, time.UTC)
	derived, manual := now.Add(18*time.Minute), now.Add(16*time.Minute)
	service, state := manualFeederETAState(now, &aman.FeederETAState{ETA: &derived, Source: aman.FeederETASourceRoute})
	repository := &memoryRepository{state: state, has: true}
	publisher := &recordingPublisher{}
	command := aman.SetManualFeederETACommand{
		Metadata: aman.CommandMetadata{CommandID: "manual-feeder-retry", ExpectedRevision: state.Revision}, FlightID: "flight-1", FeederETA: manual,
	}
	coordinator, err := sequence.NewCoordinator(sequence.CoordinatorDependencies{
		States: repository, Outcomes: repository, Committer: repository, Publisher: publisher, Now: func() time.Time { return now },
	})
	require.NoError(t, err)
	actions, err := sequence.NewActionService(coordinator, service)
	require.NoError(t, err)
	auth := aman.CommandContext{Airport: state.Airport, Actor: "1234567", Role: "EKCH_FMH", ReceivedAt: now}
	first, err := actions.SetManualFeederETA(context.Background(), auth, command)
	require.NoError(t, err)
	require.True(t, first.Changed)
	require.False(t, first.Duplicate)

	restarted, err := sequence.NewCoordinator(sequence.CoordinatorDependencies{
		States: repository, Outcomes: repository, Committer: repository, Publisher: publisher, Now: func() time.Time { return now },
	})
	require.NoError(t, err)
	restartedActions, err := sequence.NewActionService(restarted, service)
	require.NoError(t, err)
	retry, err := restartedActions.SetManualFeederETA(context.Background(), auth, command)
	require.NoError(t, err)
	require.True(t, retry.Duplicate)
	require.False(t, retry.Changed)
	require.Len(t, repository.commits, 1)
	require.Equal(t, first.Outcome, retry.Outcome)
	require.JSONEq(t, `{"action":"set_manual_feeder_eta","actor":"1234567","airport":"EKCH","changed":true,"feeder_eta":"2026-09-11T20:16:00Z","flight_id":"flight-1","received_at":"2026-09-11T20:00:00Z","role":"EKCH_FMH"}`, string(repository.commits[0].AuditRecords[0].Payload))
}

func manualFeederETAState(now time.Time, derived *aman.FeederETAState) (*Service, aman.AirportState) {
	family, fix := "TESPI", "TNO"
	flight := aman.AMANFlight{
		ID: "flight-1", VATSIMCID: "1234567", CurrentCallsign: "SAS123", State: aman.StateAirborne, DataStatus: aman.DataFresh,
		SelectedFeeder: &family, SelectedSTARFamily: &family, SelectedFeederFix: &fix,
		FeederETA: cloneFeederETA(derived), DerivedFeederETA: cloneFeederETA(derived), FreezeReason: aman.FreezeNone, UpdatedAt: now,
	}
	return &Service{}, aman.AirportState{
		Airport: "EKCH", PolicyVersion: "test", Mode: aman.ModeShadow, GeneratedAt: now, Flights: []aman.AMANFlight{flight},
		RunwayGroups: []aman.RunwayGroupPolicy{{
			ID: "ARRIVAL-22", ActiveRatePerHour: 20, RateEffectiveAt: &now,
			RateSchedule: []aman.RunwayGroupRatePoint{{EffectiveAt: now, ArrivalsPerHour: 20}},
		}},
	}
}
