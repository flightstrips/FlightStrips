package postgres

import (
	"FlightStrips/internal/aman"
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"strings"
)

func (r *amanRepository) FindActiveFactFlight(ctx context.Context, airport, callsign string) (aman.FlightID, error) {
	rows, err := r.pool.Query(ctx, `-- AMAN active fact identity
SELECT flight_id FROM aman_flights
WHERE airport=$1 AND upper(btrim(current_callsign))=$2 AND state NOT IN ('landed','removed')
ORDER BY flight_id LIMIT 2`, airport, strings.ToUpper(strings.TrimSpace(callsign)))
	if err != nil {
		return "", err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return "", err
	}
	if len(ids) == 0 {
		return "", &aman.DomainError{Class: aman.ErrorNotFound, Message: "active AMAN flight was not found"}
	}
	if len(ids) > 1 {
		return "", &aman.DomainError{Class: aman.ErrorActiveFlightConflict, Message: "callsign resolves to multiple active AMAN flights"}
	}
	return aman.FlightID(ids[0]), nil
}

func (r *amanRepository) LoadHoldingFactSnapshots(ctx context.Context, airport string, ids []aman.FlightID) ([]aman.HoldingFactSnapshot, error) {
	keys := make([]string, len(ids))
	for i, id := range ids {
		keys[i] = string(id)
	}
	rows, err := r.pool.Query(ctx, `-- AMAN holding fact snapshots
SELECT flight_id, vatsim_cid, payload->'HoldingClearance' FROM aman_flights
WHERE airport=$1 AND flight_id=ANY($2::text[])`, airport, keys)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (aman.HoldingFactSnapshot, error) {
		var value aman.HoldingFactSnapshot
		var raw []byte
		err := row.Scan(&value.FlightID, &value.VATSIMCID, &raw)
		if err == nil && len(raw) > 0 {
			err = json.Unmarshal(raw, &value.Clearance)
		}
		return value, err
	})
}
