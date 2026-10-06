package operational

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/navdata"
	"FlightStrips/internal/aman/prediction"
	"FlightStrips/internal/aman/predictor"
	"FlightStrips/internal/aman/terminal"
	"FlightStrips/internal/aman/trajectory"
	"github.com/stretchr/testify/require"
)

func holdingReleaseFixture(now time.Time) (aman.AMANFlight, trajectory.Result, predictor.PerformanceWindInput) {
	hold, group := "LUGAS-HOLD", aman.RunwayGroupID("ARRIVAL-22L")
	cfl, track := int32(12000), 270.0
	flight := operationalFlight("HELD", group, "LUGAS", "M", now.Add(20*time.Minute))
	flight.State, flight.DataStatus, flight.SelectedHolding = aman.StateStable, aman.DataFresh, &hold
	flight.Slot = &aman.Slot{Time: now.Add(40 * time.Minute), RunwayGroupID: group, Sequence: 1, Revision: 1, Reason: "rate_wtc"}
	flight.HoldingClearance = &aman.HoldingClearance{Hold: "LUGAS", HoldType: aman.HoldingClearanceEnroute, ClearedAltitude: &cfl, ObservedAt: now}
	flight.HoldingStack = &aman.HoldingStackState{HoldingID: hold, FirstObservedAt: now.Add(-4 * time.Minute), CandidateObservedAt: now, ConsecutiveObservations: 2, Confirmed: true}
	entry := now.Add(20 * time.Minute)
	raw := acceptedRawPrediction(now, now.Add(32*time.Minute))
	raw.HoldingFixETA, raw.OperationalTETA = &entry, flight.Prediction.OperationalTETA
	flight.Prediction = &raw
	projection := trajectory.Result{
		SelectedHolding: &navdata.HoldingPattern{ID: navdata.HoldingID(hold), Fix: "LUGAS"},
		Remaining: []trajectory.RemainingLeg{
			{ID: "HOLDING_TO:LUGAS", To: "LUGAS", DistanceNM: 120, CourseTrueDegrees: 90,
				Start: navdata.Coordinate{LatitudeDeg: 56, LongitudeDeg: 10}, End: navdata.Coordinate{LatitudeDeg: 55.8, LongitudeDeg: 12.8}},
			{ID: "LUGAS-APPROACH", From: "LUGAS", To: "APPROACH", DistanceNM: 35, CourseTrueDegrees: 220,
				Start: navdata.Coordinate{LatitudeDeg: 55.8, LongitudeDeg: 12.8}, End: navdata.Coordinate{LatitudeDeg: 55.62, LongitudeDeg: 12.66}},
		},
	}
	input := predictor.PerformanceWindInput{PredictionAt: now, WeatherEvaluationAt: now, AircraftICAO: "A320",
		WakeTurbulenceCategory: predictor.CategoryMedium, AltitudeFeet: 14000, CruiseAltitudeFeet: 30000,
		CurrentGroundspeedKnots: 280, CurrentTrackTrueDegrees: &track, UseObservedGroundspeedBeforeTOD: true}
	return flight, projection, input
}

func TestConfirmedHoldingReleaseUsesFixToLandingAndSurvivesLapsAndRestart(t *testing.T) {
	now := time.Date(2026, time.October, 4, 19, 0, 0, 0, time.UTC)
	flight, projection, input := holdingReleaseFixture(now)
	wind := &countingUnavailableWind{}
	service := &Service{deps: Dependencies{Wind: wind}}
	require.NoError(t, service.captureHoldingReleaseBasis(context.Background(), &flight, projection, input))
	require.NotNil(t, flight.HoldingReleaseBasis)
	require.NoError(t, flight.HoldingReleaseBasis.Validate())

	// Independently estimate the canonical path at the holding CFL. Neither
	// the 120 NM PPOS prefix nor its track/speed-derived wind belongs to it.
	expected, err := predictor.EstimatePerformanceWind(context.Background(), nil, unavailableWind{}, predictor.PerformanceWindInput{
		PredictionAt: now, WeatherEvaluationAt: now, AircraftICAO: "A320", WakeTurbulenceCategory: predictor.CategoryMedium,
		AltitudeFeet: 12000, CruiseAltitudeFeet: 12000, CurrentGroundspeedKnots: 280, DescentConfirmed: true,
		Remaining: predictorLegs(projection.Remaining[1:]),
	}, predictor.PerformanceWindConfig{})
	require.NoError(t, err)
	require.Equal(t, expected.Duration, flight.HoldingReleaseBasis.PostHoldingTransit)
	first := holdingPlanForFlight(flight, *flight.Prediction)
	require.NotNil(t, first)
	require.Equal(t, flight.Slot.Time.Add(-expected.Duration), first.ApproachReleaseTime)
	requests := wind.requests

	for _, elapsed := range []time.Duration{time.Minute, 4 * time.Minute, time.Hour} {
		input.PredictionAt, input.WeatherEvaluationAt = now.Add(elapsed), now.Add(elapsed)
		input.AltitudeFeet, input.CurrentGroundspeedKnots = 7000, 190
		projection.Remaining[0].DistanceNM, projection.Remaining[0].CourseTrueDegrees = 3, 300
		projection.Remaining[0].Start = navdata.Coordinate{LatitudeDeg: 55.9, LongitudeDeg: 12.7}
		projection.Remaining[1].ID = "rematerialized-terminal-leg"
		require.NoError(t, service.captureHoldingReleaseBasis(context.Background(), &flight, projection, input))
		raw := acceptedRawPrediction(input.PredictionAt, input.PredictionAt.Add(50*time.Minute))
		entry := input.PredictionAt.Add(15 * time.Minute)
		raw.HoldingFixETA = &entry
		raw.HoldingPlan = holdingPlanForFlight(flight, raw)
		reduced, err := prediction.Reduce(prediction.DefaultConfig(), flight, prediction.Input{Raw: raw, State: flight.State, Slot: flight.Slot})
		require.NoError(t, err)
		flight = reduced.Flight
		state := aman.AirportState{Flights: []aman.AMANFlight{flight}}
		service.refreshHoldingPlans(&state)
		flight = state.Flights[0]
		require.Equal(t, raw.RawTETA, flight.Prediction.RawTETA, "the live arrival forecast still follows surveillance")
		require.Equal(t, first, flight.Prediction.HoldingPlan, "an overdue EAT must remain visible through later laps")
		require.NoError(t, flight.Prediction.HoldingPlan.Validate())
		require.Equal(t, expected.Duration, holdingTransit(flight), "sequencing must use the same retained transit")
	}
	require.Equal(t, requests, wind.requests, "routine holding updates must not recalculate the accepted transit")
	payload, err := json.Marshal(flight)
	require.NoError(t, err)
	var restored aman.AMANFlight
	require.NoError(t, json.Unmarshal(payload, &restored))
	require.Equal(t, first, holdingPlanForFlight(restored, *restored.Prediction))
	for _, move := range []time.Duration{-3 * time.Minute, 6 * time.Minute} {
		slot := *restored.Slot
		slot.Time = flight.Slot.Time.Add(move)
		restored.Slot = &slot
		plan := holdingPlanForFlight(restored, *restored.Prediction)
		require.NotNil(t, plan)
		require.Equal(t, first.ApproachReleaseTime.Add(move), plan.ApproachReleaseTime)
		require.Equal(t, first.PostHoldingTransit, plan.PostHoldingTransit)
	}
}

func TestHoldingReleaseBasisExpiresWithHoldOrRunwayAndRecalculatesChangedPath(t *testing.T) {
	now := time.Date(2026, time.October, 4, 19, 0, 0, 0, time.UTC)
	service := &Service{deps: Dependencies{Wind: unavailableWind{}}}
	flight, projection, input := holdingReleaseFixture(now)
	require.NoError(t, service.captureHoldingReleaseBasis(context.Background(), &flight, projection, input))
	first := *flight.HoldingReleaseBasis
	projection.Remaining[1].DistanceNM += 20
	require.NoError(t, service.captureHoldingReleaseBasis(context.Background(), &flight, projection, input))
	require.NotEqual(t, first.RouteDigest, flight.HoldingReleaseBasis.RouteDigest)
	require.Greater(t, flight.HoldingReleaseBasis.PostHoldingTransit, first.PostHoldingTransit)
	for _, change := range []string{"cancel hold", "different hold", "landing", "removal", "new runway", "ground"} {
		t.Run(change, func(t *testing.T) {
			copy := flight
			switch change {
			case "cancel hold":
				copy.HoldingClearance = nil
			case "different hold":
				copy.HoldingClearance = &aman.HoldingClearance{Hold: "OLPIB", HoldType: aman.HoldingClearanceEnroute}
			case "landing":
				copy.State = aman.StateLanded
			case "removal":
				clearSequencingState(&copy)
			case "new runway":
				assignFlightToRunwayGroup(&copy, "ARRIVAL-04R")
			case "ground":
				clearGroundedOperationalState(&copy)
			}
			state := aman.AirportState{Flights: []aman.AMANFlight{copy}}
			service.refreshHoldingPlans(&state)
			require.Nil(t, state.Flights[0].HoldingReleaseBasis)
		})
	}
}

func TestConfirmedHoldingCanPromoteIntoVacancyDespiteNextLapETA(t *testing.T) {
	now := time.Date(2026, time.October, 4, 19, 0, 0, 0, time.UTC)
	flight, _, _ := holdingReleaseFixture(now)
	group := *flight.SelectedRunwayGroup
	flight.Slot.Time, flight.Slot.Sequence = now.Add(12*time.Minute), 2
	flight.Prediction.RawTETA, flight.Prediction.OperationalTETA = now.Add(50*time.Minute), now.Add(3*time.Minute)
	flight.HoldingReleaseBasis = &aman.HoldingReleaseBasis{HoldingID: *flight.SelectedHolding, Fix: "LUGAS", RunwayGroupID: group,
		RouteDigest: "path", HoldingEntryTime: now.Add(-4 * time.Minute), PostHoldingTransit: 4 * time.Minute}
	lead := operationalFlight("LEAD", group, "OTHER", "M", now.Add(3*time.Minute))
	lead.State = aman.StateStable
	lead.Slot = &aman.Slot{Time: now.Add(3 * time.Minute), RunwayGroupID: group, Sequence: 1, Revision: 1, Reason: "rate_wtc"}
	lead.Prediction.RawTETA = now.Add(3 * time.Minute)
	state := aman.AirportState{Airport: "EKCH", Revision: 1, Flights: []aman.AMANFlight{lead, flight},
		RunwayGroups: []aman.RunwayGroupPolicy{{ID: group, ActiveRatePerHour: 20, RateEffectiveAt: &now}}}
	service := &Service{deps: Dependencies{Terminal: terminal.Configuration{RunwayGroups: []terminal.RunwayGroup{{ID: group}}}}}
	promotions := service.resequence(&state, now)
	require.Len(t, promotions, 1)
	require.Equal(t, now.Add(6*time.Minute), state.Flights[1].Slot.Time)
	require.Equal(t, now.Add(2*time.Minute), state.Flights[1].Prediction.HoldingPlan.ApproachReleaseTime)
	require.Equal(t, 4*time.Minute, state.Flights[1].HoldingReleaseBasis.PostHoldingTransit)
}

func TestLightHoldingReleaseRetainsRETAFixTransitAcrossSpeedChanges(t *testing.T) {
	now := time.Date(2026, time.October, 4, 19, 0, 0, 0, time.UTC)
	flight, projection, input := holdingReleaseFixture(now)
	input.WakeTurbulenceCategory = predictor.CategoryLight
	input.CurrentGroundspeedKnots = 140
	service := &Service{}
	require.NoError(t, service.captureHoldingReleaseBasis(context.Background(), &flight, projection, input))
	require.Equal(t, 15*time.Minute, flight.HoldingReleaseBasis.PostHoldingTransit)
	first := holdingPlanForFlight(flight, *flight.Prediction)
	input.CurrentGroundspeedKnots = 100
	projection.Remaining[0].DistanceNM = 1
	require.NoError(t, service.captureHoldingReleaseBasis(context.Background(), &flight, projection, input))
	require.Equal(t, first, holdingPlanForFlight(flight, *flight.Prediction))
}

func TestGoAroundClearsCapturedHoldingRelease(t *testing.T) {
	now := time.Date(2026, time.October, 4, 19, 0, 0, 0, time.UTC)
	flight, projection, input := holdingReleaseFixture(now)
	service := &Service{deps: Dependencies{Wind: unavailableWind{}, Terminal: terminal.Configuration{
		ConfigVersion: "test", RunwayGroups: []terminal.RunwayGroup{{ID: *flight.SelectedRunwayGroup}}}}}
	require.NoError(t, service.captureHoldingReleaseBasis(context.Background(), &flight, projection, input))
	flight.Prediction.HoldingPlan = holdingPlanForFlight(flight, *flight.Prediction)
	state := aman.AirportState{Airport: "EKCH", Revision: 1, Flights: []aman.AMANFlight{flight},
		RunwayGroups: []aman.RunwayGroupPolicy{{ID: *flight.SelectedRunwayGroup, ActiveRatePerHour: 20, RateEffectiveAt: &now}}}
	mutation, err := service.ReportGoAround(aman.CommandContext{Airport: "EKCH", Actor: "1234567", Role: "EKDK_FMP", ReceivedAt: now},
		aman.ReportGoAroundCommand{Metadata: aman.CommandMetadata{CommandID: "go-around", ExpectedRevision: 1}, Callsign: flight.Callsign, DetectedAt: now})
	require.NoError(t, err)
	change, err := mutation(state)
	require.NoError(t, err)
	updated := change.State.Flights[0]
	require.Equal(t, aman.StateGoAround, updated.State)
	require.Nil(t, updated.HoldingReleaseBasis)
	require.Nil(t, updated.HoldingStack)
	require.Nil(t, updated.Prediction.HoldingPlan)
	require.Nil(t, updated.Prediction.HoldingFixETA)
}

type holdingReleaseGeometry struct {
	terminalIdentityGeometry
	holding navdata.HoldingPattern
}

func (g holdingReleaseGeometry) ActiveGeometrySnapshot(ctx context.Context, airport navdata.AirportID) (navdata.ActiveGeometrySnapshot, error) {
	snapshot, err := g.terminalIdentityGeometry.ActiveGeometrySnapshot(ctx, airport)
	snapshot.Holdings = []navdata.HoldingPattern{g.holding}
	return snapshot, err
}

func TestReconcileConfirmedHoldingKeepsReleaseAcrossInboundAndOutboundPositions(t *testing.T) {
	service, state, now := recomputeFlightFixture(t)
	geometry := service.deps.Geometry.(terminalIdentityGeometry)
	hold := navdata.HoldingID("TESPI-HOLD")
	geometry.path.HoldingIDs = []navdata.HoldingID{hold}
	service.deps.Geometry = holdingReleaseGeometry{terminalIdentityGeometry: geometry,
		holding: navdata.HoldingPattern{ID: hold, Fix: "TESPI"}}
	service.deps.Terminal.Paths[0].SelectedHolding = hold
	flight := state.Flights[0]
	flight.Slot.Time = now.Add(30 * time.Minute)
	wake, aircraft, cfl := "M", "A320", int32(10000)
	var accepted *aman.HoldingPlan
	var previousRaw time.Time
	for i, latitude := range []float64{55.099, 55.101, 55.4, 55.098} {
		at := now.Add(time.Duration(i) * time.Minute)
		observation := *flight.LatestObservation
		surveillance := *observation.Surveillance
		altitude, gs, track := 10000-i*500, 240.0-float64(i)*20, float64(i)*90
		surveillance.LatitudeDegrees, surveillance.LongitudeDegrees = latitude, 12.1
		surveillance.AltitudeFeet, surveillance.GroundspeedKnots, surveillance.TrackTrueDegrees, surveillance.ObservedAt = &altitude, &gs, &track, &at
		observation.Surveillance, observation.ReconciledAt, observation.WakeCategory, observation.AircraftType = &surveillance, at, &wake, &aircraft
		observation.HoldingClearance = &aman.HoldingClearance{Hold: "TESPI", HoldType: aman.HoldingClearanceEnroute, ClearedAltitude: &cfl, ObservedAt: at}
		var err error
		flight, err = service.reconcileFlight(context.Background(), state, flight, observation, at)
		require.NoError(t, err)
		state.Flights = []aman.AMANFlight{flight}
		service.refreshHoldingPlans(&state)
		flight = state.Flights[0]
		if i == 0 {
			require.Nil(t, flight.HoldingReleaseBasis, "approaching the fix does not establish physical holding")
			continue
		}
		require.NotNil(t, flight.HoldingReleaseBasis)
		require.NotNil(t, flight.Prediction.HoldingPlan)
		if i == 1 {
			accepted = flight.Prediction.HoldingPlan
		} else {
			require.Equal(t, accepted, flight.Prediction.HoldingPlan)
			require.NotEqual(t, previousRaw, flight.Prediction.RawTETA, "physical prediction still responds to racetrack position")
		}
		previousRaw = flight.Prediction.RawTETA
		require.Equal(t, now.Add(30*time.Minute), flight.Slot.Time)
	}
}
