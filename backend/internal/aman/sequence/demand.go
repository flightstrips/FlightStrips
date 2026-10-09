package sequence

import (
	"sort"
	"time"

	"FlightStrips/internal/aman"
)

const demandWindow = 15 * time.Minute

// prepareInput snapshots demand before any placement or promotion changes slots.
func prepareInput(input Input) (map[aman.RunwayGroupID]preparedPolicy, map[aman.RunwayGroupID][]preparedFlight, error) {
	families, err := prepareSTARFamilyPolicies(input.STARFamilyPolicies)
	if err != nil {
		return nil, nil, err
	}
	policies, err := preparePoliciesWithSTARFamilies(input.Policies, families)
	if err != nil {
		return nil, nil, err
	}
	flights, err := prepareFlights(input.Flights, policies)
	if err != nil {
		return nil, nil, err
	}
	for group, values := range flights {
		policy := policies[group]
		policy.activeHoldingFamilies = make(map[string]bool)
		for _, flight := range values {
			if flight.State == aman.StatePlanned || flight.State == aman.StateLanded || flight.State == aman.StateRemoved {
				continue
			}
			if flight.DemandArrivalAt != nil {
				policy.demandTimes = append(policy.demandTimes, *flight.DemandArrivalAt)
			}
			if flight.ActiveHoldingSince != nil && flight.STARFamily != "" {
				policy.activeHoldingFamilies[flight.STARFamily] = true
			}
		}
		sort.Slice(policy.demandTimes, func(i, j int) bool { return policy.demandTimes[i].Before(policy.demandTimes[j]) })
		policies[group] = policy
	}
	return policies, flights, nil
}

func (p preparedPolicy) demandBusy(at *time.Time, threshold uint32) bool {
	if at == nil {
		return false
	}
	start, end := at.Add(-demandWindow/2), at.Add(demandWindow/2)
	first := sort.Search(len(p.demandTimes), func(i int) bool { return !p.demandTimes[i].Before(start) })
	last := sort.Search(len(p.demandTimes), func(i int) bool { return !p.demandTimes[i].Before(end) })
	return uint64(last-first)*4 >= uint64(threshold)
}
