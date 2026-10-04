package trafficprediction

import (
	"fmt"
	"math"
	"strings"
	"time"

	"FlightStrips/internal/aman"
)

func (m *ReadModel) warn(code string, callsign aman.Callsign, message string) {
	component := "traffic_prediction"
	warning := aman.Warning{Source: aman.WarningSourceTrafficPrediction, Component: &component,
		Severity: aman.WarningSeverityWarning, Code: code, Message: message}
	if callsign != "" {
		warning.Callsign = &callsign
	}
	warning.ID = warning.Identity()
	// Global conditions can apply to several buckets; publish them once.
	for _, current := range m.Warnings {
		if current.ID == warning.ID {
			return
		}
	}
	m.Warnings = append(m.Warnings, warning)
}

func missingTimingDetail(flight aman.AMANFlight, now time.Time, airport AirportPosition) (string, string) {
	observation := flight.LatestObservation
	code := "missing_timing"
	var reasons []string
	if !isAirborne(flight) {
		missingDeparture := observation == nil || observation.PlannedTiming == nil || observation.PlannedTiming.EstimatedOffBlockTime == nil || observation.PlannedTiming.EstimatedOffBlockTime.IsZero()
		missingDuration := observation == nil || observation.PlannedTiming == nil || observation.PlannedTiming.EstimatedEnrouteTime == nil || *observation.PlannedTiming.EstimatedEnrouteTime <= 0
		if missingDeparture {
			reasons = append(reasons, "no valid filed off-block time (EOBT)")
		}
		if missingDuration {
			reasons = append(reasons, "no valid filed enroute duration (EET)")
		}
		code = "missing_departure_timing"
		if missingDeparture && !missingDuration {
			code = "missing_departure_time"
		}
		if !missingDeparture && missingDuration {
			code = "missing_flight_duration"
		}
		if !missingDeparture && !missingDuration {
			code = "arrival_timing_unavailable"
			reasons = append(reasons, "filed departure timing is available, but no accepted arrival estimate or committed slot is available")
		}
		message := fmt.Sprintf("%s is still planned", flight.Callsign)
		if observation != nil && observation.Origin != "" {
			message += " at " + observation.Origin
		}
		return code, message + ": " + strings.Join(reasons, "; ") + ". It cannot be placed in an arrival-time bucket until valid departure timing is received or airborne tracking supplies an estimate. " + observationDetail(flight, now)
	}
	if flight.DataStatus != aman.DataFresh {
		code = "aircraft_updates_unavailable"
		if flight.DataStatus == aman.DataStale && observation != nil && observation.SourceStatus == aman.DataFresh {
			code = "arrival_prediction_not_updated"
			reasons = append(reasons, "aircraft messages are arriving, but AMAN has not accepted a new arrival estimate")
		} else {
			reasons = append(reasons, "aircraft updates are "+string(flight.DataStatus))
		}
	}
	if observation == nil || observation.Surveillance == nil {
		reasons = append(reasons, "no surveillance position is available")
	} else {
		fact := observation.Surveillance
		if !validPosition(fact.LatitudeDegrees, fact.LongitudeDegrees) {
			reasons = append(reasons, "surveillance coordinates are invalid")
		}
		if fact.ObservedAt == nil {
			reasons = append(reasons, "the surveillance timestamp is missing")
		} else if fact.ObservedAt.After(now) {
			reasons = append(reasons, "the surveillance timestamp is in the future")
		} else if now.Sub(*fact.ObservedAt) > 2*time.Minute {
			reasons = append(reasons, "the last surveillance position is more than two minutes old")
		}
		if fact.GroundspeedKnots == nil || math.IsNaN(*fact.GroundspeedKnots) || math.IsInf(*fact.GroundspeedKnots, 0) || *fact.GroundspeedKnots <= 40 {
			reasons = append(reasons, "a usable airborne groundspeed is missing")
		}
		if fact.AltitudeFeet == nil || *fact.AltitudeFeet < 1000 {
			reasons = append(reasons, "an airborne altitude of at least 1,000 ft is unavailable")
		}
	}
	if !validPosition(airport.LatitudeDegrees, airport.LongitudeDegrees) {
		reasons = append(reasons, "destination coordinates are unavailable for the position fallback")
	}
	if prediction := flight.Prediction; prediction != nil && prediction.DegradationReason != nil && !prediction.Publishable {
		reasons = append(reasons, "arrival prediction unavailable: "+predictionFailureDetail(flight, now))
	}
	if len(reasons) == 0 {
		reasons = append(reasons, "no accepted arrival estimate or committed slot is available")
	}
	return code, fmt.Sprintf("%s is omitted from arrival counts: %s. No retained arrival estimate or committed slot is available; awaiting usable aircraft updates. %s", flight.Callsign, strings.Join(reasons, "; "), observationDetail(flight, now))
}

func retainedTimingWarning(flight aman.AMANFlight, arrival, now time.Time) (string, string) {
	code := "stale_flight_data"
	cause := "Current aircraft data is not usable for a new arrival estimate."
	if observation := flight.LatestObservation; observation != nil {
		if observation.SourceStatus == aman.DataFresh {
			code = "arrival_prediction_not_updated"
			cause = "Aircraft messages are arriving, but AMAN could not refresh the arrival estimate: " + predictionFailureDetail(flight, now) + "."
		} else if observation.SourceStatus == aman.DataStale {
			cause = "The aircraft source reports stale data."
		}
	}
	retained := "arrival estimate"
	if flight.Slot != nil && flight.Slot.Time.Equal(arrival) {
		retained = "assigned arrival time"
	}
	predictionAge := ""
	if prediction := flight.Prediction; prediction != nil && !prediction.GeneratedAt.IsZero() {
		predictionAge = " Retained estimate calculated at " + prediction.GeneratedAt.UTC().Format("2006-01-02 15:04:05 UTC") + "."
	}
	return code, fmt.Sprintf("%s: %s Keeping %s %s.%s %s", flight.Callsign, cause, retained, arrival.UTC().Format("15:04 UTC"), predictionAge, observationDetail(flight, now))
}

func predictionFailureDetail(flight aman.AMANFlight, now time.Time) string {
	if flight.Prediction == nil || flight.Prediction.DegradationReason == nil || strings.TrimSpace(*flight.Prediction.DegradationReason) == "" {
		return "the latest update did not produce a usable calculation; no specific failure reason was recorded"
	}
	reason := *flight.Prediction.DegradationReason
	if strings.HasPrefix(reason, "wtc_light_reta_unavailable:") {
		reason = strings.TrimPrefix(reason, "wtc_light_reta_unavailable:")
	}
	for _, kind := range []struct{ prefix, message string }{
		{"missing_essential_data:", "the latest update is missing required data: "},
		{"invalid_essential_data:", "the latest update contains invalid data: "},
	} {
		if fields, ok := strings.CutPrefix(reason, kind.prefix); ok {
			labels := strings.Split(fields, ",")
			for index, field := range labels {
				switch strings.TrimSpace(field) {
				case "surveillance":
					labels[index] = "aircraft position"
				case "filed_route":
					labels[index] = "filed route"
				case "groundspeed":
					labels[index] = "groundspeed"
				case "altitude":
					labels[index] = "altitude"
				default:
					labels[index] = strings.ReplaceAll(strings.TrimSpace(field), "_", " ")
				}
			}
			return kind.message + strings.Join(labels, ", ")
		}
	}
	switch reason {
	case "unknown_star_family":
		return "the filed route does not match a configured STAR entry"
	case "tma_entry_surveillance_stale":
		if observation := flight.LatestObservation; observation != nil && observation.Surveillance != nil {
			at := observation.Surveillance.ObservedAt
			if at == nil {
				return "the position timestamp is missing"
			}
			if at.After(now) {
				return "the position timestamp is ahead of the AMAN update time"
			}
			if now.Sub(*at) > 2*time.Minute {
				return "the position is more than two minutes old"
			}
		}
		return "the current position was not accepted by the TMA calculation"
	case "tma_entry_surveillance_invalid":
		return "the current position is invalid for the TMA calculation"
	case "wtc_light_reta_unavailable":
		return "the required groundspeed-based calculation for this light aircraft is unavailable"
	}
	if strings.Contains(reason, "prediction segment is invalid") {
		return "AMAN rejected the new calculation because a calculated flight segment failed validation"
	}
	if strings.HasPrefix(reason, "route geometry is not publishable:") {
		return "the current position could not be matched to usable remaining route geometry (" + strings.TrimSpace(strings.TrimPrefix(reason, "route geometry is not publishable:")) + ")"
	}
	reason = strings.TrimPrefix(reason, "raw prediction: ")
	reason = strings.TrimPrefix(reason, "invalid_argument: ")
	return strings.ReplaceAll(reason, "_", " ")
}

func observationDetail(flight aman.AMANFlight, now time.Time) string {
	observation := flight.LatestObservation
	if observation == nil {
		return "No observation has been received."
	}
	at := observation.FlightPlan.ObservedAt
	kind := "flight plan"
	if observation.Surveillance != nil && observation.Surveillance.ObservedAt != nil {
		at = observation.Surveillance.ObservedAt
		kind = "position"
	}
	if at == nil && !observation.ReconciledAt.IsZero() {
		at = &observation.ReconciledAt
		kind = "received message"
	}
	if at == nil {
		return "Last observation time is unavailable."
	}
	provider := string(observation.Provider)
	if observation.Provider == aman.ObservationProviderEuroScope {
		provider = "EuroScope"
	} else if observation.Provider == aman.ObservationProviderVATSIM {
		provider = "VATSIM"
	} else if provider == "" {
		provider = "aircraft source"
	}
	age := fmt.Sprintf("%s before this AMAN update", now.Sub(*at).Round(time.Second))
	if at.After(now) {
		ahead := at.Sub(now).Round(time.Second).String()
		if ahead == "0s" {
			ahead = "less than 1s"
		}
		age = fmt.Sprintf("timestamp is %s ahead of this AMAN update", ahead)
	}
	status := string(observation.SourceStatus)
	if status == "" {
		status = "unreported"
	}
	return fmt.Sprintf("Last %s observation (%s): %s (%s); source status: %s.", provider, kind, at.UTC().Format("2006-01-02 15:04:05 UTC"), age, status)
}
