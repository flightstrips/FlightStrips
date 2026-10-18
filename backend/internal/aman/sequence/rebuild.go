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
			diagnose("holding_evidence_unavailable", f, "Holding level, entry time or fix-to-runway transit is unavailable; reservations in this hold are retained")
		}
	}
	for i := range working.Flights {
		f := &working.Flights[i]
		f.ManualOrder = nil
		f.ReleaseRank = nil
		if !f.RebuildTMA && blocked[f.HoldingStackID] {
			f.RebuildRetained = true
		}
		if !f.RebuildRetained && f.FreezeReason != aman.FreezeManual {
			f.ProtectCurrentSlot = false
			if f.FreezeReason == aman.FreezeSuperstable || f.FreezeReason == aman.FreezeTMA {
				f.FreezeReason = aman.FreezeNone
				f.FrozenAt = nil
				f.FrozenOperationalTETA = nil
				f.CapturedSlot = nil
			}
		} else if f.FreezeReason == aman.FreezeNone && f.CurrentSlot != nil {
			f.ProtectCurrentSlot = true
		}
	}
	policies, groups, err := prepareInput(working)
	if err != nil {
		return RebuildDecision{}, err
	}
	byID := map[aman.Callsign]preparedFlight{}
	allocated := map[aman.RunwayGroupID][]allocatedEntry{}
	tmaLast := map[aman.RunwayGroupID]time.Time{}
	for group, flights := range groups {
		for _, f := range flights {
			byID[f.Callsign] = f
			if isRetained(f) {
				slot := f.CurrentSlot
				if f.CapturedSlot != nil {
					slot = f.CapturedSlot
				}
				if slot == nil {
					diagnose("retained_without_slot", f.Flight, "Aircraft cannot be rebuilt without usable timing")
					continue
				}
				_, closed, _ := policies[group].blockingClosure(slot.Time)
				if closed || policies[group].intervalAt(slot.Time) == 0 {
					return RebuildDecision{}, invalidTransition("retained slot is incompatible with runway availability")
				}
				allocated[group] = append(allocated[group], allocatedEntry{flight: f, time: slot.Time, reason: protectedReason(f)})
				if f.RebuildTMA && slot.Time.After(tmaLast[group]) {
					tmaLast[group] = slot.Time
				}
			}
		}
		sortEntries(allocated[group])
		for i := 1; i < len(allocated[group]); i++ {
			if !allocated[group][i].time.Before(at) && !adjacentValid(policies[group], allocated[group][i-1], allocated[group][i]) {
				return RebuildDecision{}, invalidTransition(fmt.Sprintf("retained reservations %s and %s violate runway separation", allocated[group][i-1].flight.Callsign, allocated[group][i].flight.Callsign))
			}
		}
	}
	queues := map[string][]preparedFlight{}
	inbound := []preparedFlight{}
	tma := []preparedFlight{}
	for _, f := range byID {
		if f.RebuildTMA && !isRetained(f) {
			tma = append(tma, f)
		} else if f.HoldingStackID != "" && !blocked[f.HoldingStackID] && !f.RebuildTMA {
			queues[f.HoldingStackID] = append(queues[f.HoldingStackID], f)
		} else if !isRetained(f) {
			inbound = append(inbound, f)
		}
	}
	// Current position-based estimates determine TMA order. Old automatic
	// captures no longer reserve opportunities an aircraft cannot use.
	sort.Slice(tma, func(i, j int) bool {
		if !tma[i].OperationalTETA.Equal(tma[j].OperationalTETA) {
			return tma[i].OperationalTETA.Before(tma[j].OperationalTETA)
		}
		return tma[i].Callsign < tma[j].Callsign
	})
	for _, f := range tma {
		lower := at.Add(time.Second)
		if f.OperationalTETA.After(lower) {
			lower = f.OperationalTETA
		}
		if tmaLast[f.RunwayGroupID].After(lower) {
			lower = tmaLast[f.RunwayGroupID]
		}
		if f.SlotNotBefore != nil && f.SlotNotBefore.After(lower) {
			lower = *f.SlotNotBefore
		}
		f.SlotNotBefore = &lower
		f.stableOrder = nil
		f.HoldingQueueTime, f.ActiveHoldingSince, f.ArrivalQueueTime = nil, nil, nil
		candidate, err := findCandidate(policies[f.RunwayGroupID], allocated[f.RunwayGroupID], f)
		if err != nil {
			return RebuildDecision{}, err
		}
		allocated[f.RunwayGroupID] = append(allocated[f.RunwayGroupID], allocatedEntry{flight: f, time: candidate, reason: ReasonRateWTC})
		sortEntries(allocated[f.RunwayGroupID])
		tmaLast[f.RunwayGroupID] = candidate
	}
	for group, entries := range allocated {
		for _, e := range entries {
			if isRetained(e.flight) && !e.flight.RebuildTMA && e.time.Before(tmaLast[group]) {
				diagnose("protected_tma_priority_exception", e.flight.Flight, "Retained reservation precedes traffic already in the TMA")
			}
		}
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
		if isRetained(f) {
			if f.CurrentSlot != nil {
				release := f.CurrentSlot.Time.Add(-f.HoldingTransit)
				if release.Before(globalRelease) || !releases[f.HoldingStackID].IsZero() && release.Before(releases[f.HoldingStackID].Add(time.Minute)) {
					diagnose("protected_release_exception", f.Flight, "Retained reservation prevents the preferred release order; unsafe stack releases are withheld")
				}
				if release.After(globalRelease) {
					globalRelease = release
				}
				if release.After(releases[f.HoldingStackID]) {
					releases[f.HoldingStackID] = release
				}
				if f.CurrentSlot.Time.After(heldLast[f.RunwayGroupID]) {
					heldLast[f.RunwayGroupID] = f.CurrentSlot.Time
				}
			}
			continue
		}
		lower := globalRelease
		if previous := releases[f.HoldingStackID]; !previous.IsZero() && previous.Add(time.Minute).After(lower) {
			lower = previous.Add(time.Minute)
		}
		lower = lower.Add(f.HoldingTransit)
		if f.SlotNotBefore != nil && f.SlotNotBefore.After(lower) {
			lower = *f.SlotNotBefore
		}
		if tmaLast[f.RunwayGroupID].After(lower) {
			lower = tmaLast[f.RunwayGroupID]
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
			if f.ReleaseRank == nil && !f.RebuildTMA && !isRetained(*f) {
				// The initial placement follows held traffic. That administrative
				// bound must not hide an otherwise reachable cross-stream gap.
				f.SlotNotBefore = byID[f.Callsign].SlotNotBefore
			}
		}
		allocated[group] = fillSTARSpacingGaps(policies[group], allocated[group], at)
		for i, e := range allocated[group] {
			entry := CandidateEntry{Callsign: e.flight.Callsign, RunwayGroupID: group, Sequence: i + 1, Time: e.time, OperationalTETA: e.flight.OperationalTETA, WakeCategory: e.flight.category, FreezeReason: e.flight.FreezeReason, Protected: isRetained(e.flight), Reason: e.reason}
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

func isRetained(f preparedFlight) bool {
	return f.RebuildRetained || f.FreezeReason != aman.FreezeNone
}
