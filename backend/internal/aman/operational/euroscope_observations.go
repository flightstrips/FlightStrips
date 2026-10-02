package operational

import (
	"FlightStrips/internal/aman"
	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"context"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"
)

const euroScopeMaximumDerivedGroundspeed = 700.0

// EuroScopePositionObserver maps authoritative EuroScope position reports to
// the provider-neutral AMAN observation contract. AMAN identifies flights by
// normalized callsign; the EuroScope session only isolates position history.
type EuroScopePositionObserver struct {
	sink     aman.ObservationSink
	airports map[string]struct{}
	now      func() time.Time

	mu       sync.Mutex
	previous map[string]euroScopePosition
}

type EuroScopePositionObserverDependencies struct {
	Sink            aman.ObservationSink
	EnabledAirports []string
	Now             func() time.Time
}

type euroScopePosition struct {
	latitude, longitude float64
	at                  time.Time
}

func NewEuroScopePositionObserver(deps EuroScopePositionObserverDependencies) (*EuroScopePositionObserver, error) {
	if deps.Sink == nil {
		return nil, fmt.Errorf("EuroScope AMAN position observer requires observation sink")
	}
	airports := map[string]struct{}{}
	for _, airport := range deps.EnabledAirports {
		if value := strings.ToUpper(strings.TrimSpace(airport)); value != "" {
			airports[value] = struct{}{}
		}
	}
	if len(airports) == 0 {
		return nil, fmt.Errorf("EuroScope AMAN position observer requires enabled airport")
	}
	if deps.Now == nil {
		deps.Now = time.Now
	}
	return &EuroScopePositionObserver{sink: deps.Sink, airports: airports, now: deps.Now, previous: map[string]euroScopePosition{}}, nil
}

// ObserveEuroScopePosition is called from the EuroScope strip path. The
// first report creates the flight and establishes a reliable reference; every
// later coherent report derives the ground speed and true track needed by the
// physical predictor.
func (o *EuroScopePositionObserver) ObserveEuroScopePosition(ctx context.Context, session int32, strip *models.Strip, latitude, longitude float64, altitude int32) error {
	if strip == nil {
		return nil
	}
	destination := strings.ToUpper(strings.TrimSpace(strip.Destination))
	if _, enabled := o.airports[destination]; !enabled {
		return nil
	}
	callsign, origin := strings.ToUpper(strings.TrimSpace(strip.Callsign)), strings.ToUpper(strings.TrimSpace(strip.Origin))
	if callsign == "" || origin == "" {
		return nil
	}
	filedRoute := optionalStripString(strip.Route)
	if latitude < -90 || latitude > 90 || longitude < -180 || longitude > 180 {
		return fmt.Errorf("EuroScope AMAN position has invalid coordinates")
	}
	now := shared.ReceiptTime(ctx, o.now).UTC()
	current := euroScopePosition{latitude: latitude, longitude: longitude, at: now}
	if session == 0 {
		session = strip.Session
	}
	positionKey := fmt.Sprintf("%d\x00%s", session, callsign)
	o.mu.Lock()
	previous, known := o.previous[positionKey]
	o.previous[positionKey] = current
	o.mu.Unlock()
	var groundspeed, track *float64
	if known && current.at.After(previous.at) {
		interval := current.at.Sub(previous.at)
		if interval <= 2*time.Minute {
			distance := geoDistanceNM(previous.latitude, previous.longitude, current.latitude, current.longitude)
			derivedGroundspeed := distance / interval.Hours()
			if derivedGroundspeed >= 1 && derivedGroundspeed <= euroScopeMaximumDerivedGroundspeed {
				derivedTrack := geoBearingTrue(previous.latitude, previous.longitude, current.latitude, current.longitude)
				groundspeed, track = &derivedGroundspeed, &derivedTrack
			}
		}
	}
	altitudeFeet := int(altitude)
	observation := aman.FlightObservation{
		Callsign: callsign,
		Origin:   origin, Destination: destination,
		AircraftType: optionalStripString(strip.AircraftType), FiledRoute: filedRoute, RequestedLevel: requestedLevel(strip.RequestedAltitude),
		FlightPlan:         aman.FlightPlanFact{Revision: vatsimRevision(strip.VatsimRevision), ObservedAt: &now},
		Surveillance:       &aman.SurveillanceFact{LatitudeDegrees: latitude, LongitudeDegrees: longitude, AltitudeFeet: &altitudeFeet, GroundspeedKnots: groundspeed, TrackTrueDegrees: track, ObservedAt: &now},
		SurveillanceSource: aman.SurveillanceSourceEuroScope, Provider: aman.ObservationProviderEuroScope, ReconciledAt: now, SourceStatus: aman.DataFresh,
	}
	if groundspeed != nil && altitude > 1000 && *groundspeed > 40 {
		observation.TakeoffDetected = &now
	}
	if err := observation.Validate(); err != nil {
		return fmt.Errorf("map EuroScope AMAN observation: %w", err)
	}
	if err := o.sink.Observe(ctx, observation); err != nil {
		return fmt.Errorf("publish EuroScope AMAN observation: %w", err)
	}
	return nil
}

func stripStringValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func optionalStripString(value *string) *string {
	if text := stripStringValue(value); text != "" {
		return &text
	}
	return nil
}

func requestedLevel(value *int32) *int {
	if value == nil || *value < 0 {
		return nil
	}
	level := int(*value)
	return &level
}

func vatsimRevision(value *int64) *uint64 {
	if value == nil || *value < 0 {
		return nil
	}
	revision := uint64(*value)
	return &revision
}

func geoDistanceNM(latitudeA, longitudeA, latitudeB, longitudeB float64) float64 {
	return math.Hypot((latitudeA-latitudeB)*60, (longitudeA-longitudeB)*60*math.Cos((latitudeA+latitudeB)*math.Pi/360))
}

func geoBearingTrue(latitudeA, longitudeA, latitudeB, longitudeB float64) float64 {
	if latitudeA == latitudeB && longitudeA == longitudeB {
		return 0
	}
	latitudeARadians, latitudeBRadians := latitudeA*math.Pi/180, latitudeB*math.Pi/180
	deltaLongitude := (longitudeB - longitudeA) * math.Pi / 180
	y := math.Sin(deltaLongitude) * math.Cos(latitudeBRadians)
	x := math.Cos(latitudeARadians)*math.Sin(latitudeBRadians) - math.Sin(latitudeARadians)*math.Cos(latitudeBRadians)*math.Cos(deltaLongitude)
	return math.Mod(math.Atan2(y, x)*180/math.Pi+360, 360)
}
