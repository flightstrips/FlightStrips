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

func TestPlaceFlightAtTimeAuditsGapExceptionAndRetriesAfterRestart(t *testing.T) {
	now := time.Date(2026, time.September, 12, 12, 0, 0, 0, time.UTC)
	repository := &memoryRepository{has: true, state: manualPlacementState(now)}
	auth := aman.CommandContext{Airport: "EKCH", Actor: "1234567", Role: "EKCH_FMH", ReceivedAt: now}
	command := aman.PlaceFlightAtTimeCommand{
		Metadata: aman.CommandMetadata{CommandID: "place-in-gap", ExpectedRevision: 7}, FlightID: "TARGET",
		RunwayGroupID: "north", SlotTime: now.Add(5 * time.Minute), AllowGap: true,
	}

	placed, err := manualPlacementActions(t, repository, now.Add(time.Second)).PlaceFlightAtTime(context.Background(), auth, command)
	require.NoError(t, err)
	require.True(t, placed.Changed)
	require.Equal(t, aman.SequenceRevision(8), placed.CurrentRevision)
	target := stateFlight(t, repository.state, "TARGET")
	require.Equal(t, command.SlotTime, target.Slot.Time)
	require.Equal(t, aman.FreezeManual, target.FreezeReason)
	require.Equal(t, target.Slot.Time, target.FrozenSlot.Time)
	require.Equal(t, &aman.RunwayGapException{
		GapID: "gap-1", FlightID: "TARGET", RunwayGroupID: "north", Opportunity: command.SlotTime, CommandID: "place-in-gap",
	}, target.RunwayGapException)
	require.Len(t, repository.commits, 1)
	var audit map[string]any
	require.NoError(t, json.Unmarshal(repository.commits[0].AuditRecords[0].Payload, &audit))
	require.Equal(t, auth.Actor, audit["actor"])
	require.Equal(t, auth.Role, audit["role"])
	require.Equal(t, auth.Airport, audit["airport"])
	require.Equal(t, "gap-1", audit["overridden_gap_id"])
	require.NotNil(t, audit["prior_slot"])
	require.NotNil(t, audit["new_slot"])

	retry, err := manualPlacementActions(t, repository, now.Add(2*time.Second)).PlaceFlightAtTime(context.Background(), auth, command)
	require.NoError(t, err)
	require.True(t, retry.Duplicate)
	require.False(t, retry.Changed)
	require.Equal(t, placed.Outcome, retry.Outcome)
	require.Len(t, repository.commits, 1)
}

func TestPlaceFlightAtTimeEnforcesGapAuthorityAndPayloadTrust(t *testing.T) {
	now := time.Date(2026, time.September, 12, 12, 0, 0, 0, time.UTC)
	base := aman.PlaceFlightAtTimeCommand{
		Metadata: aman.CommandMetadata{CommandID: "place", ExpectedRevision: 7}, FlightID: "TARGET",
		RunwayGroupID: "north", SlotTime: now.Add(5 * time.Minute), AllowGap: true,
	}

	for _, test := range []struct {
		name string
		auth aman.CommandContext
		edit func(*aman.PlaceFlightAtTimeCommand)
		want aman.ErrorClass
	}{
		{name: "non FMP cannot request exception", auth: aman.CommandContext{Airport: "EKCH", Actor: "spoof", Role: "EKCH_TWR", ReceivedAt: now}, want: aman.ErrorUnauthorized},
		{name: "normal placement cannot enter GAP", auth: aman.CommandContext{Airport: "EKCH", Actor: "123", Role: "EKCH_TWR", ReceivedAt: now}, edit: func(c *aman.PlaceFlightAtTimeCommand) { c.AllowGap = false }, want: aman.ErrorInvalidTransition},
		{name: "exception must name real GAP use", auth: aman.CommandContext{Airport: "EKCH", Actor: "123", Role: "EKCH_FMH", ReceivedAt: now}, edit: func(c *aman.PlaceFlightAtTimeCommand) { c.SlotTime = now.Add(8 * time.Minute) }, want: aman.ErrorInvalidArgument},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository := &memoryRepository{has: true, state: manualPlacementState(now)}
			command := base
			command.Metadata.CommandID = test.name
			if test.edit != nil {
				test.edit(&command)
			}
			_, err := manualPlacementActions(t, repository, now).PlaceFlightAtTime(context.Background(), test.auth, command)
			requireDomainErrorClass(t, err, test.want)
			require.Empty(t, repository.commits)
		})
	}

	repository := &memoryRepository{has: true, state: manualPlacementState(now)}
	placed, err := manualPlacementActions(t, repository, now).PlaceFlightAtTime(context.Background(), aman.CommandContext{
		Airport: "EKCH", Actor: "tower", Role: "EKCH_TWR", ReceivedAt: now,
	}, aman.PlaceFlightAtTimeCommand{
		Metadata: aman.CommandMetadata{CommandID: "normal-placement", ExpectedRevision: 7}, FlightID: "TARGET",
		RunwayGroupID: "north", SlotTime: now.Add(8 * time.Minute),
	})
	require.NoError(t, err)
	require.True(t, placed.Changed)
	require.Nil(t, stateFlight(t, repository.state, "TARGET").RunwayGapException)

	typeOf := reflect.TypeFor[aman.PlaceFlightAtTimeCommand]()
	for _, forbidden := range []string{"Airport", "Actor", "Role", "ReceivedAt", "AllowedBy"} {
		_, present := typeOf.FieldByName(forbidden)
		require.Falsef(t, present, "manual placement payload must not accept server-owned %s", forbidden)
	}
}

func TestPlaceFlightAtTimeValidatesGridRunwayRevisionAndAtomicResult(t *testing.T) {
	now := time.Date(2026, time.September, 12, 12, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name        string
		editState   func(*aman.AirportState)
		editCommand func(*aman.PlaceFlightAtTimeCommand)
		want        aman.ErrorClass
	}{
		{name: "off grid", editCommand: func(c *aman.PlaceFlightAtTimeCommand) { c.SlotTime = c.SlotTime.Add(30 * time.Second) }, want: aman.ErrorInvalidArgument},
		{name: "stale revision", editCommand: func(c *aman.PlaceFlightAtTimeCommand) { c.Metadata.ExpectedRevision-- }, want: aman.ErrorRevisionConflict},
		{name: "unknown runway", editCommand: func(c *aman.PlaceFlightAtTimeCommand) { c.RunwayGroupID = "missing" }, want: aman.ErrorNotFound},
		{name: "inactive runway", editState: func(s *aman.AirportState) { s.ActiveRunwayGroups = []aman.RunwayGroupID{"south"} }, want: aman.ErrorInvalidArgument},
		{name: "incompatible runway", editState: func(s *aman.AirportState) { s.Flights[0].SelectedFeeder = stringPointer("OTHER") }, want: aman.ErrorInvalidArgument},
		{name: "occupied protected opportunity", editState: func(s *aman.AirportState) {
			s.Flights = append(s.Flights, gapCommandFlight("BLOCKER", "north", now.Add(5*time.Minute), 2, aman.StateStable, aman.FreezeSuperstable))
		}, want: aman.ErrorInvalidTransition},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := manualPlacementState(now)
			if test.editState != nil {
				test.editState(&state)
			}
			repository := &memoryRepository{has: true, state: cloneGapState(t, state)}
			before := cloneGapState(t, repository.state)
			command := aman.PlaceFlightAtTimeCommand{
				Metadata: aman.CommandMetadata{CommandID: test.name, ExpectedRevision: 7}, FlightID: "TARGET",
				RunwayGroupID: "north", SlotTime: now.Add(5 * time.Minute), AllowGap: true,
			}
			if test.editCommand != nil {
				test.editCommand(&command)
			}
			_, err := manualPlacementActions(t, repository, now).PlaceFlightAtTime(context.Background(), aman.CommandContext{
				Airport: "EKCH", Actor: "123", Role: "EKCH_FMH", ReceivedAt: now,
			}, command)
			requireDomainErrorClass(t, err, test.want)
			require.Equal(t, before, repository.state)
			require.Empty(t, repository.commits)
			require.Empty(t, repository.outcomes)
		})
	}
}

func TestRunwayGapExceptionClearsOnMoveAndGapRemoval(t *testing.T) {
	now := time.Date(2026, time.September, 12, 12, 0, 0, 0, time.UTC)
	place := func(t *testing.T) (*memoryRepository, *sequence.ActionService, aman.CommandContext) {
		repository := &memoryRepository{has: true, state: manualPlacementState(now)}
		auth := aman.CommandContext{Airport: "EKCH", Actor: "123", Role: "EKCH_FMH", ReceivedAt: now}
		actions := manualPlacementActions(t, repository, now.Add(time.Second))
		_, err := actions.PlaceFlightAtTime(context.Background(), auth, aman.PlaceFlightAtTimeCommand{
			Metadata: aman.CommandMetadata{CommandID: "place", ExpectedRevision: 7}, FlightID: "TARGET",
			RunwayGroupID: "north", SlotTime: now.Add(5 * time.Minute), AllowGap: true,
		})
		require.NoError(t, err)
		return repository, actions, auth
	}

	t.Run("move elsewhere", func(t *testing.T) {
		repository, actions, auth := place(t)
		_, err := actions.PlaceFlightAtTime(context.Background(), auth, aman.PlaceFlightAtTimeCommand{
			Metadata: aman.CommandMetadata{CommandID: "move-out", ExpectedRevision: 8}, FlightID: "TARGET",
			RunwayGroupID: "north", SlotTime: now.Add(8 * time.Minute),
		})
		require.NoError(t, err)
		require.Nil(t, stateFlight(t, repository.state, "TARGET").RunwayGapException)
	})

	t.Run("remove relevant GAP", func(t *testing.T) {
		repository, actions, auth := place(t)
		_, err := actions.RemoveRunwayGap(context.Background(), auth, aman.RemoveRunwayGapCommand{
			Metadata: aman.CommandMetadata{CommandID: "remove-gap", ExpectedRevision: 8}, RunwayGroupID: "north", GapID: "gap-1",
		})
		require.NoError(t, err)
		require.Nil(t, stateFlight(t, repository.state, "TARGET").RunwayGapException)
	})
}

func manualPlacementState(now time.Time) aman.AirportState {
	state := runwayGapDisplacementState(now)
	state.Flights = state.Flights[:1]
	state.Flights[0].ID, state.Flights[0].VATSIMCID, state.Flights[0].CurrentCallsign = "TARGET", "TARGET", "TARGET"
	state.Flights[0].SelectedFeeder = stringPointer("MONAK")
	state.ActiveRunwayGroups = []aman.RunwayGroupID{"north"}
	state.RunwayGroups = append(state.RunwayGroups, aman.RunwayGroupPolicy{ID: "south", ActiveRatePerHour: 60, RateEffectiveAt: &now})
	state.RunwayGroups[0].Selected, state.RunwayGroups[0].Active = true, true
	state.RunwayGroups[0].Gaps = []aman.RunwayGap{{
		ID: "gap-1", Start: now.Add(5 * time.Minute), End: now.Add(7 * time.Minute), Label: "stop", CreatedAt: now.Add(-time.Minute), CreatedBy: "fmp",
	}}
	return state
}

func manualPlacementActions(t *testing.T, repository *memoryRepository, recordedAt time.Time) *sequence.ActionService {
	t.Helper()
	coordinator, err := sequence.NewCoordinator(sequence.CoordinatorDependencies{
		States: repository, Outcomes: repository, Committer: repository, Publisher: &recordingPublisher{}, Now: func() time.Time { return recordedAt },
	})
	require.NoError(t, err)
	actions, err := sequence.NewActionService(coordinator, &Service{deps: Dependencies{
		FMPRoles: []string{"EKCH_FMH"}, Terminal: terminal.Configuration{
			RunwayGroups: []terminal.RunwayGroup{{ID: "north"}, {ID: "south"}},
			Paths:        []terminal.Path{{Feeder: "MONAK", RunwayGroup: "north"}},
		},
	}})
	require.NoError(t, err)
	return actions
}
