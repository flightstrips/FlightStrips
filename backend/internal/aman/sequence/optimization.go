package sequence

import (
	"math"
	"slices"
	"time"

	"FlightStrips/internal/aman"
)

// Compare nearby allocation orders using the same placement rules as normal
// sequencing. Two sweeps bound the work; equal-delay sequences change only
// when that reduces physical STAR-order inversions. This is a local optimization, not a
// globally optimal scheduling search.
func optimizeMovableOrder(policy preparedPolicy, protected []allocatedEntry, ordered []preparedFlight, best []allocatedEntry) []allocatedEntry {
	ordered = slices.Clone(ordered)
	bestDelay := sequenceDelay(best)
	trials := 0
	for pass := 0; pass < 2; pass++ {
		changed := false
		for i := 0; i+1 < len(ordered); i++ {
			left, right := ordered[i], ordered[i+1]
			if !automaticOrderEligible(left) || !automaticOrderEligible(right) || left.State != right.State {
				continue
			}
			if trials == 32 {
				return best
			}
			trials++
			ordered[i], ordered[i+1] = right, left
			candidate, err := allocateMovableFlights(policy, protected, ordered)
			if err == nil && starInversions(candidate) <= starInversions(best) && (sequenceDelay(candidate) < bestDelay ||
				(sequenceDelay(candidate) == bestDelay && reducesSTARInversions(best, candidate))) && retainsOrderBoundaries(best, candidate) {
				best, bestDelay = candidate, sequenceDelay(candidate)
				changed = true
			} else {
				ordered[i], ordered[i+1] = left, right
			}
		}
		if !changed {
			break
		}
	}
	return best
}

func automaticOrderEligible(flight preparedFlight) bool {
	return flight.FreezeReason == aman.FreezeNone && !flight.ProtectCurrentSlot &&
		flight.ManualOrder == nil && flight.State != aman.StateStable &&
		flight.State != aman.StateGoAround && !hasHoldingPriority(flight)
}

func hasHoldingPriority(flight preparedFlight) bool {
	// Selected holding fixes and arrival predictions exist for ordinary STAR
	// traffic too; only evidence of an actual holding queue protects its order.
	return flight.HoldingSlotProtected || flight.HoldingStackID != "" ||
		flight.HoldingQueueTime != nil || flight.ActiveHoldingSince != nil
}

// Stable arrivals may exchange existing slots only within the same explicitly
// selected STAR family. Equal-delay exchanges can restore the physical arrival
// order. Neither slot is compacted or taken from another flight.
func optimizeStableSameSTAR(policy preparedPolicy, entries []allocatedEntry) []allocatedEntry {
	trials := 0
	for i := 0; i+1 < len(entries); i++ {
		left, right := entries[i], entries[i+1]
		if !stableSwapEligible(left.flight) || !stableSwapEligible(right.flight) ||
			left.reason != ReasonStable || right.reason != ReasonStable ||
			left.flight.SelectedSTARFamily == "" || left.flight.SelectedSTARFamily != right.flight.SelectedSTARFamily {
			continue
		}
		if trials == 128 {
			break
		}
		trials++
		trialLeft, trialRight := right, left
		trialLeft.time, trialRight.time = left.time, right.time
		beforeDelay := sequenceDelay([]allocatedEntry{left, right})
		afterDelay := sequenceDelay([]allocatedEntry{trialLeft, trialRight})
		precedes, comparable := starPrecedes(right.flight, left.flight)
		if !automaticTargetReachable(policy, trialLeft) || !automaticTargetReachable(policy, trialRight) ||
			(comparable && !precedes) || afterDelay > beforeDelay ||
			(afterDelay == beforeDelay && !precedes) ||
			!adjacentValid(policy, trialLeft, trialRight) ||
			(i > 0 && !adjacentValid(policy, entries[i-1], trialLeft)) ||
			(i+2 < len(entries) && !adjacentValid(policy, trialRight, entries[i+2])) {
			continue
		}
		entries[i], entries[i+1] = trialLeft, trialRight
		// Revisit the preceding pair so an inversion spanning several adjacent
		// arrivals can be corrected in the same calculation.
		i = max(-1, i-2)
	}
	return entries
}

// Timing to the same fix orders approaching traffic. Once both aircraft have
// passed it, compare distance only on matching remaining route suffixes.
// Small differences are inconclusive to avoid reordering on prediction jitter.
func starPrecedes(a, b preparedFlight) (bool, bool) {
	left, right := a.STARProgress, b.STARProgress
	if a.SelectedSTARFamily == "" || a.SelectedSTARFamily != b.SelectedSTARFamily ||
		left == nil || right == nil || left.Fix == "" || left.Fix != right.Fix ||
		left.TerminalDigest == "" || left.TerminalDigest != right.TerminalDigest {
		return false, false
	}
	if left.Passed != right.Passed {
		return left.Passed, true
	}
	if !left.Passed {
		if left.ETA == nil || right.ETA == nil || !validUTC(*left.ETA) || !validUTC(*right.ETA) {
			return false, false
		}
		difference := right.ETA.Sub(*left.ETA)
		return difference > 15*time.Second, difference > 15*time.Second || difference < -15*time.Second
	}
	if !matchingRouteSuffix(left.RemainingFixes, right.RemainingFixes) ||
		math.IsNaN(left.DistanceToGoNM) || math.IsNaN(right.DistanceToGoNM) ||
		math.IsInf(left.DistanceToGoNM, 0) || math.IsInf(right.DistanceToGoNM, 0) ||
		left.DistanceToGoNM < 0 || right.DistanceToGoNM < 0 {
		return false, false
	}
	difference := right.DistanceToGoNM - left.DistanceToGoNM
	return difference > 0.5, math.Abs(difference) > 0.5
}

func matchingRouteSuffix(a, b []string) bool {
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	if len(a) > len(b) {
		a, b = b, a
	}
	return slices.Equal(a, b[len(b)-len(a):])
}

func reducesSTARInversions(before, after []allocatedEntry) bool {
	return starInversions(after) < starInversions(before)
}

func starInversions(entries []allocatedEntry) int {
	inversions := 0
	for i, entry := range entries {
		for _, later := range entries[i+1:] {
			if precedes, _ := starPrecedes(later.flight, entry.flight); precedes {
				inversions++
			}
		}
	}
	return inversions
}

func stableSwapEligible(flight preparedFlight) bool {
	return flight.State == aman.StateStable && flight.ProtectCurrentSlot &&
		flight.FreezeReason == aman.FreezeNone && flight.ManualOrder == nil && !hasHoldingPriority(flight)
}

func automaticTargetReachable(policy preparedPolicy, entry allocatedEntry) bool {
	flight := entry.flight
	return !entry.time.Before(flight.OperationalTETA.Add(-policy.EarlyTolerance)) &&
		(flight.SlotNotBefore == nil || !entry.time.Before(*flight.SlotNotBefore)) &&
		(flight.PromotionNotBefore == nil || !entry.time.Before(*flight.PromotionNotBefore))
}

func sequenceDelay(entries []allocatedEntry) time.Duration {
	var delay time.Duration
	for _, entry := range entries {
		delay += max(time.Duration(0), entry.time.Sub(entry.flight.OperationalTETA))
	}
	return delay
}

// A trial must not carry any arrival across a protected, manual, Stable or
// holding boundary, even when doing so would reduce the total delay.
func retainsOrderBoundaries(before, after []allocatedEntry) bool {
	positions := make(map[aman.Callsign]int, len(after))
	for i, entry := range after {
		positions[entry.flight.Callsign] = i
	}
	for i, anchor := range before {
		if automaticOrderEligible(anchor.flight) && anchor.reason == ReasonRateWTC {
			continue
		}
		for j, entry := range before {
			if i != j && (j < i) != (positions[entry.flight.Callsign] < positions[anchor.flight.Callsign]) {
				return false
			}
		}
	}
	return true
}
