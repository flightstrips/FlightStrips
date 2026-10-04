package holdingclearance

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"FlightStrips/internal/aman"
)

// ReadModel is the presentation-neutral list of authoritative en-route holds
// for one AMAN airport. Missing display facts remain explicit nil values.
type ReadModel struct {
	Entries  []Entry
	Warnings []aman.Warning
}

type Entry struct {
	Callsign        string
	Holding         string
	EAT             *time.Time
	ClearedAltitude *int32
	SourceStatus    aman.DataStatus
	ObservedAt      time.Time
}

func BuildReadModel(state aman.AirportState) ReadModel {
	airport := strings.ToUpper(strings.TrimSpace(state.Airport))
	result := ReadModel{Entries: make([]Entry, 0)}
	for _, flight := range state.Flights {
		if flight.State == aman.StateRemoved || flight.State == aman.StateLanded {
			continue
		}
		clearance := flight.HoldingClearance
		if clearance == nil || clearance.Hold == "" || clearance.HoldType != aman.HoldingClearanceEnroute ||
			flight.LatestObservation == nil || strings.ToUpper(strings.TrimSpace(flight.LatestObservation.Destination)) != airport {
			continue
		}

		entry := Entry{
			Callsign: flight.Callsign, Holding: clearance.Hold,
			ClearedAltitude: cloneAltitude(clearance.ClearedAltitude), SourceStatus: flight.DataStatus,
			ObservedAt: clearance.ObservedAt,
		}
		// Display AMAN's calculated release independently of EuroScope EAT
		// writeback, which may be disabled or unavailable without tracking.
		if prediction := flight.Prediction; prediction != nil && prediction.Publishable && prediction.HoldingPlan != nil &&
			!prediction.HoldingPlan.ApproachReleaseTime.IsZero() {
			release := prediction.HoldingPlan.ApproachReleaseTime.UTC()
			entry.EAT = &release
		}
		result.Entries = append(result.Entries, entry)
		if entry.EAT == nil {
			result.Warnings = append(result.Warnings, unavailableReleaseWarning(flight))
		}
	}

	slices.SortFunc(result.Entries, func(left, right Entry) int {
		if comparison := strings.Compare(strings.ToUpper(strings.TrimSpace(left.Callsign)), strings.ToUpper(strings.TrimSpace(right.Callsign))); comparison != 0 {
			return comparison
		}
		return strings.Compare(left.Callsign, right.Callsign)
	})
	slices.SortFunc(result.Warnings, func(left, right aman.Warning) int { return strings.Compare(left.ID, right.ID) })
	return result
}

func unavailableReleaseWarning(flight aman.AMANFlight) aman.Warning {
	component, callsign := "holding_release", flight.Callsign
	warning := aman.Warning{Source: aman.WarningSourceSequence, Component: &component, Callsign: &callsign,
		Severity: aman.WarningSeverityWarning, Code: "holding_release_unavailable"}
	cause := "no calculated holding release has been accepted"
	prediction := flight.Prediction
	switch {
	case !flight.SequenceDisposition.Participates():
		cause = "the aircraft is desequenced"
	case flight.Slot == nil:
		cause = "the aircraft has no assigned runway slot"
	case prediction == nil || !prediction.Publishable:
		cause = "no usable arrival calculation is available"
		if prediction != nil && prediction.DegradationReason != nil {
			cause += " (" + *prediction.DegradationReason + ")"
		}
	case prediction.HoldingPlanBlockedBy != nil:
		warning.Code, warning.RelatedCallsign = "holding_release_order_conflict", prediction.HoldingPlanBlockedBy
		cause = "a safe release behind " + *prediction.HoldingPlanBlockedBy + " cannot be established in the holding queue"
	case prediction.HoldingFixETA == nil:
		cause = "the current trajectory has no time at the cleared holding fix"
	case !flight.Slot.Time.After(prediction.RawTETA):
		cause = fmt.Sprintf("the protected runway slot %s is no later than the current free-flight arrival %s, so it cannot accommodate a holding delay", flight.Slot.Time.UTC().Format("15:04 UTC"), prediction.RawTETA.UTC().Format("15:04 UTC"))
	case !prediction.RawTETA.After(*prediction.HoldingFixETA):
		cause = "the travel time from the holding fix to the runway is unavailable"
	}
	warning.Message = fmt.Sprintf("%s has no calculated EAT for %s: %s.", callsign, flight.HoldingClearance.Hold, cause)
	if flight.Slot != nil {
		warning.Message += " Its assigned runway slot is retained; review the holding sequence before release."
	}
	warning.ID = warning.Identity()
	return warning
}
