package sequence

import (
	"FlightStrips/internal/aman"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"slices"
	"testing"
	"time"
)

func rebuildTestInput() (Input, time.Time) {
	at := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	policies := []Policy{}
	for _, group := range []aman.RunwayGroupID{"A", "B"} {
		policies = append(policies, Policy{RunwayGroupID: group, ContinuousSpacing: true, Rates: []RatePoint{{EffectiveAt: at.Add(-time.Hour), ArrivalsPerHour: 30}}, SeparationRules: []SeparationRule{{Leading: "M", Trailing: "M", Minimum: time.Minute}}, UnknownSeparation: 3 * time.Minute})
	}
	return Input{Revision: 7, Policies: policies}, at
}

func TestRebuildFillsSTARSpacingGapWithoutCreatingManualOrders(t *testing.T) {
	input, at := rebuildTestInput()
	input.Policies = input.Policies[:1]
	input.Policies[0].AllowCrossFeederOvertake = true
	input.Policies[0].SameSTARSpacing = SameSTARSpacing{Enabled: true, ActivationRatePerHour: 20, MinimumEmptySlots: 1}
	lead := rebuildHeld("HELD1", "A", "ERNOV", 10000, 20, at, 10*time.Minute)
	tail := rebuildHeld("HELD2", "A", "ERNOV", 12000, 10, at, 10*time.Minute)
	lead.STARFamily, tail.STARFamily = "ERNOV", "ERNOV"
	lead.ActiveHoldingSince = &at
	physical := at.Add(12*time.Minute + time.Second)
	inbound := Flight{Callsign: "INBOUND", RunwayGroupID: "A", State: aman.StateUnstable, FreezeReason: aman.FreezeNone,
		OperationalTETA: physical, PromotionNotBefore: &physical, WakeCategory: "M", STARFamily: "MONAK"}
	input.Flights = []Flight{lead, tail, inbound}
	decision, err := ApplyArrivalSequenceRebuild(input, at)
	require.NoError(t, err)
	require.Equal(t, []aman.Callsign{"HELD1", "INBOUND", "HELD2"}, []aman.Callsign{
		decision.Candidate.Entries[0].Callsign, decision.Candidate.Entries[1].Callsign, decision.Candidate.Entries[2].Callsign})
	require.Equal(t, at.Add(10*time.Minute+time.Second), decision.Candidate.Entries[0].Time)
	require.Equal(t, physical, decision.Candidate.Entries[1].Time)
	require.Equal(t, at.Add(14*time.Minute+time.Second), decision.Candidate.Entries[2].Time)
	require.Equal(t, []aman.Callsign{"HELD1", "HELD2"}, decision.Outcome.ReleaseOrder)
	for _, flight := range decision.Input.Flights {
		require.Nil(t, flight.ManualOrder, "an automatic rebuild must allow future gap filling")
	}
}

func rebuildHeld(id aman.Callsign, group aman.RunwayGroupID, hold string, level, minutes int, at time.Time, transit time.Duration) Flight {
	started := at.Add(-time.Duration(minutes) * time.Minute)
	return Flight{Callsign: id, RunwayGroupID: group, State: aman.StateStable, FreezeReason: aman.FreezeNone, OperationalTETA: at.Add(30 * time.Minute), WakeCategory: "M", HoldingStackID: hold, HoldingQueueID: hold, HoldingAltitudeFeet: &level, HoldingStartedAt: &started, HoldingTransit: transit, ProtectCurrentSlot: true, CurrentSlot: &aman.Slot{Time: at.Add(30 * time.Minute), RunwayGroupID: group, Sequence: 1, Revision: 7, Reason: "stable"}}
}

func TestRebuildMergesLowestClearedHeadsByHoldingAgeDeterministically(t *testing.T) {
	input, at := rebuildTestInput()
	input.Flights = []Flight{rebuildHeld("UPPER", "A", "H1", 12000, 20, at, 4*time.Minute), rebuildHeld("LOWER", "A", "H1", 10000, 5, at, 4*time.Minute), rebuildHeld("OTHER", "B", "H2", 8000, 12, at, 2*time.Minute)}
	got, err := ApplyArrivalSequenceRebuild(input, at)
	require.NoError(t, err)
	require.Equal(t, []aman.Callsign{"OTHER", "LOWER", "UPPER"}, got.Outcome.ReleaseOrder)
	original, _ := json.Marshal(got)
	slices.Reverse(input.Flights)
	again, err := ApplyArrivalSequenceRebuild(input, at)
	require.NoError(t, err)
	// Input is caller ordered; compare canonical candidate/outcome only.
	a, _ := json.Marshal(struct {
		Result  Result
		Outcome aman.ArrivalSequenceRebuildOutcome
	}{got.Candidate, got.Outcome})
	b, _ := json.Marshal(struct {
		Result  Result
		Outcome aman.ArrivalSequenceRebuildOutcome
	}{again.Candidate, again.Outcome})
	require.JSONEq(t, string(a), string(b))
	require.NotEmpty(t, original)
	require.Equal(t, at.Add(30*time.Minute), input.Flights[0].CurrentSlot.Time, "pure policy must not edit reservations")
}

func TestRebuildAllowsIdenticalEATsAcrossHoldsButSeparatesSameHold(t *testing.T) {
	input, at := rebuildTestInput()
	input.Flights = []Flight{rebuildHeld("OLD", "A", "H1", 10000, 15, at, 8*time.Minute), rebuildHeld("NEW", "B", "H2", 10000, 9, at, 2*time.Minute)}
	got, err := ApplyArrivalSequenceRebuild(input, at)
	require.NoError(t, err)
	require.Equal(t, got.Candidate.Entries[0].Time.Add(-8*time.Minute), got.Candidate.Entries[1].Time.Add(-2*time.Minute))
	require.True(t, got.Candidate.Entries[1].Time.Before(got.Candidate.Entries[0].Time), "release priority is independent of landing order")
	input.Flights[1].HoldingStackID = "H1"
	input.Flights[1].HoldingQueueID = "H1"
	input.Flights[1].HoldingAltitudeFeet = intPointer(12000)
	got, err = ApplyArrivalSequenceRebuild(input, at)
	require.NoError(t, err)
	require.GreaterOrEqual(t, got.Candidate.Entries[1].Time.Add(-2*time.Minute).Sub(got.Candidate.Entries[0].Time.Add(-8*time.Minute)), time.Minute)
}

func TestRebuildRetainsTMAThenHeldThenInboundAndCrossesGaps(t *testing.T) {
	input, at := rebuildTestInput()
	protected := Flight{Callsign: "TMA", RunwayGroupID: "A", State: aman.StateStable, FreezeReason: aman.FreezeNone, OperationalTETA: at.Add(3 * time.Minute), WakeCategory: "M", RebuildTMA: true, CurrentSlot: &aman.Slot{Time: at.Add(3 * time.Minute), Sequence: 1, RunwayGroupID: "A", Reason: "stable"}}
	input.Flights = []Flight{rebuildHeld("HOLD", "A", "H1", 10000, 15, at, time.Minute), protected, {Callsign: "INBOUND", RunwayGroupID: "A", State: aman.StateUnstable, FreezeReason: aman.FreezeNone, OperationalTETA: at.Add(time.Minute), WakeCategory: "M"}}
	input.Policies[0].Gaps = []Gap{{Start: at.Add(4 * time.Minute), End: at.Add(10 * time.Minute)}}
	got, err := ApplyArrivalSequenceRebuild(input, at)
	require.NoError(t, err)
	require.Equal(t, []aman.Callsign{"TMA", "HOLD", "INBOUND"}, []aman.Callsign{got.Candidate.Entries[0].Callsign, got.Candidate.Entries[1].Callsign, got.Candidate.Entries[2].Callsign})
	require.Equal(t, protected.CurrentSlot.Time, got.Candidate.Entries[0].Time)
	require.GreaterOrEqual(t, got.Candidate.Entries[1].Time, at.Add(10*time.Minute))
}

func TestRebuildReplacesUnusableTMACapturesBeforeAllocatingHolds(t *testing.T) {
	for _, oldOffset := range []time.Duration{-5 * time.Minute, 45 * time.Minute} {
		t.Run(oldOffset.String(), func(t *testing.T) {
			input, at := rebuildTestInput()
			estimate, capturedAt := at.Add(3*time.Minute), at.Add(-10*time.Minute)
			oldETA := at.Add(oldOffset)
			oldSlot := &aman.Slot{Time: oldETA, RunwayGroupID: "A", Sequence: 1, Reason: "freeze_tma"}
			input.Flights = []Flight{{Callsign: "TMA", RunwayGroupID: "A", State: aman.StateStable, OperationalTETA: estimate, WakeCategory: "M",
				RebuildTMA: true, FreezeReason: aman.FreezeTMA, FrozenAt: &capturedAt, FrozenOperationalTETA: &oldETA, CapturedSlot: oldSlot, CurrentSlot: oldSlot},
				rebuildHeld("HOLD", "A", "H1", 10000, 15, at, time.Minute)}
			got, err := ApplyArrivalSequenceRebuild(input, at)
			require.NoError(t, err)
			require.Equal(t, "TMA", string(got.Candidate.Entries[0].Callsign))
			require.Equal(t, estimate, got.Candidate.Entries[0].Time)
			require.True(t, got.Candidate.Entries[1].Time.After(estimate))
			require.True(t, got.Candidate.Entries[1].Time.Before(at.Add(10*time.Minute)), "the former capture must not keep blocking runway capacity")
		})
	}
}

func TestRebuildTMAPassingHoldingFootprintDoesNotRetainObsoleteCapture(t *testing.T) {
	input, at := rebuildTestInput()
	estimate, oldETA, capturedAt := at.Add(3*time.Minute), at.Add(40*time.Minute), at.Add(-time.Minute)
	oldSlot := &aman.Slot{Time: oldETA, RunwayGroupID: "A", Sequence: 1, Reason: "freeze_tma"}
	input.Flights = []Flight{{Callsign: "TMA", RunwayGroupID: "A", State: aman.StateStable, OperationalTETA: estimate, WakeCategory: "M",
		RebuildTMA: true, HoldingStackID: "H1", FreezeReason: aman.FreezeTMA, FrozenAt: &capturedAt, FrozenOperationalTETA: &oldETA, CapturedSlot: oldSlot, CurrentSlot: oldSlot},
		rebuildHeld("HOLD", "A", "H1", 10000, 15, at, time.Minute)}
	got, err := ApplyArrivalSequenceRebuild(input, at)
	require.NoError(t, err)
	require.Equal(t, estimate, got.Candidate.Entries[0].Time)
	require.Equal(t, []aman.Callsign{"HOLD"}, got.Outcome.ReleaseOrder)
	require.Empty(t, got.Outcome.Diagnostics)
}

func TestRebuildHistoricalRetainedConflictCannotBlockCurrentPlan(t *testing.T) {
	input, at := rebuildTestInput()
	oldAt := at.Add(-time.Hour)
	for i, name := range []aman.Callsign{"STALE", "DISCONNECTED"} {
		target := at.Add(time.Duration(-30+i) * time.Minute)
		slot := &aman.Slot{Time: target, RunwayGroupID: "A", Sequence: i + 1, Reason: "freeze_superstable"}
		input.Flights = append(input.Flights, Flight{Callsign: name, RunwayGroupID: "A", State: aman.StateStable, WakeCategory: "M", OperationalTETA: target,
			RebuildRetained: true, FreezeReason: aman.FreezeSuperstable, FrozenAt: &oldAt, FrozenOperationalTETA: &target, CapturedSlot: slot, CurrentSlot: slot})
	}
	input.Flights = append(input.Flights, Flight{Callsign: "CURRENT", RunwayGroupID: "A", State: aman.StateStable, WakeCategory: "M", FreezeReason: aman.FreezeNone, RebuildTMA: true, OperationalTETA: at.Add(3 * time.Minute)})
	got, err := ApplyArrivalSequenceRebuild(input, at)
	require.NoError(t, err)
	require.Equal(t, at.Add(3*time.Minute), got.Candidate.Entries[2].Time)
	// The same conflict must still reject the operation if it affects the future.
	for i := range input.Flights[:2] {
		target := at.Add(time.Duration(30+i) * time.Minute)
		input.Flights[i].CurrentSlot.Time = target
		input.Flights[i].FrozenOperationalTETA = &target
	}
	_, err = ApplyArrivalSequenceRebuild(input, at)
	require.ErrorContains(t, err, "violate runway separation")
}

func TestRebuildRetainsUnknownHoldAndReportsProtectedExceptions(t *testing.T) {
	input, at := rebuildTestInput()
	old := rebuildHeld("OLD", "A", "H1", 10000, 15, at, time.Minute)
	frozen := rebuildHeld("FROZEN", "B", "H2", 10000, 5, at, time.Minute)
	frozen.FreezeReason = aman.FreezeManual
	frozen.CurrentSlot.Time = at
	frozen.CapturedSlot = cloneSlot(frozen.CurrentSlot)
	frozen.FrozenAt = &at
	frozen.FrozenOperationalTETA = &frozen.OperationalTETA
	input.Flights = []Flight{old, frozen}
	got, err := ApplyArrivalSequenceRebuild(input, at)
	require.NoError(t, err)
	require.NotEmpty(t, got.Outcome.Diagnostics)
	require.Equal(t, frozen.CapturedSlot.Time, got.Candidate.Entries[1].Time)
	input.Flights[0].HoldingAltitudeFeet = nil
	got, err = ApplyArrivalSequenceRebuild(input, at)
	require.NoError(t, err)
	require.Equal(t, old.CurrentSlot.Time, got.Candidate.Entries[0].Time)
	require.True(t, slices.ContainsFunc(got.Outcome.Diagnostics, func(d aman.RebuildDiagnostic) bool { return d.Code == "holding_evidence_unavailable" }))
}

func TestRebuildSearchesPastConsecutiveGapsAndClosures(t *testing.T) {
	input, at := rebuildTestInput()
	input.Flights = []Flight{rebuildHeld("HOLD", "A", "H1", 10000, 15, at, time.Minute)}
	input.Policies[0].Gaps = []Gap{{Start: at, End: at.Add(2 * time.Minute)}, {Start: at.Add(4 * time.Minute), End: at.Add(6 * time.Minute)}}
	end := at.Add(4 * time.Minute)
	input.Policies[0].Closures = []aman.RunwayClosure{{Start: at.Add(2 * time.Minute), End: &end}}
	got, err := ApplyArrivalSequenceRebuild(input, at)
	require.NoError(t, err)
	require.Equal(t, at.Add(6*time.Minute), got.Candidate.Entries[0].Time)
}
