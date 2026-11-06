package services

import (
	"FlightStrips/internal/airportworkflow"
	"FlightStrips/internal/config"
	"FlightStrips/internal/database"
	"FlightStrips/internal/models"
	"FlightStrips/internal/pdc/testdata"
	"FlightStrips/internal/repository/postgres"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	es "FlightStrips/pkg/events/euroscope"
	pkg "FlightStrips/pkg/models"
	"bytes"
	"github.com/stretchr/testify/require"
	"os"
	"sync"
	"testing"
)

func radisWorkflowFixtures(t *testing.T) *airportworkflow.Registry {
	t.Helper()
	inputs := []config.AirportInput{}
	for _, airport := range []string{"ekbi", "ekyt"} {
		data, err := os.ReadFile("internal/config/testdata/radis-" + airport + ".yaml")
		require.NoError(t, err)
		inputs = append(inputs, config.AirportInput{Source: "synthetic-only-" + airport, Reader: bytes.NewReader(data)})
	}
	defs, err := config.NewAirportRegistry(config.ProductionAirportCatalog(), inputs)
	require.NoError(t, err)
	registry, err := airportworkflow.NewRegistry(defs, airportworkflow.RADISDescriptor())
	require.NoError(t, err)
	return registry
}

func TestRADISProductionSharedStateOwnAuthorityAndIsolation(t *testing.T) {
	pool, q := testdata.SetupTestDB(t)
	ctx := t.Context()
	registry := radisWorkflowFixtures(t)
	repo := postgres.NewStripRepository(pool)
	sessions := postgres.NewSessionRepository(pool)
	pub := &productionWorkflowPublisher{}
	s := NewStripService(repo, WithAirportWorkflows(registry), WithSessionReader(sessions), WithStripEventPublisher(pub), WithControllerReader(postgres.NewControllerRepository(pool)), WithSectorOwnerRepository(postgres.NewSectorOwnerRepository(pool)), WithCoordinationStore(postgres.NewCoordinationRepository(pool)), WithEuroscopeCommander(&testutil.MockEuroscopeHub{}))
	ids := []int32{}
	for _, scope := range []struct{ airport, env string }{{"EKBI", "LIVE"}, {"EKYT", "LIVE"}, {"EKBI", "SWEATBOX"}} {
		if scope.env == "LIVE" {
			require.NoError(t, q.InsertAirport(ctx, scope.airport))
		}
		id, err := q.InsertSession(ctx, database.InsertSessionParams{Airport: scope.airport, Name: scope.env})
		require.NoError(t, err)
		ids = append(ids, id)
		event := es.Strip{Callsign: "SAME123", Origin: "EKBI", Destination: "EKYT", HasFp: true, GroundState: es.GroundStateTaxi, Position: &es.Position{Lat: 0, Lon: 0, Altitude: 100}}
		require.NoError(t, s.SyncStrip(ctx, id, "MASTER", event, scope.airport))
	}
	live := ids[0]
	bound, _ := registry.Lookup("EKBI")
	read := func() *models.Strip {
		v, err := repo.GetByCallsign(ctx, live, "SAME123")
		require.NoError(t, err)
		return v
	}
	revision := func(v *models.Strip) pkg.StripRevision {
		return pkg.StripRevision{StripID: v.ID, Version: v.Version, Definition: bound.Definition().Revision()}
	}
	one := &testutil.MockCommandBinding{SessionID: live, CIDValue: "ONE"}
	two := &testutil.MockCommandBinding{SessionID: live, CIDValue: "TWO"}
	oneCtx, err := shared.WithHumanCommand(ctx, one)
	require.NoError(t, err)
	twoCtx, err := shared.WithHumanCommand(ctx, two)
	require.NoError(t, err)
	original := read()
	require.Nil(t, original.Owner)
	require.NoError(t, s.AnnotateStrip(oneCtx, live, "SAME123", "ONE", "EKBI", "", revision(original), airportworkflow.StripAnnotation{Field: "notes", Value: "shared paper note"}))
	current := read()
	require.Equal(t, "shared paper note", current.BoardState.Annotations.Notes)
	require.Error(t, s.AnnotateStrip(twoCtx, live, "SAME123", "TWO", "EKBI", "", revision(original), airportworkflow.StripAnnotation{Field: "notes", Value: "stale overwrite"}))
	for _, edit := range []airportworkflow.StripAnnotation{{Field: "deicing", Enabled: true}, {Field: "rea", Enabled: true}, {Field: "ata", Enabled: true}, {Field: "rnav", Value: "NON"}, {Field: "vfr", Enabled: true}, {Field: "atd", Value: "1234"}, {Field: "endurance_pob", Value: "3.5/4"}, {Field: "qnh", Value: "Q0999"}} {
		require.NoError(t, s.AnnotateStrip(twoCtx, live, "SAME123", "TWO", "EKBI", "", revision(read()), edit))
	}
	current = read()
	require.True(t, current.BoardState.Annotations.Deicing)
	require.NotNil(t, current.BoardState.Annotations.REA)
	require.NotNil(t, current.BoardState.Annotations.ATA)
	// A second service instance reads identical durable state (process restart).
	otherRepo := postgres.NewStripRepository(pool)
	afterRestart, err := otherRepo.GetByCallsign(ctx, live, "SAME123")
	require.NoError(t, err)
	require.Equal(t, current.BoardState, afterRestart.BoardState)
	for _, id := range ids[1:] {
		v, err := repo.GetByCallsign(ctx, id, "SAME123")
		require.NoError(t, err)
		require.Empty(t, v.BoardState.Annotations.Notes)
		require.False(t, v.BoardState.Annotations.Deicing)
	}
	one.Unavailable.Store(true)
	require.Error(t, s.AnnotateStrip(oneCtx, live, "SAME123", "ONE", "EKBI", "", revision(read()), airportworkflow.StripAnnotation{Field: "notes", Value: "offline"}))
	require.Error(t, s.AnnotateStrip(ctx, live, "SAME123", "TWO", "EKBI", "", revision(read()), airportworkflow.StripAnnotation{Field: "notes", Value: "unbound"}))
	// Another participant remains writable, without acquiring strip ownership.
	require.NoError(t, s.MoveFrontendStrip(WithWorkflowRevision(twoCtx, WorkflowRevision{StripID: read().ID, Version: read().Version, Definition: bound.Definition().Revision()}), live, "SAME123", "RUNWAY", "TWO", "EKBI", "TWR", false, false))
	require.Equal(t, []es.OutgoingMessage{es.GroundStateEvent{Callsign: "SAME123", GroundState: es.GroundStateLineup}}, two.Sent())
	require.Empty(t, one.Sent())
	// Full and incremental ES reconciliation preserve controller placement and annotations.
	require.NoError(t, s.SyncStrip(ctx, live, "MASTER", es.Strip{Callsign: "SAME123", Origin: "EKBI", Destination: "EKYT", HasFp: true, GroundState: es.GroundStateTaxi, Position: &es.Position{Lat: 0, Lon: 0, Altitude: 100}}, "EKBI"))
	require.NoError(t, s.UpdateGroundState(ctx, live, "SAME123", es.GroundStatePush, "EKBI"))
	require.Equal(t, "RUNWAY", read().Bay)
	require.Equal(t, current.BoardState, read().BoardState)

	// Ordering is serialized in the DB and revision checked; another session is untouched.
	require.NoError(t, s.SyncStrip(ctx, live, "MASTER", es.Strip{Callsign: "ORDER2", Origin: "EKBI", Destination: "EKYT", HasFp: true, Position: &es.Position{}}, "EKBI"))
	orderCtx := WithWorkflowRevision(twoCtx, WorkflowRevision{StripID: read().ID, Version: read().Version, Definition: bound.Definition().Revision()})
	require.NoError(t, s.MoveStripBetween(orderCtx, live, "SAME123", nil, "RUNWAY"))
	require.Error(t, s.MoveStripBetween(ctx, live, "SAME123", nil, "RUNWAY"))
	// Two writers with the same observed version cannot silently overwrite each other.
	before := read()
	rev := revision(before)
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, note := range []string{"first", "second"} {
		wg.Add(1)
		go func(note string) {
			defer wg.Done()
			results <- s.AnnotateStrip(twoCtx, live, "SAME123", "TWO", "EKBI", "", rev, airportworkflow.StripAnnotation{Field: "notes", Value: note})
		}(note)
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

func TestRADISFeedFirstFlightAdmitsExistingPlanningIdentity(t *testing.T) {
	pool, q := testdata.SetupTestDB(t)
	ctx := t.Context()
	workflows := radisWorkflowFixtures(t)
	require.NoError(t, q.InsertAirport(ctx, "EKBI"))
	session, err := q.InsertSession(ctx, database.InsertSessionParams{Airport: "EKBI", Name: "LIVE"})
	require.NoError(t, err)
	repo := postgres.NewStripRepository(pool)
	require.NoError(t, repo.Create(ctx, &models.Strip{Callsign: "FEEDFIRST", Session: session, Origin: "EKBI", Destination: "EKYT", Bay: "DEP_HIDDEN", HasFP: true}))
	before, err := repo.GetByCallsign(ctx, session, "FEEDFIRST")
	require.NoError(t, err)
	service := NewStripService(repo, WithAirportWorkflows(workflows), WithSessionReader(postgres.NewSessionRepository(pool)), WithStripEventPublisher(&productionWorkflowPublisher{}), WithControllerReader(postgres.NewControllerRepository(pool)), WithSectorOwnerRepository(postgres.NewSectorOwnerRepository(pool)), WithCoordinationStore(postgres.NewCoordinationRepository(pool)), WithEuroscopeCommander(&testutil.MockEuroscopeHub{}))
	require.NoError(t, service.SyncStrip(ctx, session, "MASTER", es.Strip{Callsign: "FEEDFIRST", Origin: "EKBI", Destination: "EKYT", HasFp: true, GroundState: es.GroundStateTaxi, Position: &es.Position{Lat: 0, Lon: 0, Altitude: 100}}, "EKBI"))
	after, err := repo.GetByCallsign(ctx, session, "FEEDFIRST")
	require.NoError(t, err)
	require.Equal(t, before.ID, after.ID)
	require.Equal(t, "PASSIVE_TOP", after.Bay)
	require.NotNil(t, after.BoardState)
	require.NotNil(t, after.EuroscopeSeenAt)
	changed, err := repo.UpdatePlanningBayAndSequence(ctx, session, "FEEDFIRST", "DEP_HIDDEN", 9000)
	require.NoError(t, err)
	require.Zero(t, changed)
	current, err := repo.GetByCallsign(ctx, session, "FEEDFIRST")
	require.NoError(t, err)
	require.Equal(t, "PASSIVE_TOP", current.Bay)
}
