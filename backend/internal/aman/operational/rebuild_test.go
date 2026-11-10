package operational

import (
	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/navdata"
	"FlightStrips/internal/aman/release"
	"FlightStrips/internal/aman/sequence"

	"FlightStrips/internal/aman/trajectory"
	"context"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestHoldingEpisodeStartsAtFixAndSurvivesReplayCancellation(t *testing.T) {
	at := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	hold, fix := "H1", "FIX"
	flight := aman.AMANFlight{Callsign: "TEST", SelectedHolding: &hold, HoldingClearance: &aman.HoldingClearance{Hold: fix, HoldType: aman.HoldingClearanceEnroute, ObservedAt: at.Add(-time.Hour)}}
	position := navdata.Coordinate{LatitudeDeg: 55, LongitudeDeg: 12}
	projection := trajectory.Result{SelectedHolding: &navdata.HoldingPattern{ID: "H1", Fix: "FIX"}, HoldingFixPosition: &position}
	observe := func(longitude float64, when time.Time) {
		observeHoldingEpisode(&flight, projection, aman.FlightObservation{SourceStatus: aman.DataFresh, Surveillance: &aman.SurveillanceFact{LatitudeDegrees: 55, LongitudeDegrees: longitude, ObservedAt: &when}}, when)
	}
	observe(11.94, at)
	require.Nil(t, flight.HoldingEpisode.StartedAt, "distant clearance and footprint cannot start holding")
	observe(12.06, at.Add(time.Minute))
	require.Equal(t, "fix_crossing", flight.HoldingEpisode.Source)
	start := *flight.HoldingEpisode.StartedAt
	encoded, err := json.Marshal(flight)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(encoded, &flight))
	observe(12, at.Add(2*time.Minute))
	require.Equal(t, start, *flight.HoldingEpisode.StartedAt)
	canceled := at.Add(3 * time.Minute)
	flight.ReplaceHoldingClearance(&aman.HoldingClearance{ObservedAt: canceled})
	require.NotNil(t, flight.HoldingEpisode.EndedAt)
	flight.ReplaceHoldingClearance(&aman.HoldingClearance{Hold: fix, HoldType: aman.HoldingClearanceEnroute, ObservedAt: at.Add(4 * time.Minute)})
	observe(12, at.Add(5*time.Minute))
	require.Equal(t, at.Add(5*time.Minute), *flight.HoldingEpisode.StartedAt)
	require.NoError(t, flight.HoldingEpisode.Validate())
}

func TestAcceptedHoldingTerminationOlderThanRadarStartsNewEpisodeOnReassignment(t *testing.T) {
	for _, replacement := range []string{"", "OTHER"} {
		t.Run("replacement="+replacement, func(t *testing.T) {
			at := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
			hold, fix := "H1", "FIX"
			level := int32(10000)
			flight := aman.AMANFlight{Callsign: "TEST", State: aman.StateStable, SelectedHolding: &hold,
				HoldingClearance: &aman.HoldingClearance{Hold: fix, HoldType: aman.HoldingClearanceEnroute, ClearedAltitude: &level, ObservedAt: at.Add(-time.Hour)}}
			position := navdata.Coordinate{LatitudeDeg: 55, LongitudeDeg: 12}
			projection := trajectory.Result{SelectedHolding: &navdata.HoldingPattern{ID: "H1", Fix: "FIX"}, HoldingFixPosition: &position}
			observe := func(when time.Time) {
				observeHoldingEpisode(&flight, projection, aman.FlightObservation{SourceStatus: aman.DataFresh,
					Surveillance: &aman.SurveillanceFact{LatitudeDegrees: 55, LongitudeDegrees: 12, ObservedAt: &when}}, when)
			}
			observe(at)
			oldID := flight.HoldingEpisode.ID
			flight.HoldingReleaseReservation = &aman.HoldingReleaseReservation{CommandID: "rebuild", EpisodeID: oldID, HoldingID: hold, Level: int(level), Rank: 1}
			observe(at.Add(2 * time.Minute))
			require.NotNil(t, aman.ValidHoldingReleaseReservation(flight))
			endedAt := at.Add(time.Minute)
			flight.ReplaceHoldingClearance(&aman.HoldingClearance{Hold: replacement, HoldType: aman.HoldingClearanceEnroute, ObservedAt: endedAt})
			require.Equal(t, &endedAt, flight.HoldingEpisode.EndedAt)
			require.Nil(t, flight.HoldingReleaseReservation)
			require.NoError(t, flight.HoldingEpisode.Validate())

			// Reassign before any reconciliation can discard the old episode.
			flight.ReplaceHoldingClearance(&aman.HoldingClearance{Hold: fix, HoldType: aman.HoldingClearanceEnroute, ClearedAltitude: &level, ObservedAt: endedAt.Add(30 * time.Second)})
			observe(at.Add(3 * time.Minute))
			require.Equal(t, at.Add(3*time.Minute), *flight.HoldingEpisode.StartedAt)
			require.NotEqual(t, oldID, flight.HoldingEpisode.ID)
			require.Nil(t, flight.HoldingReleaseReservation)
			require.NoError(t, flight.HoldingEpisode.Validate())
		})
	}
}

func TestRebuildCommandIsAuthorizedRevisionCheckedAndNoOpCompletes(t *testing.T) {
	service, state, now := recomputeFlightFixture(t)
	repository := &memoryRepository{state: state, has: true}
	publisher := &recordingPublisher{}
	actions := recomputeActions(t, service, repository, publisher, now)
	auth := aman.CommandContext{Airport: "EKCH", Actor: "1234567", Role: "EKDK_FMP", ReceivedAt: now}
	command := aman.RebuildArrivalSequenceCommand{Metadata: aman.CommandMetadata{CommandID: "rebuild", ExpectedRevision: state.Revision}}
	first, err := actions.RebuildArrivalSequence(context.Background(), auth, command)
	require.NoError(t, err)
	require.True(t, first.Changed)
	require.Len(t, publisher.states, 1)
	duplicate, err := actions.RebuildArrivalSequence(context.Background(), auth, command)
	require.NoError(t, err)
	require.True(t, duplicate.Duplicate)
	command.Metadata.CommandID = "again"
	command.Metadata.ExpectedRevision = repository.state.Revision
	noop, err := actions.RebuildArrivalSequence(context.Background(), auth, command)
	require.NoError(t, err)
	require.False(t, noop.Changed)
	require.Equal(t, first.CurrentRevision, noop.CurrentRevision)
	require.Len(t, publisher.states, 1)
	command.Metadata.ExpectedRevision = state.Revision
	command.Metadata.CommandID = "stale"
	_, err = actions.RebuildArrivalSequence(context.Background(), auth, command)
	requireDomainClass(t, err, aman.ErrorRevisionConflict)
	auth.Role = "EKCH_APP"
	_, err = actions.RebuildArrivalSequence(context.Background(), auth, command)
	requireDomainClass(t, err, aman.ErrorUnauthorized)
}

func rebuildApproachStackFixture(t *testing.T) (*Service, aman.AirportState, time.Time) {
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	low, _, _ := holdingReleaseFixture(now)
	upper, _, _ := holdingReleaseFixture(now)
	low.Callsign = "LOW"
	upper.Callsign = "UPPER"
	lowerLevel, upperLevel := int32(10000), int32(12000)
	low.HoldingClearance.ClearedAltitude = &lowerLevel
	upper.HoldingClearance.ClearedAltitude = &upperLevel
	group := *low.SelectedRunwayGroup
	service, _, _ := approachFixture(t)
	low.Slot.Time = now.Add(30 * time.Minute)
	low.Slot.Sequence = 2
	upper.Slot.Time = now.Add(20 * time.Minute)
	upper.Slot.Sequence = 1
	for _, f := range []*aman.AMANFlight{&low, &upper} {
		f.LatestObservation.SourceStatus = aman.DataFresh
		family, feeder := "STAR", "FF"
		f.SelectedSTARFamily, f.SelectedFeederFix = &family, &feeder
		f.Prediction.GeometryDigest = "path"
		f.TMAEntry = &aman.TMAEntryState{LastContainment: aman.TMAInside, LastObservedAt: now, FreezeTriggered: true}
		age := 5
		if f.Callsign == "UPPER" {
			age = 20
		}
		started := now.Add(-time.Duration(age) * time.Minute)
		f.HoldingEpisode = &aman.HoldingEpisode{ID: string(f.Callsign), HoldingID: *f.SelectedHolding, Fix: "LUGAS", StartedAt: &started, Source: "near_fix"}
		f.HoldingReleaseBasis = &aman.HoldingReleaseBasis{HoldingID: *f.SelectedHolding, Fix: "LUGAS", RunwayGroupID: group, RouteDigest: "path", HoldingEntryTime: f.HoldingStack.FirstObservedAt, PostHoldingTransit: 4 * time.Minute}
		f.Prediction.HoldingPlan = holdingPlanForFlight(*f, *f.Prediction)
		f.ApproachRelease = &aman.ApproachReleaseReservation{Episode: string(f.Callsign), HoldingID: *f.SelectedHolding, STARFamily: family, FeederFix: feeder, RunwayGroupID: group, HoldingEntryAt: started, ReleaseAt: f.Prediction.HoldingPlan.ApproachReleaseTime, Transit: 4 * time.Minute, RouteDigest: "path", PolicyVersion: release.PolicyVersion}
	}
	state := aman.AirportState{Airport: "EKCH", Revision: 1, GeneratedAt: now, Flights: []aman.AMANFlight{upper, low}, RunwayGroups: []aman.RunwayGroupPolicy{{ID: group, ActiveRatePerHour: 20, RateEffectiveAt: &now}}}
	return service, state, now
}

func TestRebuiltLowestFirstEATSurvivesRoutineReconciliation(t *testing.T) {
	for _, near := range []bool{false, true} {
		t.Run(fmt.Sprintf("near=%t", near), func(t *testing.T) {
			service, state, now := rebuildApproachStackFixture(t)
			if near {
				for i := range state.Flights {
					entry := now.Add(time.Minute)
					state.Flights[i].Prediction.HoldingFixETA = &entry
				}
			}
			mutation, err := service.RebuildArrivalSequence(context.Background(), aman.CommandContext{Role: "EKDK_FMP", ReceivedAt: now}, aman.RebuildArrivalSequenceCommand{Metadata: aman.CommandMetadata{CommandID: "stack", ExpectedRevision: 1}})
			require.NoError(t, err)
			change, err := mutation(state)
			require.NoError(t, err)
			got := change.State
			require.NotNil(t, got.Flights[0].Prediction.HoldingPlan)
			require.NotNil(t, got.Flights[1].Prediction.HoldingPlan)
			require.True(t, got.Flights[1].Prediction.HoldingPlan.ApproachReleaseTime.Before(got.Flights[0].Prediction.HoldingPlan.ApproachReleaseTime))
			for _, f := range got.Flights {
				require.Equal(t, f.Slot.Time.Add(-4*time.Minute), f.Prediction.HoldingPlan.ApproachReleaseTime, "future automatic EAT must follow the rebuilt slot")
			}
			require.Equal(t, got.Flights[0].Slot.Time, got.Flights[0].Prediction.OperationalTETA)
			require.Equal(t, got.Flights[1].Slot.Time, got.Flights[1].Prediction.OperationalTETA)
			for _, f := range got.Flights {
				require.False(t, f.Prediction.HoldingPlan.ApproachReleaseTime.Before(*f.Prediction.HoldingFixETA), "a recalculation cannot release an outbound aircraft before it can reach the fix")
			}
			before := got.Flights[0].Prediction.HoldingPlan.ApproachReleaseTime
			service.resequence(&got, now)
			require.Equal(t, before, got.Flights[0].Prediction.HoldingPlan.ApproachReleaseTime)

			if near {
				require.NotNil(t, got.Flights[1].ApproachRelease, "imminent rebuilt offer must be captured")
				require.Equal(t, got.Flights[1].Prediction.HoldingPlan.ApproachReleaseTime, got.Flights[1].ApproachRelease.ReleaseAt)
			}
			again, err := mutation(got)
			require.NoError(t, err)
			require.False(t, again.Changed, "rebuilding the same accepted timing must be a no-op")
		})
	}

}

func TestRebuildRecapturesSuperstableAndCannotCommitOverConcurrentUpdate(t *testing.T) {
	service, state, now := recomputeFlightFixture(t)
	f := &state.Flights[0]
	f.State = aman.StateStable
	if f.Lifecycle != nil {
		copy := *f.Lifecycle
		copy.Reason = aman.LifecycleReasonStableHorizon
		f.Lifecycle = &copy
	}
	f.FreezeReason = aman.FreezeSuperstable
	oldAt := now.Add(-time.Minute)
	f.FrozenAt = &oldAt
	teta := f.Prediction.OperationalTETA
	f.FrozenOperationalTETA = &teta
	late := *f.Slot
	late.Time = late.Time.Add(15 * time.Minute)
	f.Slot = &late
	f.FrozenSlot = &late
	mutation, err := service.RebuildArrivalSequence(context.Background(), aman.CommandContext{Role: "EKDK_FMP", ReceivedAt: now}, aman.RebuildArrivalSequenceCommand{Metadata: aman.CommandMetadata{CommandID: "recapture", ExpectedRevision: state.Revision}})
	require.NoError(t, err)
	change, err := mutation(state)
	require.NoError(t, err)
	require.NoError(t, change.State.Validate())
	rebuilt := change.State.Flights[0]
	require.Equal(t, aman.FreezeSuperstable, rebuilt.FreezeReason)
	require.True(t, rebuilt.Slot.Time.Before(late.Time))
	require.Equal(t, rebuilt.Slot.Time, rebuilt.FrozenSlot.Time)
	require.Equal(t, teta, *rebuilt.FrozenOperationalTETA)

	repository := &reconciliationConflictRepository{memoryRepository: memoryRepository{state: state, has: true}, remaining: 1}
	publisher := &recordingPublisher{}
	coordinator, err := sequence.NewCoordinator(sequence.CoordinatorDependencies{States: repository, Outcomes: repository, Committer: repository, Publisher: publisher, Now: func() time.Time { return now }})
	require.NoError(t, err)
	actions, err := sequence.NewActionService(coordinator, service)
	require.NoError(t, err)
	_, err = actions.RebuildArrivalSequence(context.Background(), aman.CommandContext{Airport: "EKCH", Actor: "1234567", Role: "EKDK_FMP", ReceivedAt: now}, aman.RebuildArrivalSequenceCommand{Metadata: aman.CommandMetadata{CommandID: "concurrent", ExpectedRevision: state.Revision}})
	requireDomainClass(t, err, aman.ErrorRevisionConflict)
	require.Empty(t, publisher.states)
	require.Equal(t, late.Time, repository.state.Flights[0].Slot.Time)
}

func TestRebuildAssignsTMAFromNowBeforeOutsideTrafficAndSurvivesReconciliation(t *testing.T) {
	service, state, now := rebuildApproachStackFixture(t)
	for i, id := range []aman.Callsign{"TMA1", "TMA2"} {
		f := state.Flights[0]
		f.Callsign = id
		f.HoldingEpisode, f.HoldingStack, f.HoldingClearance, f.HoldingReleaseBasis = nil, nil, nil, nil
		f.HoldingReleaseReservation, f.ApproachRelease = nil, nil
		p := *f.Prediction
		p.RawTETA = now.Add(time.Duration(i+1) * time.Hour)
		p.OperationalTETA = p.RawTETA
		f.Prediction = &p
		old := *f.Slot
		old.Time = p.RawTETA
		if i == 0 {
			// Matching timing alone must not reuse a capture without the
			// rebuilt-TMA reason needed for the new 90-second cadence.
			old.Time, old.Sequence, old.Reason = now.Add(90*time.Second), 1, string(sequence.ReasonFreezeTMA)
			capturedAt := now.Add(-time.Minute)
			f.FreezeReason, f.FrozenAt = aman.FreezeTMA, &capturedAt
			f.FrozenOperationalTETA, f.FrozenSlot = &old.Time, &old
			p.OperationalTETA, p.OperationalReason = old.Time, aman.OperationalReasonTMAFreeze
		}
		f.Slot = &old
		state.Flights = append(state.Flights, f)
	}
	state.RunwayGroups[0].SameSTARSpacing = &aman.SameSTARSpacingPolicy{Enabled: true, ActivationRatePerHour: 20, MinimumEmptySlots: 1}
	mutation, err := service.RebuildArrivalSequence(context.Background(), aman.CommandContext{Role: "EKDK_FMP", ReceivedAt: now}, aman.RebuildArrivalSequenceCommand{Metadata: aman.CommandMetadata{CommandID: "near-tma", ExpectedRevision: state.Revision}})
	require.NoError(t, err)
	change, err := mutation(state)
	require.NoError(t, err)
	for i := 2; i < 4; i++ {
		got := change.State.Flights[i]
		want := now.Add(time.Duration(i-1) * 90 * time.Second)
		require.Equal(t, want, got.Slot.Time)
		require.Equal(t, want, got.Prediction.OperationalTETA)
		require.Equal(t, want, *got.FrozenOperationalTETA)
		require.Equal(t, aman.FreezeTMA, got.FreezeReason)
		require.Equal(t, string(sequence.ReasonRebuildTMA), got.FrozenSlot.Reason)
	}
	for _, f := range change.State.Flights[:2] {
		require.True(t, f.Slot.Time.After(now.Add(3*time.Minute)), "outside traffic follows the immediate TMA queue")
	}
	var outcome aman.ArrivalSequenceRebuildOutcome
	require.NoError(t, json.Unmarshal(change.Outcome, &outcome))
	require.Empty(t, outcome.Diagnostics)
	require.Equal(t, now, *change.State.Flights[2].FrozenAt, "a changed capture reason requires a new capture")
	require.Equal(t, string(sequence.ReasonFreezeTMA), state.Flights[2].FrozenSlot.Reason, "the input capture must remain untouched")
	again, err := mutation(change.State)
	require.NoError(t, err)
	require.False(t, again.Changed, "an already rebuilt capture must still support a semantic no-op")
	candidate, err := sequence.Generate(service.sequenceInput(change.State))
	require.NoError(t, err)
	require.False(t, candidate.HasConflicts(), "ordinary sequencing must accept the rebuilt TMA slots")
	got := change.State
	service.resequence(&got, now)
	require.Empty(t, got.RunwayGroups[0].SequenceWarnings, "routine spacing must accept the rebuilt TMA cadence")
	for i := 2; i < 4; i++ {
		require.Equal(t, change.State.Flights[i].Slot.Time, got.Flights[i].Slot.Time)
	}
	encoded, err := json.Marshal(change.State)
	require.NoError(t, err)
	var restored aman.AirportState
	require.NoError(t, json.Unmarshal(encoded, &restored))
	service.resequence(&restored, now)
	require.Empty(t, restored.RunwayGroups[0].SequenceWarnings)
	require.Equal(t, now.Add(90*time.Second), restored.Flights[2].Slot.Time)
}

func TestRebuildAssignsTMATimeWithoutAUsablePositionEstimate(t *testing.T) {
	service, state, now := recomputeFlightFixture(t)
	f := &state.Flights[0]
	f.Prediction = nil
	f.DataStatus = aman.DataDisconnected
	f.LatestObservation.SourceStatus = aman.DataDisconnected
	wake := "M"
	f.LatestObservation.WakeCategory = &wake
	f.TMAEntry = &aman.TMAEntryState{LastContainment: aman.TMAInside, LastObservedAt: now.Add(-10 * time.Minute)}
	mutation, err := service.RebuildArrivalSequence(context.Background(), aman.CommandContext{Role: "EKDK_FMP", ReceivedAt: now}, aman.RebuildArrivalSequenceCommand{Metadata: aman.CommandMetadata{CommandID: "missing-tma"}})
	require.NoError(t, err)
	change, err := mutation(state)
	require.NoError(t, err)
	got := change.State.Flights[0]
	require.Equal(t, now.Add(90*time.Second), got.Slot.Time)
	require.Equal(t, got.Slot.Time, *got.FrozenOperationalTETA)
	require.Nil(t, got.Prediction)
	require.NoError(t, change.State.Validate())
	var outcome aman.ArrivalSequenceRebuildOutcome
	require.NoError(t, json.Unmarshal(change.Outcome, &outcome))
	require.Empty(t, outcome.Diagnostics)
}

func TestRebuildDoesNotPresentExpiredPositionEstimateAsFreshTiming(t *testing.T) {
	service, state, now := recomputeFlightFixture(t)
	f := &state.Flights[0]
	oldObserved, capturedAt := now.Add(-10*time.Minute), now.Add(-15*time.Minute)
	target := now.Add(20 * time.Minute)
	f.FreezeReason = aman.FreezeTMA
	f.FrozenAt, f.FrozenOperationalTETA = &capturedAt, &target
	f.Prediction.InputObservedAt, f.Prediction.OperationalTETA = oldObserved, target
	f.LatestObservation.Surveillance.ObservedAt = &oldObserved
	f.Prediction.OperationalReason = aman.OperationalReasonTMAFreeze
	f.Slot.Time = target
	captured := *f.Slot
	f.FrozenSlot = &captured
	mutation, err := service.RebuildArrivalSequence(context.Background(), aman.CommandContext{Role: "EKDK_FMP", ReceivedAt: now}, aman.RebuildArrivalSequenceCommand{Metadata: aman.CommandMetadata{CommandID: "expired", ExpectedRevision: state.Revision}})
	require.NoError(t, err)
	change, err := mutation(state)
	require.NoError(t, err)
	require.Nil(t, change.State.Flights[0].Slot)
	require.Nil(t, change.State.Flights[0].FrozenAt)
	require.Nil(t, change.State.Flights[0].Prediction)
	require.NoError(t, change.State.Validate())
	require.Equal(t, target, state.Flights[0].Slot.Time, "input state must remain untouched")
	var outcome aman.ArrivalSequenceRebuildOutcome
	require.NoError(t, json.Unmarshal(change.Outcome, &outcome))
	require.Contains(t, outcome.Diagnostics, aman.RebuildDiagnostic{Code: "current_prediction_unavailable", Callsign: f.Callsign, Message: "A current position-based estimate is unavailable; no slot is assigned"})
}

func TestRebuildRetriesStalePredictionFromAvailableCurrentRadar(t *testing.T) {
	service, state, now := recomputeFlightFixture(t)
	f := &state.Flights[0]
	physicalETA := f.Prediction.RawTETA
	f.DataStatus = aman.DataStale
	f.Prediction.InputObservedAt = now.Add(-10 * time.Minute)
	mutation, err := service.RebuildArrivalSequence(context.Background(), aman.CommandContext{Role: "EKDK_FMP", ReceivedAt: now}, aman.RebuildArrivalSequenceCommand{Metadata: aman.CommandMetadata{CommandID: "retry-physics", ExpectedRevision: state.Revision}})
	require.NoError(t, err)
	change, err := mutation(state)
	require.NoError(t, err)
	got := change.State.Flights[0]
	require.Equal(t, aman.DataFresh, got.DataStatus)
	require.Equal(t, *got.LatestObservation.Surveillance.ObservedAt, got.Prediction.InputObservedAt)
	require.Equal(t, physicalETA, got.Prediction.OperationalTETA)
	require.False(t, got.Slot.Time.Before(physicalETA))
}

func TestRebuildDiscardsAllPreviousApproachAdmissions(t *testing.T) {
	for _, kind := range []string{"future", "due", "observed", "manual"} {
		t.Run(kind, func(t *testing.T) {
			service, state, now := rebuildApproachStackFixture(t)
			f := &state.Flights[0]
			switch kind {
			case "due":
				f.ApproachRelease.ReleaseAt = now.Add(-time.Minute)
			case "observed":
				f.ApproachRelease.ReleaseAt = now.Add(-2 * time.Minute)
				released := now.Add(-time.Minute)
				f.ApproachRelease.ReleasedAt = &released
			case "manual":
				f.FreezeReason = aman.FreezeManual
				f.FrozenAt = &now
				f.FrozenSlot = f.Slot
				f.FrozenOperationalTETA = &f.Prediction.OperationalTETA
				f.Prediction.OperationalReason = aman.OperationalReasonManualOverride
			}
			oldRelease := f.ApproachRelease.ReleaseAt
			mutation, err := service.RebuildArrivalSequence(context.Background(), aman.CommandContext{Role: "EKDK_FMP", ReceivedAt: now}, aman.RebuildArrivalSequenceCommand{Metadata: aman.CommandMetadata{CommandID: "fresh"}})
			require.NoError(t, err)
			change, err := mutation(state)
			require.NoError(t, err)
			upper, lower := change.State.Flights[0], change.State.Flights[1]
			require.NotNil(t, upper.Prediction.HoldingPlan)
			require.NotEqual(t, oldRelease, upper.Prediction.HoldingPlan.ApproachReleaseTime)
			require.True(t, lower.Prediction.HoldingPlan.ApproachReleaseTime.Before(upper.Prediction.HoldingPlan.ApproachReleaseTime))
			require.Nil(t, upper.Prediction.HoldingPlanBlockedBy)
			require.NotEqual(t, aman.FreezeManual, upper.FreezeReason)
			if upper.ApproachRelease != nil {
				require.Nil(t, upper.ApproachRelease.ReleasedAt)
				require.Equal(t, upper.Prediction.HoldingPlan.ApproachReleaseTime, upper.ApproachRelease.ReleaseAt)
			}
			var outcome aman.ArrivalSequenceRebuildOutcome
			require.NoError(t, json.Unmarshal(change.Outcome, &outcome))
			require.Empty(t, outcome.Diagnostics)
			require.Equal(t, oldRelease, state.Flights[0].ApproachRelease.ReleaseAt, "input admissions must remain untouched")
		})
	}
}

func TestRebuildClearsUnavailableReservationsAndOldSpacingWarnings(t *testing.T) {
	service, state, now := rebuildApproachStackFixture(t)
	f := &state.Flights[0]
	f.DataStatus = aman.DataDisconnected
	f.LatestObservation.SourceStatus = aman.DataDisconnected
	oldAt := now.Add(-time.Hour)
	f.Prediction.InputObservedAt = oldAt
	f.RawTETASamples = []aman.RawTETASample{{TETA: f.Prediction.RawTETA, GeneratedAt: oldAt}}
	order := 1
	f.ManualOrder = &order
	f.ManualPosition = &aman.ManualSequencePosition{BeforeCallsign: "LOW"}
	f.HoldingReleaseReservation = &aman.HoldingReleaseReservation{CommandID: "old", EpisodeID: f.HoldingEpisode.ID, HoldingID: *f.SelectedHolding, Level: 12000, Rank: 1}
	f.ApproachRelease.ReleaseAt = now.Add(-time.Minute)
	state.RunwayGroups[0].SequenceWarnings = []aman.RunwayGroupSequenceWarning{{Code: "protected_same_star_spacing", Callsign: "UPPER", RelatedCallsign: "LOW", STARFamily: "STAR"}}
	mutation, err := service.RebuildArrivalSequence(context.Background(), aman.CommandContext{Role: "EKDK_FMP", ReceivedAt: now}, aman.RebuildArrivalSequenceCommand{Metadata: aman.CommandMetadata{CommandID: "clear"}})
	require.NoError(t, err)
	change, err := mutation(state)
	require.NoError(t, err)
	got := change.State.Flights[0]
	require.Nil(t, got.Slot)
	require.Nil(t, got.Order)
	require.Nil(t, got.ManualOrder)
	require.Nil(t, got.ManualPosition)
	require.Nil(t, got.ApproachRelease)
	require.Nil(t, got.HoldingReleaseReservation)
	require.Nil(t, got.Prediction)
	require.Empty(t, got.RawTETASamples)
	require.Empty(t, change.State.RunwayGroups[0].SequenceWarnings)
	require.NotNil(t, change.State.Flights[1].Slot)
	require.NotNil(t, state.Flights[0].Slot)
	require.Len(t, state.RunwayGroups[0].SequenceWarnings, 1, "the revision-checked input must not change")
}

func TestRebuildPreservesPhysicalFeederEvidenceAndClearsTimingOverrides(t *testing.T) {
	for _, inTMA := range []bool{false, true} {
		for _, kind := range []string{"route", "passed", "manual with derived route", "legacy route", "manual only", "holding only"} {
			t.Run(fmt.Sprintf("tma=%t/%s", inTMA, kind), func(t *testing.T) {
				service, state, now := recomputeFlightFixture(t)
				f := &state.Flights[0]
				family, fix := "TESPI", "TNO"
				f.SelectedSTARFamily, f.SelectedFeederFix = &family, &fix
				progress := *f.RouteProgress
				progress.TerminalDigest = "current-terminal"
				f.RouteProgress = &progress
				if inTMA {
					f.TMAEntry = &aman.TMAEntryState{LastContainment: aman.TMAInside, LastObservedAt: now}
				}
				routeETA, manualETA := now.Add(2*time.Minute), now.Add(time.Hour)
				route := &aman.FeederETAState{Source: aman.FeederETASourceRoute, ETA: &routeETA}
				var want *aman.FeederETAState
				switch kind {
				case "route":
					f.FeederETA, f.DerivedFeederETA, want = route, route, route
				case "passed":
					passed := &aman.FeederETAState{Source: aman.FeederETASourcePassed, Passed: true}
					f.FeederETA, f.DerivedFeederETA, want = passed, passed, passed
					f.Prediction.Calculation = &aman.PredictionCalculation{
						NoWindDuration: time.Minute, Duration: time.Minute,
						Legs: []aman.PredictionLeg{{ID: "final", From: fix, To: "RUNWAY", DistanceNM: 5, Duration: time.Minute}},
					}
				case "manual with derived route":
					f.FeederETA = &aman.FeederETAState{Source: aman.FeederETASourceManual, ETA: &manualETA}
					f.DerivedFeederETA, want = route, route
				case "legacy route":
					f.FeederETA, f.DerivedFeederETA, want = route, nil, route
				case "manual only":
					f.FeederETA = &aman.FeederETAState{Source: aman.FeederETASourceManual, ETA: &manualETA}
					f.DerivedFeederETA = nil
				case "holding only":
					held := &aman.FeederETAState{Source: aman.FeederETASourceHolding, ETA: &manualETA}
					f.FeederETA, f.DerivedFeederETA = held, held
				}
				beforeProgress := sequenceSTARProgress(*f)
				if want != nil {
					require.NotNil(t, beforeProgress)
				}
				original, err := json.Marshal(state)
				require.NoError(t, err)
				mutation, err := service.RebuildArrivalSequence(context.Background(), aman.CommandContext{Role: "EKDK_FMP", ReceivedAt: now}, aman.RebuildArrivalSequenceCommand{Metadata: aman.CommandMetadata{CommandID: "feeder"}})
				require.NoError(t, err)
				change, err := mutation(state)
				require.NoError(t, err)
				got := change.State.Flights[0]
				require.Equal(t, want, got.FeederETA)
				require.Equal(t, want, got.DerivedFeederETA)
				require.Equal(t, beforeProgress, sequenceSTARProgress(got))
				require.NoError(t, change.State.Validate())
				after, err := json.Marshal(state)
				require.NoError(t, err)
				require.JSONEq(t, string(original), string(after), "rebuild must not mutate the source evidence")
			})
		}
	}
}

func TestRebuildKeepsFeederETAFromRetriedPrediction(t *testing.T) {
	service, state, now := recomputeFlightFixture(t)
	f := &state.Flights[0]
	want := cloneFeederETA(f.DerivedFeederETA)
	require.NotNil(t, want)
	f.FeederETA, f.DerivedFeederETA = nil, nil
	f.Prediction.InputObservedAt = now.Add(-10 * time.Minute)
	mutation, err := service.RebuildArrivalSequence(context.Background(), aman.CommandContext{Role: "EKDK_FMP", ReceivedAt: now}, aman.RebuildArrivalSequenceCommand{Metadata: aman.CommandMetadata{CommandID: "retry-feeder"}})
	require.NoError(t, err)
	change, err := mutation(state)
	require.NoError(t, err)
	got := change.State.Flights[0]
	require.Equal(t, *got.LatestObservation.Surveillance.ObservedAt, got.Prediction.InputObservedAt)
	require.Equal(t, want, got.FeederETA)
	require.Equal(t, want, got.DerivedFeederETA)
}
