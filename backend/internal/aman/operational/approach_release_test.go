package operational

import (
	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/terminal"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func approachFixture(t *testing.T) (*Service, aman.AirportState, time.Time) {
	at := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	cfg := terminal.Configuration{ApproachRelease: &aman.ApproachReleasePolicy{Enabled: true, TargetBuffer: 10, TaperStart: 6, MaxAdvanceSeconds: 600, LookaheadSeconds: 120}}
	svc := &Service{deps: Dependencies{Terminal: cfg, Now: func() time.Time { return at }}, tmaVolume: testTMAService(t).tmaVolume}
	state := aman.AirportState{Airport: "EKCH", Revision: 1, GeneratedAt: at, Authoritative: true}
	for i := 0; i < 6; i++ {
		f, _, _ := holdingReleaseFixture(at)
		f.Callsign = fmt.Sprintf("HELD%d", i)
		family, hold, feeder := fmt.Sprintf("STAR%d", i), fmt.Sprintf("HOLD%d", i), fmt.Sprintf("FF%d", i)
		f.SelectedSTARFamily = &family
		f.SelectedFeederFix = &feeder
		f.SelectedHolding = &hold
		f.HoldingClearance.Hold = hold
		f.HoldingStack.HoldingID = hold
		f.Slot.Time = at.Add(12*time.Minute + time.Duration(i)*90*time.Second)
		f.Slot.Sequence = i + 1
		f.Prediction.GeometryDigest = "physical-path"
		f.HoldingReleaseBasis = &aman.HoldingReleaseBasis{HoldingID: hold, Fix: hold, RunwayGroupID: *f.SelectedRunwayGroup, RouteDigest: "physical-path", HoldingEntryTime: f.HoldingStack.FirstObservedAt, PostHoldingTransit: 12 * time.Minute}
		observed, alt := at, 12000
		f.LatestObservation.Surveillance = &aman.SurveillanceFact{ObservedAt: &observed, AltitudeFeet: &alt}
		state.Flights = append(state.Flights, f)
	}
	effective := at.Add(-time.Hour)
	state.RunwayGroups = []aman.RunwayGroupPolicy{{ID: *state.Flights[0].SelectedRunwayGroup, ActiveRatePerHour: 40, RateEffectiveAt: &effective}}
	return svc, state, at
}

func TestApproachReleaseIntegrationPreservesSlotsAndPhysicalTransit(t *testing.T) {
	svc, state, at := approachFixture(t)
	svc.refreshHoldingPlans(&state)
	require.Equal(t, 6, state.ReleaseBuffer.Projected())
	for i, f := range state.Flights {
		require.NotNil(t, f.ApproachRelease)
		require.Equal(t, at, f.Prediction.HoldingPlan.ApproachReleaseTime)
		require.Equal(t, at.Add(12*time.Minute+time.Duration(i)*90*time.Second), f.Slot.Time)
		require.Equal(t, 12*time.Minute, f.Prediction.HoldingPlan.PostHoldingTransit)
		require.Equal(t, time.Duration(i)*90*time.Second, f.Prediction.HoldingPlan.AdditionalApproachTime)
	}
	raw, err := json.Marshal(state)
	require.NoError(t, err)
	var restored aman.AirportState
	require.NoError(t, json.Unmarshal(raw, &restored))
	svc.refreshHoldingPlans(&restored)
	require.Equal(t, state, restored)
}

func TestReleaseBufferDoesNotInferReleaseFromCancellationOrElapsedSlot(t *testing.T) {
	svc, state, at := approachFixture(t)
	svc.refreshHoldingPlans(&state)
	state.Flights[0].ReplaceHoldingClearance(&aman.HoldingClearance{ObservedAt: at.Add(time.Second)})
	svc.deps.Now = func() time.Time { return at.Add(40 * time.Minute) }
	svc.refreshHoldingPlans(&state)
	require.Equal(t, 6, state.ReleaseBuffer.Projected())
	require.Greater(t, state.ReleaseBuffer.Unknown, 0)
	require.Nil(t, state.Flights[0].ApproachRelease.ReleasedAt)
	require.Equal(t, at, state.Flights[1].ApproachRelease.ReleaseAt)
	state.Flights[0].State = aman.StateLanded
	svc.refreshHoldingPlans(&state)
	require.Equal(t, 5, state.ReleaseBuffer.Projected())
	require.Nil(t, state.Flights[0].ApproachRelease)
}

func TestUnknownGeometryAndManualProtectionPreventNewAdvance(t *testing.T) {
	svc, state, _ := approachFixture(t)
	// Keep slots far enough away that no legacy imminent EAT is reconstructed.
	for i := range state.Flights {
		state.Flights[i].Slot.Time = state.Flights[i].Slot.Time.Add(5 * time.Minute)
	}
	svc.tmaVolume = nil
	svc.refreshHoldingPlans(&state)
	require.Equal(t, "degraded", state.ReleaseBuffer.Status)
	for _, f := range state.Flights {
		require.Nil(t, f.ApproachRelease)
	}
	svc, state, _ = approachFixture(t)
	state.Flights[2].FreezeReason = aman.FreezeManual
	svc.refreshHoldingPlans(&state)
	require.Nil(t, state.Flights[2].ApproachRelease)
	require.NotNil(t, state.Flights[3].ApproachRelease, "another independent feeder remains eligible")
}

func TestCapacityChangeReplansFutureOfferButPreservesDueRelease(t *testing.T) {
	svc, state, at := approachFixture(t)
	svc.refreshHoldingPlans(&state)
	// This future offer can be retracted before it is due.
	state.Flights[0].ApproachRelease.ReleaseAt = at.Add(time.Minute)
	state.Flights[0].Slot.Time = at.Add(40 * time.Minute)
	svc.refreshHoldingPlans(&state)
	require.Equal(t, at.Add(18*time.Minute), state.Flights[0].ApproachRelease.ReleaseAt)
	require.Equal(t, 10*time.Minute, state.Flights[0].ApproachRelease.AdditionalApproachTime)
	require.Equal(t, 5, state.ReleaseBuffer.Projected(), "a postponed future offer no longer consumes the current intake window")
	// Aircraft already eligible for release cannot be recalled by a capacity
	// change. Expose the excessive absorption and stop additional advancement.
	state.Flights[1].Slot.Time = at.Add(40 * time.Minute)
	svc.refreshHoldingPlans(&state)
	require.Equal(t, at, state.Flights[1].ApproachRelease.ReleaseAt)
	require.Equal(t, "degraded", state.ReleaseBuffer.Status)
	require.Contains(t, state.ReleaseBuffer.BlockedReasons, "approach_absorption_budget_exceeded")
}

func TestLegacyManualEATIsReservedBeforePhysicalPredictionRefresh(t *testing.T) {
	svc, state, at := approachFixture(t)
	f := &state.Flights[0]
	entry := at.Add(-4 * time.Minute)
	f.FreezeReason = aman.FreezeManual
	f.Prediction.HoldingPlan = &aman.HoldingPlan{HoldingEntryTime: entry, ApproachReleaseTime: at,
		ExpectedHoldingDuration: 4 * time.Minute, PostHoldingTransit: 12 * time.Minute}
	// A different new physical journey would otherwise replace the accepted EAT.
	f.HoldingReleaseBasis.PostHoldingTransit = 5 * time.Minute
	svc.refreshHoldingPlans(&state)
	require.NotNil(t, state.Flights[0].ApproachRelease)
	require.Equal(t, at, state.Flights[0].ApproachRelease.ReleaseAt)
	require.Equal(t, at, state.Flights[0].Prediction.HoldingPlan.ApproachReleaseTime)
	require.Equal(t, 6, state.ReleaseBuffer.Projected(), "the protected legacy release remains accounted for")
}

func TestDueEATRetainsAcceptedJourneyWhenNewTransitCannotMeetSlot(t *testing.T) {
	svc, state, at := approachFixture(t)
	state.Flights = state.Flights[:1]
	at = at.Add(20 * time.Minute)
	svc.deps.Now = func() time.Time { return at }
	f := &state.Flights[0]
	f.LatestObservation.Surveillance.ObservedAt = &at
	f.Slot.Time = at.Add(10 * time.Minute)
	f.HoldingReleaseBasis.PostHoldingTransit = 20 * time.Minute
	entry := at.Add(-4 * time.Minute)
	f.Prediction.HoldingPlan = &aman.HoldingPlan{HoldingEntryTime: entry, ApproachReleaseTime: at, ExpectedHoldingDuration: 4 * time.Minute, PostHoldingTransit: 10 * time.Minute}
	svc.refreshHoldingPlans(&state)
	require.Equal(t, at, state.Flights[0].Prediction.HoldingPlan.ApproachReleaseTime)
	require.Equal(t, 10*time.Minute, state.Flights[0].Prediction.HoldingPlan.PostHoldingTransit)
	require.Contains(t, state.ReleaseBuffer.BlockedReasons, "approach_release_timing_conflict")
	require.Equal(t, "degraded", state.ReleaseBuffer.Status)
}

func TestReadinessSlipPostponesFutureEATButPreservesDueTarget(t *testing.T) {
	for _, due := range []bool{false, true} {
		t.Run(fmt.Sprintf("due=%v", due), func(t *testing.T) {
			svc, state, at := approachFixture(t)
			independent := state.Flights[1]
			independent.Slot.Time = at.Add(17 * time.Minute)
			state.Flights = state.Flights[:1]
			f := &state.Flights[0]
			f.HoldingStack, f.HoldingReleaseBasis = nil, nil
			entry := at.Add(30 * time.Second)
			f.Prediction.HoldingFixETA = &entry
			f.Prediction.RawTETA = entry.Add(12 * time.Minute)
			f.Slot.Time = at.Add(14 * time.Minute)
			svc.refreshHoldingPlans(&state)
			require.NotNil(t, f.ApproachRelease)
			target := at.Add(time.Minute)
			f.ApproachRelease.ReleaseAt = target
			f.ApproachRelease.HoldingEntryAt = entry
			entry = at.Add(90 * time.Second)
			f.Prediction.RawTETA = entry.Add(12 * time.Minute)
			if due {
				at = target
			}
			svc.deps.Now = func() time.Time { return at }
			f.LatestObservation.Surveillance.ObservedAt = &at
			independent.LatestObservation.Surveillance.ObservedAt = &at
			state.Flights = append(state.Flights, independent)
			svc.refreshHoldingPlans(&state)
			f = &state.Flights[0]
			require.NoError(t, f.ApproachRelease.Validate())
			require.NoError(t, f.Prediction.HoldingPlan.Validate())
			if due {
				require.Equal(t, target, f.ApproachRelease.ReleaseAt)
				require.Equal(t, target, f.Prediction.HoldingPlan.ApproachReleaseTime)
				require.Equal(t, "degraded", state.ReleaseBuffer.Status)
				require.Contains(t, state.ReleaseBuffer.BlockedReasons, "approach_release_timing_conflict")
				require.Nil(t, state.Flights[1].ApproachRelease, "a due readiness conflict stops new advancement")
			} else {
				require.Equal(t, entry.Add(time.Second), f.ApproachRelease.ReleaseAt)
				require.Equal(t, entry, f.Prediction.HoldingPlan.HoldingEntryTime)
				require.Equal(t, "ready", state.ReleaseBuffer.Status)
				require.NotNil(t, state.Flights[1].ApproachRelease, "an independent feeder can still release")
			}
		})
	}
}

func TestAcceptedEATSurvivesInfeasibleFreshHoldingPlan(t *testing.T) {
	for _, basis := range []bool{false, true} {
		t.Run(fmt.Sprintf("basis=%v", basis), func(t *testing.T) {
			svc, state, at := approachFixture(t)
			state.Flights = state.Flights[:2]
			state.Flights[1].Slot.Time = at.Add(17 * time.Minute)
			f := &state.Flights[0]
			entry := at.Add(-4 * time.Minute)
			f.Prediction.HoldingPlan = &aman.HoldingPlan{HoldingEntryTime: entry, ApproachReleaseTime: at, ExpectedHoldingDuration: 4 * time.Minute, PostHoldingTransit: 12 * time.Minute}
			*f = svc.captureLegacyApproachRelease(*f, at)
			if basis {
				f.HoldingReleaseBasis.PostHoldingTransit = 20 * time.Minute
			} else {
				f.HoldingStack, f.HoldingReleaseBasis = nil, nil
				f.Prediction.HoldingFixETA = &entry
				f.Prediction.RawTETA = at.Add(20 * time.Minute)
			}
			require.Nil(t, holdingPlanForFlight(*f, *f.Prediction), "the fresh journey cannot fit the assigned slot")
			svc.refreshHoldingPlans(&state)
			require.Equal(t, at, f.ApproachRelease.ReleaseAt)
			require.NotNil(t, f.Prediction.HoldingPlan, "the accepted EAT stays published")
			require.Equal(t, at, f.Prediction.HoldingPlan.ApproachReleaseTime)
			require.Equal(t, 12*time.Minute, f.Prediction.HoldingPlan.PostHoldingTransit)
			require.NoError(t, f.Prediction.HoldingPlan.Validate())
			require.Equal(t, "degraded", state.ReleaseBuffer.Status)
			require.Contains(t, state.ReleaseBuffer.BlockedReasons, "approach_release_timing_conflict")
			require.Nil(t, state.Flights[1].ApproachRelease)
			require.Equal(t, 1, state.ReleaseBuffer.Projected())
		})
	}
}

func TestAdvancedReleaseUsesRateAtApproachEntry(t *testing.T) {
	svc, state, at := approachFixture(t)
	state.Flights = state.Flights[:2]
	first := &state.Flights[0]
	second := &state.Flights[1]
	second.SelectedSTARFamily, second.SelectedHolding, second.SelectedFeederFix = first.SelectedSTARFamily, first.SelectedHolding, first.SelectedFeederFix
	second.HoldingClearance.Hold = first.HoldingClearance.Hold
	second.HoldingStack.HoldingID = first.HoldingStack.HoldingID
	second.HoldingStack.FirstObservedAt = at.Add(-3 * time.Minute)
	second.HoldingReleaseBasis.HoldingID, second.HoldingReleaseBasis.Fix = first.HoldingReleaseBasis.HoldingID, first.HoldingReleaseBasis.Fix
	second.HoldingReleaseBasis.HoldingEntryTime = second.HoldingStack.FirstObservedAt
	first.Slot.Time, second.Slot.Time = at.Add(20*time.Minute), at.Add(21*time.Minute)
	state.RunwayGroups[0].RateSchedule = []aman.RunwayGroupRatePoint{{EffectiveAt: at.Add(-time.Hour), ArrivalsPerHour: 30}, {EffectiveAt: at.Add(5 * time.Minute), ArrivalsPerHour: 60}}
	svc.refreshHoldingPlans(&state)
	require.Equal(t, at, first.ApproachRelease.ReleaseAt)
	require.Equal(t, at.Add(2*time.Minute), second.ApproachRelease.ReleaseAt)
	require.Equal(t, at.Add(20*time.Minute), first.Slot.Time)
	require.Equal(t, at.Add(21*time.Minute), second.Slot.Time)
}

func TestPostponementKeepsAcceptedFutureStreamOrder(t *testing.T) {
	svc, state, at := approachFixture(t)
	state.Flights = state.Flights[:2]
	first, second := &state.Flights[0], &state.Flights[1]
	second.SelectedSTARFamily, second.SelectedHolding, second.SelectedFeederFix = first.SelectedSTARFamily, first.SelectedHolding, first.SelectedFeederFix
	second.HoldingClearance.Hold = first.HoldingClearance.Hold
	second.HoldingStack.HoldingID = first.HoldingStack.HoldingID
	second.HoldingStack.FirstObservedAt = at.Add(-3 * time.Minute)
	second.HoldingReleaseBasis.HoldingID, second.HoldingReleaseBasis.Fix = first.HoldingReleaseBasis.HoldingID, first.HoldingReleaseBasis.Fix
	second.HoldingReleaseBasis.HoldingEntryTime = second.HoldingStack.FirstObservedAt
	first.Slot.Time, second.Slot.Time = at.Add(18*time.Minute), at.Add(20*time.Minute)
	svc.refreshHoldingPlans(&state)
	first.ApproachRelease.ReleaseAt = at.Add(time.Minute)
	second.ApproachRelease.ReleaseAt = at.Add(2 * time.Minute)
	// A capacity change pushes the leader beyond the follower's old offer.
	first.Slot.Time = at.Add(40 * time.Minute)
	svc.refreshHoldingPlans(&state)
	require.Equal(t, at.Add(18*time.Minute), first.ApproachRelease.ReleaseAt)
	require.Equal(t, at.Add(18*time.Minute+90*time.Second), second.ApproachRelease.ReleaseAt)
	require.Equal(t, "degraded", state.ReleaseBuffer.Status, "the follower needs a compatible landing slot")
	require.Contains(t, state.ReleaseBuffer.BlockedReasons, "approach_release_timing_conflict")
}
