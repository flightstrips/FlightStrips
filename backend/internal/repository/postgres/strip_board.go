package postgres

import (
	"FlightStrips/internal/database"
	"FlightStrips/internal/models"
	"context"
	"encoding/json"
	"fmt"
)

// UpdateStripBoardState is a strip-identity and revision CAS. Generic source updates
// deliberately do not write this column. No provider I/O occurs in persistence.
func (r *stripRepository) UpdateStripBoardState(ctx context.Context, session, id, version int32, state *models.StripBoardState) (bool, error) {
	data, err := json.Marshal(state)
	if err != nil {
		return false, err
	}
	count, err := r.queries.UpdateStripBoardState(ctx, database.UpdateStripBoardStateParams{Session: session, ID: id, Version: version, BoardState: data})
	return count == 1, err
}

// ReorderStripBoard uses the same session-row serialization boundary as AppendToBay.
// Only order is changed here; provider effects run before entering this method.
func (r *stripRepository) ReorderStripBoard(ctx context.Context, session, id, version int32, bay string, after *string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "SELECT id FROM sessions WHERE id=$1 FOR UPDATE", session); err != nil {
		return err
	}
	var actual int32
	var currentBay string
	if err = tx.QueryRow(ctx, "SELECT version,bay FROM strips WHERE session=$1 AND id=$2 FOR UPDATE", session, id).Scan(&actual, &currentBay); err != nil {
		return err
	}
	if actual != version || currentBay != bay {
		return fmt.Errorf("stale RADIS ordering revision")
	}
	rows, err := tx.Query(ctx, "SELECT id,callsign FROM strips WHERE session=$1 AND bay=$2 ORDER BY sequence,id", session, bay)
	if err != nil {
		return err
	}
	type entry struct {
		id       int32
		callsign string
	}
	list := []entry{}
	index := 0
	found := after == nil
	for rows.Next() {
		var e entry
		if err = rows.Scan(&e.id, &e.callsign); err != nil {
			rows.Close()
			return err
		}
		if e.id == id {
			continue
		}
		list = append(list, e)
		if after != nil && e.callsign == *after {
			index = len(list)
			found = true
		}
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("RADIS predecessor is not in the selected bay")
	}
	list = append(list, entry{})
	copy(list[index+1:], list[index:])
	list[index] = entry{id: id}
	for i, e := range list {
		if _, err = tx.Exec(ctx, "UPDATE strips SET sequence=$1,version=version+1 WHERE session=$2 AND id=$3 AND sequence IS DISTINCT FROM $1", int32(i+1)*1000, session, e.id); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// A stale source snapshot must not turn an admitted airport strip back into a
// hidden planning row. The predicate is checked atomically with the bay write.
func (r *stripRepository) UpdatePlanningBayAndSequence(ctx context.Context, session int32, callsign, bay string, sequence int32) (int64, error) {
	result, err := r.db.Exec(ctx, "UPDATE strips SET bay=$1,sequence=$2,version=version+1 WHERE session=$3 AND callsign=$4 AND euroscope_seen_at IS NULL AND board_state IS NULL", bay, sequence, session, callsign)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected(), nil
}
