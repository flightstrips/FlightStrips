package coordinationrequest

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
)

// TrackingController resolves the current strip fact for the AMAN flight.
func (r *Repository) TrackingController(ctx context.Context, airport string, flightID FlightID) (ControllerID, error) {
	var controller string
	err := r.pool.QueryRow(ctx, `SELECT COALESCE(s.tracking_controller, '') FROM aman_flights f
		JOIN sessions se ON se.airport=f.airport JOIN strips s ON s.session=se.id AND upper(s.callsign)=upper(f.current_callsign)
		WHERE f.airport=$1 AND f.flight_id=$2 ORDER BY se.id DESC LIMIT 1`, airport, flightID).Scan(&controller)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return ControllerID(strings.TrimSpace(controller)), err
}
