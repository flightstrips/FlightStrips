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
