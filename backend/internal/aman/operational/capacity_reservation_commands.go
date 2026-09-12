package operational

import (
	"encoding/json"
	"sort"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/sequence"
)

func (s *Service) CreateCapacityReservation(auth aman.CommandContext, command aman.CreateCapacityReservationCommand) (sequence.CommandMutation, error) {
	if err := s.authorizeRunwayGap(auth); err != nil {
		return nil, err
	}
	return func(state aman.AirportState) (sequence.CommandChange, error) {
		groupIndex := runwayGroupIndex(state.RunwayGroups, command.RunwayGroupID)
		if groupIndex < 0 {
			return sequence.CommandChange{}, &aman.DomainError{Class: aman.ErrorNotFound, Message: "AMAN runway group was not found"}
		}
		group := state.RunwayGroups[groupIndex]
		anchor := command.AfterFlightID
		interval, err := aman.NormalizeRunwayClosureInterval(aman.RunwayClosureIntervalInput{
			RunwayGroupID: command.RunwayGroupID, AfterFlightID: &anchor,
		}, state)
		if err != nil {
			return sequence.CommandChange{}, err
		}
		if group.ActiveRatePerHour == 0 {
			return sequence.CommandChange{}, &aman.DomainError{Class: aman.ErrorInvalidTransition, Message: "capacity reservation requires an active runway rate"}
		}
		start := interval.Start()
		end := start.Add(rateOpportunityDuration(group.ActiveRatePerHour))
		for _, existing := range group.CapacityReservations {
			if start.Before(existing.End) && existing.Start.Before(end) {
				return sequence.CommandChange{}, &aman.DomainError{Class: aman.ErrorInvalidTransition, Message: "runway opportunity is already reserved"}
			}
		}
		for _, gap := range group.Gaps {
			if start.Before(gap.End) && gap.Start.Before(end) {
				return sequence.CommandChange{}, &aman.DomainError{Class: aman.ErrorInvalidTransition, Message: "runway opportunity is unavailable"}
			}
		}
		for _, closure := range group.Closures {
			if (closure.End == nil || start.Before(*closure.End)) && closure.Start.Before(end) {
				return sequence.CommandChange{}, &aman.DomainError{Class: aman.ErrorInvalidTransition, Message: "runway opportunity is unavailable"}
			}
		}
		reservation, err := aman.NewRunwayCapacityReservation(command.Metadata.CommandID, start, end, command.Label, auth.ReceivedAt, auth.Actor)
		if err != nil {
			return sequence.CommandChange{}, err
		}
		state.RunwayGroups = append([]aman.RunwayGroupPolicy(nil), state.RunwayGroups...)
		reservations := append([]aman.RunwayCapacityReservation(nil), group.CapacityReservations...)
		reservations = append(reservations, reservation)
		sort.Slice(reservations, func(i, j int) bool {
			if !reservations[i].Start.Equal(reservations[j].Start) {
				return reservations[i].Start.Before(reservations[j].Start)
			}
			return reservations[i].ID < reservations[j].ID
		})
		state.RunwayGroups[groupIndex].CapacityReservations = reservations
		displacements, err := s.displaceFlightsFromRunwayGap(&state, command.RunwayGroupID, aman.RunwayGap{Start: start, End: end})
		if err != nil {
			return sequence.CommandChange{}, err
		}
		change, err := commandChange(state, true, "create_capacity_reservation", "", map[string]any{
			"airport": auth.Airport, "actor": auth.Actor, "role": auth.Role, "received_at": auth.ReceivedAt,
			"command_id": command.Metadata.CommandID, "expected_revision": command.Metadata.ExpectedRevision,
			"runway_group_id": command.RunwayGroupID, "reservation_id": reservation.ID, "label": reservation.Label,
			"reason": command.Reason, "anchor_flight_id": command.AfterFlightID,
			"normalized_interval": gapIntervalAudit(start, end), "accepted_rate_per_hour": group.ActiveRatePerHour,
			"creator": reservation.CreatedBy, "created_at": reservation.CreatedAt,
		})
		if err != nil {
			return sequence.CommandChange{}, err
		}
		for _, displacement := range displacements {
			payload, marshalErr := json.Marshal(map[string]any{
				"action": "capacity_reservation_displacement", "reservation_id": reservation.ID,
				"command_id": command.Metadata.CommandID, "actor": auth.Actor, "role": auth.Role, "received_at": auth.ReceivedAt,
				"runway_group_id": command.RunwayGroupID, "flight_id": displacement.FlightID,
				"previous_opportunity": displacement.Previous, "new_opportunity": displacement.New,
				"overridden_protection_reason": displacement.ProtectionReason,
			})
			if marshalErr != nil {
				return sequence.CommandChange{}, marshalErr
			}
			change.Audit = append(change.Audit, sequence.AuditEntry{Category: "aman.capacity_reservation_displacement", Payload: payload})
		}
		return change, nil
	}, nil
}

func (s *Service) RemoveCapacityReservation(auth aman.CommandContext, command aman.RemoveCapacityReservationCommand) (sequence.CommandMutation, error) {
	if err := s.authorizeRunwayGap(auth); err != nil {
		return nil, err
	}
	return func(state aman.AirportState) (sequence.CommandChange, error) {
		groupIndex := runwayGroupIndex(state.RunwayGroups, command.RunwayGroupID)
		if groupIndex < 0 {
			return sequence.CommandChange{}, &aman.DomainError{Class: aman.ErrorNotFound, Message: "AMAN runway group was not found"}
		}
		state.RunwayGroups = append([]aman.RunwayGroupPolicy(nil), state.RunwayGroups...)
		reservations := append([]aman.RunwayCapacityReservation(nil), state.RunwayGroups[groupIndex].CapacityReservations...)
		index := -1
		for i := range reservations {
			if reservations[i].ID == command.ReservationID {
				index = i
				break
			}
		}
		if index < 0 {
			return sequence.CommandChange{}, &aman.DomainError{Class: aman.ErrorNotFound, Message: "AMAN capacity reservation was not found"}
		}
		removed := reservations[index]
		state.RunwayGroups[groupIndex].CapacityReservations = append(reservations[:index], reservations[index+1:]...)
		promotions := s.resequence(&state, auth.ReceivedAt)
		change, err := s.commandChange(state, true, "remove_capacity_reservation", "", map[string]any{
			"airport": auth.Airport, "actor": auth.Actor, "role": auth.Role, "received_at": auth.ReceivedAt,
			"command_id": command.Metadata.CommandID, "expected_revision": command.Metadata.ExpectedRevision,
			"runway_group_id": command.RunwayGroupID, "reservation_id": removed.ID, "label": removed.Label,
			"normalized_interval": gapIntervalAudit(removed.Start, removed.End), "creator": removed.CreatedBy,
			"created_at": removed.CreatedAt, "removal_reason": command.Reason,
		})
		change.Audit = append(change.Audit, vacancyPromotionAuditEntries(promotions)...)
		return change, err
	}, nil
}

func rateOpportunityDuration(rate uint32) time.Duration {
	nanoseconds := uint64(time.Hour)
	return time.Duration((nanoseconds + uint64(rate) - 1) / uint64(rate))
}
