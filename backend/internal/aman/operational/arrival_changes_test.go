package operational

import (
	"context"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/navdata"
	"FlightStrips/internal/aman/terminal"
	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"github.com/stretchr/testify/require"
)

func TestAssignedSTAROverridesFiledRouteAndClears(t *testing.T) {
	service := &Service{deps: Dependencies{Terminal: terminal.Configuration{Feeders: []terminal.Feeder{{ID: "MONAK"}, {ID: "TESPI"}}}}}
	observation := aman.FlightObservation{FiledRoute: stringPointer("DCT MONAK"), AssignedSTAR: stringPointer("TESPI1A")}
	feeder, ok := service.observationFeeder(observation, "ARRIVAL-22")
	require.True(t, ok)
	require.Equal(t, navdata.FeederID("TESPI"), feeder)
	observation.AssignedSTAR = stringPointer("UNKNOWN1A")
	_, ok = service.observationFeeder(observation, "ARRIVAL-22")
	require.False(t, ok, "unknown assignment must not reuse the filed STAR")
	observation.AssignedSTAR = nil
	feeder, ok = service.observationFeeder(observation, "ARRIVAL-22")
	require.True(t, ok)
	require.Equal(t, navdata.FeederID("MONAK"), feeder)
}

type arrivalChangeGeometry struct {
	terminalIdentityGeometry
	holdings []navdata.HoldingPattern
}

func (g arrivalChangeGeometry) ActiveGeometrySnapshot(ctx context.Context, airport navdata.AirportID) (navdata.ActiveGeometrySnapshot, error) {
	snapshot, err := g.terminalIdentityGeometry.ActiveGeometrySnapshot(ctx, airport)
	snapshot.Holdings = g.holdings
	return snapshot, err
}

type arrivalChangeMaterializer struct{ calls int }

func (m *arrivalChangeMaterializer) MaterializeRoute(context.Context, navdata.RouteQuery, string) (navdata.RouteKey, error) {
	m.calls++
	return "new-route", nil
}

func arrivalChangeFixture(now time.Time) (*Service, aman.AirportState, aman.FlightObservation, *arrivalChangeMaterializer) {
	group := aman.RunwayGroupID("ARRIVAL-22")
	version := navdata.DatasetVersion{Cycle: "2610", SourceRevision: "test", EffectiveFrom: now.Add(-time.Hour), EffectiveUntil: now.Add(time.Hour)}
	origin, entry, hold, end := navdata.FixID("ORIGIN"), navdata.FixID("ENTRY"), navdata.FixID("HOLD"), navdata.FixID("END")
	id := navdata.HoldingID("TESPI-HOLD")
	path := navdata.TerminalPath{Version: version, Airport: "EKCH", Feeder: "TESPI", STARFamily: "TESPI", FeederFix: end, RunwayGroup: group, HoldingIDs: []navdata.HoldingID{id}, Legs: []navdata.ProcedureLeg{
		{ID: "TO-HOLD", PathTerminator: navdata.PathTF, FromFix: &entry, ToFix: &hold},
		{ID: "TO-END", PathTerminator: navdata.PathTF, FromFix: &hold, ToFix: &end},
	}}
	materializer := &arrivalChangeMaterializer{}
	geometry := arrivalChangeGeometry{terminalIdentityGeometry: terminalIdentityGeometry{
		version: version, path: path,
		route: navdata.RouteGeometry{Version: version, Digest: "new-digest", Coverage: navdata.CoverageComplete, Legs: []navdata.ProcedureLeg{{ID: "ROUTE", PathTerminator: navdata.PathTF, FromFix: &origin, ToFix: &entry}}},
		fixes: []navdata.Fix{{ID: origin, Position: navdata.Coordinate{LatitudeDeg: 55, LongitudeDeg: 12}}, {ID: entry, Position: navdata.Coordinate{LatitudeDeg: 55.1, LongitudeDeg: 12.1}}, {ID: hold, Position: navdata.Coordinate{LatitudeDeg: 55.2, LongitudeDeg: 12.2}}, {ID: end, Position: navdata.Coordinate{LatitudeDeg: 55.3, LongitudeDeg: 12.3}}},
	}, holdings: []navdata.HoldingPattern{{ID: id, Fix: hold}}}
	service := &Service{deps: Dependencies{Materializer: materializer, Geometry: geometry, Wind: unavailableWind{}, Mode: aman.ModeAuthoritative,
		Terminal: terminal.Configuration{Airport: "EKCH", ConfigVersion: "test", Feeders: []terminal.Feeder{{ID: "MONAK"}, {ID: "TESPI"}}, RunwayGroups: []terminal.RunwayGroup{{ID: group}}},
	}}
	altitude, groundspeed, revision := 10000, 300.0, uint64(7)
	observation := aman.FlightObservation{Callsign: "SAS123", Origin: "ESSA", Destination: "EKCH", FiledRoute: stringPointer("DCT MONAK"), AssignedSTAR: stringPointer("TESPI1A"), WakeCategory: stringPointer("L"),
		ReconciledAt: now, SourceStatus: aman.DataFresh, Provider: aman.ObservationProviderEuroScope, FlightPlan: aman.FlightPlanFact{Revision: &revision}, TakeoffDetected: timePointer(now.Add(-time.Hour)),
		Surveillance: &aman.SurveillanceFact{LatitudeDegrees: 55.01, LongitudeDegrees: 12.01, AltitudeFeet: &altitude, GroundspeedKnots: &groundspeed, ObservedAt: &now},
	}
	state := aman.AirportState{Airport: "EKCH", Authoritative: true, ActiveRunwayGroups: []aman.RunwayGroupID{group}, RunwayGroups: []aman.RunwayGroupPolicy{{ID: group, Selected: true, ActiveRatePerHour: 20, RateSchedule: []aman.RunwayGroupRatePoint{{EffectiveAt: now.Add(-time.Hour), ArrivalsPerHour: 20}}}}}
	return service, state, observation, materializer
}

func TestSTARReassignmentRebuildsProgressAndPreservesProtectionUnlessHoldingOccupied(t *testing.T) {
	for _, scenario := range []struct {
		name             string
		occupied, outage bool
	}{{name: "protected"}, {name: "holding-queue", occupied: true}, {name: "holding-queue-after-outage", occupied: true, outage: true}} {
		t.Run(scenario.name, func(t *testing.T) {
			occupied := scenario.occupied
			now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
			service, state, observation, materializer := arrivalChangeFixture(now)
			flight := gapCommandFlight("SAS123", "ARRIVAL-22", now.Add(20*time.Minute), 1, aman.StateStable, aman.FreezeManual)
			previous := observation
			previous.AssignedSTAR, previous.ReconciledAt = stringPointer("MONAK1A"), now.Add(-time.Second)
			flight.LatestObservation, flight.SelectedFeeder = &previous, stringPointer("MONAK")
			flight.ActiveRouteKey, flight.ActiveRouteDatasetID = stringPointer("old-route"), stringPointer(navigationDatasetID(service.deps.Geometry.(arrivalChangeGeometry).version))
			flight.RouteProgress = &aman.RouteProgress{FlightPlanRevision: 7, RunwayGroupID: "ARRIVAL-22", LegIndex: 99}
			flight.RawTETASamples = []aman.RawTETASample{{TETA: flight.Prediction.RawTETA, GeneratedAt: now}}
			flight.HoldingStack = &aman.HoldingStackState{HoldingID: "OLD-HOLD", FirstObservedAt: now.Add(-time.Hour), Confirmed: true}
			if occupied {
				leader := gapCommandFlight("LEADER", "ARRIVAL-22", now.Add(15*time.Minute), 1, aman.StateStable, aman.FreezeNone)
				leader.SelectedFeeder, leader.SelectedSTARFamily, leader.SelectedHolding = stringPointer("TESPI"), stringPointer("TESPI"), stringPointer("TESPI-HOLD")
				leader.HoldingClearance = &aman.HoldingClearance{Hold: "HOLD", HoldType: aman.HoldingClearanceEnroute, ObservedAt: now.Add(-time.Minute)}
				leader.HoldingStack = &aman.HoldingStackState{HoldingID: "TESPI-HOLD", FirstObservedAt: now.Add(-time.Minute), Confirmed: true}
				leader.Prediction.HoldingFixETA = timePointer(now.Add(-time.Minute))
				state.Flights = []aman.AMANFlight{leader}
			}
			if scenario.outage {
				geometry := service.deps.Geometry
				service.deps.Geometry = unavailableGeometry{}
				pending, err := service.reconcileFlight(t.Context(), state, flight, observation, now)
				require.Error(t, err)
				require.True(t, pending.ArrivalPathChanged)
				flight = cloneGapState(t, aman.AirportState{Flights: []aman.AMANFlight{pending}}).Flights[0]
				service.deps.Geometry = geometry
			}
			updated, err := service.reconcileFlight(t.Context(), state, flight, observation, now)
			require.NoError(t, err)
			require.Equal(t, 1, materializer.calls, "unchanged FPL revision must still rebuild the route")
			require.Equal(t, "TESPI", updated.STARFamilyIdentity())
			require.Equal(t, "new-route", *updated.ActiveRouteKey)
			require.NotEqual(t, 99, updated.RouteProgress.LegIndex)
			require.False(t, updated.ArrivalPathChanged)
			require.True(t, updated.Prediction.RawTETA.Before(now.Add(10*time.Minute)), "same radar timestamp must not retain the old STAR prediction")
			if occupied {
				require.Nil(t, updated.Slot)
				require.Equal(t, aman.FreezeNone, updated.FreezeReason)
				state.Flights = append(state.Flights, updated)
				service.resequence(&state, now)
				require.NotNil(t, state.Flights[1].Slot)
				require.True(t, state.Flights[1].Slot.Time.After(state.Flights[0].Slot.Time), "new STAR arrival must follow existing holding traffic")
			} else {
				require.Equal(t, flight.Slot, updated.Slot)
				require.Equal(t, aman.FreezeManual, updated.FreezeReason)
			}
		})
	}
}

func TestManualRemovalSurvivesReconciliationAndRestartButNotNewSession(t *testing.T) {
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	first := &memoryRepository{}
	deps := disconnectedExpiryDependencies(first, &recordingPublisher{}, &now)
	service, err := New(deps)
	require.NoError(t, err)
	removed := newFlight(aman.FlightObservation{Callsign: "SAS123", Origin: "ESSA", Destination: "EKCH", ReconciledAt: now, SourceStatus: aman.DataFresh}, now)
	removed.State = aman.StateRemoved
	removed.Lifecycle = &aman.LifecycleState{EnteredAt: now, Reason: aman.LifecycleReasonManualRemoval, LastEventAt: now, LastEventID: "remove", LastEventFingerprint: "test"}
	first.state, first.has = service.initialState("EKCH", now), true
	first.state.SessionID, first.state.Revision, first.state.Flights = 1, 7, []aman.AMANFlight{removed}
	first.state = cloneGapState(t, first.state)
	repository := &sessionRepository{states: map[int32]*memoryRepository{1: first, 2: {}}}
	deps.Repository = repository
	service, err = New(deps)
	require.NoError(t, err)
	now = now.Add(time.Second)
	observation := aman.FlightObservation{Callsign: "SAS123", Origin: "ESSA", Destination: "EKCH", AssignedSTAR: stringPointer("TESPI1A"), Provider: aman.ObservationProviderEuroScope, ReconciledAt: now, SourceStatus: aman.DataFresh}
	for _, id := range []int32{1, 2} {
		ctx := aman.WithSession(t.Context(), id)
		require.NoError(t, service.Observe(ctx, observation))
		require.NoError(t, service.reconcileAirport(ctx, "EKCH"))
		flight := stateFlight(t, repository.states[id].state, "SAS123")
		if id == 1 {
			require.Equal(t, aman.StateRemoved, flight.State)
			require.Equal(t, aman.LifecycleReasonManualRemoval, flight.Lifecycle.Reason)
			require.Equal(t, "TESPI1A", *flight.LatestObservation.AssignedSTAR)
			require.Nil(t, flight.Slot)
		} else {
			require.NotEqual(t, aman.StateRemoved, flight.State)
			require.Equal(t, aman.SequenceDispositionActive, flight.SequenceDisposition)
		}
	}
}

func TestManualRemovalRestoresExplicitlyWithFreshPrediction(t *testing.T) {
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	service, state, observation, _ := arrivalChangeFixture(now)
	removed := newFlight(observation, now)
	removed.State = aman.StateRemoved
	removed.Lifecycle = &aman.LifecycleState{EnteredAt: now, Reason: aman.LifecycleReasonManualRemoval, LastEventAt: now, LastEventID: "remove", LastEventFingerprint: "test"}
	state.Flights = []aman.AMANFlight{removed}
	auth := aman.CommandContext{Airport: "EKCH", Role: "EKDK_FMP", Actor: "123", ReceivedAt: now}
	mutation, err := service.ResumeFlight(auth, aman.ResumeFlightCommand{Callsign: "SAS123"})
	require.NoError(t, err)
	change, err := mutation(state)
	require.NoError(t, err)
	require.True(t, change.Changed)
	require.NotEqual(t, aman.StateRemoved, change.State.Flights[0].State)
	require.Equal(t, aman.SequenceDispositionActive, change.State.Flights[0].SequenceDisposition)
	require.NotNil(t, change.State.Flights[0].Slot)
	for _, invalid := range []aman.FlightObservation{
		{Destination: "ESGG", ReconciledAt: now, SourceStatus: aman.DataFresh},
		{Destination: "EKCH", ReconciledAt: now.Add(-time.Hour), SourceStatus: aman.DataFresh},
		{Destination: "EKCH", ReconciledAt: now, SourceStatus: aman.DataDisconnected},
	} {
		state.Flights[0].LatestObservation = &invalid
		_, err = mutation(state)
		require.Error(t, err)
		require.Equal(t, aman.StateRemoved, state.Flights[0].State)
	}
	_, err = service.ResumeFlight(aman.CommandContext{Role: "EKCH_APP"}, aman.ResumeFlightCommand{Callsign: "SAS123"})
	require.Error(t, err)
	state.Flights[0].State = aman.StateLanded
	_, err = mutation(state)
	require.Error(t, err)
}

type airportChangeRepository map[string]*memoryRepository

func (r airportChangeRepository) LoadAirportState(ctx context.Context, airport string) (aman.AirportState, error) {
	if repository := r[airport]; repository != nil {
		return repository.LoadAirportState(ctx, airport)
	}
	return aman.AirportState{}, &aman.DomainError{Class: aman.ErrorNotFound}
}

func (r airportChangeRepository) Commit(ctx context.Context, commit aman.StateCommit) (aman.CommitResult, error) {
	if r[commit.State.Airport] == nil {
		r[commit.State.Airport] = &memoryRepository{}
	}
	return r[commit.State.Airport].Commit(ctx, commit)
}

func TestRestartRestoresEuroScopeDestinationBeforeCrossAirportAdmission(t *testing.T) {
	for _, destination := range []string{"EKCH", "ESGG"} {
		t.Run(destination, func(t *testing.T) {
			now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
			first := &memoryRepository{}
			deps := disconnectedExpiryDependencies(first, &recordingPublisher{}, &now)
			repository := airportChangeRepository{"EKCH": first}
			deps.Repository, deps.Airports = repository, []string{"EKCH", "EKBI"}
			service, err := New(deps)
			require.NoError(t, err)
			ctx := aman.WithSession(t.Context(), 12)
			es := aman.FlightObservation{Callsign: "SAS123", Origin: "ESSA", Destination: destination,
				Provider: aman.ObservationProviderEuroScope, AssignedSTAR: stringPointer("TESPI1A"),
				ReconciledAt: now.Add(-time.Hour), SourceStatus: aman.DataFresh}
			flight := newFlight(es, es.ReconciledAt)
			if destination != "EKCH" {
				flight.State = aman.StateRemoved
				flight.Lifecycle = &aman.LifecycleState{EnteredAt: es.ReconciledAt, Reason: aman.LifecycleReasonDiverted,
					LastEventAt: es.ReconciledAt, LastEventID: "divert", LastEventFingerprint: "test"}
			}
			first.state, first.has = service.initialState("EKCH", now), true
			first.state.SessionID, first.state.Revision, first.state.Flights = 12, 7, []aman.AMANFlight{flight}
			first.state = cloneGapState(t, first.state)
			// Simulate a restarted owner with no destination cache, and reconcile
			// the conflicting network destination before the persisted airport.
			service, err = New(deps)
			require.NoError(t, err)
			network := es
			network.Provider, network.Destination, network.AssignedSTAR = aman.ObservationProviderVATSIM, "EKBI", nil
			network.ReconciledAt = now
			require.NoError(t, service.Observe(ctx, network))
			require.NoError(t, service.reconcileAirport(ctx, "EKBI"))
			require.Empty(t, repository["EKBI"].state.Flights)
			fact, found := service.destinationObservation(ctx, es.Callsign, now)
			require.True(t, found)
			require.Equal(t, destination, fact.Destination)
			require.Equal(t, "TESPI1A", *fact.AssignedSTAR)
			require.NoError(t, service.reconcileAirport(ctx, "EKCH"))
			require.Equal(t, destination, first.state.Flights[0].LatestObservation.Destination)

			// A newer ES assignment must still move the flight normally.
			now = now.Add(time.Second)
			es.Destination, es.ReconciledAt = "EKBI", now
			require.NoError(t, service.Observe(ctx, es))
			require.NoError(t, service.reconcileAirport(ctx, "EKCH"))
			require.Equal(t, aman.StateRemoved, first.state.Flights[0].State)
			require.NoError(t, service.reconcileAirport(ctx, "EKBI"))
			require.Len(t, repository["EKBI"].state.Flights, 1)
			require.NotEqual(t, aman.StateRemoved, repository["EKBI"].state.Flights[0].State)
		})
	}
}

func TestRestoredDestinationAuthorityDoesNotCrossSessions(t *testing.T) {
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	first := &memoryRepository{}
	deps := disconnectedExpiryDependencies(first, &recordingPublisher{}, &now)
	repository := airportChangeRepository{"EKCH": first}
	deps.Repository, deps.Airports = repository, []string{"EKCH", "EKBI"}
	service, err := New(deps)
	require.NoError(t, err)
	es := aman.FlightObservation{Callsign: "SAS123", Origin: "ESSA", Destination: "EKCH",
		Provider: aman.ObservationProviderEuroScope, ReconciledAt: now, SourceStatus: aman.DataFresh}
	first.state, first.has = service.initialState("EKCH", now), true
	first.state.SessionID, first.state.Flights = 12, []aman.AMANFlight{newFlight(es, now)}
	ctx := aman.WithSession(t.Context(), 13)
	network := es
	network.Provider, network.Destination = aman.ObservationProviderVATSIM, "EKBI"
	require.NoError(t, service.Observe(ctx, network))
	require.NoError(t, service.reconcileAirport(ctx, "EKBI"))
	require.Len(t, repository["EKBI"].state.Flights, 1)
	require.Equal(t, int32(13), repository["EKBI"].state.SessionID)
}

func TestRestartedEuroScopeAuthorityDoesNotPreventDisconnectedExpiry(t *testing.T) {
	base := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	now := base
	repository := &memoryRepository{}
	deps := disconnectedExpiryDependencies(repository, &recordingPublisher{}, &now)
	service, err := New(deps)
	require.NoError(t, err)
	es := aman.FlightObservation{Callsign: "SAS123", Origin: "ESSA", Destination: "EKCH",
		Provider: aman.ObservationProviderEuroScope, AssignedSTAR: stringPointer("TESPI1A"),
		SourceStatus: aman.DataFresh, ReconciledAt: base.Add(-time.Hour)}
	flight := gapCommandFlight(es.Callsign, "ARRIVAL-22", base.Add(15*time.Minute), 1, aman.StateStable, aman.FreezeManual)
	flight.LatestObservation = &es
	repository.state, repository.has = service.initialState("EKCH", base), true
	repository.state.Revision, repository.state.Flights = 7, []aman.AMANFlight{flight}
	repository.state = cloneGapState(t, repository.state)
	service, err = New(deps)
	require.NoError(t, err)
	network := es
	network.Provider, network.AssignedSTAR = aman.ObservationProviderVATSIM, nil
	network.SourceStatus, network.ReconciledAt = aman.DataDisconnected, now
	require.NoError(t, service.Observe(t.Context(), network))
	require.NoError(t, service.reconcileAirport(t.Context(), "EKCH"))
	lost := stateFlight(t, repository.state, es.Callsign)
	require.Equal(t, aman.DataDisconnected, lost.DataStatus)
	require.Equal(t, "TESPI1A", *lost.LatestObservation.AssignedSTAR)
	require.Equal(t, base.Add(5*time.Minute), *lost.Lifecycle.Absence.RemovalDueAt)
	require.NotNil(t, lost.Slot)

	now = base.Add(4 * time.Minute)
	network.SourceStatus, network.ReconciledAt = aman.DataStale, now
	require.NoError(t, service.Observe(t.Context(), network))
	require.NoError(t, service.reconcileAirport(t.Context(), "EKCH"))
	lost = stateFlight(t, repository.state, es.Callsign)
	require.Equal(t, base.Add(5*time.Minute), *lost.Lifecycle.Absence.RemovalDueAt)

	now = base.Add(5 * time.Minute)
	require.NoError(t, service.reconcileAirport(t.Context(), "EKCH"))
	lost = stateFlight(t, repository.state, es.Callsign)
	require.Equal(t, aman.StateRemoved, lost.State)
	require.Nil(t, lost.Slot)
	require.Nil(t, lost.FrozenSlot)
}

func TestSupportedDiversionAdmitsAtNewAirportUnlessManuallyRemoved(t *testing.T) {
	for _, manuallyRemoved := range []bool{false, true} {
		t.Run(map[bool]string{false: "diversion", true: "manual-exclusion"}[manuallyRemoved], func(t *testing.T) {
			now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
			first := &memoryRepository{}
			deps := disconnectedExpiryDependencies(first, &recordingPublisher{}, &now)
			repository := airportChangeRepository{"EKCH": first}
			deps.Repository, deps.Airports = repository, []string{"EKCH", "EKBI"}
			service, err := New(deps)
			require.NoError(t, err)
			flight := gapCommandFlight("SAS123", "ARRIVAL-22", now.Add(15*time.Minute), 1, aman.StateStable, aman.FreezeManual)
			if manuallyRemoved {
				flight.State = aman.StateRemoved
				clearSequencingState(&flight)
				flight.Lifecycle = &aman.LifecycleState{EnteredAt: now, Reason: aman.LifecycleReasonManualRemoval, LastEventAt: now, LastEventID: "remove", LastEventFingerprint: "test"}
			}
			first.state, first.has = service.initialState("EKCH", now), true
			first.state.Revision, first.state.Flights = 7, []aman.AMANFlight{flight}
			observation := aman.FlightObservation{Callsign: "SAS123", Origin: "ESSA", Destination: "EKBI", ReconciledAt: now, Provider: aman.ObservationProviderEuroScope, SourceStatus: aman.DataFresh}
			require.NoError(t, service.Observe(t.Context(), observation))
			require.NoError(t, service.reconcileAirport(t.Context(), "EKCH"))
			require.NoError(t, service.reconcileAirport(t.Context(), "EKBI"))
			require.Equal(t, aman.StateRemoved, first.state.Flights[0].State)
			require.Nil(t, first.state.Flights[0].Slot)
			if manuallyRemoved {
				require.Empty(t, repository["EKBI"].state.Flights)
			} else {
				require.Len(t, repository["EKBI"].state.Flights, 1)
				require.NotEqual(t, aman.StateRemoved, repository["EKBI"].state.Flights[0].State)
			}
		})
	}
}

func TestEuroScopeSTARSurvivesNetworkUpdatesAndExplicitClearing(t *testing.T) {
	at := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	service := &Service{observed: map[string]map[string]aman.FlightObservation{}}
	es := aman.FlightObservation{Callsign: "SAS123", Origin: "ESSA", Destination: "EKCH", AssignedSTAR: stringPointer("TESPI1A"), Provider: aman.ObservationProviderEuroScope, ReconciledAt: at, SourceStatus: aman.DataFresh}
	require.NoError(t, service.Observe(t.Context(), es))
	network := es
	network.Provider, network.AssignedSTAR, network.ReconciledAt = aman.ObservationProviderVATSIM, nil, at.Add(time.Second)
	require.NoError(t, service.Observe(t.Context(), network))
	require.Equal(t, "TESPI1A", *service.observations("EKCH")["SAS123"].AssignedSTAR)
	es.AssignedSTAR, es.ReconciledAt = nil, at.Add(2*time.Second)
	require.NoError(t, service.Observe(t.Context(), es))
	require.Nil(t, service.observations("EKCH")["SAS123"].AssignedSTAR)
	es.AssignedSTAR, es.ReconciledAt = stringPointer("MONAK1A"), at
	require.NoError(t, service.Observe(t.Context(), es))
	require.Nil(t, service.observations("EKCH")["SAS123"].AssignedSTAR, "old strip updates cannot reinstate an assignment")
}

func TestRestartedOwnerKeepsPersistedSTARAndFreshDestinationAuthority(t *testing.T) {
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	service, state, observation, _ := arrivalChangeFixture(now)
	flight := newFlight(observation, now)
	flight.State = aman.StateRemoved
	flight.Lifecycle = &aman.LifecycleState{EnteredAt: now, Reason: aman.LifecycleReasonManualRemoval, LastEventAt: now, LastEventID: "remove", LastEventFingerprint: "test"}
	network := observation
	network.AssignedSTAR, network.Provider, network.ReconciledAt = nil, aman.ObservationProviderVATSIM, now.Add(time.Second)
	updated, err := service.reconcileFlight(t.Context(), state, flight, network, network.ReconciledAt)
	require.NoError(t, err)
	require.Equal(t, "TESPI1A", *updated.LatestObservation.AssignedSTAR)
	require.Equal(t, aman.StateRemoved, updated.State)
	flight.LatestObservation.Destination = "ESGG"
	network.Destination = "EKCH"
	updated, err = service.reconcileFlight(t.Context(), state, flight, network, network.ReconciledAt)
	require.NoError(t, err)
	require.Equal(t, "ESGG", updated.LatestObservation.Destination)
}

func TestDiversionReleasesSlotImmediatelyAndReturnUsesFreshFacts(t *testing.T) {
	for _, fullSync := range []bool{false, true} {
		t.Run(map[bool]string{false: "individual", true: "full-sync"}[fullSync], func(t *testing.T) {
			now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
			repository := &memoryRepository{}
			deps := disconnectedExpiryDependencies(repository, &recordingPublisher{}, &now)
			service, err := New(deps)
			require.NoError(t, err)
			ctx := aman.WithSession(t.Context(), 12)
			flight := gapCommandFlight("SAS123", "ARRIVAL-22", now.Add(15*time.Minute), 1, aman.StateStable, aman.FreezeManual)
			repository.state, repository.has = service.initialState("EKCH", now), true
			repository.state.SessionID, repository.state.Revision, repository.state.Flights = 12, 7, []aman.AMANFlight{flight}
			observer, err := NewEuroScopeStripObserver(EuroScopeStripObserverDependencies{Sink: service, EnabledAirports: []string{"EKCH"}, Now: func() time.Time { return now }})
			require.NoError(t, err)
			strip := &models.Strip{Session: 12, Callsign: "SAS123", Origin: "ESSA", Destination: "EKCH", Star: stringPointer("TESPI1A")}
			require.NoError(t, observer.ObserveEuroScopeStrip(ctx, strip))
			require.Equal(t, "TESPI1A", *service.observations("12/EKCH")["SAS123"].AssignedSTAR)
			now = now.Add(time.Second)
			strip.Destination = "ESGG"
			if fullSync {
				require.NoError(t, observer.ObserveEuroScopeStrips(ctx, 12, []shared.EuroScopeStripObservation{{Strip: strip, ObservedAt: now}}))
			} else {
				require.NoError(t, observer.ObserveEuroScopeStrip(ctx, strip))
			}
			require.NoError(t, service.reconcileAirport(ctx, "EKCH"))
			removed := stateFlight(t, repository.state, "SAS123")
			require.Equal(t, aman.StateRemoved, removed.State)
			require.Equal(t, aman.LifecycleReasonDiverted, removed.Lifecycle.Reason)
			require.Nil(t, removed.Slot)
			require.Nil(t, removed.Lifecycle.Absence, "diversion does not use disappearance grace")
			// A restarted owner must reject an older airport observation.
			service, err = New(deps)
			require.NoError(t, err)
			old := aman.FlightObservation{Callsign: "SAS123", Origin: "ESSA", Destination: "EKCH", ReconciledAt: now.Add(-time.Second), SourceStatus: aman.DataFresh}
			updated, err := service.reconcileFlight(ctx, repository.state, removed, old, now)
			require.NoError(t, err)
			require.Equal(t, removed, updated)
			old.ReconciledAt = now.Add(time.Second)
			updated, err = service.reconcileFlight(ctx, repository.state, removed, old, old.ReconciledAt)
			require.NoError(t, err)
			require.NotEqual(t, aman.StateRemoved, updated.State)
			require.Nil(t, updated.Slot)
		})
	}
}

func TestDestinationAuthorityAndSessionIsolation(t *testing.T) {
	at := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	service := &Service{observed: map[string]map[string]aman.FlightObservation{}, liveSessions: map[int32]bool{1: true, 2: true}}
	ctx := aman.WithSession(t.Context(), 1)
	es := aman.FlightObservation{Callsign: "SAS123", Origin: "ESSA", Destination: "ESGG", Provider: aman.ObservationProviderEuroScope, ReconciledAt: at, SourceStatus: aman.DataFresh}
	require.NoError(t, service.Observe(ctx, es))
	network := es
	network.Provider, network.Destination, network.ReconciledAt = aman.ObservationProviderVATSIM, "EKCH", at.Add(time.Second)
	network.FlightPlan.ObservedAt = timePointer(at.Add(-time.Minute))
	require.NoError(t, service.Observe(context.Background(), network))
	fact, found := service.destinationObservation(ctx, "SAS123", network.ReconciledAt)
	require.True(t, found)
	require.Equal(t, "ESGG", fact.Destination)
	fact, found = service.destinationObservation(aman.WithSession(t.Context(), 2), "SAS123", network.ReconciledAt)
	require.True(t, found)
	require.Equal(t, "EKCH", fact.Destination)
	// A later receipt of an old network flight plan cannot roll back the diversion.
	network.FlightPlan.ObservedAt = timePointer(at.Add(-time.Minute))
	network.ReconciledAt = at.Add(5 * time.Minute)
	require.NoError(t, service.Observe(context.Background(), network))
	fact, _ = service.destinationObservation(ctx, "SAS123", network.ReconciledAt)
	require.Equal(t, "ESGG", fact.Destination)
	// Even a genuinely newer network flight plan cannot change ES's destination.
	network.FlightPlan.ObservedAt = timePointer(at.Add(10 * time.Minute))
	network.ReconciledAt = at.Add(10 * time.Minute)
	require.NoError(t, service.Observe(context.Background(), network))
	fact, _ = service.destinationObservation(ctx, "SAS123", network.ReconciledAt)
	require.Equal(t, "ESGG", fact.Destination)
}

func TestEuroScopeDiversionAndReturnRemainAuthoritativeOverNewerNetworkReceipts(t *testing.T) {
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	repository := &memoryRepository{}
	deps := disconnectedExpiryDependencies(repository, &recordingPublisher{}, &now)
	service, err := New(deps)
	require.NoError(t, err)
	ctx := aman.WithSession(t.Context(), 12)
	service.liveSessions[12] = true
	flight := gapCommandFlight("SAS123", "ARRIVAL-22", now.Add(15*time.Minute), 1, aman.StateStable, aman.FreezeManual)
	es := aman.FlightObservation{Callsign: "SAS123", Origin: "ESSA", Destination: "EKCH", Provider: aman.ObservationProviderEuroScope,
		AssignedSTAR: stringPointer("TESPI1A"), ReconciledAt: now, SourceStatus: aman.DataFresh, FlightPlan: aman.FlightPlanFact{ObservedAt: timePointer(now)}}
	flight.LatestObservation = &es
	repository.state, repository.has = service.initialState("EKCH", now), true
	repository.state.SessionID, repository.state.Revision, repository.state.Flights = 12, 7, []aman.AMANFlight{flight}
	require.NoError(t, service.Observe(ctx, es))
	// The merged observation's receipt is later than the next ES diversion.
	now = now.Add(30 * time.Second)
	network := es
	network.Provider, network.AssignedSTAR, network.ReconciledAt = aman.ObservationProviderVATSIM, nil, now
	require.NoError(t, service.Observe(context.Background(), network))
	require.NoError(t, service.reconcileAirport(ctx, "EKCH"))
	require.Equal(t, now, repository.state.Flights[0].LatestObservation.ReconciledAt)
	es.Destination, es.ReconciledAt = "ESGG", now.Add(-10*time.Second)
	es.FlightPlan.ObservedAt = timePointer(es.ReconciledAt)
	require.NoError(t, service.Observe(ctx, es))
	require.NoError(t, service.reconcileAirport(ctx, "EKCH"))
	removed := stateFlight(t, repository.state, "SAS123")
	require.Equal(t, aman.LifecycleReasonDiverted, removed.Lifecycle.Reason)
	require.Nil(t, removed.Slot)
	// VATSIM cannot return the flight, even after the former ES timeout.
	now = now.Add(10 * time.Minute)
	network.ReconciledAt, network.FlightPlan.ObservedAt = now, timePointer(now)
	require.NoError(t, service.Observe(context.Background(), network))
	require.NoError(t, service.reconcileAirport(ctx, "EKCH"))
	require.Equal(t, "ESGG", repository.state.Flights[0].LatestObservation.Destination)
	require.Equal(t, aman.StateRemoved, repository.state.Flights[0].State)
	// An explicit ES return restores admission despite the later VATSIM receipt.
	es.Destination, es.ReconciledAt = "EKCH", now.Add(-time.Second)
	es.FlightPlan.ObservedAt = timePointer(es.ReconciledAt)
	require.NoError(t, service.Observe(ctx, es))
	require.NoError(t, service.reconcileAirport(ctx, "EKCH"))
	require.NotEqual(t, aman.StateRemoved, repository.state.Flights[0].State)
	require.Equal(t, "EKCH", repository.state.Flights[0].LatestObservation.Destination)
	require.Equal(t, "TESPI1A", *repository.state.Flights[0].LatestObservation.AssignedSTAR)
}

func TestRestartKeepsEuroScopeDiversionAuthoritativeWithoutNewEuroScopeReports(t *testing.T) {
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	service, state, es, _ := arrivalChangeFixture(now)
	es.Destination = "ESGG"
	es.FlightPlan.ObservedAt = timePointer(now)
	removed := newFlight(es, now)
	removed.State = aman.StateRemoved
	removed.Lifecycle = &aman.LifecycleState{EnteredAt: now, Reason: aman.LifecycleReasonDiverted, LastEventAt: now, LastEventID: "divert", LastEventFingerprint: "test"}
	removed = cloneGapState(t, aman.AirportState{Flights: []aman.AMANFlight{removed}}).Flights[0]
	network := es
	network.Provider, network.Destination, network.AssignedSTAR = aman.ObservationProviderVATSIM, "EKCH", nil
	network.ReconciledAt, network.FlightPlan.ObservedAt = now.Add(time.Hour), timePointer(now.Add(time.Hour))
	updated, err := service.reconcileFlight(t.Context(), state, removed, network, network.ReconciledAt)
	require.NoError(t, err)
	require.Equal(t, aman.StateRemoved, updated.State)
	require.Equal(t, "ESGG", updated.LatestObservation.Destination)
	require.Equal(t, "TESPI1A", *updated.LatestObservation.AssignedSTAR)
}

func TestMissingNetworkReportPreservesEuroScopeFactsAndDisappearance(t *testing.T) {
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	_, _, es, _ := arrivalChangeFixture(now)
	es.Destination = "ESGG"
	es.FlightPlan.ObservedAt = timePointer(now)
	network := es
	network.Provider, network.Destination, network.AssignedSTAR = aman.ObservationProviderVATSIM, "EKCH", nil
	network.Missing, network.SourceStatus = true, aman.DataStale
	network.ReconciledAt = now.Add(time.Hour)
	merged := mergeSurveillanceObservation(es, network)
	require.Equal(t, "ESGG", merged.Destination)
	require.Equal(t, "TESPI1A", *merged.AssignedSTAR)
	require.Equal(t, now, observationFactTime(merged))
	require.True(t, merged.Missing)
	require.Equal(t, aman.DataStale, merged.SourceStatus)
}
