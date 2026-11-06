package euroscope

import (
	"FlightStrips/internal/airportworkflow"
	"FlightStrips/internal/config"
	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	"FlightStrips/pkg/events"
	es "FlightStrips/pkg/events/euroscope"
	"bytes"
	"context"
	"github.com/stretchr/testify/require"
	"os"
	"testing"
	"time"
)

func TestRADISBackendSyncExcludesLocalPlanningAndRemovedRowsAndPreservesESState(t *testing.T) {
	data, err := os.ReadFile("../config/testdata/radis-ekbi.yaml")
	require.NoError(t, err)
	defs, err := config.NewAirportRegistry(config.ProductionAirportCatalog(), []config.AirportInput{{Source: "synthetic-only", Reader: bytes.NewReader(data)}})
	require.NoError(t, err)
	def, _ := defs.Lookup("EKBI")
	bound, err := airportworkflow.Bind(def, airportworkflow.RADISDescriptor())
	require.NoError(t, err)
	seen := time.Now().UTC()
	state := es.GroundStateLineup
	rows := []*models.Strip{{ID: 1, Session: 42, Callsign: "LOCAL1", Origin: "EKBI", Bay: "ACTIVE", BoardState: &models.StripBoardState{Local: true}}, {ID: 2, Session: 42, Callsign: "PLANNING", Origin: "EKBI", Bay: "PASSIVE_TOP"}, {ID: 3, Session: 42, Callsign: "REMOVED", Origin: "EKBI", Bay: "RUNWAY", HasFP: true, EuroscopeSeenAt: &seen, BoardState: &models.StripBoardState{RemovedAt: &seen}}, {ID: 4, Session: 42, Callsign: "ACTIVE1", Origin: "EKBI", Bay: "ACTIVE", HasFP: true, EuroscopeSeenAt: &seen, State: &state, BoardState: &models.StripBoardState{}}}
	repo := &testutil.MockStripRepository{ListFn: func(_ context.Context, session int32) ([]*models.Strip, error) {
		require.EqualValues(t, 42, session)
		return rows, nil
	}}
	hub := &Hub{server: &testutil.MockServer{StripRepoVal: repo}, airports: defs}
	client := startQueuedTestClient(&Client{send: make(chan events.OutgoingMessage, 8), workflow: bound, definition: def, session: 42, airport: "EKBI", user: shared.NewAuthenticatedUser("OWN", 0, nil)})
	require.True(t, hub.sendBackendSyncIfNeeded(client))
	for range 3 {
		_, ok := (<-client.send).(es.FsScratchPadEvent)
		require.True(t, ok)
	}
	event, ok := (<-client.send).(es.BackendSyncEvent)
	require.True(t, ok)
	require.Len(t, event.Strips, 1)
	require.Equal(t, "ACTIVE1", event.Strips[0].Callsign)
	require.Equal(t, es.GroundStateLineup, event.Strips[0].GroundState)
	require.Zero(t, event.Latitude)
	require.Zero(t, event.Longitude)
}

func TestRADISFullSyncDoesNotDisconnectNeverObservedFindStrip(t *testing.T) {
	runtime := &syncRuntimeSpy{}
	service := &EuroscopeSyncService{runtime: runtime}
	service.reconcileStaleStrip(42, map[string]bool{}, &models.Strip{Callsign: "FINDONLY", HasFP: false, BoardState: &models.StripBoardState{}})
	require.Empty(t, runtime.scheduleDisconnectCalls)
	seen := time.Now()
	service.reconcileStaleStrip(42, map[string]bool{}, &models.Strip{Callsign: "SEEN", HasFP: true, EuroscopeSeenAt: &seen, BoardState: &models.StripBoardState{}})
	require.Len(t, runtime.scheduleDisconnectCalls, 1)
	require.Equal(t, "SEEN", runtime.scheduleDisconnectCalls[0].Callsign)
}
