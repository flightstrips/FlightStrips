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
