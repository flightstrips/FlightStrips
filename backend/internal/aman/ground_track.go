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

// DerivedGroundspeed fills missing ES motion data from fresh, successive positions.
// A gap beyond the surveillance freshness window cannot establish current speed.
func DerivedGroundspeed(previous *FlightObservation, current *SurveillanceFact) (float64, bool) {
	if previous == nil || previous.Surveillance == nil || previous.Surveillance.ObservedAt == nil || current == nil || current.ObservedAt == nil {
		return 0, false
	}
	from := previous.Surveillance
	seconds := current.ObservedAt.Sub(*from.ObservedAt).Seconds()
	if seconds <= 0 || seconds > 30 {
		return 0, false
	}
	lat1, lat2 := from.LatitudeDegrees*math.Pi/180, current.LatitudeDegrees*math.Pi/180
	dlat, dlon := lat2-lat1, (current.LongitudeDegrees-from.LongitudeDegrees)*math.Pi/180
	a := math.Pow(math.Sin(dlat/2), 2) + math.Cos(lat1)*math.Cos(lat2)*math.Pow(math.Sin(dlon/2), 2)
	distance := 3440.065 * 2 * math.Atan2(math.Sqrt(math.Min(1, a)), math.Sqrt(math.Max(0, 1-a)))
	knots := distance * 3600 / seconds
	if math.IsNaN(knots) || math.IsInf(knots, 0) || knots <= 0 || knots > 1200 {
		return 0, false
	}
	return knots, true
}
