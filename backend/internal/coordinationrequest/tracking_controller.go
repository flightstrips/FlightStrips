package coordinationrequest

import (
	"FlightStrips/internal/aman"
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
)

// TrackingController resolves the current strip fact for the AMAN flight.
func (r *Repository) TrackingController(ctx context.Context, airport string, callsign Callsign) (ControllerID, error) {
	var controller string
	if id := aman.SessionID(ctx); id > 0 {
		err := r.pool.QueryRow(ctx, `SELECT COALESCE(s.tracking_controller,'') FROM strips s
JOIN sessions se ON se.id=s.session WHERE s.session=$1 AND se.airport=$2 AND upper(s.callsign)=upper($3)`, id, airport, callsign).Scan(&controller)
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return ControllerID(strings.TrimSpace(controller)), err
	}
	err := r.pool.QueryRow(ctx, `SELECT COALESCE(s.tracking_controller, '') FROM aman_flights f
		JOIN sessions se ON se.airport=f.airport JOIN strips s ON s.session=se.id AND upper(s.callsign)=upper(f.callsign)
		WHERE f.airport=$1 AND f.callsign=$2 ORDER BY se.id DESC LIMIT 1`, airport, callsign).Scan(&controller)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return ControllerID(strings.TrimSpace(controller)), err
}
