package operational

import (
	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/terminal"
	"context"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

type sessionRepository struct{ states map[int32]*memoryRepository }

func (r *sessionRepository) LoadAirportState(ctx context.Context, airport string) (aman.AirportState, error) {
	return r.states[aman.SessionID(ctx)].LoadAirportState(ctx, airport)
}
func (r *sessionRepository) Commit(ctx context.Context, commit aman.StateCommit) (aman.CommitResult, error) {
	return r.states[aman.SessionID(ctx)].Commit(ctx, commit)
}

type testSessions []Session

func (s testSessions) AMANSessions(context.Context) ([]Session, error) { return s, nil }

func TestSessionReconciliationSeparatesSameAirportCallsignAndNetworkTraffic(t *testing.T) {
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	repository := &sessionRepository{states: map[int32]*memoryRepository{1: {}, 2: {}, 3: {}}}
	service, err := New(Dependencies{
		Repository: repository, Materializer: unavailableNavigation{}, Geometry: unavailableGeometry{}, Wind: unavailableWind{}, Publisher: &recordingPublisher{},
		Terminal: terminal.Configuration{Airport: "EKCH", ConfigVersion: "test", RunwayGroups: []terminal.RunwayGroup{{ID: "ARRIVAL-22"}}},
		Airports: []string{"EKCH"}, Mode: aman.ModeShadow, Now: func() time.Time { return now },
		Sessions: testSessions{{ID: 1, Airport: "EKCH"}, {ID: 2, Airport: "EKCH"}, {ID: 3, Airport: "EKCH", Live: true}},
	})
	require.NoError(t, err)
	for id := int32(1); id <= 2; id++ {
		route := []string{"ONE", "TWO"}[id-1]
		require.NoError(t, service.Observe(aman.WithSession(context.Background(), id), aman.FlightObservation{Callsign: "SAS123", Origin: "ESSA", Destination: "EKCH", Provider: aman.ObservationProviderEuroScope, FiledRoute: &route, ReconciledAt: now, SourceStatus: aman.DataFresh}))
	}
	require.NoError(t, service.Observe(context.Background(), aman.FlightObservation{Callsign: "NETWORK1", Origin: "ESSA", Destination: "EKCH", Provider: aman.ObservationProviderVATSIM, ReconciledAt: now, SourceStatus: aman.DataFresh}))
	service.Reconcile(context.Background())
	for id := int32(1); id <= 3; id++ {
		state := repository.states[id].state
		require.Equal(t, id, state.SessionID)
		require.Len(t, state.Flights, 1)
		if id < 3 {
			require.Equal(t, "SAS123", state.Flights[0].Callsign)
			require.Equal(t, []string{"ONE", "TWO"}[id-1], *state.Flights[0].LatestObservation.FiledRoute)
		} else {
			require.Equal(t, "NETWORK1", state.Flights[0].Callsign)
		}
	}
	missing := service.sessionObservations(aman.WithSession(context.Background(), 1), "EKCH")["SAS123"]
	missing.Missing = true
	require.NoError(t, service.Observe(aman.WithSession(context.Background(), 1), missing))
	require.True(t, service.sessionObservations(aman.WithSession(context.Background(), 1), "EKCH")["SAS123"].Missing)
	require.False(t, service.sessionObservations(aman.WithSession(context.Background(), 2), "EKCH")["SAS123"].Missing)
}

func TestLiveSessionRetainsNetworkArrivalAfterEuroScopeWithdrawal(t *testing.T) {
	now := time.Date(2026, 11, 9, 18, 0, 0, 0, time.UTC)
	repository := &sessionRepository{states: map[int32]*memoryRepository{1: {}, 2: {}}}
	service, err := New(Dependencies{
		Repository: repository, Materializer: unavailableNavigation{}, Geometry: unavailableGeometry{}, Wind: unavailableWind{}, Publisher: &recordingPublisher{},
		Terminal: terminal.Configuration{Airport: "EKCH", ConfigVersion: "test", RunwayGroups: []terminal.RunwayGroup{{ID: "ARRIVAL-22"}}},
		Airports: []string{"EKCH"}, Mode: aman.ModeShadow, Now: func() time.Time { return now },
		Sessions: testSessions{{ID: 1, Airport: "EKCH", Live: true}, {ID: 2, Airport: "EKCH"}},
	})
	require.NoError(t, err)
	route, star := "DCT MONAK", "MONAK1A"
	local := aman.FlightObservation{Callsign: "SVA223", Origin: "EDDF", Destination: "EKCH", Provider: aman.ObservationProviderEuroScope,
		FiledRoute: &route, AssignedSTAR: &star, ReconciledAt: now, SourceStatus: aman.DataFresh}
	for id := int32(1); id <= 2; id++ {
		require.NoError(t, service.Observe(aman.WithSession(t.Context(), id), local))
	}
	service.Reconcile(t.Context())
	local.Missing = true
	for id := int32(1); id <= 2; id++ {
		require.NoError(t, service.Observe(aman.WithSession(t.Context(), id), local))
	}
	service.Reconcile(t.Context())

	// A full-sync withdrawal must not hide a current network arrival, even
	// after the source-disappearance grace period has elapsed.
	now = now.Add(6 * time.Minute)
	altitude, speed := 30000, 450.0
	network := aman.FlightObservation{Callsign: local.Callsign, Origin: local.Origin, Destination: local.Destination,
		Provider: aman.ObservationProviderVATSIM, SurveillanceSource: aman.SurveillanceSourceVATSIM,
		VATSIMFlight: &aman.VATSIMFlightFact{Connection: &aman.VATSIMConnection{PilotFingerprint: "pilot-a", LogonAt: now.Add(-time.Hour)}},
		ReconciledAt: now, SourceStatus: aman.DataFresh, TakeoffDetected: &now,
		Surveillance: &aman.SurveillanceFact{LatitudeDegrees: 54, LongitudeDegrees: 11, AltitudeFeet: &altitude, GroundspeedKnots: &speed, ObservedAt: &now}}
	require.NoError(t, service.Observe(t.Context(), network))
	service.Reconcile(t.Context())
	live := repository.states[1].state.Flights
	require.Len(t, live, 1)
	require.False(t, live[0].LatestObservation.Missing)
	require.Equal(t, aman.DataFresh, live[0].DataStatus)
	require.Equal(t, aman.StateAirborne, live[0].State)
	require.Nil(t, live[0].Lifecycle.Absence)
	require.Equal(t, network.Surveillance, live[0].LatestObservation.Surveillance)
	require.Equal(t, star, *live[0].LatestObservation.AssignedSTAR, "controller facts remain authoritative")
	episodeID := live[0].ArrivalEpisodeID
	require.NotEmpty(t, episodeID)
	live[0].GoAroundDetection = &aman.GoAroundDetectionState{
		PolicyVersion: liveGoAroundPolicyVersion + "/test", LastControllerCommandID: "report-go-around",
	}
	require.True(t, service.sessionObservations(aman.WithSession(t.Context(), 2), "EKCH")[local.Callsign].Missing,
		"network presence must not revive training traffic")

	// Withdrawal from both sources must still start normal disappearance.
	network.Missing = true
	require.NoError(t, service.Observe(t.Context(), network))
	service.Reconcile(t.Context())
	require.True(t, repository.states[1].state.Flights[0].LatestObservation.Missing)
	require.NotNil(t, repository.states[1].state.Flights[0].Lifecycle.Absence)
	now = now.Add(6 * time.Minute)
	service.Reconcile(t.Context())
	require.Equal(t, aman.StateRemoved, repository.states[1].state.Flights[0].State)
	require.Equal(t, episodeID, repository.states[1].state.DisconnectedArrivals[local.Callsign].EpisodeID)

	// Reproduce PostgreSQL's active projection and restart without the old
	// flight row. A fresh report for the same arrival keeps its telex history.
	repository.states[1].state = cloneGapState(t, repository.states[1].state)
	repository.states[1].state.Flights = nil
	service, err = New(service.deps)
	require.NoError(t, err)
	now = now.Add(time.Second)
	local.Missing, local.ReconciledAt = false, now
	localContext := aman.WithSession(t.Context(), 1)
	require.NoError(t, service.Observe(localContext, local))
	service.Reconcile(t.Context())
	provisional := repository.states[1].state
	require.Len(t, provisional.Flights, 1)
	require.Equal(t, aman.StatePlanned, provisional.Flights[0].State)
	require.Equal(t, episodeID, provisional.DisconnectedArrivals[local.Callsign].EpisodeID)

	now = now.Add(time.Second)
	local.Missing, local.ReconciledAt = true, now
	require.NoError(t, service.Observe(localContext, local))
	service.Reconcile(t.Context())
	now = now.Add(6 * time.Minute)
	service.Reconcile(t.Context())
	expired := repository.states[1].state
	require.Equal(t, aman.StateRemoved, expired.Flights[0].State)
	history := expired.DisconnectedArrivals[local.Callsign]
	require.Equal(t, episodeID, history.EpisodeID)
	require.Equal(t, "report-go-around", history.GoAroundCommandID)
	require.Equal(t, now, history.RemovedAt)

	repository.states[1].state = cloneGapState(t, expired)
	repository.states[1].state.Flights = nil
	service, err = New(service.deps)
	require.NoError(t, err)
	now = now.Add(time.Second)
	network.Missing, network.ReconciledAt = false, now
	network.VATSIMFlight = &aman.VATSIMFlightFact{Connection: &aman.VATSIMConnection{PilotFingerprint: "pilot-a", LogonAt: now}}
	network.Surveillance.ObservedAt = &now
	require.NoError(t, service.Observe(t.Context(), network))
	service.Reconcile(t.Context())
	restored := repository.states[1].state
	require.Len(t, restored.Flights, 1)
	require.Equal(t, aman.StateAirborne, restored.Flights[0].State)
	require.False(t, restored.Flights[0].LatestObservation.Missing)
	require.Equal(t, episodeID, restored.Flights[0].ArrivalEpisodeID)
	require.NotNil(t, restored.Flights[0].GoAroundDetection)
	require.Equal(t, "report-go-around", restored.Flights[0].GoAroundDetection.LastControllerCommandID)
	require.Empty(t, restored.DisconnectedArrivals)
}

func TestLiveSessionNewPrefileStartsNewEpisodeBeforeExpiry(t *testing.T) {
	for _, scenario := range []struct {
		name                string
		missing             bool
		cachedRadar         bool
		earlyLocalDeparture bool
	}{
		{name: "during grace period", missing: true},
		{name: "without an omission"},
		{name: "cached EuroScope radar", cachedRadar: true},
		{name: "early EuroScope takeoff with the same prefile", earlyLocalDeparture: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			ctx := t.Context()
			now := time.Date(2026, 11, 9, 18, 0, 0, 0, time.UTC)
			repository := &memoryRepository{}
			deps := disconnectedExpiryDependencies(repository, &recordingPublisher{}, &now)
			deps.Sessions = testSessions{{ID: 1, Airport: "EKCH", Live: true}}
			service, err := New(deps)
			require.NoError(t, err)
			altitude, speed := 30000, 450.0
			observation := aman.FlightObservation{
				Callsign: "SAS123", Origin: "ESSA", Destination: "EKCH", Provider: aman.ObservationProviderVATSIM,
				SurveillanceSource: aman.SurveillanceSourceVATSIM,
				SourceStatus:       aman.DataFresh, ReconciledAt: now, TakeoffDetected: timePointer(now),
				Surveillance: &aman.SurveillanceFact{LatitudeDegrees: 54, LongitudeDegrees: 11,
					AltitudeFeet: &altitude, GroundspeedKnots: &speed, ObservedAt: timePointer(now)},
				VATSIMFlight: &aman.VATSIMFlightFact{Connection: &aman.VATSIMConnection{
					PilotFingerprint: "pilot-a", LogonAt: now.Add(-time.Hour),
				}},
			}
			require.NoError(t, service.Observe(ctx, observation))
			if scenario.cachedRadar {
				local := observation
				local.Provider, local.SurveillanceSource, local.VATSIMFlight = aman.ObservationProviderEuroScope, aman.SurveillanceSourceEuroScope, nil
				require.NoError(t, service.Observe(aman.WithSession(ctx, 1), local))
			}
			service.Reconcile(ctx)
			old := stateFlight(t, repository.state, "SAS123")
			require.Equal(t, aman.StateAirborne, old.State)
			repository.state.Flights[0].GoAroundDetection = &aman.GoAroundDetectionState{
				PolicyVersion: liveGoAroundPolicyVersion + "/test", LastControllerCommandID: "old-go-around",
			}

			if scenario.missing {
				now = now.Add(time.Minute)
				missing := observation
				missing.ReconciledAt, missing.Missing = now, true
				require.NoError(t, service.Observe(ctx, missing))
				service.Reconcile(ctx)
				require.NotNil(t, stateFlight(t, repository.state, "SAS123").Lifecycle.Absence)
			}
			now = now.Add(time.Minute)
			prefile := observation
			departure := now.Add(time.Hour)
			prefile.ReconciledAt, prefile.Surveillance, prefile.TakeoffDetected = now, nil, nil
			prefile.VATSIMFlight = &aman.VATSIMFlightFact{Prefiled: true, PrefiledDepartureAt: &departure}
			require.NoError(t, service.Observe(ctx, prefile))
			service.Reconcile(ctx)
			if scenario.cachedRadar {
				require.Equal(t, old.TelexEpisodeID(), stateFlight(t, repository.state, "SAS123").TelexEpisodeID(),
					"fresh local radar still belongs to the current arrival")
				now = now.Add(aman.ArrivalSurveillanceFreshness)
				service.Reconcile(ctx)
			}
			planned := stateFlight(t, repository.state, "SAS123")
			require.Equal(t, aman.StatePlanned, planned.State)
			require.NotEqual(t, old.TelexEpisodeID(), planned.TelexEpisodeID())
			require.Equal(t, &departure, planned.ConsumedPrefileDepartureAt)
			require.Nil(t, planned.LatestObservation.TakeoffDetected)
			require.Nil(t, planned.GoAroundDetection)
			require.Empty(t, repository.state.DisconnectedArrivals)
			for range 2 {
				now = now.Add(time.Minute)
				service.Reconcile(ctx)
				pending := stateFlight(t, repository.state, "SAS123")
				require.Equal(t, aman.StatePlanned, pending.State)
				require.Equal(t, planned.TelexEpisodeID(), pending.TelexEpisodeID())
				require.Nil(t, pending.LatestObservation.TakeoffDetected)
			}

			now = departure.Add(time.Minute)
			if scenario.earlyLocalDeparture {
				now = departure.Add(-10 * time.Minute)
			}
			observation.ReconciledAt, observation.TakeoffDetected = now, timePointer(now)
			position := *observation.Surveillance
			position.ObservedAt = timePointer(now)
			observation.Surveillance = &position
			observation.VATSIMFlight = &aman.VATSIMFlightFact{Connection: &aman.VATSIMConnection{
				PilotFingerprint: "pilot-a", LogonAt: departure,
			}}
			if scenario.earlyLocalDeparture {
				observation.Provider, observation.SurveillanceSource, observation.VATSIMFlight = aman.ObservationProviderEuroScope, aman.SurveillanceSourceEuroScope, nil
				require.NoError(t, service.Observe(aman.WithSession(ctx, 1), observation))
			} else {
				require.NoError(t, service.Observe(ctx, observation))
			}
			service.Reconcile(ctx)
			airborne := stateFlight(t, repository.state, "SAS123")
			require.Equal(t, aman.StateAirborne, airborne.State)
			require.Equal(t, planned.TelexEpisodeID(), airborne.TelexEpisodeID())
			require.Equal(t, now, *airborne.LatestObservation.TakeoffDetected)
			if !scenario.earlyLocalDeparture {
				return
			}

			now = now.Add(aman.ArrivalSurveillanceFreshness + time.Second)
			service.Reconcile(ctx)
			stillAirborne := stateFlight(t, repository.state, "SAS123")
			require.Equal(t, aman.StateAirborne, stillAirborne.State)
			require.Equal(t, planned.TelexEpisodeID(), stillAirborne.TelexEpisodeID())

			// A restart can refresh the same prefile observation timestamp.
			// Continue to recognize the already-consumed filed departure.
			repository.state = cloneGapState(t, repository.state)
			service, err = New(deps)
			require.NoError(t, err)
			prefile.ReconciledAt = now
			require.NoError(t, service.Observe(ctx, prefile))
			service.Reconcile(ctx)
			reloaded := stateFlight(t, repository.state, "SAS123")
			require.Equal(t, aman.StateAirborne, reloaded.State)
			require.Equal(t, planned.TelexEpisodeID(), reloaded.TelexEpisodeID())

			now = departure.Add(time.Minute)
			nextDeparture := now.Add(time.Hour)
			prefile.ReconciledAt = now
			prefile.VATSIMFlight = &aman.VATSIMFlightFact{Prefiled: true, PrefiledDepartureAt: &nextDeparture}
			require.NoError(t, service.Observe(ctx, prefile))
			service.Reconcile(ctx)
			next := stateFlight(t, repository.state, "SAS123")
			require.Equal(t, aman.StatePlanned, next.State)
			require.NotEqual(t, planned.TelexEpisodeID(), next.TelexEpisodeID())
			require.Equal(t, &nextDeparture, next.ConsumedPrefileDepartureAt)
		})
	}
}
