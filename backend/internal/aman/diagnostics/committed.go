package diagnostics

import (
	"FlightStrips/internal/aman"
	"FlightStrips/internal/metrics"
	"context"
	"encoding/json"
	"log/slog"
	"slices"
	"time"
)

// LogCommittedChanges runs only after persistence succeeds. Calculation
// attempts and optimistic retries must never be reported as committed changes.
func LogCommittedChanges(ctx context.Context, before, after aman.AirportState, audits []aman.AuditRecord, commandID string) {
	operations := map[string][]string{}
	for _, record := range audits {
		if record.Category != "aman.sequence_operation" && record.Category != "aman.queue_promotion" && record.Category != "aman.spacing_evaluated" {
			continue
		}
		var detail struct {
			Callsign  string `json:"callsign"`
			Operation string `json:"operation"`
		}
		_ = json.Unmarshal(record.Payload, &detail)
		if record.Category == "aman.queue_promotion" {
			detail.Operation = "vacancy_promotion"
		}
		if detail.Callsign != "" && detail.Operation != "" && !slices.Contains(operations[detail.Callsign], detail.Operation) {
			operations[detail.Callsign] = append(operations[detail.Callsign], detail.Operation)
		}
		slog.InfoContext(ctx, "AMAN sequencing operation committed", "session", after.SessionID, "airport", after.Airport,
			"revision", after.Revision, "command_id", commandID, "callsign", detail.Callsign, "category", record.Category, "details", string(record.Payload))
	}
	previous := map[string]aman.AMANFlight{}
	for _, f := range before.Flights {
		previous[f.Callsign] = f
	}
	oldNeighbours, newNeighbours := sequenceNeighbours(before), sequenceNeighbours(after)
	for _, f := range after.Flights {
		old, exists := previous[f.Callsign]
		if !exists {
			continue
		}
		oldGain, oldAvailable := diagnosticGuidance(old)
		newGain, newAvailable := diagnosticGuidance(f)
		var movement time.Duration
		if old.Slot != nil && f.Slot != nil {
			movement = f.Slot.Time.Sub(old.Slot.Time)
		}
		orderChanged := oldNeighbours[f.Callsign] != newNeighbours[f.Callsign]
		guidanceChanged := oldAvailable != newAvailable || (oldAvailable && newAvailable && (absSeconds(newGain-oldGain) >= 60 || oldGain > 30 && newGain < -30 || oldGain < -30 && newGain > 30))
		if absSeconds(int64(movement/time.Second)) < 30 && !orderChanged && !guidanceChanged {
			continue
		}
		causes := slices.Clone(operations[f.Callsign])
		if commandID != "" {
			causes = append(causes, "controller_command")
		}
		metrics.RecordAMANGuidanceChange(ctx, movement, guidanceChanged, orderChanged)
		oldPhysical, _ := aman.SmoothedPhysicalPrediction(old)
		newPhysical, _ := aman.SmoothedPhysicalPrediction(f)
		slog.InfoContext(ctx, "AMAN guidance change committed", "session", after.SessionID, "airport", after.Airport, "callsign", f.Callsign,
			"revision", after.Revision, "command_id", commandID, "operations", causes,
			"previous_raw_eta", diagnosticRawETA(old), "raw_eta", diagnosticRawETA(f),
			"previous_smoothed_eta", oldPhysical, "smoothed_eta", newPhysical,
			"previous_operational_eta", diagnosticOperationalETA(old), "operational_eta", diagnosticOperationalETA(f),
			"previous_target", diagnosticSlotTime(old), "target", diagnosticSlotTime(f), "target_movement_seconds", movement.Seconds(),
			"previous_guidance_seconds", oldGain, "guidance_seconds", newGain, "previous_available", oldAvailable, "available", newAvailable,
			"previous_neighbours", oldNeighbours[f.Callsign], "neighbours", newNeighbours[f.Callsign], "order_changed", orderChanged)
	}
}

func diagnosticGuidance(f aman.AMANFlight) (int64, bool) {
	p, ok := aman.GainLossPrediction(f)
	if !ok {
		return 0, false
	}
	seconds, err := aman.GainLossGuidance(p, f.Slot.Time)
	return seconds, err == nil
}
func absSeconds(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
func diagnosticRawETA(f aman.AMANFlight) time.Time {
	if f.Prediction == nil {
		return time.Time{}
	}
	return f.Prediction.RawTETA
}
func diagnosticOperationalETA(f aman.AMANFlight) time.Time {
	if f.Prediction == nil {
		return time.Time{}
	}
	return f.Prediction.OperationalTETA
}
func diagnosticSlotTime(f aman.AMANFlight) time.Time {
	if f.Slot == nil {
		return time.Time{}
	}
	return f.Slot.Time
}

func sequenceNeighbours(state aman.AirportState) map[string][2]string {
	flights := []aman.AMANFlight{}
	for _, f := range state.Flights {
		if f.Slot != nil && f.State != aman.StateRemoved && f.State != aman.StateLanded {
			flights = append(flights, f)
		}
	}
	slices.SortFunc(flights, func(a, b aman.AMANFlight) int {
		if a.Slot.RunwayGroupID < b.Slot.RunwayGroupID {
			return -1
		}
		if a.Slot.RunwayGroupID > b.Slot.RunwayGroupID {
			return 1
		}
		if c := a.Slot.Time.Compare(b.Slot.Time); c != 0 {
			return c
		}
		if a.Callsign < b.Callsign {
			return -1
		}
		if a.Callsign > b.Callsign {
			return 1
		}
		return 0
	})
	result := map[string][2]string{}
	for i, f := range flights {
		var pair [2]string
		if i > 0 && flights[i-1].Slot.RunwayGroupID == f.Slot.RunwayGroupID {
			pair[0] = flights[i-1].Callsign
		}
		if i+1 < len(flights) && flights[i+1].Slot.RunwayGroupID == f.Slot.RunwayGroupID {
			pair[1] = flights[i+1].Callsign
		}
		result[f.Callsign] = pair
	}
	return result
}
