package sat

import (
	"math"
	"strings"
)

const earthRadiusMetres = 6371000.0

// StandAtPosition returns the closest physical stand whose configured radius
// contains the supplied position. Overlapping radii are resolved by distance,
// with exact distance ties resolved by lexical stand name.
func (r *StandCapabilityRegistry) StandAtPosition(airport string, latitude, longitude float64) (Stand, bool) {
	if r == nil {
		return Stand{}, false
	}
	stands := r.byAirport[strings.ToUpper(strings.TrimSpace(airport))]
	closestDistance := math.MaxFloat64
	var closest Stand
	found := false
	for _, stand := range stands {
		if stand.Radius <= 0 || latitudeDistanceExceeds(latitude, stand.Latitude, stand.Radius) {
			continue
		}
		distance := greatCircleMetres(latitude, longitude, stand.Latitude, stand.Longitude)
		if distance <= stand.Radius && (distance < closestDistance || found && distance == closestDistance && stand.Name < closest.Name) {
			closest, closestDistance, found = stand, distance, true
		}
	}
	return closest, found
}

// PositionNearAirport reports whether a position is within maxDistanceMetres
// of any configured stand at the airport. Stand centres provide a stable
// airport-local reference without coupling SAT geometry to one airport's
// coordinates or layout.
func (r *StandCapabilityRegistry) PositionNearAirport(airport string, latitude, longitude, maxDistanceMetres float64) bool {
	if r == nil || maxDistanceMetres < 0 {
		return false
	}
	stands := r.byAirport[strings.ToUpper(strings.TrimSpace(airport))]
	for _, stand := range stands {
		if !latitudeDistanceExceeds(latitude, stand.Latitude, maxDistanceMetres) && greatCircleMetres(latitude, longitude, stand.Latitude, stand.Longitude) <= maxDistanceMetres {
			return true
		}
	}
	return false
}

func greatCircleMetres(lat1, lon1, lat2, lon2 float64) float64 {
	toRadians := math.Pi / 180
	lat1, lon1, lat2, lon2 = lat1*toRadians, lon1*toRadians, lat2*toRadians, lon2*toRadians
	dLat, dLon := lat2-lat1, lon2-lon1
	a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(lat1)*math.Cos(lat2)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return earthRadiusMetres * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

// A spherical distance cannot be smaller than its latitude separation. Reject
// distant stands with this lower bound before evaluating trigonometric distance.
// Longitude remains unrestricted, including polar and antimeridian positions.
// The small rounding allowance keeps the exact great-circle boundary decision.
func latitudeDistanceExceeds(latitude, standLatitude, radius float64) bool {
	if latitude < -90 || latitude > 90 || standLatitude < -90 || standLatitude > 90 {
		return false
	}
	return math.Abs(latitude-standLatitude)*(earthRadiusMetres*math.Pi/180) > radius+0.000001
}
