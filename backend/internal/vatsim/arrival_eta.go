package vatsim

import (
	"FlightStrips/internal/models"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

const (
	ETAFiled = "FILED"
	ETALive  = "LIVE"

	minimumLiveGroundspeed = 100
	minimumLiveAltitude    = 1000
	etaHysteresis          = 2 * time.Minute
)

// AirportCoordinates is the configured destination used for live ETA
// calculations. A zero location intentionally disables live estimation and
// lets the filed estimate remain authoritative.
type AirportCoordinates struct {
	Latitude  float64
	Longitude float64
}

// ArrivalETAOption configures the ETA portion of a reconciler. Options keep
// the existing VATSIM reconciliation constructor stable for callers that do
// not need a live calculation or a controlled clock.
type ArrivalETAOption func(*Reconciler)

// WithLegacyArrivalETAWriter controls whether VATSIM reconciliation owns the
// accepted ArrivalETA field. AMAN read-only and authoritative modes disable
// this legacy writer before reconciliation begins.
func WithLegacyArrivalETAWriter(enabled bool) ArrivalETAOption {
	return func(r *Reconciler) {
		r.legacyArrivalETAWriter = enabled
	}
}

// WithAirportCoordinates enables live ETA calculation against the configured
// airport location.
func WithAirportCoordinates(latitude, longitude float64) ArrivalETAOption {
	return func(r *Reconciler) {
		r.airportCoordinates = AirportCoordinates{Latitude: latitude, Longitude: longitude}
	}
}

// WithClock injects time for deterministic ETA and reveal-boundary tests.
func WithClock(now func() time.Time) ArrivalETAOption {
	return func(r *Reconciler) {
		if now != nil {
			r.now = now
		}
	}
}

// calculateArrivalETA returns the best usable estimate. Live estimates require
// a connected flight, a clearly airborne altitude, a non-ground groundspeed,
// and a configured airport. Otherwise the filed EOBT plus enroute duration is
// used when available.
func calculateArrivalETA(now time.Time, flight Flight, airport AirportCoordinates) (models.ArrivalETA, bool) {
	if reliableLiveMovement(flight, airport) {
		distance := greatCircleDistanceNM(flight.Latitude, flight.Longitude, airport.Latitude, airport.Longitude)
		eta := now.UTC().Add(time.Duration(float64(distance) / float64(flight.Groundspeed) * float64(time.Hour))).Round(time.Minute)
		groundspeed := int32(flight.Groundspeed)
		return models.ArrivalETA{
			Time:            eta,
			Source:          ETALive,
			CalculatedAt:    now.UTC(),
			EOBT:            strings.TrimSpace(flight.FlightPlan.EOBT),
			EnrouteDuration: strings.TrimSpace(flight.FlightPlan.EnrouteDuration),
			DistanceNM:      &distance,
			Groundspeed:     &groundspeed,
		}, true
	}

	eta, err := filedArrivalETA(now, flight.FlightPlan.EOBT, flight.FlightPlan.EnrouteDuration)
	if err != nil {
		return models.ArrivalETA{}, false
	}
	return models.ArrivalETA{
		Time:            eta,
		Source:          ETAFiled,
		CalculatedAt:    now.UTC(),
		EOBT:            strings.TrimSpace(flight.FlightPlan.EOBT),
		EnrouteDuration: strings.TrimSpace(flight.FlightPlan.EnrouteDuration),
	}, true
}

func reliableLiveMovement(flight Flight, airport AirportCoordinates) bool {
	return flight.Online() &&
		flight.Altitude >= minimumLiveAltitude &&
		flight.Groundspeed >= minimumLiveGroundspeed &&
		validCoordinates(flight.Latitude, flight.Longitude) &&
		validCoordinates(airport.Latitude, airport.Longitude)
}

func validCoordinates(latitude, longitude float64) bool {
	return latitude >= -90 && latitude <= 90 && longitude >= -180 && longitude <= 180 && (latitude != 0 || longitude != 0)
}

// filedArrivalETA interprets VATSIM's time-only fields relative to the UTC
// service day nearest to now. Choosing the nearest EOBT first correctly covers
// flights around midnight before adding the planned enroute duration.
func filedArrivalETA(now time.Time, eobt, enroute string) (time.Time, error) {
	hour, minute, err := parseClock(eobt)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse EOBT: %w", err)
	}
	duration, err := parseDuration(enroute)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse enroute duration: %w", err)
	}
	now = now.UTC()
	candidates := []time.Time{
		time.Date(now.Year(), now.Month(), now.Day()-1, hour, minute, 0, 0, time.UTC),
		time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, time.UTC),
		time.Date(now.Year(), now.Month(), now.Day()+1, hour, minute, 0, 0, time.UTC),
	}
	base := candidates[0]
	for _, candidate := range candidates[1:] {
		if absoluteDuration(candidate.Sub(now)) < absoluteDuration(base.Sub(now)) {
			base = candidate
		}
	}
	return base.Add(duration), nil
}

func parseClock(value string) (int, int, error) {
	value = strings.ReplaceAll(strings.TrimSpace(value), ":", "")
	if len(value) != 4 {
		return 0, 0, fmt.Errorf("expected HHMM, got %q", value)
	}
	hour, hourErr := strconv.Atoi(value[:2])
	minute, minuteErr := strconv.Atoi(value[2:])
	if hourErr != nil || minuteErr != nil || hour > 23 || minute > 59 {
		return 0, 0, fmt.Errorf("invalid HHMM %q", value)
	}
	return hour, minute, nil
}

func parseDuration(value string) (time.Duration, error) {
	hour, minute, err := parseClock(value)
	if err != nil {
		return 0, err
	}
	return time.Duration(hour)*time.Hour + time.Duration(minute)*time.Minute, nil
}

func absoluteDuration(value time.Duration) time.Duration {
	if value < 0 {
		return -value
	}
	return value
}

// greatCircleDistanceNM uses the haversine formula and returns nautical miles.
func greatCircleDistanceNM(latitudeA, longitudeA, latitudeB, longitudeB float64) float64 {
	const earthRadiusNM = 3440.065
	latA := latitudeA * math.Pi / 180
	latB := latitudeB * math.Pi / 180
	deltaLat := (latitudeB - latitudeA) * math.Pi / 180
	deltaLon := (longitudeB - longitudeA) * math.Pi / 180
	a := math.Sin(deltaLat/2)*math.Sin(deltaLat/2) + math.Cos(latA)*math.Cos(latB)*math.Sin(deltaLon/2)*math.Sin(deltaLon/2)
	return earthRadiusNM * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

func acceptedArrivalETA(previous *models.ArrivalETA, candidate models.ArrivalETA) (models.ArrivalETA, bool) {
	if previous == nil || previous.Time.IsZero() {
		return candidate, true
	}
	if previous.Source == candidate.Source && absoluteDuration(candidate.Time.Sub(previous.Time)) <= etaHysteresis {
		return *previous, false
	}
	if absoluteDuration(candidate.Time.Sub(previous.Time)) <= etaHysteresis {
		candidate.Time = previous.Time
	}
	return candidate, true
}
