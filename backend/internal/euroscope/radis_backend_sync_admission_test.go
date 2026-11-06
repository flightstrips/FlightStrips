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

func TestRADISBackendSyncSkipsFeedPlanningRowsAndRestoresStoredGroundState(t *testing.T) {
	data, err := os.ReadFile("../config/testdata/radis-ekbi.yaml")
	require.NoError(t, err)
	defs, err := config.NewAirportRegistry(config.ProductionAirportCatalog(), []config.AirportInput{{Source: "synthetic-only", Reader: bytes.NewReader(data)}})
	require.NoError(t, err)
	def, _ := defs.Lookup("EKBI")
	bound, err := airportworkflow.Bind(def, airportworkflow.RADISDescriptor())
	require.NoError(t, err)
	seen := time.Now().UTC()
	state := es.GroundStateLineup
	rows := []*models.Strip{{ID: 1, Session: 42, Callsign: "FEEDONLY", Origin: "EKBI", Bay: "DEP_HIDDEN", HasFP: true}, {ID: 2, Session: 42, Callsign: "LOCAL", Origin: "EKBI", Bay: "ACTIVE", BoardState: &models.StripBoardState{Local: true}}, {ID: 3, Session: 42, Callsign: "ASSOCIATED", Origin: "EKBI", Bay: "RUNWAY", HasFP: true, EuroscopeSeenAt: &seen, State: &state, BoardState: &models.StripBoardState{}}}
	repo := &testutil.MockStripRepository{ListFn: func(context.Context, int32) ([]*models.Strip, error) { return rows, nil }}
	hub := &Hub{server: &testutil.MockServer{StripRepoVal: repo}, airports: defs}
	client := startQueuedTestClient(&Client{send: make(chan events.OutgoingMessage, 4), workflow: bound, definition: def, session: 42, airport: "EKBI", user: shared.NewAuthenticatedUser("OWN", 0, nil)})
	require.True(t, hub.sendBackendSyncIfNeeded(client))
	for range 2 {
		_, ok := (<-client.send).(es.FsScratchPadEvent)
		require.True(t, ok)
	}
	event, ok := (<-client.send).(es.BackendSyncEvent)
	require.True(t, ok)
	require.Len(t, event.Strips, 1)
	require.Equal(t, "ASSOCIATED", event.Strips[0].Callsign)
	require.Equal(t, es.GroundStateLineup, event.Strips[0].GroundState)
}
