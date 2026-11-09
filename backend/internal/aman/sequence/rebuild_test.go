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

func TestRebuildAssignsImmediateTMAQueueBeforeOutsideTraffic(t *testing.T) {
	input, at := rebuildTestInput()
	input.Policies[0].Rates[0].ArrivalsPerHour = 20
	input.Policies[0].SameSTARSpacing = SameSTARSpacing{Enabled: true, ActivationRatePerHour: 20, MinimumEmptySlots: 1}
	tma1 := rebuildHeld("TMA1", "A", "H1", 10000, 15, at, time.Minute)
	tma2 := rebuildHeld("TMA2", "A", "H1", 12000, 15, at, time.Minute)
	tma1.RebuildTMA, tma2.RebuildTMA = true, true
	tma1.OperationalTETA, tma2.OperationalTETA = at.Add(time.Hour), at.Add(2*time.Hour)
	tma1.STARFamily, tma2.STARFamily = "TUDLO", "TUDLO"
	tma1.ActiveHoldingSince = &at
	input.Flights = []Flight{rebuildHeld("HOLD", "A", "H1", 10000, 15, at, time.Minute), tma2, tma1,
		{Callsign: "INBOUND", RunwayGroupID: "A", State: aman.StateUnstable, FreezeReason: aman.FreezeNone, OperationalTETA: at.Add(time.Minute), WakeCategory: "M"}}
	got, err := ApplyArrivalSequenceRebuild(input, at)
	require.NoError(t, err)
	require.Len(t, got.Candidate.Entries, 4)
	require.Equal(t, "TMA1", string(got.Candidate.Entries[0].Callsign))
	require.Equal(t, at.Add(90*time.Second), got.Candidate.Entries[0].Time)
	require.Equal(t, "TMA2", string(got.Candidate.Entries[1].Callsign))
	require.Equal(t, at.Add(180*time.Second), got.Candidate.Entries[1].Time)
	require.Equal(t, ReasonRebuildTMA, got.Candidate.Entries[0].Reason)
	require.Equal(t, "HOLD", string(got.Candidate.Entries[2].Callsign))
	require.Equal(t, "INBOUND", string(got.Candidate.Entries[3].Callsign))
	require.Equal(t, []aman.Callsign{"HOLD"}, got.Outcome.ReleaseOrder)
	require.Empty(t, got.Outcome.Diagnostics)
}

func TestRebuildTMAQueueStillRespectsWakeAndRunwayClosures(t *testing.T) {
	input, at := rebuildTestInput()
	input.Policies[0].SeparationRules = CPHSeparationRules()
	end := at.Add(2 * time.Minute)
	input.Policies[0].Closures = []aman.RunwayClosure{{Start: at, End: &end}}
	input.Flights = []Flight{
		{Callsign: "HEAVY", RunwayGroupID: "A", State: aman.StateStable, FreezeReason: aman.FreezeNone, OperationalTETA: at.Add(time.Hour), WakeCategory: "H", RebuildTMA: true},
		{Callsign: "MEDIUM", RunwayGroupID: "A", State: aman.StateStable, FreezeReason: aman.FreezeNone, OperationalTETA: at.Add(2 * time.Hour), WakeCategory: "M", RebuildTMA: true},
	}
	got, err := ApplyArrivalSequenceRebuild(input, at)
	require.NoError(t, err)
	require.Equal(t, end, got.Candidate.Entries[0].Time)
	require.Equal(t, end.Add(2*time.Minute), got.Candidate.Entries[1].Time)
}

func TestRebuildDiscardsConflictingManualAndAutomaticCaptures(t *testing.T) {
	for _, freeze := range []aman.FreezeReason{aman.FreezeManual, aman.FreezeSuperstable, aman.FreezeTMA} {
		t.Run(string(freeze), func(t *testing.T) {
			input, at := rebuildTestInput()
			for i, id := range []aman.Callsign{"BAW816", "BAW822"} {
				f := rebuildHeld(id, "A", "H1", 10000+i*2000, 15-i, at, 4*time.Minute)
				f.STARFamily = "TUDLO"
				f.FreezeReason = freeze
				f.CapturedSlot = cloneSlot(f.CurrentSlot)
				f.FrozenAt = &at
				f.FrozenOperationalTETA = &f.OperationalTETA
				f.ManualOrder = intPointer(2 - i)
				input.Flights = append(input.Flights, f)
			}
			got, err := ApplyArrivalSequenceRebuild(input, at)
			require.NoError(t, err)
			require.Empty(t, got.Outcome.Diagnostics)
			require.Equal(t, []aman.Callsign{"BAW816", "BAW822"}, got.Outcome.ReleaseOrder)
			require.True(t, got.Candidate.Entries[0].Time.Before(input.Flights[0].CurrentSlot.Time))
			require.GreaterOrEqual(t, got.Candidate.Entries[1].Time.Sub(got.Candidate.Entries[0].Time), 2*time.Minute)
			for _, f := range got.Input.Flights {
				require.Equal(t, aman.FreezeNone, f.FreezeReason)
				require.Nil(t, f.CapturedSlot)
				require.Nil(t, f.ManualOrder)
				require.False(t, f.ProtectCurrentSlot)
			}
			for _, entry := range got.Candidate.Entries {
				require.False(t, entry.Protected)
			}
		})
	}
}

func TestRebuildUnknownHoldingEvidenceUsesNewPositionTiming(t *testing.T) {
	input, at := rebuildTestInput()
	f := rebuildHeld("UNKNOWN", "A", "H1", 10000, 15, at, time.Minute)
	f.OperationalTETA = at.Add(5 * time.Minute)
	f.HoldingAltitudeFeet = nil
	input.Flights = []Flight{f}
	got, err := ApplyArrivalSequenceRebuild(input, at)
	require.NoError(t, err)
	require.Equal(t, f.OperationalTETA, got.Candidate.Entries[0].Time)
	require.NotEqual(t, f.CurrentSlot.Time, got.Candidate.Entries[0].Time)
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
