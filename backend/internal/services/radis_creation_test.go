package services

import (
	"FlightStrips/internal/database"
	"FlightStrips/internal/models"
	"FlightStrips/internal/pdc/testdata"
	"FlightStrips/internal/repository/postgres"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	"FlightStrips/internal/vatsim"
	fe "FlightStrips/pkg/events/frontend"
	pkg "FlightStrips/pkg/models"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"sync"
	"testing"
	"time"
)

type fixedRADISSource struct{ snapshot vatsim.Snapshot }

func (s fixedRADISSource) Snapshot() vatsim.Snapshot { return s.snapshot }

func TestLocalStripCreationAndConnectedOnlyFindProductionStorage(t *testing.T) {
	pool, q := testdata.SetupTestDB(t)
	ctx := t.Context()
	registry := radisWorkflowFixtures(t)
	for _, a := range []string{"EKBI", "EKYT"} {
		require.NoError(t, q.InsertAirport(ctx, a))
	}
	live, err := q.InsertSession(ctx, database.InsertSessionParams{Airport: "EKBI", Name: "LIVE"})
	require.NoError(t, err)
	training, err := q.InsertSession(ctx, database.InsertSessionParams{Airport: "EKBI", Name: "SWEATBOX"})
	require.NoError(t, err)
	repo := postgres.NewStripRepository(pool)
	sessions := postgres.NewSessionRepository(pool)
	s := NewStripService(repo, WithAirportWorkflows(registry), WithSessionReader(sessions), WithStripEventPublisher(&productionWorkflowPublisher{}), WithEuroscopeCommander(&testutil.MockEuroscopeHub{}))
	b := &testutil.MockCommandBinding{SessionID: live, CIDValue: "OWN"}
	own, err := shared.WithHumanCommand(ctx, b)
	require.NoError(t, err)
	bound, _ := registry.Lookup("EKBI")
	revision := bound.Definition().Revision()
	local := pkg.LocalStripCreation{Definition: revision, Callsign: " local1 ", Origin: "EKBI", Destination: "EKBI", Aircraft: "C172", Notes: "local paper"}
	require.NoError(t, s.CreateLocalStrip(own, live, "OWN", "EKBI", local))
	stored, err := repo.GetByCallsign(ctx, live, "LOCAL1")
	require.NoError(t, err)
	require.True(t, stored.IsManual)
	require.False(t, stored.HasFP)
	require.Nil(t, stored.EuroscopeSeenAt)
	require.True(t, stored.BoardState.Local)
	require.Error(t, s.CreateLocalStrip(own, live, "OWN", "EKBI", local))
	require.Error(t, s.CreateLocalStrip(ctx, live, "OWN", "EKBI", local))
	// Local paper movement still requires the own plugin, but has no ES effect.
	moving := WithWorkflowRevision(own, WorkflowRevision{StripID: stored.ID, Version: stored.Version, Definition: revision})
	require.NoError(t, s.MoveFrontendStrip(moving, live, "LOCAL1", "ACTIVE", "OWN", "EKBI", "TWR", false, false))
	require.Empty(t, b.Sent())
	require.Error(t, s.CreateManualFPL(own, live, fe.CreateManualFPLAction{Callsign: "LOCAL1"}, "OWN", "EKBI"))
	require.Error(t, s.CreateVFRFPL(own, live, fe.CreateVFRFPLAction{Callsign: "LOCAL1"}, "OWN"))
	source := vatsim.NewSyntheticSource()
	now := time.Now().UTC()
	s.SetConnectedFlightSource(source, time.Minute, time.Now)
	for i := range 24 {
		source.Upsert(vatsim.Flight{Callsign: fmt.Sprintf("FIND%02d", i), CID: fmt.Sprint(100 + i), State: vatsim.FlightStateOnline, Latitude: 40, Altitude: 30000, LogonTime: now, FlightPlan: vatsim.FlightPlan{Origin: "EKBI", Destination: "EHAM", AircraftShort: "C172", Revision: 3}})
	}
	source.Upsert(vatsim.Flight{Callsign: "ARRIVAL", CID: "200", State: vatsim.FlightStateOnline, LogonTime: now, FlightPlan: vatsim.FlightPlan{Origin: "EHAM", Destination: "EKBI"}})
	source.Upsert(vatsim.Flight{Callsign: "PREFILE", CID: "201", State: vatsim.FlightStatePrefile, FlightPlan: vatsim.FlightPlan{Origin: "EKBI", Destination: "EKYT"}})
	source.Upsert(vatsim.Flight{Callsign: "UNRELATED", CID: "202", State: vatsim.FlightStateOnline, FlightPlan: vatsim.FlightPlan{Origin: "EHAM", Destination: "EGLL"}})
	flights, err := s.FindConnectedFlights(own, live, "OWN", "EKBI", revision, "")
	require.NoError(t, err)
	require.Len(t, flights, 25, "no demo cap, distance or altitude limit applies to FIND")
	for _, f := range flights {
		require.NotEqual(t, "PREFILE", f.Callsign)
		require.NotEqual(t, "UNRELATED", f.Callsign)
	}
	selected := flights[1]
	// A prior planning row is admitted in place rather than duplicated.
	require.NoError(t, repo.Create(ctx, &models.Strip{Callsign: selected.Callsign, Session: live, Origin: "EKYT", Destination: "EKBI", Bay: "UNKNOWN", HasFP: true, AircraftType: func() *string { v := "OLD"; return &v }(), Route: func() *string { v := "DCT OLD"; return &v }()}))
	planning, err := repo.GetByCallsign(ctx, live, selected.Callsign)
	require.NoError(t, err)
	require.NoError(t, s.AdmitConnectedFlight(own, live, "OWN", "EKBI", revision, selected))
	admitted, err := repo.GetByCallsign(ctx, live, selected.Callsign)
	require.NoError(t, err)
	require.Equal(t, planning.ID, admitted.ID)
	require.NotNil(t, admitted.BoardState)
	require.False(t, admitted.HasFP, "FIND needs ES association before field commands")
	require.Equal(t, selected.Origin, admitted.Origin)
	require.Equal(t, selected.Destination, admitted.Destination)
	require.Equal(t, selected.Aircraft, *admitted.AircraftType)
	require.Equal(t, "", *admitted.Route)
	require.Equal(t, "PASSIVE_TOP", admitted.Bay)
	require.NoError(t, s.AdmitConnectedFlight(own, live, "OWN", "EKBI", revision, selected))
	after, err := repo.GetByCallsign(ctx, live, selected.Callsign)
	require.NoError(t, err)
	require.Equal(t, admitted.ID, after.ID)
	require.Equal(t, admitted.Version, after.Version)
	source.Remove(selected.Callsign)
	require.Error(t, s.AdmitConnectedFlight(own, live, "OWN", "EKBI", revision, selected))
	trainingBinding := &testutil.MockCommandBinding{SessionID: training, CIDValue: "OWN"}
	trainingCtx, err := shared.WithHumanCommand(ctx, trainingBinding)
	require.NoError(t, err)
	_, err = s.FindConnectedFlights(trainingCtx, training, "OWN", "EKBI", revision, "")
	require.ErrorContains(t, err, "environment")
	_, err = s.FindConnectedFlights(own, live, "OWN", "EKYT", revision, "")
	require.Error(t, err)
	stale := source.Snapshot()
	stale.Timestamp = now.Add(-time.Hour)
	s.SetConnectedFlightSource(fixedRADISSource{stale}, time.Minute, time.Now)
	_, err = s.FindConnectedFlights(own, live, "OWN", "EKBI", revision, "")
	require.ErrorContains(t, err, "stale")
	b.Unavailable.Store(true)
	_, err = s.FindConnectedFlights(own, live, "OWN", "EKBI", revision, "")
	require.Error(t, err)
	require.Error(t, s.CreateLocalStrip(own, live, "OWN", "EKBI", local))
	require.Error(t, s.AdmitConnectedFlight(own, live, "OWN", "EKBI", revision, flights[0]))
	require.Empty(t, b.Sent(), "NEW/FIND never creates or replaces an ES flight plan")
	// Concurrent NEW requests share the unique airport/session identity.
	b.Unavailable.Store(false)
	local.Callsign = "RACE1"
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); results <- s.CreateLocalStrip(own, live, "OWN", "EKBI", local) }()
	}
	wg.Wait()
	close(results)
	wins := 0
	for e := range results {
		if e == nil {
			wins++
		}
	}
	require.Equal(t, 1, wins)
}

func TestRADISAdmissionRevalidatesAfterSessionLockWait(t *testing.T) {
	pool, q := testdata.SetupTestDB(t)
	ctx := t.Context()
	registry := radisWorkflowFixtures(t)
	require.NoError(t, q.InsertAirport(ctx, "EKBI"))
	session, err := q.InsertSession(ctx, database.InsertSessionParams{Airport: "EKBI", Name: "LIVE"})
	require.NoError(t, err)
	repo := postgres.NewStripRepository(pool)
	service := NewStripService(repo, WithAirportWorkflows(registry), WithSessionReader(postgres.NewSessionRepository(pool)), WithStripEventPublisher(&productionWorkflowPublisher{}), WithEuroscopeCommander(&testutil.MockEuroscopeHub{}))
	binding := &testutil.MockCommandBinding{SessionID: session, CIDValue: "OWN"}
	own, err := shared.WithHumanCommand(ctx, binding)
	require.NoError(t, err)
	bound, _ := registry.Lookup("EKBI")
	revision := bound.Definition().Revision()
	blocked := func(action func() error, invalidate func()) {
		tx, err := pool.Begin(ctx)
		require.NoError(t, err)
		defer tx.Rollback(ctx)
		_, err = tx.Exec(ctx, "SELECT id FROM sessions WHERE id=$1 FOR UPDATE", session)
		require.NoError(t, err)
		done := make(chan error, 1)
		go func() { done <- action() }()
		require.Eventually(t, func() bool {
			var waiting int
			err := pool.QueryRow(ctx, "SELECT count(*) FROM pg_stat_activity WHERE wait_event_type='Lock' AND query LIKE 'SELECT id FROM sessions WHERE id=%'").Scan(&waiting)
			return err == nil && waiting > 0
		}, 3*time.Second, 10*time.Millisecond)
		invalidate()
		require.NoError(t, tx.Commit(ctx))
		select {
		case err := <-done:
			require.Error(t, err)
		case <-time.After(3 * time.Second):
			t.Fatal("admission failed to finish")
		}
	}
	blocked(func() error {
		return service.CreateLocalStrip(own, session, "OWN", "EKBI", pkg.LocalStripCreation{Definition: revision, Callsign: "BLOCKEDNEW", Origin: "EKBI", Destination: "EKBI"})
	}, func() { binding.Unavailable.Store(true) })
	_, err = repo.GetByCallsign(ctx, session, "BLOCKEDNEW")
	require.ErrorIs(t, err, pgx.ErrNoRows)
	binding.Unavailable.Store(false)
	source := vatsim.NewSyntheticSource()
	source.Upsert(vatsim.Flight{Callsign: "BLOCKEDFIND", CID: "1", State: vatsim.FlightStateOnline, LogonTime: time.Now(), FlightPlan: vatsim.FlightPlan{Origin: "EKBI", Destination: "EKYT", Revision: 1}})
	service.SetConnectedFlightSource(source, time.Minute, time.Now)
	candidates, err := service.FindConnectedFlights(own, session, "OWN", "EKBI", revision, "")
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	blocked(func() error {
		return service.AdmitConnectedFlight(own, session, "OWN", "EKBI", revision, candidates[0])
	}, func() { source.Remove("BLOCKEDFIND") })
	_, err = repo.GetByCallsign(ctx, session, "BLOCKEDFIND")
	require.ErrorIs(t, err, pgx.ErrNoRows)
}
