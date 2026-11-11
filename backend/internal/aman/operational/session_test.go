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

	// A fresh network report must also restore an already expired arrival.
	now = now.Add(time.Second)
	network.Missing, network.ReconciledAt = false, now
	require.NoError(t, service.Observe(t.Context(), network))
	service.Reconcile(t.Context())
	require.Equal(t, aman.StateAirborne, repository.states[1].state.Flights[0].State)
	require.False(t, repository.states[1].state.Flights[0].LatestObservation.Missing)
}
