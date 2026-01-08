package shared

import (
	"FlightStrips/internal/database"
	"FlightStrips/pkg/events/euroscope"
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
	BAY_TAXI     = "TAXI"
	BAY_TAXI_TWR = "TAXI_TWR"
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

func GetDepartureBay(strip euroscope.Strip, existing *database.Strip) string {
	// TODO handle arrivals in this method. Maybe split into two but the entry point should be in this file and the same
	// for both arrivals and departures.
	if strip.Origin != "EKCH" {
		if existing.Bay != "" {
			return existing.Bay
		}
		return BAY_HIDDEN
	}

	if GetDistance(strip.Position.Lat, strip.Position.Lon, AirportLatitude, AirportLongitude) > RelevantDistance {
		return BAY_HIDDEN
	}

	if existing.Bay != "" && existing.State != nil && strip.GroundState == *existing.State {
		return existing.Bay
	}

	if strip.GroundState == euroscope.GroundStatePush {
		return BAY_PUSH
	}

	if strip.GroundState == euroscope.GroundStateTaxi {
		return BAY_TAXI
	}

	if strip.GroundState == euroscope.GroundStateDepart {
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

func GetDepartureBayFromGroundState(state string, existing database.Strip) string {
	if state == euroscope.GroundStatePush {
		return BAY_PUSH
	}

	if state == euroscope.GroundStateTaxi {
		return BAY_TAXI
	}

	if state == euroscope.GroundStateDepart {
		return BAY_DEPART
	}

	return existing.Bay
}

func GetDepartureBayFromPosition(lat, lon float64, alt int64, existing database.Strip) string {
	if GetDistance(lat, lon, AirportLatitude, AirportLongitude) > RelevantDistance {
		return BAY_HIDDEN
	}

	bay := existing.Bay

	if existing.Origin != "EKCH" {
		return bay
	}

	if bay != BAY_DEPART || existing.State == nil || *existing.State != BAY_AIRBORNE {
		return bay
	}

	if alt > AirportElevation+AltitudeErrorMargin {
		return BAY_AIRBORNE
	}

	return bay
}

func GetGroundState(bay string) string {
	if bay == BAY_PUSH {
		return euroscope.GroundStatePush
	}
	if bay == BAY_TAXI || bay == BAY_TAXI_TWR {
		return euroscope.GroundStateTaxi
	}
	if bay == BAY_DEPART {
		return euroscope.GroundStateDepart
	}

	return euroscope.GroundStateUnknown
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
