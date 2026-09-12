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

func TestCapacityReservationCreateRetryRestartAndRemove(t *testing.T) {
	now := time.Date(2026, time.September, 12, 12, 0, 0, 0, time.UTC)
	group := aman.RunwayGroupID("north")
	anchor := gapCommandFlight("ANCHOR", group, now, 1, aman.StateStable, aman.FreezeNone)
	protected := gapCommandFlight("PROTECTED", group, now.Add(time.Minute), 2, aman.StateStable, aman.FreezeTMA)
	trailing := gapCommandFlight("TRAILING", group, now.Add(2*time.Minute), 3, aman.StateStable, aman.FreezeSuperstable)
	repository := &memoryRepository{has: true, state: aman.AirportState{
		Airport: "EKCH", Revision: 7, GeneratedAt: now, PolicyVersion: "reservation-v1", Mode: aman.ModeAuthoritative,
		Flights: []aman.AMANFlight{anchor, protected, trailing}, RunwayGroups: []aman.RunwayGroupPolicy{{
			ID: group, ActiveRatePerHour: 60, RateEffectiveAt: &now,
		}},
	}}
	auth := aman.CommandContext{Airport: "EKCH", Actor: "1234567", Role: "EKCH_FMH", ReceivedAt: now}
	command := aman.CreateCapacityReservationCommand{
		Metadata: aman.CommandMetadata{CommandID: "extra-1", ExpectedRevision: 7}, RunwayGroupID: group,
		AfterFlightID: anchor.ID, Reason: "medevac capacity",
	}
	actions := runwayGapActions(t, repository, &recordingPublisher{}, now.Add(time.Second))
	created, err := actions.CreateCapacityReservation(context.Background(), auth, command)
	require.NoError(t, err)
	require.Equal(t, aman.SequenceRevision(8), created.CurrentRevision)
	require.Len(t, repository.state.RunwayGroups[0].CapacityReservations, 1)
	reservation := repository.state.RunwayGroups[0].CapacityReservations[0]
	require.Equal(t, now.Add(time.Minute), reservation.Start)
	require.Equal(t, now.Add(2*time.Minute), reservation.End)
	require.Equal(t, aman.DefaultCapacityReservationLabel, reservation.Label)
	require.Len(t, repository.state.Flights, 3, "capacity must not create an AMAN flight identity")
	require.Equal(t, now.Add(3*time.Minute), stateFlight(t, repository.state, protected.ID).Slot.Time)
	require.Equal(t, aman.FreezeTMA, stateFlight(t, repository.state, protected.ID).FreezeReason)
	require.True(t, stateFlight(t, repository.state, trailing.ID).Slot.Time.After(trailing.Slot.Time))
	require.Equal(t, aman.FreezeSuperstable, stateFlight(t, repository.state, trailing.ID).FreezeReason)
	require.Len(t, repository.commits[0].AuditRecords, 3)

	var createAudit map[string]any
	require.NoError(t, json.Unmarshal(repository.commits[0].AuditRecords[0].Payload, &createAudit))
	require.Equal(t, "extra-1", createAudit["command_id"])
	require.Equal(t, "ANCHOR", createAudit["anchor_flight_id"])
	require.Equal(t, float64(60), createAudit["accepted_rate_per_hour"])
	require.Equal(t, auth.Actor, createAudit["creator"])

	for _, retryActions := range []*sequence.ActionService{actions, runwayGapActions(t, repository, &recordingPublisher{}, now.Add(2*time.Second))} {
		retry, retryErr := retryActions.CreateCapacityReservation(context.Background(), auth, command)
		require.NoError(t, retryErr)
		require.True(t, retry.Duplicate)
		require.Equal(t, created.Outcome, retry.Outcome)
	}
	require.Len(t, repository.commits, 1)

	_, err = actions.RemoveCapacityReservation(context.Background(), auth, aman.RemoveCapacityReservationCommand{
		Metadata: aman.CommandMetadata{CommandID: "stale-remove", ExpectedRevision: 7}, RunwayGroupID: group,
		ReservationID: reservation.ID, Reason: "cancelled",
	})
	requireDomainErrorClass(t, err, aman.ErrorRevisionConflict)
	removed, err := actions.RemoveCapacityReservation(context.Background(), auth, aman.RemoveCapacityReservationCommand{
		Metadata: aman.CommandMetadata{CommandID: "remove-extra", ExpectedRevision: 8}, RunwayGroupID: group,
		ReservationID: reservation.ID, Reason: "slot released",
	})
	require.NoError(t, err)
	require.True(t, removed.Changed)
	require.Empty(t, repository.state.RunwayGroups[0].CapacityReservations)
	var removeAudit map[string]any
	require.NoError(t, json.Unmarshal(repository.commits[1].AuditRecords[0].Payload, &removeAudit))
	require.Equal(t, "slot released", removeAudit["removal_reason"])
	require.Equal(t, "FLIGHT", removeAudit["label"])
	retryRemove, err := runwayGapActions(t, repository, &recordingPublisher{}, now.Add(3*time.Second)).RemoveCapacityReservation(context.Background(), auth, aman.RemoveCapacityReservationCommand{
		Metadata: aman.CommandMetadata{CommandID: "remove-extra", ExpectedRevision: 8}, RunwayGroupID: group,
		ReservationID: reservation.ID, Reason: "slot released",
	})
	require.NoError(t, err)
	require.True(t, retryRemove.Duplicate)
	require.Len(t, repository.commits, 2)
}

func TestCapacityReservationFreezesAcceptedIntervalAndRollsBackAtomically(t *testing.T) {
	now := time.Date(2026, time.September, 12, 12, 0, 0, 0, time.UTC)
	group := aman.RunwayGroupID("north")
	anchor := gapCommandFlight("ANCHOR", group, now, 1, aman.StateStable, aman.FreezeNone)
	blocked := gapCommandFlight("BLOCKED", group, now.Add(2*time.Minute), 2, aman.StateStable, aman.FreezeManual)
	repository := closureRepository(now, []aman.RunwayGroupID{group}, anchor, blocked)
	repository.state.RunwayGroups[0].ActiveRatePerHour = 30
	auth := closureAuth(now)
	actions := closureActions(t, repository, now.Add(time.Second))
	_, err := actions.CreateCapacityReservation(context.Background(), auth, aman.CreateCapacityReservationCommand{
		Metadata: aman.CommandMetadata{CommandID: "fixed-rate", ExpectedRevision: 7}, RunwayGroupID: group,
		AfterFlightID: anchor.ID, Label: "VIP", Reason: "operational priority",
	})
	require.NoError(t, err)
	accepted := repository.state.RunwayGroups[0].CapacityReservations[0]
	require.Equal(t, now.Add(2*time.Minute), accepted.Start)
	require.Equal(t, now.Add(4*time.Minute), accepted.End)
	repository.state.RunwayGroups[0].ActiveRatePerHour = 60
	require.Equal(t, now.Add(4*time.Minute), repository.state.RunwayGroups[0].CapacityReservations[0].End)

	repository.state.RunwayGroups[0].Closures = []aman.RunwayClosure{{ID: "closed", Start: now.Add(4 * time.Minute), Reason: "closed", CreatedAt: now, CreatedBy: "FMP"}}
	before := cloneGapState(t, repository.state)
	_, err = actions.CreateCapacityReservation(context.Background(), auth, aman.CreateCapacityReservationCommand{
		Metadata: aman.CommandMetadata{CommandID: "impossible", ExpectedRevision: 8}, RunwayGroupID: group,
		AfterFlightID: blocked.ID, Reason: "must rollback",
	})
	requireDomainErrorClass(t, err, aman.ErrorInvalidTransition)
	require.Equal(t, before, repository.state)
	require.Len(t, repository.commits, 1)
}

func TestCapacityReservationCommandsRequireTrustedFMPContext(t *testing.T) {
	now := time.Date(2026, time.September, 12, 12, 0, 0, 0, time.UTC)
	repository := closureRepository(now, []aman.RunwayGroupID{"north"}, gapCommandFlight("ANCHOR", "north", now, 1, aman.StateStable, aman.FreezeNone))
	_, err := closureActions(t, repository, now).CreateCapacityReservation(context.Background(), aman.CommandContext{
		Airport: "EKCH", Actor: "attacker", Role: "EKCH_TWR", ReceivedAt: now,
	}, aman.CreateCapacityReservationCommand{
		Metadata: aman.CommandMetadata{CommandID: "spoof", ExpectedRevision: 7}, RunwayGroupID: "north", AfterFlightID: "ANCHOR", Reason: "spoof",
	})
	requireDomainErrorClass(t, err, aman.ErrorUnauthorized)
	require.Empty(t, repository.commits)

	for _, commandType := range []reflect.Type{reflect.TypeFor[aman.CreateCapacityReservationCommand](), reflect.TypeFor[aman.RemoveCapacityReservationCommand]()} {
		for _, forbidden := range []string{"Airport", "Actor", "Role", "CreatedAt", "CreatedBy", "ReceivedAt", "VATSIMCID", "Callsign", "Surveillance"} {
			_, present := commandType.FieldByName(forbidden)
			require.Falsef(t, present, "capacity command must not accept server-owned %s", forbidden)
		}
	}
}
