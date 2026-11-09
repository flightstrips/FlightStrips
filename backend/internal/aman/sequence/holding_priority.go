package sequence

import (
	"slices"

	"FlightStrips/internal/aman"
)

// Exchange existing automatic slots rather than retaining a late holding
// reservation while only pushing its inbound blocker further back. Each
// exchange advances one held aircraft past one inbound aircraft; held aircraft
// never cross each other, and the runway opportunities themselves do not move.
func prioritizeHeldAircraft(policy preparedPolicy, entries []allocatedEntry) []allocatedEntry {
	for i := 0; i+1 < len(entries); i++ {
		incoming, held := entries[i], entries[i+1]
		if !holdingExchangeEligible(incoming.flight) || !holdingExchangeEligible(held.flight) ||
			held.flight.ActiveHoldingSince == nil || incoming.flight.ActiveHoldingSince != nil ||
			incoming.flight.ArrivalQueueTime == nil ||
			!held.flight.ActiveHoldingSince.Before(*incoming.flight.ArrivalQueueTime) ||
			held.flight.PromotionNotBefore == nil || incoming.flight.PromotionNotBefore == nil {
			continue
		}
		held.time, incoming.time = incoming.time, held.time
		// A held aircraft's smoothed/frozen prediction may still reflect its old
		// delay. The current physical release bound establishes reachability.
		if held.time.Before(*held.flight.PromotionNotBefore) ||
			!automaticTargetReachable(policy, incoming) {
			continue
		}
		remaining := slices.Clone(entries)
		remaining[i], remaining[i+1] = held, incoming
		valid := true
		for index := i; index < len(remaining); index++ {
			candidate := remaining[index]
			// Delaying the inbound also tightens its followers' release bounds.
			// Check those reservations before accepting the exchange, including
			// frozen followers that operational repair cannot move afterwards.
			if index > i+1 && !policy.holdingPrecedes(incoming.flight.Flight, candidate.flight.Flight) {
				continue
			}
			others := slices.Delete(slices.Clone(remaining), index, index+1)
			if ok, _, _ := placementWithStableOrder(policy, others, candidate.flight, candidate.time, false); !ok {
				valid = false
				break
			}
		}
		if !valid {
			continue
		}
		entries[i], entries[i+1] = held, incoming
		noteOperation(&entries[i].flight, "holding_priority_exchange")
		noteOperation(&entries[i+1].flight, "holding_priority_exchange")
		// Revisit the preceding pair to advance through consecutive inbound
		// aircraft without crossing another held or protected reservation.
		i = max(-1, i-2)
	}
	return entries
}

func holdingExchangeEligible(flight preparedFlight) bool {
	return flight.HoldingPriorityExchange && flight.FreezeReason == aman.FreezeNone &&
		flight.ManualOrder == nil && flight.ReleaseRank == nil &&
		!flight.RebuildTMA &&
		(flight.State == aman.StateStable || flight.State == aman.StateUnstable || flight.State == aman.StateAirborne)
}
