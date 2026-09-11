package operational

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/sequence"
	"FlightStrips/internal/aman/terminal"
	"github.com/stretchr/testify/require"
)

func TestDesequenceAndResumeAreAuditedDurableAndEarliestLegal(t *testing.T) {
	now := time.Date(2026, time.September, 12, 12, 0, 0, 0, time.UTC)
	group := aman.RunwayGroupID("north")
	target := dispositionFlight("DSEQ", group, now, now.Add(-3*time.Minute), 1)
	target.Lifecycle = &aman.LifecycleState{EnteredAt: now.Add(-time.Hour), Reason: aman.LifecycleReasonStableHorizon, LastEventID: "stable", LastEventFingerprint: "test", LastEventAt: now.Add(-time.Minute)}
	leader := dispositionFlight("LEAD", group, now.Add(6*time.Minute), now.Add(6*time.Minute), 2)
	leader.FreezeReason = aman.FreezeManual
	leader.FrozenAt, leader.FrozenOperationalTETA = timePointer(now), timePointer(now.Add(6*time.Minute))
	leader.FrozenSlot = retargetSlot(leader.Slot, group)
	repository := dispositionRepository(now, group, target, leader)
	actions := dispositionActions(t, repository, now)
	auth := aman.CommandContext{Airport: "EKCH", Actor: "1234567", Role: "EKCH_FMH", ReceivedAt: now}
	desequence := aman.DesequenceFlightCommand{Metadata: aman.CommandMetadata{CommandID: "dseq-1", ExpectedRevision: 5}, FlightID: target.ID}

	accepted, err := actions.DesequenceFlight(context.Background(), auth, desequence)
	require.NoError(t, err)
	require.True(t, accepted.Changed)
	require.Equal(t, aman.SequenceDispositionDesequenced, repository.state.Flights[0].SequenceDisposition)
	require.Equal(t, target.Slot.Time, repository.state.Flights[0].Slot.Time, "DSEQ must retain its former slot as non-capacity history")
	require.Equal(t, target.Slot.Sequence, repository.state.Flights[0].Slot.Sequence)
	require.Equal(t, target.Lifecycle, repository.state.Flights[0].Lifecycle)
	require.Len(t, repository.state.Flights, 2)

	restarted := dispositionActions(t, repository, now.Add(time.Second))
	retry, err := restarted.DesequenceFlight(context.Background(), auth, desequence)
	require.NoError(t, err)
	require.True(t, retry.Duplicate)
	require.Equal(t, accepted.Outcome, retry.Outcome)
	require.Len(t, repository.commits, 1)

	_, err = restarted.ResumeFlight(context.Background(), auth, aman.ResumeFlightCommand{
		Metadata: aman.CommandMetadata{CommandID: "stale-resume", ExpectedRevision: 5}, FlightID: target.ID,
	})
	requireDomainErrorClass(t, err, aman.ErrorRevisionConflict)

	auth.ReceivedAt = now.Add(2 * time.Second)
	resumed, err := restarted.ResumeFlight(context.Background(), auth, aman.ResumeFlightCommand{
		Metadata: aman.CommandMetadata{CommandID: "resume-1", ExpectedRevision: 6}, FlightID: target.ID,
	})
	require.NoError(t, err)
	require.True(t, resumed.Changed)
	require.Equal(t, aman.SequenceDispositionActive, repository.state.Flights[0].SequenceDisposition)
	require.Equal(t, now.Add(12*time.Minute), repository.state.Flights[0].Slot.Time, "resume must skip the GAP and satisfy wake/STAR spacing from the protected slot")
	require.NotEqual(t, target.Slot.Time, repository.state.Flights[0].Slot.Time, "former slot must not be reserved")
	require.Len(t, repository.state.Flights, 2)

	var audit map[string]any
	require.NoError(t, json.Unmarshal(repository.commits[1].AuditRecords[0].Payload, &audit))
	require.Equal(t, "desequenced", audit["before_disposition"])
	require.Equal(t, "active", audit["after_disposition"])
	require.NotNil(t, audit["before_slot"])
	require.NotNil(t, audit["after_slot"])
	require.Equal(t, auth.Actor, audit["actor"])
}

func TestResumeNoCapacityRollsBackAtomically(t *testing.T) {
	now := time.Date(2026, time.September, 12, 12, 0, 0, 0, time.UTC)
	group := aman.RunwayGroupID("north")
	flight := dispositionFlight("DSEQ", group, now, now.Add(-3*time.Minute), 1)
	flight.SequenceDisposition = aman.SequenceDispositionDesequenced
	repository := dispositionRepository(now, group, flight)
	repository.state.RunwayGroups[0].RateSchedule = nil
	repository.state.RunwayGroups[0].ActiveRatePerHour = 0
	before := repository.state
	actions := dispositionActions(t, repository, now)

	_, err := actions.ResumeFlight(context.Background(), aman.CommandContext{
		Airport: "EKCH", Actor: "1234567", Role: "EKCH_FMH", ReceivedAt: now,
	}, aman.ResumeFlightCommand{Metadata: aman.CommandMetadata{CommandID: "no-capacity", ExpectedRevision: 5}, FlightID: flight.ID})
	requireDomainErrorClass(t, err, aman.ErrorInvalidTransition)
	require.Equal(t, before, repository.state)
	require.Empty(t, repository.commits)
}

func TestRemoveFlightUsesLifecycleAndPersistsIdempotentAudit(t *testing.T) {
	now := time.Date(2026, time.September, 12, 12, 0, 0, 0, time.UTC)
	group := aman.RunwayGroupID("north")
	flight := dispositionFlight("REMOVE", group, now.Add(time.Minute), now.Add(3*time.Minute), 1)
	flight.SequenceDisposition = aman.SequenceDispositionDesequenced
	flight.Lifecycle = &aman.LifecycleState{EnteredAt: now.Add(-time.Hour), Reason: aman.LifecycleReasonStableHorizon, LastEventID: "stable", LastEventFingerprint: "test", LastEventAt: now.Add(-time.Minute)}
	repository := dispositionRepository(now, group, flight)
	actions := dispositionActions(t, repository, now)
	auth := aman.CommandContext{Airport: "EKCH", Actor: "1234567", Role: "EKCH_FMH", ReceivedAt: now}
	command := aman.RemoveFlightCommand{Metadata: aman.CommandMetadata{CommandID: "remove-1", ExpectedRevision: 5}, FlightID: flight.ID}

	removed, err := actions.RemoveFlight(context.Background(), auth, command)
	require.NoError(t, err)
	require.True(t, removed.Changed)
	require.Equal(t, aman.StateRemoved, repository.state.Flights[0].State)
	require.Equal(t, aman.LifecycleReasonManualRemoval, repository.state.Flights[0].Lifecycle.Reason)
	require.Nil(t, repository.state.Flights[0].Slot)
	require.Equal(t, aman.SequenceDispositionDesequenced, repository.state.Flights[0].SequenceDisposition, "removal must remain orthogonal to disposition")

	var audit map[string]any
	require.NoError(t, json.Unmarshal(repository.commits[0].AuditRecords[0].Payload, &audit))
	require.Equal(t, "stable_horizon", audit["before_removal_reason"])
	require.Equal(t, "manual_removal", audit["after_removal_reason"])
	require.Equal(t, "stable", audit["before_state"])
	require.Equal(t, "removed", audit["after_state"])
	require.NotNil(t, audit["before_slot"])
	require.Nil(t, audit["after_slot"])

	retry, err := dispositionActions(t, repository, now.Add(time.Second)).RemoveFlight(context.Background(), auth, command)
	require.NoError(t, err)
	require.True(t, retry.Duplicate)
	require.Len(t, repository.commits, 1)
}

func TestFlightDispositionCommandsRejectSpoofablePayloadsAndNonFMP(t *testing.T) {
	for _, command := range []reflect.Type{
		reflect.TypeFor[aman.DesequenceFlightCommand](), reflect.TypeFor[aman.ResumeFlightCommand](), reflect.TypeFor[aman.RemoveFlightCommand](),
	} {
		for _, forbidden := range []string{"Airport", "Actor", "Role", "ReceivedAt", "AuditIdentity"} {
			_, present := command.FieldByName(forbidden)
			require.Falsef(t, present, "%s must not accept server-owned %s", command.Name(), forbidden)
		}
	}
	now := time.Date(2026, time.September, 12, 12, 0, 0, 0, time.UTC)
	group := aman.RunwayGroupID("north")
	unauthorized := aman.CommandContext{Airport: "EKCH", Actor: "7654321", Role: "EKCH_TWR", ReceivedAt: now}
	for name, invoke := range map[string]func(*sequence.ActionService) error{
		"desequence": func(actions *sequence.ActionService) error {
			_, err := actions.DesequenceFlight(context.Background(), unauthorized, aman.DesequenceFlightCommand{Metadata: aman.CommandMetadata{CommandID: "unauthorized-dseq", ExpectedRevision: 5}, FlightID: "DSEQ"})
			return err
		},
		"resume": func(actions *sequence.ActionService) error {
			_, err := actions.ResumeFlight(context.Background(), unauthorized, aman.ResumeFlightCommand{Metadata: aman.CommandMetadata{CommandID: "unauthorized-resume", ExpectedRevision: 5}, FlightID: "DSEQ"})
			return err
		},
		"remove": func(actions *sequence.ActionService) error {
			_, err := actions.RemoveFlight(context.Background(), unauthorized, aman.RemoveFlightCommand{Metadata: aman.CommandMetadata{CommandID: "unauthorized-remove", ExpectedRevision: 5}, FlightID: "DSEQ"})
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			repository := dispositionRepository(now, group, dispositionFlight("DSEQ", group, now, now, 1))
			requireDomainErrorClass(t, invoke(dispositionActions(t, repository, now)), aman.ErrorUnauthorized)
			require.Empty(t, repository.commits)
		})
	}
}

func dispositionFlight(id string, group aman.RunwayGroupID, teta, slotAt time.Time, order int) aman.AMANFlight {
	star := "MONAK"
	return aman.AMANFlight{
		ID: aman.FlightID(id), VATSIMCID: "CID-" + id, CurrentCallsign: id,
		State: aman.StateStable, DataStatus: aman.DataFresh, FreezeReason: aman.FreezeNone,
		Prediction: &aman.Prediction{
			RawTETA: teta, OperationalTETA: teta, OperationalReason: aman.OperationalReasonPredicted,
			GeneratedAt: teta.Add(-time.Minute), InputObservedAt: teta.Add(-time.Minute), Confidence: aman.ConfidenceHigh, Publishable: true,
			DatasetVersion: "test", GeometryDigest: "test", ModelVersion: "test", ConfigVersion: "test", Sources: []string{},
		},
		SelectedRunwayGroup: &group, SelectedFeeder: &star,
		Slot: &aman.Slot{Time: slotAt, RunwayGroupID: group, Sequence: order, Revision: 5, Reason: "prior"}, Order: &order,
	}
}

func dispositionRepository(now time.Time, group aman.RunwayGroupID, flights ...aman.AMANFlight) *memoryRepository {
	spacing := &aman.SameSTARSpacingPolicy{Enabled: true, ActivationRatePerHour: 20, MinimumEmptySlots: 1}
	return &memoryRepository{has: true, state: aman.AirportState{
		Airport: "EKCH", Revision: 5, GeneratedAt: now.Add(-time.Minute), PolicyVersion: "dseq-v1", Mode: aman.ModeAuthoritative,
		ActiveRunwayGroups: []aman.RunwayGroupID{group}, Flights: flights,
		RunwayGroups: []aman.RunwayGroupPolicy{{ID: group, Selected: true, ActiveRatePerHour: 20, SameSTARSpacing: spacing,
			RateSchedule: []aman.RunwayGroupRatePoint{{EffectiveAt: now, ArrivalsPerHour: 20}},
			Gaps:         []aman.RunwayGap{{ID: "gap", Start: now, End: now.Add(6 * time.Minute), Label: "stop", CreatedAt: now.Add(-time.Hour), CreatedBy: "fmp"}},
		}},
	}}
}

func dispositionActions(t *testing.T, repository *memoryRepository, recordedAt time.Time) *sequence.ActionService {
	t.Helper()
	coordinator, err := sequence.NewCoordinator(sequence.CoordinatorDependencies{
		States: repository, Outcomes: repository, Committer: repository, Publisher: &recordingPublisher{}, Now: func() time.Time { return recordedAt },
	})
	require.NoError(t, err)
	service := &Service{deps: Dependencies{
		FMPRoles: []string{"EKCH_FMH"}, Terminal: terminal.Configuration{Airport: "EKCH", ConfigVersion: "test", RunwayGroups: []terminal.RunwayGroup{{ID: "north"}}},
	}}
	actions, err := sequence.NewActionService(coordinator, service)
	require.NoError(t, err)
	return actions
}
