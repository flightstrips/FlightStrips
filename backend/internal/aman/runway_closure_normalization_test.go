package aman

import (
	"errors"
	"testing"
	"time"
)

func TestNormalizeRunwayClosureIntervalAbsoluteFiniteAndIndefinite(t *testing.T) {
	start := closureNormalizationTime()
	end := start.Add(10 * time.Minute)
	state := closureNormalizationState(start)

	finite, err := NormalizeRunwayClosureInterval(RunwayClosureIntervalInput{
		RunwayGroupID: "north", Start: &start, End: &end,
	}, state)
	if err != nil {
		t.Fatalf("normalize finite closure: %v", err)
	}
	if finite.Start() != start || finite.End() == nil || *finite.End() != end {
		t.Fatalf("finite interval = [%s, %v), want [%s, %s)", finite.Start(), finite.End(), start, end)
	}

	indefinite, err := NormalizeRunwayClosureInterval(RunwayClosureIntervalInput{
		RunwayGroupID: "north", Start: &start,
	}, state)
	if err != nil {
		t.Fatalf("normalize indefinite closure: %v", err)
	}
	if indefinite.Start() != start || indefinite.End() != nil {
		t.Fatalf("indefinite interval = [%s, %v), want [%s, nil)", indefinite.Start(), indefinite.End(), start)
	}
}

func TestNormalizeRunwayClosureIntervalAfterAircraftUsesExactNextGridOpportunity(t *testing.T) {
	base := closureNormalizationTime()
	state := closureNormalizationState(base)
	flightID := FlightID("SAS101")
	groupID := RunwayGroupID("north")
	state.Flights = []AMANFlight{{
		ID: flightID, SelectedRunwayGroup: &groupID,
		Slot: &Slot{Time: base.Add(3 * time.Minute), RunwayGroupID: groupID, Sequence: 2, Revision: state.Revision},
	}}

	interval, err := NormalizeRunwayClosureInterval(RunwayClosureIntervalInput{
		RunwayGroupID: groupID, AfterFlightID: &flightID,
	}, state)
	if err != nil {
		t.Fatalf("normalize after-aircraft closure: %v", err)
	}
	if want := base.Add(6 * time.Minute); interval.Start() != want {
		t.Fatalf("start = %s, want exact next 20/hour opportunity %s", interval.Start(), want)
	}
}

func TestNormalizeRunwayClosureIntervalHonorsRateAndGapBoundaries(t *testing.T) {
	base := closureNormalizationTime()
	state := closureNormalizationState(base)
	state.RunwayGroups[0].RateSchedule = []RunwayGroupRatePoint{
		{EffectiveAt: base, ArrivalsPerHour: 20},
		{EffectiveAt: base.Add(10 * time.Minute), ArrivalsPerHour: 30},
	}
	state.RunwayGroups[0].Gaps = []RunwayGap{{
		ID: "gap", Start: base.Add(10 * time.Minute), End: base.Add(14 * time.Minute),
		Label: "inspection", CreatedAt: base, CreatedBy: "controller",
	}}
	flightID := FlightID("SAS102")
	groupID := RunwayGroupID("north")
	state.Flights = []AMANFlight{{
		ID: flightID, SelectedRunwayGroup: &groupID,
		Slot: &Slot{Time: base.Add(9 * time.Minute), RunwayGroupID: groupID, Sequence: 4, Revision: state.Revision},
	}}

	interval, err := NormalizeRunwayClosureInterval(RunwayClosureIntervalInput{
		RunwayGroupID: groupID, AfterFlightID: &flightID,
	}, state)
	if err != nil {
		t.Fatalf("normalize across rate and GAP boundaries: %v", err)
	}
	if want := base.Add(14 * time.Minute); interval.Start() != want {
		t.Fatalf("start = %s, want first open new-rate opportunity %s", interval.Start(), want)
	}

	state.RunwayGroups[0].Gaps = nil
	state.Flights[0].Slot.Time = base.Add(10 * time.Minute)
	interval, err = NormalizeRunwayClosureInterval(RunwayClosureIntervalInput{
		RunwayGroupID: groupID, AfterFlightID: &flightID,
	}, state)
	if err != nil {
		t.Fatalf("normalize from exact rate boundary: %v", err)
	}
	if want := base.Add(12 * time.Minute); interval.Start() != want {
		t.Fatalf("start = %s, want opportunity strictly after boundary %s", interval.Start(), want)
	}
}

func TestNormalizeRunwayClosureIntervalRejectsInvalidInputs(t *testing.T) {
	base := closureNormalizationTime()
	end := base.Add(time.Minute)
	localStart := base.In(time.FixedZone("CEST", 2*60*60))
	flightID := FlightID("SAS103")
	groupID := RunwayGroupID("north")
	valid := closureNormalizationState(base)
	valid.Flights = []AMANFlight{{
		ID: flightID, SelectedRunwayGroup: &groupID,
		Slot: &Slot{Time: base, RunwayGroupID: groupID, Sequence: 1, Revision: valid.Revision},
	}}

	tests := map[string]struct {
		input  RunwayClosureIntervalInput
		mutate func(*AirportState)
	}{
		"missing start":          {input: RunwayClosureIntervalInput{RunwayGroupID: groupID}},
		"both starts":            {input: RunwayClosureIntervalInput{RunwayGroupID: groupID, Start: &base, AfterFlightID: &flightID}},
		"non-UTC absolute start": {input: RunwayClosureIntervalInput{RunwayGroupID: groupID, Start: &localStart}},
		"missing anchor":         {input: RunwayClosureIntervalInput{RunwayGroupID: groupID, AfterFlightID: flightIDPointer("MISSING")}},
		"unassigned anchor": {
			input:  RunwayClosureIntervalInput{RunwayGroupID: groupID, AfterFlightID: &flightID},
			mutate: func(state *AirportState) { state.Flights[0].Slot = nil },
		},
		"wrong-runway anchor": {
			input: RunwayClosureIntervalInput{RunwayGroupID: "south", AfterFlightID: &flightID},
		},
		"stale anchor": {
			input:  RunwayClosureIntervalInput{RunwayGroupID: groupID, AfterFlightID: &flightID},
			mutate: func(state *AirportState) { state.Flights[0].Slot.Revision-- },
		},
		"zero end":     {input: RunwayClosureIntervalInput{RunwayGroupID: groupID, Start: &base, End: timePointer(time.Time{})}},
		"equal end":    {input: RunwayClosureIntervalInput{RunwayGroupID: groupID, Start: &base, End: &base}},
		"reversed end": {input: RunwayClosureIntervalInput{RunwayGroupID: groupID, Start: &base, End: timePointer(base.Add(-time.Second))}},
		"non-UTC end": {
			input: RunwayClosureIntervalInput{RunwayGroupID: groupID, Start: &base, End: timePointer(end.In(time.FixedZone("CEST", 2*60*60)))},
		},
		"missing runway grid": {
			input: RunwayClosureIntervalInput{RunwayGroupID: groupID, AfterFlightID: &flightID},
			mutate: func(state *AirportState) {
				state.RunwayGroups[0].RateSchedule, state.RunwayGroups[0].RateEffectiveAt = nil, nil
			},
		},
		"opportunity overflow": {
			input: RunwayClosureIntervalInput{RunwayGroupID: groupID, AfterFlightID: &flightID},
			mutate: func(state *AirportState) {
				state.RunwayGroups[0].RateSchedule = []RunwayGroupRatePoint{{EffectiveAt: time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC), ArrivalsPerHour: 1}}
				state.Flights[0].Slot.Time = time.Date(1000, 1, 1, 0, 0, 0, 0, time.UTC)
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			state := valid
			state.Flights = append([]AMANFlight(nil), valid.Flights...)
			state.RunwayGroups = append([]RunwayGroupPolicy(nil), valid.RunwayGroups...)
			if test.mutate != nil {
				test.mutate(&state)
			}
			_, err := NormalizeRunwayClosureInterval(test.input, state)
			var domain *DomainError
			if !errors.As(err, &domain) || domain.Class != ErrorInvalidArgument {
				t.Fatalf("error = %v, want invalid_argument domain error", err)
			}
		})
	}
}

func closureNormalizationState(base time.Time) AirportState {
	effective := base
	return AirportState{
		Revision: 7,
		RunwayGroups: []RunwayGroupPolicy{
			{ID: "north", ActiveRatePerHour: 20, RateEffectiveAt: &effective},
			{ID: "south", ActiveRatePerHour: 20, RateEffectiveAt: &effective},
		},
	}
}

func closureNormalizationTime() time.Time {
	return time.Date(2026, time.September, 12, 12, 0, 0, 0, time.UTC)
}

func flightIDPointer(value FlightID) *FlightID { return &value }
