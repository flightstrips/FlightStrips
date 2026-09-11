package aman

import (
	"sort"
	"time"
)

// RunwayClosureIntervalInput requires exactly one start form; nil End is indefinite.
type RunwayClosureIntervalInput struct {
	RunwayGroupID RunwayGroupID
	Start         *time.Time
	AfterFlightID *FlightID
	End           *time.Time
}

// RunwayClosureInterval is an immutable normalized [Start, End) value.
type RunwayClosureInterval struct {
	start time.Time
	end   *time.Time
}

func (i RunwayClosureInterval) Start() time.Time { return i.start }

// End returns the exclusive UTC boundary, or nil when indefinite.
func (i RunwayClosureInterval) End() *time.Time {
	if i.end == nil {
		return nil
	}
	value := *i.end
	return &value
}

// NormalizeRunwayClosureInterval resolves absolute input or the first unblocked grid opportunity strictly after an anchor's current authoritative slot.
func NormalizeRunwayClosureInterval(input RunwayClosureIntervalInput, state AirportState) (RunwayClosureInterval, error) {
	if !isTrimmedNonEmpty(string(input.RunwayGroupID)) {
		return RunwayClosureInterval{}, invalid("runway closure runway group is required")
	}
	if (input.Start == nil) == (input.AfterFlightID == nil) {
		return RunwayClosureInterval{}, invalid("runway closure requires exactly one absolute or after-aircraft start")
	}

	group, err := closureRunwayGroup(state.RunwayGroups, input.RunwayGroupID)
	if err != nil {
		return RunwayClosureInterval{}, err
	}
	start := time.Time{}
	if input.Start != nil {
		if err := requireUTCTime("runway closure start", *input.Start); err != nil {
			return RunwayClosureInterval{}, err
		}
		start = *input.Start
	} else {
		flight, err := closureAnchorFlight(state.Flights, *input.AfterFlightID)
		if err != nil {
			return RunwayClosureInterval{}, err
		}
		if flight.SelectedRunwayGroup == nil || flight.Slot == nil {
			return RunwayClosureInterval{}, invalid("runway closure anchor is not assigned")
		}
		if *flight.SelectedRunwayGroup != input.RunwayGroupID || flight.Slot.RunwayGroupID != input.RunwayGroupID {
			return RunwayClosureInterval{}, invalid("runway closure anchor belongs to another runway group")
		}
		if flight.Slot.Revision != state.Revision {
			return RunwayClosureInterval{}, invalid("runway closure anchor slot is stale")
		}
		if err := requireUTCTime("runway closure anchor slot", flight.Slot.Time); err != nil {
			return RunwayClosureInterval{}, err
		}
		start, err = nextClosureOpportunity(*group, flight.Slot.Time)
		if err != nil {
			return RunwayClosureInterval{}, err
		}
	}

	var end *time.Time
	if input.End != nil {
		if err := requireUTCTime("runway closure end", *input.End); err != nil {
			return RunwayClosureInterval{}, err
		}
		if !start.Before(*input.End) {
			return RunwayClosureInterval{}, invalid("runway closure start must be before end")
		}
		value := *input.End
		end = &value
	}
	return RunwayClosureInterval{start: start, end: end}, nil
}

func closureRunwayGroup(groups []RunwayGroupPolicy, id RunwayGroupID) (*RunwayGroupPolicy, error) {
	for index := range groups {
		if groups[index].ID == id {
			return &groups[index], nil
		}
	}
	return nil, invalid("runway closure runway group does not exist")
}

func closureAnchorFlight(flights []AMANFlight, id FlightID) (*AMANFlight, error) {
	if !isTrimmedNonEmpty(string(id)) {
		return nil, invalid("runway closure anchor aircraft is required")
	}
	for index := range flights {
		if flights[index].ID == id {
			return &flights[index], nil
		}
	}
	return nil, invalid("runway closure anchor aircraft does not exist")
}

func nextClosureOpportunity(group RunwayGroupPolicy, after time.Time) (time.Time, error) {
	rates := group.RateSchedule
	if len(rates) == 0 && group.RateEffectiveAt != nil && group.ActiveRatePerHour > 0 {
		rates = []RunwayGroupRatePoint{{EffectiveAt: *group.RateEffectiveAt, ArrivalsPerHour: group.ActiveRatePerHour}}
	}
	for index, rate := range rates {
		if rate.ArrivalsPerHour == 0 || rate.EffectiveAt.Location() != time.UTC || rate.EffectiveAt.IsZero() {
			return time.Time{}, invalid("runway closure runway grid is invalid")
		}
		if index+1 < len(rates) && !after.Before(rates[index+1].EffectiveAt) {
			continue
		}
		candidate := rate.EffectiveAt
		if !after.Before(candidate) {
			interval := time.Duration((uint64(time.Hour) + uint64(rate.ArrivalsPerHour) - 1) / uint64(rate.ArrivalsPerHour))
			delta := after.Sub(candidate)
			steps := delta/interval + 1
			if steps > time.Duration(1<<63-1)/interval {
				return time.Time{}, invalid("runway closure next opportunity exceeds the supported range")
			}
			candidate = candidate.Add(steps * interval)
			if !candidate.After(after) {
				return time.Time{}, invalid("runway closure next opportunity exceeds the supported range")
			}
		}
		if index+1 < len(rates) && !candidate.Before(rates[index+1].EffectiveAt) {
			continue
		}
		for gapIndex := sort.Search(len(group.Gaps), func(i int) bool { return group.Gaps[i].End.After(candidate) }); gapIndex < len(group.Gaps); gapIndex++ {
			gap := group.Gaps[gapIndex]
			if candidate.Before(gap.Start) {
				break
			}
			if candidate.Before(gap.End) {
				return nextClosureOpportunity(group, gap.End.Add(-time.Nanosecond))
			}
		}
		return candidate, nil
	}
	return time.Time{}, invalid("runway closure runway has no later grid opportunity")
}
