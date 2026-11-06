package services

import (
	"FlightStrips/internal/database"
	"FlightStrips/internal/models"
	"FlightStrips/internal/pdc/testdata"
	"FlightStrips/internal/repository/postgres"
	"FlightStrips/internal/sat"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestScopedStandInputsIsolateAirportSessionAndExpiry(t *testing.T) {
	pool, queries := testdata.SetupTestDB(t)
	inputs := map[string]sat.AirportInputs{}
	for _, airport := range []string{"AAAA", "BBBB"} {
		stands, err := sat.LoadStandCapabilities(strings.NewReader("STAND:" + airport + ":A1:N055.37.42.710:E012.38.33.450:30\nSTAND:" + airport + ":A2:N055.37.42.710:E012.38.33.451:30\n"))
		require.NoError(t, err)
		policy, err := sat.LoadAirlineAssignment(strings.NewReader(`{"rules":[],"stand_groups":{},"fallback_rules":{`+testFallbackJSON("A1")+`}}`), stands)
		require.NoError(t, err)
		inputs[airport] = sat.AirportInputs{Stands: stands, Policy: policy}
	}
	registry := sat.NewAirportInputRegistry(inputs)
	assignments := postgres.NewStandAssignmentRepository(pool)
	service, err := NewStandAllocationService(pool, postgres.NewStripRepository(pool), assignments, registry.Stands(), inputs["AAAA"].Policy, WithStandAirportInputs(registry))
	require.NoError(t, err)
	var sessions []int32
	for _, scope := range []struct{ airport, name string }{{"AAAA", "LIVE"}, {"BBBB", "LIVE"}, {"AAAA", "SWEATBOX_1"}} {
		_ = queries.InsertAirport(t.Context(), scope.airport)
		session, err := queries.InsertSession(t.Context(), database.InsertSessionParams{Airport: scope.airport, Name: scope.name})
		require.NoError(t, err)
		sessions = append(sessions, session)
		testdata.SeedTestStrip(t, queries, session, "SAS123")
		request := standAllocationRequest(session, "SAS123")
		request.Airport = scope.airport
		result, err := service.Allocate(t.Context(), request)
		require.NoError(t, err)
		require.Equal(t, "A1", result.Assignment.Stand)
	}
	wrong := standAllocationRequest(sessions[0], "SAS123")
	wrong.Airport = "BBBB"
	_, err = service.Allocate(t.Context(), wrong)
	require.ErrorContains(t, err, "does not match session")
	wrong.Airport = "EKCH"
	_, err = service.Preview(t.Context(), wrong)
	require.ErrorContains(t, err, "unavailable")
	past := time.Now().Add(-time.Minute)
	creator := "TEST"
	for _, session := range sessions {
		require.NoError(t, service.CreateManualBlock(t.Context(), map[bool]string{true: "BBBB", false: "AAAA"}[session == sessions[1]], &models.StandBlock{SessionID: session, Stand: "A2", BlockType: "MANUAL", CreatedBy: &creator, ExpiresAt: &past}))
	}
	require.NoError(t, service.ReleaseExpiredBlocks(t.Context(), sessions[0]))
	blocks, err := assignments.ListBlocks(t.Context(), sessions[0])
	require.NoError(t, err)
	require.Empty(t, blocks)
	for _, session := range sessions[1:] {
		blocks, err := assignments.ListBlocks(t.Context(), session)
		require.NoError(t, err)
		require.Len(t, blocks, 1)
		_, err = assignments.GetAssignment(t.Context(), session, "SAS123")
		require.NoError(t, err)
	}
}
