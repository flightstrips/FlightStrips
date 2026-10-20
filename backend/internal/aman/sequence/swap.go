package sequence

import (
	"slices"
	"sort"

	"FlightStrips/internal/aman"
)

// ApplySwap exchanges positions atomically, retaining the order of every other
// flight. Timings are regenerated under the existing rate, wake, and freeze
// constraints, so exchanging positions does not promise identical slot times.
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
	if first.FreezeReason != aman.FreezeNone || second.FreezeReason != aman.FreezeNone {
		return Decision{}, invalidTransition("swap requires both flight freezes to be released first")
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
	for position, id := range order {
		index := flightIndex(working.Flights, id)
		working.Flights[index].ManualOrder = intPointer(position + 1)
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
	for index := range working.Flights {
		working.Flights[index].ProtectCurrentSlot = input.Flights[index].ProtectCurrentSlot
	}
	return Decision{Input: working, Candidate: candidate, Changed: true}, nil
}
