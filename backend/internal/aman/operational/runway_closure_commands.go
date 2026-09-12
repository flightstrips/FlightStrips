package operational

import (
	"encoding/json"
	"sort"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/sequence"
)

func (s *Service) CreateRunwayClosure(auth aman.CommandContext, command aman.CreateRunwayClosureCommand) (sequence.CommandMutation, error) {
	if err := s.authorizeRunwayGap(auth); err != nil {
		return nil, err
	}
	return func(state aman.AirportState) (sequence.CommandChange, error) {
		interval, err := aman.NormalizeRunwayClosureInterval(command.Interval, state)
		if err != nil {
			return sequence.CommandChange{}, err
		}
		groupIndex := runwayGroupIndex(state.RunwayGroups, command.Interval.RunwayGroupID)
		if groupIndex < 0 {
			return sequence.CommandChange{}, &aman.DomainError{Class: aman.ErrorNotFound, Message: "AMAN runway group was not found"}
		}
		state.RunwayGroups = append([]aman.RunwayGroupPolicy(nil), state.RunwayGroups...)
		group := &state.RunwayGroups[groupIndex]
		group.Closures = append([]aman.RunwayClosure(nil), group.Closures...)
		priorIDs := make([]aman.RunwayClosureID, len(group.Closures))
		for index, closure := range group.Closures {
			priorIDs[index] = closure.ID
		}
		closure := aman.RunwayClosure{
			ID: aman.RunwayClosureID(command.Metadata.CommandID), Start: interval.Start(), End: interval.End(),
			Reason: command.Reason, CreatedAt: auth.ReceivedAt, CreatedBy: auth.Actor,
		}
		group.Closures = append(group.Closures, closure)
		sort.Slice(group.Closures, func(i, j int) bool { return closureLess(group.Closures[i], group.Closures[j]) })
		displacements, err := s.displaceFlightsFromRunwayClosure(&state, command.Interval.RunwayGroupID, closure, auth.ReceivedAt)
		if err != nil {
			return sequence.CommandChange{}, err
		}
		provenance := map[string]any{"kind": "absolute", "requested_start": command.Interval.Start}
		if command.Interval.AfterFlightID != nil {
			provenance = map[string]any{"kind": "after_aircraft", "anchor_flight_id": *command.Interval.AfterFlightID}
		}
		change, err := commandChange(state, true, "create_runway_closure", "", map[string]any{
			"airport": auth.Airport, "actor": auth.Actor, "role": auth.Role, "received_at": auth.ReceivedAt,
			"runway_group_id": command.Interval.RunwayGroupID, "closure_id": closure.ID, "reason": closure.Reason,
			"normalized_interval": map[string]any{"start": closure.Start, "end": closure.End}, "input_provenance": provenance,
			"creator": closure.CreatedBy, "prior_ids": priorIDs, "replaced_ids": []aman.RunwayClosureID{},
		})
		queueInput := s.sequenceInput(state)
		if len(queueInput.Flights) > 0 && len(queueInput.Policies) > 0 {
			change.QueueOffers = &sequence.QueueOfferCalculation{Input: queueInput, Config: sequence.QueueOfferConfig{Validity: queueOfferValidity}}
		}
		for _, displacement := range displacements {
			payload, marshalErr := json.Marshal(map[string]any{
				"action": "runway_closure_displacement", "closure_id": closure.ID, "runway_group_id": command.Interval.RunwayGroupID,
				"flight_id": displacement.FlightID, "previous_opportunity": displacement.Previous,
				"new_opportunity": displacement.New, "overridden_protection_reason": displacement.ProtectionReason,
				"reason": displacement.Reason,
			})
			if marshalErr != nil {
				return sequence.CommandChange{}, marshalErr
			}
			change.Audit = append(change.Audit, sequence.AuditEntry{Category: "aman.runway_closure_displacement", Payload: payload})
		}
		return change, err
	}, nil
}

type closureDisplacement struct {
	FlightID         aman.FlightID
	Previous         gapOpportunity
	New              *gapOpportunity
	ProtectionReason string
	Reason           string
}

func (s *Service) displaceFlightsFromRunwayClosure(state *aman.AirportState, groupID aman.RunwayGroupID, closure aman.RunwayClosure, at time.Time) ([]closureDisplacement, error) {
	input := s.sequenceInput(*state)
	affected := make(map[aman.FlightID]closureDisplacement)
	for _, flight := range state.Flights {
		if flight.SelectedRunwayGroup == nil || *flight.SelectedRunwayGroup != groupID || flight.Slot == nil ||
			flight.Slot.Time.Before(closure.Start) || (closure.End != nil && !flight.Slot.Time.Before(*closure.End)) || !flight.SequenceDisposition.Participates() {
			continue
		}
		affected[flight.ID] = closureDisplacement{FlightID: flight.ID,
			Previous:         gapOpportunity{Time: flight.Slot.Time, RunwayGroupID: groupID, Sequence: flight.Slot.Sequence},
			ProtectionReason: gapProtectionReason(flight)}
	}
	if len(affected) == 0 {
		return nil, nil
	}
	ordered := make([]aman.FlightID, 0, len(affected))
	for id := range affected {
		ordered = append(ordered, id)
	}
	sort.Slice(ordered, func(i, j int) bool {
		left, right := affected[ordered[i]].Previous, affected[ordered[j]].Previous
		if left.Sequence != right.Sequence {
			return left.Sequence < right.Sequence
		}
		return ordered[i] < ordered[j]
	})
	working := input
	working.Flights = nil
	byID := make(map[aman.FlightID]sequence.Flight, len(input.Flights))
	for _, flight := range input.Flights {
		byID[flight.ID] = flight
		if _, displaced := affected[flight.ID]; !displaced {
			working.Flights = append(working.Flights, flight)
		}
	}
	if len(byID) < len(affected) {
		return nil, &aman.DomainError{Class: aman.ErrorInvalidTransition, Message: "runway closure contains a flight that is not eligible for a complete sequence"}
	}
	state.Flights = append([]aman.AMANFlight(nil), state.Flights...)
	for _, id := range ordered {
		candidate := byID[id]
		candidate.FreezeReason, candidate.FrozenAt, candidate.FrozenOperationalTETA = aman.FreezeNone, nil, nil
		candidate.CapturedSlot, candidate.CurrentSlot, candidate.ManualOrder, candidate.ProtectCurrentSlot = nil, nil, nil, false
		selected := groupID
		entry, ok := closureCandidate(working, candidate, groupID)
		if !ok {
			for _, alternate := range state.ActiveRunwayGroups {
				index := flightIndex(state.Flights, id)
				if alternate == groupID || index < 0 || !s.runwayAssignmentCompatible(state.Flights[index], alternate) {
					continue
				}
				alternateEntry, available := closureCandidate(working, candidate, alternate)
				if available && (!ok || alternateEntry.Time.Before(entry.Time) || (alternateEntry.Time.Equal(entry.Time) && alternate < selected)) {
					selected, entry, ok = alternate, alternateEntry, true
				}
			}
		}
		displacement := affected[id]
		index := flightIndex(state.Flights, id)
		if !ok {
			state.Flights[index].SequenceDisposition = aman.SequenceDispositionDesequenced
			state.Flights[index].UpdatedAt = at
			displacement.Reason = "closure_no_capacity"
			affected[id] = displacement
			continue
		}
		if selected != groupID {
			assignFlightToRunwayGroup(&state.Flights[index], selected)
		}
		candidate.RunwayGroupID = selected
		working.Flights = append(working.Flights, candidate)
		newOpportunity := gapOpportunity{Time: entry.Time, RunwayGroupID: selected, Sequence: entry.Sequence}
		displacement.New, displacement.Reason = &newOpportunity, "closure_displacement"
		affected[id] = displacement
	}
	result, err := sequence.Generate(working)
	if err != nil || result.HasConflicts() {
		return nil, &aman.DomainError{Class: aman.ErrorInvalidTransition, Message: "runway closure cannot produce a legal atomic sequence"}
	}
	*state = s.applyDecision(*state, sequence.Decision{Input: working, Candidate: result, Changed: true})
	for _, id := range ordered {
		index := flightIndex(state.Flights, id)
		before := byID[id]
		if state.Flights[index].SequenceDisposition.Participates() {
			state.Flights[index].FreezeReason, state.Flights[index].FrozenAt = before.FreezeReason, before.FrozenAt
			state.Flights[index].FrozenOperationalTETA = before.FrozenOperationalTETA
			if before.FreezeReason != aman.FreezeNone {
				state.Flights[index].FrozenSlot = retargetSlot(state.Flights[index].Slot, *state.Flights[index].SelectedRunwayGroup)
			}
			if before.ManualOrder != nil && state.Flights[index].Order != nil {
				order := *state.Flights[index].Order
				state.Flights[index].ManualOrder = &order
			}
			state.Flights[index].UpdatedAt = at
		}
	}
	resultAudit := make([]closureDisplacement, len(ordered))
	for index, id := range ordered {
		resultAudit[index] = affected[id]
	}
	return resultAudit, nil
}

func closureCandidate(input sequence.Input, flight sequence.Flight, groupID aman.RunwayGroupID) (sequence.CandidateEntry, bool) {
	flight.RunwayGroupID = groupID
	trial := input
	trial.Flights = append(append([]sequence.Flight(nil), input.Flights...), flight)
	result, err := sequence.Generate(trial)
	if err != nil || result.HasConflicts() {
		return sequence.CandidateEntry{}, false
	}
	for _, entry := range result.Entries {
		if entry.FlightID == flight.ID {
			return entry, true
		}
	}
	return sequence.CandidateEntry{}, false
}

func (s *Service) RemoveRunwayClosure(auth aman.CommandContext, command aman.RemoveRunwayClosureCommand) (sequence.CommandMutation, error) {
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
		group.Closures = append([]aman.RunwayClosure(nil), group.Closures...)
		closureIndex := -1
		for index := range group.Closures {
			if group.Closures[index].ID == command.ClosureID {
				closureIndex = index
				break
			}
		}
		if closureIndex < 0 {
			return sequence.CommandChange{}, &aman.DomainError{Class: aman.ErrorNotFound, Message: "AMAN runway closure was not found"}
		}
		removed := group.Closures[closureIndex]
		group.Closures = append(group.Closures[:closureIndex], group.Closures[closureIndex+1:]...)
		promotions := s.resequence(&state, auth.ReceivedAt)
		change, err := s.commandChange(state, true, "remove_runway_closure", "", map[string]any{
			"airport": auth.Airport, "actor": auth.Actor, "role": auth.Role, "received_at": auth.ReceivedAt,
			"runway_group_id": command.RunwayGroupID, "closure_id": removed.ID,
			"normalized_interval": map[string]any{"start": removed.Start, "end": removed.End}, "creator": removed.CreatedBy,
			"removal_reason": command.Reason, "removed_ids": []aman.RunwayClosureID{removed.ID},
		})
		change.Audit = append(change.Audit, vacancyPromotionAuditEntries(promotions)...)
		return change, err
	}, nil
}

func closureLess(left, right aman.RunwayClosure) bool {
	if !left.Start.Equal(right.Start) {
		return left.Start.Before(right.Start)
	}
	if left.End == nil || right.End == nil {
		if left.End != nil {
			return true
		}
		if right.End != nil {
			return false
		}
	} else if !left.End.Equal(*right.End) {
		return left.End.Before(*right.End)
	}
	return left.ID < right.ID
}

func expireRunwayClosures(state *aman.AirportState, now time.Time) ([]aman.AuditRecord, error) {
	var audits []aman.AuditRecord
	for groupIndex := range state.RunwayGroups {
		group := &state.RunwayGroups[groupIndex]
		kept := make([]aman.RunwayClosure, 0, len(group.Closures))
		for _, closure := range group.Closures {
			if closure.End == nil || closure.End.After(now) {
				kept = append(kept, closure)
				continue
			}
			payload, err := json.Marshal(map[string]any{
				"action": "expire_runway_closure", "airport": state.Airport, "runway_group_id": group.ID,
				"closure_id": closure.ID, "normalized_interval": map[string]any{"start": closure.Start, "end": closure.End},
				"creator": closure.CreatedBy, "created_at": closure.CreatedAt, "expiry_reason": "interval_elapsed",
				"expired_at": now, "removed_ids": []aman.RunwayClosureID{closure.ID},
			})
			if err != nil {
				return nil, err
			}
			audits = append(audits, aman.AuditRecord{Airport: state.Airport, Category: "aman.expire_runway_closure", Payload: payload, RecordedAt: now})
		}
		if len(kept) != len(group.Closures) {
			group.Closures = kept
		}
	}
	return audits, nil
}
