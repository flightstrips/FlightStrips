// Package groundplanning keeps provisional ground arrivals behind occupied holds.
package groundplanning

import (
	"strings"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/predictor"
	"FlightStrips/internal/aman/sequence"
)

const CapacityUnavailableReason = "ground_arrival_behind_holding:runway_capacity_unavailable"

func IsGround(flight aman.AMANFlight) bool {
	if flight.State != aman.StatePlanned {
		return false
	}
	observation := flight.LatestObservation
	if observation == nil {
		return true
	}
	if observation.TakeoffDetected != nil {
		return false
	}
	position := observation.Surveillance
	return position == nil || position.AltitudeFeet == nil || position.GroundspeedKnots == nil ||
		*position.AltitudeFeet < 1000 || *position.GroundspeedKnots <= 40
}

// Floor returns the last occupied holding target on this flight's runway.
// Unassigned ground arrivals use the selected runway, just as their forecast does.
func Floor(state aman.AirportState, flight aman.AMANFlight) time.Time {
	if !IsGround(flight) || !flight.SequenceDisposition.Participates() {
		return time.Time{}
	}
	group := runwayGroup(state, flight)
	var floor time.Time
	for _, held := range state.Flights {
		if runwayGroup(state, held) != group || group == "" {
			continue
		}
		if at, ok := HoldingAt(held); ok && at.After(floor) {
			floor = at
		}
	}
	return floor
}

func HoldingAt(held aman.AMANFlight) (time.Time, bool) {
	if held.State == aman.StatePlanned || held.State == aman.StateLanded || held.State == aman.StateRemoved ||
		!held.SequenceDisposition.Participates() || held.HoldingStack == nil || !held.HoldingStack.Confirmed || held.HoldingStack.HoldingID == "" {
		return time.Time{}, false
	}
	cleared := held.HoldingClearance != nil && held.HoldingClearance.HoldType == aman.HoldingClearanceEnroute && held.HoldingClearance.Hold != ""
	if !cleared && (held.Prediction == nil || held.Prediction.HoldingFixETA == nil) {
		return time.Time{}, false
	}
	if held.Slot != nil && !held.Slot.Time.IsZero() {
		return held.Slot.Time, true
	}
	if held.Prediction != nil && held.Prediction.Publishable && !held.Prediction.OperationalTETA.IsZero() {
		return held.Prediction.OperationalTETA, true
	}
	return time.Time{}, false
}

// Plan creates soft arrival targets without assigning ground aircraft committed
// slots or moving airborne aircraft. Normal runway rates, gaps and separation
// apply to these provisional targets.
func Plan(state aman.AirportState, input sequence.Input, now time.Time) map[aman.Callsign]time.Time {
	result := make(map[aman.Callsign]time.Time)
	working := input
	working.Flights = nil
	for _, flight := range input.Flights {
		if flight.State == aman.StatePlanned {
			continue
		}
		if flight.CurrentSlot != nil {
			flight.FreezeReason, flight.CapturedSlot = aman.FreezeNone, nil
			flight.FrozenAt, flight.FrozenOperationalTETA = nil, nil
			flight.ProtectCurrentSlot = true
		}
		working.Flights = append(working.Flights, flight)
	}
	ground := make(map[aman.Callsign]struct{})
	for _, flight := range state.Flights {
		floor := Floor(state, flight)
		at, ok := ExpectedAt(flight)
		if floor.IsZero() || !ok || at.After(floor) && (flight.Prediction == nil || flight.Prediction.OperationalReason != aman.OperationalReasonHoldingPriority) {
			continue
		}
		lower := floor.Add(time.Nanosecond)
		if !lower.After(now) {
			lower = now.Add(time.Nanosecond)
		}
		working.Flights = append(working.Flights, sequence.Flight{
			Callsign: flight.Callsign, RunwayGroupID: runwayGroup(state, flight), State: aman.StatePlanned,
			OperationalTETA: at, SlotNotBefore: &lower, ArrivalQueueTime: &now, FreezeReason: aman.FreezeNone,
			WakeCategory: wakeCategory(flight),
		})
		ground[flight.Callsign] = struct{}{}
	}
	if len(ground) == 0 {
		return result
	}
	generated, err := sequence.Generate(working)
	if err != nil || generated.HasConflicts() {
		return result
	}
	for _, entry := range generated.Entries {
		if _, ok := ground[entry.Callsign]; ok {
			result[entry.Callsign] = entry.Time
		}
	}
	return result
}

func ExpectedAt(flight aman.AMANFlight) (time.Time, bool) {
	if prediction := flight.Prediction; prediction != nil && prediction.Publishable {
		if prediction.OperationalReason == aman.OperationalReasonManualOverride || prediction.OperationalReason == aman.OperationalReasonHoldingPriority {
			return prediction.OperationalTETA, !prediction.OperationalTETA.IsZero()
		}
		if !prediction.RawTETA.IsZero() {
			return prediction.RawTETA, true
		}
		if !prediction.OperationalTETA.IsZero() {
			return prediction.OperationalTETA, true
		}
	}
	observation := flight.LatestObservation
	if observation == nil || observation.PlannedTiming == nil || observation.PlannedTiming.EstimatedOffBlockTime == nil ||
		observation.PlannedTiming.EstimatedOffBlockTime.IsZero() || observation.PlannedTiming.EstimatedEnrouteTime == nil || *observation.PlannedTiming.EstimatedEnrouteTime <= 0 {
		return time.Time{}, false
	}
	return observation.PlannedTiming.EstimatedOffBlockTime.Add(predictor.DefaultEXOT).Add(*observation.PlannedTiming.EstimatedEnrouteTime), true
}

// ProjectionInput also covers raw filed-time forecasts that have no persisted
// prediction yet. It uses the same CPH spacing as operational sequencing.
func ProjectionInput(state aman.AirportState) sequence.Input {
	input := sequence.Input{Revision: state.Revision}
	for _, group := range state.RunwayGroups {
		policy := sequence.Policy{RunwayGroupID: group.ID, ContinuousSpacing: true, UnknownSeparation: 3 * time.Minute, SeparationRules: sequence.CPHSeparationRules()}
		for _, rate := range group.RateSchedule {
			policy.Rates = append(policy.Rates, sequence.RatePoint{EffectiveAt: rate.EffectiveAt, ArrivalsPerHour: rate.ArrivalsPerHour})
		}
		if len(policy.Rates) == 0 && group.ActiveRatePerHour > 0 && group.RateEffectiveAt != nil {
			policy.Rates = []sequence.RatePoint{{EffectiveAt: *group.RateEffectiveAt, ArrivalsPerHour: group.ActiveRatePerHour}}
		}
		if len(policy.Rates) == 0 {
			continue
		}
		for _, gap := range group.Gaps {
			policy.Gaps = append(policy.Gaps, sequence.Gap{Start: gap.Start, End: gap.End})
		}
		for _, reservation := range group.CapacityReservations {
			policy.Gaps = append(policy.Gaps, sequence.Gap{Start: reservation.Start, End: reservation.End})
		}
		policy.Closures = group.Closures
		if spacing := group.SameSTARSpacing; spacing != nil {
			policy.SameSTARSpacing = sequence.SameSTARSpacing{Enabled: spacing.Enabled, ActivationRatePerHour: spacing.ActivationRatePerHour, MinimumEmptySlots: spacing.MinimumEmptySlots}
		}
		input.Policies = append(input.Policies, policy)
	}
	for _, flight := range state.Flights {
		if IsGround(flight) || flight.State == aman.StateLanded || flight.State == aman.StateRemoved || !flight.SequenceDisposition.Participates() {
			continue
		}
		at := time.Time{}
		if flight.Slot != nil {
			at = flight.Slot.Time
		} else if flight.Prediction != nil && flight.Prediction.Publishable {
			at = flight.Prediction.OperationalTETA
		}
		if at.IsZero() {
			continue
		}
		input.Flights = append(input.Flights, sequence.Flight{Callsign: flight.Callsign, State: flight.State, RunwayGroupID: runwayGroup(state, flight),
			OperationalTETA: at, CurrentSlot: flight.Slot, FreezeReason: aman.FreezeNone, WakeCategory: wakeCategory(flight), STARFamily: flight.STARFamilyIdentity()})
	}
	return input
}

func runwayGroup(state aman.AirportState, flight aman.AMANFlight) aman.RunwayGroupID {
	if flight.SelectedRunwayGroup != nil {
		return *flight.SelectedRunwayGroup
	}
	for _, group := range state.RunwayGroups {
		if group.Selected {
			return group.ID
		}
	}
	if len(state.RunwayGroups) == 1 {
		return state.RunwayGroups[0].ID
	}
	return ""
}

func wakeCategory(flight aman.AMANFlight) sequence.WakeCategory {
	if flight.LatestObservation != nil && flight.LatestObservation.WakeCategory != nil {
		return sequence.WakeCategory(strings.ToUpper(*flight.LatestObservation.WakeCategory))
	}
	return ""
}
