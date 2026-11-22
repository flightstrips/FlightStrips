package services

import (
	"FlightStrips/internal/database"
	"FlightStrips/internal/models"
	"FlightStrips/internal/pdc/testdata"
	"FlightStrips/internal/repository/postgres"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	es "FlightStrips/pkg/events/euroscope"
	"context"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestFlightPlanEETOnlyObservationPersistsAndMissingValueRetainsKnownFact(t *testing.T) {
	pool, q := testdata.SetupTestDB(t)
	ctx := t.Context()
	require.NoError(t, q.InsertAirport(ctx, "EKBI"))
	session, err := q.InsertSession(ctx, database.InsertSessionParams{Airport: "EKBI", Name: "LIVE"})
	require.NoError(t, err)
	repo := postgres.NewStripRepository(pool)
	service := NewStripService(repo, WithAirportWorkflows(radisWorkflowFixtures(t)), WithSessionReader(postgres.NewSessionRepository(pool)), WithStripEventPublisher(&productionWorkflowPublisher{}), WithControllerReader(postgres.NewControllerRepository(pool)), WithSectorOwnerRepository(postgres.NewSectorOwnerRepository(pool)), WithCoordinationStore(postgres.NewCoordinationRepository(pool)), WithEuroscopeCommander(&testutil.MockEuroscopeHub{}))
	observer := &syncRevisionObserver{}
	service.SetEuroScopeAMANStripObserver(observer)
	event := es.Strip{Callsign: "EETONLY", Origin: "EKBI", Destination: "EKYT", HasFp: true, EnrouteDuration: "0100", GroundState: es.GroundStateTaxi, Position: &es.Position{Lat: 0, Lon: 0, Altitude: 100}}
	require.NoError(t, service.SyncStrip(ctx, session, "MASTER", event, "EKBI"))
	before, err := repo.GetByCallsign(ctx, session, event.Callsign)
	require.NoError(t, err)
	require.Equal(t, "0100", before.FlightPlanEET)
	require.Len(t, observer.strips, 1)
	require.Equal(t, before.ID, observer.strips[0].ID)
	require.Equal(t, before.Version, observer.strips[0].Version)

	// The full-sync cache can miss an intervening write. The UPDATE must
	// return its actual revision rather than incrementing the cached one.
	rows, err := repo.UpdateClearedAltitude(ctx, session, event.Callsign, before.ClearedAltitude, nil)
	require.NoError(t, err)
	require.EqualValues(t, 1, rows)
	cachedBefore := *before
	batch := &shared.SyncState{ExistingStrips: map[string]*models.Strip{event.Callsign: &cachedBefore}}
	batchCtx := shared.WithSyncState(ctx, batch)
	event.EnrouteDuration = "0130"
	require.NoError(t, service.SyncStrip(batchCtx, session, "MASTER", event, "EKBI"))
	after, err := repo.GetByCallsign(ctx, session, event.Callsign)
	require.NoError(t, err)
	require.Equal(t, "0130", after.FlightPlanEET)
	require.Greater(t, after.Version, before.Version+1)
	queued := batch.AMANStrips[event.Callsign].Strip
	require.NotNil(t, queued)
	require.Equal(t, after.ID, queued.ID)
	require.Equal(t, after.Version, queued.Version)
	require.Equal(t, after.Version, batch.ExistingStrips[event.Callsign].Version)
	require.NoError(t, service.FlushEuroScopeAMANStrips(batchCtx, session))
	require.Equal(t, after.Version, observer.strips[1].Version)
	cached := *before
	applySyncStripUpdate(&cached, after)
	require.Equal(t, "0130", cached.FlightPlanEET)
	event.EnrouteDuration = ""
	require.NoError(t, service.SyncStrip(ctx, session, "MASTER", event, "EKBI"))
	retained, err := repo.GetByCallsign(ctx, session, event.Callsign)
	require.NoError(t, err)
	require.Equal(t, "0130", retained.FlightPlanEET)
	require.Equal(t, after.Version, retained.Version)
	require.Equal(t, retained.ID, observer.strips[2].ID)
	require.Equal(t, retained.Version, observer.strips[2].Version)
}

type syncRevisionObserver struct{ amanStripObserverSpy }

func (o *syncRevisionObserver) ObserveEuroScopeStrips(_ context.Context, _ int32, observations []shared.EuroScopeStripObservation) error {
	for _, observation := range observations {
		o.strips = append(o.strips, observation.Strip)
	}
	return nil
}

func TestFullSyncPreservesNewerTrackerClearance(t *testing.T) {
	for _, scenario := range []struct {
		name            string
		holding         bool
		newLevel        int32
		incomingLevel   int32
		sender          string
		unrelatedUpdate bool
	}{
		{name: "held arrival receives a newer level", holding: true, newLevel: 7000, incomingLevel: 9000},
		{name: "nonholding arrival receives a newer clear", incomingLevel: 9000},
		{name: "tracker reasserts same level outside a hold", newLevel: 9000, incomingLevel: 7000},
		{name: "tracking sender reasserts same level", newLevel: 9000, incomingLevel: 7000, sender: "EKCH_APP"},
		{name: "tracking sender sync after unrelated edit", newLevel: 7000, incomingLevel: 7000, sender: "EKCH_APP", unrelatedUpdate: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			pool, queries := testdata.SetupTestDB(t)
			ctx := t.Context()
			session := testdata.SeedTestSession(t, queries)
			repo := postgres.NewStripRepository(pool)
			service := NewStripService(repo, WithAirportWorkflows(testWorkflows), WithSessionReader(postgres.NewSessionRepository(pool)), WithStripEventPublisher(&productionWorkflowPublisher{}), WithControllerReader(postgres.NewControllerRepository(pool)), WithSectorOwnerRepository(postgres.NewSectorOwnerRepository(pool)), WithCoordinationStore(postgres.NewCoordinationRepository(pool)), WithEuroscopeCommander(&testutil.MockEuroscopeHub{}))
			observer := &syncRevisionObserver{}
			service.SetEuroScopeAMANStripObserver(observer)
			oldLevel := int32(9000)
			strip := &models.Strip{
				Session: session, Callsign: "SAS1198", Origin: "ESSA", Destination: "EKCH", Bay: "ARR_HIDDEN",
				TrackingController: "EKCH_APP", ClearedAltitude: &oldLevel, HasFP: true,
			}
			if scenario.holding {
				strip.Hold, strip.HoldType, strip.HoldEat = "OLPIB", "enroute", "1422"
			}
			require.NoError(t, repo.Create(ctx, strip))
			cached, err := repo.GetByCallsign(ctx, session, strip.Callsign)
			require.NoError(t, err)
			if scenario.unrelatedUpdate {
				_, err = pool.Exec(ctx, "UPDATE strips SET remarks = 'unrelated edit', version = version + 1 WHERE id = $1", cached.ID)
				require.NoError(t, err)
			} else {
				rows, err := repo.UpdateClearedAltitude(ctx, session, strip.Callsign, &scenario.newLevel, nil)
				require.NoError(t, err)
				require.EqualValues(t, 1, rows)
			}
			if scenario.holding {
				rows, err := repo.UpdateHold(ctx, session, strip.Callsign, "TESPI", "enroute", "1431", nil)
				require.NoError(t, err)
				require.EqualValues(t, 1, rows)
			}
			current, err := repo.GetByCallsign(ctx, session, strip.Callsign)
			require.NoError(t, err)
			require.Greater(t, current.Version, cached.Version)

			batch := &shared.SyncState{ExistingStrips: map[string]*models.Strip{strip.Callsign: cached}}
			sender := scenario.sender
			if sender == "" {
				sender = "EKCH_FMP"
			}
			batchCtx := shared.WithStripSyncController(shared.WithSyncState(ctx, batch), sender)
			incoming := es.Strip{
				Callsign: strip.Callsign, Origin: strip.Origin, Destination: strip.Destination, HasFp: true,
				TrackingController: strip.TrackingController, ClearedAltitude: scenario.incomingLevel, Remarks: "new full-sync remark",
			}
			require.NoError(t, service.SyncStrip(batchCtx, session, "MASTER", incoming, "EKCH"))
			updated, err := repo.GetByCallsign(ctx, session, strip.Callsign)
			require.NoError(t, err)
			require.Greater(t, updated.Version, current.Version)
			require.Equal(t, &incoming.Remarks, updated.Remarks)
			require.Equal(t, &scenario.newLevel, updated.ClearedAltitude)
			require.Equal(t, current.Hold, updated.Hold)
			require.Equal(t, current.HoldEat, updated.HoldEat)
			require.NoError(t, service.FlushEuroScopeAMANStrips(batchCtx, session))
			require.Len(t, observer.strips, 1)
			require.Equal(t, updated.Version, observer.strips[0].Version)
			require.Equal(t, updated.ClearedAltitude, observer.strips[0].ClearedAltitude)
			require.Equal(t, updated.Hold, observer.strips[0].Hold)
			if scenario.sender == "EKCH_APP" && !scenario.unrelatedUpdate {
				// The dedicated-write watermark only protects older bases. A fresh
				// tracking sync must still be able to issue the next clearance.
				require.NoError(t, service.SyncStrip(batchCtx, session, "MASTER", incoming, "EKCH"))
				fresh, err := repo.GetByCallsign(ctx, session, strip.Callsign)
				require.NoError(t, err)
				require.Equal(t, &incoming.ClearedAltitude, fresh.ClearedAltitude)
			}
		})
	}
}

func TestFullSyncQueuesPersistedReplacementAfterWriteConflict(t *testing.T) {
	for _, change := range []string{"remarks", "bay", "restart"} {
		t.Run(change, func(t *testing.T) {
			pool, queries := testdata.SetupTestDB(t)
			ctx := t.Context()
			require.NoError(t, queries.InsertAirport(ctx, "EKCH"))
			session, err := queries.InsertSession(ctx, database.InsertSessionParams{Airport: "EKCH", Name: "LIVE"})
			require.NoError(t, err)
			repo := postgres.NewStripRepository(pool)
			appendStore := &observedAppendStore{StripLifecycleStore: repo, appendStrip: repo.PersistAtEndOfBay}
			service := NewStripService(repo, WithAirportWorkflows(testWorkflows), WithSessionReader(postgres.NewSessionRepository(pool)), WithStripEventPublisher(&productionWorkflowPublisher{}), WithControllerReader(postgres.NewControllerRepository(pool)), WithSectorOwnerRepository(postgres.NewSectorOwnerRepository(pool)), WithCoordinationStore(postgres.NewCoordinationRepository(pool)), WithEuroscopeCommander(&testutil.MockEuroscopeHub{}))
			service.lifecycleStore = appendStore
			observer := &syncRevisionObserver{}
			service.SetEuroScopeAMANStripObserver(observer)
			event := es.Strip{Callsign: "REPLACED", Origin: "EKCH", Destination: "ESSA", HasFp: true, GroundState: es.GroundStateTaxi, Position: &es.Position{Lat: 55.6, Lon: 12.65, Altitude: 0}}
			require.NoError(t, service.SyncStrip(ctx, session, "MASTER", event, "EKCH"))
			cached, err := repo.GetByCallsign(ctx, session, event.Callsign)
			require.NoError(t, err)
			if change != "remarks" {
				cached.Bay = "STAND"
			}
			if change == "restart" {
				cached.Origin, cached.Destination = "EGLL", "EKCH"
			}
			require.NoError(t, repo.Delete(ctx, session, event.Callsign))

			replacement := *cached
			replacement.Remarks = new(string)
			*replacement.Remarks = "replacement strip"
			require.NoError(t, repo.Create(ctx, &replacement))
			require.NotEqual(t, cached.ID, replacement.ID)

			event.Remarks = "obsolete full-sync remark"
			previousAppends := appendStore.calls
			batch := &shared.SyncState{ExistingStrips: map[string]*models.Strip{event.Callsign: cached}}
			batchCtx := shared.WithSyncState(ctx, batch)
			require.NoError(t, service.SyncStrip(batchCtx, session, "MASTER", event, "EKCH"))
			require.NoError(t, service.FlushEuroScopeAMANStrips(batchCtx, session))
			require.Len(t, observer.strips, 2, "the replacement snapshot must still contain the callsign")
			require.Equal(t, replacement.ID, observer.strips[1].ID)
			require.Equal(t, replacement.Version, observer.strips[1].Version)
			require.Equal(t, replacement.Remarks, observer.strips[1].Remarks)
			persisted, err := repo.GetByCallsign(ctx, session, event.Callsign)
			require.NoError(t, err)
			require.Equal(t, replacement.Version, persisted.Version)
			require.Equal(t, replacement.Remarks, persisted.Remarks)
			if change != "remarks" {
				require.Greater(t, appendStore.calls, previousAppends, "exercise the transactional append conflict")
			}
		})
	}
}

type observedAppendStore struct {
	StripLifecycleStore
	appendStrip func(context.Context, *models.Strip, bool, int32) error
	calls       int
}

func (s *observedAppendStore) PersistAtEndOfBay(ctx context.Context, strip *models.Strip, create bool, spacing int32) error {
	s.calls++
	return s.appendStrip(ctx, strip, create, spacing)
}

func TestFlightPlanEETDoesNotCarryIntoRestartedFlightLifecycle(t *testing.T) {
	pool, q := testdata.SetupTestDB(t)
	ctx := t.Context()
	require.NoError(t, q.InsertAirport(ctx, "EKCH"))
	session, err := q.InsertSession(ctx, database.InsertSessionParams{Airport: "EKCH", Name: "LIVE"})
	require.NoError(t, err)
	repo := postgres.NewStripRepository(pool)
	oldCFL := int32(9000)
	require.NoError(t, repo.Create(ctx, &models.Strip{Callsign: "TURNAROUND", Session: session, Origin: "EGLL", Destination: "EKCH", Bay: "STAND", HasFP: true, FlightPlanEET: "0130", Hold: "OLPIB", HoldType: "enroute", TrackingController: "EKCH_APP", ClearedAltitude: &oldCFL}))
	service := NewStripService(repo, WithAirportWorkflows(testWorkflows), WithSessionReader(postgres.NewSessionRepository(pool)), WithStripEventPublisher(&productionWorkflowPublisher{}), WithControllerReader(postgres.NewControllerRepository(pool)), WithSectorOwnerRepository(postgres.NewSectorOwnerRepository(pool)), WithCoordinationStore(postgres.NewCoordinationRepository(pool)), WithEuroscopeCommander(&testutil.MockEuroscopeHub{}))
	event := es.Strip{Callsign: "TURNAROUND", Origin: "EKCH", Destination: "ESSA", HasFp: true, GroundState: es.GroundStateTaxi, Position: &es.Position{Lat: 55.6, Lon: 12.65, Altitude: 0}}
	ctx = shared.WithStripSyncController(ctx, "EKCH_FMP")
	cached, err := repo.GetByCallsign(ctx, session, event.Callsign)
	require.NoError(t, err)
	remark := "intervening unrelated update"
	_, err = pool.Exec(ctx, "UPDATE strips SET remarks = $1, version = version + 1 WHERE id = $2", remark, cached.ID)
	require.NoError(t, err)
	rows, err := repo.UpdateClearedAltitude(ctx, session, event.Callsign, &oldCFL, nil)
	require.NoError(t, err)
	require.EqualValues(t, 1, rows)
	ctx = shared.WithSyncState(ctx, &shared.SyncState{ExistingStrips: map[string]*models.Strip{event.Callsign: cached}})
	require.NoError(t, service.SyncStrip(ctx, session, "MASTER", event, "EKCH"))
	restarted, err := repo.GetByCallsign(ctx, session, event.Callsign)
	require.NoError(t, err)
	require.Equal(t, "EKCH", restarted.Origin)
	require.Equal(t, "ESSA", restarted.Destination)
	require.Empty(t, restarted.FlightPlanEET)
	require.NotNil(t, restarted.ClearedAltitude)
	require.Zero(t, *restarted.ClearedAltitude, "the restarted flight must receive the lifecycle CFL reset")
}
