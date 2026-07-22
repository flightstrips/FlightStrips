package sequence

import (
	"fmt"
	"slices"
	"sort"
	"time"

	"FlightStrips/internal/aman"
)

// Decision is the complete, uncommitted result of one pure sequence-policy
// operation. #323 owns revision allocation, persistence, and publication.
type Decision struct {
	Input     Input
	Candidate Result
	Changed   bool
}

func ApplyMove(input Input, command MoveFlightCommand) (Decision, error) {
	if err := command.Validate(input.Revision); err != nil {
		return Decision{}, err
	}
	working := cloneInput(input)
	targetIndex, target, err := activeFlight(working, command.FlightID)
	if err != nil {
		return Decision{}, err
	}
	anchorID := command.BeforeFlightID
	if anchorID == nil {
		anchorID = command.AfterFlightID
	}
	_, anchor, err := activeFlight(working, *anchorID)
	if err != nil {
		return Decision{}, err
	}
	if target.RunwayGroupID != command.RunwayGroupID || anchor.RunwayGroupID != command.RunwayGroupID {
		return Decision{}, invalidArgument("move flight and anchor must belong to the requested runway group")
	}
	if target.FreezeReason != aman.FreezeNone {
		return Decision{}, invalidTransition("move requires the flight freeze to be released first")
	}

	current, err := Generate(working)
	if err != nil {
		return Decision{}, err
	}
	if current.HasConflicts() {
		return Decision{}, invalidTransition("move cannot start from a conflicting protected sequence")
	}
	order := groupOrder(current, command.RunwayGroupID)
	from, anchorAt := slices.Index(order, command.FlightID), slices.Index(order, *anchorID)
	if from < 0 || anchorAt < 0 {
		return Decision{}, notFound("move flight or anchor has no active slot")
	}
	order = slices.Delete(order, from, from+1)
	anchorAt = slices.Index(order, *anchorID)
	insertAt := anchorAt
	if command.AfterFlightID != nil {
		insertAt++
	}
	order = slices.Insert(order, insertAt, command.FlightID)

	for position, id := range order {
		index := flightIndex(working.Flights, id)
		value := position + 1
		working.Flights[index].ManualOrder = &value
	}
	working.Flights[targetIndex].ManualOrder = intPointer(slices.Index(order, command.FlightID) + 1)

	candidate, err := Generate(working)
	if err != nil {
		return Decision{}, invalidTransition("move cannot satisfy rate and WTC policy: " + err.Error())
	}
	if candidate.HasConflicts() {
		return Decision{}, invalidTransition("move would conflict with a protected slot")
	}
	wantOrder := groupOrder(candidate, command.RunwayGroupID)
	targetAt, finalAnchorAt := slices.Index(wantOrder, command.FlightID), slices.Index(wantOrder, *anchorID)
	validAnchor := command.BeforeFlightID != nil && targetAt+1 == finalAnchorAt || command.AfterFlightID != nil && finalAnchorAt+1 == targetAt
	if !validAnchor {
		return Decision{}, invalidTransition("move cannot satisfy the requested anchor under rate and WTC policy")
	}
	return Decision{Input: working, Candidate: candidate, Changed: !inputsEqualOrder(input, working) || len(candidate.Movements) > 0}, nil
}

func ApplyManualFreeze(input Input, command ApplyManualFreezeCommand) (Decision, error) {
	if err := command.Validate(input.Revision); err != nil {
		return Decision{}, err
	}
	working := cloneInput(input)
	index, flight, err := activeFlight(working, command.FlightID)
	if err != nil {
		return Decision{}, err
	}
	if flight.State == aman.StateGoAround {
		return Decision{}, invalidTransition("go-around flight cannot be manually frozen")
	}
	if flight.FreezeReason != aman.FreezeNone {
		return Decision{}, invalidTransition("manual freeze requires an unfrozen flight")
	}
	if flight.CurrentSlot == nil {
		return Decision{}, invalidTransition("manual freeze requires a committed current slot")
	}
	at, operational := command.At, flight.OperationalTETA
	working.Flights[index].FreezeReason = aman.FreezeManual
	working.Flights[index].FrozenAt = &at
	working.Flights[index].FrozenOperationalTETA = &operational
	working.Flights[index].CapturedSlot = cloneSlot(flight.CurrentSlot)
	candidate, err := Generate(working)
	if err != nil {
		return Decision{}, err
	}
	return Decision{Input: working, Candidate: candidate, Changed: true}, nil
}

func ReleaseManualFreeze(input Input, command ReleaseManualFreezeCommand) (Decision, error) {
	if err := command.Validate(input.Revision); err != nil {
		return Decision{}, err
	}
	working := cloneInput(input)
	index, flight, err := activeFlight(working, command.FlightID)
	if err != nil {
		return Decision{}, err
	}
	if flight.FreezeReason != aman.FreezeManual {
		return Decision{}, invalidTransition("manual freeze release requires FreezeManual")
	}
	working.Flights[index].FreezeReason = aman.FreezeNone
	working.Flights[index].FrozenAt = nil
	working.Flights[index].FrozenOperationalTETA = nil
	working.Flights[index].CapturedSlot = nil
	candidate, err := Generate(working)
	if err != nil {
		return Decision{}, err
	}
	return Decision{Input: working, Candidate: candidate, Changed: true}, nil
}

func ApplyRate(input Input, command SetRateCommand) (Decision, error) {
	if err := command.Validate(input.Revision); err != nil {
		return Decision{}, err
	}
	working := cloneInput(input)
	policyIndex := -1
	for index := range working.Policies {
		if working.Policies[index].RunwayGroupID == command.RunwayGroupID {
			policyIndex = index
			break
		}
	}
	if policyIndex < 0 {
		return Decision{}, notFound("rate runway group was not found")
	}
	rates := working.Policies[policyIndex].Rates
	changed := true
	replaced := false
	for index := range rates {
		if rates[index].EffectiveAt.Equal(command.EffectiveAt) {
			changed = rates[index].ArrivalsPerHour != command.ArrivalsPerHour
			rates[index].ArrivalsPerHour = command.ArrivalsPerHour
			replaced = true
			break
		}
	}
	if !replaced {
		rates = append(rates, RatePoint{EffectiveAt: command.EffectiveAt, ArrivalsPerHour: command.ArrivalsPerHour})
	}
	sort.Slice(rates, func(i, j int) bool { return rates[i].EffectiveAt.Before(rates[j].EffectiveAt) })
	working.Policies[policyIndex].Rates = rates
	candidate, err := Generate(working)
	if err != nil {
		return Decision{}, err
	}
	return Decision{Input: working, Candidate: candidate, Changed: changed || len(candidate.Movements) > 0}, nil
}

func ApplyGoAround(input Input, policy GoAroundPolicy, command ApplyGoAroundCommand) (Decision, error) {
	if err := policy.Validate(); err != nil {
		return Decision{}, err
	}
	if err := command.Validate(input.Revision); err != nil {
		return Decision{}, err
	}
	working := cloneInput(input)
	targetIndex, target, err := activeFlight(working, command.FlightID)
	if err != nil {
		return Decision{}, err
	}
	if target.State != aman.StateGoAround {
		return Decision{}, invalidTransition("go-around sequence policy requires StateGoAround")
	}
	if target.CurrentSlot == nil {
		return Decision{}, invalidTransition("go-around sequence policy requires a current slot")
	}
	policies, err := preparePolicies(working.Policies)
	if err != nil {
		return Decision{}, err
	}
	prepared, err := prepareFlights(working.Flights, policies)
	if err != nil {
		return Decision{}, err
	}
	groupPolicy := policies[target.RunwayGroupID]
	groupFlights := prepared[target.RunwayGroupID]
	var moving preparedFlight
	entries := make([]allocatedEntry, 0, len(groupFlights))
	for _, flight := range groupFlights {
		if flight.ID == target.ID {
			moving = flight
			continue
		}
		if flight.State == aman.StateLanded || flight.State == aman.StateRemoved {
			continue
		}
		if flight.CurrentSlot == nil {
			return Decision{}, invalidTransition(fmt.Sprintf("go-around cascade requires current slot for flight %q", flight.ID))
		}
		slot := flight.CurrentSlot
		if flight.FreezeReason != aman.FreezeNone {
			if flight.CapturedSlot == nil {
				return Decision{}, invalidTransition(fmt.Sprintf("frozen flight %q has no captured slot", flight.ID))
			}
			slot = flight.CapturedSlot
		}
		entries = append(entries, allocatedEntry{flight: flight, time: slot.Time, reason: protectedReason(flight)})
	}
	moving.FreezeReason = aman.FreezeNone
	moving.CapturedSlot = nil
	moving.FrozenAt = nil
	moving.FrozenOperationalTETA = nil
	moving.ManualOrder = nil
	sortEntries(entries)

	earliest := command.DetectedAt.Add(policy.Delay)
	var allocated []allocatedEntry
	var cascaded map[aman.FlightID]struct{}
	for attempts := 0; attempts <= len(entries)+1; attempts++ {
		candidate, ok := nextGridAtOrAfter(groupPolicy, earliest)
		if !ok {
			return Decision{}, invalidTransition("go-around has no rate slot at or after its target")
		}
		allocated, cascaded, earliest, err = cascadeGoAround(groupPolicy, entries, moving, candidate, policy.MaxCascade)
		if err != nil {
			return Decision{}, err
		}
		if allocated != nil {
			break
		}
	}
	if allocated == nil {
		return Decision{}, invalidTransition("go-around could not pass a protected manual slot")
	}

	working.Flights[targetIndex].FreezeReason = aman.FreezeNone
	working.Flights[targetIndex].FrozenAt = nil
	working.Flights[targetIndex].FrozenOperationalTETA = nil
	working.Flights[targetIndex].CapturedSlot = nil
	working.Flights[targetIndex].ManualOrder = nil

	result, err := resultWithGoAroundGroup(working, target.RunwayGroupID, allocated, target.ID)
	if err != nil {
		return Decision{}, err
	}
	for _, entry := range result.Entries {
		if entry.RunwayGroupID != target.RunwayGroupID {
			continue
		}
		index := flightIndex(working.Flights, entry.FlightID)
		working.Flights[index].ManualOrder = intPointer(entry.Sequence)
	}
	for _, entry := range result.Entries {
		if _, moved := cascaded[entry.FlightID]; !moved {
			continue
		}
		index := flightIndex(working.Flights, entry.FlightID)
		if working.Flights[index].FreezeReason == aman.FreezeSuperstable {
			working.Flights[index].CapturedSlot = &aman.Slot{Time: entry.Time, RunwayGroupID: entry.RunwayGroupID, Sequence: entry.Sequence, Revision: input.Revision, Reason: string(entry.Reason)}
		}
	}
	return Decision{Input: working, Candidate: result, Changed: true}, nil
}

func cascadeGoAround(policy preparedPolicy, current []allocatedEntry, moving preparedFlight, candidate time.Time, limit int) ([]allocatedEntry, map[aman.FlightID]struct{}, time.Time, error) {
	index := sort.Search(len(current), func(i int) bool { return !current[i].time.Before(candidate) })
	if index > 0 {
		leading := current[index-1]
		required := requiredGap(policy, leading.flight, moving, candidate)
		if candidate.Sub(leading.time) < required {
			return nil, nil, leading.time.Add(required), nil
		}
	}
	result := append([]allocatedEntry(nil), current[:index]...)
	last := allocatedEntry{flight: moving, time: candidate, reason: ReasonGoAround}
	result = append(result, last)
	cascaded := map[aman.FlightID]struct{}{}
	for position := index; position < len(current); position++ {
		next := current[position]
		if adjacentValid(policy, last, next) {
			result = append(result, current[position:]...)
			return result, cascaded, time.Time{}, nil
		}
		if next.flight.FreezeReason == aman.FreezeManual {
			required := requiredGap(policy, next.flight, moving, next.time)
			return nil, nil, next.time.Add(required), nil
		}
		if len(cascaded) >= limit {
			return nil, nil, time.Time{}, invalidTransition("go-around cascade bound exceeded")
		}
		nextTime, ok := nextGridAtOrAfter(policy, last.time.Add(requiredGap(policy, last.flight, next.flight, last.time)))
		if !ok {
			return nil, nil, time.Time{}, invalidTransition("go-around cascade has no following rate slot")
		}
		for !adjacentValid(policy, last, allocatedEntry{flight: next.flight, time: nextTime}) {
			nextTime, ok = nextGridAtOrAfter(policy, nextTime.Add(time.Nanosecond))
			if !ok {
				return nil, nil, time.Time{}, invalidTransition("go-around cascade has no WTC-valid following slot")
			}
		}
		next.time = nextTime
		next.reason = ReasonGoAroundCascade
		result = append(result, next)
		last = next
		cascaded[next.flight.ID] = struct{}{}
	}
	return result, cascaded, time.Time{}, nil
}

func resultWithGoAroundGroup(input Input, group aman.RunwayGroupID, allocated []allocatedEntry, target aman.FlightID) (Result, error) {
	result := Result{Entries: []CandidateEntry{}, Movements: []SlotMovement{}, Warnings: []Warning{}}
	for index, entry := range allocated {
		candidate := CandidateEntry{FlightID: entry.flight.ID, RunwayGroupID: group, Sequence: index + 1, Time: entry.time, OperationalTETA: entry.flight.OperationalTETA, WakeCategory: entry.flight.category, FreezeReason: entry.flight.FreezeReason, Protected: entry.flight.FreezeReason != aman.FreezeNone, Reason: entry.reason}
		if entry.flight.ID == target {
			candidate.FreezeReason, candidate.Protected, candidate.Reason = aman.FreezeNone, false, ReasonGoAround
		}
		result.Entries = append(result.Entries, candidate)
		if movement := movementFor(entry.flight, candidate); movement != nil {
			result.Movements = append(result.Movements, *movement)
		}
		if !entry.flight.known {
			result.Warnings = append(result.Warnings, Warning{Severity: SeverityDegraded, Code: WarningUnknownWakeCategory, RunwayGroupID: group, FlightID: entry.flight.ID})
		}
	}
	for _, policy := range input.Policies {
		if policy.RunwayGroupID == group {
			continue
		}
		flights := make([]Flight, 0)
		for _, flight := range input.Flights {
			if flight.RunwayGroupID == policy.RunwayGroupID {
				flights = append(flights, flight)
			}
		}
		other, err := Generate(Input{Revision: input.Revision, Policies: []Policy{policy}, Flights: flights})
		if err != nil {
			return Result{}, err
		}
		result.Entries = append(result.Entries, other.Entries...)
		result.Movements = append(result.Movements, other.Movements...)
		result.Warnings = append(result.Warnings, other.Warnings...)
	}
	sort.Slice(result.Entries, func(i, j int) bool {
		if result.Entries[i].RunwayGroupID != result.Entries[j].RunwayGroupID {
			return result.Entries[i].RunwayGroupID < result.Entries[j].RunwayGroupID
		}
		return result.Entries[i].Sequence < result.Entries[j].Sequence
	})
	sort.Slice(result.Movements, func(i, j int) bool {
		if result.Movements[i].RunwayGroupID != result.Movements[j].RunwayGroupID {
			return result.Movements[i].RunwayGroupID < result.Movements[j].RunwayGroupID
		}
		return result.Movements[i].FlightID < result.Movements[j].FlightID
	})
	sortWarnings(result.Warnings)
	return result, nil
}

func cloneInput(input Input) Input {
	copy := Input{Revision: input.Revision, Policies: make([]Policy, len(input.Policies)), Flights: make([]Flight, len(input.Flights))}
	for index, policy := range input.Policies {
		copy.Policies[index] = policy
		copy.Policies[index].Rates = slices.Clone(policy.Rates)
		copy.Policies[index].SeparationRules = slices.Clone(policy.SeparationRules)
	}
	for index, flight := range input.Flights {
		copy.Flights[index] = flight
		copy.Flights[index].InitialBaselineTETA = cloneTime(flight.InitialBaselineTETA)
		copy.Flights[index].ManualOrder = cloneInt(flight.ManualOrder)
		copy.Flights[index].FrozenAt = cloneTime(flight.FrozenAt)
		copy.Flights[index].FrozenOperationalTETA = cloneTime(flight.FrozenOperationalTETA)
		copy.Flights[index].CapturedSlot = cloneSlot(flight.CapturedSlot)
		copy.Flights[index].CurrentSlot = cloneSlot(flight.CurrentSlot)
	}
	return copy
}

func activeFlight(input Input, id aman.FlightID) (int, Flight, error) {
	index := flightIndex(input.Flights, id)
	if index < 0 {
		return -1, Flight{}, notFound(fmt.Sprintf("flight %q was not found", id))
	}
	flight := input.Flights[index]
	if flight.State == aman.StateLanded || flight.State == aman.StateRemoved {
		return -1, Flight{}, invalidTransition(fmt.Sprintf("flight %q is not active", id))
	}
	return index, flight, nil
}

func groupOrder(result Result, group aman.RunwayGroupID) []aman.FlightID {
	values := []aman.FlightID{}
	for _, entry := range result.Entries {
		if entry.RunwayGroupID == group {
			values = append(values, entry.FlightID)
		}
	}
	return values
}

func protectedReason(flight preparedFlight) CandidateReason {
	if flight.FreezeReason == aman.FreezeSuperstable {
		return ReasonFreezeSuperstable
	}
	if flight.FreezeReason == aman.FreezeManual {
		return ReasonFreezeManual
	}
	return ReasonRateWTC
}

func flightIndex(flights []Flight, id aman.FlightID) int {
	return slices.IndexFunc(flights, func(flight Flight) bool { return flight.ID == id })
}

func cloneSlot(value *aman.Slot) *aman.Slot {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func intPointer(value int) *int { return &value }

func inputsEqualOrder(a, b Input) bool {
	if len(a.Flights) != len(b.Flights) {
		return false
	}
	for index := range a.Flights {
		left, right := a.Flights[index].ManualOrder, b.Flights[index].ManualOrder
		if left == nil || right == nil {
			if left != nil || right != nil {
				return false
			}
			continue
		}
		if *left != *right {
			return false
		}
	}
	return true
}
