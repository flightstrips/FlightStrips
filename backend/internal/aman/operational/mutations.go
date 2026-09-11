package operational

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/etareview"
	"FlightStrips/internal/aman/prediction"
	"FlightStrips/internal/aman/sequence"
)

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
	return s.commandChange(s.applyDecision(state, decision), true, action, flight.ID, extra)
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
		change, err := s.commandChange(state, changed, action, flightID, nil)
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
	s.refreshHoldingPlans(&state)
	return state
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
