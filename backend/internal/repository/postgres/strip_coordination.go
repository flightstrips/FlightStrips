package postgres

import (
	"FlightStrips/internal/models"
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// AcceptCoordination consumes exactly the selected coordination and changes its
// strip's owner/history in one transaction. A stale strip or missing row has no
// effects, including when cancellation/another assumption wins while we wait.
func (r *stripRepository) AcceptCoordination(ctx context.Context, session int32, strip *models.Strip, coordination *models.Coordination, owner string, nextOwners, previousOwners []string) (bool, error) {
	next, err := marshalOwners(nextOwners)
	if err != nil {
		return false, err
	}
	previous, err := marshalOwners(previousOwners)
	if err != nil {
		return false, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)

	var version int32
	err = tx.QueryRow(ctx, "SELECT version FROM strips WHERE session=$1 AND id=$2 FOR UPDATE", session, strip.ID).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if version != strip.Version || coordination.StripID != strip.ID {
		return false, nil
	}
	consumed, err := tx.Exec(ctx, `DELETE FROM coordinations
		WHERE id=$1 AND session=$2 AND strip_id=$3 AND from_position=$4 AND to_position=$5`,
		coordination.ID, session, strip.ID, coordination.FromPosition, coordination.ToPosition)
	if err != nil {
		return false, err
	}
	if consumed.RowsAffected() != 1 {
		return false, nil
	}

	updated, err := tx.Exec(ctx, `UPDATE strips SET owner=$1,
		next_controller_sector=CASE WHEN owner IS DISTINCT FROM $1::varchar THEN NULL ELSE next_controller_sector END,
		next_owners=$2::jsonb, previous_owners=$3::jsonb,
		next_display_label=NULL, next_display_frequency=NULL, version=version+1
		WHERE session=$4 AND id=$5`, owner, next, previous, session, strip.ID)
	if err != nil {
		return false, err
	}
	if updated.RowsAffected() != 1 {
		return false, nil
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}
