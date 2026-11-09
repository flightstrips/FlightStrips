package sequence

import (
	"slices"
	"time"

	"FlightStrips/internal/aman"
)

// Fill unused capacity between arrivals, including vacancies left by removed
// flights and same-STAR spacing. Prefer holding aircraft, then existing sequence
// order. Inbound traffic may use a vacancy
// after the preferred candidates fail the complete placement checks.
// This does not authorize a general reorder across controller constraints.
func fillSequenceGaps(policy preparedPolicy, entries []allocatedEntry, at time.Time) []allocatedEntry {
	if !policy.AllowCrossFeederOvertake {
		return entries
	}
	entries = slices.Clone(entries)
	moved := make(map[aman.Callsign]bool)
	for len(moved) < len(entries) {
		filled := false
		for gap := 0; gap+1 < len(entries) && !filled; gap++ {
			leading := entries[gap]
			trailing := entries[gap+1]
			// Inspect each vacant interval independently. Candidates can already
			// be inside a wider same-STAR interval, or have no later same-STAR
			// anchor at all after a flight leaves the sequence.
			for _, index := range gapFillCandidateOrder(entries, gap+2) {
				target := entries[index]
				flight := target.flight
				if moved[flight.Callsign] || !gapFillEligible(flight) {
					continue
				}
				lower := promotionLowerBound(policy, flight, at)
				if flight.SlotNotBefore != nil && flight.SlotNotBefore.After(lower) {
					lower = *flight.SlotNotBefore
				}
				for _, crossed := range entries[gap+1 : index] {
					if !canPassForGap(flight, crossed.flight) {
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
					valid, _, later := gapFillPlacement(policy, remaining, flight, candidate)
					if valid {
						target.time = candidate
						noteOperation(&target.flight, "gap_fill")
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
	return flight.STARFamily != "" && flight.ManualOrder == nil &&
		(flight.ReleaseRank == nil || holdingGapCandidate(flight) && flight.HoldingTransit > 0) &&
		!flight.RebuildTMA && flight.PromotionNotBefore != nil &&
		(flight.FreezeReason == aman.FreezeNone || flight.FreezeReason == aman.FreezeSuperstable) &&
		(flight.State == aman.StateStable || flight.State == aman.StateUnstable || flight.State == aman.StateAirborne)
}

func holdingGapCandidate(flight preparedFlight) bool {
	return flight.ActiveHoldingSince != nil || flight.HoldingStartedAt != nil && flight.HoldingStackID != ""
}

func gapFillCandidateOrder(entries []allocatedEntry, first int) []int {
	indices := make([]int, 0, len(entries)-first)
	for _, held := range []bool{true, false} {
		for index := first; index < len(entries); index++ {
			if holdingGapCandidate(entries[index].flight) == held {
				indices = append(indices, index)
			}
		}
	}
	return indices
}

func gapFillPlacement(policy preparedPolicy, entries []allocatedEntry, flight preparedFlight, candidate time.Time) (bool, time.Time, time.Time) {
	if !holdingGapCandidate(flight) {
		// The ordered candidate search has already tried the holding and earlier
		// arrivals. Relax automatic admission priority only for this empty gap;
		// same-hold release order, controller boundaries and spacing still apply.
		policy.allowInboundGapFill = true
	}
	return placementWithStableOrder(policy, entries, flight, candidate, false)
}

func canPassForGap(target, crossed preparedFlight) bool {
	return crossed.ManualOrder == nil && crossed.STARFamily != "" && crossed.STARFamily != target.STARFamily &&
		(crossed.FreezeReason == aman.FreezeNone || crossed.FreezeReason == aman.FreezeSuperstable) &&
		!crossed.RebuildTMA &&
		(target.HoldingQueueID == "" || target.HoldingQueueID != crossed.HoldingQueueID)
}
