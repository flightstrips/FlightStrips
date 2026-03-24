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
	// BAY_TAXI Used for departures (TWY DEP-UPR — intermediate hold short, apron-only)
	BAY_TAXI = "TAXI"
	// BAY_TAXI_LWR Used for departures at final hold short (TWY DEP-LWR — visible to apron and TWR)
	BAY_TAXI_LWR = "TAXI_LWR"
	BAY_TAXI_TWR = "TAXI_TWR"
	// BAY_DEPART Used for departures (lined up / runway dep — TWR scope)
	BAY_DEPART = "DEPART"
	// BAY_AIRBORNE Used for departures
	BAY_AIRBORNE = "AIRBORNE"
	// BAY_FINAL Used for arrivals
	BAY_FINAL = "FINAL"
	// BAY_RWY_ARR Used for arrivals (TWR scope — runway cleared for landing)
	BAY_RWY_ARR = "RWY_ARR"
	// BAY_TWY_ARR Used for arrivals (vacated runway, taxiing to stand)
	BAY_TWY_ARR = "TWY_ARR"
	// BAY_STAND Used for arrivals
	BAY_STAND       = "STAND"
	BAY_HIDDEN      = "HIDDEN"
	BAY_ARR_HIDDEN  = "ARR_HIDDEN"
	BAY_CONTROLZONE = "CONTROLZONE"
)

const (
	// AirportElevation The airport elevation for EKCH in feet
	AirportElevation = 17
	// AirportLatitude The airport latitude for EKCH in feet
	AirportLatitude = 55.6181
	// AirportLongitude The airport longitude for EKCH in feet
	AirportLongitude = 12.6560

	RelevantDistance = 30
)

func hasKnownPosition(lat, lon float64) bool {
	return lat != 0 || lon != 0
}

func bayTracksGroundState(bay string) bool {
	switch bay {
	case BAY_PUSH, BAY_TAXI, BAY_TAXI_LWR, BAY_TAXI_TWR, BAY_DEPART, BAY_AIRBORNE:
		return true
	default:
		return false
	}
}

func GetDepartureBay(strip euroscope.Strip, existing *database.Strip, airborneAltitudeAGL int64, airport string) string {
	// Arrivals: bay is set once when first seen within range, never changed by this function after that.
	if strip.Destination == airport {
		if existing != nil && existing.Bay != "" {
			return existing.Bay
		}

		return BAY_ARR_HIDDEN
	}

	// Strips not departing from this airport: keep existing bay or hide.
	if strip.Origin != airport {
		if existing != nil && existing.Bay != "" {
			return existing.Bay
		}
		return BAY_HIDDEN
	}

	// Departures from this airport.
	// TODO: airport latitude/longitude should be stored in config, not hardcoded
	if hasKnownPosition(strip.Position.Lat, strip.Position.Lon) &&
		GetDistance(strip.Position.Lat, strip.Position.Lon, AirportLatitude, AirportLongitude) > RelevantDistance {
		return BAY_HIDDEN
	}

	if existing != nil && existing.Bay != "" && existing.State != nil &&
		strip.GroundState == *existing.State && bayTracksGroundState(existing.Bay) {
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

	// If the strip is already at rwy-dep, preserve it and only allow a forward
	// transition to AIRBORNE. Never fall back to CLEARED based on the cleared flag.
	if existing != nil && existing.Bay == BAY_DEPART {
		if int64(strip.Position.Altitude) > int64(AirportElevation)+airborneAltitudeAGL {
			return BAY_AIRBORNE
		}
		return BAY_DEPART
	}

	if !strip.Cleared {
		return BAY_NOT_CLEARED
	}

	if !hasKnownPosition(strip.Position.Lat, strip.Position.Lon) {
		return BAY_CLEARED
	}

	if int64(strip.Position.Altitude) < int64(AirportElevation)+airborneAltitudeAGL {
		return BAY_CLEARED
	}

	return BAY_AIRBORNE
}

func GetDepartureBayFromGroundState(state string, existing database.Strip, airport string) string {
	// Arrivals keep their existing arrival bay; ground-state updates are only used
	// to advance departures through departure-tracking bays.
	if existing.Destination == airport {
		if existing.Bay != "" {
			return existing.Bay
		}
		return BAY_ARR_HIDDEN
	}

	if existing.Origin != airport {
		if existing.Bay != "" {
			return existing.Bay
		}
		return BAY_HIDDEN
	}

	if state == euroscope.GroundStatePush {
		return BAY_PUSH
	}

	if state == euroscope.GroundStateTaxi {
		return BAY_TAXI
	}

	if state == euroscope.GroundStateDepart || state == euroscope.GroundStateLineup {
		return BAY_DEPART
	}

	if existing.Bay != "" {
		return existing.Bay
	}
	return BAY_HIDDEN
}

func GetDepartureBayFromPosition(lat, lon float64, alt int64, existing database.Strip, airborneAltitudeAGL int64, airport string) string {
	// Arrivals: position updates never change the bay (set once in GetDepartureBay).
	if existing.Destination == airport {
		if existing.Bay != "" {
			return existing.Bay
		}
		return BAY_ARR_HIDDEN
	}

	// Resolve the existing bay, falling back to HIDDEN if it was never set.
	existingBay := existing.Bay
	if existingBay == "" {
		existingBay = BAY_HIDDEN
	}

	// Non-departures from this airport: keep existing bay unchanged.
	if existing.Origin != airport {
		return existingBay
	}

	// Departures from this airport.
	if !hasKnownPosition(lat, lon) {
		return existingBay
	}

	if GetDistance(lat, lon, AirportLatitude, AirportLongitude) > RelevantDistance {
		return BAY_HIDDEN
	}

	bay := existingBay

	if bay != BAY_DEPART {
		return bay
	}

	if alt > int64(AirportElevation)+airborneAltitudeAGL {
		return BAY_AIRBORNE
	}

	return bay
}

func GetGroundState(bay string) string {
	if bay == BAY_PUSH {
		return euroscope.GroundStatePush
	}
	if bay == BAY_TAXI || bay == BAY_TAXI_LWR || bay == BAY_TAXI_TWR {
		return euroscope.GroundStateTaxi
	}
	// Drag-and-drop to rwy-dep sets LINEUP. DEPA is only set once runway_cleared is triggered.
	if bay == BAY_DEPART {
		return euroscope.GroundStateLineup
	}

	return euroscope.GroundStateUnknown
}

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
