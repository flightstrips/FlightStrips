package postgres

import (
	"FlightStrips/internal/aman"
	"context"
	"encoding/json"
	"strings"

	"github.com/jackc/pgx/v5"
)

func (r *amanRepository) FindActiveFactFlight(ctx context.Context, airport, callsign string) (aman.Callsign, error) {
	if aman.SessionID(ctx) > 0 {
		state, err := r.LoadAirportState(ctx, airport)
		if err != nil {
			return "", err
		}
		for _, flight := range state.Flights {
			if strings.EqualFold(flight.Callsign, strings.TrimSpace(callsign)) && flight.State != aman.StateLanded && flight.State != aman.StateRemoved {
				return flight.Callsign, nil
			}
		}
		return "", &aman.DomainError{Class: aman.ErrorNotFound, Message: "active AMAN flight was not found in session"}
	}
	rows, err := r.pool.Query(ctx, `-- AMAN active fact identity
SELECT callsign FROM aman_flights
WHERE airport=$1 AND callsign=$2 AND state NOT IN ('landed','removed')
ORDER BY callsign LIMIT 2`, airport, strings.ToUpper(strings.TrimSpace(callsign)))
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
	return aman.Callsign(ids[0]), nil
}

func (r *amanRepository) LoadHoldingFactSnapshots(ctx context.Context, airport string, ids []aman.Callsign) ([]aman.HoldingFactSnapshot, error) {
	if aman.SessionID(ctx) > 0 {
		state, err := r.LoadAirportState(ctx, airport)
		if err != nil {
			return nil, err
		}
		keys := make(map[aman.Callsign]bool, len(ids))
		for _, id := range ids {
			keys[id] = true
		}
		result := make([]aman.HoldingFactSnapshot, 0, len(ids))
		for _, flight := range state.Flights {
			if keys[flight.Callsign] {
				result = append(result, aman.HoldingFactSnapshot{Callsign: flight.Callsign, Clearance: flight.HoldingClearance})
			}
		}
		return result, nil
	}
	keys := make([]string, len(ids))
	for i, id := range ids {
		keys[i] = string(id)
	}
	rows, err := r.pool.Query(ctx, `-- AMAN holding fact snapshots
SELECT callsign, payload->'HoldingClearance' FROM aman_flights
WHERE airport=$1 AND callsign=ANY($2::text[])`, airport, keys)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (aman.HoldingFactSnapshot, error) {
		var value aman.HoldingFactSnapshot
		var raw []byte
		err := row.Scan(&value.Callsign, &raw)
		if err == nil && len(raw) > 0 {
			err = json.Unmarshal(raw, &value.Clearance)
		}
		return value, err
	})
}
