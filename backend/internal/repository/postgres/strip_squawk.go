package postgres

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
)

// UpdateSquawkWithPrevious captures the old code under the same row lock as
// the write, so validation can identify both newly affected and resolved peers.
func (r *stripRepository) UpdateSquawkWithPrevious(ctx context.Context, session int32, callsign, code string, assigned bool) (*string, int64, error) {
	column := "squawk"
	if assigned {
		column = "assigned_squawk"
	}
	var previous *string
	err := r.db.QueryRow(ctx, `-- update squawk with previous code
WITH prior AS MATERIALIZED (
 SELECT id, `+column+` AS code FROM strips WHERE session=$1 AND callsign=$2 FOR UPDATE
)
UPDATE strips s SET `+column+`=$3, version=s.version+1
FROM prior WHERE s.id=prior.id RETURNING prior.code`, session, callsign, code).Scan(&previous)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	return previous, 1, nil
}
