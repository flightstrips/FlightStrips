package operational

import (
	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/lifecycle"
	"context"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestPreliminaryTimingCannotReserveCapacityOrTriggerAutomaticProtection(t *testing.T) {
	at := time.Date(2026, 7, 20, 18, 10, 0, 0, time.UTC)
	for _, origin := range []string{"EGLL", "EKBI"} {
		for _, airborne := range []bool{false, true} {
			t.Run(origin+map[bool]string{false: "/planned", true: "/first-seen-airborne"}[airborne], func(t *testing.T) {
				eet, eobt := 99*time.Minute, at
				if origin == "EKBI" {
					eet = 15 * time.Minute
				}
				observation := aman.FlightObservation{Origin: origin, PlannedTiming: &aman.PlannedTiming{EstimatedEnrouteTime: &eet, EstimatedOffBlockTime: &eobt}}
				if airborne {
					observation.TakeoffDetected = &at
				}
				f := superstableCandidate(at, feederETAAt(at.Add(time.Minute)))
				f.Prediction, f.Slot = nil, nil
				applyPreliminaryPrediction(&f, observation, at)
				eta := f.Prediction.RawTETA
				// Holding evidence and a near feeder must never protect this estimate.
				hold := "ROSBI"
				f.SelectedHolding = &hold
				f.HoldingStack = &aman.HoldingStackState{HoldingID: hold, Confirmed: true, FirstObservedAt: at}
				f.Slot = &aman.Slot{RunwayGroupID: *f.SelectedRunwayGroup, Time: eta, Sequence: 1}
				require.False(t, sequenceEligible(f))
				require.False(t, holdingSlotProtected(f))
				require.False(t, captureTMAFreeze(&f, at))
				require.False(t, applySuperstable(lifecycle.DefaultConfig(), &f, aman.FreezeNone, at))
				state := aman.AirportState{Flights: []aman.AMANFlight{f}, GeneratedAt: at}
				(&Service{}).resequence(&state, at)
				require.Nil(t, state.Flights[0].Slot)
				require.Nil(t, state.Flights[0].FrozenSlot)
				require.Equal(t, eta, state.Flights[0].Prediction.OperationalTETA, "advisory timing remains visible")
			})
		}
	}
}

func TestFirstPhysicalPredictionDiscardsPersistedPreliminarySlotAndFreeze(t *testing.T) {
	for _, freeze := range []aman.FreezeReason{aman.FreezeNone, aman.FreezeTMA, aman.FreezeSuperstable} {
		t.Run(string(freeze), func(t *testing.T) {
			svc, state, at := recomputeFlightFixture(t)
			f := state.Flights[0]
			f.State = aman.StateUnstable
			f.Prediction = nil
			f.Slot = nil
			elapsed := 99 * time.Minute
			applyPreliminaryPrediction(&f, aman.FlightObservation{TakeoffDetected: &at, PlannedTiming: &aman.PlannedTiming{EstimatedEnrouteTime: &elapsed}}, at)
			oldETA := f.Prediction.RawTETA
			f.Slot = &aman.Slot{RunwayGroupID: *f.SelectedRunwayGroup, Time: oldETA, Sequence: 1}
			f.FreezeReason = freeze
			if freeze != aman.FreezeNone {
				f.FrozenSlot = f.Slot
				f.FrozenAt, f.FrozenOperationalTETA = &at, &oldETA
			}
			// Reproduce a startup commitment loaded after a server restart.
			data, err := json.Marshal(f)
			require.NoError(t, err)
			require.NoError(t, json.Unmarshal(data, &f))
			observation := *f.LatestObservation
			position := *observation.Surveillance
			position.ObservedAt = &at
			observation.Surveillance = &position
			observation.ReconciledAt = at
			updated, err := svc.reconcileFlight(context.Background(), state, f, observation, at)
			require.NoError(t, err)
			require.False(t, isPreliminaryPrediction(updated.Prediction))
			require.Nil(t, updated.Slot)
			require.Nil(t, updated.FrozenSlot)
			require.Equal(t, aman.FreezeNone, updated.FreezeReason)
			require.Equal(t, updated.Prediction.RawTETA, updated.Prediction.OperationalTETA)
			state.Flights = []aman.AMANFlight{updated}
			svc.resequence(&state, at)
			require.NotNil(t, state.Flights[0].Slot)
			require.True(t, state.Flights[0].Slot.Time.Before(oldETA.Add(-30*time.Minute)))
		})
	}
}

func TestPreliminaryRecoveryPreservesManualAndAcceptedPhysicalCommitments(t *testing.T) {
	at := time.Date(2026, 7, 20, 18, 10, 0, 0, time.UTC)
	manual := superstableCandidate(at, feederETAAt(at.Add(time.Minute)))
	manual.Prediction.ModelVersion = "aman-airborne-takeoff-eet-v1"
	manual.FreezeReason = aman.FreezeManual
	manual.FrozenAt = &at
	manual.FrozenOperationalTETA = &manual.Prediction.OperationalTETA
	manual.FrozenSlot = manual.Slot
	prior := manual
	discardAutomaticPreliminarySlot(&manual)
	require.Equal(t, prior, manual)
	require.True(t, sequenceEligible(manual), "an explicit manual timing commitment remains eligible")

	physical := superstableCandidate(at, feederETAAt(at.Add(time.Minute)))
	physical.Prediction.ModelVersion = modelVersion
	require.True(t, applySuperstable(lifecycle.DefaultConfig(), &physical, aman.FreezeNone, at))
	physical.DataStatus = aman.DataDisconnected
	prior = physical
	discardAutomaticPreliminarySlot(&physical)
	require.Equal(t, prior, physical, "accepted physical commitments survive a source outage")
}
