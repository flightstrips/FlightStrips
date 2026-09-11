package coordinationrequest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

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
