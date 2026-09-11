package coordinationrequest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrRevisionConflict = errors.New("coordination request revision conflict")
	ErrCommandConflict  = errors.New("coordination request command identity conflict")
)

type CommitResult struct {
	Request           Request
	SupersededRequest *Request
	Revision          uint64
	Duplicate         bool
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
// revision because every successful non-duplicate submission adds one row.
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
		if persisted.ID == request.ID {
			if persisted.Airport != request.Airport || persisted.FlightID != request.FlightID || persisted.Kind != request.Kind ||
				persisted.SubmittedBy != request.SubmittedBy || persisted.SubmittedRole != request.SubmittedRole || !reflect.DeepEqual(persisted.Payload, request.Payload) {
				return CommitResult{}, ErrCommandConflict
			}
			return CommitResult{Request: persisted, Revision: uint64(len(current)), Duplicate: true}, tx.Commit(ctx)
		}
	}
	if uint64(len(current)) != expectedRevision {
		return CommitResult{}, fmt.Errorf("%w: expected %d, current %d", ErrRevisionConflict, expectedRevision, len(current))
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
	return CommitResult{Request: request, SupersededRequest: superseded, Revision: uint64(len(current) + 1)}, nil
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
