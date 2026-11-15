package release

import (
	"FlightStrips/internal/aman"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"slices"
	"testing"
	"time"
)

func fixture(n int) Input {
	at := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	in := Input{At: at, Policy: &aman.ApproachReleasePolicy{Enabled: true, TargetBuffer: 10, TaperStart: 6, MaxAdvanceSeconds: 600, LookaheadSeconds: 120}}
	for i := 0; i < n; i++ {
		key := fmt.Sprintf("F%d", i)
		in.Traffic = append(in.Traffic, Traffic{Callsign: key, WakeCategory: "M", Active: true, Eligible: true, Episode: key, HoldingID: key, STARFamily: key, FeederFix: key,
			RunwayGroupID: "A", RouteDigest: "geometry", EntryAt: at.Add(-time.Minute), ReadyAt: at,
			BaselineReleaseAt: at.Add(time.Duration(i) * 90 * time.Second), SlotAt: at.Add(12*time.Minute + time.Duration(i)*90*time.Second),
			Transit: 12 * time.Minute, MinimumSpacing: 90 * time.Second, BusySpacing: 3 * time.Minute})
	}
	return in
}

func TestGuidancePreviewBeforeAdmissionWindowPreservesActualCapacityAndStreamOrder(t *testing.T) {
	in := fixture(2)
	for i := range in.Traffic {
		f := &in.Traffic[i]
		f.HoldingID, f.STARFamily, f.FeederFix = "HOLD", "STAR", "FEEDER"
		f.EntryAt = in.At.Add(time.Duration(7+i) * time.Minute)
		f.ReadyAt = f.EntryAt
		f.Transit = 10 * time.Minute
		f.BaselineReleaseAt = in.At.Add(time.Duration(12+i) * time.Minute)
		f.SlotAt = f.BaselineReleaseAt.Add(f.Transit)
	}
	before, err := json.Marshal(in)
	require.NoError(t, err)
	actual := Plan(in)
	require.Zero(t, actual.Buffer.Projected())
	for _, d := range actual.Decisions {
		require.Nil(t, d.Reservation)
	}
	plans := GuidancePlans(in)
	require.Len(t, plans, 2)
	require.Equal(t, in.At.Add(7*time.Minute+time.Second), plans["F0"].ApproachReleaseTime)
	require.Equal(t, 299*time.Second, plans["F0"].AdditionalApproachTime)
	require.Equal(t, in.At.Add(8*time.Minute+31*time.Second), plans["F1"].ApproachReleaseTime, "the preview retains base stream spacing")
	for _, p := range plans {
		require.NoError(t, p.Validate())
	}
	after, err := json.Marshal(in)
	require.NoError(t, err)
	require.Equal(t, before, after)
	require.Equal(t, actual, Plan(in), "guidance cannot create admissions or change the live buffer")
	slices.Reverse(in.Traffic)
	require.Equal(t, plans, GuidancePlans(in))
}

func TestGuidancePreviewRequiresUsableWorkloadAndCountsFutureIncoming(t *testing.T) {
	for _, kind := range []string{"quiet", "disabled", "unknown", "blocked", "full", "future incoming", "committed incoming", "distant incoming", "manual or blocked head", "beyond preview"} {
		t.Run(kind, func(t *testing.T) {
			in := fixture(1)
			in.Traffic[0].EntryAt, in.Traffic[0].ReadyAt = in.At.Add(7*time.Minute), in.At.Add(7*time.Minute)
			in.Traffic[0].BaselineReleaseAt = in.At.Add(12 * time.Minute)
			in.Traffic[0].Transit = 10 * time.Minute
			in.Traffic[0].SlotAt = in.At.Add(22 * time.Minute)
			switch kind {
			case "disabled":
				in.Policy.Enabled = false
			case "unknown":
				in.WorkloadUnknown = true
			case "blocked":
				in.BlockedReason = "approach_release_spacing_conflict"
			case "full", "future incoming", "committed incoming", "distant incoming":
				for i := 0; i < 10; i++ {
					f := Traffic{Callsign: fmt.Sprintf("OTHER%d", i), Active: true, InApproach: kind == "full"}
					if kind != "full" {
						entry := in.At.Add(6 * time.Minute)
						if kind == "distant incoming" {
							entry = in.At.Add(20 * time.Minute)
						}
						if kind == "committed incoming" {
							entry = in.At.Add(time.Hour)
							f.Incoming = true // An explicit expedite is already committed.
						}
						f.PredictedApproachEntry = &entry
					}
					in.Traffic = append(in.Traffic, f)
				}
			case "manual or blocked head":
				older := in.Traffic[0]
				older.Callsign, older.Eligible = "OLDER", false
				older.EntryAt = in.At.Add(6 * time.Minute)
				in.Traffic = append(in.Traffic, older)
			case "beyond preview":
				in.Traffic[0].EntryAt, in.Traffic[0].ReadyAt = in.At.Add(30*time.Minute), in.At.Add(30*time.Minute)
				in.Traffic[0].BaselineReleaseAt = in.At.Add(35 * time.Minute)
				in.Traffic[0].SlotAt = in.At.Add(45 * time.Minute)
			}
			plans := GuidancePlans(in)
			if kind == "quiet" || kind == "distant incoming" {
				require.Contains(t, plans, aman.Callsign("F0"))
			} else {
				require.Empty(t, plans, "do not promise downstream absorption without a feasible preview")
			}
		})
	}
}

func TestEnroutePreviewUsesCapacityAtReleaseRatherThanEndOfPlanningHorizon(t *testing.T) {
	in := fixture(2)
	for i := range in.Traffic {
		f := &in.Traffic[i]
		f.EntryAt = in.At.Add(time.Duration(18+i*4) * time.Minute)
		f.ReadyAt = f.EntryAt
		f.BaselineReleaseAt = f.EntryAt.Add(5 * time.Minute)
		f.SlotAt = f.BaselineReleaseAt.Add(f.Transit)
	}
	// Ten aircraft arriving at minute 21 fill later capacity, without erasing
	// an earlier opportunity. No future landing is assumed to free a place.
	for i := 0; i < 10; i++ {
		entry := in.At.Add(21 * time.Minute)
		in.Traffic = append(in.Traffic, Traffic{Callsign: fmt.Sprintf("IN%d", i), Active: true, PredictedApproachEntry: &entry})
	}
	plans := GuidancePlans(in)
	require.Contains(t, plans, aman.Callsign("F0"), "enroute sees a preview eighteen minutes before release")
	require.Equal(t, in.At.Add(18*time.Minute+time.Second), plans["F0"].ApproachReleaseTime)
	require.NotContains(t, plans, aman.Callsign("F1"), "later intake cannot assume that earlier traffic has landed")
	require.Zero(t, Plan(in).Buffer.Projected())
	// Crossing the old 12-minute horizon with unchanged physical facts cannot
	// withdraw the preview, even as later arrivals enter the planning window.
	for elapsed := time.Duration(0); elapsed <= 15*time.Minute; elapsed += 30 * time.Second {
		in.At = fixture(0).At.Add(elapsed)
		require.Equal(t, plans["F0"], GuidancePlans(in)["F0"], "elapsed=%s", elapsed)
	}
}

func TestGuidancePreviewRevalidatesEarlierDecisionsAgainstFinalIntake(t *testing.T) {
	for _, test := range []struct {
		name   string
		offset time.Duration
	}{
		{"later processed earlier release", 0},
		{"revalidation exceeds preview horizon", 10*time.Minute + time.Second},
	} {
		t.Run(test.name, func(t *testing.T) {
			in := fixture(3)
			for i, baseline := range []time.Duration{25*time.Minute + test.offset, 24*time.Minute + test.offset, 13 * time.Minute} {
				f := &in.Traffic[i]
				f.EntryAt = in.At.Add(time.Duration(i-3) * time.Minute)
				f.BaselineReleaseAt = in.At.Add(baseline)
				f.SlotAt = f.BaselineReleaseAt.Add(f.Transit)
			}
			for i := 0; i < 5; i++ {
				in.Traffic = append(in.Traffic, Traffic{Callsign: fmt.Sprintf("APP%d", i), Active: true, InApproach: true})
			}
			before, err := json.Marshal(in)
			require.NoError(t, err)
			actual := Plan(in)
			plans := GuidancePlans(in)
			require.Equal(t, in.At.Add(3*time.Minute), plans["F2"].ApproachReleaseTime)
			require.Equal(t, in.At.Add(16*time.Minute+30*time.Second+test.offset), plans["F1"].ApproachReleaseTime)
			if test.offset == 0 {
				require.Equal(t, in.At.Add(20*time.Minute), plans["F0"].ApproachReleaseTime, "F1 crosses F0's initial release, requiring another capacity check")
			} else {
				require.NotContains(t, plans, aman.Callsign("F0"), "do not publish a release postponed beyond the planning horizon")
			}
			for callsign, p := range plans {
				count := 5 + 1
				for other, preceding := range plans {
					if other != callsign && !preceding.ApproachReleaseTime.After(p.ApproachReleaseTime) {
						count++
					}
				}
				require.LessOrEqual(t, p.AdditionalApproachTime, Allowance(*in.Policy, count), "final capacity must support %s's published advance", callsign)
				require.NoError(t, p.Validate())
			}
			after, err := json.Marshal(in)
			require.NoError(t, err)
			require.Equal(t, before, after)
			require.Equal(t, actual, Plan(in), "preview stabilization cannot change actual admissions")
			slices.Reverse(in.Traffic)
			require.Equal(t, plans, GuidancePlans(in))
		})
	}
}

func TestGuidancePreviewPropagatesCapacityPostponementThroughItsStream(t *testing.T) {
	in := fixture(3)
	for i, baseline := range []time.Duration{25 * time.Minute, 26 * time.Minute, 13 * time.Minute} {
		f := &in.Traffic[i]
		f.EntryAt = in.At.Add(time.Duration(i-3) * time.Minute)
		f.BaselineReleaseAt = in.At.Add(baseline)
		f.SlotAt = f.BaselineReleaseAt.Add(f.Transit)
	}
	in.Traffic[1].HoldingID, in.Traffic[1].STARFamily, in.Traffic[1].FeederFix = in.Traffic[0].HoldingID, in.Traffic[0].STARFamily, in.Traffic[0].FeederFix
	// A later rate period and a future arrival both constrain a postponed
	// follower. Deliberately unsorted rate points must not mutate the input.
	in.Traffic[1].Spacing = []StreamSpacing{
		{EffectiveAt: in.At.Add(18 * time.Minute), Minimum: 4 * time.Minute},
		{EffectiveAt: in.At.Add(-time.Hour), Minimum: 90 * time.Second},
	}
	for i := 0; i < 5; i++ {
		in.Traffic = append(in.Traffic, Traffic{Callsign: fmt.Sprintf("APP%d", i), Active: true, InApproach: true})
	}
	entry := in.At.Add(20 * time.Minute)
	in.Traffic = append(in.Traffic, Traffic{Callsign: "INCOMING", Active: true, PredictedApproachEntry: &entry})
	before, err := json.Marshal(in)
	require.NoError(t, err)
	plans := GuidancePlans(in)
	require.Equal(t, in.At.Add(3*time.Minute), plans["F2"].ApproachReleaseTime)
	require.Equal(t, in.At.Add(17*time.Minute+30*time.Second), plans["F0"].ApproachReleaseTime)
	require.Equal(t, in.At.Add(23*time.Minute+30*time.Second), plans["F1"].ApproachReleaseTime, "postponement must recheck both final occupancy and stream spacing")
	require.Equal(t, 150*time.Second, plans["F1"].AdditionalApproachTime)
	after, err := json.Marshal(in)
	require.NoError(t, err)
	require.Equal(t, before, after)
	slices.Reverse(in.Traffic)
	require.Equal(t, plans, GuidancePlans(in))
}

func TestPreviewRechecksCapacityWhenSpacingPushesReleaseAcrossIncomingTraffic(t *testing.T) {
	in := fixture(2)
	for i := range in.Traffic {
		f := &in.Traffic[i]
		f.HoldingID, f.STARFamily, f.FeederFix = "HOLD", "STAR", "FEED"
		f.EntryAt = in.At.Add(time.Duration(7+i) * time.Minute)
		f.ReadyAt = f.EntryAt
		f.BaselineReleaseAt = in.At.Add(13 * time.Minute)
		f.SlotAt = f.BaselineReleaseAt.Add(f.Transit)
	}
	passage := in.At.Add(7 * time.Minute)
	in.Traffic[0].Eligible = false
	in.Traffic[0].PredictedStreamPassage = &passage
	in.Traffic[1].MinimumSpacing = 3 * time.Minute
	for i := 0; i < 10; i++ {
		entry := in.At.Add(9 * time.Minute)
		in.Traffic = append(in.Traffic, Traffic{Callsign: fmt.Sprintf("IN%d", i), Active: true, PredictedApproachEntry: &entry})
	}
	require.Empty(t, GuidancePlans(in), "spacing reaches a full buffer; do not retain the initial spare-capacity allowance")
}

func TestImminentAdmissionFollowsUnheldPassageWithoutReservingOrReleasingTheLeader(t *testing.T) {
	in := fixture(2)
	for i := range in.Traffic {
		f := &in.Traffic[i]
		f.HoldingID, f.STARFamily, f.FeederFix = "HOLD", "STAR", "FEED"
		f.EntryAt = in.At.Add(time.Duration(10+i*20) * time.Second)
		f.ReadyAt = f.EntryAt
		f.BaselineReleaseAt = in.At.Add(3 * time.Minute)
		f.SlotAt = f.BaselineReleaseAt.Add(f.Transit)
	}
	passage := in.Traffic[0].EntryAt.Add(time.Second)
	in.Traffic[0].Eligible, in.Traffic[0].Incoming = false, true
	in.Traffic[0].PredictedStreamPassage = &passage
	result := Plan(in)
	require.Nil(t, result.Decisions[0].Reservation, "a normal arrival must not acquire a fabricated admission")
	require.False(t, in.Traffic[0].Released, "a forecast does not establish observed movement")
	require.NotNil(t, result.Decisions[1].Reservation)
	require.Equal(t, passage.Add(90*time.Second), result.Decisions[1].Reservation.ReleaseAt)
	require.Nil(t, result.Decisions[1].Reservation.ReleasedAt)
	require.Equal(t, 1, result.Buffer.Incoming)
	require.Equal(t, 1, result.Buffer.Reserved)
	// Once capacity is full, even that physical stream opportunity supplies no
	// extra advance. The normal arrival remains counted throughout.
	for i := 0; i < 9; i++ {
		in.Traffic = append(in.Traffic, Traffic{Callsign: fmt.Sprintf("APP%d", i), Active: true, InApproach: true})
	}
	for _, d := range Plan(in).Decisions {
		require.Nil(t, d.Reservation)
	}
}

func TestInitialGroupAndRestartReserveEachAircraftOnce(t *testing.T) {
	in := fixture(6)
	original, _ := json.Marshal(in)
	out := Plan(in)
	require.Equal(t, 6, out.Buffer.Reserved)
	for _, d := range out.Decisions {
		require.NotNil(t, d.Reservation)
		require.Equal(t, in.At, d.Reservation.ReleaseAt)
		require.NoError(t, d.Reservation.Validate())
	}
	unchanged, _ := json.Marshal(in)
	require.Equal(t, original, unchanged)
	slices.Reverse(in.Traffic)
	require.Equal(t, out, Plan(in))
	for i := range in.Traffic {
		for _, d := range out.Decisions {
			if in.Traffic[i].Callsign == d.Callsign {
				in.Traffic[i].Reservation = d.Reservation
			}
		}
	}
	encoded, _ := json.Marshal(in)
	var restored Input
	require.NoError(t, json.Unmarshal(encoded, &restored))
	restored.At = restored.At.Add(3 * time.Minute)
	require.Equal(t, out.Buffer.Reserved, Plan(restored).Buffer.Reserved, "missed EATs do not free capacity")
	restored.Traffic[0].InApproach = true
	restored.Traffic[0].Released = true
	next := Plan(restored)
	require.Equal(t, 6, next.Buffer.Projected())
	require.Equal(t, 1, next.Buffer.InApproach)
	restored.Traffic[0].Active = false
	require.Equal(t, 5, Plan(restored).Buffer.Projected(), "confirmed retirement releases exactly one reservation")
}

func TestFiveIndependentStreamsAndSixthAircraftRetainOnlyBaseEntrySpacing(t *testing.T) {
	in := fixture(6)
	in.Traffic[5].HoldingID = in.Traffic[0].HoldingID
	in.Traffic[5].STARFamily = in.Traffic[0].STARFamily
	in.Traffic[5].FeederFix = in.Traffic[0].FeederFix
	in.Traffic[5].EntryAt = in.At.Add(-30 * time.Second)
	out := Plan(in)
	require.Equal(t, 6, out.Buffer.Projected())
	for i := 0; i < 5; i++ {
		require.Equal(t, in.At, out.Decisions[i].Reservation.ReleaseAt)
	}
	require.Equal(t, in.At.Add(90*time.Second), out.Decisions[5].Reservation.ReleaseAt)
}

func TestNewHoldingAssignmentDoesNotRetireAdmissionUntilPhysicallyConfirmed(t *testing.T) {
	in := fixture(1)
	r := Plan(in).Decisions[0].Reservation
	in.Traffic[0].Reservation = r
	in.Traffic[0].Episode = "replacement-hold"
	in.Traffic[0].HoldingID = "OTHER-HOLD"
	in.Traffic[0].BaselineReleaseAt = in.At.Add(time.Hour)
	require.Equal(t, 1, Plan(in).Buffer.Projected(), "a clearance alone does not remove an aircraft from the approach pipeline")
	in.Traffic[0].NewHoldingEpisode = true
	require.Zero(t, Plan(in).Buffer.Projected(), "confirmed reholding replaces the prior admission")
}

func TestTaperAndSoftTarget(t *testing.T) {
	in := fixture(1)
	for _, test := range []struct {
		n    int
		want time.Duration
	}{{1, 10 * time.Minute}, {6, 10 * time.Minute}, {7, 450 * time.Second}, {8, 5 * time.Minute}, {9, 150 * time.Second}, {10, 0}, {12, 0}} {
		require.Equal(t, test.want, Allowance(*in.Policy, test.n))
	}
	for i := 0; i < 10; i++ {
		in.Traffic = append(in.Traffic, Traffic{Callsign: fmt.Sprintf("APP%d", i), Active: true, InApproach: true})
	}
	in.Traffic[0].BaselineReleaseAt = in.At.Add(5 * time.Minute)
	require.Nil(t, Plan(in).Decisions[0].Reservation, "full buffer prevents additional advancement")
	in.Traffic[0].BaselineReleaseAt = in.At
	require.Equal(t, 11, Plan(in).Buffer.Projected(), "the desired buffer is not a hard airspace limit")
}

func TestBlockedStreamDoesNotBlockOthersAndSameSTARReleaseGapIsRelaxed(t *testing.T) {
	in := fixture(4)
	in.Traffic[0].BaselineReleaseAt = in.At.Add(time.Hour)
	in.Traffic[1].HoldingID = in.Traffic[0].HoldingID
	in.Traffic[1].STARFamily = in.Traffic[0].STARFamily
	in.Traffic[1].EntryAt = in.At.Add(-30 * time.Second)
	out := Plan(in)
	require.Nil(t, out.Decisions[0].Reservation)
	require.Nil(t, out.Decisions[1].Reservation)
	require.NotNil(t, out.Decisions[2].Reservation)
	require.NotNil(t, out.Decisions[3].Reservation)
	in = fixture(2)
	in.Traffic[1].STARFamily = in.Traffic[0].STARFamily
	in.Traffic[1].EntryAt = in.At.Add(-30 * time.Second)
	out = Plan(in)
	require.Equal(t, in.At, out.Decisions[0].Reservation.ReleaseAt)
	require.Equal(t, in.At.Add(90*time.Second), out.Decisions[1].Reservation.ReleaseAt, "extra three-minute same-STAR spacing is not applied during buildup")
}

func TestMissingFactsDisabledPolicyAndIncomingDeduplication(t *testing.T) {
	in := fixture(2)
	in.Traffic[0].Incoming = true
	out := Plan(in)
	require.Equal(t, 2, out.Buffer.Projected())
	require.Zero(t, out.Buffer.Incoming)
	in.Traffic[0].Unknown = true
	out = Plan(in)
	require.Equal(t, "degraded", out.Buffer.Status)
	require.Nil(t, out.Decisions[1].Reservation)
	in = fixture(1)
	in.Policy.Enabled = false
	require.Nil(t, Plan(in).Decisions[0].Reservation)
	in.Traffic[0].Reservation = Plan(fixture(1)).Decisions[0].Reservation
	require.Equal(t, 1, Plan(in).Buffer.Reserved, "disabling preserves outstanding commitments")
}

func TestFutureReservationCanAdvanceButDueReservationCannotBePostponed(t *testing.T) {
	in := fixture(1)
	r := Plan(in).Decisions[0].Reservation
	r.ReleaseAt = in.At.Add(time.Minute)
	in.Traffic[0].Reservation = r
	out := Plan(in)
	require.Equal(t, in.At, out.Decisions[0].Reservation.ReleaseAt)
	require.Equal(t, 1, out.Buffer.Reserved)
	in.Traffic[0].Reservation = out.Decisions[0].Reservation
	in.Traffic[0].BaselineReleaseAt = in.At.Add(30 * time.Minute)
	require.Equal(t, in.At, Plan(in).Decisions[0].Reservation.ReleaseAt)
}

func TestSpacingReevaluatesWhenCandidateCrossesRateBoundary(t *testing.T) {
	for _, test := range []struct {
		name string
		next time.Duration
		want time.Duration
	}{
		{"increase", time.Minute, 90 * time.Second},
		{"decrease", 4 * time.Minute, 4 * time.Minute},
	} {
		t.Run(test.name, func(t *testing.T) {
			in := fixture(2)
			in.Policy.LookaheadSeconds = 300
			in.Traffic[1].HoldingID, in.Traffic[1].STARFamily = in.Traffic[0].HoldingID, in.Traffic[0].STARFamily
			in.Traffic[1].EntryAt = in.At.Add(-30 * time.Second)
			for i := range in.Traffic {
				in.Traffic[i].BaselineReleaseAt = in.At.Add(8*time.Minute + time.Duration(i)*time.Minute)
				// Deliberately unordered input must remain untouched.
				in.Traffic[i].Spacing = []StreamSpacing{{EffectiveAt: in.At.Add(90 * time.Second), Minimum: test.next, Busy: test.next}, {EffectiveAt: in.At.Add(-time.Hour), Minimum: 2 * time.Minute, Busy: 2 * time.Minute}}
			}
			before, _ := json.Marshal(in)
			out := Plan(in)
			require.Equal(t, in.At, out.Decisions[0].Reservation.ReleaseAt)
			require.Equal(t, in.At.Add(test.want), out.Decisions[1].Reservation.ReleaseAt)
			after, _ := json.Marshal(in)
			require.Equal(t, before, after)
		})
	}
}

func TestPostponedFutureLeaderConstrainsFollowingRelease(t *testing.T) {
	in := fixture(2)
	in.Traffic[1].HoldingID, in.Traffic[1].STARFamily = in.Traffic[0].HoldingID, in.Traffic[0].STARFamily
	in.Traffic[1].EntryAt = in.At.Add(-30 * time.Second)
	for i := range in.Traffic {
		f := &in.Traffic[i]
		f.BaselineReleaseAt = in.At.Add(8 * time.Minute)
		f.Reservation = &aman.ApproachReleaseReservation{Episode: f.Episode, HoldingID: f.HoldingID, STARFamily: f.STARFamily, FeederFix: f.FeederFix, RunwayGroupID: f.RunwayGroupID, HoldingEntryAt: f.EntryAt,
			ReleaseAt: in.At.Add(time.Duration(i+1) * 30 * time.Second), Transit: f.Transit, RouteDigest: f.RouteDigest, PolicyVersion: PolicyVersion}
	}
	in.Traffic[0].ReadyAt = in.At.Add(90 * time.Second)
	out := Plan(in)
	require.Equal(t, in.At.Add(90*time.Second), out.Decisions[0].Reservation.ReleaseAt)
	require.Equal(t, in.At.Add(3*time.Minute), out.Decisions[1].Reservation.ReleaseAt)
	require.Equal(t, 1, out.Buffer.Projected(), "the postponed distant offer persists outside the intake window")
	require.Equal(t, in.At.Add(30*time.Second), in.Traffic[0].Reservation.ReleaseAt, "input commitments were not mutated")
}

func TestFutureStreamOffersCanAdvanceWithoutWaitingBehindTheirFollowers(t *testing.T) {
	in := fixture(2)
	in.Traffic[1].HoldingID, in.Traffic[1].STARFamily = in.Traffic[0].HoldingID, in.Traffic[0].STARFamily
	in.Traffic[1].EntryAt = in.At.Add(-30 * time.Second)
	for i := range in.Traffic {
		f := &in.Traffic[i]
		f.BaselineReleaseAt = in.At.Add(8 * time.Minute)
		f.Reservation = &aman.ApproachReleaseReservation{Episode: f.Episode, HoldingID: f.HoldingID, STARFamily: f.STARFamily, FeederFix: f.FeederFix, RunwayGroupID: f.RunwayGroupID, HoldingEntryAt: f.EntryAt,
			ReleaseAt: in.At.Add(time.Duration(i+1) * time.Minute), Transit: f.Transit, RouteDigest: f.RouteDigest, PolicyVersion: PolicyVersion}
	}
	out := Plan(in)
	require.Equal(t, in.At, out.Decisions[0].Reservation.ReleaseAt)
	require.Equal(t, in.At.Add(90*time.Second), out.Decisions[1].Reservation.ReleaseAt)
	require.Equal(t, 2, out.Buffer.Projected())
}

func TestEntrySpacingUsesObservedReleaseWhenLaterThanOfferedEAT(t *testing.T) {
	in := fixture(2)
	in.Traffic[1].HoldingID, in.Traffic[1].STARFamily = in.Traffic[0].HoldingID, in.Traffic[0].STARFamily
	in.Traffic[1].EntryAt = in.At.Add(-30 * time.Second)
	in.Traffic[1].BaselineReleaseAt = in.At.Add(8 * time.Minute)
	in.Traffic[1].Spacing = []StreamSpacing{{EffectiveAt: in.At.Add(-time.Hour), Minimum: 2 * time.Minute}}
	f := &in.Traffic[0]
	f.InApproach = true
	f.Reservation = &aman.ApproachReleaseReservation{Episode: f.Episode, HoldingID: f.HoldingID, STARFamily: f.STARFamily, FeederFix: f.FeederFix, RunwayGroupID: f.RunwayGroupID,
		HoldingEntryAt: in.At.Add(-10 * time.Minute), ReleaseAt: in.At.Add(-5 * time.Minute), ReleasedAt: &in.At, Transit: f.Transit, RouteDigest: f.RouteDigest, PolicyVersion: PolicyVersion}
	out := Plan(in)
	require.Equal(t, in.At.Add(2*time.Minute), out.Decisions[1].Reservation.ReleaseAt)
}

func TestLateDepartureDegradesIntakeWithoutMovingDueEATs(t *testing.T) {
	in := fixture(3)
	base := in.At
	in.At = base.Add(3 * time.Minute)
	in.Traffic[1].HoldingID, in.Traffic[1].STARFamily = in.Traffic[0].HoldingID, in.Traffic[0].STARFamily
	for i := 0; i < 2; i++ {
		f := &in.Traffic[i]
		f.Reservation = &aman.ApproachReleaseReservation{Episode: f.Episode, HoldingID: f.HoldingID, STARFamily: f.STARFamily, FeederFix: f.FeederFix,
			RunwayGroupID: f.RunwayGroupID, HoldingEntryAt: f.EntryAt, ReleaseAt: base.Add(time.Duration(i) * 90 * time.Second),
			Transit: f.Transit, RouteDigest: f.RouteDigest, PolicyVersion: PolicyVersion}
	}
	in.Traffic[2].ReadyAt, in.Traffic[2].BaselineReleaseAt = in.At, in.At.Add(5*time.Minute)
	require.NotNil(t, Plan(in).Decisions[2].Reservation, "the independent stream can advance before a conflict is known")
	in.Traffic[0].Released = true // First evidence of departure at 12:03, after its 12:00 EAT.
	before, err := json.Marshal(in)
	require.NoError(t, err)
	out := Plan(in)
	require.Equal(t, "degraded", out.Buffer.Status)
	require.Contains(t, out.Buffer.BlockedReasons, "approach_release_spacing_conflict")
	require.Equal(t, base, out.Decisions[0].Reservation.ReleaseAt)
	require.Equal(t, in.At, *out.Decisions[0].Reservation.ReleasedAt)
	require.Equal(t, base.Add(90*time.Second), out.Decisions[1].Reservation.ReleaseAt)
	require.Nil(t, out.Decisions[2].Reservation, "a spacing conflict stops additional advances, including independent streams")
	require.Equal(t, 2, out.Buffer.Projected(), "both irrevocable admissions remain counted")
	after, err := json.Marshal(in)
	require.NoError(t, err)
	require.Equal(t, before, after, "validation must not mutate accepted input facts")
	slices.Reverse(in.Traffic)
	require.Equal(t, out, Plan(in), "validation follows accepted release order, not input order")
	for i := range in.Traffic {
		for _, d := range out.Decisions {
			if in.Traffic[i].Callsign == d.Callsign {
				in.Traffic[i].Reservation = d.Reservation
			}
		}
	}
	encoded, err := json.Marshal(in)
	require.NoError(t, err)
	var restored Input
	require.NoError(t, json.Unmarshal(encoded, &restored))
	restored.At = base.Add(5 * time.Minute)
	// An elapsed spacing interval cannot establish that the follower actually departed safely.
	require.Equal(t, "degraded", Plan(restored).Buffer.Status)
	for i := range restored.Traffic {
		f := &restored.Traffic[i]
		if f.Callsign == "F1" {
			actual := base.Add(4*time.Minute + 30*time.Second)
			f.Reservation.ReleasedAt = &actual
		}
		if f.Callsign == "F2" {
			f.BaselineReleaseAt, f.ReadyAt = restored.At.Add(5*time.Minute), restored.At
		}
	}
	resolved := Plan(restored)
	require.Equal(t, "ready", resolved.Buffer.Status, "a later observed departure satisfying spacing resolves the conflict")
	require.Equal(t, base.Add(90*time.Second), resolved.Decisions[1].Reservation.ReleaseAt, "even resolution preserves the accepted EAT")
	require.NotNil(t, resolved.Decisions[2].Reservation)
}

func TestDueEATSpacingUsesWakeAndEffectiveRateWithoutRoundingTargets(t *testing.T) {
	for _, tc := range []struct {
		name              string
		leading, trailing string
		minimum, gap      time.Duration
		conflict          bool
	}{
		{"wake conflict", "J", "M", time.Minute, 2 * time.Minute, true},
		{"base-rate conflict", "M", "M", 2 * time.Minute, 90 * time.Second, true},
		{"exact boundary", "M", "M", 90 * time.Second, 90 * time.Second, false},
		{"fractional target", "M", "M", 90 * time.Second, 90*time.Second + time.Nanosecond, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := fixture(3)
			base := in.At
			in.At = base.Add(5 * time.Minute)
			in.Traffic[0].WakeCategory, in.Traffic[1].WakeCategory = tc.leading, tc.trailing
			in.Traffic[1].FeederFix = in.Traffic[0].FeederFix
			in.Traffic[1].Spacing = []StreamSpacing{{EffectiveAt: base, Minimum: tc.minimum}, {EffectiveAt: in.At.Add(time.Hour), Minimum: time.Hour}}
			for i := 0; i < 2; i++ {
				f := &in.Traffic[i]
				target := base
				if i == 1 {
					target = base.Add(time.Minute + tc.gap)
				}
				f.Reservation = &aman.ApproachReleaseReservation{Episode: f.Episode, HoldingID: f.HoldingID, STARFamily: f.STARFamily, FeederFix: f.FeederFix,
					RunwayGroupID: f.RunwayGroupID, HoldingEntryAt: f.EntryAt, ReleaseAt: target, Transit: f.Transit, RouteDigest: f.RouteDigest, PolicyVersion: PolicyVersion}
				// Holding timing can disappear; due commitments still require validation.
				f.Transit, f.EntryAt, f.ReadyAt = 0, time.Time{}, time.Time{}
			}
			actual := base.Add(time.Minute)
			in.Traffic[0].Reservation.ReleasedAt = &actual
			in.Traffic[2].ReadyAt, in.Traffic[2].BaselineReleaseAt = in.At, in.At.Add(5*time.Minute)
			out := Plan(in)
			require.Equal(t, in.Traffic[1].Reservation.ReleaseAt, out.Decisions[1].Reservation.ReleaseAt)
			if tc.conflict {
				require.Equal(t, "degraded", out.Buffer.Status)
				require.Contains(t, out.Buffer.BlockedReasons, "approach_release_spacing_conflict")
				require.Nil(t, out.Decisions[2].Reservation)
			} else {
				require.Equal(t, "ready", out.Buffer.Status)
				require.NotNil(t, out.Decisions[2].Reservation)
			}
		})
	}
}

func TestNominalTimingConflictRetainsCapacityAndAllowsTaperedIntake(t *testing.T) {
	in := fixture(9)
	for i := range in.Traffic {
		in.Traffic[i].BaselineReleaseAt = in.At.Add(8 * time.Minute)
	}
	f := &in.Traffic[0]
	f.EntryAt, f.ReadyAt = in.At.Add(114*time.Second), in.At.Add(114*time.Second)
	f.BaselineReleaseAt = in.At.Add(49 * time.Second)
	f.Reservation = &aman.ApproachReleaseReservation{Episode: f.Episode, HoldingID: f.HoldingID, STARFamily: f.STARFamily,
		FeederFix: f.FeederFix, RunwayGroupID: f.RunwayGroupID, HoldingEntryAt: f.EntryAt,
		ReleaseAt: in.At.Add(116 * time.Second), Transit: f.Transit, RouteDigest: f.RouteDigest, PolicyVersion: PolicyVersion}
	before, err := json.Marshal(in)
	require.NoError(t, err)
	out := Plan(in)
	require.Equal(t, "ready", out.Buffer.Status)
	require.Empty(t, out.Buffer.BlockedReasons)
	require.Equal(t, []string{TimingConflictReason}, out.Buffer.AdvisoryReasons)
	require.Equal(t, f.Reservation, out.Decisions[0].Reservation, "the incompatible nominal target does not cancel a reservation")
	for _, d := range out.Decisions[1:6] {
		require.NotNil(t, d.Reservation)
		require.Equal(t, in.At, d.Reservation.ReleaseAt)
	}
	require.Equal(t, in.At.Add(30*time.Second), out.Decisions[6].Reservation.ReleaseAt, "the retained reservation still counts toward the seventh admission's taper")
	require.Nil(t, out.Decisions[7].Reservation)
	require.Nil(t, out.Decisions[8].Reservation)
	require.Equal(t, 7, out.Buffer.Projected())
	after, err := json.Marshal(in)
	require.NoError(t, err)
	require.Equal(t, before, after)
	slices.Reverse(in.Traffic)
	require.Equal(t, out, Plan(in))

	in.BlockedReason = "approach_absorption_budget_exceeded"
	blocked := Plan(in)
	require.Equal(t, "degraded", blocked.Buffer.Status)
	require.Contains(t, blocked.Buffer.BlockedReasons, in.BlockedReason)
	require.Equal(t, []string{TimingConflictReason}, blocked.Buffer.AdvisoryReasons)
	require.Equal(t, 1, blocked.Buffer.Projected(), "an advisory must not override a real intake blocker")
}

func TestLateHoldingEntryConstrainsItsStreamWithoutBlockingIndependentIntake(t *testing.T) {
	in := fixture(3)
	f := &in.Traffic[0]
	f.EntryAt, f.ReadyAt = in.At.Add(time.Minute), in.At.Add(time.Minute)
	f.BaselineReleaseAt = in.At.Add(5 * time.Minute)
	f.Reservation = &aman.ApproachReleaseReservation{Episode: f.Episode, HoldingID: f.HoldingID, STARFamily: f.STARFamily,
		FeederFix: f.FeederFix, RunwayGroupID: f.RunwayGroupID, HoldingEntryAt: in.At.Add(-time.Minute),
		ReleaseAt: in.At, Transit: f.Transit, RouteDigest: f.RouteDigest, PolicyVersion: PolicyVersion}
	in.Traffic[1].STARFamily = f.STARFamily
	in.Traffic[1].EntryAt, in.Traffic[1].ReadyAt = in.At.Add(70*time.Second), in.At.Add(70*time.Second)
	in.Traffic[1].BaselineReleaseAt = in.At.Add(8 * time.Minute)
	out := Plan(in)
	require.Equal(t, "ready", out.Buffer.Status)
	require.Equal(t, []string{TimingConflictReason}, out.Buffer.AdvisoryReasons)
	require.Equal(t, in.At, out.Decisions[0].Reservation.ReleaseAt)
	require.Nil(t, out.Decisions[0].Reservation.ReleasedAt)
	require.Nil(t, out.Decisions[1].Reservation, "spacing after the late holding entry lies beyond lookahead")
	require.NotNil(t, out.Decisions[2].Reservation)
	require.Equal(t, 2, out.Buffer.Projected())
}

func TestPredictedEntrySlipCannotCreateDueSpacingConflict(t *testing.T) {
	in := fixture(3)
	base := in.At
	in.At = base.Add(3 * time.Minute)
	in.Traffic[1].STARFamily = in.Traffic[0].STARFamily
	for i := 0; i < 2; i++ {
		f := &in.Traffic[i]
		f.Reservation = &aman.ApproachReleaseReservation{Episode: f.Episode, HoldingID: f.HoldingID, STARFamily: f.STARFamily,
			FeederFix: f.FeederFix, RunwayGroupID: f.RunwayGroupID, HoldingEntryAt: f.EntryAt,
			ReleaseAt: base.Add(time.Duration(i) * 90 * time.Second), Transit: f.Transit, RouteDigest: f.RouteDigest, PolicyVersion: PolicyVersion}
	}
	// Both EATs are due and correctly spaced. A new prediction moves the
	// leader's holding arrival beyond its accepted EAT, without observing
	// either departure. It must not rewrite their spacing history.
	in.Traffic[0].EntryAt, in.Traffic[0].ReadyAt = base.Add(time.Minute), base.Add(time.Minute)
	in.Traffic[2].ReadyAt, in.Traffic[2].BaselineReleaseAt = in.At, in.At.Add(5*time.Minute)
	before, err := json.Marshal(in)
	require.NoError(t, err)
	out := Plan(in)
	require.Equal(t, "ready", out.Buffer.Status)
	require.Empty(t, out.Buffer.BlockedReasons)
	require.Equal(t, []string{TimingConflictReason}, out.Buffer.AdvisoryReasons)
	for i := 0; i < 2; i++ {
		require.Equal(t, in.Traffic[i].Reservation, out.Decisions[i].Reservation, "accepted EATs and departure evidence must stay intact")
	}
	require.NotNil(t, out.Decisions[2].Reservation, "predicted timing must not block independent intake")
	require.Equal(t, 3, out.Buffer.Projected())
	after, err := json.Marshal(in)
	require.NoError(t, err)
	require.Equal(t, before, after)
	slices.Reverse(in.Traffic)
	require.Equal(t, out, Plan(in))

	// Actual departure evidence remains authoritative and can establish a
	// genuine conflict against the due follower's unchanged commitment.
	for i := range in.Traffic {
		if in.Traffic[i].Callsign == "F0" {
			in.Traffic[i].Released = true
		}
	}
	observed := Plan(in)
	require.Equal(t, "degraded", observed.Buffer.Status)
	require.Contains(t, observed.Buffer.BlockedReasons, "approach_release_spacing_conflict")
	require.Nil(t, observed.Decisions[2].Reservation)
	require.Equal(t, 2, observed.Buffer.Projected())
}
