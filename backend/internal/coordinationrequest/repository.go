package coordinationrequest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrRevisionConflict = errors.New("coordination request revision conflict")
	ErrCommandConflict  = errors.New("coordination request command identity conflict")
	ErrRequestNotFound  = errors.New("coordination request not found")
	ErrInvalidState     = errors.New("coordination request is not pending")
)

type CommitResult struct {
	Request           Request
	SupersededRequest *Request
	Revision          uint64
	Duplicate         bool
}

type TransferResult struct {
	Requests  []Request
	Revision  uint64
	Duplicate bool
}

type ExpiryFact struct {
	Airport, FactID string
	FlightID        FlightID
	Revision        uint64
	Reason          ExpiryReason
	OccurredAt      time.Time
}

type OwnershipFact struct {
	Airport    string
	FlightID   FlightID
	FactID     string
	Revision   uint64
	Owner      ControllerID
	ObservedAt time.Time
}

// Repository is the sole persistence owner for coordination request aggregates.
type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (r *Repository) Save(ctx context.Context, request Request) error {
	if err := request.Validate(); err != nil {
		return err
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("encode coordination request: %w", err)
	}
	tag, err := r.pool.Exec(ctx, `INSERT INTO aman_coordination_requests
        (request_id, airport, command_id, created_at, payload) VALUES ($1, $2, $3, $4, $5)
        ON CONFLICT (request_id) DO UPDATE SET payload = EXCLUDED.payload
        WHERE aman_coordination_requests.airport = EXCLUDED.airport
          AND aman_coordination_requests.command_id = EXCLUDED.command_id`,
		request.ID, request.Airport, request.CommandID, request.CreatedAt, payload)
	if err == nil && tag.RowsAffected() != 1 {
		return errors.New("coordination request identity conflicts with persisted owner")
	}
	return err
}

func (r *Repository) Get(ctx context.Context, airport string, id RequestID) (Request, error) {
	var raw []byte
	err := r.pool.QueryRow(ctx, `SELECT payload FROM aman_coordination_requests WHERE airport=$1 AND request_id=$2`, airport, id).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return Request{}, fmt.Errorf("%w: %s", ErrRequestNotFound, id)
	}
	if err != nil {
		return Request{}, err
	}
	var request Request
	if err = json.Unmarshal(raw, &request); err != nil {
		return Request{}, err
	}
	return request, request.Validate()
}

// Decide atomically accepts or rejects a pending request. Persisted command
// identity is checked before revision/state so exact retries remain idempotent.
func (r *Repository) Decide(ctx context.Context, id RequestID, decision Decision, expectedRevision uint64) (CommitResult, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return CommitResult{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, decision.Airport); err != nil {
		return CommitResult{}, err
	}
	rows, err := tx.Query(ctx, `SELECT payload FROM aman_coordination_requests WHERE airport=$1 ORDER BY created_at, request_id FOR UPDATE`, decision.Airport)
	if err != nil {
		return CommitResult{}, err
	}
	var current []Request
	for rows.Next() {
		var raw []byte
		var request Request
		if err = rows.Scan(&raw); err == nil {
			err = json.Unmarshal(raw, &request)
		}
		if err != nil {
			rows.Close()
			return CommitResult{}, err
		}
		if err = request.Validate(); err != nil {
			rows.Close()
			return CommitResult{}, err
		}
		current = append(current, request)
	}
	rows.Close()
	for _, request := range current {
		if request.CommandID == decision.CommandID {
			return CommitResult{}, ErrCommandConflict
		}
		if request.Decision != nil && request.Decision.CommandID == decision.CommandID {
			if request.ID != id || request.Decision.AfterState != decision.AfterState || request.Decision.Reason != decision.Reason ||
				request.Decision.Actor != decision.Actor || request.Decision.Role != decision.Role || request.Decision.AuthoritativeRecipient != decision.AuthoritativeRecipient {
				return CommitResult{}, ErrCommandConflict
			}
			return CommitResult{Request: request, Revision: coordinationRevision(current), Duplicate: true}, tx.Commit(ctx)
		}
	}
	if coordinationRevision(current) != expectedRevision {
		return CommitResult{}, fmt.Errorf("%w: expected %d, current %d", ErrRevisionConflict, expectedRevision, coordinationRevision(current))
	}
	for _, request := range current {
		if request.ID != id {
			continue
		}
		if request.State != StatePending {
			return CommitResult{}, fmt.Errorf("%w: current state %s", ErrInvalidState, request.State)
		}
		if request.RecipientController != decision.AuthoritativeRecipient || request.Airport == "" {
			return CommitResult{}, ErrWrongRecipient
		}
		resolved, resolveErr := request.Decide(decision.CommandID, decision.Actor, decision.Role, decision.AuthoritativeRecipient, decision.AfterState, decision.Reason, decision.ReceivedAt)
		if resolveErr != nil {
			return CommitResult{}, resolveErr
		}
		if err = save(ctx, tx, resolved); err != nil {
			return CommitResult{}, err
		}
		if err = tx.Commit(ctx); err != nil {
			return CommitResult{}, err
		}
		return CommitResult{Request: resolved, Revision: expectedRevision + 1}, nil
	}
	return CommitResult{}, ErrRequestNotFound
}

func coordinationRevision(requests []Request) uint64 {
	revision := uint64(len(requests))
	transfers := make(map[string]struct{})
	expiries := make(map[string]struct{})
	for _, request := range requests {
		if request.Decision != nil {
			revision++
		}
		for _, transfer := range request.RecipientTransfers {
			key := string(request.FlightID) + "\x00" + transfer.OwnershipFact + "\x00" + fmt.Sprint(transfer.OwnershipRevision)
			transfers[key] = struct{}{}
		}
		if request.Expiry != nil {
			expiries[request.Expiry.FactID+"\x00"+fmt.Sprint(request.Expiry.FactRevision)] = struct{}{}
		}
	}
	return revision + uint64(len(transfers)) + uint64(len(expiries))
}

// ExpirePending atomically applies one authoritative lifecycle/DSEQ fact to
// every pending request for the flight. Exact fact replay is a durable no-op.
func (r *Repository) ExpirePending(ctx context.Context, fact ExpiryFact) (TransferResult, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return TransferResult{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	result, err := ExpirePendingTx(ctx, tx, fact)
	if err != nil {
		return TransferResult{}, err
	}
	return result, tx.Commit(ctx)
}

// ExpirePendingTx joins expiry to an owning operational transaction.
func ExpirePendingTx(ctx context.Context, tx pgx.Tx, fact ExpiryFact) (TransferResult, error) {
	if fact.Airport == "" || fact.Airport != strings.TrimSpace(fact.Airport) || fact.FlightID == "" ||
		fact.FactID == "" || fact.FactID != strings.TrimSpace(fact.FactID) || fact.Revision == 0 ||
		!fact.Reason.valid() || !utc(fact.OccurredAt) {
		return TransferResult{}, errors.New("authoritative coordination expiry fact is invalid")
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, fact.Airport); err != nil {
		return TransferResult{}, err
	}
	rows, err := tx.Query(ctx, `SELECT payload FROM aman_coordination_requests WHERE airport=$1 ORDER BY created_at, request_id FOR UPDATE`, fact.Airport)
	if err != nil {
		return TransferResult{}, err
	}
	var current []Request
	for rows.Next() {
		var raw []byte
		var request Request
		if err = rows.Scan(&raw); err == nil {
			err = json.Unmarshal(raw, &request)
		}
		if err == nil {
			err = request.Validate()
		}
		if err != nil {
			rows.Close()
			return TransferResult{}, err
		}
		current = append(current, request)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return TransferResult{}, err
	}
	result := TransferResult{Revision: coordinationRevision(current)}
	var duplicates []Request
	for _, request := range current {
		if request.Expiry != nil && request.Expiry.FactID == fact.FactID && request.Expiry.FactRevision == fact.Revision {
			if request.Expiry.Reason != fact.Reason || !request.Expiry.ExpiredAt.Equal(fact.OccurredAt) {
				return TransferResult{}, ErrCommandConflict
			}
			duplicates = append(duplicates, request)
		}
	}
	if len(duplicates) > 0 {
		return TransferResult{Requests: duplicates, Revision: result.Revision, Duplicate: true}, nil
	}
	for _, request := range current {
		if request.FlightID != fact.FlightID || request.State != StatePending {
			continue
		}
		updated, expireErr := request.Expire(Expiry{FactID: fact.FactID, FactRevision: fact.Revision, Reason: fact.Reason, ExpiredAt: fact.OccurredAt})
		if expireErr != nil {
			return TransferResult{}, expireErr
		}
		if err = save(ctx, tx, updated); err != nil {
			return TransferResult{}, err
		}
		result.Requests = append(result.Requests, updated)
	}
	if len(result.Requests) > 0 {
		result.Revision++
	}
	return result, nil
}

// TransferPending atomically applies one authoritative ownership fact to every
// pending request for its flight. Replaying the same fact is a durable no-op.
func (r *Repository) TransferPending(ctx context.Context, fact OwnershipFact) (TransferResult, error) {
	if fact.Airport == "" || fact.Airport != strings.TrimSpace(fact.Airport) || fact.FlightID == "" ||
		fact.FactID == "" || fact.FactID != strings.TrimSpace(fact.FactID) || fact.Revision == 0 ||
		fact.Owner != ControllerID(strings.TrimSpace(string(fact.Owner))) || !utc(fact.ObservedAt) {
		return TransferResult{}, errors.New("authoritative ownership fact is invalid")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return TransferResult{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, fact.Airport); err != nil {
		return TransferResult{}, err
	}
	rows, err := tx.Query(ctx, `SELECT payload FROM aman_coordination_requests WHERE airport=$1 ORDER BY created_at, request_id FOR UPDATE`, fact.Airport)
	if err != nil {
		return TransferResult{}, err
	}
	var current []Request
	for rows.Next() {
		var raw []byte
		var request Request
		if err = rows.Scan(&raw); err == nil {
			err = json.Unmarshal(raw, &request)
		}
		if err == nil {
			err = request.Validate()
		}
		if err != nil {
			rows.Close()
			return TransferResult{}, err
		}
		current = append(current, request)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return TransferResult{}, err
	}
	var duplicates []Request
	for _, request := range current {
		for _, transfer := range request.RecipientTransfers {
			if request.FlightID == fact.FlightID && transfer.OwnershipFact == fact.FactID && transfer.OwnershipRevision == fact.Revision {
				if transfer.NewRecipient != fact.Owner || !transfer.TransferredAt.Equal(fact.ObservedAt) {
					return TransferResult{}, ErrCommandConflict
				}
				duplicates = append(duplicates, request)
				break
			}
		}
	}
	if len(duplicates) > 0 {
		return TransferResult{Requests: duplicates, Revision: coordinationRevision(current), Duplicate: true}, tx.Commit(ctx)
	}
	result := TransferResult{Revision: coordinationRevision(current)}
	for _, request := range current {
		if request.FlightID != fact.FlightID || request.State != StatePending ||
			(request.RecipientController == fact.Owner && request.effectiveRecipientStatus() == RecipientAssigned) ||
			(fact.Owner == "" && request.effectiveRecipientStatus() == RecipientUnassigned) {
			continue
		}
		updated, transferErr := request.TransferRecipient(fact.FactID, fact.Revision, fact.Owner, fact.ObservedAt)
		if transferErr != nil {
			return TransferResult{}, transferErr
		}
		if err = save(ctx, tx, updated); err != nil {
			return TransferResult{}, err
		}
		result.Requests = append(result.Requests, updated)
	}
	if len(result.Requests) > 0 {
		result.Revision++
	}
	if err = tx.Commit(ctx); err != nil {
		return TransferResult{}, err
	}
	return result, nil
}

// ReplayAirport restores validated requests in deterministic creation order.
func (r *Repository) ReplayAirport(ctx context.Context, airport string) ([]Request, error) {
	if airport == "" || airport != strings.TrimSpace(airport) {
		return nil, errors.New("airport is required")
	}
	rows, err := r.pool.Query(ctx, `SELECT payload FROM aman_coordination_requests
        WHERE airport = $1 ORDER BY created_at, request_id`, airport)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	requests := make([]Request, 0)
	for rows.Next() {
		var payload []byte
		var request Request
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(payload, &request); err != nil {
			return nil, fmt.Errorf("decode coordination request: %w", err)
		}
		if err := request.Validate(); err != nil {
			return nil, fmt.Errorf("validate persisted coordination request: %w", err)
		}
		requests = append(requests, request)
	}
	return requests, rows.Err()
}

// Submit atomically records a request and supersedes only the prior pending
// request for the same flight and kind. The airport row count is the command
// revision. Submissions and decisions each advance it exactly once.
func (r *Repository) Submit(ctx context.Context, request Request, expectedRevision uint64) (CommitResult, error) {
	if err := request.Validate(); err != nil {
		return CommitResult{}, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return CommitResult{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, request.Airport); err != nil {
		return CommitResult{}, err
	}
	rows, err := tx.Query(ctx, `SELECT payload FROM aman_coordination_requests WHERE airport=$1 ORDER BY created_at, request_id FOR UPDATE`, request.Airport)
	if err != nil {
		return CommitResult{}, err
	}
	var current []Request
	for rows.Next() {
		var raw []byte
		var persisted Request
		if err = rows.Scan(&raw); err == nil {
			err = json.Unmarshal(raw, &persisted)
		}
		if err != nil {
			rows.Close()
			return CommitResult{}, err
		}
		current = append(current, persisted)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return CommitResult{}, err
	}
	for _, persisted := range current {
		if err := persisted.Validate(); err != nil {
			return CommitResult{}, fmt.Errorf("validate persisted coordination request: %w", err)
		}
		if persisted.Decision != nil && persisted.Decision.CommandID == request.CommandID {
			return CommitResult{}, ErrCommandConflict
		}
		if persisted.ID == request.ID {
			if persisted.Airport != request.Airport || persisted.FlightID != request.FlightID || persisted.Kind != request.Kind ||
				persisted.SubmittedBy != request.SubmittedBy || persisted.SubmittedRole != request.SubmittedRole || !reflect.DeepEqual(persisted.Payload, request.Payload) {
				return CommitResult{}, ErrCommandConflict
			}
			return CommitResult{Request: persisted, Revision: coordinationRevision(current), Duplicate: true}, tx.Commit(ctx)
		}
	}
	if coordinationRevision(current) != expectedRevision {
		return CommitResult{}, fmt.Errorf("%w: expected %d, current %d", ErrRevisionConflict, expectedRevision, coordinationRevision(current))
	}
	var superseded *Request
	for index := len(current) - 1; index >= 0; index-- {
		candidate := current[index]
		if candidate.FlightID == request.FlightID && candidate.Kind == request.Kind && candidate.State == StatePending {
			candidate, err = candidate.Supersede(request.ID, request.CreatedAt)
			if err != nil {
				return CommitResult{}, err
			}
			request.Supersedes = &candidate.ID
			superseded = &candidate
			break
		}
	}
	if err = save(ctx, tx, request); err == nil && superseded != nil {
		err = save(ctx, tx, *superseded)
	}
	if err != nil {
		return CommitResult{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return CommitResult{}, err
	}
	return CommitResult{Request: request, SupersededRequest: superseded, Revision: coordinationRevision(current) + 1}, nil
}

type executor interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func save(ctx context.Context, target executor, request Request) error {
	payload, err := json.Marshal(request)
	if err != nil {
		return err
	}
	_, err = target.Exec(ctx, `INSERT INTO aman_coordination_requests (request_id, airport, command_id, created_at, payload)
        VALUES ($1,$2,$3,$4,$5) ON CONFLICT (request_id) DO UPDATE SET payload=EXCLUDED.payload`, request.ID, request.Airport, request.CommandID, request.CreatedAt, payload)
	return err
}
