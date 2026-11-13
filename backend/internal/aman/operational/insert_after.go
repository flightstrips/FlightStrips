package operational

import (
	"context"
	"strings"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/sequence"
)

func (s *Service) InsertFlightAfter(ctx context.Context, auth aman.CommandContext, command aman.InsertFlightAfterCommand) (sequence.CommandMutation, error) {
	if err := s.authorizeFlightDisposition(auth); err != nil {
		return nil, err
	}
	return func(state aman.AirportState) (sequence.CommandChange, error) {
		index, anchorIndex := flightIndex(state.Flights, command.Callsign), flightIndex(state.Flights, command.AfterCallsign)
		if index < 0 {
			return sequence.CommandChange{}, domainNotFound(command.Callsign)
		}
		if anchorIndex < 0 {
			return sequence.CommandChange{}, domainNotFound(command.AfterCallsign)
		}
		target, anchor := state.Flights[index], state.Flights[anchorIndex]
		invalid := func(message string) (sequence.CommandChange, error) {
			return sequence.CommandChange{}, &aman.DomainError{Class: aman.ErrorInvalidTransition, Message: message}
		}
		if target.State == aman.StatePlanned || target.State == aman.StateLanded || target.State == aman.StateRemoved || !target.SequenceDisposition.Participates() {
			return invalid("insertion requires an active airborne arrival")
		}
		if !freshArrivalObservation(target.LatestObservation, auth.ReceivedAt) {
			return invalid("insertion requires current aircraft surveillance")
		}
		if anchor.Slot == nil || anchor.SelectedRunwayGroup == nil || !anchor.SequenceDisposition.Participates() || anchor.State == aman.StateLanded || anchor.State == aman.StateRemoved {
			return invalid("anchor must have an active landing slot")
		}
		group := *anchor.SelectedRunwayGroup
		active := false
		for _, id := range state.ActiveRunwayGroups {
			if id == group {
				active = true
			}
		}
		if !active {
			return invalid("anchor runway is not active")
		}
		if command.STARFamily != nil {
			family := strings.ToUpper(strings.TrimSpace(*command.STARFamily))
			if family == "" {
				target.ControllerSTARFamily = nil
			} else {
				target.ControllerSTARFamily = &family
			}
		}
		resolved := s.resolveFlightSTAR(target, *target.LatestObservation, group)
		if resolved.reason != "" {
			return invalid("select a configured STAR compatible with the anchor runway: " + resolved.reason)
		}
		if target.SelectedRunwayGroup != nil && *target.SelectedRunwayGroup != group && target.Slot != nil {
			return invalid("change runway before moving a sequenced flight to another runway")
		}
		runwayChanged := target.SelectedRunwayGroup == nil || *target.SelectedRunwayGroup != group
		if runwayChanged || command.STARFamily != nil {
			priorSlot, priorOrder := target.Slot, target.Order
			assignFlightToRunwayGroup(&target, group)
			if !runwayChanged {
				// Refresh STAR-dependent geometry without making a TMA-frozen
				// target ineligible while its new path is calculated.
				target.Slot, target.Order = priorSlot, priorOrder
			}
		}
		// A changed runway or STAR requires a prediction from that path; never use
		// an estimate calculated for a different approach.
		changedPath := command.STARFamily != nil || runwayChanged
		if changedPath || target.Prediction == nil || !target.Prediction.Publishable || isPreliminaryPrediction(target.Prediction) {
			updated, err := s.fallbackPredictionForRunway(ctx, state, target, *target.LatestObservation, target.LatestObservation, group, auth.ReceivedAt, "controller_insertion")
			if err != nil {
				return invalid("arrival estimate unavailable: " + err.Error())
			}
			target = updated
		}
		state.Flights = append([]aman.AMANFlight(nil), state.Flights...)
		state.Flights[index] = target
		invalidateHoldingRebuild(&state, command.Callsign)
		decision, err := sequence.InsertAfter(s.sequenceInput(state), command.Callsign, command.AfterCallsign, auth.ReceivedAt)
		if err != nil {
			return sequence.CommandChange{}, err
		}
		candidate := s.applyDecision(state, decision)
		for i := range candidate.Flights {
			before, after := state.Flights[i], &candidate.Flights[i]
			if after.Slot == nil || before.Slot != nil && before.Slot.Time.Equal(after.Slot.Time) {
				continue
			}
			after.UpdatedAt = auth.ReceivedAt
			after.QueueOffers = nil
			// Pending release offers are recalculated against the new EAT. Already
			// released traffic remains a historical fact, even when its landing moves.
			if r := after.ApproachRelease; r != nil && r.ReleasedAt == nil && r.ReleaseAt.After(auth.ReceivedAt) {
				after.ApproachRelease = nil
			}
			after.HoldingReleaseReservation = nil
		}
		s.refreshHoldingPlans(&candidate)
		return s.commandChange(candidate, true, "insert_flight_after", command.Callsign, map[string]any{"actor": auth.Actor, "role": auth.Role, "after_callsign": command.AfterCallsign, "star_family": target.STARFamilyIdentity(), "fallback_reason": stringValue(target.Prediction.DegradationReason), "movements": decision.Candidate.Movements, "operations": decision.Candidate.Operations})
	}, nil
}
