package operational

import (
	"context"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/navdata"
	"FlightStrips/internal/aman/sequence"
	"FlightStrips/internal/aman/terminal"
	"github.com/stretchr/testify/require"
)

func TestRecomputeFlightIsRevisionCheckedAuditedAndDurablyIdempotent(t *testing.T) {
	service, state, now := recomputeFlightFixture(t)
	repository := &memoryRepository{state: state, has: true}
	publisher := &recordingPublisher{}
	actions := recomputeActions(t, service, repository, publisher, now)
	auth := aman.CommandContext{Airport: "EKCH", Actor: "1234567", Role: "EKCH_FMH", ReceivedAt: now}
	command := aman.RecomputeFlightCommand{Metadata: aman.CommandMetadata{CommandID: "recompute-1", ExpectedRevision: state.Revision}, FlightID: "flight-1"}

	first, err := actions.RecomputeFlight(context.Background(), auth, command)
	require.NoError(t, err)
	require.True(t, first.Changed)
	require.False(t, first.Duplicate)
	require.Equal(t, state.Revision+1, first.CurrentRevision)
	require.Len(t, publisher.states, 1, "the replacement state is the server confirmation")
	require.JSONEq(t, `{"action":"recompute_flight","actor":"1234567","airport":"EKCH","changed":true,"flight_id":"flight-1","input_observed_at":"2026-09-11T12:00:00Z","received_at":"2026-09-11T12:01:00Z","role":"EKCH_FMH"}`, string(repository.commits[0].AuditRecords[0].Payload))

	retry, err := actions.RecomputeFlight(context.Background(), auth, command)
	require.NoError(t, err)
	require.True(t, retry.Duplicate)
	require.Len(t, repository.commits, 1)

	restarted := recomputeActions(t, service, repository, publisher, now)
	retry, err = restarted.RecomputeFlight(context.Background(), auth, command)
	require.NoError(t, err)
	require.True(t, retry.Duplicate)
	require.Equal(t, first.Outcome, retry.Outcome)
	require.Len(t, repository.commits, 1)

	stale := command
	stale.Metadata = aman.CommandMetadata{CommandID: "recompute-stale", ExpectedRevision: state.Revision}
	result, err := restarted.RecomputeFlight(context.Background(), auth, stale)
	requireDomainClass(t, err, aman.ErrorRevisionConflict)
	require.Equal(t, state.Revision+1, result.CurrentRevision)
	require.Len(t, repository.commits, 1)
}

func TestRecomputeFlightPreservesFrozenOperationalSlotAndProtectedOrder(t *testing.T) {
	service, state, now := recomputeFlightFixture(t)
	flight := &state.Flights[0]
	flight.State, flight.FreezeReason = aman.StateStable, aman.FreezeSuperstable
	frozenTETA, frozenAt := flight.Prediction.OperationalTETA, now.Add(-time.Minute)
	flight.FrozenOperationalTETA, flight.FrozenAt = &frozenTETA, &frozenAt
	flight.Slot = &aman.Slot{Time: frozenTETA, RunwayGroupID: *flight.SelectedRunwayGroup, Sequence: 1, Revision: state.Revision, Reason: "protected"}
	frozenSlot := *flight.Slot
	flight.FrozenSlot = &frozenSlot
	other := protectedOperationalFlight("flight-2", *flight.SelectedRunwayGroup, flight.STARFamilyIdentity(), "L", frozenTETA.Add(3*time.Minute), 2, aman.FreezeManual)
	state.Flights = append(state.Flights, other)

	mutation, err := service.RecomputeFlight(context.Background(), aman.CommandContext{Airport: "EKCH", Actor: "1234567", Role: "EKCH_FMH", ReceivedAt: now}, aman.RecomputeFlightCommand{Metadata: aman.CommandMetadata{CommandID: "frozen", ExpectedRevision: state.Revision}, FlightID: flight.ID})
	require.NoError(t, err)
	change, err := mutation(state)
	require.NoError(t, err)
	require.Equal(t, aman.FreezeSuperstable, change.State.Flights[0].FreezeReason)
	require.Equal(t, frozenTETA, change.State.Flights[0].Prediction.OperationalTETA)
	require.Equal(t, frozenSlot.Time, change.State.Flights[0].Slot.Time)
	require.Equal(t, 1, change.State.Flights[0].Slot.Sequence)
	require.Equal(t, other.FrozenSlot.Time, change.State.Flights[1].Slot.Time)
	require.Equal(t, 2, change.State.Flights[1].Slot.Sequence)
}

func TestRecomputeFlightPredictionFailureIsAtomic(t *testing.T) {
	service, state, now := recomputeFlightFixture(t)
	service.deps.Geometry = unavailableGeometry{}
	repository := &memoryRepository{state: state, has: true}
	actions := recomputeActions(t, service, repository, &recordingPublisher{}, now)
	_, err := actions.RecomputeFlight(context.Background(), aman.CommandContext{Airport: "EKCH", Actor: "1234567", Role: "EKCH_FMH", ReceivedAt: now}, aman.RecomputeFlightCommand{Metadata: aman.CommandMetadata{CommandID: "failure", ExpectedRevision: state.Revision}, FlightID: "flight-1"})
	require.Error(t, err)
	require.Empty(t, repository.commits)
	require.Equal(t, state, repository.state)
}

func recomputeActions(t *testing.T, service *Service, repository *memoryRepository, publisher *recordingPublisher, now time.Time) *sequence.ActionService {
	t.Helper()
	coordinator, err := sequence.NewCoordinator(sequence.CoordinatorDependencies{States: repository, Outcomes: repository, Committer: repository, Publisher: publisher, Now: func() time.Time { return now }})
	require.NoError(t, err)
	actions, err := sequence.NewActionService(coordinator, service)
	require.NoError(t, err)
	return actions
}

func recomputeFlightFixture(t *testing.T) (*Service, aman.AirportState, time.Time) {
	t.Helper()
	observedAt := time.Date(2026, time.September, 11, 12, 0, 0, 0, time.UTC)
	now := observedAt.Add(time.Minute)
	group := aman.RunwayGroupID("ARRIVAL-22L")
	version := navdata.DatasetVersion{Cycle: "2609", SourceRevision: "test", EffectiveFrom: observedAt.Add(-time.Hour), EffectiveUntil: observedAt.Add(time.Hour)}
	origin, star, feederFix := navdata.FixID("ORIGIN"), navdata.FixID("TESPI"), navdata.FixID("TNO")
	path := navdata.TerminalPath{Version: version, Airport: "EKCH", Feeder: "TESPI", STARFamily: "TESPI", FeederFix: feederFix, RunwayGroup: group, Legs: []navdata.ProcedureLeg{{ID: "TERMINAL", PathTerminator: navdata.PathTF, FromFix: &star, ToFix: &feederFix}}}
	geometry := terminalIdentityGeometry{version: version, path: path, route: navdata.RouteGeometry{Version: version, Digest: "route-digest", Coverage: navdata.CoverageComplete, Legs: []navdata.ProcedureLeg{{ID: "ROUTE", PathTerminator: navdata.PathTF, FromFix: &origin, ToFix: &star}}}, fixes: []navdata.Fix{{ID: origin, Position: navdata.Coordinate{LatitudeDeg: 55, LongitudeDeg: 12}}, {ID: star, Position: navdata.Coordinate{LatitudeDeg: 55.1, LongitudeDeg: 12.1}}, {ID: feederFix, Position: navdata.Coordinate{LatitudeDeg: 55.2, LongitudeDeg: 12.2}}}}
	service := &Service{deps: Dependencies{Materializer: fixedNavigation{key: "route"}, Geometry: geometry, Terminal: terminal.Configuration{ConfigVersion: "test-v1", Feeders: []terminal.Feeder{{ID: "TESPI"}}, Paths: []terminal.Path{{Feeder: "TESPI", RunwayGroup: group}}, RunwayGroups: []terminal.RunwayGroup{{ID: group}}}}}
	altitude, groundspeed, route, wake := 10_000, 300.0, "DCT TESPI", "L"
	observation := aman.FlightObservation{FlightID: "flight-1", VATSIMCID: "123", Callsign: "SAS123", Origin: "ENGM", Destination: "EKCH", FiledRoute: &route, WakeCategory: &wake, ReconciledAt: observedAt, SourceStatus: aman.DataFresh, Surveillance: &aman.SurveillanceFact{LatitudeDegrees: 55.01, LongitudeDegrees: 12.01, AltitudeFeet: &altitude, GroundspeedKnots: &groundspeed, ObservedAt: &observedAt}}
	effective := observedAt
	state := aman.AirportState{Airport: "EKCH", Revision: 7, GeneratedAt: observedAt, PolicyVersion: "test", Mode: aman.ModeAuthoritative, Authoritative: true, RunwayGroups: []aman.RunwayGroupPolicy{{ID: group, Selected: true, ActiveRatePerHour: 20, RateEffectiveAt: &effective, RateSchedule: []aman.RunwayGroupRatePoint{{EffectiveAt: effective, ArrivalsPerHour: 20}}}}}
	flight, err := service.reconcileFlight(context.Background(), state, newFlight(observation, observedAt), observation, observedAt)
	require.NoError(t, err)
	require.NotNil(t, flight.Prediction)
	state.Flights = []aman.AMANFlight{flight}
	service.resequence(&state, observedAt)
	return service, state, now
}
