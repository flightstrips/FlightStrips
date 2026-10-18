package operational

import (
	"encoding/json"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/navdata"
	"github.com/stretchr/testify/require"
)

func TestRecoveredArrivalRestoresApproachBufferAfterRouteRejoin(t *testing.T) {
	bufferService, bufferState, at := approachFixture(t)
	for i := range bufferState.Flights {
		bufferState.Flights[i].Slot.Time = bufferState.Flights[i].Slot.Time.Add(5 * time.Minute)
	}
	service, state, observation, _ := arrivalChangeFixture(at)
	service.tmaVolume = bufferService.tmaVolume
	geometry := service.deps.Geometry.(arrivalChangeGeometry)
	// Introduce the aircraft near the terminal end of a long filed route,
	// initially outside the route corridor and tracking toward HOLD.
	for i, longitude := range []float64{-20, -4, -2, 1} {
		geometry.fixes[i].Position = navdata.Coordinate{LatitudeDeg: 1, LongitudeDeg: longitude}
	}
	service.deps.Geometry = geometry
	observation.WakeCategory, observation.AircraftType = stringPointer("M"), stringPointer("A320")
	observation.Surveillance.LatitudeDegrees, observation.Surveillance.LongitudeDegrees = 1.3, -2.9
	track, altitude, groundspeed := 108.0, 10000, 300.0
	observation.Surveillance.TrackTrueDegrees = &track
	flight, err := service.reconcileFlight(t.Context(), state, newFlight(observation, at), observation, at)
	require.NoError(t, err)
	require.Equal(t, "HOLD", flight.RouteProgress.RecoveryCandidateFix)

	// Persist/reload the provisional progress as a normal reconciliation does.
	flight = cloneGapState(t, aman.AirportState{Flights: []aman.AMANFlight{flight}}).Flights[0]
	later := at.Add(10 * time.Second)
	observation.Surveillance = &aman.SurveillanceFact{LatitudeDegrees: 1.19, LongitudeDegrees: -2.85,
		ObservedAt: &later, AltitudeFeet: &altitude, GroundspeedKnots: &groundspeed, TrackTrueDegrees: &track}
	observation.ReconciledAt = later
	flight, err = service.reconcileFlight(t.Context(), state, flight, observation, later)
	require.NoError(t, err, "a physical rejoin must not remain stuck behind provisional progress")
	require.Equal(t, aman.DataFresh, flight.DataStatus)
	require.Equal(t, later, flight.Prediction.InputObservedAt)
	require.Empty(t, flight.RouteProgress.RecoveryCandidateFix)
	require.NotNil(t, bufferService.predictedTMAEntry(flight))

	bufferState.Flights = append(bufferState.Flights, flight)
	bufferService.deps.Now = func() time.Time { return later }
	bufferService.refreshHoldingPlans(&bufferState)
	require.Equal(t, "ready", bufferState.ReleaseBuffer.Status)
	require.Zero(t, bufferState.ReleaseBuffer.Unknown)
	require.NotNil(t, bufferState.Flights[0].ApproachRelease, "the recovered arrival must no longer block independent intake")
}

func TestUnresolvedAirborneArrivalBlocksAdditionalIntake(t *testing.T) {
	for _, kind := range []string{"missing prediction", "unpublishable", "preliminary", "stale prediction", "outside horizon", "imminent entry", "subsecond observation", "newer observation", "planned", "landed", "removed"} {
		t.Run(kind, func(t *testing.T) {
			svc, state, at := approachFixture(t)
			for i := range state.Flights {
				state.Flights[i].Slot.Time = state.Flights[i].Slot.Time.Add(5 * time.Minute)
			}
			arrival := aman.AMANFlight{Callsign: "UNRESOLVED", State: aman.StateAirborne, DataStatus: aman.DataFresh, FreezeReason: aman.FreezeNone,
				LatestObservation: &aman.FlightObservation{}, TMAEntry: &aman.TMAEntryState{LastContainment: aman.TMAOutside, LastObservedAt: at}}
			observation := tmaObservation(at, 1, -10, 12000)
			arrival.LatestObservation = &observation
			if kind != "missing prediction" {
				prediction := acceptedRawPrediction(at, at.Add(20*time.Minute))
				prediction.OperationalTETA, prediction.OperationalReason = prediction.RawTETA, aman.OperationalReasonPredicted
				prediction.Calculation = &aman.PredictionCalculation{Legs: []aman.PredictionLeg{{To: "TMA", StartLatitude: 1, StartLongitude: -10, EndLatitude: 1, EndLongitude: 1, Duration: 11 * time.Minute}}}
				arrival.Prediction = &prediction
			}
			switch kind {
			case "unpublishable":
				arrival.Prediction.Publishable = false
			case "preliminary":
				arrival.Prediction.ModelVersion = "aman-airborne-v1"
			case "stale prediction":
				arrival.Prediction.InputObservedAt = at.Add(-5 * time.Minute)
			case "imminent entry":
				arrival.Prediction.Calculation.Legs[0].StartLongitude, arrival.Prediction.Calculation.Legs[0].Duration = -1, 2*time.Minute
			case "subsecond observation", "newer observation":
				arrival.Prediction.Calculation.Legs[0].StartLongitude, arrival.Prediction.Calculation.Legs[0].Duration = -1, 2*time.Minute
				arrival.LatestObservation.Surveillance.ObservedAt = timePointer(at.Add(750 * time.Millisecond))
				if kind == "newer observation" {
					arrival.LatestObservation.Surveillance.ObservedAt = timePointer(at.Add(time.Second + 750*time.Millisecond))
				}
				svc.deps.Now = func() time.Time { return at.Add(2 * time.Second) }
			case "planned":
				arrival.State = aman.StatePlanned
			case "landed":
				arrival.State = aman.StateLanded
			case "removed":
				arrival.State = aman.StateRemoved
			}
			state.Flights = append(state.Flights, arrival)
			svc.refreshHoldingPlans(&state)
			switch kind {
			case "outside horizon", "imminent entry", "subsecond observation", "planned", "landed", "removed":
				require.Equal(t, "ready", state.ReleaseBuffer.Status)
				require.Zero(t, state.ReleaseBuffer.Unknown)
				require.NotNil(t, state.Flights[0].ApproachRelease)
				if kind == "imminent entry" || kind == "subsecond observation" {
					require.Equal(t, 1, state.ReleaseBuffer.Incoming)
				}
			default:
				require.Equal(t, "degraded", state.ReleaseBuffer.Status)
				require.Equal(t, 1, state.ReleaseBuffer.Unknown)
				for _, f := range state.Flights[:6] {
					require.Nil(t, f.ApproachRelease, "unaccounted traffic must block new advancement")
				}
			}
		})
	}
}

func TestReleasedReservationRechecksBudgetWithoutHoldingTiming(t *testing.T) {
	for _, kind := range []string{"inside approach", "observed release", "stale surveillance", "unpublishable"} {
		t.Run(kind, func(t *testing.T) {
			svc, state, at := approachFixture(t)
			independent := state.Flights[1]
			independent.Slot.Time = at.Add(17 * time.Minute)
			state.Flights = state.Flights[:1]
			svc.refreshHoldingPlans(&state)
			f := &state.Flights[0]
			require.NotNil(t, f.ApproachRelease)
			accepted := *f.ApproachRelease
			f.HoldingStack, f.HoldingReleaseBasis, f.HoldingClearance, f.SelectedHolding = nil, nil, nil, nil
			f.Prediction.HoldingPlan, f.Prediction.HoldingFixETA = nil, nil
			f.TMAEntry = &aman.TMAEntryState{LastContainment: aman.TMAInside, LastObservedAt: at}
			f.Slot.Time = at.Add(40 * time.Minute)
			switch kind {
			case "observed release":
				f.ApproachRelease.ReleasedAt = &at
			case "stale surveillance":
				f.DataStatus = aman.DataStale
			case "unpublishable":
				f.Prediction.Publishable = false
			}
			state.Flights = append(state.Flights, independent)
			svc.refreshHoldingPlans(&state)
			f = &state.Flights[0]
			require.Equal(t, "degraded", state.ReleaseBuffer.Status)
			require.Contains(t, state.ReleaseBuffer.BlockedReasons, "approach_absorption_budget_exceeded")
			require.Equal(t, accepted.ReleaseAt, f.ApproachRelease.ReleaseAt)
			require.Equal(t, accepted.Transit, f.ApproachRelease.Transit)
			require.Equal(t, 28*time.Minute, f.ApproachRelease.AdditionalApproachTime)
			require.Nil(t, f.Prediction.HoldingPlan)
			require.Nil(t, state.Flights[1].ApproachRelease)
			require.Equal(t, 1, state.ReleaseBuffer.Projected())
		})
	}
}

func TestRETALateReleaseSpacesFollowersFromObservedDeparture(t *testing.T) {
	svc, state, _ := recomputeFlightFixture(t) // Exercises the production WTC/L RETA branch.
	first := state.Flights[0]
	at := first.Prediction.InputObservedAt
	require.Equal(t, aman.PredictionBasisRETA, first.Prediction.Basis)
	require.Nil(t, first.Prediction.Calculation, "RETA must not invent a performance calculation")
	first.TMAEntry = &aman.TMAEntryState{LastContainment: aman.TMAInside, LastObservedAt: at}
	first.ApproachRelease = &aman.ApproachReleaseReservation{Episode: "ORIGIN/1", HoldingID: "ORIGIN", STARFamily: first.STARFamilyIdentity(), FeederFix: *first.SelectedFeederFix,
		RunwayGroupID: *first.SelectedRunwayGroup, ReleaseAt: at.Add(-3 * time.Minute), HoldingEntryAt: at.Add(-5 * time.Minute), Transit: 12 * time.Minute, RouteDigest: first.Prediction.GeometryDigest, PolicyVersion: "test"}
	first.Slot.Time = at.Add(12 * time.Minute)
	follower, _, _ := holdingReleaseFixture(at)
	follower.SelectedRunwayGroup, follower.Slot.RunwayGroupID = first.SelectedRunwayGroup, *first.SelectedRunwayGroup
	follower.SelectedSTARFamily, follower.SelectedFeederFix = first.SelectedSTARFamily, first.SelectedFeederFix
	follower.Slot.Time = at.Add(18 * time.Minute)
	follower.HoldingReleaseBasis = &aman.HoldingReleaseBasis{HoldingID: *follower.SelectedHolding, Fix: "LUGAS", RunwayGroupID: *first.SelectedRunwayGroup,
		RouteDigest: "physical", HoldingEntryTime: follower.HoldingStack.FirstObservedAt, PostHoldingTransit: 12 * time.Minute}
	observation := tmaObservation(at, 1, -1, 12000)
	follower.LatestObservation.Surveillance = observation.Surveillance
	svc.deps.Terminal.ApproachRelease = &aman.ApproachReleasePolicy{Enabled: true, TargetBuffer: 10, TaperStart: 6, MaxAdvanceSeconds: 600, LookaheadSeconds: 120}
	svc.tmaVolume = testTMAService(t).tmaVolume
	svc.deps.Now = func() time.Time { return at }
	state.Flights = []aman.AMANFlight{first, follower}
	// Route evidence must survive the same persisted aggregate used after restart.
	encoded, err := json.Marshal(state)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(encoded, &state))
	svc.refreshHoldingPlans(&state)
	require.NotNil(t, state.Flights[0].ApproachRelease.ReleasedAt)
	require.Equal(t, at, *state.Flights[0].ApproachRelease.ReleasedAt)
	require.Nil(t, state.Flights[1].ApproachRelease, "three-minute entry spacing lies outside the current two-minute admission window")
	departureAt := at
	at = at.Add(time.Minute)
	for i := range state.Flights {
		state.Flights[i].LatestObservation.Surveillance.ObservedAt = &at
	}
	svc.refreshHoldingPlans(&state)
	require.Equal(t, departureAt, *state.Flights[0].ApproachRelease.ReleasedAt, "later observations cannot move the first accepted departure")
	require.NotNil(t, state.Flights[1].ApproachRelease)
	require.Equal(t, departureAt.Add(3*time.Minute), state.Flights[1].ApproachRelease.ReleaseAt)
	require.Equal(t, "ready", state.ReleaseBuffer.Status)
}

func TestRETAReleaseRequiresFreshPhysicalRouteEvidence(t *testing.T) {
	_, state, _ := recomputeFlightFixture(t)
	flight := state.Flights[0]
	at := flight.Prediction.InputObservedAt
	reservation := aman.ApproachReleaseReservation{HoldingID: "ORIGIN", ReleaseAt: at.Add(-time.Minute)}
	require.True(t, observedPastReservedHold(flight, reservation))
	observation := *flight.LatestObservation
	surveillance := *observation.Surveillance
	surveillance.ObservedAt = timePointer(at.Add(750 * time.Millisecond))
	observation.Surveillance = &surveillance
	flight.LatestObservation = &observation
	require.True(t, observedPastReservedHold(flight, reservation), "the accepted prediction uses this same subsecond radar report rounded to seconds")
	for _, kind := range []string{"hold ahead", "holding ETA", "stale prediction", "newer observation", "before EAT", "missing geometry"} {
		t.Run(kind, func(t *testing.T) {
			f, r := flight, reservation
			prediction := *flight.Prediction
			f.Prediction = &prediction
			switch kind {
			case "hold ahead":
				prediction.RemainingRouteFixes = []string{"ORIGIN", "TNO"}
			case "holding ETA":
				prediction.HoldingFixETA = timePointer(at.Add(time.Minute))
			case "stale prediction":
				prediction.InputObservedAt = at.Add(-time.Minute)
			case "newer observation":
				observation := *flight.LatestObservation
				surveillance := *observation.Surveillance
				surveillance.ObservedAt = timePointer(at.Add(time.Second + 750*time.Millisecond))
				observation.Surveillance = &surveillance
				f.LatestObservation = &observation
			case "before EAT":
				r.ReleaseAt = at.Add(time.Minute)
			case "missing geometry":
				prediction.RemainingRouteFixes = nil
			}
			require.False(t, observedPastReservedHold(f, r))
		})
	}
	legacy := *flight.Prediction
	legacy.RemainingRouteFixes = nil
	legacy.Calculation = &aman.PredictionCalculation{Legs: []aman.PredictionLeg{{To: "TNO"}}}
	flight.Prediction = &legacy
	require.True(t, observedPastReservedHold(flight, reservation), "older performance predictions retain usable geometry")
}

func TestLateObservedDeparturePreservesDueEATAndBlocksIndependentAdvance(t *testing.T) {
	svc, state, at := approachFixture(t)
	base := at
	independent := state.Flights[2]
	independent.Slot.Time = base.Add(20 * time.Minute)
	state.Flights = state.Flights[:2]
	first, second := &state.Flights[0], &state.Flights[1]
	second.SelectedSTARFamily, second.SelectedHolding, second.SelectedFeederFix = first.SelectedSTARFamily, first.SelectedHolding, first.SelectedFeederFix
	second.HoldingClearance.Hold = first.HoldingClearance.Hold
	second.HoldingStack.HoldingID = first.HoldingStack.HoldingID
	second.HoldingStack.FirstObservedAt = base.Add(-3 * time.Minute)
	second.HoldingReleaseBasis.HoldingID, second.HoldingReleaseBasis.Fix = first.HoldingReleaseBasis.HoldingID, first.HoldingReleaseBasis.Fix
	second.HoldingReleaseBasis.HoldingEntryTime = second.HoldingStack.FirstObservedAt
	svc.refreshHoldingPlans(&state)
	require.Equal(t, base, first.ApproachRelease.ReleaseAt)
	dueEAT := base.Add(90 * time.Second)
	require.Equal(t, dueEAT, second.ApproachRelease.ReleaseAt)
	at = base.Add(3 * time.Minute)
	svc.deps.Now = func() time.Time { return at }
	first.HoldingStack, first.HoldingReleaseBasis, first.HoldingClearance, first.SelectedHolding = nil, nil, nil, nil
	first.Prediction.HoldingFixETA, first.Prediction.HoldingPlan = nil, nil
	first.Prediction.RemainingRouteFixes = []string{"APPROACH"}
	first.RouteProgress = &aman.RouteProgress{GeometryDigest: "physical-path"}
	first.TMAEntry = &aman.TMAEntryState{LastContainment: aman.TMAInside, LastObservedAt: at}
	state.Flights = append(state.Flights, independent)
	for i := range state.Flights {
		state.Flights[i].LatestObservation.Surveillance.ObservedAt = &at
		state.Flights[i].Prediction.InputObservedAt = at
	}
	svc.refreshHoldingPlans(&state)
	require.Equal(t, "degraded", state.ReleaseBuffer.Status)
	require.Contains(t, state.ReleaseBuffer.BlockedReasons, "approach_release_spacing_conflict")
	require.Equal(t, base, state.Flights[0].ApproachRelease.ReleaseAt)
	require.Equal(t, at, *state.Flights[0].ApproachRelease.ReleasedAt)
	require.Equal(t, dueEAT, state.Flights[1].ApproachRelease.ReleaseAt)
	require.Equal(t, dueEAT, state.Flights[1].Prediction.HoldingPlan.ApproachReleaseTime, "the published EAT remains irrevocable")
	require.Nil(t, state.Flights[2].ApproachRelease, "an independent feeder receives no additional advancement during the conflict")
	require.Equal(t, 2, state.ReleaseBuffer.Projected())
}

func TestRetainedReleaseBaseSpacingSurvivesMissingHoldingTiming(t *testing.T) {
	for _, observed := range []bool{false, true} {
		for _, kind := range []string{"20 per hour", "40 per hour", "before rate decrease", "after rate decrease", "missing prediction", "unpublishable", "stale", "missing assignment"} {
			name := "due/"
			if observed {
				name = "observed/"
			}
			t.Run(name+kind, func(t *testing.T) {
				svc, state, at := approachFixture(t)
				independent := state.Flights[2]
				independent.Slot.Time = at.Add(20 * time.Minute)
				state.Flights = state.Flights[:2]
				state.RunwayGroups[0].ActiveRatePerHour = 20
				conflict := true
				switch kind {
				case "40 per hour":
					state.RunwayGroups[0].ActiveRatePerHour = 40
					conflict = false
				case "before rate decrease", "after rate decrease":
					changeAt := at.Add(-20 * time.Second)
					if kind == "after rate decrease" {
						changeAt = at.Add(-time.Minute)
					} else {
						conflict = false
					}
					state.RunwayGroups[0].RateSchedule = []aman.RunwayGroupRatePoint{{EffectiveAt: at.Add(-time.Hour), ArrivalsPerHour: 40}, {EffectiveAt: changeAt, ArrivalsPerHour: 20}}
				}
				leaderEAT, leaderDeparture := at.Add(-5*time.Minute), at.Add(-2*time.Minute)
				followerEAT, followerDeparture := at.Add(-30*time.Second), at.Add(-30*time.Second)
				if observed {
					followerEAT = at.Add(-2 * time.Minute)
				}
				for i := range state.Flights {
					f := &state.Flights[i]
					f.SelectedSTARFamily, f.SelectedFeederFix = state.Flights[0].SelectedSTARFamily, state.Flights[0].SelectedFeederFix
					f.ApproachRelease = &aman.ApproachReleaseReservation{Episode: "accepted/" + f.Callsign, HoldingID: *f.SelectedHolding,
						STARFamily: f.STARFamilyIdentity(), FeederFix: *f.SelectedFeederFix, RunwayGroupID: *f.SelectedRunwayGroup,
						HoldingEntryAt: at.Add(-10 * time.Minute), Transit: 12 * time.Minute, RouteDigest: "physical-path", PolicyVersion: "test"}
					if i == 0 {
						f.ApproachRelease.ReleaseAt, f.ApproachRelease.ReleasedAt = leaderEAT, &leaderDeparture
					} else {
						f.ApproachRelease.ReleaseAt = followerEAT
						if observed {
							f.ApproachRelease.ReleasedAt = &followerDeparture
						}
					}
					f.Slot.Time = f.ApproachRelease.ReleaseAt.Add(f.ApproachRelease.Transit)
					f.HoldingStack, f.HoldingReleaseBasis, f.HoldingClearance, f.SelectedHolding = nil, nil, nil, nil
					f.Prediction.HoldingPlan, f.Prediction.HoldingFixETA = nil, nil
					f.TMAEntry = &aman.TMAEntryState{LastContainment: aman.TMAInside, LastObservedAt: at}
					if i == 1 {
						switch kind {
						case "missing prediction":
							f.Prediction = nil
						case "unpublishable":
							f.Prediction.Publishable = false
						case "stale":
							f.DataStatus = aman.DataStale
						case "missing assignment":
							f.SelectedRunwayGroup = nil
						}
					}
					require.NoError(t, f.ApproachRelease.Validate())
				}
				state.Flights = append(state.Flights, independent)
				svc.refreshHoldingPlans(&state)
				require.Equal(t, leaderEAT, state.Flights[0].ApproachRelease.ReleaseAt)
				require.Equal(t, followerEAT, state.Flights[1].ApproachRelease.ReleaseAt, "accepted EATs stay unchanged despite missing timing")
				require.Equal(t, leaderDeparture, *state.Flights[0].ApproachRelease.ReleasedAt)
				if observed {
					require.Equal(t, followerDeparture, *state.Flights[1].ApproachRelease.ReleasedAt)
				}
				if state.Flights[1].Prediction != nil {
					require.Nil(t, state.Flights[1].Prediction.HoldingPlan)
				}
				if conflict {
					require.Equal(t, "degraded", state.ReleaseBuffer.Status)
					require.Contains(t, state.ReleaseBuffer.BlockedReasons, "approach_release_spacing_conflict", "90 seconds must not satisfy the 180-second base rate even with zero M-to-M wake spacing")
					require.Nil(t, state.Flights[2].ApproachRelease, "the base-rate conflict blocks independent advancement")
				} else {
					require.Equal(t, "ready", state.ReleaseBuffer.Status, "use the rate at the fixed entry time, not a later decrease")
					require.NotNil(t, state.Flights[2].ApproachRelease)
				}
			})
		}
	}
}
