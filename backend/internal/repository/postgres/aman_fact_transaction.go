package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/coordinationrequest"
	pkgModels "FlightStrips/pkg/models"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// This view is private to one repository and event. CPU work uses detached
// snapshots; only the final conditional SQL statement takes row locks.
type factStateScope struct {
	mu      sync.Mutex
	raw     []byte
	session aman.SessionSnapshot
	valid   bool
}

func (s *factStateScope) sessionSnapshot() (aman.SessionSnapshot, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v := s.session
	v.ArrivalRunways = append([]string(nil), v.ArrivalRunways...)
	return v, s.valid
}
func (s *factStateScope) payload() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.valid {
		return nil
	}
	return append([]byte(nil), s.raw...)
}
func (s *factStateScope) invalidate() { s.mu.Lock(); s.valid = false; s.mu.Unlock() }
func (s *factStateScope) store(raw []byte, session aman.SessionSnapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.raw = append([]byte(nil), raw...)
	s.session = session
	s.valid = true
}
func (r *amanRepository) factScope(ctx context.Context) *factStateScope {
	scope, _ := ctx.Value(r).(*factStateScope)
	if scope != nil {
		session, _ := scope.sessionSnapshot()
		if session.ID != aman.SessionID(ctx) {
			return nil
		}
	}
	return scope
}

func decodeSessionSnapshot(id int32, airport, name string, raw []byte) (aman.SessionSnapshot, error) {
	var runways pkgModels.ActiveRunways
	if err := json.Unmarshal(raw, &runways); err != nil {
		return aman.SessionSnapshot{}, corruptAMANData("decode session runways", err)
	}
	return aman.SessionSnapshot{ID: id, Airport: airport, Live: strings.EqualFold(name, "LIVE"), ArrivalRunways: runways.ArrivalRunways}, nil
}

func decodeFactState(ctx context.Context, airport string, raw []byte) (aman.AirportState, error) {
	if len(raw) == 0 {
		return aman.AirportState{}, &aman.DomainError{Class: aman.ErrorNotFound, Message: "AMAN session state was not found"}
	}
	var state aman.AirportState
	if err := json.Unmarshal(raw, &state); err != nil {
		return state, corruptAMANData("decode session state", err)
	}
	if state.SessionID != aman.SessionID(ctx) || state.Airport != airport {
		return state, corruptAMANData("session state identity", errors.New("session/airport mismatch"))
	}
	if err := state.Validate(); err != nil {
		return state, corruptAMANData("validate session state", err)
	}
	return state, nil
}

// LoadRouteFactState combines the early authorization rejection, session
// inputs, and aggregate read. Tracking authority is revalidated under a row
// lock by CommitRouteFact; this read grants no durable authority.
func (r *amanRepository) LoadRouteFactState(ctx context.Context, airport, callsign, controller string) (context.Context, aman.AirportState, error) {
	var actual, name string
	var runways, raw []byte
	var tracker *string
	err := r.pool.QueryRow(ctx, `SELECT s.airport,s.name,s.active_runways,a.payload,t.tracking_controller
FROM sessions s
LEFT JOIN aman_session_states a ON a.session_id=s.id AND a.airport=$2
LEFT JOIN strips t ON t.session=s.id AND t.callsign=$3 WHERE s.id=$1`, aman.SessionID(ctx), airport, callsign).Scan(&actual, &name, &runways, &raw, &tracker)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && (actual != airport || tracker == nil || !strings.EqualFold(strings.TrimSpace(*tracker), controller)) {
		return ctx, aman.AirportState{}, &aman.DomainError{Class: aman.ErrorUnauthorized, Message: "only the current tracking controller may report a direct-to fact"}
	}
	if err != nil {
		return ctx, aman.AirportState{}, err
	}
	state, err := decodeFactState(ctx, airport, raw)
	if err != nil {
		return ctx, state, err
	}
	session, err := decodeSessionSnapshot(aman.SessionID(ctx), airport, name, runways)
	if err != nil {
		return ctx, state, err
	}
	scope := &factStateScope{}
	scope.store(raw, session)
	ctx = context.WithValue(ctx, r, scope)
	return aman.WithSessionSnapshot(ctx, scope.sessionSnapshot), state, nil
}

func (r *amanRepository) reloadFactState(ctx context.Context, airport string, scope *factStateScope) (aman.AirportState, error) {
	var raw, runways []byte
	var name string
	err := r.pool.QueryRow(ctx, `SELECT a.payload,s.name,s.active_runways FROM aman_session_states a JOIN sessions s ON s.id=a.session_id AND s.airport=a.airport WHERE a.session_id=$1 AND a.airport=$2`, aman.SessionID(ctx), airport).Scan(&raw, &name, &runways)
	if errors.Is(err, pgx.ErrNoRows) {
		return decodeFactState(ctx, airport, nil)
	}
	if err != nil {
		return aman.AirportState{}, err
	}
	state, err := decodeFactState(ctx, airport, raw)
	if err != nil {
		return state, err
	}
	session, err := decodeSessionSnapshot(aman.SessionID(ctx), airport, name, runways)
	if err == nil {
		scope.store(raw, session)
	}
	return state, err
}

// Each DML statement is its own PostgreSQL transaction. Facts remain durable
// before prediction begins. NOWAIT avoids waiting with another row locked;
// locks are held only for this statement, never for navigation or prediction.
const scopedSessionCommitSQL = `WITH session_read AS MATERIALIZED (
 SELECT id,airport,name,active_runways FROM sessions WHERE id=$1 AND airport=$2 FOR KEY SHARE NOWAIT
), previous AS MATERIALIZED (
 SELECT a.revision FROM aman_session_states a JOIN session_read s ON s.id=a.session_id
 WHERE a.session_id=$1 AND a.airport=$2 AND a.revision=$3 AND a.payload=$4::jsonb
 AND ($6<>'' OR ((lower(s.name)='live')=$12::boolean AND
  CASE WHEN jsonb_typeof(s.active_runways->'arrival_runways')='array' THEN s.active_runways->'arrival_runways' ELSE '[]'::jsonb END=$11::jsonb))
 FOR NO KEY UPDATE OF a NOWAIT
), tracker AS MATERIALIZED (
 SELECT t.tracking_controller FROM strips t
 WHERE $6<>'' AND t.session=$1 AND t.callsign=$6 AND EXISTS(SELECT 1 FROM previous)
 FOR SHARE OF t NOWAIT
), authority AS (
 SELECT $6='' OR EXISTS(SELECT 1 FROM tracker WHERE upper(btrim(tracking_controller))=$7) AS allowed
), written AS (
 UPDATE aman_session_states a SET revision=$8,payload=$5::jsonb
 WHERE a.session_id=$1 AND a.airport=$2 AND a.revision=$3 AND a.payload=$4::jsonb
 AND $8<>$3 AND EXISTS(SELECT 1 FROM previous) AND (SELECT allowed FROM authority)
 RETURNING a.session_id
), audit AS (
 INSERT INTO aman_session_audit_records(session_id,airport,revision,payload)
 SELECT $1,record->>'Airport',(record->>'Revision')::bigint,record
 FROM jsonb_array_elements($9::jsonb) AS records(record) WHERE EXISTS(SELECT 1 FROM written)
 RETURNING id
), request_updates AS (
 INSERT INTO aman_coordination_requests(request_id,airport,command_id,created_at,payload,session_id)
 SELECT record->>'id',record->>'airport',record->>'command_id',(record->>'created_at')::timestamptz,record,$1
 FROM jsonb_array_elements($10::jsonb) AS records(record) WHERE EXISTS(SELECT 1 FROM written)
 ON CONFLICT ((COALESCE(session_id,0)),request_id) DO UPDATE SET payload=EXCLUDED.payload
 RETURNING request_id
)
SELECT EXISTS(SELECT 1 FROM session_read),EXISTS(SELECT 1 FROM previous),(SELECT allowed FROM authority),
 EXISTS(SELECT 1 FROM written),(SELECT name FROM session_read),(SELECT active_runways FROM session_read),
 (SELECT count(*) FROM audit)+(SELECT count(*) FROM request_updates)`

func (r *amanRepository) CommitRouteFact(ctx context.Context, commit aman.StateCommit, callsign, controller string) (aman.CommitResult, error) {
	if callsign == "" || controller == "" || r.factScope(ctx) == nil {
		return aman.CommitResult{}, &aman.DomainError{Class: aman.ErrorUnauthorized, Message: "direct-to transaction requires tracking authority"}
	}
	result, handled, err := r.commitScopedSession(ctx, commit, callsign, controller)
	if !handled {
		return aman.CommitResult{}, &aman.DomainError{Class: aman.ErrorInvalidArgument, Message: "unsupported direct-to fact transaction"}
	}
	return result, err
}

// ValidateRouteFact authorizes an unchanged callback without constructing or
// locking an aggregate replacement. The historical fact is already durable;
// only its current identity and the aircraft tracking guard need checking.
func (r *amanRepository) ValidateRouteFact(ctx context.Context, revision aman.SequenceRevision, airport, callsign, controller string) error {
	scope := r.factScope(ctx)
	if scope == nil || callsign == "" || controller == "" {
		return &aman.DomainError{Class: aman.ErrorUnauthorized, Message: "direct-to validation requires tracking authority"}
	}
	if _, valid := scope.sessionSnapshot(); !valid {
		return revisionConflict()
	}
	var current, authorized bool
	err := r.pool.QueryRow(ctx, `WITH tracker AS MATERIALIZED (
 SELECT tracking_controller FROM strips WHERE session=$1 AND callsign=$4 FOR SHARE NOWAIT
)
SELECT EXISTS(SELECT 1 FROM aman_session_states a JOIN sessions s ON s.id=a.session_id AND s.airport=a.airport
 WHERE a.session_id=$1 AND a.airport=$2 AND a.revision=$3),
 EXISTS(SELECT 1 FROM tracker WHERE upper(btrim(tracking_controller))=$5)`, aman.SessionID(ctx), airport, revision, callsign, controller).Scan(&current, &authorized)
	if err != nil {
		scope.invalidate()
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "55P03" {
			return revisionConflict()
		}
		return err
	}
	if !current {
		scope.invalidate()
		return revisionConflict()
	}
	if !authorized {
		return &aman.DomainError{Class: aman.ErrorUnauthorized, Message: "only the current tracking controller may report a direct-to fact"}
	}
	return nil
}

func (r *amanRepository) commitScopedSession(ctx context.Context, commit aman.StateCommit, callsign, controller string) (aman.CommitResult, bool, error) {
	scope := r.factScope(ctx)
	if scope == nil || commit.ExpectedRevision == 0 || commit.CommandOutcome != nil || len(commit.ValidationEvidence) > 0 {
		return aman.CommitResult{}, false, nil
	}
	raw := scope.payload()
	if len(raw) == 0 {
		return aman.CommitResult{}, true, revisionConflict()
	}
	if commit.State.SessionID != aman.SessionID(ctx) {
		return aman.CommitResult{}, true, &aman.DomainError{Class: aman.ErrorInvalidArgument, Message: "AMAN commit belongs to another session"}
	}
	if err := commit.Validate(); err != nil {
		return aman.CommitResult{}, true, err
	}
	previous, err := decodeFactState(ctx, commit.State.Airport, raw)
	if err != nil {
		return aman.CommitResult{}, true, err
	}
	if previous.Revision != commit.ExpectedRevision {
		scope.invalidate()
		return aman.CommitResult{}, true, revisionConflict()
	}
	expiries := coordinationExpiryFacts(previous, commit.State)
	changed := commit.State.Revision == commit.ExpectedRevision+1
	if !changed && len(commit.AuditRecords) > 0 {
		return aman.CommitResult{}, false, nil
	}
	if !changed && !airportStatesEqual(previous, commit.State) {
		return aman.CommitResult{}, true, &aman.DomainError{Class: aman.ErrorInvalidArgument, Message: "unchanged session state differs from persisted state"}
	}
	stored := cloneAirportState(commit.State)
	stored.Flights = nil
	for _, flight := range commit.State.Flights {
		if retainAMANFlight(flight) {
			stored.Flights = append(stored.Flights, flight)
		}
	}
	if stored.Flights == nil {
		stored.Flights = []aman.AMANFlight{}
	}
	next, err := json.Marshal(stored)
	if err != nil {
		return aman.CommitResult{}, true, err
	}
	records := commit.AuditRecords
	if records == nil {
		records = []aman.AuditRecord{}
	}
	audit, err := json.Marshal(records)
	if err != nil {
		return aman.CommitResult{}, true, err
	}
	var tx pgx.Tx
	var correlation coordinationrequest.CommitResult
	updates := []coordinationrequest.Request{}
	var target rowQuerier = r.pool
	if changed && (len(expiries) > 0 || coordinationrequest.HasAggregateClearance(ctx)) {
		tx, err = r.pool.Begin(ctx)
		if err != nil {
			return aman.CommitResult{}, true, err
		}
		defer func() { _ = tx.Rollback(ctx) }()
		updates, correlation, err = coordinationrequest.PrepareAggregateTransaction(ctx, tx, stored.Airport, expiries)
		if err != nil {
			scope.invalidate()
			return aman.CommitResult{}, true, err
		}
		target = tx
	}
	if updates == nil {
		updates = []coordinationrequest.Request{}
	}
	requestUpdates, err := json.Marshal(updates)
	if err != nil {
		return aman.CommitResult{}, true, err
	}
	expectedSession, _ := scope.sessionSnapshot()
	if expectedSession.ArrivalRunways == nil {
		expectedSession.ArrivalRunways = []string{}
	}
	expectedRunways, err := json.Marshal(expectedSession.ArrivalRunways)
	if err != nil {
		return aman.CommitResult{}, true, err
	}
	var sessionExists, current, authorized, written bool
	var name *string
	var runways []byte
	var auditCount int64
	err = target.QueryRow(ctx, scopedSessionCommitSQL, aman.SessionID(ctx), stored.Airport, commit.ExpectedRevision, raw, next, callsign, controller, stored.Revision, audit, requestUpdates, expectedRunways, expectedSession.Live).Scan(&sessionExists, &current, &authorized, &written, &name, &runways, &auditCount)
	if err != nil {
		scope.invalidate()
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "55P03" {
			err = revisionConflict()
		}
		return aman.CommitResult{}, true, err
	}
	if !sessionExists {
		scope.invalidate()
		return aman.CommitResult{}, true, &aman.DomainError{Class: aman.ErrorUnauthorized, Message: "AMAN airport does not belong to session"}
	}
	if !current {
		scope.invalidate()
		return aman.CommitResult{}, true, revisionConflict()
	}
	if !authorized {
		return aman.CommitResult{}, true, &aman.DomainError{Class: aman.ErrorUnauthorized, Message: "only the current tracking controller may report a direct-to fact"}
	}
	if changed && !written {
		scope.invalidate()
		return aman.CommitResult{}, true, revisionConflict()
	}
	session, err := decodeSessionSnapshot(aman.SessionID(ctx), stored.Airport, *name, runways)
	if err != nil {
		scope.invalidate()
		return aman.CommitResult{}, true, err
	}
	if !changed {
		next = raw
	}
	if tx != nil {
		if err := tx.Commit(ctx); err != nil {
			scope.invalidate()
			return aman.CommitResult{}, true, err
		}
		coordinationrequest.CompleteAggregateClearance(ctx, r.pool, correlation)
	}
	scope.store(next, session)
	return aman.CommitResult{State: cloneAirportState(commit.State)}, true, nil
}
