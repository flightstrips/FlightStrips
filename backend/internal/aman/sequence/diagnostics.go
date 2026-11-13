package sequence

import (
	"slices"
	"strings"
	"time"

	"FlightStrips/internal/aman"
)

type Operation struct {
	Callsign aman.Callsign
	Kind     string
}

// STAROrderSnapshot records the evidence available when a same-STAR pair is
// first placed together or changes committed relative order. A nil PreviousOrder
// means at least one flight had no reservation; nil progress is explicit evidence
// unavailability. This describes input and final order, not optimizer trial history.
type STAROrderSnapshot struct {
	RunwayGroup   aman.RunwayGroupID
	STARFamily    string
	First         STAROrderFlight
	Second        STAROrderFlight
	PreviousOrder *bool
	CurrentOrder  bool
	Comparable    bool
	FirstPrecedes bool
}

type STAROrderFlight struct {
	Callsign           aman.Callsign
	State              aman.FlightState
	OperationalTETA    time.Time
	WakeCategory       WakeCategory
	CurrentSlot        *aman.Slot
	Progress           *STARProgress
	FreezeReason       aman.FreezeReason
	ManualOrder        *int
	ManualPosition     *aman.ManualSequencePosition
	HoldingPriority    bool
	ProtectCurrentSlot bool
}

func STAROrderDiagnostics(input Input, result Result) []STAROrderSnapshot {
	_, groups, err := prepareInput(input)
	if err != nil {
		return nil
	}
	positions := make(map[aman.Callsign]int, len(result.Entries))
	for _, entry := range result.Entries {
		positions[entry.Callsign] = entry.Sequence
	}
	var snapshots []STAROrderSnapshot
	for group, flights := range groups {
		slices.SortFunc(flights, func(a, b preparedFlight) int { return strings.Compare(string(a.Callsign), string(b.Callsign)) })
		for i, a := range flights {
			for _, b := range flights[i+1:] {
				aPosition, aFound := positions[a.Callsign]
				bPosition, bFound := positions[b.Callsign]
				if !aFound || !bFound || !sameSelectedSTAR(a, b) {
					continue
				}
				current := aPosition < bPosition
				var previous *bool
				if a.CurrentSlot != nil && b.CurrentSlot != nil {
					order := a.CurrentSlot.Time.Before(b.CurrentSlot.Time) ||
						(a.CurrentSlot.Time.Equal(b.CurrentSlot.Time) && a.CurrentSlot.Sequence < b.CurrentSlot.Sequence)
					previous = &order
					if order == current {
						continue
					}
				}
				precedes, comparable := starPrecedes(a, b)
				snapshots = append(snapshots, STAROrderSnapshot{RunwayGroup: group, STARFamily: a.SelectedSTARFamily,
					First: starOrderFlight(a), Second: starOrderFlight(b), PreviousOrder: previous, CurrentOrder: current,
					Comparable: comparable, FirstPrecedes: precedes})
			}
		}
	}
	// Map iteration must not change persisted diagnostic payloads.
	slices.SortFunc(snapshots, func(a, b STAROrderSnapshot) int {
		if order := strings.Compare(string(a.First.Callsign), string(b.First.Callsign)); order != 0 {
			return order
		}
		return strings.Compare(string(a.Second.Callsign), string(b.Second.Callsign))
	})
	return snapshots
}

func starOrderFlight(f preparedFlight) STAROrderFlight {
	return STAROrderFlight{Callsign: f.Callsign, State: f.State, OperationalTETA: f.OperationalTETA,
		WakeCategory: f.WakeCategory,
		CurrentSlot:  cloneSlot(f.CurrentSlot), Progress: cloneSTARProgress(f.STARProgress), FreezeReason: f.FreezeReason,
		ManualOrder: f.ManualOrder, ManualPosition: f.ManualPosition, HoldingPriority: hasHoldingPriority(f), ProtectCurrentSlot: f.ProtectCurrentSlot}
}

func cloneSTARProgress(progress *STARProgress) *STARProgress {
	if progress == nil {
		return nil
	}
	copy := *progress
	copy.ETA = cloneTime(progress.ETA)
	copy.RemainingFixes = slices.Clone(progress.RemainingFixes)
	return &copy
}

func noteOperation(flight *preparedFlight, kind string) {
	if !slices.Contains(flight.Operations, kind) {
		flight.Operations = append(slices.Clone(flight.Operations), kind)
	}
}

type SpacingSnapshot struct {
	Callsign    aman.Callsign
	RunwayGroup aman.RunwayGroupID
	STARFamily  string
	DemandCount int
	Threshold   uint32
	Active      bool
}

// SpacingDiagnostics uses precisely the demand and family policy seen by the
// allocator. It never derives demand from assigned slots.
func SpacingDiagnostics(input Input) []SpacingSnapshot {
	policies, flights, err := prepareInput(input)
	if err != nil {
		return nil
	}
	var result []SpacingSnapshot
	for group, values := range flights {
		policy := policies[group]
		for _, flight := range values {
			spacing, configured := policy.starFamilies.spacingFor(flight.STARFamily, policy.SameSTARSpacing)
			count := 0
			if flight.DemandArrivalAt != nil {
				start, end := flight.DemandArrivalAt.Add(-demandWindow/2), flight.DemandArrivalAt.Add(demandWindow/2)
				for _, at := range policy.demandTimes {
					if !at.Before(start) && at.Before(end) {
						count++
					}
				}
			}
			allocationAt := flight.OperationalTETA
			if flight.CurrentSlot != nil {
				allocationAt = flight.CurrentSlot.Time
			}
			active := configured && flight.STARFamily != "" && spacing.Enabled && policy.rateAt(allocationAt) > 0 && (policy.activeHoldingFamilies[flight.STARFamily] || (flight.DemandArrivalAt != nil && uint64(count)*4 >= uint64(spacing.ActivationRatePerHour)))
			result = append(result, SpacingSnapshot{Callsign: flight.Callsign, RunwayGroup: group, STARFamily: flight.STARFamily, DemandCount: count, Threshold: spacing.ActivationRatePerHour, Active: active})
		}
	}
	slices.SortFunc(result, func(a, b SpacingSnapshot) int {
		if a.Callsign < b.Callsign {
			return -1
		}
		if a.Callsign > b.Callsign {
			return 1
		}
		return 0
	})
	return result
}
