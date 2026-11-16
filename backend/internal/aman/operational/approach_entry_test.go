package operational

import (
	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/navdata"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFilteredLandingPredictionKeepsFreshApproachWorkloadAcrossRestart(t *testing.T) {
	bufferService, bufferState, at := approachFixture(t)
	for i := range bufferState.Flights {
		bufferState.Flights[i].Slot.Time = bufferState.Flights[i].Slot.Time.Add(5 * time.Minute)
	}
	service, state, observation, _ := arrivalChangeFixture(at)
	service.tmaVolume = bufferService.tmaVolume
	geometry := service.deps.Geometry.(arrivalChangeGeometry)
	for i, longitude := range []float64{-20, -4, -2, 1} {
		geometry.fixes[i].Position = navdata.Coordinate{LatitudeDeg: 1, LongitudeDeg: longitude}
	}
	service.deps.Geometry = geometry
	observation.WakeCategory, observation.AircraftType = stringPointer("M"), stringPointer("A320")
	observation.Surveillance.LatitudeDegrees, observation.Surveillance.LongitudeDegrees = 1, -10
	flight, err := service.reconcileFlight(t.Context(), state, newFlight(observation, at), observation, at)
	require.NoError(t, err)
	require.NotNil(t, flight.ApproachEntry)
	require.True(t, flight.ApproachEntry.EntryAt.After(at.Add(30*time.Minute)))

	// A four-minute spike retains the landing estimate, while newer physical
	// surveillance must still account for the remote aircraft's TMA entry.
	flight.Prediction.RawTETA = flight.Prediction.RawTETA.Add(-4 * time.Minute)
	retained := *flight.Prediction
	later := at.Add(15 * time.Second)
	observation.Surveillance.ObservedAt, observation.ReconciledAt = &later, later
	flight, err = service.reconcileFlight(t.Context(), state, flight, observation, later)
	require.NoError(t, err)
	require.NotNil(t, flight.PredictionFilter.Pending)
	require.Equal(t, retained, *flight.Prediction, "workload capture must not bypass the landing ETA filter")
	require.Equal(t, later, flight.ApproachEntry.ObservedAt)
	require.NoError(t, flight.ApproachEntry.Validate())

	for _, publishable := range []bool{true, false} {
		copy := cloneGapState(t, bufferState)
		remote := cloneGapState(t, aman.AirportState{Flights: []aman.AMANFlight{flight}}).Flights[0]
		remote.Prediction.Publishable = publishable
		copy.Flights = append(copy.Flights, remote)
		bufferService.deps.Now = func() time.Time { return later }
		bufferService.refreshHoldingPlans(&copy)
		require.Equal(t, "ready", copy.ReleaseBuffer.Status)
		require.Zero(t, copy.ReleaseBuffer.Unknown)
		admissions, previews := 0, 0
		for _, f := range copy.Flights[:6] {
			if f.ApproachRelease != nil {
				admissions++
			}
			if f.ApproachGuidance != nil {
				previews++
			}
		}
		require.Equal(t, 5, admissions)
		require.Equal(t, 1, previews)
	}
}

func TestApproachWorkloadStillCountsImminentTrafficAndRejectsInvalidEvidence(t *testing.T) {
	for _, kind := range []string{"imminent", "newer observation", "stale surveillance", "unavailable calculation", "changed runway", "invalidated route"} {
		t.Run(kind, func(t *testing.T) {
			svc, state, at := approachFixture(t)
			for i := range state.Flights {
				state.Flights[i].Slot.Time = state.Flights[i].Slot.Time.Add(5 * time.Minute)
			}
			obs := tmaObservation(at, 1, -1, 12000)
			raw := acceptedRawPrediction(at, at.Add(20*time.Minute))
			raw.Calculation = &aman.PredictionCalculation{Legs: []aman.PredictionLeg{{To: "TMA", StartLatitude: 1, StartLongitude: -1, EndLatitude: 1, EndLongitude: 1, Duration: 2 * time.Minute}}}
			remote := aman.AMANFlight{Callsign: "REMOTE", State: aman.StateAirborne, DataStatus: aman.DataFresh,
				SelectedRunwayGroup: state.Flights[0].SelectedRunwayGroup, LatestObservation: &obs, Prediction: &raw}
			svc.captureApproachEntry(&remote, raw)
			require.NotNil(t, remote.ApproachEntry)
			retained := raw
			retained.InputObservedAt, retained.Publishable = at.Add(-time.Minute), false
			remote.Prediction = &retained
			switch kind {
			case "newer observation":
				obs.Surveillance.ObservedAt = timePointer(at.Add(time.Second))
				at = at.Add(time.Second)
			case "stale surveillance":
				at = at.Add(tmaSurveillanceFresh + time.Second)
			case "unavailable calculation":
				markPredictionNonPublishable(&remote, "route unavailable")
			case "changed runway":
				group := aman.RunwayGroupID("OTHER")
				remote.SelectedRunwayGroup = &group
			case "invalidated route":
				invalidateRunwayPath(&remote)
			}
			// Keep the independent holds current so only REMOTE can cause a block.
			for i := range state.Flights {
				state.Flights[i].LatestObservation.Surveillance.ObservedAt = &at
			}
			state.Flights = append(state.Flights, remote)
			svc.deps.Now = func() time.Time { return at }
			svc.refreshHoldingPlans(&state)
			if kind == "imminent" {
				require.Equal(t, "ready", state.ReleaseBuffer.Status)
				require.Equal(t, 1, state.ReleaseBuffer.Incoming)
				require.Zero(t, state.ReleaseBuffer.Unknown)
			} else {
				require.Equal(t, "degraded", state.ReleaseBuffer.Status)
				require.Equal(t, []string{"REMOTE"}, state.ReleaseBuffer.UnknownCallsigns)
				for _, f := range state.Flights[:6] {
					require.Nil(t, f.ApproachRelease)
					require.Nil(t, f.ApproachGuidance)
				}
			}
		})
	}
}

func TestPendingRunwayChangeDoesNotReuseRetainedApproachTiming(t *testing.T) {
	for _, stored := range []string{"absent", "previous runway", "selected runway"} {
		t.Run(stored, func(t *testing.T) {
			svc, state, at := approachFixture(t)
			for i := range state.Flights {
				state.Flights[i].Slot.Time = state.Flights[i].Slot.Time.Add(5 * time.Minute)
			}
			obs := tmaObservation(at, 1, -1, 12000)
			raw := acceptedRawPrediction(at, at.Add(20*time.Minute))
			raw.Calculation = &aman.PredictionCalculation{Legs: []aman.PredictionLeg{{To: "TMA", StartLatitude: 1, StartLongitude: -1, EndLatitude: 1, EndLongitude: 1, Duration: 2 * time.Minute}}}
			remote := aman.AMANFlight{Callsign: "REMOTE", State: aman.StateAirborne, DataStatus: aman.DataFresh,
				SelectedRunwayGroup: state.Flights[0].SelectedRunwayGroup, LatestObservation: &obs, Prediction: &raw}
			svc.captureApproachEntry(&remote, raw)
			assignFlightToRunwayGroup(&remote, "ARRIVAL-04R")
			require.True(t, remote.RunwayChangePending)
			require.True(t, remote.Prediction.Publishable)
			switch stored {
			case "absent":
				remote.ApproachEntry = nil
			case "selected runway":
				svc.captureApproachEntry(&remote, raw)
			}
			// A release refresh can run before the pending prediction is rebuilt,
			// including after this aggregate has been persisted and reloaded.
			remote = cloneGapState(t, aman.AirportState{Flights: []aman.AMANFlight{remote}}).Flights[0]
			state.Flights = append(state.Flights, remote)
			svc.refreshHoldingPlans(&state)
			if stored == "selected runway" {
				require.NotNil(t, svc.predictedTMAEntry(remote), "fresh entry for the selected runway takes priority over the pending flag")
				require.Equal(t, "ready", state.ReleaseBuffer.Status)
				require.Equal(t, 1, state.ReleaseBuffer.Incoming)
			} else {
				require.Nil(t, svc.predictedTMAEntry(remote))
				require.Equal(t, "degraded", state.ReleaseBuffer.Status)
				require.Equal(t, []string{"REMOTE"}, state.ReleaseBuffer.UnknownCallsigns)
				require.Zero(t, state.ReleaseBuffer.Incoming)
				for _, f := range state.Flights[:6] {
					require.Nil(t, f.ApproachRelease)
				}
			}
			remote.RunwayChangePending = false
			require.NotNil(t, svc.predictedTMAEntry(remote), "current legacy calculation is available after the rebuild clears the pending flag")
		})
	}
}
