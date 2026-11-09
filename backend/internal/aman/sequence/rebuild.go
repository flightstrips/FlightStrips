package sequence

import (
	"FlightStrips/internal/aman"
	"fmt"
	"slices"
	"sort"
	"time"
)

type RebuildDecision struct {
	Decision
	Outcome aman.ArrivalSequenceRebuildOutcome
}

// ApplyArrivalSequenceRebuild merges the heads of physical stacks by elapsed
// holding time. It allocates releases and landings together, not by sorting ETA.
func ApplyArrivalSequenceRebuild(input Input, at time.Time) (RebuildDecision, error) {
	if !validUTC(at) {
		return RebuildDecision{}, invalidArgument("rebuild requires a UTC calculation time")
	}
	working := cloneInput(input)
	outcome := aman.ArrivalSequenceRebuildOutcome{ReleaseOrder: []aman.Callsign{}, Diagnostics: []aman.RebuildDiagnostic{}}
	diagnose := func(code string, f Flight, message string) {
		outcome.Diagnostics = append(outcome.Diagnostics, aman.RebuildDiagnostic{Code: code, Callsign: f.Callsign, Message: message})
	}
	blocked := map[string]bool{}
	for _, f := range working.Flights {
		if f.HoldingStackID == "" || f.RebuildTMA {
			continue
		}
		if f.HoldingAltitudeFeet == nil || f.HoldingStartedAt == nil || f.HoldingTransit <= 0 {
			blocked[f.HoldingStackID] = true
			diagnose("holding_evidence_unavailable", f, "Holding level, entry time or fix-to-runway transit is unavailable; using current position-based timing")
		}
	}
	for i := range working.Flights {
		f := &working.Flights[i]
		f.ManualOrder = nil
		f.ReleaseRank = nil
		f.ProtectCurrentSlot = false
		f.HoldingSlotProtected = false
		f.FreezeReason = aman.FreezeNone
		f.FrozenAt, f.FrozenOperationalTETA, f.CapturedSlot = nil, nil, nil
	}
	policies, groups, err := prepareInput(working)
	if err != nil {
		return RebuildDecision{}, err
	}
	byID := map[aman.Callsign]preparedFlight{}
	allocated := map[aman.RunwayGroupID][]allocatedEntry{}
	for _, flights := range groups {
		for _, f := range flights {
			byID[f.Callsign] = f
		}
	}
	queues := map[string][]preparedFlight{}
	inbound := []preparedFlight{}
	tma := []preparedFlight{}
	tmaLast := map[aman.RunwayGroupID]time.Time{}
	for _, f := range byID {
		if f.RebuildTMA {
			tma = append(tma, f)
		} else if f.HoldingStackID != "" && !blocked[f.HoldingStackID] {
			queues[f.HoldingStackID] = append(queues[f.HoldingStackID], f)
		} else {
			inbound = append(inbound, f)
		}
	}
	// Near-runway traffic must not inherit distant forecasts or old captures.
	// Start each runway's TMA queue at now + 90 seconds, then use that cadence
	// (with any longer wake separation) before allocating outside traffic.
	sort.Slice(tma, func(i, j int) bool {
		if !tma[i].OperationalTETA.Equal(tma[j].OperationalTETA) {
			return tma[i].OperationalTETA.Before(tma[j].OperationalTETA)
		}
		return tma[i].Callsign < tma[j].Callsign
	})
	for _, f := range tma {
		policy := policies[f.RunwayGroupID]
		policy.rates = []RatePoint{{EffectiveAt: at, ArrivalsPerHour: 40}}
		policy.ContinuousSpacing = true
		policy.starFamilies = preparedSTARFamilyPolicies{authoritative: true}
		lower := at.Add(rebuildTMAInterval)
		if last := tmaLast[f.RunwayGroupID]; !last.IsZero() {
			lower = last.Add(rebuildTMAInterval)
		}
		f.OperationalTETA, f.SlotNotBefore, f.PromotionNotBefore = lower, &lower, nil
		f.stableOrder = nil
		f.HoldingQueueTime, f.ActiveHoldingSince, f.ArrivalQueueTime = nil, nil, nil
		candidate, err := findCandidate(policy, allocated[f.RunwayGroupID], f)
		if err != nil {
			return RebuildDecision{}, err
		}
		allocated[f.RunwayGroupID] = append(allocated[f.RunwayGroupID], allocatedEntry{flight: f, time: candidate, reason: ReasonRebuildTMA})
		sortEntries(allocated[f.RunwayGroupID])
		tmaLast[f.RunwayGroupID] = candidate
	}
	for id := range queues {
		sort.Slice(queues[id], func(i, j int) bool {
			a, b := queues[id][i], queues[id][j]
			if *a.HoldingAltitudeFeet != *b.HoldingAltitudeFeet {
				return *a.HoldingAltitudeFeet < *b.HoldingAltitudeFeet
			}
			return holdingAgeLess(a, b)
		})
	}
	ordered := []preparedFlight{}
	for len(queues) > 0 {
		var selected string
		for id, q := range queues {
			if selected == "" || holdingAgeLess(q[0], queues[selected][0]) {
				selected = id
			}
		}
		ordered = append(ordered, queues[selected][0])
		queues[selected] = queues[selected][1:]
		if len(queues[selected]) == 0 {
			delete(queues, selected)
		}
	}
	releases := map[string]time.Time{}
	globalRelease := at.Add(time.Second)
	heldLast := map[aman.RunwayGroupID]time.Time{}
	for rank, f := range ordered {
		value := rank + 1
		f.ReleaseRank = &value
		index := flightIndex(working.Flights, f.Callsign)
		working.Flights[index].ReleaseRank = &value
		outcome.ReleaseOrder = append(outcome.ReleaseOrder, f.Callsign)
		lower := globalRelease
		if previous := releases[f.HoldingStackID]; !previous.IsZero() && previous.Add(time.Minute).After(lower) {
			lower = previous.Add(time.Minute)
		}
		lower = lower.Add(f.HoldingTransit)
		if f.SlotNotBefore != nil && f.SlotNotBefore.After(lower) {
			lower = *f.SlotNotBefore
		}
		if last := tmaLast[f.RunwayGroupID]; last.After(lower) {
			lower = last
		}
		f.OperationalTETA = lower
		f.SlotNotBefore = &lower
		f.stableOrder = nil
		// Release bounds above replace legacy admission FIFO only in this calculation.
		f.HoldingQueueTime = nil
		f.ActiveHoldingSince = nil
		f.ArrivalQueueTime = nil
		candidate, err := findCandidate(policies[f.RunwayGroupID], allocated[f.RunwayGroupID], f)
		if err != nil {
			return RebuildDecision{}, invalidTransition(fmt.Sprintf("rebuild cannot allocate %s: %v", f.Callsign, err))
		}
		allocated[f.RunwayGroupID] = append(allocated[f.RunwayGroupID], allocatedEntry{flight: f, time: candidate, reason: ReasonRateWTC})
		sortEntries(allocated[f.RunwayGroupID])
		release := candidate.Add(-f.HoldingTransit)
		releases[f.HoldingStackID] = release
		globalRelease = release
		if candidate.After(heldLast[f.RunwayGroupID]) {
			heldLast[f.RunwayGroupID] = candidate
		}
	}
	sort.Slice(inbound, func(i, j int) bool {
		a, b := inbound[i], inbound[j]
		if !a.OperationalTETA.Equal(b.OperationalTETA) {
			return a.OperationalTETA.Before(b.OperationalTETA)
		}
		return a.Callsign < b.Callsign
	})
	for _, f := range inbound {
		lower := at
		for _, v := range []time.Time{tmaLast[f.RunwayGroupID], heldLast[f.RunwayGroupID]} {
			if v.After(lower) {
				lower = v
			}
		}
		if f.SlotNotBefore != nil && f.SlotNotBefore.After(lower) {
			lower = *f.SlotNotBefore
		}
		f.SlotNotBefore = &lower
		f.stableOrder = nil
		f.HoldingQueueTime = nil
		f.ActiveHoldingSince = nil
		f.ArrivalQueueTime = nil
		candidate, err := findCandidate(policies[f.RunwayGroupID], allocated[f.RunwayGroupID], f)
		if err != nil {
			return RebuildDecision{}, err
		}
		allocated[f.RunwayGroupID] = append(allocated[f.RunwayGroupID], allocatedEntry{flight: f, time: candidate, reason: ReasonRateWTC})
		sortEntries(allocated[f.RunwayGroupID])
	}
	// A rebuilt automatic sequence is not a controller-imposed manual order.
	// Slots and holding release ranks retain its reservations; later
	// reconciliation must still be able to fill unused capacity.
	result := Result{Entries: []CandidateEntry{}, Movements: []SlotMovement{}, Warnings: []Warning{}}
	groupIDs := make([]aman.RunwayGroupID, 0, len(allocated))
	for id := range allocated {
		groupIDs = append(groupIDs, id)
	}
	slices.Sort(groupIDs)
	for _, group := range groupIDs {
		for i := range allocated[group] {
			f := &allocated[group][i].flight
			if f.ReleaseRank == nil && !f.RebuildTMA {
				// The initial placement follows held traffic. That administrative
				// bound must not hide an otherwise reachable cross-stream gap.
				f.SlotNotBefore = byID[f.Callsign].SlotNotBefore
			}
		}
		allocated[group] = fillSequenceGaps(policies[group], allocated[group], at)
		for i, e := range allocated[group] {
			entry := CandidateEntry{Callsign: e.flight.Callsign, RunwayGroupID: group, Sequence: i + 1, Time: e.time, OperationalTETA: e.flight.OperationalTETA, WakeCategory: e.flight.category, FreezeReason: e.flight.FreezeReason, Reason: e.reason}
			result.Entries = append(result.Entries, entry)
			if movement := movementFor(e.flight, entry); movement != nil {
				result.Movements = append(result.Movements, *movement)
			}
		}
	}
	sort.Slice(outcome.Diagnostics, func(i, j int) bool {
		a, b := outcome.Diagnostics[i], outcome.Diagnostics[j]
		if a.Callsign != b.Callsign {
			return a.Callsign < b.Callsign
		}
		return a.Code < b.Code
	})
	return RebuildDecision{Decision: Decision{Input: working, Candidate: result, Changed: len(result.Movements) > 0 || !inputsEqualOrder(input, working)}, Outcome: outcome}, nil
}

func holdingAgeLess(a, b preparedFlight) bool {
	if !a.HoldingStartedAt.Equal(*b.HoldingStartedAt) {
		return a.HoldingStartedAt.Before(*b.HoldingStartedAt)
	}
	return a.Callsign < b.Callsign
}
