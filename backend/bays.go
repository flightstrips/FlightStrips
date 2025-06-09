package main

import (
	"FlightStrips/data"
	"math"
)

const (
	BAY_UNKNOWN = "UNKNOWN"
	// BAY_NOT_CLEARED Used for departures
	BAY_NOT_CLEARED = "NOT_CLEARED"
	// BAY_CLEARED Used for departures
	BAY_CLEARED = "CLEARED"
	// BAY_PUSH Used for departures
	BAY_PUSH = "PUSH"
	// BAY_TAXI Used for departures and arrivals
	BAY_TAXI = "TAXI"
	// BAY_DEPART Used for departures
	BAY_DEPART = "DEPART"
	// BAY_AIRBORNE Used for departures
	BAY_AIRBORNE = "AIRBORNE"
	// BAY_FINAL Used for arrivals
	BAY_FINAL = "FINAL"
	// BAY_STAND Used for arrivals
	BAY_STAND  = "STAND"
	BAY_HIDDEN = "HIDDEN"
)

const (
	// AirportElevation The airport elevation for EKCH in feet
	AirportElevation = 17
	// AirportLatitude The airport latitude for EKCH in feet
	AirportLatitude = 55.6181
	// AirportLongitude The airport longitude for EKCH in feet
	AirportLongitude = 12.6560

	AltitudeErrorMargin = 200
	RelevantDistance    = 20
)

func GetDepartureBay(strip EuroscopeStrip, existing *data.Strip) string {
	// TODO handle arrivals in this method. Maybe split into two but the entry point should be in this file and the same
	// for both arrivals and departures.
	if strip.Origin != "EKCH" {
		if existing != nil {
			return existing.Bay.String
		}
		return BAY_HIDDEN
	}

	if GetDistance(strip.Position.Lat, strip.Position.Lon, AirportLatitude, AirportLongitude) > RelevantDistance {
		return BAY_HIDDEN
	}

	if existing != nil && existing.Bay.Valid && strip.GroundState == existing.State.String && existing.Bay.String != "" {
		return existing.Bay.String
	}

	if strip.GroundState == EuroscopeGroundStatePush {
		return BAY_PUSH
	}

	if strip.GroundState == EuroscopeGroundStateTaxi {
		return BAY_TAXI
	}

	if strip.GroundState == EuroscopeGroundStateDepart {
		return BAY_DEPART
	}

	if !strip.Cleared {
		return BAY_NOT_CLEARED
	}

	if strip.Position.Altitude < AirportElevation+AltitudeErrorMargin {
		return BAY_CLEARED
	}

	return BAY_AIRBORNE
}

func GetDepartureBayFromGroundState(state string, existing data.Strip) string {
	if state == EuroscopeGroundStatePush {
		return BAY_PUSH
	}

	if state == EuroscopeGroundStateTaxi {
		return BAY_TAXI
	}

	if state == EuroscopeGroundStateDepart {
		return BAY_DEPART
	}

	return existing.Bay.String
}

func GetDepartureBayFromPosition(lat, lon float64, alt int64, existing data.Strip) string {
	if GetDistance(lat, lon, AirportLatitude, AirportLongitude) > RelevantDistance {
		return BAY_HIDDEN
	}

	if existing.Origin != "EKCH" {
		return existing.Bay.String
	}

	if !existing.Bay.Valid || existing.Bay.String != BAY_DEPART || existing.State.String != BAY_AIRBORNE {
		return existing.Bay.String
	}

	if alt > AirportElevation+AltitudeErrorMargin {
		return BAY_AIRBORNE
	}

	return existing.Bay.String
}

func GetGroundState(bay string) string {
	if bay == BAY_PUSH {
		return EuroscopeGroundStatePush
	}
	if bay == BAY_TAXI {
		return EuroscopeGroundStateTaxi
	}
	if bay == BAY_DEPART {
		return EuroscopeGroundStateDepart
	}

	return EuroscopeGroundStateUnknown
}

// TODO arrival bays. This needs to not auto move the strips as much as strips must be dragged to the runway bay.

func GetDistance(lat1 float64, lon1 float64, lat2 float64, lon2 float64) float64 {
	const earthRadiusNM = 3440.065 // Earth radius in nautical miles

	lat1Rad := lat1 * math.Pi / 180
	lon1Rad := lon1 * math.Pi / 180
	lat2Rad := lat2 * math.Pi / 180
	lon2Rad := lon2 * math.Pi / 180

	dlat := lat2Rad - lat1Rad
	dlon := lon2Rad - lon1Rad

	a := math.Sin(dlat/2)*math.Sin(dlat/2) + math.Cos(lat1Rad)*math.Cos(lat2Rad)*math.Sin(dlon/2)*math.Sin(dlon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return earthRadiusNM * c
}
