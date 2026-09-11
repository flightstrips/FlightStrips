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
		provenance := map[string]any{"kind": "absolute", "requested_start": command.Interval.Start}
		if command.Interval.AfterFlightID != nil {
			provenance = map[string]any{"kind": "after_aircraft", "anchor_flight_id": *command.Interval.AfterFlightID}
		}
		return commandChange(state, true, "create_runway_closure", "", map[string]any{
			"airport": auth.Airport, "actor": auth.Actor, "role": auth.Role, "received_at": auth.ReceivedAt,
			"runway_group_id": command.Interval.RunwayGroupID, "closure_id": closure.ID, "reason": closure.Reason,
			"normalized_interval": map[string]any{"start": closure.Start, "end": closure.End}, "input_provenance": provenance,
			"creator": closure.CreatedBy, "prior_ids": priorIDs, "replaced_ids": []aman.RunwayClosureID{},
		})
	}, nil
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
		return commandChange(state, true, "remove_runway_closure", "", map[string]any{
			"airport": auth.Airport, "actor": auth.Actor, "role": auth.Role, "received_at": auth.ReceivedAt,
			"runway_group_id": command.RunwayGroupID, "closure_id": removed.ID,
			"normalized_interval": map[string]any{"start": removed.Start, "end": removed.End}, "creator": removed.CreatedBy,
			"removal_reason": command.Reason, "removed_ids": []aman.RunwayClosureID{removed.ID},
		})
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
