package operational

import (
	"math"
	"time"

	"FlightStrips/internal/aman"
)

// An arrival forecast is not takeoff evidence. Retain actual takeoff history
// across strip-only refreshes and restarts, and admit legacy position feeds
// only when their surveillance meets the EuroScope airborne classifier.
func takeoffEvidence(flight aman.AMANFlight, previous *aman.FlightObservation, observation aman.FlightObservation, now time.Time) *time.Time {
	if at := takeoffAfterGroundedAt(observation.TakeoffDetected, flight.LastGroundedAt); at != nil {
		return at
	}
	if previous != nil {
		if at := takeoffAfterGroundedAt(previous.TakeoffDetected, flight.LastGroundedAt); at != nil {
			return at
		}
	}
	if baseline := flight.ArrivalBaseline; baseline != nil && !baseline.AirborneSensedAt.IsZero() {
		at := baseline.AirborneSensedAt
		if accepted := takeoffAfterGroundedAt(&at, flight.LastGroundedAt); accepted != nil {
			return accepted
		}
	}
	position := observation.Surveillance
	if !airborneSurveillance(position) {
		return nil
	}
	at := observedAt(position, now)
	if at.After(now) || now.Sub(at) > aman.ArrivalSurveillanceFreshness {
		return nil
	}
	return takeoffAfterGroundedAt(&at, flight.LastGroundedAt)
}

func takeoffAfterGroundedAt(takeoff, groundedAt *time.Time) *time.Time {
	if takeoff != nil && groundedAt != nil && !takeoff.After(*groundedAt) {
		return nil
	}
	return takeoff
}

func recordGroundReport(flight *aman.AMANFlight, observation aman.FlightObservation, now time.Time) {
	if !groundedSurveillance(observation.Surveillance) || !hasFreshLandedRadar(observation, now) {
		return
	}
	at := *observation.Surveillance.ObservedAt
	if flight.LastGroundedAt == nil || at.After(*flight.LastGroundedAt) {
		flight.LastGroundedAt = &at
	}
}

func airborneSurveillance(position *aman.SurveillanceFact) bool {
	return position != nil && position.AltitudeFeet != nil && position.GroundspeedKnots != nil &&
		*position.AltitudeFeet > 1000 && *position.GroundspeedKnots > 40 &&
		!math.IsNaN(*position.GroundspeedKnots) && !math.IsInf(*position.GroundspeedKnots, 0) && *position.GroundspeedKnots <= euroScopeMaximumDerivedGroundspeed
}

func applyPredepartureObservation(flight aman.AMANFlight, observation aman.FlightObservation, now time.Time) aman.AMANFlight {
	clearGroundedOperationalState(&flight)
	flight.State = aman.StatePlanned
	flight.Prediction, flight.ArrivalBaseline = nil, nil
	flight.PredictionFilter, flight.RawTETASamples = nil, nil
	flight.HoldingReleaseReservation, flight.ApproachRelease, flight.ApproachGuidance = nil, nil, nil
	flight.PredictionFailureReason, flight.STARSelectionRequired = nil, false
	applyPreliminaryPrediction(&flight, observation, now)
	setGroundedLifecycle(&flight, aman.LifecycleReasonInitial, now)
	return flight
}
