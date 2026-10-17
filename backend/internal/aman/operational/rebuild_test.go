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

func TestRebuildRefreshesFrozenTMATimingFromCurrentPositionPrediction(t *testing.T) {
	service, state, now := recomputeFlightFixture(t)
	f := &state.Flights[0]
	physicalETA := f.Prediction.RawTETA
	oldETA, oldAt := now.Add(-5*time.Minute), now.Add(-10*time.Minute)
	f.FreezeReason = aman.FreezeTMA
	f.FrozenAt, f.FrozenOperationalTETA = &oldAt, &oldETA
	f.Prediction.OperationalTETA, f.Prediction.OperationalReason = oldETA, aman.OperationalReasonTMAFreeze
	f.Slot.Time = oldETA
	captured := *f.Slot
	f.FrozenSlot = &captured
	f.TMAEntry = &aman.TMAEntryState{LastContainment: aman.TMAInside, LastObservedAt: f.Prediction.InputObservedAt, FreezeTriggered: true}
	// A released aircraft can still lie in the published hold footprint. Its
	// ended episode must not turn current TMA traffic back into a held aircraft.
	hold := "OLD-HOLD"
	started, ended := now.Add(-25*time.Minute), now.Add(-20*time.Minute)
	f.SelectedHolding = &hold
	f.HoldingStack = &aman.HoldingStackState{HoldingID: hold, FirstObservedAt: started, CandidateObservedAt: f.Prediction.InputObservedAt, Confirmed: true, ConsecutiveObservations: 2}
	f.HoldingEpisode = &aman.HoldingEpisode{ID: "ended", HoldingID: hold, Fix: "FIX", StartedAt: &started, Source: "near_fix", EndedAt: &ended}
	mutation, err := service.RebuildArrivalSequence(context.Background(), aman.CommandContext{Role: "EKDK_FMP", ReceivedAt: now}, aman.RebuildArrivalSequenceCommand{Metadata: aman.CommandMetadata{CommandID: "refresh-tma", ExpectedRevision: state.Revision}})
	require.NoError(t, err)
	change, err := mutation(state)
	require.NoError(t, err)
	got := change.State.Flights[0]
	require.NoError(t, change.State.Validate())
	require.Equal(t, physicalETA, got.Prediction.OperationalTETA)
	require.False(t, got.Slot.Time.Before(physicalETA))
	require.True(t, got.Slot.Time.After(now))
	require.Equal(t, aman.FreezeTMA, got.FreezeReason)
	require.Equal(t, got.Slot.Time, got.FrozenSlot.Time)
	require.Equal(t, physicalETA, *got.FrozenOperationalTETA)
	require.Equal(t, now, *got.FrozenAt)
	require.Equal(t, oldETA, state.Flights[0].Slot.Time, "calculation must not mutate its revision-checked input")
	require.Contains(t, string(change.Audit[0].Payload), "before_capture")
	require.Len(t, got.RawTETASamples, 1, "routine smoothing must not restore the stale estimate")
	noOp, err := mutation(change.State)
	require.NoError(t, err)
	require.False(t, noOp.Changed)
	updated, err := service.reconcileFlight(context.Background(), change.State, got, *got.LatestObservation, now)
	require.NoError(t, err)
	require.Equal(t, physicalETA, updated.Prediction.OperationalTETA, "routine surveillance must retain the refreshed capture")
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
	require.Equal(t, target, change.State.Flights[0].Slot.Time)
	require.Equal(t, capturedAt, *change.State.Flights[0].FrozenAt)
	var outcome aman.ArrivalSequenceRebuildOutcome
	require.NoError(t, json.Unmarshal(change.Outcome, &outcome))
	require.Contains(t, outcome.Diagnostics, aman.RebuildDiagnostic{Code: "current_prediction_unavailable", Callsign: f.Callsign, Message: "A current position-based estimate is unavailable; the existing timing is retained"})
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

func TestRebuildRetainsDueAdmissionButWithholdsUnsafeRelease(t *testing.T) {
	service, state, now := rebuildApproachStackFixture(t)
	due := now.Add(-time.Minute)
	state.Flights[0].ApproachRelease.ReleaseAt = due
	mutation, err := service.RebuildArrivalSequence(context.Background(), aman.CommandContext{Role: "EKDK_FMP", ReceivedAt: now}, aman.RebuildArrivalSequenceCommand{Metadata: aman.CommandMetadata{CommandID: "due"}})
	require.NoError(t, err)
	change, err := mutation(state)
	require.NoError(t, err)
	upper := change.State.Flights[0]
	require.NotNil(t, upper.ApproachRelease)
	require.Equal(t, due, upper.ApproachRelease.ReleaseAt, "a due capacity commitment cannot be recalled")
	require.Nil(t, upper.Prediction.HoldingPlan, "the buffer must not restore an unsafe upper-first EAT")
	require.Equal(t, aman.Callsign("LOW"), *upper.Prediction.HoldingPlanBlockedBy)
	var outcome aman.ArrivalSequenceRebuildOutcome
	require.NoError(t, json.Unmarshal(change.Outcome, &outcome))
	found := false
	for _, d := range outcome.Diagnostics {
		if d.Code == "holding_release_blocked" && d.Callsign == "UPPER" {
			found = true
		}
	}
	require.True(t, found, "the retained admission exception must be reported")
}

func TestRebuildResetsOnlyFutureAutomaticApproachOffers(t *testing.T) {
	for _, kind := range []string{"future", "manual", "unavailable", "due", "observed"} {
		t.Run(kind, func(t *testing.T) {
			_, state, now := rebuildApproachStackFixture(t)
			f := state.Flights[0]
			input := sequence.Flight{Callsign: f.Callsign}
			switch kind {
			case "manual":
				input.FreezeReason = aman.FreezeManual
			case "unavailable":
				input.RebuildRetained = true
			case "due":
				f.ApproachRelease.ReleaseAt = now
			case "observed":
				at := f.ApproachRelease.ReleaseAt.Add(time.Second)
				f.ApproachRelease.ReleasedAt = &at
			}
			old := f.ApproachRelease
			plan := f.Prediction.HoldingPlan
			resetRebuiltApproachRelease(&f, input, now)
			if kind == "future" {
				require.Nil(t, f.ApproachRelease)
				require.Nil(t, f.Prediction.HoldingPlan)
				require.NotNil(t, state.Flights[0].Prediction.HoldingPlan, "candidate preparation must not mutate the source")
			} else {
				require.Equal(t, old, f.ApproachRelease)
				require.Equal(t, plan, f.Prediction.HoldingPlan)
			}
		})
	}
}
