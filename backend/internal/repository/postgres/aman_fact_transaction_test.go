package postgres

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/coordinationrequest"
	"FlightStrips/internal/models"
	"FlightStrips/internal/pdc/testdata"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func factTransactionFixture(t *testing.T) (*amanRepository, *pgxpool.Pool, context.Context, aman.AirportState) {
	t.Helper()
	pool, queries := testdata.SetupTestDB(t)
	id := testdata.SeedTestSession(t, queries)
	ctx := aman.WithSession(t.Context(), id)
	repo := NewAMANRepository(pool)
	state := amanState(1, "unused", "SAS123")
	state.SessionID = id
	_, err := repo.Commit(ctx, aman.StateCommit{State: state})
	require.NoError(t, err)
	strips := NewStripRepository(pool)
	for _, callsign := range []string{"SAS123", "SAS124"} {
		require.NoError(t, strips.Create(ctx, &models.Strip{Session: id, Callsign: callsign, TrackingController: "EKCH_A_APP"}))
	}
	ctx, state, err = repo.LoadRouteFactState(ctx, "EKCH", "SAS123", "EKCH_A_APP")
	require.NoError(t, err)
	return repo, pool, ctx, state
}

func nextFactTransactionState(t *testing.T, state aman.AirportState) aman.AirportState {
	t.Helper()
	raw, err := json.Marshal(state)
	require.NoError(t, err)
	var next aman.AirportState
	require.NoError(t, json.Unmarshal(raw, &next))
	next.Revision++
	next.GeneratedAt = next.GeneratedAt.Add(time.Second)
	next.Flights[0].Slot.Revision = next.Revision
	next.Flights[0].ActiveRouteFact.ID = "transaction-direct"
	return next
}

func TestFactTransactionRejectsHandoffBeforeWriteAndReplay(t *testing.T) {
	repo, pool, ctx, state := factTransactionFixture(t)
	_, err := pool.Exec(ctx, `UPDATE strips SET tracking_controller='EKCH_B_APP' WHERE session=$1 AND callsign='SAS123'`, state.SessionID)
	require.NoError(t, err)
	err = repo.ValidateRouteFact(ctx, state.Revision, "EKCH", "SAS123", "EKCH_A_APP")
	requireDomainErrorClass(t, err, aman.ErrorUnauthorized)
	for _, next := range []aman.AirportState{nextFactTransactionState(t, state), state} {
		_, err = repo.CommitRouteFact(ctx, aman.StateCommit{ExpectedRevision: state.Revision, State: next}, "SAS123", "EKCH_A_APP")
		requireDomainErrorClass(t, err, aman.ErrorUnauthorized)
	}
	loaded, err := NewAMANRepository(pool).LoadAirportState(ctx, "EKCH")
	require.NoError(t, err)
	require.Equal(t, state, loaded)
}

func TestFactTransactionCASRejectsConcurrentOrReplacedPayload(t *testing.T) {
	for _, tamper := range []bool{false, true} {
		t.Run(map[bool]string{false: "revision", true: "same_revision_payload"}[tamper], func(t *testing.T) {
			repo, pool, ctx, state := factTransactionFixture(t)
			otherCtx, _, err := repo.LoadRouteFactState(ctx, "EKCH", "SAS123", "EKCH_A_APP")
			require.NoError(t, err)
			next := nextFactTransactionState(t, state)
			if tamper {
				_, err = pool.Exec(ctx, `UPDATE aman_session_states SET payload=jsonb_set(payload,'{PolicyVersion}','"external-policy"'::jsonb) WHERE session_id=$1`, state.SessionID)
				require.NoError(t, err)
			} else {
				_, err = repo.CommitRouteFact(otherCtx, aman.StateCommit{ExpectedRevision: state.Revision, State: next}, "SAS123", "EKCH_A_APP")
				require.NoError(t, err)
			}
			_, err = repo.CommitRouteFact(ctx, aman.StateCommit{ExpectedRevision: state.Revision, State: next}, "SAS123", "EKCH_A_APP")
			requireDomainErrorClass(t, err, aman.ErrorRevisionConflict)
			_, ok := aman.ReadSessionSnapshot(ctx)
			require.False(t, ok, "a failed CAS invalidates the receipt used by publication")
			loaded, err := repo.LoadAirportState(ctx, "EKCH")
			require.NoError(t, err)
			if tamper {
				require.Equal(t, "external-policy", loaded.PolicyVersion)
			} else {
				require.Equal(t, next, loaded)
			}
		})
	}
}

func TestFactTransactionRejectsDeletedSession(t *testing.T) {
	repo, pool, ctx, state := factTransactionFixture(t)
	_, err := pool.Exec(ctx, `DELETE FROM sessions WHERE id=$1`, state.SessionID)
	require.NoError(t, err)
	_, err = repo.CommitRouteFact(ctx, aman.StateCommit{ExpectedRevision: state.Revision, State: nextFactTransactionState(t, state)}, "SAS123", "EKCH_A_APP")
	requireDomainErrorClass(t, err, aman.ErrorUnauthorized)
}

func TestFactTransactionAuditBatchAndAtomicRollback(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "all_records", true: "rollback"}[fail], func(t *testing.T) {
			repo, pool, ctx, state := factTransactionFixture(t)
			next := nextFactTransactionState(t, state)
			records := make([]aman.AuditRecord, 30)
			for i := range records {
				records[i] = aman.AuditRecord{Airport: "EKCH", Revision: next.Revision, Category: "aman.prediction", Payload: []byte{byte(i)}, RecordedAt: next.GeneratedAt}
			}
			if fail {
				_, err := pool.Exec(ctx, `CREATE FUNCTION reject_fact_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'audit failure'; END $$`)
				require.NoError(t, err)
				_, err = pool.Exec(ctx, `CREATE TRIGGER reject_fact_audit BEFORE INSERT ON aman_session_audit_records FOR EACH ROW EXECUTE FUNCTION reject_fact_audit()`)
				require.NoError(t, err)
			}
			_, err := repo.CommitRouteFact(ctx, aman.StateCommit{ExpectedRevision: state.Revision, State: next, AuditRecords: records}, "SAS123", "EKCH_A_APP")
			if fail {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			loaded, err := NewAMANRepository(pool).LoadAirportState(ctx, "EKCH")
			require.NoError(t, err)
			stored, err := repo.ListAuditRecords(ctx, "EKCH")
			require.NoError(t, err)
			if fail {
				require.Equal(t, state, loaded)
				require.Empty(t, stored)
			} else {
				require.Equal(t, next, loaded)
				require.Equal(t, records, stored)
			}
		})
	}
}

func TestFactTransactionDoesNotWaitHoldingStripLocks(t *testing.T) {
	repo, pool, ctx, state := factTransactionFixture(t)
	blocker, err := pool.Begin(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = blocker.Rollback(context.Background()) })
	_, err = blocker.Exec(ctx, `SELECT revision FROM aman_session_states WHERE session_id=$1 AND airport='EKCH' FOR NO KEY UPDATE`, state.SessionID)
	require.NoError(t, err)
	deadline, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	started := time.Now()
	_, err = repo.CommitRouteFact(deadline, aman.StateCommit{ExpectedRevision: state.Revision, State: nextFactTransactionState(t, state)}, "SAS123", "EKCH_A_APP")
	requireDomainErrorClass(t, err, aman.ErrorRevisionConflict)
	require.Less(t, time.Since(started), 500*time.Millisecond, "NOWAIT must release the session lock immediately")
	_, err = pool.Exec(deadline, `UPDATE strips SET position_altitude=10000 WHERE session=$1 AND callsign='SAS123'`, state.SessionID)
	require.NoError(t, err, "an aggregate conflict must leave no tracking lock behind")
	other := aman.WithSession(ctx, state.SessionID+1)
	_, ok := aman.ReadSessionSnapshot(other)
	require.False(t, ok, "a receipt is never usable by another session")
}

func TestFactTransactionLocksOnlyTargetAndReleasesBeforePrediction(t *testing.T) {
	repo, pool, ctx, state := factTransactionFixture(t)
	next := nextFactTransactionState(t, state)
	oldRaw := repo.factScope(ctx).payload()
	nextRaw, err := json.Marshal(next)
	require.NoError(t, err)
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	// Hold the production statement's locks in an explicit test transaction
	// to observe exactly what it protects. Runtime uses autocommit instead.
	var a, b, c, d bool
	var name *string
	var runways []byte
	var audits int64
	err = tx.QueryRow(ctx, scopedSessionCommitSQL, state.SessionID, state.Airport, state.Revision, oldRaw, nextRaw, "SAS123", "EKCH_A_APP", next.Revision, []byte(`[]`), []byte(`[]`), []byte(`[]`), true).Scan(&a, &b, &c, &d, &name, &runways, &audits)
	require.NoError(t, err)
	require.True(t, a && b && c && d)
	genericDone := make(chan error, 1)
	go func() {
		generic := nextFactTransactionState(t, state)
		generic.PolicyVersion = "generic-writer"
		_, err := NewAMANRepository(pool).Commit(aman.WithSession(t.Context(), state.SessionID), aman.StateCommit{ExpectedRevision: state.Revision, State: generic})
		genericDone <- err
	}()
	deadline, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	_, err = pool.Exec(deadline, `UPDATE strips SET position_altitude=12000 WHERE session=$1 AND callsign='SAS124'`, state.SessionID)
	require.NoError(t, err, "other aircraft must never wait on the direct-to tracking guard")
	handoffCtx, cancelHandoff := context.WithTimeout(ctx, 150*time.Millisecond)
	_, err = pool.Exec(handoffCtx, `UPDATE strips SET tracking_controller='EKCH_B_APP' WHERE session=$1 AND callsign='SAS123'`, state.SessionID)
	cancelHandoff()
	require.Error(t, err, "the same aircraft's handoff must serialize with the fact write")
	require.NoError(t, tx.Commit(ctx))
	select {
	case err := <-genericDone:
		requireDomainErrorClass(t, err, aman.ErrorRevisionConflict)
	case <-time.After(2 * time.Second):
		t.Fatal("generic writer did not resolve its revision conflict")
	}
	_, err = pool.Exec(deadline, `UPDATE strips SET tracking_controller='EKCH_B_APP',position_altitude=13000 WHERE session=$1 AND callsign='SAS123'`, state.SessionID)
	require.NoError(t, err, "all tracking and position locks are released before prediction starts")
}

func TestFactTransactionRechecksRunwaySnapshotAtPredictionWrite(t *testing.T) {
	repo, pool, ctx, state := factTransactionFixture(t)
	_, err := pool.Exec(ctx, `UPDATE sessions SET active_runways='{"arrival_runways":["04L"]}'::jsonb WHERE id=$1`, state.SessionID)
	require.NoError(t, err)
	next := nextFactTransactionState(t, state)
	_, err = repo.Commit(ctx, aman.StateCommit{ExpectedRevision: state.Revision, State: next})
	requireDomainErrorClass(t, err, aman.ErrorRevisionConflict)
	_, err = repo.LoadAirportState(ctx, "EKCH")
	require.NoError(t, err)
	snapshot, ok := aman.ReadSessionSnapshot(ctx)
	require.True(t, ok)
	require.Equal(t, []string{"04L"}, snapshot.ArrivalRunways)
	_, err = repo.Commit(ctx, aman.StateCommit{ExpectedRevision: state.Revision, State: next})
	require.NoError(t, err)
}

func TestFactTransactionExpiryCorrelationAndAuditAreAtomic(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "commit", true: "rollback"}[fail], func(t *testing.T) {
			repo, pool, ctx, state := factTransactionFixture(t)
			coord := coordinationrequest.NewRepository(pool)
			accepted, err := coordinationrequest.New("accepted-direct", "EKCH", "SAS123", "EKCH_A_APP", "123", "EKCH_FMP", coordinationrequest.KindRouteDirect,
				coordinationrequest.Payload{RouteDirect: &coordinationrequest.RouteDirectPayload{DirectTo: "KAS"}}, state.GeneratedAt.Add(-2*time.Minute))
			require.NoError(t, err)
			accepted, err = accepted.Decide("accept-direct", "456", "EKCH_A_APP", "EKCH_A_APP", coordinationrequest.StateAccepted, "", state.GeneratedAt.Add(-time.Minute))
			require.NoError(t, err)
			require.NoError(t, coord.Save(ctx, accepted))
			pending, err := coordinationrequest.New("pending-speed", "EKCH", "SAS123", "EKCH_A_APP", "123", "EKCH_FMP", coordinationrequest.KindSpeed,
				coordinationrequest.Payload{Speed: &coordinationrequest.SpeedPayload{Requested: "220 KT"}}, state.GeneratedAt.Add(-time.Minute))
			require.NoError(t, err)
			require.NoError(t, coord.Save(ctx, pending))
			fact := coordinationrequest.ClearanceFact{Airport: "EKCH", Callsign: "SAS123", FactID: state.Flights[0].ActiveRouteFact.ID, Kind: coordinationrequest.KindRouteDirect, Value: "KAS", Issuer: "EKCH_A_APP", ObservedAt: state.GeneratedAt}
			ctx = coordinationrequest.WithAggregateClearance(ctx, fact)
			next := nextFactTransactionState(t, state)
			next.Flights[0].State = aman.StateLanded
			next.Flights[0].Slot, next.Flights[0].Order = nil, nil
			next.Flights[0].ActiveRouteFact = state.Flights[0].ActiveRouteFact
			record := aman.AuditRecord{Airport: "EKCH", Revision: next.Revision, Category: "aman.landed", Payload: []byte(`{}`), RecordedAt: next.GeneratedAt}
			if fail {
				_, err = pool.Exec(ctx, `CREATE FUNCTION reject_combined_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'audit failure'; END $$`)
				require.NoError(t, err)
				_, err = pool.Exec(ctx, `CREATE TRIGGER reject_combined_audit BEFORE INSERT ON aman_session_audit_records FOR EACH ROW EXECUTE FUNCTION reject_combined_audit()`)
				require.NoError(t, err)
			}
			_, err = repo.Commit(ctx, aman.StateCommit{ExpectedRevision: state.Revision, State: next, AuditRecords: []aman.AuditRecord{record}})
			if fail {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			requests, err := coord.ReplayAirport(ctx, "EKCH")
			require.NoError(t, err)
			require.Len(t, requests, 2)
			loaded, err := NewAMANRepository(pool).LoadAirportState(ctx, "EKCH")
			require.NoError(t, err)
			if fail {
				require.Equal(t, state, loaded, "the previously accepted fact remains durable")
				require.Nil(t, requests[0].Clearance)
				require.Equal(t, coordinationrequest.StatePending, requests[1].State)
			} else {
				require.Equal(t, next.Revision, loaded.Revision)
				require.NotNil(t, requests[0].Clearance)
				require.Equal(t, coordinationrequest.StateExpired, requests[1].State)
				require.Equal(t, coordinationrequest.ExpiryFlightCompleted, requests[1].Expiry.Reason)
			}
			service := coordinationrequest.NewService(coord, coord)
			notifications := 0
			service.SetClearanceNotifier(func(string) { notifications++ })
			result, err := service.ObserveClearance(ctx, fact)
			require.NoError(t, err, "a failed prediction falls back to independent correlation")
			require.Equal(t, fact.FactID, result.Request.Clearance.FactID)
			require.False(t, result.Duplicate)
			require.Equal(t, 1, notifications, "the committed receipt retains the ordinary service notification")
			result, err = service.ObserveClearance(ctx, fact)
			require.NoError(t, err)
			require.True(t, result.Duplicate)
			require.Equal(t, 1, notifications)
		})
	}
}
