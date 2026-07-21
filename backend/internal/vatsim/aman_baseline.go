package vatsim

import (
	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/predictor"
	"time"
)

// AMANBaselineInput maps only the normalized AMAN facts the VATSIM boundary
// actually has. In particular VATSIM supplies its service-day EOBT and filed
// EET; it does not manufacture an API estimated flight time or route geometry.
// TakeoffDetected is produced by the existing VATSIM movement classifier.
func AMANBaselineInput(now time.Time, expectedDestination string, observation aman.FlightObservation, previouslyObserved, resetHeldAirborne bool) predictor.Input {
	input := predictor.Input{
		Now: now, ExpectedDestination: expectedDestination, Destination: observation.Destination,
		Airborne:           predictor.AirborneObservation{SensedAt: observation.TakeoffDetected, PreviouslyObserved: previouslyObserved},
		FlightPlanRevision: observation.FlightPlan.Revision, FlightPlanObservedAt: observation.FlightPlan.ObservedAt,
		ResetHeldAirborne: resetHeldAirborne,
	}
	if observation.PlannedTiming != nil {
		input.Timing.EOBT = observation.PlannedTiming.EstimatedOffBlockTime
		input.Timing.FiledEET = observation.PlannedTiming.EstimatedEnrouteTime
	}
	return input
}
