package prediction

import (
	"encoding/json"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"github.com/stretchr/testify/require"
)

func TestReducePersistsMedianDeadbandRateLimitAndBypasses(t *testing.T) {
	now := predictionTime()
	config := DefaultConfig()
	config.Deadband = 30 * time.Second
	config.MaximumRoutineMove = 2 * time.Minute
	flight := predictionFlight(now)

	first, err := Reduce(config, flight, Input{Raw: rawPrediction(now, now.Add(20*time.Minute)), State: aman.StateStable})
	require.NoError(t, err)
	require.Equal(t, now.Add(20*time.Minute), first.Flight.Prediction.OperationalTETA)

	second, err := Reduce(config, first.Flight, Input{Raw: rawPrediction(now.Add(time.Minute), now.Add(20*time.Minute+10*time.Second)), State: aman.StateStable})
	require.NoError(t, err)
	require.Equal(t, now.Add(20*time.Minute), second.Flight.Prediction.OperationalTETA)
	require.Equal(t, aman.OperationalReasonDeadband, second.Flight.Prediction.OperationalReason)

	third, err := Reduce(config, second.Flight, Input{Raw: rawPrediction(now.Add(2*time.Minute), now.Add(30*time.Minute)), State: aman.StateStable})
	require.NoError(t, err)
	require.Len(t, third.Flight.RawTETASamples, 3)
	require.Equal(t, now.Add(20*time.Minute), third.Flight.Prediction.OperationalTETA)

	fourth, err := Reduce(config, third.Flight, Input{Raw: rawPrediction(now.Add(3*time.Minute), now.Add(30*time.Minute)), State: aman.StateStable})
	require.NoError(t, err)
	require.Len(t, fourth.Flight.RawTETASamples, 3)
	require.Equal(t, now.Add(22*time.Minute), fourth.Flight.Prediction.OperationalTETA)
	require.Equal(t, aman.OperationalReasonRateLimited, fourth.Flight.Prediction.OperationalReason)

	persisted, err := json.Marshal(third.Flight)
	require.NoError(t, err)
	var restored aman.AMANFlight
	require.NoError(t, json.Unmarshal(persisted, &restored))
	restarted, err := Reduce(config, restored, Input{Raw: rawPrediction(now.Add(3*time.Minute), now.Add(30*time.Minute)), State: aman.StateStable})
	require.NoError(t, err)
	require.Equal(t, fourth.Flight.RawTETASamples, restarted.Flight.RawTETASamples)
	require.Equal(t, fourth.Flight.Prediction.OperationalTETA, restarted.Flight.Prediction.OperationalTETA)

	route, err := Reduce(config, fourth.Flight, Input{Raw: rawPrediction(now.Add(4*time.Minute), now.Add(35*time.Minute)), State: aman.StateStable, RouteRevision: true})
	require.NoError(t, err)
	require.Equal(t, now.Add(35*time.Minute), route.Flight.Prediction.OperationalTETA)
	require.Equal(t, aman.OperationalReasonRouteRevision, route.Flight.Prediction.OperationalReason)

	unstable, err := Reduce(config, route.Flight, Input{Raw: rawPrediction(now.Add(5*time.Minute), now.Add(40*time.Minute)), State: aman.StateUnstable})
	require.NoError(t, err)
	require.Equal(t, now.Add(40*time.Minute), unstable.Flight.Prediction.OperationalTETA)
	require.Equal(t, aman.OperationalReasonFirstUnstable, unstable.Flight.Prediction.OperationalReason)
}

func TestReduceRestoresWindowAndSuperstableFreezeDeterministically(t *testing.T) {
	now := predictionTime()
	config := DefaultConfig()
	config.ExcessiveDrift = time.Minute
	slot := aman.Slot{Time: now.Add(25 * time.Minute), RunwayGroupID: "north", Sequence: 3, Reason: "spacing"}
	freezeRaw := rawPrediction(now, now.Add(20*time.Minute))
	holdingFix := now.Add(config.SuperstableHorizon)
	freezeRaw.HoldingFixETA = &holdingFix

	frozen, err := Reduce(config, predictionFlight(now), Input{Raw: freezeRaw, State: aman.StateStable, Slot: &slot})
	require.NoError(t, err)
	require.Equal(t, aman.FreezeSuperstable, frozen.Flight.FreezeReason)
	require.Equal(t, aman.OperationalReasonSuperstableFreeze, frozen.Flight.Prediction.OperationalReason)
	require.NotNil(t, frozen.Flight.FrozenSlot)
	require.Equal(t, slot, *frozen.Flight.FrozenSlot)

	persisted, err := json.Marshal(frozen.Flight)
	require.NoError(t, err)
	var restored aman.AMANFlight
	require.NoError(t, json.Unmarshal(persisted, &restored))
	require.NoError(t, restored.Validate())

	drifted, err := Reduce(config, restored, Input{Raw: rawPrediction(now.Add(time.Minute), now.Add(24*time.Minute)), State: aman.StateStable, RouteRevision: true})
	require.NoError(t, err)
	require.Equal(t, now.Add(20*time.Minute), drifted.Flight.Prediction.OperationalTETA)
	require.True(t, drifted.ExcessiveDrift)
	require.Equal(t, 4*time.Minute, drifted.RawDrift)

	manual := now.Add(22 * time.Minute)
	overridden, err := Reduce(config, drifted.Flight, Input{Raw: rawPrediction(now.Add(2*time.Minute), now.Add(23*time.Minute)), State: aman.StateStable, ManualOverride: &manual})
	require.NoError(t, err)
	require.Equal(t, aman.FreezeManual, overridden.Flight.FreezeReason)
	require.Equal(t, manual, overridden.Flight.Prediction.OperationalTETA)

	released, err := Reduce(config, overridden.Flight, Input{Raw: rawPrediction(now.Add(3*time.Minute), now.Add(23*time.Minute)), State: aman.StateStable, ReleaseFreeze: true})
	require.NoError(t, err)
	require.Equal(t, aman.FreezeNone, released.Flight.FreezeReason)
	require.NotEqual(t, aman.OperationalReasonManualOverride, released.Flight.Prediction.OperationalReason)

	goAround, err := Reduce(config, released.Flight, Input{Raw: rawPrediction(now.Add(4*time.Minute), now.Add(30*time.Minute)), State: aman.StateGoAround, ConfirmedGoAround: true})
	require.NoError(t, err)
	require.Equal(t, aman.FreezeNone, goAround.Flight.FreezeReason)
	require.Nil(t, goAround.Flight.Slot)
	require.Equal(t, now.Add(30*time.Minute), goAround.Flight.Prediction.OperationalTETA)
}

func TestReduceDoesNotFreezeWithoutSlotOrOutsideExactBoundary(t *testing.T) {
	now := predictionTime()
	config := DefaultConfig()
	raw := rawPrediction(now, now.Add(20*time.Minute))
	holdingFix := now.Add(config.SuperstableHorizon + time.Second)
	raw.HoldingFixETA = &holdingFix
	slot := aman.Slot{Time: now.Add(25 * time.Minute), RunwayGroupID: "north", Sequence: 1, Reason: "spacing"}

	result, err := Reduce(config, predictionFlight(now), Input{Raw: raw, State: aman.StateStable, Slot: &slot})
	require.NoError(t, err)
	require.Equal(t, aman.FreezeNone, result.Flight.FreezeReason)

	holdingFix = now.Add(config.SuperstableHorizon)
	raw = rawPrediction(now.Add(time.Minute), now.Add(20*time.Minute))
	holdingFix = raw.GeneratedAt.Add(config.SuperstableHorizon)
	raw.HoldingFixETA = &holdingFix
	result, err = Reduce(config, predictionFlight(now), Input{Raw: raw, State: aman.StateStable})
	require.NoError(t, err)
	require.Equal(t, aman.FreezeNone, result.Flight.FreezeReason)

	withoutSelectedHolding := predictionFlight(now)
	withoutSelectedHolding.SelectedHolding = nil
	result, err = Reduce(config, withoutSelectedHolding, Input{Raw: raw, State: aman.StateStable, Slot: &slot})
	require.NoError(t, err)
	require.Equal(t, aman.FreezeNone, result.Flight.FreezeReason)
}

func TestReduceIgnoresStaleRawAndFrozenSlotUpdates(t *testing.T) {
	now := predictionTime()
	config := DefaultConfig()
	slot := aman.Slot{Time: now.Add(25 * time.Minute), RunwayGroupID: "north", Sequence: 1, Reason: "spacing"}
	raw := rawPrediction(now, now.Add(20*time.Minute))
	holdingFix := now.Add(config.SuperstableHorizon)
	raw.HoldingFixETA = &holdingFix

	frozen, err := Reduce(config, predictionFlight(now), Input{Raw: raw, State: aman.StateStable, Slot: &slot})
	require.NoError(t, err)
	require.Equal(t, aman.FreezeSuperstable, frozen.Flight.FreezeReason)

	changedSlot := slot
	changedSlot.Time = changedSlot.Time.Add(time.Minute)
	current, err := Reduce(config, frozen.Flight, Input{Raw: rawPrediction(now.Add(time.Minute), now.Add(24*time.Minute)), State: aman.StateStable, Slot: &changedSlot})
	require.NoError(t, err)
	require.Equal(t, slot, *current.Flight.Slot)
	require.Equal(t, slot, *current.Flight.FrozenSlot)

	stale, err := Reduce(config, current.Flight, Input{Raw: rawPrediction(now, now.Add(40*time.Minute)), State: aman.StateStable})
	require.NoError(t, err)
	require.Equal(t, current.Flight, stale.Flight)
}

func TestOperationalOverrideActionsDoNotAcceptOrChangeRawPrediction(t *testing.T) {
	now := predictionTime()
	config := DefaultConfig()
	flight := predictionFlight(now)
	first, err := Reduce(config, flight, Input{Raw: rawPrediction(now, now.Add(30*time.Minute)), State: aman.StateUnstable})
	require.NoError(t, err)
	rawBefore := first.Flight.Prediction.RawTETA
	historyBefore := append([]aman.RawTETASample(nil), first.Flight.RawTETASamples...)

	manual := now.Add(25 * time.Minute)
	overridden, err := ApplyManualOperationalTETA(first.Flight, manual, now.Add(time.Minute))
	require.NoError(t, err)
	require.Equal(t, manual, overridden.Prediction.OperationalTETA)
	require.Equal(t, aman.OperationalReasonManualOverride, overridden.Prediction.OperationalReason)
	require.Equal(t, aman.FreezeManual, overridden.FreezeReason)
	require.Equal(t, rawBefore, overridden.Prediction.RawTETA)
	require.Equal(t, historyBefore, overridden.RawTETASamples)

	released, err := ReleaseManualOperationalTETA(config, overridden, now.Add(2*time.Minute))
	require.NoError(t, err)
	require.Equal(t, aman.FreezeNone, released.FreezeReason)
	require.Equal(t, rawBefore, released.Prediction.RawTETA)
	require.Equal(t, historyBefore, released.RawTETASamples)
	require.NotEqual(t, aman.OperationalReasonManualOverride, released.Prediction.OperationalReason)
}

func predictionTime() time.Time {
	return time.Date(2026, time.July, 22, 12, 0, 0, 0, time.UTC)
}

func predictionFlight(now time.Time) aman.AMANFlight {
	holding := "north-hold"
	return aman.AMANFlight{ID: "flight-1", VATSIMCID: "1234567", CurrentCallsign: "SAS123", State: aman.StateStable, DataStatus: aman.DataFresh, SelectedHolding: &holding, FreezeReason: aman.FreezeNone, UpdatedAt: now}
}

func rawPrediction(generatedAt, rawTETA time.Time) aman.Prediction {
	return aman.Prediction{RawTETA: rawTETA, GeneratedAt: generatedAt, InputObservedAt: generatedAt, Confidence: aman.ConfidenceHigh, Publishable: true, DatasetVersion: "2607", GeometryDigest: "geometry", ModelVersion: "performance-wind-v1", ConfigVersion: "ekch-v1", Sources: []string{"vatsim", "airacnet"}}
}
