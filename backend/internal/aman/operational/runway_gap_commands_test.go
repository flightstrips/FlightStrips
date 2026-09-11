package operational

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/sequence"
	"github.com/stretchr/testify/require"
)

func TestRunwayGapCommandsAuthorizePersistMergeRetryAndRemove(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, time.September, 12, 12, 0, 0, 0, time.UTC)
	existing := aman.RunwayGap{
		ID: "old-gap", Start: now.Add(5 * time.Minute), End: now.Add(8 * time.Minute),
		Label: "inspection", CreatedAt: now.Add(-time.Hour), CreatedBy: "old-controller",
	}
	repository := &memoryRepository{has: true, state: aman.AirportState{
		Airport: "EKCH", Revision: 7, GeneratedAt: now.Add(-time.Minute), PolicyVersion: "gap-v1", Mode: aman.ModeAuthoritative,
		RunwayGroups: []aman.RunwayGroupPolicy{{ID: "north", ActiveRatePerHour: 20, Gaps: []aman.RunwayGap{existing}}},
	}}
	publisher := &recordingPublisher{}
	actions := runwayGapActions(t, repository, publisher, now.Add(time.Second))
	auth := aman.CommandContext{Airport: "EKCH", Actor: "1234567", Role: "EKCH_FMH", ReceivedAt: now}
	slots := uint32(2)
	create := aman.CreateRunwayGapCommand{
		Metadata: aman.CommandMetadata{CommandID: "create-gap", ExpectedRevision: 7}, RunwayGroupID: "north",
		Interval: aman.RunwayGapIntervalInput{Start: now, SlotCount: &slots}, Label: "approach stop",
	}

	created, err := actions.CreateRunwayGap(ctx, auth, create)
	require.NoError(t, err)
	require.True(t, created.Changed)
	require.Equal(t, aman.SequenceRevision(8), created.CurrentRevision)
	require.Len(t, repository.state.RunwayGroups[0].Gaps, 1)
	gap := repository.state.RunwayGroups[0].Gaps[0]
	require.Equal(t, aman.RunwayGapID("create-gap"), gap.ID)
	require.Equal(t, now, gap.Start)
	require.Equal(t, now.Add(8*time.Minute), gap.End)
	require.Equal(t, auth.Actor, gap.CreatedBy)
	require.Equal(t, auth.ReceivedAt, gap.CreatedAt)
	require.Len(t, repository.commits[0].AuditRecords, 1)
	require.Equal(t, "aman.create_runway_gap", repository.commits[0].AuditRecords[0].Category)
	var createAudit map[string]any
	require.NoError(t, json.Unmarshal(repository.commits[0].AuditRecords[0].Payload, &createAudit))
	require.Equal(t, []any{"old-gap"}, createAudit["replaced_ids"])
	require.NotNil(t, createAudit["before_interval"])
	require.NotNil(t, createAudit["after_interval"])

	restarted := runwayGapActions(t, repository, publisher, now.Add(2*time.Second))
	retried, err := restarted.CreateRunwayGap(ctx, auth, create)
	require.NoError(t, err)
	require.True(t, retried.Duplicate)
	require.False(t, retried.Changed)
	require.Equal(t, created.Outcome, retried.Outcome)
	require.Len(t, repository.commits, 1, "restart retry must not create another persisted GAP or audit")

	_, err = restarted.RemoveRunwayGap(ctx, auth, aman.RemoveRunwayGapCommand{
		Metadata: aman.CommandMetadata{CommandID: "stale-remove", ExpectedRevision: 7}, RunwayGroupID: "north", GapID: gap.ID,
	})
	requireDomainErrorClass(t, err, aman.ErrorRevisionConflict)
	require.Len(t, repository.state.RunwayGroups[0].Gaps, 1)

	removed, err := restarted.RemoveRunwayGap(ctx, auth, aman.RemoveRunwayGapCommand{
		Metadata: aman.CommandMetadata{CommandID: "remove-gap", ExpectedRevision: 8}, RunwayGroupID: "north", GapID: gap.ID,
	})
	require.NoError(t, err)
	require.True(t, removed.Changed)
	require.Equal(t, aman.SequenceRevision(9), removed.CurrentRevision)
	require.Empty(t, repository.state.RunwayGroups[0].Gaps)
	require.Len(t, repository.commits[1].AuditRecords, 1)
	var removeAudit map[string]any
	require.NoError(t, json.Unmarshal(repository.commits[1].AuditRecords[0].Payload, &removeAudit))
	require.NotNil(t, removeAudit["before_interval"])
	require.Nil(t, removeAudit["after_interval"])
	require.Equal(t, []any{"create-gap"}, removeAudit["removed_ids"])
}

func TestRunwayGapCommandsRejectNonFMPBeforeStateMutation(t *testing.T) {
	now := time.Date(2026, time.September, 12, 12, 0, 0, 0, time.UTC)
	repository := &memoryRepository{has: true, state: aman.AirportState{
		Airport: "EKCH", Revision: 1, GeneratedAt: now, PolicyVersion: "gap-v1", Mode: aman.ModeAuthoritative,
		RunwayGroups: []aman.RunwayGroupPolicy{{ID: "north", ActiveRatePerHour: 20}},
	}}
	actions := runwayGapActions(t, repository, &recordingPublisher{}, now)
	end := now.Add(time.Minute)
	_, err := actions.CreateRunwayGap(context.Background(), aman.CommandContext{
		Airport: "EKCH", Actor: "7654321", Role: "EKCH_TWR", ReceivedAt: now,
	}, aman.CreateRunwayGapCommand{
		Metadata: aman.CommandMetadata{CommandID: "spoof-attempt", ExpectedRevision: 1}, RunwayGroupID: "north",
		Interval: aman.RunwayGapIntervalInput{Start: now, End: &end}, Label: "approach stop",
	})
	requireDomainErrorClass(t, err, aman.ErrorUnauthorized)
	require.Empty(t, repository.commits)
}

func TestCreateRunwayGapAtomicallyDisplacesEveryProtectionClassAndAuditsReplay(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, time.September, 12, 12, 0, 0, 0, time.UTC)
	state := runwayGapDisplacementState(now)
	end := now.Add(5 * time.Minute)
	command := aman.CreateRunwayGapCommand{
		Metadata: aman.CommandMetadata{CommandID: "gap-displace", ExpectedRevision: state.Revision}, RunwayGroupID: "north",
		Interval: aman.RunwayGapIntervalInput{Start: now, End: &end}, Label: "approach stop",
	}
	auth := aman.CommandContext{Airport: "EKCH", Actor: "1234567", Role: "EKCH_FMH", ReceivedAt: now}

	firstRepository := &memoryRepository{has: true, state: cloneGapState(t, state)}
	first, err := runwayGapActions(t, firstRepository, &recordingPublisher{}, now.Add(time.Second)).CreateRunwayGap(ctx, auth, command)
	require.NoError(t, err)
	require.True(t, first.Changed)
	require.Equal(t, aman.SequenceRevision(8), first.CurrentRevision)
	require.Len(t, firstRepository.commits, 1)
	require.Len(t, firstRepository.commits[0].AuditRecords, 7, "the GAP and the complete displacement cascade share one commit")

	wantReasons := map[aman.FlightID]string{
		"UNPROTECTED": "none", "STABLE": "stable", "SUPERSTABLE": "superstable", "MANUAL": "manual", "TMA": "tma", "TRAILING": "superstable",
	}
	directlyAffected := map[aman.FlightID]bool{"UNPROTECTED": true, "STABLE": true, "SUPERSTABLE": true, "MANUAL": true, "TMA": true}
	seen := make(map[aman.FlightID]bool)
	for index, record := range firstRepository.commits[0].AuditRecords {
		require.Equal(t, aman.SequenceRevision(8), record.Revision)
		if index == 0 {
			require.Equal(t, "aman.create_runway_gap", record.Category)
			var gapAudit struct {
				DisplacedFlightIDs []aman.FlightID `json:"displaced_flight_ids"`
			}
			require.NoError(t, json.Unmarshal(record.Payload, &gapAudit))
			require.Equal(t, []aman.FlightID{"MANUAL", "STABLE", "SUPERSTABLE", "TMA", "TRAILING", "UNPROTECTED"}, gapAudit.DisplacedFlightIDs)
			continue
		}
		require.Equal(t, "aman.runway_gap_displacement", record.Category)
		var audit struct {
			FlightID                   aman.FlightID  `json:"flight_id"`
			OverriddenProtectionReason string         `json:"overridden_protection_reason"`
			PreviousOpportunity        gapOpportunity `json:"previous_opportunity"`
			NewOpportunity             gapOpportunity `json:"new_opportunity"`
		}
		require.NoError(t, json.Unmarshal(record.Payload, &audit))
		require.Equal(t, wantReasons[audit.FlightID], audit.OverriddenProtectionReason)
		if directlyAffected[audit.FlightID] {
			require.True(t, audit.PreviousOpportunity.Time.Before(end))
		}
		require.False(t, audit.NewOpportunity.Time.Before(end))
		require.True(t, audit.NewOpportunity.Time.After(audit.PreviousOpportunity.Time))
		seen[audit.FlightID] = true
	}
	require.Len(t, seen, len(wantReasons))
	for _, flight := range firstRepository.state.Flights {
		require.False(t, flight.Slot.Time.Before(end), flight.ID)
		require.Equal(t, aman.SequenceRevision(8), flight.Slot.Revision)
		if flight.FreezeReason != aman.FreezeNone {
			require.Equal(t, flight.Slot.Time, flight.FrozenSlot.Time, "freeze protection must reanchor after displacement")
			require.Equal(t, flight.FreezeReason, stateFlight(t, state, flight.ID).FreezeReason)
		}
	}
	for index, id := range []aman.FlightID{"UNPROTECTED", "STABLE", "SUPERSTABLE", "MANUAL", "TMA", "TRAILING"} {
		require.Equal(t, index+1, stateFlight(t, firstRepository.state, id).Slot.Sequence, "GAP insertion must preserve committed sequence order")
	}
	require.True(t, stateFlight(t, firstRepository.state, "TRAILING").Slot.Time.After(end), "trailing protected traffic must move behind the displaced block")

	retry, err := runwayGapActions(t, firstRepository, &recordingPublisher{}, now.Add(2*time.Second)).CreateRunwayGap(ctx, auth, command)
	require.NoError(t, err)
	require.True(t, retry.Duplicate)
	require.False(t, retry.Changed)
	require.Equal(t, first.Outcome, retry.Outcome)
	require.Len(t, firstRepository.commits, 1)

	secondRepository := &memoryRepository{has: true, state: cloneGapState(t, state)}
	second, err := runwayGapActions(t, secondRepository, &recordingPublisher{}, now.Add(time.Second)).CreateRunwayGap(ctx, auth, command)
	require.NoError(t, err)
	require.Equal(t, first.Outcome, second.Outcome)
	firstJSON, err := json.Marshal(firstRepository.commits[0])
	require.NoError(t, err)
	secondJSON, err := json.Marshal(secondRepository.commits[0])
	require.NoError(t, err)
	require.Equal(t, firstJSON, secondJSON, "equivalent replay must produce the same state, outcome, and audit")
}

func TestCreateRunwayGapImpossibleCapacityRollsBackStateRevisionAndAudit(t *testing.T) {
	now := time.Date(2026, time.September, 12, 12, 0, 0, 0, time.UTC)
	state := runwayGapDisplacementState(now)
	state.Flights = state.Flights[:1]
	state.RunwayGroups[0].RateEffectiveAt = nil
	repository := &memoryRepository{has: true, state: cloneGapState(t, state)}
	before := cloneGapState(t, repository.state)
	end := now.Add(10 * time.Minute)

	_, err := runwayGapActions(t, repository, &recordingPublisher{}, now.Add(time.Second)).CreateRunwayGap(context.Background(), aman.CommandContext{
		Airport: "EKCH", Actor: "1234567", Role: "EKCH_FMH", ReceivedAt: now,
	}, aman.CreateRunwayGapCommand{
		Metadata: aman.CommandMetadata{CommandID: "impossible-gap", ExpectedRevision: state.Revision}, RunwayGroupID: "north",
		Interval: aman.RunwayGapIntervalInput{Start: now, End: &end}, Label: "approach stop",
	})
	requireDomainErrorClass(t, err, aman.ErrorInvalidTransition)
	require.Equal(t, before, repository.state)
	require.Equal(t, aman.SequenceRevision(7), repository.state.Revision)
	require.Empty(t, repository.commits)
	require.Empty(t, repository.outcomes)
}

func TestRunwayGapPayloadHasNoAuditIdentityFields(t *testing.T) {
	typeOf := reflect.TypeFor[aman.CreateRunwayGapCommand]()
	for _, forbidden := range []string{"Airport", "Actor", "Role", "CreatedAt", "CreatedBy", "ReceivedAt"} {
		_, present := typeOf.FieldByName(forbidden)
		require.Falsef(t, present, "runway GAP payload must not accept server-owned %s", forbidden)
	}
}

func runwayGapDisplacementState(now time.Time) aman.AirportState {
	flights := []aman.AMANFlight{
		gapCommandFlight("UNPROTECTED", "north", now, 1, aman.StateUnstable, aman.FreezeNone),
		gapCommandFlight("STABLE", "north", now.Add(time.Minute), 2, aman.StateStable, aman.FreezeNone),
		gapCommandFlight("SUPERSTABLE", "north", now.Add(2*time.Minute), 3, aman.StateStable, aman.FreezeSuperstable),
		gapCommandFlight("MANUAL", "north", now.Add(3*time.Minute), 4, aman.StateStable, aman.FreezeManual),
		gapCommandFlight("TMA", "north", now.Add(4*time.Minute), 5, aman.StateStable, aman.FreezeTMA),
		gapCommandFlight("TRAILING", "north", now.Add(5*time.Minute), 6, aman.StateStable, aman.FreezeSuperstable),
	}
	return aman.AirportState{
		Airport: "EKCH", Revision: 7, GeneratedAt: now.Add(-time.Minute), PolicyVersion: "gap-v1", Mode: aman.ModeAuthoritative,
		Flights: flights, RunwayGroups: []aman.RunwayGroupPolicy{{ID: "north", ActiveRatePerHour: 60, RateEffectiveAt: &now}},
	}
}

func gapCommandFlight(id string, group aman.RunwayGroupID, at time.Time, sequenceNumber int, state aman.FlightState, freeze aman.FreezeReason) aman.AMANFlight {
	feeder := "MONAK"
	prediction := acceptedRawPrediction(at.Add(-time.Minute), at)
	prediction.OperationalTETA, prediction.OperationalReason = at, aman.OperationalReasonPredicted
	flight := aman.AMANFlight{
		ID: aman.FlightID(id), VATSIMCID: id, CurrentCallsign: id, State: state, DataStatus: aman.DataFresh,
		Prediction: &prediction, SelectedRunwayGroup: &group, SelectedFeeder: &feeder, FreezeReason: freeze, UpdatedAt: at,
	}
	flight.Slot = &aman.Slot{Time: at, RunwayGroupID: group, Sequence: sequenceNumber, Revision: 7, Reason: "rate_wtc"}
	if freeze != aman.FreezeNone {
		flight.FrozenAt, flight.FrozenOperationalTETA = &at, &at
		frozen := *flight.Slot
		flight.FrozenSlot = &frozen
	}
	return flight
}

func cloneGapState(t *testing.T, state aman.AirportState) aman.AirportState {
	t.Helper()
	encoded, err := json.Marshal(state)
	require.NoError(t, err)
	var clone aman.AirportState
	require.NoError(t, json.Unmarshal(encoded, &clone))
	return clone
}

func stateFlight(t *testing.T, state aman.AirportState, id aman.FlightID) aman.AMANFlight {
	t.Helper()
	for _, flight := range state.Flights {
		if flight.ID == id {
			return flight
		}
	}
	require.FailNow(t, "flight not found", id)
	return aman.AMANFlight{}
}

func runwayGapActions(t *testing.T, repository *memoryRepository, publisher *recordingPublisher, recordedAt time.Time) *sequence.ActionService {
	t.Helper()
	coordinator, err := sequence.NewCoordinator(sequence.CoordinatorDependencies{
		States: repository, Outcomes: repository, Committer: repository, Publisher: publisher, Now: func() time.Time { return recordedAt },
	})
	require.NoError(t, err)
	actions, err := sequence.NewActionService(coordinator, &Service{deps: Dependencies{FMPRoles: []string{"EKCH_FMH"}}})
	require.NoError(t, err)
	return actions
}

func requireDomainErrorClass(t *testing.T, err error, class aman.ErrorClass) {
	t.Helper()
	var domain *aman.DomainError
	require.ErrorAs(t, err, &domain)
	require.Equal(t, class, domain.Class)
}
