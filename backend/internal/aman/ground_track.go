package aman

import "math"

// DerivedGroundTrack calculates true ground track from successive surveillance
// positions. Missing, unchanged and non-increasing samples have no known track.
func DerivedGroundTrack(previous *FlightObservation, current *SurveillanceFact) (float64, bool) {
	if previous == nil || previous.Surveillance == nil || previous.Surveillance.ObservedAt == nil || current == nil || current.ObservedAt == nil || !current.ObservedAt.After(*previous.Surveillance.ObservedAt) {
		return 0, false
	}
	from := previous.Surveillance
	if from.LatitudeDegrees == current.LatitudeDegrees && from.LongitudeDegrees == current.LongitudeDegrees {
		return 0, false
	}
	lat1, lat2 := from.LatitudeDegrees*math.Pi/180, current.LatitudeDegrees*math.Pi/180
	dLon := (current.LongitudeDegrees - from.LongitudeDegrees) * math.Pi / 180
	y := math.Sin(dLon) * math.Cos(lat2)
	x := math.Cos(lat1)*math.Sin(lat2) - math.Sin(lat1)*math.Cos(lat2)*math.Cos(dLon)
	return math.Mod(math.Atan2(y, x)*180/math.Pi+360, 360), true
}
