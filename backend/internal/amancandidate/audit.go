package amancandidate

import (
	"FlightStrips/internal/aman"
	"FlightStrips/internal/cluster"
	pb "FlightStrips/pkg/events/cluster"
	"encoding/json"
	"fmt"
	"time"
)

// The legacy policy's in-process audit payload is decoded at this boundary
// into a closed typed fact. No payload bytes, JSON strings or generic objects
// enter the candidate event or Object Store. Unknown categories fail closed.
func convertAudit(record aman.AuditRecord, commandID string, index int, actor *pb.Actor) (*pb.AmanAudit, error) {
	id, err := cluster.AmanIntentID(commandID, fmt.Sprintf("audit/%d/%s", index, record.Category))
	if err != nil {
		return nil, err
	}
	result := &pb.AmanAudit{Id: id, AirportRevision: uint64(record.Revision), CreatedAt: timestamp(record.RecordedAt), Actor: actor}
	switch record.Category {
	case "aman.superstable_applied", "aman.tma_freeze_applied":
		var v struct {
			Callsign        string               `json:"callsign"`
			State           string               `json:"state"`
			Reason          string               `json:"freeze_reason"`
			FeederETA       *aman.FeederETAState `json:"feeder_eta"`
			FrozenAt        *time.Time           `json:"frozen_at"`
			OperationalTETA *time.Time           `json:"frozen_operational_teta"`
			Slot            *aman.Slot           `json:"frozen_slot"`
			TMAEntry        *aman.TMAEntryState  `json:"tma_entry"`
		}
		if err = json.Unmarshal(record.Payload, &v); err != nil {
			return nil, err
		}
		result.Fact = &pb.AmanAudit_Freeze{Freeze: &pb.AmanFreezeAudit{Callsign: v.Callsign, State: v.State, Reason: v.Reason, FeederEta: encodeAmanFeederEta(v.FeederETA), FrozenAt: optionalTimestamp(v.FrozenAt), OperationalTeta: optionalTimestamp(v.OperationalTETA), Slot: encodeAmanSlot(v.Slot), TmaEntry: encodeAmanTmaEntry(v.TMAEntry)}}
	case "aman.go_around_confirmation_pending":
		var v struct {
			Callsign      string      `json:"callsign"`
			EpisodeID     string      `json:"episode_id"`
			Reason        string      `json:"reason"`
			DetectedAt    time.Time   `json:"detected_at"`
			EvidenceTimes []time.Time `json:"evidence_times"`
		}
		if err = json.Unmarshal(record.Payload, &v); err != nil {
			return nil, err
		}
		value := &pb.AmanGoAroundAudit{Callsign: v.Callsign, EpisodeId: v.EpisodeID, Decision: "pending", Reason: v.Reason, DetectedAt: timestamp(v.DetectedAt)}
		for _, t := range v.EvidenceTimes {
			value.EvidenceTimes = append(value.EvidenceTimes, timestamp(t))
		}
		result.Fact = &pb.AmanAudit_GoAround{GoAround: value}
	case "aman.queue_promotion":
		var v struct {
			Callsign     string    `json:"callsign"`
			RunwayGroup  string    `json:"runway_group_id"`
			FromSequence uint32    `json:"from_sequence"`
			FromTime     time.Time `json:"from_time"`
			ToSequence   uint32    `json:"to_sequence"`
			ToTime       time.Time `json:"to_time"`
		}
		if err = json.Unmarshal(record.Payload, &v); err != nil {
			return nil, err
		}
		result.Fact = &pb.AmanAudit_Sequence{Sequence: &pb.AmanSequenceAudit{Callsign: v.Callsign, RunwayGroupId: v.RunwayGroup, Before: &pb.AmanSlot{Time: timestamp(v.FromTime), Sequence: v.FromSequence, RunwayGroupId: v.RunwayGroup}, After: &pb.AmanSlot{Time: timestamp(v.ToTime), Sequence: v.ToSequence, RunwayGroupId: v.RunwayGroup, Revision: uint64(record.Revision)}, Reason: "queue_promotion"}}
	case "aman.expire_runway_closure":
		var v struct {
			RunwayGroup  string    `json:"runway_group_id"`
			ID           string    `json:"closure_id"`
			Creator      string    `json:"creator"`
			CreatedAt    time.Time `json:"created_at"`
			ExpiryReason string    `json:"expiry_reason"`
			ExpiredAt    time.Time `json:"expired_at"`
			RemovedIDs   []string  `json:"removed_ids"`
			Interval     struct {
				Start time.Time  `json:"start"`
				End   *time.Time `json:"end"`
			} `json:"normalized_interval"`
		}
		if err = json.Unmarshal(record.Payload, &v); err != nil {
			return nil, err
		}
		result.Fact = &pb.AmanAudit_Capacity{Capacity: &pb.AmanCapacityAudit{RunwayGroupId: v.RunwayGroup, ObjectId: v.ID, Kind: pb.AmanCapacityAudit_CLOSURE, Action: "expire_runway_closure", Start: timestamp(v.Interval.Start), End: optionalTimestamp(v.Interval.End), Creator: v.Creator, CreatedAt: timestamp(v.CreatedAt), ExpiryReason: v.ExpiryReason, ExpiredAt: timestamp(v.ExpiredAt), RemovedIds: v.RemovedIDs}}
	case "aman.runway_gap_displacement", "aman.runway_closure_displacement", "aman.capacity_reservation_displacement":
		var v struct {
			Callsign string `json:"callsign"`
			Group    string `json:"runway_group_id"`
			Reason   string `json:"overridden_protection_reason"`
			Before   struct {
				Time     time.Time `json:"time"`
				Group    string    `json:"runway_group_id"`
				Sequence uint32    `json:"sequence"`
			} `json:"previous_opportunity"`
			After struct {
				Time     time.Time `json:"time"`
				Group    string    `json:"runway_group_id"`
				Sequence uint32    `json:"sequence"`
			} `json:"new_opportunity"`
		}
		if err = json.Unmarshal(record.Payload, &v); err != nil {
			return nil, err
		}
		result.Fact = &pb.AmanAudit_Sequence{Sequence: &pb.AmanSequenceAudit{Callsign: v.Callsign, RunwayGroupId: v.Group, Reason: record.Category + "/" + v.Reason, Before: &pb.AmanSlot{Time: timestamp(v.Before.Time), RunwayGroupId: v.Before.Group, Sequence: v.Before.Sequence}, After: &pb.AmanSlot{Time: timestamp(v.After.Time), RunwayGroupId: v.After.Group, Sequence: v.After.Sequence, Revision: uint64(record.Revision)}}}
	case "aman.move_flight", "aman.place_flight_at_time", "aman.lock_flight", "aman.unlock_flight", "aman.desequence_flight", "aman.resume_flight", "aman.remove_flight", "aman.accept_teta", "aman.keep_fpl_eta", "aman.reset_teta_override", "aman.set_rate", "aman.select_runway_group", "aman.set_active_runway_groups", "aman.set_manual_eta", "aman.set_manual_feeder_eta", "aman.reset_manual_feeder_eta", "aman.recompute_flight", "aman.change_runway", "aman.report_go_around", "aman.confirm_go_around", "aman.reject_go_around", "aman.create_runway_gap", "aman.remove_runway_gap", "aman.create_runway_closure", "aman.remove_runway_closure", "aman.create_capacity_reservation", "aman.remove_capacity_reservation":
		var value struct {
			Action  string `json:"action"`
			Changed bool   `json:"changed"`
		}
		if err = json.Unmarshal(record.Payload, &value); err != nil {
			return nil, err
		}
		outcome := "unchanged"
		if value.Changed {
			outcome = "changed"
		}
		result.Fact = &pb.AmanAudit_Command{Command: &pb.AmanCommandAudit{CommandId: commandID, CommandKind: value.Action, Outcome: outcome}}
	default:
		return nil, fmt.Errorf("unmapped operational audit category %q", record.Category)
	}
	return result, nil
}
