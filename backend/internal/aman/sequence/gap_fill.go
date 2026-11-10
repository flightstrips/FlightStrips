package sequence

import (
	"slices"
	"time"

	"FlightStrips/internal/aman"
)

// Earlier holding traffic owns usable capacity first. An independent stream
// may pass it only when it cannot use this instant itself, either because it
// cannot physically reach the runway or because runway/STAR spacing prevents
// it. Allocation orders the hold first, so this insertion keeps its slot.
func canFillHoldingGap(policy preparedPolicy, entries []allocatedEntry, older allocatedEntry, later preparedFlight, candidate time.Time) bool {
	if !policy.AllowCrossFeederOvertake || !candidate.Before(older.time) ||
		!canPassForGap(later, older.flight) || older.flight.ReleaseRank != nil || later.ReleaseRank != nil {
		return false
	}
	if older.flight.PromotionNotBefore != nil && candidate.Before(*older.flight.PromotionNotBefore) {
		return true
	}
	// OperationalTETA and the assigned slot can contain accumulated delay;
	// neither proves that the held aircraft cannot arrive sooner.
	var leading, trailing *allocatedEntry
	for i := range entries {
		entry := &entries[i]
		if entry.flight.Callsign == older.flight.Callsign {
			continue
		}
		if entry.time.Before(candidate) {
			leading = entry
		} else {
			trailing = entry
			break
		}
	}
	return leading != nil && candidate.Sub(leading.time) < requiredGap(policy, leading.flight, older.flight, candidate) ||
		trailing != nil && trailing.time.Sub(candidate) < requiredGap(policy, older.flight, trailing.flight, trailing.time)
}

// Fill unused capacity between arrivals, including vacancies left by removed
// flights and same-STAR spacing. A later aircraft may use that capacity without
// moving either reservation.
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
			for index := gap + 2; index < len(entries); index++ {
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
					valid, _, later := placementWithStableOrder(policy, remaining, flight, candidate, false)
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
	return flight.STARFamily != "" && flight.ManualOrder == nil && flight.ReleaseRank == nil &&
		!flight.RebuildTMA && flight.PromotionNotBefore != nil &&
		(flight.FreezeReason == aman.FreezeNone || flight.FreezeReason == aman.FreezeSuperstable) &&
		(flight.State == aman.StateStable || flight.State == aman.StateUnstable || flight.State == aman.StateAirborne)
}

func canPassForGap(target, crossed preparedFlight) bool {
	return crossed.ManualOrder == nil && crossed.STARFamily != "" && crossed.STARFamily != target.STARFamily &&
		(crossed.FreezeReason == aman.FreezeNone || crossed.FreezeReason == aman.FreezeSuperstable) &&
		!crossed.RebuildTMA &&
		(target.HoldingQueueID == "" || target.HoldingQueueID != crossed.HoldingQueueID)
}
