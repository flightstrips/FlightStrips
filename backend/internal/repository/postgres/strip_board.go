package postgres

import (
	"FlightStrips/internal/database"
	"FlightStrips/internal/models"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
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

// AdmitStripBoardEntry prevents duplicate identities under the board's existing DB lock.
// FIND can reveal an existing planning row, but never unhides a removed strip.
func (r *stripRepository) AdmitStripBoardEntry(ctx context.Context, strip *models.Strip, allowExisting bool, guard func() error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if guard == nil {
		return fmt.Errorf("admission validation is required")
	}
	commit := func() error {
		if err := guard(); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	if _, err = tx.Exec(ctx, "SELECT id FROM sessions WHERE id=$1 FOR UPDATE", strip.Session); err != nil {
		return err
	}
	q := r.queries.WithTx(tx)
	row, err := q.LockStrip(ctx, database.LockStripParams{Session: strip.Session, Callsign: strip.Callsign})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if guardErr := guard(); guardErr != nil {
		return guardErr
	}
	if err == nil {
		if !allowExisting {
			return fmt.Errorf("callsign already has an airport strip; open or restore it")
		}
		existing, err := stripToModel(row)
		if err != nil {
			return err
		}
		if existing.VatsimCID != nil && strip.VatsimCID != nil && *existing.VatsimCID != *strip.VatsimCID {
			return fmt.Errorf("callsign belongs to another flight identity")
		}
		if existing.BoardState != nil {
			if existing.BoardState.RemovedAt != nil {
				return fmt.Errorf("removed strip must be restored under the airport policy")
			}
			if strip.BoardState != nil && strip.BoardState.ManualAdmission && !existing.BoardState.ManualAdmission {
				state := *existing.BoardState
				state.ManualAdmission = true
				data, err := json.Marshal(&state)
				if err != nil {
					return err
				}
				count, err := q.UpdateStripBoardState(ctx, database.UpdateStripBoardStateParams{Session: strip.Session, ID: existing.ID, Version: existing.Version, BoardState: data})
				if err != nil {
					return err
				}
				if count != 1 {
					return fmt.Errorf("strip changed during manual admission")
				}
			}
			if existing.VatsimCID == nil && strip.VatsimCID != nil {
				if _, err = tx.Exec(ctx, "UPDATE strips SET vatsim_cid=$1,vatsim_revision=$2,vatsim_seen_at=$3,version=version+1 WHERE session=$4 AND id=$5", strip.VatsimCID, strip.VatsimRevision, strip.VatsimSeenAt, strip.Session, existing.ID); err != nil {
					return err
				}
			}
			return commit()
		}
		if existing.EuroscopeSeenAt == nil && strip.VatsimCID != nil && strip.VatsimSeenAt != nil {
			bound := &stripRepository{db: tx, queries: q}
			value := func(v *string) string {
				if v != nil {
					return *v
				}
				return ""
			}
			source := models.VatsimStripSource{FlightPlanEET: strip.FlightPlanEET, CID: *strip.VatsimCID, Revision: *strip.VatsimRevision, SeenAt: *strip.VatsimSeenAt, Origin: strip.Origin, Destination: strip.Destination, Alternate: value(strip.Alternative), Route: value(strip.Route), Remarks: value(strip.Remarks), AssignedSquawk: value(strip.AssignedSquawk), AircraftType: value(strip.AircraftType), FlightRules: strip.FlightRules}
			if _, err := bound.UpdateVatsimSource(ctx, strip.Session, strip.Callsign, source); err != nil {
				return err
			}
		}
		sequence, err := q.GetMaxSequenceInBay(ctx, database.GetMaxSequenceInBayParams{Session: strip.Session, Bay: strip.Bay})
		if err != nil {
			return err
		}
		data, err := json.Marshal(strip.BoardState)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "UPDATE strips SET board_state=$1,bay=$2,sequence=$3,has_fp=CASE WHEN euroscope_seen_at IS NULL THEN false ELSE has_fp END,vatsim_cid=COALESCE(vatsim_cid,$6),vatsim_revision=$7,vatsim_seen_at=$8,flight_plan_eet=CASE WHEN euroscope_seen_at IS NULL THEN $9 ELSE flight_plan_eet END,version=version+1 WHERE session=$4 AND id=$5", data, strip.Bay, sequence+1000, strip.Session, existing.ID, strip.VatsimCID, strip.VatsimRevision, strip.VatsimSeenAt, strip.FlightPlanEET); err != nil {
			return err
		}
		return commit()
	}
	sequence, err := q.GetMaxSequenceInBay(ctx, database.GetMaxSequenceInBayParams{Session: strip.Session, Bay: strip.Bay})
	if err != nil {
		return err
	}
	sequence += 1000
	strip.Sequence = &sequence
	bound := &stripRepository{db: tx, queries: q}
	if err = bound.Create(ctx, strip); err != nil {
		return err
	}
	return commit()
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
