package sequence

import (
	"slices"
	"time"

	"FlightStrips/internal/aman"
)

// Fill capacity left between two arrivals by same-STAR spacing. An aircraft
// from another stream may use that capacity without moving either reservation.
// This does not authorize a general reorder across controller constraints.
func fillSTARSpacingGaps(policy preparedPolicy, entries []allocatedEntry, at time.Time) []allocatedEntry {
	if !policy.AllowCrossFeederOvertake {
		return entries
	}
	entries = slices.Clone(entries)
	moved := make(map[aman.Callsign]bool)
	for len(moved) < len(entries) {
		filled := false
		for gap := 0; gap+1 < len(entries) && !filled; gap++ {
			leading := entries[gap]
			if leading.flight.STARFamily == "" {
				continue
			}
			// Cross-stream insertions do not consume the whole STAR interval.
			// Find its next same-family reservation even when earlier fillers
			// already separate the anchors, including on later reconciliations.
			trailingIndex := gap + 1
			for trailingIndex < len(entries) && entries[trailingIndex].flight.STARFamily != leading.flight.STARFamily {
				trailingIndex++
			}
			if trailingIndex == len(entries) {
				continue
			}
			trailing := entries[trailingIndex]
			if sameSTARGap(policy, leading.flight, trailing.flight, trailing.time) <= policy.intervalAt(trailing.time) {
				continue
			}
			for index := trailingIndex + 1; index < len(entries); index++ {
				target := entries[index]
				flight := target.flight
				if moved[flight.Callsign] || !gapFillEligible(flight) || flight.STARFamily == leading.flight.STARFamily {
					continue
				}
				lower := promotionLowerBound(policy, flight, at)
				if flight.SlotNotBefore != nil && flight.SlotNotBefore.After(lower) {
					lower = *flight.SlotNotBefore
				}
				for _, crossed := range entries[gap+1 : index] {
					if !canPassForSTARGap(flight, crossed.flight) {
						// An earlier filler from the same stream is an order
						// boundary, but space after it can still be usable.
						if boundary := crossed.time.Add(time.Nanosecond); boundary.After(lower) {
							lower = boundary
						}
					}
				}
				if !lower.Before(trailing.time) {
					continue
				}
				if index+1 < len(entries) && !adjacentValid(policy, entries[index-1], entries[index+1]) {
					// Removing a flight must not expose an invalid wake or
					// same-STAR pair at the slot it vacates.
					continue
				}
				remaining := slices.Delete(slices.Clone(entries), index, index+1)
				if boundary := leading.time.Add(requiredGap(policy, leading.flight, flight, lower)); boundary.After(lower) {
					lower = boundary
				}
				candidate, ok := nextGridAtOrAfter(policy, lower)
				for attempts := 0; ok && candidate.Before(trailing.time) && attempts <= len(remaining)+len(policy.Gaps)+len(policy.Closures)+len(policy.rates); attempts++ {
					// Cross-stream committed order is relaxed only for this empty
					// interval. Manual order, holding priority, gaps and spacing
					// on both sides are still checked by normal placement.
					valid, _, later := placementWithStableOrder(policy, remaining, flight, candidate, false)
					if valid {
						target.time = candidate
						entries = append(remaining, target)
						sortEntries(entries)
						moved[flight.Callsign] = true
						filled = true
						break
					}
					candidate, ok = nextGridAtOrAfter(policy, later)
				}
				if filled {
					break
				}
			}
		}
		if !filled {
			break
		}
	}
	return entries
}

func gapFillEligible(flight preparedFlight) bool {
	return flight.STARFamily != "" && flight.ManualOrder == nil && flight.ReleaseRank == nil &&
		!flight.RebuildRetained && !flight.RebuildTMA && flight.PromotionNotBefore != nil &&
		(flight.FreezeReason == aman.FreezeNone || flight.FreezeReason == aman.FreezeSuperstable) &&
		(flight.State == aman.StateStable || flight.State == aman.StateUnstable || flight.State == aman.StateAirborne)
}

func canPassForSTARGap(target, crossed preparedFlight) bool {
	return crossed.ManualOrder == nil && crossed.STARFamily != "" && crossed.STARFamily != target.STARFamily &&
		(crossed.FreezeReason == aman.FreezeNone || crossed.FreezeReason == aman.FreezeSuperstable) &&
		!crossed.RebuildRetained && !crossed.RebuildTMA &&
		(target.HoldingQueueID == "" || target.HoldingQueueID != crossed.HoldingQueueID)
}
