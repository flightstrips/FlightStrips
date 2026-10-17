package trajectory

import (
	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/navdata"
	"math"
	"time"
)

// HoldingEntry detects a sampled crossing independently of route progress,
// which an active holding clearance deliberately pins at the holding fix.
func HoldingEntry(previous *aman.SurveillanceFact, current aman.SurveillanceFact, fix navdata.Coordinate) (*time.Time, string) {
	if current.ObservedAt == nil {
		return nil, ""
	}
	point := coordinate(current.LatitudeDegrees, current.LongitudeDegrees)
	if previous != nil && previous.ObservedAt != nil && current.ObservedAt.After(*previous.ObservedAt) {
		elapsed := current.ObservedAt.Sub(*previous.ObservedAt)
		start := coordinate(previous.LatitudeDegrees, previous.LongitudeDegrees)
		distance := wgs84NM(start, point)
		if elapsed <= 2*time.Minute && distance > 0 && distance/elapsed.Hours() <= 700 {
			fraction, cross := geodesicProject(fix, start, point, distance)
			if cross <= 1 && fraction > 0 && fraction < 1 {
				at := previous.ObservedAt.Add(time.Duration(math.Round(float64(elapsed) * fraction)))
				return &at, "fix_crossing"
			}
		}
	}
	if wgs84NM(point, fix) <= 1 {
		at := *current.ObservedAt
		return &at, "near_fix"
	}
	return nil, ""
}
