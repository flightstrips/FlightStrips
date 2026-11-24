package postgres

import (
	"FlightStrips/internal/models"
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// CommitStripAssumption changes the owner and route in one transaction. It
// consumes the selected coordination when present and requires the strip to
// still have no coordination for a direct assumption.
func (r *stripRepository) CommitStripAssumption(ctx context.Context, session int32, strip *models.Strip, coordination *models.Coordination, owner string, nextOwners, previousOwners []string) (bool, error) {
	next, err := marshalOwners(nextOwners)
	if err != nil {
		return false, err
	}
	previous, err := marshalOwners(previousOwners)
	if err != nil {
		return false, err
	}
	observedNext, err := marshalOwners(strip.NextOwners)
	if err != nil {
		return false, err
	}
	observedPrevious, err := marshalOwners(strip.PreviousOwners)
	if err != nil {
		return false, err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)

	var version int32
	// Route recalculation can update station eligibility without bumping the
	// version. Compare the route as well as the revision used for the decision.
	err = tx.QueryRow(ctx, `SELECT version FROM strips
		WHERE session=$1 AND id=$2
		AND COALESCE(NULLIF(next_owners, 'null'::jsonb), '[]'::jsonb)=$3::jsonb
		AND COALESCE(NULLIF(previous_owners, 'null'::jsonb), '[]'::jsonb)=$4::jsonb
		FOR UPDATE`, session, strip.ID, observedNext, observedPrevious).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if version != strip.Version {
		return false, nil
	}
	if coordination != nil {
		if coordination.StripID != strip.ID {
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
	} else {
		// The locked strip row also serializes coordination inserts through
		// their foreign key. Recheck for a transfer created after our lookup.
		var pending bool
		err := tx.QueryRow(ctx, `SELECT EXISTS (
			SELECT 1 FROM coordinations WHERE session=$1 AND strip_id=$2
		)`, session, strip.ID).Scan(&pending)
		if err != nil || pending {
			return false, err
		}
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
