package sequence

import (
	"fmt"
	"slices"
	"sort"
	"time"

	"FlightStrips/internal/aman"
)

const ReasonManualInsertion CandidateReason = "manual_insertion"

// InsertAfter expresses relative order. Other streams may use capacity between
// the anchor and target, and this explicit command may retime frozen followers.
func InsertAfter(input Input, targetID, anchorID aman.Callsign, at time.Time) (Decision, error) {
	working := cloneInput(input)
	targetIndex, target, err := activeFlight(working, targetID)
	if err != nil {
		return Decision{}, err
	}
	_, anchor, err := activeFlight(working, anchorID)
	if err != nil {
		return Decision{}, err
	}
	if targetID == anchorID || target.RunwayGroupID != anchor.RunwayGroupID || anchor.CurrentSlot == nil {
		return Decision{}, invalidArgument("insertion requires another sequenced flight on the same runway")
	}
	if anchor.CurrentSlot.Time.Before(at) {
		return Decision{}, invalidTransition("anchor slot is already in the past")
	}
	// Replace the target's instruction, but retain compatible instructions on
	// other aircraft. In particular, a separately instructed "Y after X" must
	// not be discarded merely to use Y as an interleaver for X.
	replaceInsertionPosition(&working, targetID, anchorID)
	working.Flights[targetIndex].ManualOrder = nil
	working.Flights[targetIndex].ManualPosition = &aman.ManualSequencePosition{AfterCallsign: anchorID}
	policies, groups, err := prepareInput(working)
	if err != nil {
		return Decision{}, err
	}
	policy := policies[anchor.RunwayGroupID]
	var moving preparedFlight
	entries := []allocatedEntry{}
	for _, f := range groups[anchor.RunwayGroupID] {
		if f.Callsign == targetID {
			moving = f
			continue
		}
		if f.State == aman.StateLanded || f.State == aman.StateRemoved {
			continue
		}
		if f.CurrentSlot == nil {
			continue
		} // other unslotted demand is handled by normal reconciliation
		entries = append(entries, allocatedEntry{flight: f, time: f.CurrentSlot.Time, reason: protectedReason(f)})
	}
	sortEntries(entries)
	anchorAt := slices.IndexFunc(entries, func(e allocatedEntry) bool { return e.flight.Callsign == anchorID })
	if anchorAt < 0 {
		return Decision{}, invalidTransition("anchor has no committed runway slot")
	}
	// Keep the next flight of this stream behind the inserted target. This is
	// an order boundary, not an adjacency boundary: another STAR may still pass.
	for _, e := range entries[anchorAt+1:] {
		if e.flight.STARFamily == moving.STARFamily && moving.STARFamily != "" {
			working.Flights[targetIndex].ManualPosition.BeforeCallsign = e.flight.Callsign
			break
		}
	}
	policies, groups, err = prepareInput(working)
	if err != nil {
		return Decision{}, err
	}
	policy = policies[anchor.RunwayGroupID]
	// This controller action explicitly allows independent STAR streams to
	// interleave. The ordinary automatic-policy switch is left unchanged.
	policy.AllowCrossFeederOvertake = true
	for _, f := range groups[anchor.RunwayGroupID] {
		if f.Callsign == targetID {
			moving = f
		}
		for i := range entries {
			if entries[i].flight.Callsign == f.Callsign {
				entries[i].flight = f
			}
		}
	}
	prefix := slices.Clone(entries[:anchorAt+1])
	suffix := slices.Clone(entries[anchorAt+1:])
	for i := 1; i < len(prefix); i++ {
		if !adjacentValid(policy, prefix[i-1], prefix[i]) {
			return Decision{}, invalidTransition("the sequence preceding the anchor has a spacing conflict")
		}
	}
	best, err := scheduleInsertion(policy, prefix, moving, suffix, at)
	if err != nil {
		return Decision{}, err
	}
	bestTime, bestCost := insertionScore(best, targetID)
	baselineTime := bestTime
	baseline := slices.Clone(best)
	// Candidates retain the existing holding-first priority. Search legal
	// interleavings before committing the cascade; each branch consumes a real
	// interval before the baseline target, so the search is bounded by capacity.
	candidates := gapFillCandidateOrder(suffix, 0)
	// Bound work by candidate evaluations, rather than wall time, so replay is
	// deterministic even with unusually large sets of reachable arrivals.
	remainingSearch := 4096
	exploreChildren := false
	var search func([]allocatedEntry, []allocatedEntry)
	search = func(front, rest []allocatedEntry) {
		for priority := 0; priority < len(candidates) && remainingSearch > 0; priority++ {
			original := suffix[candidates[priority]]
			index := slices.IndexFunc(rest, func(e allocatedEntry) bool { return e.flight.Callsign == original.flight.Callsign })
			if index < 0 {
				continue
			}
			filler := rest[index]
			retainProtected := filler.flight.FreezeReason != aman.FreezeNone && filler.flight.ManualOrder == nil && filler.time.Before(baselineTime)
			if moving.STARFamily == "" || moving.STARFamily != anchor.STARFamily || filler.flight.STARFamily == moving.STARFamily || !gapFillEligible(filler.flight) && !retainProtected {
				continue
			}
			crossed := append([]allocatedEntry{{flight: moving}}, rest[:index]...)
			canPass := true
			for _, e := range crossed {
				if e.flight.Callsign == targetID {
					e.flight.FreezeReason = aman.FreezeNone
					e.flight.ManualOrder = nil
				}
				if !canPassForGap(filler.flight, e.flight) || positionPrecedes(e.flight, filler.flight) {
					canPass = false
					break
				}
			}
			if !canPass {
				continue
			}
			if index > 0 && index+1 < len(rest) && !adjacentValid(policy, rest[index-1], rest[index+1]) {
				continue
			}
			lower := promotionLowerBound(policy, filler.flight, at)
			if retainProtected && filler.time.After(lower) {
				lower = filler.time
			}
			candidate, ok := insertionTime(policy, front, filler.flight, lower)
			if !ok || candidate.After(filler.time) || !candidate.Before(bestTime) {
				continue
			}
			if filler.flight.FreezeReason != aman.FreezeNone && !candidate.Equal(filler.time) {
				continue
			}
			for _, e := range crossed {
				for _, original := range baseline {
					if original.flight.Callsign == e.flight.Callsign {
						e.time = original.time
						break
					}
				}
				if e.flight.Callsign == targetID {
					e.flight.FreezeReason = aman.FreezeNone
					e.flight.ManualOrder = nil
				}
				if policy.holdingPrecedes(e.flight.Flight, filler.flight.Flight) && !canFillHoldingGap(policy, baseline, e, filler.flight, candidate) {
					canPass = false
					break
				}
			}
			if !canPass {
				continue
			}
			filler.time, filler.reason = candidate, ReasonQueuePromotion
			nextFront := append(slices.Clone(front), filler)
			nextRest := slices.Delete(slices.Clone(rest), index, index+1)
			remainingSearch--
			trial, trialErr := scheduleInsertion(policy, nextFront, moving, nextRest, at)
			if trialErr != nil {
				continue
			}
			targetAt, cost := insertionScore(trial, targetID)
			if targetAt.After(bestTime) {
				continue
			}
			if targetAt.Before(bestTime) || targetAt.Equal(bestTime) && cost < bestCost {
				best, bestTime, bestCost = trial, targetAt, cost
			}
			if exploreChildren {
				search(nextFront, nextRest)
			}
		}
	}
	search(prefix, suffix)
	// Try every single interleaver before spending the remaining work budget
	// on combinations, so a deep early branch cannot hide a better stream.
	exploreChildren = true
	search(prefix, suffix)
	// Persist only local relationships. The ordinary engine must reproduce the
	// committed cascade without a synthetic full-runway ManualOrder.
	for _, e := range best {
		index := flightIndex(working.Flights, e.flight.Callsign)
		if index < 0 {
			continue
		}
		if working.Flights[index].FreezeReason != aman.FreezeNone {
			reason := string(e.reason)
			if captured := working.Flights[index].CapturedSlot; captured != nil && captured.Reason == string(ReasonRebuildTMA) {
				// This marker retains the rebuild's spacing rules even when
				// the explicit insertion moves the captured landing time.
				reason = captured.Reason
			}
			working.Flights[index].CapturedSlot = &aman.Slot{Time: e.time, RunwayGroupID: anchor.RunwayGroupID, Sequence: slices.IndexFunc(best, func(v allocatedEntry) bool { return v.flight.Callsign == e.flight.Callsign }) + 1, Revision: input.Revision, Reason: reason}
		}
	}
	result := insertionResult(input, anchor.RunwayGroupID, best)
	if remainingSearch == 0 {
		result.Operations = append(result.Operations, Operation{Callsign: targetID, Kind: "insertion_search_bounded"})
	}
	// Preserve other runways exactly; this operation has one authoritative anchor.
	for _, f := range input.Flights {
		if f.RunwayGroupID != anchor.RunwayGroupID && f.CurrentSlot != nil && f.State != aman.StateLanded && f.State != aman.StateRemoved {
			result.Entries = append(result.Entries, CandidateEntry{Callsign: f.Callsign, RunwayGroupID: f.RunwayGroupID, Sequence: f.CurrentSlot.Sequence, Time: f.CurrentSlot.Time, OperationalTETA: f.OperationalTETA, WakeCategory: f.WakeCategory, FreezeReason: f.FreezeReason, Protected: f.FreezeReason != aman.FreezeNone, Reason: CandidateReason(f.CurrentSlot.Reason)})
		}
	}
	sort.Slice(result.Entries, func(i, j int) bool {
		if result.Entries[i].RunwayGroupID != result.Entries[j].RunwayGroupID {
			return result.Entries[i].RunwayGroupID < result.Entries[j].RunwayGroupID
		}
		return result.Entries[i].Sequence < result.Entries[j].Sequence
	})
	return Decision{Input: working, Candidate: result, Changed: true}, nil
}

func replaceInsertionPosition(input *Input, target, anchor aman.Callsign) {
	group := input.Flights[flightIndex(input.Flights, anchor)].RunwayGroupID
	flights := []Flight{}
	for _, f := range input.Flights {
		if f.RunwayGroupID == group && f.CurrentSlot != nil && f.Callsign != target {
			flights = append(flights, f)
		}
	}
	sort.Slice(flights, func(i, j int) bool {
		if flights[i].CurrentSlot.Time.Equal(flights[j].CurrentSlot.Time) {
			return flights[i].Callsign < flights[j].Callsign
		}
		return flights[i].CurrentSlot.Time.Before(flights[j].CurrentSlot.Time)
	})
	order := make([]aman.Callsign, 0, len(flights)+1)
	for _, f := range flights {
		order = append(order, f.Callsign)
		if f.Callsign == anchor {
			order = append(order, target)
		}
	}
	rank := make(map[aman.Callsign]int, len(order))
	for i, id := range order {
		rank[id] = i
	}
	for i := range input.Flights {
		f := &input.Flights[i]
		if f.Callsign == target {
			f.ManualPosition = nil
			continue
		}
		if f.ManualPosition == nil {
			continue
		}
		at, known := rank[f.Callsign]
		if !known {
			continue
		}
		position := *f.ManualPosition
		if position.AfterCallsign == target && at < rank[target] {
			position.AfterCallsign = ""
		}
		if position.BeforeCallsign == target && at > rank[target] {
			position.BeforeCallsign = ""
		}
		f.ManualPosition = &position
		if position.AfterCallsign == "" && position.BeforeCallsign == "" {
			f.ManualPosition = nil
		}
	}
}

func insertionTime(policy preparedPolicy, prior []allocatedEntry, f preparedFlight, lower time.Time) (time.Time, bool) {
	if f.SlotNotBefore != nil && f.SlotNotBefore.After(lower) {
		lower = *f.SlotNotBefore
	}
	if f.PromotionNotBefore != nil && f.PromotionNotBefore.After(lower) {
		lower = *f.PromotionNotBefore
	}
	if len(prior) > 0 {
		last := prior[len(prior)-1]
		if bound := last.time.Add(requiredGap(policy, last.flight, f, lower)); bound.After(lower) {
			lower = bound
		}
	}
	candidate, ok := nextGridAtOrAfter(policy, lower)
	for attempts := 0; ok && attempts <= len(policy.Gaps)+len(policy.Closures)+len(policy.rates)+len(prior)+4; attempts++ {
		for _, e := range prior {
			if positionPrecedes(f, e.flight) {
				return time.Time{}, false
			}
		}
		// This explicit action may change stable landing order, but it retains
		// the normal runway, holding-transit and rebuilt-release constraints.
		valid, _, later := placementWithStableOrder(policy, prior, f, candidate, false)
		if valid {
			return candidate, true
		}
		candidate, ok = nextGridAtOrAfter(policy, later)
	}
	return time.Time{}, false
}

func scheduleInsertion(policy preparedPolicy, prefix []allocatedEntry, target preparedFlight, suffix []allocatedEntry, at time.Time) ([]allocatedEntry, error) {
	result := slices.Clone(prefix)
	lower := target.OperationalTETA.Add(-policy.EarlyTolerance)
	if at.After(lower) {
		lower = at
	}
	when, ok := insertionTime(policy, result, target, lower)
	if !ok {
		return nil, invalidTransition("no legal slot behind anchor")
	}
	result = append(result, allocatedEntry{flight: target, time: when, reason: ReasonManualInsertion})
	for _, e := range suffix {
		lower = e.time
		if e.flight.OperationalTETA.Add(-policy.EarlyTolerance).After(lower) {
			lower = e.flight.OperationalTETA.Add(-policy.EarlyTolerance)
		}
		when, ok = insertionTime(policy, result, e.flight, lower)
		if !ok {
			return nil, invalidTransition(fmt.Sprintf("no legal following slot for %s", e.flight.Callsign))
		}
		e.time = when
		result = append(result, e)
	}
	return result, nil
}

func insertionScore(entries []allocatedEntry, target aman.Callsign) (time.Time, time.Duration) {
	var at time.Time
	var cost time.Duration
	for _, e := range entries {
		if e.flight.Callsign == target {
			at = e.time
		}
		if e.flight.CurrentSlot != nil {
			cost += e.time.Sub(e.flight.CurrentSlot.Time)
		}
	}
	return at, cost
}

func insertionResult(input Input, group aman.RunwayGroupID, entries []allocatedEntry) Result {
	result := Result{Entries: []CandidateEntry{}, Movements: []SlotMovement{}, Warnings: []Warning{}}
	for i, e := range entries {
		candidate := CandidateEntry{Callsign: e.flight.Callsign, RunwayGroupID: group, Sequence: i + 1, Time: e.time, OperationalTETA: e.flight.OperationalTETA, WakeCategory: e.flight.category, FreezeReason: e.flight.FreezeReason, Protected: e.flight.FreezeReason != aman.FreezeNone, Reason: e.reason}
		result.Entries = append(result.Entries, candidate)
		if e.reason == ReasonQueuePromotion {
			result.Operations = append(result.Operations, Operation{Callsign: e.flight.Callsign, Kind: "gap_fill"})
		}
		if movement := movementFor(e.flight, candidate); movement != nil {
			result.Movements = append(result.Movements, *movement)
		}
		if !e.flight.known {
			result.Warnings = append(result.Warnings, Warning{Severity: SeverityDegraded, Code: WarningUnknownWakeCategory, RunwayGroupID: group, Callsign: e.flight.Callsign})
		}
	}
	return result
}
