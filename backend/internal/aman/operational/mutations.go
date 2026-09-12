package operational

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strings"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/etareview"
	"FlightStrips/internal/aman/lifecycle"
	"FlightStrips/internal/aman/prediction"
	"FlightStrips/internal/aman/sequence"
)

// RecomputeFlight reruns the physical predictor from the observation stored in
// the revision-checked airport aggregate. It does not accept a client result,
// release a freeze, or apply a client-selected order.
func (s *Service) RecomputeFlight(ctx context.Context, auth aman.CommandContext, command aman.RecomputeFlightCommand) (sequence.CommandMutation, error) {
	return func(state aman.AirportState) (sequence.CommandChange, error) {
		index := flightIndex(state.Flights, command.FlightID)
		if index < 0 {
			return sequence.CommandChange{}, domainNotFound(command.FlightID)
		}
		current := state.Flights[index]
		if current.State == aman.StateLanded || current.State == aman.StateRemoved {
			return sequence.CommandChange{}, &aman.DomainError{Class: aman.ErrorInvalidTransition, Message: "inactive AMAN flight cannot be recomputed"}
		}
		if current.LatestObservation == nil {
			return sequence.CommandChange{}, &aman.DomainError{Class: aman.ErrorInvalidTransition, Message: "AMAN flight has no authoritative observation to recompute"}
		}
		observation := *current.LatestObservation
		if observation.Missing || observation.SourceStatus != aman.DataFresh {
			return sequence.CommandChange{}, &aman.DomainError{Class: aman.ErrorInvalidTransition, Message: "AMAN flight observation is not current"}
		}
		updated, err := s.reconcileFlight(ctx, state, current, observation, auth.ReceivedAt)
		if err != nil {
			return sequence.CommandChange{}, err
		}
		state.Flights = append([]aman.AMANFlight(nil), state.Flights...)
		state.Flights[index] = updated
		promotions := s.resequence(&state, auth.ReceivedAt)
		// A completed calculation is itself an authoritative state change even
		// when policy retains the same TETA. This allocates and publishes the
		// confirming revision that resolves the client's pending command.
		change, err := s.commandChange(state, true, "recompute_flight", command.FlightID, map[string]any{
			"airport": auth.Airport, "actor": auth.Actor, "role": auth.Role,
			"received_at": auth.ReceivedAt, "input_observed_at": observation.ReconciledAt,
		})
		change.Audit = append(change.Audit, vacancyPromotionAuditEntries(promotions)...)
		return change, err
	}, nil
}

// DefaultGoAroundDelay is the operational landing-time target applied from
// go-around detection until a new physical prediction is established.
const DefaultGoAroundDelay = 10 * time.Minute

func (s *Service) MoveFlight(_ aman.CommandContext, command aman.MoveFlightCommand) (sequence.CommandMutation, error) {
	return func(state aman.AirportState) (sequence.CommandChange, error) {
		index := flightIndex(state.Flights, command.FlightID)
		if index < 0 {
			return sequence.CommandChange{}, domainNotFound(command.FlightID)
		}
		state.Flights = append([]aman.AMANFlight(nil), state.Flights...)
		decision, err := sequence.ApplyMove(s.sequenceInput(state), sequence.MoveFlightCommand{Metadata: command.Metadata, FlightID: command.FlightID, RunwayGroupID: command.RunwayGroupID, BeforeFlightID: command.BeforeFlightID, AfterFlightID: command.AfterFlightID})
		if err != nil {
			return sequence.CommandChange{}, err
		}
		return s.commandChange(s.applyDecision(state, decision), decision.Changed, "move_flight", command.FlightID, nil)
	}, nil
}

func (s *Service) PlaceFlightAtTime(auth aman.CommandContext, command aman.PlaceFlightAtTimeCommand) (sequence.CommandMutation, error) {
	if command.AllowGap {
		if err := s.authorizeRunwayGap(auth); err != nil {
			return nil, err
		}
	}
	return func(state aman.AirportState) (sequence.CommandChange, error) {
		index := flightIndex(state.Flights, command.FlightID)
		if index < 0 {
			return sequence.CommandChange{}, domainNotFound(command.FlightID)
		}
		flight := state.Flights[index]
		if flight.State == aman.StatePlanned || flight.State == aman.StateLanded || flight.State == aman.StateRemoved ||
			flight.Prediction == nil || !flight.SequenceDisposition.Participates() {
			return sequence.CommandChange{}, &aman.DomainError{Class: aman.ErrorInvalidTransition, Message: "flight is not eligible for manual grid placement"}
		}
		groupIndex := runwayGroupIndex(state.RunwayGroups, command.RunwayGroupID)
		if groupIndex < 0 {
			return sequence.CommandChange{}, &aman.DomainError{Class: aman.ErrorNotFound, Message: "AMAN runway group was not found"}
		}
		if !slices.Contains(state.ActiveRunwayGroups, command.RunwayGroupID) {
			return sequence.CommandChange{}, &aman.DomainError{Class: aman.ErrorInvalidArgument, Message: "requested runway group is not active"}
		}
		if !s.runwayAssignmentCompatible(flight, command.RunwayGroupID) {
			return sequence.CommandChange{}, &aman.DomainError{Class: aman.ErrorInvalidArgument, Message: "requested runway group is not compatible with the flight's arrival"}
		}
		for _, closure := range state.RunwayGroups[groupIndex].Closures {
			if !command.SlotTime.Before(closure.Start) && (closure.End == nil || command.SlotTime.Before(*closure.End)) {
				return sequence.CommandChange{}, &aman.DomainError{Class: aman.ErrorInvalidTransition, Message: "manual placement cannot use runway closure capacity"}
			}
		}
		input := s.sequenceInput(state)
		onGrid, err := sequence.IsGridOpportunity(input, command.RunwayGroupID, command.SlotTime)
		if err != nil || !onGrid {
			return sequence.CommandChange{}, &aman.DomainError{Class: aman.ErrorInvalidArgument, Message: "requested time is not a runway-grid opportunity"}
		}
		var overridden *aman.RunwayGap
		for gapIndex := range state.RunwayGroups[groupIndex].Gaps {
			gap := &state.RunwayGroups[groupIndex].Gaps[gapIndex]
			if !command.SlotTime.Before(gap.Start) && command.SlotTime.Before(gap.End) {
				overridden = gap
				break
			}
		}
		if overridden != nil && !command.AllowGap {
			return sequence.CommandChange{}, &aman.DomainError{Class: aman.ErrorInvalidTransition, Message: "normal manual placement cannot use an active runway GAP"}
		}
		if overridden == nil && command.AllowGap {
			return sequence.CommandChange{}, &aman.DomainError{Class: aman.ErrorInvalidArgument, Message: "GAP exception requested outside an active runway GAP"}
		}

		candidate := state
		candidate.Flights = append([]aman.AMANFlight(nil), state.Flights...)
		target := &candidate.Flights[index]
		priorSlot := target.Slot
		assignFlightToRunwayGroup(target, command.RunwayGroupID)
		target.FreezeReason, target.FrozenAt = aman.FreezeManual, timePointer(auth.ReceivedAt)
		frozenTETA := target.Prediction.OperationalTETA
		target.FrozenOperationalTETA = &frozenTETA
		target.FrozenSlot = &aman.Slot{Time: command.SlotTime, RunwayGroupID: command.RunwayGroupID, Sequence: 1, Revision: state.Revision, Reason: string(sequence.ReasonFreezeManual)}
		target.ManualOrder, target.RunwayGapException, target.UpdatedAt = nil, nil, auth.ReceivedAt
		input = s.sequenceInput(candidate)
		result, err := sequence.Generate(input)
		if err != nil || result.HasConflicts() {
			return sequence.CommandChange{}, &aman.DomainError{Class: aman.ErrorInvalidTransition, Message: "requested opportunity cannot produce a legal atomic sequence"}
		}
		candidate = s.applyDecision(candidate, sequence.Decision{Input: input, Candidate: result, Changed: true})
		placed := &candidate.Flights[index]
		if placed.Slot == nil || !placed.Slot.Time.Equal(command.SlotTime) || placed.Slot.RunwayGroupID != command.RunwayGroupID {
			return sequence.CommandChange{}, &aman.DomainError{Class: aman.ErrorInvalidTransition, Message: "requested opportunity was not retained by sequencing"}
		}
		placed.FrozenSlot = retargetSlot(placed.Slot, command.RunwayGroupID)
		var gapID aman.RunwayGapID
		if overridden != nil {
			gapID = overridden.ID
			placed.RunwayGapException = &aman.RunwayGapException{
				GapID: gapID, FlightID: placed.ID, RunwayGroupID: command.RunwayGroupID,
				Opportunity: command.SlotTime, CommandID: command.Metadata.CommandID,
			}
		}
		return s.commandChange(candidate, true, "place_flight_at_time", command.FlightID, map[string]any{
			"airport": auth.Airport, "actor": auth.Actor, "role": auth.Role, "received_at": auth.ReceivedAt,
			"prior_slot": priorSlot, "new_slot": placed.Slot, "allow_gap": command.AllowGap, "overridden_gap_id": gapID,
		})
	}, nil
}

// ChangeRunway builds and validates a complete candidate before returning it
// to the coordinator. Protected flights keep their committed time and order;
// incompatible or conflicting candidates never reach persistence.
func (s *Service) ChangeRunway(auth aman.CommandContext, command aman.ChangeRunwayCommand) (sequence.CommandMutation, error) {
	return func(state aman.AirportState) (sequence.CommandChange, error) {
		index := flightIndex(state.Flights, command.FlightID)
		if index < 0 {
			return sequence.CommandChange{}, domainNotFound(command.FlightID)
		}
		flight := state.Flights[index]
		if flight.State == aman.StateLanded || flight.State == aman.StateRemoved {
			return sequence.CommandChange{}, &aman.DomainError{Class: aman.ErrorInvalidTransition, Message: "inactive AMAN flight cannot change runway"}
		}
		active := false
		for _, group := range state.ActiveRunwayGroups {
			active = active || group == command.RunwayGroupID
		}
		if !active {
			return sequence.CommandChange{}, &aman.DomainError{Class: aman.ErrorInvalidArgument, Message: "requested runway group is not active"}
		}
		if !s.runwayAssignmentCompatible(flight, command.RunwayGroupID) {
			return sequence.CommandChange{}, &aman.DomainError{Class: aman.ErrorInvalidArgument, Message: "requested runway group is not compatible with the flight's arrival"}
		}
		beforeGroup := flight.SelectedRunwayGroup
		if beforeGroup != nil && *beforeGroup == command.RunwayGroupID {
			return s.commandChange(state, false, "change_runway", command.FlightID, map[string]any{
				"airport": auth.Airport, "actor": auth.Actor, "role": auth.Role, "received_at": auth.ReceivedAt,
				"before_runway_group_id": *beforeGroup, "after_runway_group_id": command.RunwayGroupID,
			})
		}

		candidate := state
		candidate.Flights = append([]aman.AMANFlight(nil), state.Flights...)
		target := &candidate.Flights[index]
		oldSlot, oldFrozenSlot, oldOrder, oldManualOrder := target.Slot, target.FrozenSlot, target.Order, target.ManualOrder
		protected := target.State == aman.StateStable || target.FreezeReason != aman.FreezeNone || target.ManualOrder != nil
		if protected && oldSlot == nil {
			return sequence.CommandChange{}, &aman.DomainError{Class: aman.ErrorInvalidTransition, Message: "protected flight has no committed slot to preserve"}
		}
		assignFlightToRunwayGroup(target, command.RunwayGroupID)
		if protected {
			target.Slot, target.FrozenSlot, target.Order, target.ManualOrder = retargetSlot(oldSlot, command.RunwayGroupID), retargetSlot(oldFrozenSlot, command.RunwayGroupID), oldOrder, oldManualOrder
		}
		target.UpdatedAt = auth.ReceivedAt
		input := s.sequenceInput(candidate)
		result, err := sequence.Generate(input)
		if err != nil || result.HasConflicts() {
			return sequence.CommandChange{}, &aman.DomainError{Class: aman.ErrorInvalidTransition, Message: "requested runway change cannot produce a legal atomic sequence"}
		}
		candidate = s.applyDecision(candidate, sequence.Decision{Input: input, Candidate: result, Changed: true})
		displaced := make([]map[string]any, 0)
		for i, before := range state.Flights {
			after := candidate.Flights[i]
			if before.ID == command.FlightID || reflect.DeepEqual(before.Slot, after.Slot) {
				continue
			}
			displaced = append(displaced, map[string]any{"flight_id": before.ID, "before_slot": before.Slot, "after_slot": after.Slot})
		}
		beforeID := aman.RunwayGroupID("")
		if beforeGroup != nil {
			beforeID = *beforeGroup
		}
		return s.commandChange(candidate, true, "change_runway", command.FlightID, map[string]any{
			"airport": auth.Airport, "actor": auth.Actor, "role": auth.Role, "received_at": auth.ReceivedAt,
			"before_runway_group_id": beforeID, "after_runway_group_id": command.RunwayGroupID, "displacements": displaced,
		})
	}, nil
}

func retargetSlot(slot *aman.Slot, group aman.RunwayGroupID) *aman.Slot {
	if slot == nil {
		return nil
	}
	copy := *slot
	copy.RunwayGroupID = group
	return &copy
}

func (s *Service) LockFlight(auth aman.CommandContext, command aman.LockFlightCommand) (sequence.CommandMutation, error) {
	return s.sequenceMutation("lock_flight", command.FlightID, auth.ReceivedAt, func(input sequence.Input) (sequence.Decision, error) {
		return sequence.ApplyManualFreeze(input, sequence.ApplyManualFreezeCommand{Metadata: command.Metadata, FlightID: command.FlightID, At: auth.ReceivedAt})
	}), nil
}

func (s *Service) UnlockFlight(auth aman.CommandContext, command aman.UnlockFlightCommand) (sequence.CommandMutation, error) {
	return s.sequenceMutation("unlock_flight", command.FlightID, auth.ReceivedAt, func(input sequence.Input) (sequence.Decision, error) {
		return sequence.ReleaseManualFreeze(input, sequence.ReleaseManualFreezeCommand{Metadata: command.Metadata, FlightID: command.FlightID, At: auth.ReceivedAt})
	}), nil
}

func (s *Service) DesequenceFlight(auth aman.CommandContext, command aman.DesequenceFlightCommand) (sequence.CommandMutation, error) {
	if err := s.authorizeFlightDisposition(auth); err != nil {
		return nil, err
	}
	return func(state aman.AirportState) (sequence.CommandChange, error) {
		index := flightIndex(state.Flights, command.FlightID)
		if index < 0 {
			return sequence.CommandChange{}, domainNotFound(command.FlightID)
		}
		before := state.Flights[index]
		if before.State == aman.StateRemoved {
			return sequence.CommandChange{}, &aman.DomainError{Class: aman.ErrorInvalidTransition, Message: "removed AMAN flight cannot be desequenced"}
		}
		if before.SequenceDisposition == aman.SequenceDispositionDesequenced {
			return s.dispositionChange(state, false, "desequence_flight", auth, before, before)
		}
		state.Flights = append([]aman.AMANFlight(nil), state.Flights...)
		state.Flights[index].SequenceDisposition = aman.SequenceDispositionDesequenced
		state.Flights[index].UpdatedAt = auth.ReceivedAt
		s.resequence(&state, auth.ReceivedAt)
		return s.dispositionChange(state, true, "desequence_flight", auth, before, state.Flights[index])
	}, nil
}

func (s *Service) ResumeFlight(auth aman.CommandContext, command aman.ResumeFlightCommand) (sequence.CommandMutation, error) {
	if err := s.authorizeFlightDisposition(auth); err != nil {
		return nil, err
	}
	return func(state aman.AirportState) (sequence.CommandChange, error) {
		index := flightIndex(state.Flights, command.FlightID)
		if index < 0 {
			return sequence.CommandChange{}, domainNotFound(command.FlightID)
		}
		before := state.Flights[index]
		if before.SequenceDisposition.Participates() {
			return s.dispositionChange(state, false, "resume_flight", auth, before, before)
		}
		if before.State == aman.StateLanded || before.State == aman.StateRemoved || before.Prediction == nil || before.SelectedRunwayGroup == nil {
			return sequence.CommandChange{}, &aman.DomainError{Class: aman.ErrorInvalidTransition, Message: "desequenced flight has no resumable operational prediction"}
		}
		if !slices.Contains(state.ActiveRunwayGroups, *before.SelectedRunwayGroup) {
			return sequence.CommandChange{}, &aman.DomainError{Class: aman.ErrorInvalidTransition, Message: "desequenced flight is not assigned to an active runway group"}
		}

		candidate := state
		candidate.Flights = append([]aman.AMANFlight(nil), state.Flights...)
		candidate.Flights[index].SequenceDisposition = aman.SequenceDispositionActive
		input := s.sequenceInput(candidate)
		for i := range input.Flights {
			if input.Flights[i].ID != command.FlightID {
				continue
			}
			input.Flights[i].FreezeReason, input.Flights[i].FrozenAt = aman.FreezeNone, nil
			input.Flights[i].FrozenOperationalTETA, input.Flights[i].CapturedSlot = nil, nil
			input.Flights[i].CurrentSlot, input.Flights[i].ManualOrder = nil, nil
			input.Flights[i].ProtectCurrentSlot = false
		}
		result, err := sequence.Generate(input)
		if err != nil || result.HasConflicts() {
			return sequence.CommandChange{}, &aman.DomainError{Class: aman.ErrorInvalidTransition, Message: "resume could not produce a complete legal sequence"}
		}
		if !slices.ContainsFunc(result.Entries, func(entry sequence.CandidateEntry) bool { return entry.FlightID == command.FlightID }) {
			return sequence.CommandChange{}, &aman.DomainError{Class: aman.ErrorInvalidTransition, Message: "resume could not produce a complete legal sequence"}
		}
		candidate = s.applyDecision(candidate, sequence.Decision{Input: input, Candidate: result, Changed: true})
		after := &candidate.Flights[index]
		after.UpdatedAt = auth.ReceivedAt
		after.FreezeReason, after.FrozenAt, after.FrozenOperationalTETA = before.FreezeReason, before.FrozenAt, before.FrozenOperationalTETA
		if before.FreezeReason != aman.FreezeNone && after.Slot != nil {
			after.FrozenSlot = retargetSlot(after.Slot, after.Slot.RunwayGroupID)
		}
		if before.ManualOrder != nil && after.Order != nil {
			order := *after.Order
			after.ManualOrder = &order
		}
		return s.dispositionChange(candidate, true, "resume_flight", auth, before, *after)
	}, nil
}

func (s *Service) RemoveFlight(auth aman.CommandContext, command aman.RemoveFlightCommand) (sequence.CommandMutation, error) {
	if err := s.authorizeFlightDisposition(auth); err != nil {
		return nil, err
	}
	return func(state aman.AirportState) (sequence.CommandChange, error) {
		index := flightIndex(state.Flights, command.FlightID)
		if index < 0 {
			return sequence.CommandChange{}, domainNotFound(command.FlightID)
		}
		before := state.Flights[index]
		result, err := lifecycle.Reduce(lifecycle.DefaultConfig(), before, lifecycle.Event{
			ID: command.Metadata.CommandID, Kind: lifecycle.EventManualRemoval, OccurredAt: auth.ReceivedAt,
		})
		if err != nil {
			return sequence.CommandChange{}, err
		}
		state.Flights = append([]aman.AMANFlight(nil), state.Flights...)
		state.Flights[index] = result.Flight
		clearSequencingState(&state.Flights[index])
		expireActiveRouteFact(&state.Flights[index])
		s.resequence(&state, auth.ReceivedAt)
		return s.dispositionChange(state, true, "remove_flight", auth, before, state.Flights[index])
	}, nil
}

func (s *Service) authorizeFlightDisposition(auth aman.CommandContext) error {
	for _, role := range s.deps.FMPRoles {
		if strings.EqualFold(strings.TrimSpace(role), auth.Role) {
			return nil
		}
	}
	return &aman.DomainError{Class: aman.ErrorUnauthorized, Message: "flight disposition command requires a configured FMP role"}
}

func (s *Service) dispositionChange(state aman.AirportState, changed bool, action string, auth aman.CommandContext, before, after aman.AMANFlight) (sequence.CommandChange, error) {
	return s.commandChange(state, changed, action, before.ID, map[string]any{
		"airport": auth.Airport, "actor": auth.Actor, "role": auth.Role, "received_at": auth.ReceivedAt,
		"before_disposition": before.SequenceDisposition.OrDefault(), "after_disposition": after.SequenceDisposition.OrDefault(),
		"before_slot": slotAudit(before.Slot), "after_slot": slotAudit(after.Slot), "before_state": before.State, "after_state": after.State,
		"before_removed": before.State == aman.StateRemoved, "after_removed": after.State == aman.StateRemoved,
		"before_removal_reason": lifecycleReason(before), "after_removal_reason": lifecycleReason(after),
	})
}

func slotAudit(slot *aman.Slot) any {
	if slot == nil {
		return nil
	}
	return map[string]any{"time": slot.Time, "runway_group_id": slot.RunwayGroupID, "sequence": slot.Sequence, "reason": slot.Reason}
}

func lifecycleReason(flight aman.AMANFlight) aman.LifecycleReason {
	if flight.Lifecycle == nil {
		return ""
	}
	return flight.Lifecycle.Reason
}

func (s *Service) SetRate(auth aman.CommandContext, command aman.SetRateCommand) (sequence.CommandMutation, error) {
	return func(state aman.AirportState) (sequence.CommandChange, error) {
		input := s.sequenceInput(state)
		decision, err := sequence.ApplyRate(input, sequence.SetRateCommand{Metadata: command.Metadata, RunwayGroupID: command.RunwayGroupID, ArrivalsPerHour: command.ArrivalsPerHour, EffectiveAt: command.EffectiveAt})
		if err != nil {
			return sequence.CommandChange{}, err
		}
		for _, warning := range decision.Candidate.Warnings {
			if warning.RunwayGroupID == command.RunwayGroupID && warning.Severity == sequence.SeverityConflict && warning.Code == sequence.WarningProtectedSameSTAR {
				return sequence.CommandChange{}, &aman.DomainError{
					Class: aman.ErrorInvalidTransition, Message: string(sequence.WarningProtectedSameSTAR) + ": protected slots prevent the requested runway-group rate",
				}
			}
		}
		state = s.applyDecision(state, decision)
		for i := range state.RunwayGroups {
			if state.RunwayGroups[i].ID == command.RunwayGroupID {
				for _, policy := range decision.Input.Policies {
					if policy.RunwayGroupID != command.RunwayGroupID {
						continue
					}
					state.RunwayGroups[i].RateSchedule = make([]aman.RunwayGroupRatePoint, len(policy.Rates))
					for rateIndex, rate := range policy.Rates {
						state.RunwayGroups[i].RateSchedule[rateIndex] = aman.RunwayGroupRatePoint{EffectiveAt: rate.EffectiveAt, ArrivalsPerHour: rate.ArrivalsPerHour}
					}
					break
				}
			}
		}
		updateActiveRates(state.RunwayGroups, auth.ReceivedAt)
		var promotions []sequence.VacancyPromotion
		if decision.Changed {
			promotions = s.resequence(&state, auth.ReceivedAt)
		}
		change, err := s.commandChange(state, decision.Changed, "set_rate", "", map[string]any{"runway_group_id": command.RunwayGroupID, "arrivals_per_hour": command.ArrivalsPerHour})
		change.Audit = append(change.Audit, vacancyPromotionAuditEntries(promotions)...)
		return change, err
	}, nil
}

func (s *Service) SelectRunwayGroup(auth aman.CommandContext, command aman.SelectRunwayGroupCommand) (sequence.CommandMutation, error) {
	return func(state aman.AirportState) (sequence.CommandChange, error) {
		groupIndex := -1
		for index := range state.RunwayGroups {
			if state.RunwayGroups[index].ID == command.RunwayGroupID {
				groupIndex = index
				break
			}
		}
		if groupIndex < 0 {
			return sequence.CommandChange{}, &aman.DomainError{Class: aman.ErrorNotFound, Message: "runway selection group was not found"}
		}

		state.RunwayGroups = append([]aman.RunwayGroupPolicy(nil), state.RunwayGroups...)
		if legacySelected, reset := discardLegacyRunwayGroupSelections(state.RunwayGroups); reset {
			state.Flights = append([]aman.AMANFlight(nil), state.Flights...)
			reassignFlightsToGroup(&state, legacySelected)
		}
		before := append([]aman.RunwayGroupSelectionPoint(nil), state.RunwayGroups[groupIndex].SelectionSchedule...)
		state.RunwayGroups[groupIndex].SelectionSchedule = upsertRunwayGroupSelection(
			before,
			aman.RunwayGroupSelectionPoint{
				EffectiveAt: command.EffectiveAt, CommandRevision: state.Revision + 1,
				Source: aman.RunwayGroupSelectionSourceFMPCommand,
			},
		)
		scheduleChanged := !reflect.DeepEqual(before, state.RunwayGroups[groupIndex].SelectionSchedule)
		protected := []aman.FlightID{}
		for _, flight := range state.Flights {
			if flight.SelectedRunwayGroup != nil && *flight.SelectedRunwayGroup != command.RunwayGroupID &&
				(flight.State == aman.StateStable || flight.FreezeReason != aman.FreezeNone) {
				protected = append(protected, flight.ID)
			}
		}
		selected, selectionChanged := selectedRunwayGroupAt(state.RunwayGroups, auth.ReceivedAt)
		if selectionChanged {
			if err := s.activateRunwayGroup(&state, selected, auth.ReceivedAt); err != nil {
				return sequence.CommandChange{}, err
			}
		} else {
			clearRunwayGroupSelectionConflicts(state.RunwayGroups)
		}
		return s.commandChange(state, scheduleChanged || selectionChanged, "select_runway_group", "", map[string]any{
			"runway_group_id": command.RunwayGroupID, "effective_at": command.EffectiveAt, "protected_flight_ids": protected,
		})
	}, nil
}

func (s *Service) SetActiveRunwayGroups(auth aman.CommandContext, command aman.SetActiveRunwayGroupsCommand) (sequence.CommandMutation, error) {
	return func(state aman.AirportState) (sequence.CommandChange, error) {
		if !runwayGroupsMatchTerminal(state.RunwayGroups, s.deps.Terminal.RunwayGroups) {
			return sequence.CommandChange{}, &aman.DomainError{Class: aman.ErrorInvalidArgument, Message: "airport runway groups do not match active terminal configuration"}
		}
		requested := make(map[aman.RunwayGroupID]struct{}, len(command.RunwayGroupIDs))
		for _, id := range command.RunwayGroupIDs {
			requested[id] = struct{}{}
		}
		ordered := make([]aman.RunwayGroupID, 0, len(requested))
		for _, group := range s.deps.Terminal.RunwayGroups {
			if _, active := requested[group.ID]; active {
				ordered = append(ordered, group.ID)
				delete(requested, group.ID)
			}
		}
		if len(requested) > 0 {
			return sequence.CommandChange{}, &aman.DomainError{Class: aman.ErrorNotFound, Message: "active runway group was not found"}
		}
		if !s.activeRunwayGroupSetConfigured(ordered) {
			return sequence.CommandChange{}, &aman.DomainError{Class: aman.ErrorInvalidArgument, Message: "active runway group set is not operationally compatible"}
		}

		beforeActive := append([]aman.RunwayGroupID(nil), state.ActiveRunwayGroups...)
		beforeGroups := append([]aman.RunwayGroupPolicy(nil), state.RunwayGroups...)
		state.ActiveRunwayGroups = append([]aman.RunwayGroupID(nil), ordered...)
		state.RunwayGroups = append([]aman.RunwayGroupPolicy(nil), state.RunwayGroups...)
		for i := range state.RunwayGroups {
			state.RunwayGroups[i].Selected = state.RunwayGroups[i].ID == ordered[0]
			state.RunwayGroups[i].SelectionSchedule = nil
			state.RunwayGroups[i].SelectionConflict = nil
		}
		beforeFlights := append([]aman.AMANFlight(nil), state.Flights...)
		protectedIncompatible, err := s.reconcileActiveRunwayAssignments(&state, ordered)
		if err != nil {
			return sequence.CommandChange{}, err
		}
		changed := !reflect.DeepEqual(beforeActive, state.ActiveRunwayGroups) ||
			!reflect.DeepEqual(beforeGroups, state.RunwayGroups) || !reflect.DeepEqual(beforeFlights, state.Flights)
		return s.commandChange(state, changed, "set_active_runway_groups", "", map[string]any{
			"runway_group_ids": ordered, "airport": auth.Airport, "actor": auth.Actor,
			"role": auth.Role, "received_at": auth.ReceivedAt,
			"protected_incompatible_flight_ids": protectedIncompatible,
		})
	}, nil
}

func (s *Service) CreateRunwayGap(auth aman.CommandContext, command aman.CreateRunwayGapCommand) (sequence.CommandMutation, error) {
	if err := s.authorizeRunwayGap(auth); err != nil {
		return nil, err
	}
	return func(state aman.AirportState) (sequence.CommandChange, error) {
		groupIndex := runwayGroupIndex(state.RunwayGroups, command.RunwayGroupID)
		if groupIndex < 0 {
			return sequence.CommandChange{}, &aman.DomainError{Class: aman.ErrorNotFound, Message: "AMAN runway group was not found"}
		}
		interval, err := aman.NormalizeRunwayGapInterval(command.Interval, state.RunwayGroups[groupIndex].ActiveRatePerHour)
		if err != nil {
			return sequence.CommandChange{}, err
		}
		merged, err := aman.MergeRunwayGap(state.RunwayGroups, aman.RunwayGapMergeInput{
			RunwayGroupID: command.RunwayGroupID, CommandID: command.Metadata.CommandID,
			Interval: interval, Label: command.Label, CreatedAt: auth.ReceivedAt, CreatedBy: auth.Actor,
		})
		if err != nil {
			return sequence.CommandChange{}, err
		}
		state.RunwayGroups = merged.RunwayGroups
		displacements, err := s.displaceFlightsFromRunwayGap(&state, command.RunwayGroupID, merged.Union)
		if err != nil {
			return sequence.CommandChange{}, err
		}
		change, err := commandChange(state, true, "create_runway_gap", "", map[string]any{
			"airport": auth.Airport, "actor": auth.Actor, "role": auth.Role, "received_at": auth.ReceivedAt,
			"runway_group_id": command.RunwayGroupID, "gap_id": merged.Union.ID, "label": merged.Union.Label,
			"before_interval": gapIntervalAudit(interval.Start(), interval.End()),
			"after_interval":  gapIntervalAudit(merged.Union.Start, merged.Union.End), "replaced_ids": merged.ReplacedIDs,
			"displaced_flight_ids": gapDisplacementIDs(displacements),
		})
		if err != nil {
			return sequence.CommandChange{}, err
		}
		queueInput := s.sequenceInput(state)
		if len(queueInput.Flights) > 0 && len(queueInput.Policies) > 0 {
			change.QueueOffers = &sequence.QueueOfferCalculation{Input: queueInput, Config: sequence.QueueOfferConfig{Validity: queueOfferValidity}}
		}
		for _, displacement := range displacements {
			payload, marshalErr := json.Marshal(map[string]any{
				"action": "runway_gap_displacement", "gap_id": merged.Union.ID,
				"runway_group_id": command.RunwayGroupID, "flight_id": displacement.FlightID,
				"previous_opportunity": displacement.Previous, "new_opportunity": displacement.New,
				"overridden_protection_reason": displacement.ProtectionReason,
			})
			if marshalErr != nil {
				return sequence.CommandChange{}, marshalErr
			}
			change.Audit = append(change.Audit, sequence.AuditEntry{Category: "aman.runway_gap_displacement", Payload: payload})
		}
		return change, nil
	}, nil
}

type gapOpportunity struct {
	Time          time.Time          `json:"time"`
	RunwayGroupID aman.RunwayGroupID `json:"runway_group_id"`
	Sequence      int                `json:"sequence"`
}

type gapDisplacement struct {
	FlightID         aman.FlightID
	Previous         gapOpportunity
	New              gapOpportunity
	ProtectionReason string
}

func (s *Service) displaceFlightsFromRunwayGap(state *aman.AirportState, groupID aman.RunwayGroupID, gap aman.RunwayGap) ([]gapDisplacement, error) {
	affected := make(map[aman.FlightID]struct{})
	cascade := make(map[aman.FlightID]gapDisplacement)
	for _, flight := range state.Flights {
		if flight.SelectedRunwayGroup == nil || *flight.SelectedRunwayGroup != groupID || flight.Slot == nil || flight.Slot.Time.Before(gap.Start) {
			continue
		}
		cascade[flight.ID] = gapDisplacement{
			FlightID: flight.ID, Previous: gapOpportunity{Time: flight.Slot.Time, RunwayGroupID: flight.Slot.RunwayGroupID, Sequence: flight.Slot.Sequence},
			ProtectionReason: gapProtectionReason(flight),
		}
		if flight.Slot.Time.Before(gap.End) {
			affected[flight.ID] = struct{}{}
		}
	}
	if len(affected) == 0 {
		return nil, nil
	}

	input := s.sequenceInput(*state)
	earlyTolerance := time.Duration(0)
	for _, policy := range input.Policies {
		if policy.RunwayGroupID == groupID {
			earlyTolerance = policy.EarlyTolerance
			break
		}
	}
	original := make(map[aman.FlightID]sequence.Flight, len(cascade))
	orderedIDs := make([]aman.FlightID, 0, len(cascade))
	for id := range cascade {
		orderedIDs = append(orderedIDs, id)
	}
	sort.Slice(orderedIDs, func(i, j int) bool {
		left, right := cascade[orderedIDs[i]].Previous, cascade[orderedIDs[j]].Previous
		if left.Sequence != right.Sequence {
			return left.Sequence < right.Sequence
		}
		if !left.Time.Equal(right.Time) {
			return left.Time.Before(right.Time)
		}
		return orderedIDs[i] < orderedIDs[j]
	})
	sequenceOrder := make(map[aman.FlightID]int, len(orderedIDs))
	for index, id := range orderedIDs {
		sequenceOrder[id] = index + 1
	}
	for index := range input.Flights {
		displacement, cascades := cascade[input.Flights[index].ID]
		if !cascades {
			continue
		}
		original[input.Flights[index].ID] = input.Flights[index]
		input.Flights[index].FreezeReason = aman.FreezeNone
		input.Flights[index].FrozenAt = nil
		input.Flights[index].FrozenOperationalTETA = nil
		input.Flights[index].CapturedSlot = nil
		input.Flights[index].ProtectCurrentSlot = false
		order := sequenceOrder[input.Flights[index].ID]
		input.Flights[index].ManualOrder = &order
		earliest := displacement.Previous.Time.Add(earlyTolerance).Add(time.Nanosecond)
		if _, directlyAffected := affected[input.Flights[index].ID]; directlyAffected && gap.End.After(earliest) {
			earliest = gap.End
		}
		if input.Flights[index].OperationalTETA.Before(earliest) {
			input.Flights[index].OperationalTETA = earliest
		}
	}
	if len(original) != len(cascade) {
		return nil, &aman.DomainError{Class: aman.ErrorInvalidTransition, Message: "runway GAP contains a flight that is not eligible for a complete sequence"}
	}

	result, err := sequence.Generate(input)
	if err != nil || result.HasConflicts() {
		return nil, &aman.DomainError{Class: aman.ErrorInvalidTransition, Message: "runway GAP cannot produce a legal atomic sequence"}
	}
	for _, entry := range result.Entries {
		displacement, cascades := cascade[entry.FlightID]
		if !cascades {
			continue
		}
		if _, directlyAffected := affected[entry.FlightID]; directlyAffected && entry.Time.Before(gap.End) {
			return nil, &aman.DomainError{Class: aman.ErrorInvalidTransition, Message: "runway GAP displacement did not produce a later opportunity"}
		}
		displacement.New = gapOpportunity{Time: entry.Time, RunwayGroupID: entry.RunwayGroupID, Sequence: entry.Sequence}
		cascade[entry.FlightID] = displacement
	}
	ordered := make([]gapDisplacement, 0, len(cascade))
	for id, displacement := range cascade {
		if displacement.New.Time.IsZero() {
			return nil, &aman.DomainError{Class: aman.ErrorInvalidTransition, Message: fmt.Sprintf("runway GAP displacement omitted flight %q", id)}
		}
		if !displacement.New.Time.After(displacement.Previous.Time) {
			return nil, &aman.DomainError{Class: aman.ErrorInvalidTransition, Message: fmt.Sprintf("runway GAP displacement did not move flight %q behind its previous opportunity", id)}
		}
		ordered = append(ordered, displacement)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].FlightID < ordered[j].FlightID })
	for index := range input.Flights {
		if saved, affected := original[input.Flights[index].ID]; affected {
			input.Flights[index] = saved
		}
	}
	*state = s.applyDecision(*state, sequence.Decision{Input: input, Candidate: result, Changed: true})
	for index := range state.Flights {
		if _, cascades := cascade[state.Flights[index].ID]; cascades && state.Flights[index].FreezeReason != aman.FreezeNone {
			state.Flights[index].FrozenSlot = retargetSlot(state.Flights[index].Slot, groupID)
		}
	}
	return ordered, nil
}

func gapProtectionReason(flight aman.AMANFlight) string {
	if flight.FreezeReason != aman.FreezeNone {
		return string(flight.FreezeReason)
	}
	if flight.State == aman.StateStable {
		return "stable"
	}
	if flight.ManualOrder != nil {
		return "manual_order"
	}
	return "none"
}

func gapDisplacementIDs(displacements []gapDisplacement) []aman.FlightID {
	ids := make([]aman.FlightID, len(displacements))
	for index := range displacements {
		ids[index] = displacements[index].FlightID
	}
	return ids
}

func (s *Service) RemoveRunwayGap(auth aman.CommandContext, command aman.RemoveRunwayGapCommand) (sequence.CommandMutation, error) {
	if err := s.authorizeRunwayGap(auth); err != nil {
		return nil, err
	}
	return func(state aman.AirportState) (sequence.CommandChange, error) {
		groupIndex := runwayGroupIndex(state.RunwayGroups, command.RunwayGroupID)
		if groupIndex < 0 {
			return sequence.CommandChange{}, &aman.DomainError{Class: aman.ErrorNotFound, Message: "AMAN runway group was not found"}
		}
		state.RunwayGroups = append([]aman.RunwayGroupPolicy(nil), state.RunwayGroups...)
		group := &state.RunwayGroups[groupIndex]
		group.Gaps = append([]aman.RunwayGap(nil), group.Gaps...)
		gapIndex := -1
		for index := range group.Gaps {
			if group.Gaps[index].ID == command.GapID {
				gapIndex = index
				break
			}
		}
		if gapIndex < 0 {
			return sequence.CommandChange{}, &aman.DomainError{Class: aman.ErrorNotFound, Message: "AMAN runway GAP was not found"}
		}
		removed := group.Gaps[gapIndex]
		group.Gaps = append(group.Gaps[:gapIndex], group.Gaps[gapIndex+1:]...)
		state.Flights = append([]aman.AMANFlight(nil), state.Flights...)
		for index := range state.Flights {
			if exception := state.Flights[index].RunwayGapException; exception != nil && exception.GapID == removed.ID {
				state.Flights[index].RunwayGapException = nil
			}
		}
		return commandChange(state, true, "remove_runway_gap", "", map[string]any{
			"airport": auth.Airport, "actor": auth.Actor, "role": auth.Role, "received_at": auth.ReceivedAt,
			"runway_group_id": command.RunwayGroupID, "gap_id": removed.ID,
			"before_interval": gapIntervalAudit(removed.Start, removed.End), "after_interval": nil,
			"removed_ids": []aman.RunwayGapID{removed.ID},
		})
	}, nil
}

func (s *Service) authorizeRunwayGap(auth aman.CommandContext) error {
	for _, role := range s.deps.FMPRoles {
		if strings.EqualFold(strings.TrimSpace(role), auth.Role) {
			return nil
		}
	}
	return &aman.DomainError{Class: aman.ErrorUnauthorized, Message: "runway capacity command requires a configured FMP role"}
}

func runwayGroupIndex(groups []aman.RunwayGroupPolicy, id aman.RunwayGroupID) int {
	for index := range groups {
		if groups[index].ID == id {
			return index
		}
	}
	return -1
}

func gapIntervalAudit(start, end time.Time) map[string]time.Time {
	return map[string]time.Time{"start": start, "end": end}
}

func (s *Service) reconcileActiveRunwayAssignments(state *aman.AirportState, active []aman.RunwayGroupID) ([]aman.FlightID, error) {
	state.Flights = append([]aman.AMANFlight(nil), state.Flights...)
	input := s.sequenceInput(*state)
	working := input
	working.Flights = nil
	activeSet := make(map[aman.RunwayGroupID]struct{}, len(active))
	for _, group := range active {
		activeSet[group] = struct{}{}
	}

	movable := make([]sequence.Flight, 0, len(input.Flights))
	protectedIncompatible := make([]aman.FlightID, 0)
	for _, flight := range state.Flights {
		if flight.SelectedRunwayGroup == nil || (flight.State != aman.StateStable && flight.FreezeReason == aman.FreezeNone) ||
			flight.State == aman.StateLanded || flight.State == aman.StateRemoved {
			continue
		}
		_, isActive := activeSet[*flight.SelectedRunwayGroup]
		if !isActive || !s.runwayAssignmentCompatible(flight, *flight.SelectedRunwayGroup) {
			protectedIncompatible = append(protectedIncompatible, flight.ID)
		}
	}
	for _, flight := range input.Flights {
		index := flightIndex(state.Flights, flight.ID)
		if index < 0 {
			continue
		}
		operational := state.Flights[index]
		if operational.State != aman.StateStable && operational.FreezeReason == aman.FreezeNone {
			movable = append(movable, flight)
			continue
		}
		working.Flights = append(working.Flights, flight)
	}
	sort.Slice(movable, func(i, j int) bool {
		if !movable[i].OperationalTETA.Equal(movable[j].OperationalTETA) {
			return movable[i].OperationalTETA.Before(movable[j].OperationalTETA)
		}
		return movable[i].ID < movable[j].ID
	})

	for _, flight := range movable {
		index := flightIndex(state.Flights, flight.ID)
		var selected aman.RunwayGroupID
		var earliest time.Time
		for _, group := range active {
			if !s.runwayAssignmentCompatible(state.Flights[index], group) {
				continue
			}
			candidate := flight
			candidate.RunwayGroupID = group
			candidate.CurrentSlot = nil
			candidate.ManualOrder = nil
			trial := working
			trial.Flights = append(append([]sequence.Flight(nil), working.Flights...), candidate)
			result, err := sequence.Generate(trial)
			if err != nil {
				return nil, &aman.DomainError{Class: aman.ErrorInvalidTransition, Message: "active runway assignment could not produce a valid sequence"}
			}
			for _, entry := range result.Entries {
				if entry.FlightID == flight.ID && (selected == "" || entry.Time.Before(earliest)) {
					selected, earliest = group, entry.Time
					break
				}
			}
		}
		if selected == "" {
			return nil, &aman.DomainError{Class: aman.ErrorInvalidTransition, Message: fmt.Sprintf("flight %q has no compatible active runway opportunity", flight.ID)}
		}
		assignFlightToRunwayGroup(&state.Flights[index], selected)
		flight.RunwayGroupID, flight.CurrentSlot, flight.ManualOrder = selected, nil, nil
		working.Flights = append(working.Flights, flight)
	}

	if len(working.Flights) > 0 {
		result, err := sequence.Generate(working)
		if err != nil {
			return nil, &aman.DomainError{Class: aman.ErrorInvalidTransition, Message: "active runway assignments could not produce a valid sequence"}
		}
		*state = s.applyDecision(*state, sequence.Decision{Input: working, Candidate: result, Changed: true})
	}
	sort.Slice(protectedIncompatible, func(i, j int) bool { return protectedIncompatible[i] < protectedIncompatible[j] })
	return protectedIncompatible, nil
}

func (s *Service) runwayAssignmentCompatible(flight aman.AMANFlight, group aman.RunwayGroupID) bool {
	if flight.SelectedFeeder == nil {
		return false
	}
	for _, path := range s.deps.Terminal.Paths {
		if string(path.Feeder) == *flight.SelectedFeeder && path.RunwayGroup == group {
			return true
		}
	}
	return false
}

func assignFlightToRunwayGroup(flight *aman.AMANFlight, group aman.RunwayGroupID) {
	flight.SelectedRunwayGroup = &group
	flight.SelectedHolding, flight.HoldingStack = nil, nil
	flight.ActiveRouteKey, flight.ActiveRouteDatasetID, flight.RouteProgress = nil, nil, nil
	flight.Slot, flight.Order, flight.ManualOrder = nil, nil, nil
	flight.QueueOffers = nil
}

func (s *Service) activeRunwayGroupSetConfigured(requested []aman.RunwayGroupID) bool {
	for _, configured := range s.deps.Terminal.ActiveRunwayGroupSets {
		if len(configured) != len(requested) {
			continue
		}
		members := make(map[aman.RunwayGroupID]struct{}, len(configured))
		for _, id := range configured {
			members[id] = struct{}{}
		}
		compatible := true
		for _, id := range requested {
			_, compatible = members[id]
			if !compatible {
				break
			}
		}
		if compatible {
			return true
		}
	}
	return false
}

func selectedRunwayGroupAt(groups []aman.RunwayGroupPolicy, now time.Time) (aman.RunwayGroupID, bool) {
	working := append([]aman.RunwayGroupPolicy(nil), groups...)
	return updateSelectedRunwayGroup(working, now)
}

func (s *Service) activateRunwayGroup(state *aman.AirportState, selected aman.RunwayGroupID, now time.Time) error {
	candidate := *state
	candidate.Flights = append([]aman.AMANFlight(nil), state.Flights...)
	candidate.RunwayGroups = append([]aman.RunwayGroupPolicy(nil), state.RunwayGroups...)
	for index := range candidate.RunwayGroups {
		candidate.RunwayGroups[index].Selected = candidate.RunwayGroups[index].ID == selected
		candidate.RunwayGroups[index].SelectionConflict = nil
	}
	candidate.ActiveRunwayGroups = []aman.RunwayGroupID{selected}
	reassignFlightsToGroup(&candidate, selected)
	input := s.sequenceInput(candidate)
	if len(input.Flights) > 0 && len(input.Policies) > 0 {
		generated, err := sequence.Generate(input)
		if err != nil {
			return &aman.DomainError{Class: aman.ErrorInvalidTransition, Message: "runway selection could not produce a valid sequence"}
		}
		for _, warning := range generated.Warnings {
			if warning.RunwayGroupID == selected && warning.Severity == sequence.SeverityConflict {
				return &aman.DomainError{
					Class:   aman.ErrorInvalidTransition,
					Message: fmt.Sprintf("%s: protected flight %q conflicts with the requested runway selection", warning.Code, warning.FlightID),
				}
			}
		}
		candidate = s.applyDecision(candidate, sequence.Decision{Input: input, Candidate: generated, Changed: true})
	}
	*state = candidate
	return nil
}

func setRunwayGroupSelectionConflict(groups []aman.RunwayGroupPolicy, selected aman.RunwayGroupID, message string) {
	for index := range groups {
		if groups[index].ID == selected {
			groups[index].SelectionConflict = &message
		} else {
			groups[index].SelectionConflict = nil
		}
	}
}

func clearRunwayGroupSelectionConflicts(groups []aman.RunwayGroupPolicy) {
	for index := range groups {
		groups[index].SelectionConflict = nil
	}
}

func upsertRunwayGroupSelection(schedule []aman.RunwayGroupSelectionPoint, point aman.RunwayGroupSelectionPoint) []aman.RunwayGroupSelectionPoint {
	result := append([]aman.RunwayGroupSelectionPoint(nil), schedule...)
	replaced := false
	for index := range result {
		if result[index].EffectiveAt.Equal(point.EffectiveAt) {
			result[index] = point
			replaced = true
			break
		}
	}
	if !replaced {
		result = append(result, point)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].EffectiveAt.Before(result[j].EffectiveAt) })
	return result
}

func reassignFlightsToGroup(state *aman.AirportState, selected aman.RunwayGroupID) {
	for i := range state.Flights {
		flight := &state.Flights[i]
		if flight.State == aman.StateStable || flight.State == aman.StateLanded || flight.State == aman.StateRemoved || flight.FreezeReason != aman.FreezeNone {
			continue
		}
		if flight.SelectedRunwayGroup != nil && *flight.SelectedRunwayGroup == selected {
			continue
		}
		assignFlightToRunwayGroup(flight, selected)
	}
}

func (s *Service) AcceptTETA(auth aman.CommandContext, command aman.AcceptTETACommand) (sequence.CommandMutation, error) {
	return s.flightMutation("accept_teta", command.FlightID, func(flight aman.AMANFlight) (aman.AMANFlight, bool, error) {
		result, err := etareview.ResolveAcceptCalculated(flight, etareview.AcceptCalculated{At: auth.ReceivedAt, Actor: auth.Actor})
		return result.Flight, result.Changed, err
	}), nil
}

func (s *Service) KeepFPLETA(auth aman.CommandContext, command aman.KeepFPLETACommand) (sequence.CommandMutation, error) {
	return s.flightMutation("keep_fpl_eta", command.FlightID, func(flight aman.AMANFlight) (aman.AMANFlight, bool, error) {
		result, err := etareview.ResolveKeepInitial(flight, etareview.KeepInitial{At: auth.ReceivedAt, Actor: auth.Actor})
		return result.Flight, result.Changed, err
	}), nil
}

func (s *Service) SetManualETA(auth aman.CommandContext, command aman.SetManualETACommand) (sequence.CommandMutation, error) {
	return s.flightMutation("set_manual_eta", command.FlightID, func(flight aman.AMANFlight) (aman.AMANFlight, bool, error) {
		if flight.ETAReview != nil && flight.ETAReview.Status == aman.ReviewPending {
			result, err := etareview.ResolveSetManual(flight, etareview.SetManual{At: auth.ReceivedAt, Actor: auth.Actor, ManualTETA: command.ManualETA})
			return result.Flight, result.Changed, err
		}
		updated, err := prediction.ApplyManualOperationalTETA(flight, command.ManualETA, auth.ReceivedAt)
		return updated, err == nil, err
	}), nil
}

func (s *Service) ResetTETAOverride(auth aman.CommandContext, command aman.ResetTETAOverrideCommand) (sequence.CommandMutation, error) {
	return s.flightMutation("reset_teta_override", command.FlightID, func(flight aman.AMANFlight) (aman.AMANFlight, bool, error) {
		if flight.ETAReview != nil && flight.ETAReview.Status != aman.ReviewNone {
			result, err := etareview.ResolveReset(prediction.DefaultConfig(), flight, etareview.Reset{At: auth.ReceivedAt, Actor: auth.Actor})
			return result.Flight, result.Changed, err
		}
		updated, err := prediction.ReleaseManualOperationalTETA(prediction.DefaultConfig(), flight, auth.ReceivedAt)
		return updated, err == nil, err
	}), nil
}

func (s *Service) SetManualFeederETA(auth aman.CommandContext, command aman.SetManualFeederETACommand) (sequence.CommandMutation, error) {
	if err := command.Validate(); err != nil {
		return nil, err
	}
	return s.flightMutationWithAudit("set_manual_feeder_eta", command.FlightID, map[string]any{
		"airport": auth.Airport, "actor": auth.Actor, "role": auth.Role, "received_at": auth.ReceivedAt, "feeder_eta": command.FeederETA,
	}, func(flight aman.AMANFlight) (aman.AMANFlight, bool, error) {
		if flight.SelectedFeederFix == nil {
			return flight, false, &aman.DomainError{Class: aman.ErrorInvalidTransition, Message: "manual feeder ETA requires a selected feeder fix"}
		}
		if flight.DerivedFeederETA == nil && flight.FeederETA != nil && flight.FeederETA.Source != aman.FeederETASourceManual {
			flight.DerivedFeederETA = cloneFeederETA(flight.FeederETA)
		}
		passed := flight.DerivedFeederETA != nil && flight.DerivedFeederETA.Passed
		if command.FeederETA.Before(auth.ReceivedAt) && !passed {
			return flight, false, &aman.DomainError{Class: aman.ErrorInvalidArgument, Message: "past manual feeder ETA requires authoritative passed-feeder progress"}
		}
		manual := &aman.FeederETAState{ETA: timePointer(command.FeederETA), Source: aman.FeederETASourceManual}
		changed := !reflect.DeepEqual(flight.FeederETA, manual)
		flight.FeederETA = manual
		if changed {
			flight.UpdatedAt = auth.ReceivedAt
		}
		return flight, changed, nil
	}), nil
}

func (s *Service) ResetManualFeederETA(auth aman.CommandContext, command aman.ResetManualFeederETACommand) (sequence.CommandMutation, error) {
	if err := command.Validate(); err != nil {
		return nil, err
	}
	return s.flightMutationWithAudit("reset_manual_feeder_eta", command.FlightID, map[string]any{
		"airport": auth.Airport, "actor": auth.Actor, "role": auth.Role, "received_at": auth.ReceivedAt,
	}, func(flight aman.AMANFlight) (aman.AMANFlight, bool, error) {
		if flight.FeederETA == nil || flight.FeederETA.Source != aman.FeederETASourceManual {
			return flight, false, nil
		}
		flight.FeederETA = cloneFeederETA(flight.DerivedFeederETA)
		flight.UpdatedAt = auth.ReceivedAt
		return flight, true, nil
	}), nil
}

func (s *Service) ReportGoAround(auth aman.CommandContext, command aman.ReportGoAroundCommand) (sequence.CommandMutation, error) {
	return func(state aman.AirportState) (sequence.CommandChange, error) {
		index := flightIndex(state.Flights, command.FlightID)
		if index < 0 {
			return sequence.CommandChange{}, domainNotFound(command.FlightID)
		}
		flight := state.Flights[index]
		activeEpisode := flight.GoAroundDetection != nil && flight.GoAroundDetection.AwaitingReset
		confirmedEpisode := flight.GoAroundConfirmation != nil && flight.GoAroundConfirmation.Status == aman.GoAroundConfirmationConfirmed
		if activeEpisode && (confirmedEpisode || flight.State == aman.StateGoAround) {
			return sequence.CommandChange{}, &aman.DomainError{Class: aman.ErrorInvalidTransition, Message: "go-around episode is already confirmed"}
		}
		detectedAt := command.DetectedAt
		if pending := flight.GoAroundConfirmation; pending != nil && pending.Status == aman.GoAroundConfirmationPending {
			detectedAt = pending.DetectedAt
		}
		return s.applyConfirmedGoAround(state, index, auth, command.Metadata, detectedAt, "report_go_around")
	}, nil
}

func (s *Service) ConfirmGoAround(auth aman.CommandContext, command aman.ConfirmGoAroundCommand) (sequence.CommandMutation, error) {
	return func(state aman.AirportState) (sequence.CommandChange, error) {
		index, pending, err := pendingGoAround(state, command.FlightID, command.EpisodeID)
		if err != nil {
			return sequence.CommandChange{}, err
		}
		return s.applyConfirmedGoAround(state, index, auth, command.Metadata, pending.DetectedAt, "confirm_go_around")
	}, nil
}

func (s *Service) RejectGoAround(auth aman.CommandContext, command aman.RejectGoAroundCommand) (sequence.CommandMutation, error) {
	return func(state aman.AirportState) (sequence.CommandChange, error) {
		index, _, err := pendingGoAround(state, command.FlightID, command.EpisodeID)
		if err != nil {
			return sequence.CommandChange{}, err
		}
		state.Flights = append([]aman.AMANFlight(nil), state.Flights...)
		flight := &state.Flights[index]
		resolved := *flight.GoAroundConfirmation
		resolved.Status = aman.GoAroundConfirmationRejected
		resolved.DecidedAt, resolved.DecidedBy, resolved.DecisionCommandID = timePointer(auth.ReceivedAt), stringPointer(auth.Actor), stringPointer(command.Metadata.CommandID)
		revision := state.Revision + 1
		resolved.ResultingRevision = &revision
		flight.GoAroundConfirmation = &resolved
		flight.UpdatedAt = auth.ReceivedAt
		return commandChange(state, true, "reject_go_around", command.FlightID, map[string]any{"episode_id": command.EpisodeID, "reason": resolved.Reason, "detected_at": resolved.DetectedAt, "evidence_times": resolved.EvidenceTimes, "decision": "rejected", "actor": auth.Actor, "resulting_revision": revision})
	}, nil
}

func pendingGoAround(state aman.AirportState, flightID aman.FlightID, episodeID string) (int, *aman.GoAroundConfirmation, error) {
	index := flightIndex(state.Flights, flightID)
	if index < 0 {
		return -1, nil, domainNotFound(flightID)
	}
	pending := state.Flights[index].GoAroundConfirmation
	if pending == nil || pending.Status != aman.GoAroundConfirmationPending || pending.EpisodeID != episodeID {
		return -1, nil, &aman.DomainError{Class: aman.ErrorInvalidTransition, Message: "go-around confirmation is no longer pending"}
	}
	return index, pending, nil
}

func (s *Service) applyConfirmedGoAround(state aman.AirportState, index int, auth aman.CommandContext, metadata aman.CommandMetadata, detectedAt time.Time, action string) (sequence.CommandChange, error) {
	if state.Flights[index].Prediction == nil {
		return sequence.CommandChange{}, &aman.DomainError{Class: aman.ErrorInvalidTransition, Message: "go-around requires a current operational prediction"}
	}
	state.Flights = append([]aman.AMANFlight(nil), state.Flights...)
	flight := &state.Flights[index]
	recaptureTMA := flight.TMAEntry != nil && flight.TMAEntry.LastContainment == aman.TMAInside &&
		!flight.TMAEntry.LastObservedAt.After(auth.ReceivedAt) && auth.ReceivedAt.Sub(flight.TMAEntry.LastObservedAt) <= tmaSurveillanceFresh
	entry := flight.TMAEntry
	flight.FeederETA, flight.DerivedFeederETA = nil, nil
	expireActiveRouteFact(flight)
	updatedPrediction := *flight.Prediction
	updatedPrediction.OperationalTETA = detectedAt.Add(DefaultGoAroundDelay)
	updatedPrediction.OperationalReason = aman.OperationalReasonGoAround
	updatedPrediction.Publishable = true
	flight.Prediction = &updatedPrediction
	flight.State, flight.UpdatedAt = aman.StateGoAround, auth.ReceivedAt
	flight.Lifecycle = &aman.LifecycleState{EnteredAt: detectedAt, Reason: aman.LifecycleReasonGoAroundConfirmed, LastEventID: "go-around:" + metadata.CommandID, LastEventFingerprint: modelVersion, LastEventAt: detectedAt}
	extra := map[string]any{"decision": "confirmed", "actor": auth.Actor, "detected_at": detectedAt}
	if flight.GoAroundConfirmation != nil && (flight.GoAroundConfirmation.Status == aman.GoAroundConfirmationPending || action == "report_go_around" && flight.GoAroundConfirmation.Status == aman.GoAroundConfirmationRejected) {
		resolved := *flight.GoAroundConfirmation
		resolved.Status = aman.GoAroundConfirmationConfirmed
		resolved.DecidedAt, resolved.DecidedBy, resolved.DecisionCommandID = timePointer(auth.ReceivedAt), stringPointer(auth.Actor), stringPointer(metadata.CommandID)
		revision := state.Revision + 1
		resolved.ResultingRevision = &revision
		flight.GoAroundConfirmation = &resolved
		extra["episode_id"], extra["reason"], extra["evidence_times"], extra["resulting_revision"] = resolved.EpisodeID, resolved.Reason, resolved.EvidenceTimes, revision
	}
	if flight.GoAroundDetection == nil {
		flight.GoAroundDetection = &aman.GoAroundDetectionState{PolicyVersion: liveGoAroundPolicyVersion + "/" + s.deps.Terminal.ConfigVersion}
	}
	flight.GoAroundDetection.Armed = false
	flight.GoAroundDetection.ArmedAt, flight.GoAroundDetection.ArmedCorridorID = nil, ""
	flight.GoAroundDetection.ArmCount, flight.GoAroundDetection.ClimbCount, flight.GoAroundDetection.TrackAwayCount, flight.GoAroundDetection.RunwayExitCount = 0, 0, 0, 0
	flight.GoAroundDetection.ThresholdCrossed, flight.GoAroundDetection.AwaitingReset = false, true
	flight.GoAroundDetection.LastControllerCommandID = metadata.CommandID
	input := s.sequenceInput(state)
	decision, err := sequence.ApplyGoAround(input, sequence.GoAroundPolicy{Delay: DefaultGoAroundDelay, MaxCascade: len(input.Flights) + 1}, sequence.ApplyGoAroundCommand{Metadata: metadata, FlightID: flight.ID, DetectedAt: detectedAt})
	if err != nil {
		return sequence.CommandChange{}, err
	}
	state = s.applyDecision(state, decision)
	updated := &state.Flights[index]
	updated.FeederETA, updated.DerivedFeederETA, updated.TMAEntry = nil, nil, nil
	if entry != nil {
		recaptured := *entry
		recaptured.FreezeTriggered = false
		updated.TMAEntry = &recaptured
		if recaptureTMA {
			recaptured.FreezeTriggered = true
			updated.TMAEntry = &recaptured
			captureTMAFreeze(updated, auth.ReceivedAt)
		}
	}
	return s.commandChange(state, true, action, flight.ID, extra)
}

func (s *Service) sequenceMutation(action string, flightID aman.FlightID, at time.Time, apply func(sequence.Input) (sequence.Decision, error)) sequence.CommandMutation {
	return func(state aman.AirportState) (sequence.CommandChange, error) {
		decision, err := apply(s.sequenceInput(state))
		if err != nil {
			return sequence.CommandChange{}, err
		}
		state = s.applyDecision(state, decision)
		var promotions []sequence.VacancyPromotion
		if decision.Changed {
			promotions = s.resequence(&state, at)
		}
		change, err := s.commandChange(state, decision.Changed, action, flightID, nil)
		change.Audit = append(change.Audit, vacancyPromotionAuditEntries(promotions)...)
		return change, err
	}
}

func (s *Service) flightMutation(action string, flightID aman.FlightID, apply func(aman.AMANFlight) (aman.AMANFlight, bool, error)) sequence.CommandMutation {
	return s.flightMutationWithAudit(action, flightID, nil, apply)
}

func (s *Service) flightMutationWithAudit(action string, flightID aman.FlightID, extra map[string]any, apply func(aman.AMANFlight) (aman.AMANFlight, bool, error)) sequence.CommandMutation {
	return func(state aman.AirportState) (sequence.CommandChange, error) {
		index := flightIndex(state.Flights, flightID)
		if index < 0 {
			return sequence.CommandChange{}, domainNotFound(flightID)
		}
		updated, changed, err := apply(state.Flights[index])
		if err != nil {
			return sequence.CommandChange{}, err
		}
		state.Flights = append([]aman.AMANFlight(nil), state.Flights...)
		state.Flights[index] = updated
		var promotions []sequence.VacancyPromotion
		if changed {
			promotions = s.resequence(&state, updated.UpdatedAt)
		}
		change, err := s.commandChange(state, changed, action, flightID, extra)
		change.Audit = append(change.Audit, vacancyPromotionAuditEntries(promotions)...)
		return change, err
	}
}

func (s *Service) commandChange(state aman.AirportState, changed bool, action string, flightID aman.FlightID, extra map[string]any) (sequence.CommandChange, error) {
	change, err := commandChange(state, changed, action, flightID, extra)
	if err != nil || !changed {
		return change, err
	}
	change.QueueOffers = &sequence.QueueOfferCalculation{
		Input:  s.sequenceInput(state),
		Config: sequence.QueueOfferConfig{Validity: queueOfferValidity},
	}
	return change, nil
}

func (s *Service) applyDecision(state aman.AirportState, decision sequence.Decision) aman.AirportState {
	state.Flights = append([]aman.AMANFlight(nil), state.Flights...)
	inputFlights := make(map[aman.FlightID]sequence.Flight, len(decision.Input.Flights))
	for _, flight := range decision.Input.Flights {
		inputFlights[flight.ID] = flight
	}
	entries := make(map[aman.FlightID]sequence.CandidateEntry, len(decision.Candidate.Entries))
	for _, entry := range decision.Candidate.Entries {
		entries[entry.FlightID] = entry
	}
	for i := range state.Flights {
		if input, ok := inputFlights[state.Flights[i].ID]; ok {
			state.Flights[i].FreezeReason = input.FreezeReason
			state.Flights[i].FrozenAt = input.FrozenAt
			state.Flights[i].FrozenOperationalTETA = input.FrozenOperationalTETA
			state.Flights[i].FrozenSlot = input.CapturedSlot
			state.Flights[i].ManualOrder = input.ManualOrder
		}
		if entry, ok := entries[state.Flights[i].ID]; ok {
			state.Flights[i].Slot = &aman.Slot{Time: entry.Time, RunwayGroupID: entry.RunwayGroupID, Sequence: entry.Sequence, Revision: state.Revision, Reason: string(entry.Reason)}
			order := entry.Sequence
			state.Flights[i].Order = &order
		}
	}
	clearInvalidRunwayGapExceptions(&state)
	s.refreshHoldingPlans(&state)
	return state
}

func clearInvalidRunwayGapExceptions(state *aman.AirportState) {
	for index := range state.Flights {
		flight, exception := &state.Flights[index], state.Flights[index].RunwayGapException
		if exception == nil {
			continue
		}
		validSlot := flight.Slot != nil && flight.Slot.RunwayGroupID == exception.RunwayGroupID && flight.Slot.Time.Equal(exception.Opportunity)
		validGap := false
		for _, group := range state.RunwayGroups {
			for _, gap := range group.Gaps {
				if gap.ID == exception.GapID {
					validGap = group.ID == exception.RunwayGroupID && !exception.Opportunity.Before(gap.Start) && exception.Opportunity.Before(gap.End)
				}
			}
		}
		if !validSlot || !validGap {
			flight.RunwayGapException = nil
		}
	}
}

func commandChange(state aman.AirportState, changed bool, action string, flightID aman.FlightID, extra map[string]any) (sequence.CommandChange, error) {
	payload := map[string]any{"action": action, "changed": changed}
	if flightID != "" {
		payload["flight_id"] = flightID
	}
	for key, value := range extra {
		payload[key] = value
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return sequence.CommandChange{}, err
	}
	return sequence.CommandChange{State: state, Changed: changed, Outcome: encoded, Audit: []sequence.AuditEntry{{Category: "aman." + action, Payload: encoded}}}, nil
}

func flightIndex(flights []aman.AMANFlight, id aman.FlightID) int {
	for index := range flights {
		if flights[index].ID == id {
			return index
		}
	}
	return -1
}

func timePointer(value time.Time) *time.Time { return &value }

func domainNotFound(id aman.FlightID) error {
	return &aman.DomainError{Class: aman.ErrorNotFound, Message: fmt.Sprintf("AMAN flight %q was not found", id)}
}

var _ sequence.ActionMutations = (*Service)(nil)
