package sequence

import (
	"slices"
	"sort"

	"FlightStrips/internal/aman"
)

// ApplySwap exchanges positions atomically, retaining the order of every other
// flight. The selected flights may override their automatic Superstable slot
// captures; manual/TMA freezes and all other aircraft's captures remain binding.
// Timings are regenerated under the existing spacing and capacity constraints,
// so exchanging positions does not promise identical slot times.
func ApplySwap(input Input, command aman.SwapFlightsCommand) (Decision, error) {
	if err := command.Validate(); err != nil {
		return Decision{}, err
	}
	if err := validateMetadata(command.Metadata, input.Revision); err != nil {
		return Decision{}, err
	}
	working := cloneInput(input)
	_, first, err := activeFlight(working, command.Callsign)
	if err != nil {
		return Decision{}, err
	}
	_, second, err := activeFlight(working, command.OtherCallsign)
	if err != nil {
		return Decision{}, err
	}
	if first.RunwayGroupID != second.RunwayGroupID || first.SelectedSTARFamily == "" || first.SelectedSTARFamily != second.SelectedSTARFamily {
		return Decision{}, invalidArgument("swap requires flights on the same selected STAR family and runway group")
	}
	if !swapFreezeAllowed(first.FreezeReason) || !swapFreezeAllowed(second.FreezeReason) {
		return Decision{}, invalidTransition("swap cannot override manual or TMA freezes")
	}
	current, err := Generate(working)
	if err != nil {
		return Decision{}, err
	}
	if current.HasConflicts() {
		return Decision{}, invalidTransition("swap cannot start from a conflicting protected sequence")
	}
	order := groupOrder(current, first.RunwayGroupID)
	// The operator swaps the displayed, committed positions. A fresh automatic
	// candidate may already differ because predictions changed in the meantime.
	// Keep uncommitted new arrivals after the existing sequence.
	sort.SliceStable(order, func(i, j int) bool {
		a := working.Flights[flightIndex(working.Flights, order[i])].CurrentSlot
		b := working.Flights[flightIndex(working.Flights, order[j])].CurrentSlot
		if a == nil || b == nil {
			return a != nil && b == nil
		}
		if !a.Time.Equal(b.Time) {
			return a.Time.Before(b.Time)
		}
		return a.Sequence < b.Sequence
	})
	from, to := slices.Index(order, first.Callsign), slices.Index(order, second.Callsign)
	if from < 0 || to < 0 || first.CurrentSlot == nil || second.CurrentSlot == nil {
		return Decision{}, invalidTransition("swap requires two committed sequence positions")
	}
	order[from], order[to] = order[to], order[from]
	// Only the explicitly selected automatic captures may move. Temporarily
	// release slot protection for allocation, then preserve the frozen prediction
	// and capture time when recapturing the accepted slots below.
	for _, id := range []aman.Callsign{first.Callsign, second.Callsign} {
		flight := &working.Flights[flightIndex(working.Flights, id)]
		if flight.FreezeReason == aman.FreezeSuperstable {
			flight.FreezeReason = aman.FreezeNone
			flight.FrozenAt, flight.FrozenOperationalTETA, flight.CapturedSlot = nil, nil, nil
		}
	}
	for position, id := range order {
		index := flightIndex(working.Flights, id)
		working.Flights[index].ManualOrder = intPointer(position + 1)
		working.Flights[index].ManualPosition = nil
		if position >= min(from, to) && working.Flights[index].FreezeReason == aman.FreezeNone {
			working.Flights[index].ProtectCurrentSlot = false
		}
	}
	candidate, err := Generate(working)
	if err != nil {
		return Decision{}, invalidTransition("swap cannot satisfy rate and WTC policy: " + err.Error())
	}
	if candidate.HasConflicts() || !slices.Equal(order, groupOrder(candidate, first.RunwayGroupID)) {
		return Decision{}, invalidTransition("swap cannot retain the requested order with protected slots and spacing constraints")
	}
	for i := range candidate.Entries {
		entry := &candidate.Entries[i]
		index := flightIndex(working.Flights, entry.Callsign)
		original := input.Flights[index]
		if (entry.Callsign != first.Callsign && entry.Callsign != second.Callsign) || original.FreezeReason != aman.FreezeSuperstable {
			continue
		}
		flight := &working.Flights[index]
		flight.FreezeReason = original.FreezeReason
		flight.FrozenAt = cloneTime(original.FrozenAt)
		flight.FrozenOperationalTETA = cloneTime(original.FrozenOperationalTETA)
		flight.CapturedSlot = &aman.Slot{Time: entry.Time, RunwayGroupID: entry.RunwayGroupID, Sequence: entry.Sequence, Revision: input.Revision, Reason: string(ReasonFreezeSuperstable)}
		entry.FreezeReason, entry.Protected, entry.Reason = aman.FreezeSuperstable, true, ReasonFreezeSuperstable
	}
	for index := range working.Flights {
		working.Flights[index].ProtectCurrentSlot = input.Flights[index].ProtectCurrentSlot
	}
	retainManualPositions(&working, input, order, first.Callsign, second.Callsign)
	return Decision{Input: working, Candidate: candidate, Changed: true}, nil
}

func swapFreezeAllowed(reason aman.FreezeReason) bool {
	return reason == aman.FreezeNone || reason == aman.FreezeSuperstable
}
