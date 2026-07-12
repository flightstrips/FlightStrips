package sat

import (
	"math"
	"strings"
)

const earthRadiusMetres = 6371000.0

// StandAtPosition returns the closest physical stand whose configured radius
// contains the supplied position. Overlapping radii are resolved by distance,
// which makes the observed result deterministic.
func (r *StandCapabilityRegistry) StandAtPosition(airport string, latitude, longitude float64) (Stand, bool) {
	if r == nil {
		return Stand{}, false
	}
	stands := r.byAirport[strings.ToUpper(strings.TrimSpace(airport))]
	closestDistance := math.MaxFloat64
	var closest Stand
	found := false
	for _, stand := range stands {
		if stand.Radius <= 0 {
			continue
		}
		distance := greatCircleMetres(latitude, longitude, stand.Latitude, stand.Longitude)
		if distance <= stand.Radius && distance < closestDistance {
			closest, closestDistance, found = stand, distance, true
		}
	}
	return closest, found
}

func greatCircleMetres(lat1, lon1, lat2, lon2 float64) float64 {
	toRadians := math.Pi / 180
	lat1, lon1, lat2, lon2 = lat1*toRadians, lon1*toRadians, lat2*toRadians, lon2*toRadians
	dLat, dLon := lat2-lat1, lon2-lon1
	a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(lat1)*math.Cos(lat2)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return earthRadiusMetres * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}
