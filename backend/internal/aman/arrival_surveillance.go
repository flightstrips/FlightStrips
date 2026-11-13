package aman

import (
	"math"
	"time"
)

const ArrivalSurveillanceFreshness = 2 * time.Minute

// HasFreshArrivalSurveillance checks aircraft observations independently of
// prediction quality, so a failed route calculation can still be corrected.
func HasFreshArrivalSurveillance(o *FlightObservation, now time.Time) bool {
	if o == nil || o.Missing || o.SourceStatus != DataFresh || o.Surveillance == nil {
		return false
	}
	f := o.Surveillance
	if f.ObservedAt == nil || f.ObservedAt.After(now) || now.Sub(*f.ObservedAt) > ArrivalSurveillanceFreshness || f.GroundspeedKnots == nil || f.AltitudeFeet == nil {
		return false
	}
	speed := *f.GroundspeedKnots
	if math.IsNaN(speed) || math.IsInf(speed, 0) || speed <= 0 || *f.AltitudeFeet < 0 || *f.AltitudeFeet < 1000 && speed <= 50 {
		return false
	}
	return !math.IsNaN(f.LatitudeDegrees) && !math.IsInf(f.LatitudeDegrees, 0) && f.LatitudeDegrees >= -90 && f.LatitudeDegrees <= 90 &&
		!math.IsNaN(f.LongitudeDegrees) && !math.IsInf(f.LongitudeDegrees, 0) && f.LongitudeDegrees >= -180 && f.LongitudeDegrees <= 180
}
