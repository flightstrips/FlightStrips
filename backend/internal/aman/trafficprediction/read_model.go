// Package trafficprediction builds the authoritative, presentation-ready TMT
// traffic prediction from the persisted AMAN airport aggregate.
package trafficprediction

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
	"time"

	"FlightStrips/internal/aman"
)

const (
	BucketDuration = 15 * time.Minute
	Horizon        = 3 * time.Hour
)

type Status string

const (
	StatusReady        Status = "ready"
	StatusDegraded     Status = "degraded"
	StatusDisconnected Status = "disconnected"
)

type Alert string

const (
	AlertNone   Alert = "none"
	AlertYellow Alert = "yellow"
	AlertRed    Alert = "red"
)

type TimingSource string

// AirportPosition supplies the configured destination for a degraded
// airborne estimate when accepted AMAN timing is unavailable.
type AirportPosition struct {
	LatitudeDegrees  float64
	LongitudeDegrees float64
}

const (
	SourceAMAN             TimingSource = "aman"
	SourceVATSIMPlanned    TimingSource = "vatsim_planned"
	SourceVATSIMAirborne   TimingSource = "vatsim_airborne"
	SourceAirbornePosition TimingSource = "airborne_position"
)

type ReadModel struct {
	GeneratedAt     time.Time
	RangeStart      time.Time
	RangeEnd        time.Time
	BucketMinutes   int
	SourceStatus    aman.DataStatus
	Status          Status
	DegradedReasons []string
	Buckets         []Bucket
}

type Bucket struct {
	Start         time.Time
	End           time.Time
	PlannedCount  int
	AirborneCount int
	Count         int
	LoadFactor    int
	SelectedRate  *SelectedRate
	BucketHigh    bool
	WindowHigh    bool
	Alert         Alert
	Flights       []Flight
}

type SelectedRate struct {
	RunwayGroupID   aman.RunwayGroupID
	ArrivalsPerHour uint32
	EffectiveAt     time.Time
}

type Flight struct {
	Callsign     string
	Airborne     bool
	LandingAt    time.Time
	TimingSource TimingSource
	DataStatus   aman.DataStatus
}

type candidate struct {
	flight        aman.AMANFlight
	landingAt     time.Time
	timingSource  TimingSource
	authoritative bool
	hasTiming     bool
}

func Build(state aman.AirportState, sourceHealth aman.ComponentHealth) ReadModel {
	return BuildWithAirportPosition(state, sourceHealth, AirportPosition{})
}

// BuildWithAirportPosition also estimates nearby airborne arrivals from fresh
// surveillance when the configured airport position is available.
func BuildWithAirportPosition(state aman.AirportState, sourceHealth aman.ComponentHealth, airport AirportPosition) ReadModel {
	start := floorQuarter(state.GeneratedAt)
	sourceStatus := sourceDataStatus(sourceHealth)
	result := ReadModel{
		GeneratedAt: state.GeneratedAt.UTC(), RangeStart: start, RangeEnd: start.Add(Horizon),
		BucketMinutes: int(BucketDuration / time.Minute), SourceStatus: sourceStatus, Status: StatusReady,
		Buckets: make([]Bucket, int(Horizon/BucketDuration)),
	}
	if sourceStatus == aman.DataDisconnected {
		result.Status = StatusDisconnected
		addReason(&result, "source_disconnected")
	}
	if sourceStatus == aman.DataStale {
		addReason(&result, "stale_observation_source")
	}
	for index := range result.Buckets {
		bucketStart := start.Add(time.Duration(index) * BucketDuration)
		result.Buckets[index] = Bucket{Start: bucketStart, End: bucketStart.Add(BucketDuration), Alert: AlertNone}
		result.Buckets[index].SelectedRate = selectedRateAt(state.RunwayGroups, bucketStart)
		if result.Buckets[index].SelectedRate == nil {
			addReason(&result, "missing_selected_rate")
		}
	}

	deduplicated := make(map[string]candidate, len(state.Flights))
	for _, flight := range state.Flights {
		if flight.State == aman.StateLanded || flight.State == aman.StateRemoved {
			continue
		}
		if flight.DataStatus == aman.DataDisconnected {
			result.Status = StatusDisconnected
			addReason(&result, "source_disconnected")
		} else if flight.DataStatus == aman.DataStale {
			addReason(&result, "stale_flight_data")
		}
		landingAt, source, authoritative, ok := landingTime(flight, state.GeneratedAt, airport)
		value := candidate{flight: flight, landingAt: landingAt, timingSource: source, authoritative: authoritative, hasTiming: ok}
		key := normalizedCallsign(flight)
		if previous, exists := deduplicated[key]; !exists || prefer(value, previous) {
			deduplicated[key] = value
		}
	}

	allFlights := make([]candidate, 0, len(deduplicated))
	for _, value := range deduplicated {
		if !value.hasTiming {
			addReason(&result, "missing_timing:"+strings.TrimSpace(value.flight.Callsign))
			continue
		}
		allFlights = append(allFlights, value)
		if value.landingAt.Before(result.RangeStart) || !value.landingAt.Before(result.RangeEnd) {
			continue
		}
		if value.timingSource == SourceAirbornePosition {
			addReason(&result, "position_estimate")
		}
		index := int(value.landingAt.Sub(result.RangeStart) / BucketDuration)
		bucket := &result.Buckets[index]
		airborne := isAirborne(value.flight) || value.timingSource == SourceAirbornePosition
		flight := Flight{Callsign: value.flight.Callsign, Airborne: airborne, LandingAt: value.landingAt.UTC(), TimingSource: value.timingSource, DataStatus: value.flight.DataStatus}
		bucket.Flights = append(bucket.Flights, flight)
		if airborne {
			bucket.AirborneCount++
		} else {
			bucket.PlannedCount++
		}
	}

	for index := range result.Buckets {
		bucket := &result.Buckets[index]
		slices.SortFunc(bucket.Flights, func(left, right Flight) int {
			if comparison := left.LandingAt.Compare(right.LandingAt); comparison != 0 {
				return comparison
			}
			return strings.Compare(left.Callsign, right.Callsign)
		})
		bucket.Count = bucket.PlannedCount + bucket.AirborneCount
		bucket.LoadFactor = bucket.Count * 4
	}
	for index := range result.Buckets {
		bucket := &result.Buckets[index]
		if bucket.SelectedRate != nil {
			bucket.BucketHigh = exceedsTenPercent(bucket.LoadFactor, int(bucket.SelectedRate.ArrivalsPerHour))
			windowCount := 0
			windowStart, windowEnd := bucket.Start.Add(-BucketDuration), bucket.End.Add(2*BucketDuration)
			for _, flight := range allFlights {
				if !flight.landingAt.Before(windowStart) && flight.landingAt.Before(windowEnd) {
					windowCount++
				}
			}
			windowRateSum, windowRateAvailable := uint64(0), true
			for segment := 0; segment < 4; segment++ {
				rate := selectedRateAt(state.RunwayGroups, windowStart.Add(time.Duration(segment)*BucketDuration))
				if rate == nil {
					windowRateAvailable = false
					addReason(&result, "missing_selected_rate")
					break
				}
				windowRateSum += uint64(rate.ArrivalsPerHour)
			}
			if windowRateAvailable {
				bucket.WindowHigh = exceedsWindowTenPercent(windowCount, windowRateSum)
			}
		}
		switch {
		case bucket.BucketHigh && bucket.WindowHigh:
			bucket.Alert = AlertRed
		case bucket.BucketHigh || bucket.WindowHigh:
			bucket.Alert = AlertYellow
		}
	}
	if result.Status == StatusReady && len(result.DegradedReasons) > 0 {
		result.Status = StatusDegraded
	}
	return result
}

func sourceDataStatus(health aman.ComponentHealth) aman.DataStatus {
	switch health.Status {
	case aman.HealthReady:
		return aman.DataFresh
	case aman.HealthDegraded:
		return aman.DataStale
	default:
		return aman.DataDisconnected
	}
}

func floorQuarter(value time.Time) time.Time {
	value = value.UTC()
	return time.Date(value.Year(), value.Month(), value.Day(), value.Hour(), value.Minute()/15*15, 0, 0, time.UTC)
}

func landingTime(flight aman.AMANFlight, now time.Time, airport AirportPosition) (time.Time, TimingSource, bool, bool) {
	authoritative := flight.State == aman.StateUnstable || flight.State == aman.StateStable || flight.FreezeReason == aman.FreezeSuperstable
	positionAt, positionOK := airbornePositionTime(flight.LatestObservation, now, airport)
	airborne := isAirborne(flight) || flight.DataStatus == aman.DataFresh && positionOK
	// Accepted AMAN timing uses the aircraft's current route and surveillance.
	// A preliminary filed-duration estimate cannot stand in for current
	// airborne position and speed in the traffic chart.
	if prediction := flight.Prediction; prediction != nil && prediction.Publishable && !prediction.OperationalTETA.IsZero() && !preliminaryPrediction(prediction) {
		if authoritative {
			return prediction.OperationalTETA.UTC(), SourceAMAN, true, true
		}
		if airborne {
			return prediction.OperationalTETA.UTC(), SourceVATSIMAirborne, false, true
		}
	}
	if airborne {
		if flight.DataStatus != aman.DataFresh {
			return time.Time{}, "", authoritative, false
		}
		if positionOK {
			return positionAt, SourceAirbornePosition, authoritative, true
		}
		return time.Time{}, "", authoritative, false
	}
	if authoritative {
		return time.Time{}, "", true, false
	}
	if flight.Prediction != nil && flight.Prediction.Publishable && !flight.Prediction.OperationalTETA.IsZero() {
		return flight.Prediction.OperationalTETA.UTC(), SourceVATSIMPlanned, false, true
	}
	if observation := flight.LatestObservation; observation != nil && observation.PlannedTiming != nil && observation.PlannedTiming.EstimatedOffBlockTime != nil && observation.PlannedTiming.EstimatedEnrouteTime != nil {
		return observation.PlannedTiming.EstimatedOffBlockTime.Add(*observation.PlannedTiming.EstimatedEnrouteTime).UTC(), SourceVATSIMPlanned, false, true
	}
	return time.Time{}, "", false, false
}

func isAirborne(flight aman.AMANFlight) bool {
	return flight.State != aman.StatePlanned || flight.LatestObservation != nil && flight.LatestObservation.TakeoffDetected != nil
}

func preliminaryPrediction(prediction *aman.Prediction) bool {
	return strings.HasPrefix(prediction.ModelVersion, "aman-planned-") || strings.HasPrefix(prediction.ModelVersion, "aman-airborne-")
}

func airbornePositionTime(observation *aman.FlightObservation, now time.Time, airport AirportPosition) (time.Time, bool) {
	if observation == nil || observation.Surveillance == nil || observation.Surveillance.ObservedAt == nil ||
		observation.Surveillance.GroundspeedKnots == nil || observation.Surveillance.AltitudeFeet == nil ||
		*observation.Surveillance.GroundspeedKnots <= 40 || *observation.Surveillance.AltitudeFeet < 1000 {
		return time.Time{}, false
	}
	fact := observation.Surveillance
	observedAt := *fact.ObservedAt
	if observedAt.After(now) || now.Sub(observedAt) > 2*time.Minute ||
		!validPosition(fact.LatitudeDegrees, fact.LongitudeDegrees) ||
		!validPosition(airport.LatitudeDegrees, airport.LongitudeDegrees) ||
		math.IsNaN(*fact.GroundspeedKnots) || math.IsInf(*fact.GroundspeedKnots, 0) {
		return time.Time{}, false
	}
	lat1, lat2 := fact.LatitudeDegrees*math.Pi/180, airport.LatitudeDegrees*math.Pi/180
	deltaLat := lat2 - lat1
	deltaLon := (airport.LongitudeDegrees - fact.LongitudeDegrees) * math.Pi / 180
	a := math.Pow(math.Sin(deltaLat/2), 2) + math.Cos(lat1)*math.Cos(lat2)*math.Pow(math.Sin(deltaLon/2), 2)
	distanceNM := 3440.065 * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	if distanceNM <= 0 || math.IsNaN(distanceNM) || math.IsInf(distanceNM, 0) {
		return time.Time{}, false
	}
	return observedAt.Add(time.Duration(distanceNM / *fact.GroundspeedKnots * float64(time.Hour))).UTC(), true
}

func validPosition(latitude, longitude float64) bool {
	return !math.IsNaN(latitude) && !math.IsInf(latitude, 0) && !math.IsNaN(longitude) && !math.IsInf(longitude, 0) &&
		latitude >= -90 && latitude <= 90 && longitude >= -180 && longitude <= 180 && (latitude != 0 || longitude != 0)
}

func normalizedCallsign(flight aman.AMANFlight) string {
	return strings.ToUpper(strings.TrimSpace(flight.Callsign))
}

func prefer(candidate, previous candidate) bool {
	if candidate.authoritative != previous.authoritative {
		return candidate.authoritative
	}
	if candidate.hasTiming != previous.hasTiming {
		return candidate.hasTiming
	}
	if (candidate.flight.State != aman.StatePlanned) != (previous.flight.State != aman.StatePlanned) {
		return candidate.flight.State != aman.StatePlanned
	}
	if !candidate.flight.UpdatedAt.Equal(previous.flight.UpdatedAt) {
		return candidate.flight.UpdatedAt.After(previous.flight.UpdatedAt)
	}
	return candidate.flight.Callsign < previous.flight.Callsign
}

func selectedRateAt(groups []aman.RunwayGroupPolicy, at time.Time) *SelectedRate {
	type selection struct {
		index    int
		at       time.Time
		revision aman.SequenceRevision
	}
	selected := selection{index: -1}
	for index, group := range groups {
		for _, point := range group.SelectionSchedule {
			if point.EffectiveAt.After(at) {
				continue
			}
			if selected.index < 0 || point.EffectiveAt.After(selected.at) || (point.EffectiveAt.Equal(selected.at) && point.CommandRevision > selected.revision) {
				selected = selection{index: index, at: point.EffectiveAt, revision: point.CommandRevision}
			}
		}
	}
	if selected.index < 0 {
		for index := range groups {
			if groups[index].Selected {
				selected.index = index
				break
			}
		}
	}
	if selected.index < 0 {
		return nil
	}
	group := groups[selected.index]
	var rate uint32
	var effective *time.Time
	for _, point := range group.RateSchedule {
		if !point.EffectiveAt.After(at) && (effective == nil || point.EffectiveAt.After(*effective)) {
			rate = point.ArrivalsPerHour
			value := point.EffectiveAt
			effective = &value
		}
	}
	if effective == nil && group.ActiveRatePerHour > 0 && group.RateEffectiveAt != nil && !group.RateEffectiveAt.After(at) {
		rate, effective = group.ActiveRatePerHour, group.RateEffectiveAt
	}
	if rate == 0 || effective == nil {
		return nil
	}
	return &SelectedRate{RunwayGroupID: group.ID, ArrivalsPerHour: rate, EffectiveAt: effective.UTC()}
}

func exceedsTenPercent(value, capacity int) bool { return int64(value)*10 > int64(capacity)*11 }

// Four quarter-hour rates sum to four times the one-hour-window capacity.
func exceedsWindowTenPercent(count int, quarterRateSum uint64) bool {
	return int64(count)*40 > int64(quarterRateSum)*11
}

func addReason(result *ReadModel, reason string) {
	if reason == "missing_timing:" {
		reason = "missing_timing:unknown_flight"
	}
	if !slices.Contains(result.DegradedReasons, reason) {
		result.DegradedReasons = append(result.DegradedReasons, reason)
		sort.Strings(result.DegradedReasons)
	}
}

func (m ReadModel) Validate() error {
	if m.BucketMinutes != 15 || !m.RangeEnd.Equal(m.RangeStart.Add(Horizon)) || len(m.Buckets) != 12 {
		return fmt.Errorf("invalid traffic-prediction range")
	}
	for index, bucket := range m.Buckets {
		want := m.RangeStart.Add(time.Duration(index) * BucketDuration)
		if !bucket.Start.Equal(want) || !bucket.End.Equal(want.Add(BucketDuration)) || bucket.Count != bucket.PlannedCount+bucket.AirborneCount || bucket.LoadFactor != bucket.Count*4 {
			return fmt.Errorf("invalid traffic-prediction bucket %d", index)
		}
	}
	return nil
}
