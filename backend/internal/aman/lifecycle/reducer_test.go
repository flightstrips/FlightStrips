package lifecycle_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/compatibility"
	"FlightStrips/internal/aman/lifecycle"
	"FlightStrips/internal/aman/prediction"
	"github.com/stretchr/testify/require"
)

func TestReduceRunsFullLifecycleWithRecordedReasons(t *testing.T) {
	now := lifecycleTime()
	config := lifecycle.DefaultConfig()
	flight := lifecycleFlight(now, aman.StatePlanned)

	steps := []struct {
		event  lifecycle.Event
		want   aman.FlightState
		reason aman.LifecycleReason
	}{
		{event: event("airborne", lifecycle.EventAirborneDetected, now), want: aman.StateAirborne, reason: aman.LifecycleReasonAirborneDetected},
		{event: predictionEvent("unstable", now.Add(time.Minute), now.Add(46*time.Minute)), want: aman.StateUnstable, reason: aman.LifecycleReasonUnstableHorizon},
		{event: predictionEvent("stable", now.Add(6*time.Minute), now.Add(26*time.Minute)), want: aman.StateStable, reason: aman.LifecycleReasonStableHorizon},
		{event: event("go-around", lifecycle.EventGoAroundConfirmed, now.Add(7*time.Minute)), want: aman.StateGoAround, reason: aman.LifecycleReasonGoAroundConfirmed},
		{event: predictionEvent("re-enter", now.Add(8*time.Minute), now.Add(53*time.Minute)), want: aman.StateUnstable, reason: aman.LifecycleReasonUnstableHorizon},
		{event: event("landed", lifecycle.EventLandingConfirmed, now.Add(9*time.Minute)), want: aman.StateLanded, reason: aman.LifecycleReasonLandingConfirmed},
		{event: event("removed", lifecycle.EventLandedTimeout, now.Add(10*time.Minute)), want: aman.StateRemoved, reason: aman.LifecycleReasonLandedTimeout},
	}

	for _, step := range steps {
		result, err := lifecycle.Reduce(config, flight, step.event)
		require.NoError(t, err)
		require.NotNil(t, result.Transition)
		require.Equal(t, flight.State, result.Transition.From)
		require.Equal(t, step.want, result.Transition.To)
		require.Equal(t, step.reason, result.Transition.Reason)
		require.Equal(t, step.want, result.Flight.State)
		require.Equal(t, step.reason, result.Flight.Lifecycle.Reason)
		require.Equal(t, step.event.OccurredAt, result.Flight.Lifecycle.EnteredAt)
		flight = result.Flight
	}
	require.NoError(t, flight.Validate())
}

func TestReduceUsesExactHorizonAndDwellBoundaries(t *testing.T) {
	now := lifecycleTime()
	config := lifecycle.DefaultConfig()
	flight := lifecycleFlight(now, aman.StateAirborne)

	before, err := lifecycle.Reduce(config, flight, predictionEvent("outside-unstable", now.Add(time.Minute), now.Add(46*time.Minute+time.Nanosecond)))
	require.NoError(t, err)
	require.Equal(t, aman.StateAirborne, before.Flight.State)
	require.Nil(t, before.Transition)

	atUnstable, err := lifecycle.Reduce(config, before.Flight, predictionEvent("at-unstable", now.Add(2*time.Minute), now.Add(47*time.Minute)))
	require.NoError(t, err)
	require.Equal(t, aman.StateUnstable, atUnstable.Flight.State)

	beforeDwell, err := lifecycle.Reduce(config, atUnstable.Flight, predictionEvent("before-dwell", now.Add(4*time.Minute-time.Nanosecond), now.Add(24*time.Minute-time.Nanosecond)))
	require.NoError(t, err)
	require.Equal(t, aman.StateUnstable, beforeDwell.Flight.State)

	atDwell, err := lifecycle.Reduce(config, beforeDwell.Flight, predictionEvent("at-dwell", now.Add(4*time.Minute), now.Add(24*time.Minute)))
	require.NoError(t, err)
	require.Equal(t, aman.StateStable, atDwell.Flight.State)
	require.Equal(t, aman.LifecycleReasonStableHorizon, atDwell.Flight.Lifecycle.Reason)
}

func TestReduceKeepsDataStatusOrthogonalAndBlocksStaleAdvancement(t *testing.T) {
	now := lifecycleTime()
	config := lifecycle.DefaultConfig()
	flight := lifecycleFlight(now, aman.StateAirborne)

	stale, err := lifecycle.Reduce(config, flight, statusEvent("stale", now.Add(time.Minute), aman.DataStale))
	require.NoError(t, err)
	require.Equal(t, aman.StateAirborne, stale.Flight.State)
	require.Equal(t, aman.DataStale, stale.Flight.DataStatus)

	blocked, err := lifecycle.Reduce(config, stale.Flight, predictionEvent("stale-prediction", now.Add(2*time.Minute), now.Add(47*time.Minute)))
	require.NoError(t, err)
	require.Equal(t, aman.StateAirborne, blocked.Flight.State)
	require.Nil(t, blocked.Transition)

	disconnected, err := lifecycle.Reduce(config, blocked.Flight, statusEvent("disconnected", now.Add(3*time.Minute), aman.DataDisconnected))
	require.NoError(t, err)
	require.Equal(t, aman.StateAirborne, disconnected.Flight.State)
	require.Equal(t, aman.DataDisconnected, disconnected.Flight.DataStatus)

	restored, err := lifecycle.Reduce(config, disconnected.Flight, statusEvent("restored", now.Add(4*time.Minute), aman.DataFresh))
	require.NoError(t, err)
	advanced, err := lifecycle.Reduce(config, restored.Flight, predictionEvent("fresh-prediction", now.Add(5*time.Minute), now.Add(50*time.Minute)))
	require.NoError(t, err)
	require.Equal(t, aman.StateUnstable, advanced.Flight.State)
}

func TestReduceRestartsDisconnectedAndReconcilesBeforeResumingRemoval(t *testing.T) {
	now := lifecycleTime()
	config := lifecycle.DefaultConfig()
	flight := lifecycleFlight(now, aman.StateAirborne)

	missing, err := lifecycle.Reduce(config, flight, event("missing", lifecycle.EventFlightMissing, now.Add(time.Minute)))
	require.NoError(t, err)
	require.Equal(t, now.Add(time.Minute+config.RemovalTimeout), *missing.Flight.Lifecycle.Absence.RemovalDueAt)

	restartedEvent := event("restart-1", lifecycle.EventSourceRestarted, now.Add(time.Minute+20*time.Second))
	restarted, err := lifecycle.Reduce(config, missing.Flight, restartedEvent)
	require.NoError(t, err)
	require.Equal(t, aman.DataDisconnected, restarted.Flight.DataStatus)
	require.True(t, restarted.Flight.Lifecycle.ReconciliationPending)
	require.Nil(t, restarted.Flight.Lifecycle.Absence.RemovalDueAt)
	require.Equal(t, 40*time.Second, restarted.Flight.Lifecycle.Absence.Remaining)

	duplicate, err := lifecycle.Reduce(config, restarted.Flight, restartedEvent)
	require.NoError(t, err)
	require.True(t, duplicate.Duplicate)
	require.Equal(t, restarted.Flight, duplicate.Flight)
	_, err = lifecycle.Reduce(config, restarted.Flight, statusEvent("late-stale", now.Add(time.Minute+10*time.Second), aman.DataStale))
	requireDomainClass(t, err, aman.ErrorInvalidTransition)
	_, err = lifecycle.Reduce(config, restarted.Flight, statusEvent("restart-1", now.Add(2*time.Minute), aman.DataFresh))
	requireDomainClass(t, err, aman.ErrorInvalidTransition)

	persisted, err := json.Marshal(restarted.Flight)
	require.NoError(t, err)
	var restoredFlight aman.AMANFlight
	require.NoError(t, json.Unmarshal(persisted, &restoredFlight))
	require.NoError(t, restoredFlight.Validate())

	fresh, err := lifecycle.Reduce(config, restoredFlight, statusEvent("source-restored", now.Add(32*time.Minute), aman.DataFresh))
	require.NoError(t, err)
	require.True(t, fresh.Flight.Lifecycle.ReconciliationPending)
	require.Nil(t, fresh.Flight.Lifecycle.Absence.RemovalDueAt, "fresh status alone must not resume the timer")

	_, err = lifecycle.Reduce(config, fresh.Flight, event("premature-timeout", lifecycle.EventRemovalTimeout, now.Add(33*time.Minute)))
	requireDomainClass(t, err, aman.ErrorInvalidTransition)

	confirmedMissing, err := lifecycle.Reduce(config, fresh.Flight, event("fresh-snapshot-missing", lifecycle.EventFlightMissing, now.Add(34*time.Minute)))
	require.NoError(t, err)
	require.False(t, confirmedMissing.Flight.Lifecycle.ReconciliationPending)
	require.Equal(t, now.Add(34*time.Minute+40*time.Second), *confirmedMissing.Flight.Lifecycle.Absence.RemovalDueAt)

	_, err = lifecycle.Reduce(config, confirmedMissing.Flight, event("early-timeout", lifecycle.EventRemovalTimeout, now.Add(34*time.Minute+40*time.Second-time.Nanosecond)))
	requireDomainClass(t, err, aman.ErrorInvalidTransition)
	removed, err := lifecycle.Reduce(config, confirmedMissing.Flight, event("timeout", lifecycle.EventRemovalTimeout, now.Add(34*time.Minute+40*time.Second)))
	require.NoError(t, err)
	require.Equal(t, aman.StateRemoved, removed.Flight.State)
	require.Equal(t, aman.LifecycleReasonSourceDisappearance, removed.Flight.Lifecycle.Reason)
}

func TestReduceCurrentObservationCancelsPausedDisappearance(t *testing.T) {
	now := lifecycleTime()
	config := lifecycle.DefaultConfig()
	flight := lifecycleFlight(now, aman.StateAirborne)

	missing, err := lifecycle.Reduce(config, flight, event("missing", lifecycle.EventFlightMissing, now.Add(time.Minute)))
	require.NoError(t, err)
	disconnected, err := lifecycle.Reduce(config, missing.Flight, statusEvent("disconnected", now.Add(2*time.Minute), aman.DataDisconnected))
	require.NoError(t, err)
	fresh, err := lifecycle.Reduce(config, disconnected.Flight, statusEvent("fresh", now.Add(20*time.Minute), aman.DataFresh))
	require.NoError(t, err)
	observed, err := lifecycle.Reduce(config, fresh.Flight, event("observed", lifecycle.EventFlightObserved, now.Add(21*time.Minute)))
	require.NoError(t, err)
	require.False(t, observed.Flight.Lifecycle.ReconciliationPending)
	require.Nil(t, observed.Flight.Lifecycle.Absence)
}

func TestReduceSuddenAppearanceUsesDefensibleStateWithoutInventingFreeze(t *testing.T) {
	now := lifecycleTime()
	config := lifecycle.DefaultConfig()
	tests := []struct {
		name       string
		untilTETA  time.Duration
		wantState  aman.FlightState
		wantReview bool
	}{
		{name: "outside lifecycle horizons", untilTETA: config.UnstableHorizon + time.Nanosecond, wantState: aman.StateAirborne},
		{name: "at unstable horizon", untilTETA: config.UnstableHorizon, wantState: aman.StateUnstable},
		{name: "at stable horizon observes dwell", untilTETA: config.StableHorizon, wantState: aman.StateUnstable},
		{name: "outside freeze horizon observes dwell", untilTETA: config.SuperstableHorizon + time.Nanosecond, wantState: aman.StateUnstable},
		{name: "at freeze horizon requires review and dwell", untilTETA: config.SuperstableHorizon, wantState: aman.StateUnstable, wantReview: true},
		{name: "inside freeze horizon requires review and dwell", untilTETA: config.SuperstableHorizon - time.Second, wantState: aman.StateUnstable, wantReview: true},
	}

	for index, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flight := lifecycleFlight(now, aman.StatePlanned)
			flight.DataStatus = aman.DataDisconnected
			result, err := lifecycle.Reduce(config, flight, suddenEvent(fmt.Sprintf("sudden-%d", index), now.Add(time.Minute), now.Add(time.Minute+tt.untilTETA)))
			require.NoError(t, err)
			require.Equal(t, tt.wantState, result.Flight.State)
			require.Equal(t, aman.DataFresh, result.Flight.DataStatus)
			require.Equal(t, aman.LifecycleReasonSuddenAppearance, result.Flight.Lifecycle.Reason)
			require.Equal(t, aman.FreezeNone, result.Flight.FreezeReason)
			require.Nil(t, result.Flight.FrozenAt)
			require.Nil(t, result.Flight.FrozenOperationalTETA)
			require.Nil(t, result.Flight.FrozenSlot)
			require.Nil(t, result.Flight.Prediction, "the lifecycle reducer must not manufacture prediction history or confidence")
			require.Empty(t, result.Flight.RawTETASamples)
			if tt.wantReview {
				require.NotNil(t, result.Flight.OperationalException)
				require.Equal(t, aman.OperationalExceptionSuddenInsideFreeze, result.Flight.OperationalException.Reason)
				require.Equal(t, now.Add(time.Minute), result.Flight.OperationalException.DetectedAt)
			} else {
				require.Nil(t, result.Flight.OperationalException)
			}
			require.NoError(t, result.Flight.Validate())
		})
	}
}

func TestReduceExpiresOperationalPredictionWithoutLegacyFallback(t *testing.T) {
	now := lifecycleTime()
	config := lifecycle.DefaultConfig()
	flight := lifecycleFlight(now, aman.StateUnstable)
	prediction := rawPrediction(now, now.Add(30*time.Minute))
	prediction.OperationalTETA = prediction.RawTETA
	prediction.OperationalReason = aman.OperationalReasonPredicted
	flight.Prediction = &prediction

	stale, err := lifecycle.Reduce(config, flight, statusEvent("stale", now.Add(time.Minute), aman.DataStale))
	require.NoError(t, err)
	expired, err := lifecycle.Reduce(config, stale.Flight, event("prediction-expired", lifecycle.EventPredictionExpired, now.Add(2*time.Minute)))
	require.NoError(t, err)
	require.NotNil(t, expired.Flight.Prediction)
	require.False(t, expired.Flight.Prediction.Publishable)
	require.Equal(t, lifecycle.PredictionExpiredSourceData, *expired.Flight.Prediction.DegradationReason)
	require.Nil(t, compatibility.ProjectArrivalETA(aman.ModeReadOnly, expired.Flight.Prediction, now.Add(2*time.Minute), time.Hour))
	require.Nil(t, compatibility.ProjectArrivalETA(aman.ModeAuthoritative, expired.Flight.Prediction, now.Add(2*time.Minute), time.Hour))
	require.NoError(t, expired.Flight.Validate())
}

func TestReduceIsIdempotentAndRejectsOutOfOrderOrInvalidTransitions(t *testing.T) {
	now := lifecycleTime()
	config := lifecycle.DefaultConfig()
	flight := lifecycleFlight(now, aman.StatePlanned)
	firstEvent := event("airborne", lifecycle.EventAirborneDetected, now.Add(time.Minute))
	first, err := lifecycle.Reduce(config, flight, firstEvent)
	require.NoError(t, err)

	duplicate, err := lifecycle.Reduce(config, first.Flight, firstEvent)
	require.NoError(t, err)
	require.True(t, duplicate.Duplicate)
	require.Equal(t, first.Flight, duplicate.Flight)

	_, err = lifecycle.Reduce(config, first.Flight, predictionEvent("old", now, now.Add(45*time.Minute)))
	requireDomainClass(t, err, aman.ErrorInvalidTransition)
	_, err = lifecycle.Reduce(config, first.Flight, event("airborne", lifecycle.EventManualRemoval, now.Add(2*time.Minute)))
	requireDomainClass(t, err, aman.ErrorInvalidTransition)
	_, err = lifecycle.Reduce(config, first.Flight, event("cancel", lifecycle.EventPlannedCancellation, now.Add(2*time.Minute)))
	requireDomainClass(t, err, aman.ErrorInvalidTransition)
	_, err = lifecycle.Reduce(config, first.Flight, event("timeout", lifecycle.EventLandedTimeout, now.Add(2*time.Minute)))
	requireDomainClass(t, err, aman.ErrorInvalidTransition)

	cancelled, err := lifecycle.Reduce(config, lifecycleFlight(now, aman.StatePlanned), event("cancel-planned", lifecycle.EventPlannedCancellation, now.Add(time.Minute)))
	require.NoError(t, err)
	require.Equal(t, aman.StateRemoved, cancelled.Flight.State)
	require.Equal(t, aman.LifecycleReasonPlannedCancellation, cancelled.Flight.Lifecycle.Reason)

	removed, err := lifecycle.Reduce(config, lifecycleFlight(now, aman.StateStable), event("manual-remove", lifecycle.EventManualRemoval, now.Add(time.Minute)))
	require.NoError(t, err)
	require.Equal(t, aman.StateRemoved, removed.Flight.State)
}

func TestReduceRestartReplayIsEquivalent(t *testing.T) {
	now := lifecycleTime()
	config := lifecycle.DefaultConfig()
	flight := lifecycleFlight(now, aman.StateAirborne)
	unstable, err := lifecycle.Reduce(config, flight, predictionEvent("unstable", now.Add(time.Minute), now.Add(46*time.Minute)))
	require.NoError(t, err)

	persisted, err := json.Marshal(unstable.Flight)
	require.NoError(t, err)
	var restored aman.AMANFlight
	require.NoError(t, json.Unmarshal(persisted, &restored))
	require.NoError(t, restored.Validate())

	next := predictionEvent("stable", now.Add(6*time.Minute), now.Add(26*time.Minute))
	want, err := lifecycle.Reduce(config, unstable.Flight, next)
	require.NoError(t, err)
	got, err := lifecycle.Reduce(config, restored, next)
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestLifecycleAndPredictionPreserveCanonicalFreezePolicy(t *testing.T) {
	now := lifecycleTime()
	lifecycleConfig := lifecycle.DefaultConfig()
	state := lifecycleFlight(now, aman.StateUnstable)
	state.Lifecycle = &aman.LifecycleState{
		EnteredAt: now.Add(-lifecycleConfig.MinimumUnstableDwell), Reason: aman.LifecycleReasonUnstableHorizon,
		LastEventID: "unstable", LastEventFingerprint: "persisted", LastEventAt: now.Add(-lifecycleConfig.MinimumUnstableDwell),
	}

	stable, err := lifecycle.Reduce(lifecycleConfig, state, predictionEvent("stable", now, now.Add(20*time.Minute)))
	require.NoError(t, err)
	require.Equal(t, aman.StateStable, stable.Flight.State)
	holding := "north-hold"
	stable.Flight.SelectedHolding = &holding

	predictionConfig := prediction.DefaultConfig()
	raw := rawPrediction(now.Add(time.Minute), now.Add(20*time.Minute))
	holdingFix := raw.GeneratedAt.Add(predictionConfig.SuperstableHorizon)
	raw.HoldingFixETA = &holdingFix
	slot := aman.Slot{Time: now.Add(25 * time.Minute), RunwayGroupID: "north", Sequence: 3, Reason: "spacing"}
	frozen, err := prediction.Reduce(predictionConfig, stable.Flight, prediction.Input{Raw: raw, State: stable.Flight.State, Slot: &slot})
	require.NoError(t, err)
	require.Equal(t, aman.StateStable, frozen.Flight.State)
	require.Equal(t, aman.FreezeSuperstable, frozen.Flight.FreezeReason)
	require.Equal(t, slot, *frozen.Flight.FrozenSlot)

	drifted, err := prediction.Reduce(predictionConfig, frozen.Flight, prediction.Input{
		Raw: rawPrediction(now.Add(2*time.Minute), now.Add(24*time.Minute)), State: frozen.Flight.State, RouteRevision: true,
	})
	require.NoError(t, err)
	require.Equal(t, *frozen.Flight.FrozenOperationalTETA, drifted.Flight.Prediction.OperationalTETA)
	require.Positive(t, drifted.RawDrift)

	manual := now.Add(22 * time.Minute)
	overridden, err := prediction.Reduce(predictionConfig, drifted.Flight, prediction.Input{
		Raw: rawPrediction(now.Add(3*time.Minute), now.Add(23*time.Minute)), State: drifted.Flight.State, ManualOverride: &manual,
	})
	require.NoError(t, err)
	require.Equal(t, aman.FreezeManual, overridden.Flight.FreezeReason)

	released, err := prediction.Reduce(predictionConfig, overridden.Flight, prediction.Input{
		Raw: rawPrediction(now.Add(4*time.Minute), now.Add(23*time.Minute)), State: overridden.Flight.State, ReleaseFreeze: true,
	})
	require.NoError(t, err)
	require.Equal(t, aman.FreezeNone, released.Flight.FreezeReason)

	goAroundState, err := lifecycle.Reduce(lifecycleConfig, released.Flight, event("go-around", lifecycle.EventGoAroundConfirmed, now.Add(5*time.Minute)))
	require.NoError(t, err)
	goAround, err := prediction.Reduce(predictionConfig, goAroundState.Flight, prediction.Input{
		Raw: rawPrediction(now.Add(6*time.Minute), now.Add(30*time.Minute)), State: goAroundState.Flight.State, ConfirmedGoAround: true,
	})
	require.NoError(t, err)
	require.Equal(t, aman.StateGoAround, goAround.Flight.State)
	require.Equal(t, aman.FreezeNone, goAround.Flight.FreezeReason)
	require.Nil(t, goAround.Flight.Slot)
}

func lifecycleTime() time.Time {
	return time.Date(2026, time.July, 22, 12, 0, 0, 0, time.UTC)
}

func lifecycleFlight(now time.Time, state aman.FlightState) aman.AMANFlight {
	return aman.AMANFlight{
		ID: "flight-1", VATSIMCID: "1234567", CurrentCallsign: "SAS123", State: state,
		DataStatus: aman.DataFresh, FreezeReason: aman.FreezeNone, UpdatedAt: now,
	}
}

func event(id string, kind lifecycle.EventKind, at time.Time) lifecycle.Event {
	return lifecycle.Event{ID: id, Kind: kind, OccurredAt: at}
}

func predictionEvent(id string, at, operationalTETA time.Time) lifecycle.Event {
	return lifecycle.Event{ID: id, Kind: lifecycle.EventPredictionAccepted, OccurredAt: at, OperationalTETA: &operationalTETA}
}

func suddenEvent(id string, at, operationalTETA time.Time) lifecycle.Event {
	return lifecycle.Event{ID: id, Kind: lifecycle.EventSuddenAppearance, OccurredAt: at, OperationalTETA: &operationalTETA}
}

func statusEvent(id string, at time.Time, status aman.DataStatus) lifecycle.Event {
	return lifecycle.Event{ID: id, Kind: lifecycle.EventDataStatusChanged, OccurredAt: at, DataStatus: status}
}

func rawPrediction(generatedAt, rawTETA time.Time) aman.Prediction {
	return aman.Prediction{
		RawTETA: rawTETA, GeneratedAt: generatedAt, InputObservedAt: generatedAt,
		Confidence: aman.ConfidenceHigh, Publishable: true, DatasetVersion: "2607", GeometryDigest: "geometry",
		ModelVersion: "performance-wind-v1", ConfigVersion: "ekch-v1", Sources: []string{"vatsim", "airacnet"},
	}
}

func requireDomainClass(t *testing.T, err error, class aman.ErrorClass) {
	t.Helper()
	require.Error(t, err)
	var domainError *aman.DomainError
	require.True(t, errors.As(err, &domainError))
	require.Equal(t, class, domainError.Class)
}
