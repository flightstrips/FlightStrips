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
		return s.commandChange(applyDecision(state, decision), decision.Changed, "move_flight", command.FlightID, nil)
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
		state = applyDecision(state, decision)
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
		candidate = applyDecision(candidate, sequence.Decision{Input: input, Candidate: generated, Changed: true})
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
		group := selected
		flight.SelectedRunwayGroup = &group
		flight.SelectedHolding = nil
		flight.HoldingStack = nil
		flight.ActiveRouteKey = nil
		flight.ActiveRouteDatasetID = nil
		flight.RouteProgress = nil
		flight.Slot = nil
		flight.Order = nil
		flight.ManualOrder = nil
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
		if state.Flights[index].Prediction == nil {
			return sequence.CommandChange{}, &aman.DomainError{Class: aman.ErrorInvalidTransition, Message: "go-around requires a current operational prediction"}
		}
		state.Flights = append([]aman.AMANFlight(nil), state.Flights...)
		flight := &state.Flights[index]
		expireActiveRouteFact(flight)
		target := command.DetectedAt.Add(DefaultGoAroundDelay)
		updatedPrediction := *flight.Prediction
		updatedPrediction.OperationalTETA = target
		updatedPrediction.OperationalReason = aman.OperationalReasonGoAround
		updatedPrediction.Publishable = true
		flight.Prediction = &updatedPrediction
		flight.State = aman.StateGoAround
		flight.UpdatedAt = auth.ReceivedAt
		flight.Lifecycle = &aman.LifecycleState{
			EnteredAt: command.DetectedAt, Reason: aman.LifecycleReasonGoAroundConfirmed,
			LastEventID: "go-around:" + command.Metadata.CommandID, LastEventFingerprint: modelVersion, LastEventAt: command.DetectedAt,
		}
		input := s.sequenceInput(state)
		decision, err := sequence.ApplyGoAround(input, sequence.GoAroundPolicy{Delay: DefaultGoAroundDelay, MaxCascade: len(input.Flights) + 1}, sequence.ApplyGoAroundCommand{Metadata: command.Metadata, FlightID: command.FlightID, DetectedAt: command.DetectedAt})
		if err != nil {
			return sequence.CommandChange{}, err
		}
		return s.commandChange(applyDecision(state, decision), true, "report_go_around", command.FlightID, nil)
	}, nil
}

func (s *Service) sequenceMutation(action string, flightID aman.FlightID, at time.Time, apply func(sequence.Input) (sequence.Decision, error)) sequence.CommandMutation {
	return func(state aman.AirportState) (sequence.CommandChange, error) {
		decision, err := apply(s.sequenceInput(state))
		if err != nil {
			return sequence.CommandChange{}, err
		}
		state = applyDecision(state, decision)
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

func applyDecision(state aman.AirportState, decision sequence.Decision) aman.AirportState {
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
	refreshHoldingPlans(&state)
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

func domainNotFound(id aman.FlightID) error {
	return &aman.DomainError{Class: aman.ErrorNotFound, Message: fmt.Sprintf("AMAN flight %q was not found", id)}
}

var _ sequence.ActionMutations = (*Service)(nil)
