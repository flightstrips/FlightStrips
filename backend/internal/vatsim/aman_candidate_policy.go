package vatsim

import (
	"FlightStrips/internal/aman"
	"context"
	"time"
)

// MapAMANObservation shares the production flight-plan, timing, takeoff and
// surveillance mapping without the process-local observation worker cache.
func MapAMANObservation(flight Flight, snapshotAt time.Time, status aman.DataStatus, at time.Time, previous *aman.FlightObservation) (aman.FlightObservation, error) {
	next, err := (&ObservationWorker{}).mapFlight(context.Background(), flight, snapshotAt, status, at, previous)
	if err == nil && previous != nil {
		next = preserveNewerObservationFacts(*previous, next)
	}
	return next, err
}
