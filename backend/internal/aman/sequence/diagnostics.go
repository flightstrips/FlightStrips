package sequence

import (
	"FlightStrips/internal/aman"
	"slices"
)

type Operation struct {
	Callsign aman.Callsign
	Kind     string
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
