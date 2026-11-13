package sequence

import (
	"fmt"
	"slices"

	"FlightStrips/internal/aman"
)

// Resolve local controller relationships across the active runway sequence.
// The transitive predecessors also keep holding priority from contradicting a
// requested order through an intervening flight.
func prepareManualPositions(flights []preparedFlight) error {
	if !slices.ContainsFunc(flights, func(f preparedFlight) bool { return f.ManualPosition != nil }) {
		return nil
	}
	active := map[aman.Callsign]int{}
	for i, flight := range flights {
		if flight.State != aman.StateLanded && flight.State != aman.StateRemoved {
			active[flight.Callsign] = i
		}
	}
	direct := make([][]int, len(flights))
	for id, i := range active {
		position := flights[i].ManualPosition
		if position == nil {
			continue
		}
		if position.AfterCallsign == id || position.BeforeCallsign == id {
			return fmt.Errorf("flight %q has a self-referencing manual position", id)
		}
		if previous, ok := active[position.AfterCallsign]; ok {
			direct[i] = append(direct[i], previous)
		}
		if next, ok := active[position.BeforeCallsign]; ok {
			direct[next] = append(direct[next], i)
		}
	}
	visiting, visited := make([]bool, len(flights)), make([]bool, len(flights))
	var visit func(int) error
	visit = func(i int) error {
		if visiting[i] {
			return fmt.Errorf("cyclic manual sequence position at flight %q", flights[i].Callsign)
		}
		if visited[i] {
			return nil
		}
		visiting[i] = true
		predecessors := map[aman.Callsign]bool{}
		for _, previous := range direct[i] {
			if err := visit(previous); err != nil {
				return err
			}
			predecessors[flights[previous].Callsign] = true
			for id := range flights[previous].positionPredecessors {
				predecessors[id] = true
			}
		}
		flights[i].positionPredecessors = predecessors
		visiting[i], visited[i] = false, true
		return nil
	}
	for i := range flights {
		if err := visit(i); err != nil {
			return err
		}
	}
	return resolveManualHoldingOrder(flights)
}

// Add holding FIFO after the controller relationships, rejecting any edge
// which closes a cycle through other arrivals. Visit the physical holding
// order first so unaffected older entrants keep their priority.
func resolveManualHoldingOrder(flights []preparedFlight) error {
	// Keep controller relationships separate from the resolved allocation
	// order: gap filling may relax holding admission priority, but cannot
	// relax a controller's before/after relationship.
	for i := range flights {
		flights[i].orderPredecessors = make(map[aman.Callsign]bool, len(flights[i].positionPredecessors))
		for id := range flights[i].positionPredecessors {
			flights[i].orderPredecessors[id] = true
		}
	}
	holding := make([]preparedFlight, 0, len(flights))
	for _, flight := range flights {
		if flight.State != aman.StateLanded && flight.State != aman.StateRemoved {
			flight.positionPredecessors = nil
			flight.orderPredecessors = nil
			holding = append(holding, flight)
		}
	}
	// This pass orders holding candidates before merging their FIFO edges
	// with controller relationships. Those relationships are absent from the
	// copies above, so automatic STAR edges would lose their manual exceptions.
	ordered, err := orderFlightsByPrecedence(holding, false)
	if err != nil {
		return err
	}
	byID := make(map[aman.Callsign]int, len(flights))
	for i, flight := range flights {
		byID[flight.Callsign] = i
	}
	for _, older := range ordered {
		for _, later := range ordered {
			if !HoldingPrecedes(older.Flight, later.Flight) ||
				(later.ManualOrder != nil && (older.ManualOrder == nil || *later.ManualOrder < *older.ManualOrder)) ||
				(later.stableOrder != nil && older.stableOrder != nil && *later.stableOrder < *older.stableOrder) {
				continue
			}
			leading, trailing := flights[byID[older.Callsign]], flights[byID[later.Callsign]]
			if orderPrecedes(trailing, leading) || orderPrecedes(leading, trailing) {
				continue
			}
			for i := range flights {
				if flights[i].Callsign != trailing.Callsign && !orderPrecedes(trailing, flights[i]) {
					continue
				}
				if flights[i].orderPredecessors == nil {
					flights[i].orderPredecessors = map[aman.Callsign]bool{}
				}
				flights[i].orderPredecessors[leading.Callsign] = true
				for id := range leading.orderPredecessors {
					flights[i].orderPredecessors[id] = true
				}
			}
		}
	}
	return nil
}

func positionPrecedes(a, b preparedFlight) bool {
	return b.positionPredecessors[a.Callsign]
}

func orderPrecedes(a, b preparedFlight) bool {
	return b.orderPredecessors[a.Callsign]
}

// ManualPositionPrecedence includes holding priorities compatible with the
// local controller order. Operational repair must use the same resolved graph
// as slot allocation rather than restoring holding edges rejected for cycles.
func ManualPositionPrecedence(input Input) (map[aman.Callsign]map[aman.Callsign]bool, error) {
	if !slices.ContainsFunc(input.Flights, func(f Flight) bool { return f.ManualPosition != nil }) {
		return nil, nil
	}
	_, groups, err := prepareInput(input)
	if err != nil {
		return nil, err
	}
	result := make(map[aman.Callsign]map[aman.Callsign]bool, len(input.Flights))
	for _, flights := range groups {
		for _, next := range flights {
			result[next.Callsign] = next.orderPredecessors
		}
	}
	return result, nil
}

// ForgetManualPosition detaches a flight from all retained local relationships
// before it is reintroduced into the sequence, including after a go-around.
func ForgetManualPosition(input *Input, id aman.Callsign) {
	for i := range input.Flights {
		flight := &input.Flights[i]
		if flight.ManualPosition == nil {
			continue
		}
		position := *flight.ManualPosition
		if position.AfterCallsign == id {
			position.AfterCallsign = ""
		}
		if position.BeforeCallsign == id {
			position.BeforeCallsign = ""
		}
		flight.ManualPosition = &position
		if flight.Callsign == id || (position.AfterCallsign == "" && position.BeforeCallsign == "") {
			flight.ManualPosition = nil
		}
	}
}

// A controller command uses a complete temporary order for atomic placement.
// Persist just its local relationships and compatible prior controller intent.
func retainManualPositions(working *Input, original Input, order []aman.Callsign, selected ...aman.Callsign) {
	positions := make(map[aman.Callsign]int, len(order))
	for i, id := range order {
		positions[id] = i
	}
	for i := range working.Flights {
		flight := &working.Flights[i]
		at, exists := positions[flight.Callsign]
		if !exists {
			continue
		}
		flight.ManualOrder = cloneInt(original.Flights[i].ManualOrder)
		if flight.ManualOrder != nil {
			flight.ManualOrder = intPointer(at + 1)
		}
		position := aman.ManualSequencePosition{}
		if previous := original.Flights[i].ManualPosition; previous != nil {
			position = *previous
		}
		if slices.Contains(selected, flight.Callsign) {
			flight.ManualOrder = nil
			position = aman.ManualSequencePosition{}
			if at > 0 {
				position.AfterCallsign = order[at-1]
			}
			if at+1 < len(order) {
				position.BeforeCallsign = order[at+1]
			}
		} else {
			if previous, ok := positions[position.AfterCallsign]; !ok || previous >= at {
				position.AfterCallsign = ""
			}
			if next, ok := positions[position.BeforeCallsign]; !ok || next <= at {
				position.BeforeCallsign = ""
			}
		}
		flight.ManualPosition = nil
		if position.AfterCallsign != "" || position.BeforeCallsign != "" {
			flight.ManualPosition = &position
		}
	}
}
