package operational

import (
	"context"
	"errors"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/navdata"
	"FlightStrips/internal/aman/predictor"
	"FlightStrips/internal/aman/sequence"
	"FlightStrips/internal/aman/terminal"
	"FlightStrips/internal/aman/trajectory"
	"FlightStrips/internal/sat"
	"github.com/stretchr/testify/require"
)

func TestSequenceInputCarriesConfiguredSTARFamilySpacingAndWTC(t *testing.T) {
	start := time.Date(2026, time.July, 23, 12, 0, 0, 0, time.UTC)
	effective := start
	group := aman.RunwayGroupID("ARRIVAL-22")
	spacing := &aman.SameSTARSpacingPolicy{Enabled: true, ActivationRatePerHour: 20, MinimumEmptySlots: 1}
	wake := "M"
	feeder := "MONAK"
	state := aman.AirportState{
		Revision:     1,
		RunwayGroups: []aman.RunwayGroupPolicy{{ID: group, ActiveRatePerHour: 20, RateEffectiveAt: &effective, SameSTARSpacing: spacing}},
		Flights: []aman.AMANFlight{
			operationalFlight("ONE", group, feeder, wake, start),
			operationalFlight("TWO", group, feeder, wake, start),
		},
	}
	config := terminal.Configuration{RunwayGroups: []terminal.RunwayGroup{{ID: group}}}
	input := sequenceInput(state, config)
	require.Len(t, input.Policies, 1)
	require.Equal(t, sequence.SameSTARSpacing{Enabled: true, ActivationRatePerHour: 20, MinimumEmptySlots: 1}, input.Policies[0].SameSTARSpacing)
	require.Equal(t, feeder, input.Flights[0].STARFamily)

	result, err := sequence.Generate(input)
	require.NoError(t, err)
	require.Len(t, result.Entries, 2)
	require.Equal(t, 6*time.Minute, result.Entries[1].Time.Sub(result.Entries[0].Time))
}

func TestFeederRecognizesUniqueDownstreamTerminalPathJoin(t *testing.T) {
	group := aman.RunwayGroupID("ARRIVAL-22L")
	service := Service{deps: Dependencies{Terminal: terminal.Configuration{
		Feeders: []terminal.Feeder{{ID: "TESPI"}, {ID: "TUDLO"}},
		Paths: []terminal.Path{
			{Feeder: "TESPI", RunwayGroup: group, Fixes: []navdata.FixID{"TESPI", "ROSBI", "TNO", "SHARED"}},
			{Feeder: "TUDLO", RunwayGroup: group, Fixes: []navdata.FixID{"TUDLO", "LUGAS", "KOR", "SHARED"}},
		},
	}}}

	feeder, ok := service.feeder("AAL M725 ADSEN DCT TNO", group)

	require.True(t, ok)
	require.Equal(t, navdata.FeederID("TESPI"), feeder)
}

func TestFeederDoesNotGuessFromSharedTerminalPathFix(t *testing.T) {
	group := aman.RunwayGroupID("ARRIVAL-22L")
	service := Service{deps: Dependencies{Terminal: terminal.Configuration{
		Feeders: []terminal.Feeder{{ID: "TESPI"}, {ID: "TUDLO"}},
		Paths: []terminal.Path{
			{Feeder: "TESPI", RunwayGroup: group, Fixes: []navdata.FixID{"TESPI", "SHARED"}},
			{Feeder: "TUDLO", RunwayGroup: group, Fixes: []navdata.FixID{"TUDLO", "SHARED"}},
		},
	}}}

	_, ok := service.feeder("AAL DCT SHARED", group)

	require.False(t, ok)
}

func TestSequenceInputExcludesLightPistonsUntilControllerMove(t *testing.T) {
	start := time.Date(2026, time.July, 23, 12, 0, 0, 0, time.UTC)
	effective, group, wake, aircraftType := start, aman.RunwayGroupID("ARRIVAL-22"), "L", "C172"
	flight := operationalFlight("PISTON", group, "MONAK", wake, start.Add(10*time.Minute))
	flight.LatestObservation.AircraftType = &aircraftType
	state := aman.AirportState{Revision: 1, RunwayGroups: []aman.RunwayGroupPolicy{{ID: group, ActiveRatePerHour: 20, RateEffectiveAt: &effective}}, Flights: []aman.AMANFlight{flight}}
	config := terminal.Configuration{RunwayGroups: []terminal.RunwayGroup{{ID: group}}}
	aircraft := testAircraftEngines{engine: sat.EnginePiston, wtc: "L"}
	require.Empty(t, sequenceInputWithAircraft(state, config, aircraft).Flights)
	state.Flights[0].ManualSequenceIncluded = true
	require.Len(t, sequenceInputWithAircraft(state, config, aircraft).Flights, 1)
}

func TestHoldingStackRequiresConsecutiveGeometryObservations(t *testing.T) {
	start := time.Date(2026, time.July, 23, 12, 0, 0, 0, time.UTC)
	candidate := &trajectory.HoldingCandidate{HoldingID: "MONAK-HOLD"}
	first := updateHoldingStack(nil, candidate, start)
	require.False(t, first.Confirmed)
	require.Equal(t, uint32(1), first.ConsecutiveObservations)
	confirmed := updateHoldingStack(first, candidate, start.Add(time.Minute))
	require.True(t, confirmed.Confirmed)
	require.Equal(t, uint32(2), confirmed.ConsecutiveObservations)
	require.Nil(t, updateHoldingStack(confirmed, nil, start.Add(2*time.Minute)))
}

func TestResequenceRequeuesLateStableFlightWithoutMovingOtherStableSlots(t *testing.T) {
	start := time.Date(2026, time.July, 23, 12, 0, 0, 0, time.UTC)
	effective, group, wake := start, aman.RunwayGroupID("ARRIVAL-22"), "M"
	late := operationalFlight("LATE", group, "MONAK", wake, start.Add(6*time.Minute))
	late.State = aman.StateStable
	late.Slot = &aman.Slot{Time: start, RunwayGroupID: group, Sequence: 1, Reason: "rate_wtc"}
	other := operationalFlight("OTHER", group, "MONAK", wake, start.Add(3*time.Minute))
	other.State = aman.StateStable
	other.Slot = &aman.Slot{Time: start.Add(3 * time.Minute), RunwayGroupID: group, Sequence: 2, Reason: "rate_wtc"}
	state := aman.AirportState{RunwayGroups: []aman.RunwayGroupPolicy{{ID: group, ActiveRatePerHour: 20, RateEffectiveAt: &effective}}, Flights: []aman.AMANFlight{late, other}}
	service := &Service{deps: Dependencies{Terminal: terminal.Configuration{RunwayGroups: []terminal.RunwayGroup{{ID: group}}}}}
	targets := releaseGainResequenceTargets(&state)
	require.Contains(t, targets, aman.FlightID("LATE"))
	input := service.sequenceInput(state)
	for index := range input.Flights {
		if input.Flights[index].ID == "LATE" {
			input.Flights[index].ProtectCurrentSlot = false
			input.Flights[index].State = aman.StateUnstable
		}
	}
	preview, err := sequence.Generate(input)
	require.NoError(t, err)
	require.False(t, preview.HasConflicts())
	require.Equal(t, start.Add(6*time.Minute), preview.Entries[1].Time)
	service.resequence(&state, start)
	require.Equal(t, start.Add(6*time.Minute), state.Flights[0].Slot.Time)
	require.Equal(t, start.Add(3*time.Minute), state.Flights[1].Slot.Time)
}

type testAircraftEngines struct {
	engine sat.EngineType
	wtc    string
}

func (t testAircraftEngines) Lookup(string) (sat.EngineType, bool) { return t.engine, true }
func (t testAircraftEngines) LookupWTC(string) (string, bool)      { return t.wtc, true }

func TestPreliminaryPredictionsUseDocumentedPlannedAndAirborneTimes(t *testing.T) {
	now := time.Date(2026, time.July, 23, 12, 0, 0, 0, time.UTC)
	eobt := now.Add(time.Hour)
	eet := 90 * time.Minute
	observation := aman.FlightObservation{PlannedTiming: &aman.PlannedTiming{EstimatedOffBlockTime: &eobt, EstimatedEnrouteTime: &eet}}
	planned := aman.AMANFlight{State: aman.StatePlanned}
	applyPreliminaryPrediction(&planned, observation, now)
	require.Equal(t, eobt.Add(15*time.Minute+eet), planned.Prediction.RawTETA)
	require.Equal(t, "aman-planned-eobt-exot-eet-v1", planned.Prediction.ModelVersion)
	require.True(t, isPreliminaryPrediction(planned.Prediction))

	takeoff := now.Add(5 * time.Minute)
	observation.TakeoffDetected = &takeoff
	airborne := aman.AMANFlight{State: aman.StateAirborne}
	applyPreliminaryPrediction(&airborne, observation, now)
	require.Equal(t, takeoff.Add(eet), airborne.Prediction.RawTETA)
	require.Equal(t, "aman-airborne-takeoff-eet-v1", airborne.Prediction.ModelVersion)
	require.True(t, isPreliminaryPrediction(airborne.Prediction))
	physical := *airborne.Prediction
	physical.ModelVersion = modelVersion
	require.False(t, isPreliminaryPrediction(&physical))

	laterDetection := takeoff.Add(10 * time.Minute)
	observation.TakeoffDetected = &laterDetection
	anchored := aman.AMANFlight{
		State:           aman.StateAirborne,
		ArrivalBaseline: &aman.BaselineState{AirborneSensedAt: takeoff},
	}
	applyPreliminaryPrediction(&anchored, observation, now)
	require.Equal(t, takeoff.Add(eet), anchored.Prediction.RawTETA)
}

func TestServicePersistsLatestObservationAndRemovesAfterSixtySeconds(t *testing.T) {
	now := time.Date(2026, time.July, 23, 12, 0, 0, 0, time.UTC)
	repository := &memoryRepository{}
	publisher := &recordingPublisher{}
	service, err := New(Dependencies{
		Repository: repository, Materializer: unavailableNavigation{}, Geometry: unavailableGeometry{}, Wind: unavailableWind{},
		Publisher: publisher, Terminal: terminal.Configuration{Airport: "EKCH", ConfigVersion: "test", RunwayGroups: []terminal.RunwayGroup{{ID: "ARRIVAL-22"}}},
		Airports: []string{"EKCH"}, Mode: aman.ModeShadow, Now: func() time.Time { return now },
	})
	require.NoError(t, err)
	eobt, eet := now.Add(time.Hour), 90*time.Minute
	observedAt := now
	observation := aman.FlightObservation{
		FlightID: "flight-1", VATSIMCID: "123", Callsign: "SAS123", Origin: "ENGM", Destination: "EKCH",
		PlannedTiming: &aman.PlannedTiming{EstimatedOffBlockTime: &eobt, EstimatedEnrouteTime: &eet},
		FlightPlan:    aman.FlightPlanFact{ObservedAt: &observedAt}, ReconciledAt: now, SourceStatus: aman.DataFresh,
	}
	require.NoError(t, service.Observe(context.Background(), observation))
	require.NoError(t, service.reconcileAirport(context.Background(), "EKCH"))
	require.Equal(t, aman.SequenceRevision(1), repository.state.Revision)
	require.Equal(t, observation, *repository.state.Flights[0].LatestObservation)
	require.Equal(t, eobt.Add(15*time.Minute+eet), repository.state.Flights[0].Prediction.RawTETA)
	require.Len(t, publisher.states, 1)

	now = now.Add(time.Minute)
	observation.Missing, observation.ReconciledAt = true, now
	require.NoError(t, service.Observe(context.Background(), observation))
	require.NoError(t, service.reconcileAirport(context.Background(), "EKCH"))
	require.Equal(t, now.Add(time.Minute), *repository.state.Flights[0].Lifecycle.Absence.RemovalDueAt)

	now = now.Add(time.Minute)
	require.NoError(t, service.reconcileAirport(context.Background(), "EKCH"))
	require.Equal(t, aman.StateRemoved, repository.state.Flights[0].State)
}

func TestUnknownSTARFamilyRemainsDegradedAndSequenceable(t *testing.T) {
	now := time.Date(2026, time.July, 23, 12, 0, 0, 0, time.UTC)
	service, err := New(Dependencies{
		Repository: &memoryRepository{}, Materializer: unavailableNavigation{}, Geometry: unavailableGeometry{}, Wind: unavailableWind{},
		Publisher: &recordingPublisher{}, Terminal: terminal.Configuration{Airport: "EKCH", ConfigVersion: "test", RunwayGroups: []terminal.RunwayGroup{{
			ID: "ARRIVAL-22", SameSTARSpacing: &terminal.SameSTARSpacing{Enabled: true, ActivationRatePerHour: 20, MinimumEmptySlots: 1},
		}}},
		Airports: []string{"EKCH"}, Mode: aman.ModeShadow, Now: func() time.Time { return now },
	})
	require.NoError(t, err)
	takeoff, eet, route, wake := now.Add(-time.Minute), 30*time.Minute, "DCT NOTASTAR", "M"
	altitude, groundspeed, observedAt := 10000, 300.0, now
	observation := aman.FlightObservation{
		FlightID: "flight-unknown", VATSIMCID: "456", Callsign: "SAS456", Origin: "ENGM", Destination: "EKCH",
		FiledRoute: &route, WakeCategory: &wake, PlannedTiming: &aman.PlannedTiming{EstimatedEnrouteTime: &eet}, TakeoffDetected: &takeoff,
		FlightPlan: aman.FlightPlanFact{ObservedAt: &observedAt}, ReconciledAt: now, SourceStatus: aman.DataFresh,
		Surveillance: &aman.SurveillanceFact{
			LatitudeDegrees: 55, LongitudeDegrees: 12, AltitudeFeet: &altitude,
			GroundspeedKnots: &groundspeed, ObservedAt: &observedAt,
		},
	}
	state := service.initialState("EKCH", now)
	updated, err := service.reconcileFlight(context.Background(), state, newFlight(observation, now), observation, now)
	require.NoError(t, err)
	require.Nil(t, updated.SelectedFeeder)
	require.Equal(t, aman.RunwayGroupID("ARRIVAL-22"), *updated.SelectedRunwayGroup)
	require.NotNil(t, updated.Prediction)
	require.True(t, updated.Prediction.Publishable)
	require.Equal(t, string(sequence.WarningUnknownSTARFamily), *updated.Prediction.DegradationReason)

	state.Flights = []aman.AMANFlight{updated}
	input := sequenceInput(state, service.deps.Terminal)
	require.Len(t, input.Flights, 1)
	result, err := sequence.Generate(input)
	require.NoError(t, err)
	require.Len(t, result.Entries, 1)
	require.Contains(t, result.Warnings, sequence.Warning{
		Severity: sequence.SeverityDegraded, Code: sequence.WarningUnknownSTARFamily,
		RunwayGroupID: "ARRIVAL-22", FlightID: "flight-unknown",
	})
}

func TestAirbornePredictionIsNotPublishableWithoutEssentialInputs(t *testing.T) {
	now := time.Date(2026, time.July, 23, 12, 0, 0, 0, time.UTC)
	service, err := New(Dependencies{
		Repository: &memoryRepository{}, Materializer: unavailableNavigation{}, Geometry: unavailableGeometry{}, Wind: unavailableWind{},
		Publisher: &recordingPublisher{}, Terminal: terminal.Configuration{Airport: "EKCH", ConfigVersion: "test", RunwayGroups: []terminal.RunwayGroup{{ID: "ARRIVAL-22"}}},
		Airports: []string{"EKCH"}, Mode: aman.ModeShadow, Now: func() time.Time { return now },
	})
	require.NoError(t, err)
	takeoff, eet := now.Add(-time.Minute), 30*time.Minute
	observation := aman.FlightObservation{
		FlightID: "missing-input", VATSIMCID: "456", Callsign: "SAS456", Origin: "ENGM", Destination: "EKCH",
		PlannedTiming: &aman.PlannedTiming{EstimatedEnrouteTime: &eet}, TakeoffDetected: &takeoff,
		ReconciledAt: now, SourceStatus: aman.DataFresh,
	}

	updated, err := service.reconcileFlight(context.Background(), service.initialState("EKCH", now), newFlight(observation, now), observation, now)
	require.NoError(t, err)
	require.NotNil(t, updated.Prediction)
	require.False(t, updated.Prediction.Publishable)
	require.Equal(t, "missing_essential_data:surveillance,filed_route", *updated.Prediction.DegradationReason)
	require.Empty(t, sequenceInput(aman.AirportState{
		Revision:     1,
		RunwayGroups: service.initialState("EKCH", now).RunwayGroups,
		Flights:      []aman.AMANFlight{updated},
	}, service.deps.Terminal).Flights)
}

func TestInvalidGroundspeedDoesNotEnterThePredictor(t *testing.T) {
	now := time.Date(2026, time.July, 23, 12, 0, 0, 0, time.UTC)
	groundspeed, altitude, route := 0.0, 5000, "DCT MONAK"
	observation := aman.FlightObservation{
		FlightID: "stationary", VATSIMCID: "789", Callsign: "SAS789", Destination: "EKCH", FiledRoute: &route,
		ReconciledAt: now, SourceStatus: aman.DataFresh,
		Surveillance: &aman.SurveillanceFact{GroundspeedKnots: &groundspeed, AltitudeFeet: &altitude},
	}
	service, err := New(Dependencies{
		Repository: &memoryRepository{}, Materializer: unavailableNavigation{}, Geometry: unavailableGeometry{}, Wind: unavailableWind{},
		Publisher: &recordingPublisher{}, Terminal: terminal.Configuration{Airport: "EKCH", RunwayGroups: []terminal.RunwayGroup{{ID: "ARRIVAL-22"}}},
		Airports: []string{"EKCH"}, Mode: aman.ModeShadow, Now: func() time.Time { return now },
	})
	require.NoError(t, err)
	flight := newFlight(observation, now)
	flight.State = aman.StateAirborne
	flight.Prediction = &aman.Prediction{Publishable: true}

	updated, err := service.reconcileFlight(context.Background(), service.initialState("EKCH", now), flight, observation, now)

	require.NoError(t, err)
	require.False(t, updated.Prediction.Publishable)
	require.Equal(t, "invalid_essential_data:groundspeed", *updated.Prediction.DegradationReason)
}

func TestInvalidPredictionAndMissingSourceReleaseProtectedSlots(t *testing.T) {
	now := time.Date(2026, time.July, 23, 12, 0, 0, 0, time.UTC)
	order, manualOrder := 1, 1
	flight := aman.AMANFlight{
		FreezeReason: aman.FreezeManual,
		Slot:         &aman.Slot{Time: now.Add(time.Minute), RunwayGroupID: "ARRIVAL-22", Sequence: 1},
		Order:        &order, ManualOrder: &manualOrder, QueueOffers: []aman.QueueOffer{{FlightID: "flight-1"}},
		Prediction: &aman.Prediction{Publishable: true},
	}
	markPredictionNonPublishable(&flight, "missing_essential_data:surveillance")
	require.Nil(t, flight.Slot)
	require.Nil(t, flight.Order)
	require.Nil(t, flight.ManualOrder)
	require.Empty(t, flight.QueueOffers)
	require.Equal(t, aman.FreezeNone, flight.FreezeReason)

	flight.Slot = &aman.Slot{Time: now.Add(time.Minute), RunwayGroupID: "ARRIVAL-22", Sequence: 1}
	flight.FreezeReason = aman.FreezeSuperstable
	markMissing(&flight, now)
	require.Nil(t, flight.Slot)
	require.Equal(t, aman.FreezeNone, flight.FreezeReason)
}

func TestPredictionCruiseAltitudeUsesMateriallyHigherFiledLevel(t *testing.T) {
	altitude, requested := 28_800, 35_000
	observation := aman.FlightObservation{RequestedLevel: &requested, Surveillance: &aman.SurveillanceFact{AltitudeFeet: &altitude}}
	require.Equal(t, 35_000.0, predictionCruiseAltitude(observation))
	require.Equal(t, 28_800.0, predictionCruiseAltitudeForRoute(observation, true, false), "terminal traffic must not climb back to its filed cruise level")
	require.Equal(t, 28_800.0, predictionCruiseAltitudeForRoute(observation, false, true), "confirmed descent must not climb back to its filed cruise level")

	requested = 30_000
	require.Equal(t, 28_800.0, predictionCruiseAltitude(observation))
}

func TestDescentStateRequiresConsecutiveReportsAndThenLatches(t *testing.T) {
	base := time.Date(2026, time.July, 28, 9, 0, 0, 0, time.UTC)
	observation := func(at time.Time, altitude int) aman.FlightObservation {
		return aman.FlightObservation{Surveillance: &aman.SurveillanceFact{AltitudeFeet: &altitude, ObservedAt: &at}}
	}
	first := observation(base, 30_000)
	second := observation(base.Add(15*time.Second), 29_800)
	confirmed, samples := descentStateForRoute(nil, &first, second, false)
	require.False(t, confirmed)
	require.Equal(t, uint8(1), samples)

	progress := &aman.RouteProgress{DescentEvidenceSamples: samples}
	third := observation(base.Add(30*time.Second), 29_600)
	confirmed, samples = descentStateForRoute(progress, &second, third, false)
	require.True(t, confirmed)
	require.Equal(t, uint8(2), samples)

	progress.DescentConfirmed, progress.DescentEvidenceSamples = confirmed, samples
	level := observation(base.Add(45*time.Second), 29_620)
	confirmed, samples = descentStateForRoute(progress, &third, level, false)
	require.True(t, confirmed, "a level-off after TOD must not clear descent")
	require.Equal(t, uint8(2), samples)
}

func TestDescentStateRejectsStaleOrSingleNoisyAltitudeChange(t *testing.T) {
	base := time.Date(2026, time.July, 28, 9, 0, 0, 0, time.UTC)
	high, low := 30_000, 29_800
	previous := aman.FlightObservation{Surveillance: &aman.SurveillanceFact{AltitudeFeet: &high, ObservedAt: &base}}
	staleAt := base.Add(3 * time.Minute)
	current := aman.FlightObservation{Surveillance: &aman.SurveillanceFact{AltitudeFeet: &low, ObservedAt: &staleAt}}

	confirmed, samples := descentStateForRoute(&aman.RouteProgress{DescentEvidenceSamples: 1}, &previous, current, false)
	require.False(t, confirmed)
	require.Zero(t, samples)
}

func TestObservedGroundspeedIsUsedOnlyAtOrNearFiledCruiseLevel(t *testing.T) {
	altitude, requested := 35_900, 36_000
	observation := aman.FlightObservation{RequestedLevel: &requested, Surveillance: &aman.SurveillanceFact{AltitudeFeet: &altitude}}
	require.True(t, useObservedGroundspeedBeforeTOD(observation))

	altitude = 28_800
	require.False(t, useObservedGroundspeedBeforeTOD(observation))
	require.True(t, useObservedGroundspeedForRoute(observation, true), "terminal level segments should follow the observed groundspeed")
}

func TestGroundedSurveillanceKeepsPreTakeoffFlightPlanned(t *testing.T) {
	now := time.Date(2026, time.July, 23, 12, 0, 0, 0, time.UTC)
	altitude, groundspeed := 124, 0.0
	eobt, eet := now.Add(time.Hour), 90*time.Minute
	observation := aman.FlightObservation{PlannedTiming: &aman.PlannedTiming{EstimatedOffBlockTime: &eobt, EstimatedEnrouteTime: &eet}, Surveillance: &aman.SurveillanceFact{AltitudeFeet: &altitude, GroundspeedKnots: &groundspeed}}
	flight := aman.AMANFlight{State: aman.StateAirborne, FreezeReason: aman.FreezeNone, Prediction: &aman.Prediction{Publishable: true}, Slot: &aman.Slot{Time: now.Add(time.Minute)}}

	updated := applyGroundedObservation(flight, observation, now)

	require.Equal(t, aman.StatePlanned, updated.State)
	require.Nil(t, updated.Slot)
	require.NotNil(t, updated.Prediction)
	require.Equal(t, "aman-planned-eobt-exot-eet-v1", updated.Prediction.ModelVersion)
}

func TestGroundedSurveillanceLandsPostTakeoffFlight(t *testing.T) {
	now := time.Date(2026, time.July, 23, 12, 0, 0, 0, time.UTC)
	altitude, groundspeed := 26, 4.0
	takeoff := now.Add(-time.Hour)
	manualOrder := 1
	holding, routeKey, feeder := "HOLD", "route", "MONAK"
	observation := aman.FlightObservation{TakeoffDetected: &takeoff, Surveillance: &aman.SurveillanceFact{AltitudeFeet: &altitude, GroundspeedKnots: &groundspeed}}
	flight := aman.AMANFlight{
		State: aman.StateAirborne, FreezeReason: aman.FreezeManual, SelectedHolding: &holding, ActiveRouteKey: &routeKey, SelectedFeeder: &feeder,
		Prediction: &aman.Prediction{Publishable: true}, Slot: &aman.Slot{Time: now.Add(time.Minute)}, ManualOrder: &manualOrder,
	}

	updated := applyGroundedObservation(flight, observation, now)

	require.Equal(t, aman.StateLanded, updated.State)
	require.Equal(t, aman.LifecycleReasonLandingConfirmed, updated.Lifecycle.Reason)
	require.False(t, updated.Prediction.Publishable)
	require.Equal(t, "landed", *updated.Prediction.DegradationReason)
	require.Nil(t, updated.Slot)
	require.Nil(t, updated.SelectedHolding)
	require.Equal(t, aman.FreezeNone, updated.FreezeReason)
}

func TestRepairSuperstableFreezeCapturesOrReleasesSlot(t *testing.T) {
	now := time.Date(2026, time.July, 23, 12, 0, 0, 0, time.UTC)
	group := aman.RunwayGroupID("ARRIVAL-22")
	slotted := aman.AMANFlight{State: aman.StateStable, FreezeReason: aman.FreezeSuperstable, Slot: &aman.Slot{Time: now.Add(10 * time.Minute), RunwayGroupID: group, Sequence: 2}}

	repairSuperstableFreeze(&slotted)

	require.NotNil(t, slotted.FrozenSlot)
	require.Equal(t, *slotted.Slot, *slotted.FrozenSlot)

	unslotted := aman.AMANFlight{State: aman.StateLanded, FreezeReason: aman.FreezeSuperstable, FrozenAt: &now, FrozenOperationalTETA: &now}
	repairSuperstableFreeze(&unslotted)

	require.Equal(t, aman.FreezeNone, unslotted.FreezeReason)
	require.Nil(t, unslotted.FrozenAt)
	require.Nil(t, unslotted.FrozenOperationalTETA)
}

func TestFutureRateChangePreservesCurrentAndPendingSchedule(t *testing.T) {
	now := time.Date(2026, time.July, 23, 12, 0, 0, 0, time.UTC)
	service, err := New(Dependencies{
		Repository: &memoryRepository{}, Materializer: unavailableNavigation{}, Geometry: unavailableGeometry{}, Wind: unavailableWind{},
		Publisher: &recordingPublisher{}, Terminal: terminal.Configuration{Airport: "EKCH", ConfigVersion: "test", RunwayGroups: []terminal.RunwayGroup{{ID: "ARRIVAL-22"}}},
		Airports: []string{"EKCH"}, Mode: aman.ModeShadow, Now: func() time.Time { return now },
	})
	require.NoError(t, err)
	state := service.initialState("EKCH", now)
	state.Revision = 7
	future := now.Add(15 * time.Minute)
	mutation, err := service.SetRate(aman.CommandContext{ReceivedAt: now}, aman.SetRateCommand{
		Metadata:      aman.CommandMetadata{CommandID: "future-rate", ExpectedRevision: 7},
		RunwayGroupID: "ARRIVAL-22", ArrivalsPerHour: 30, EffectiveAt: future,
	})
	require.NoError(t, err)
	change, err := mutation(state)
	require.NoError(t, err)
	group := change.State.RunwayGroups[0]
	require.Equal(t, uint32(40), group.ActiveRatePerHour)
	require.Len(t, group.RateSchedule, 2)
	require.Equal(t, []sequence.RatePoint{
		{EffectiveAt: now, ArrivalsPerHour: 40},
		{EffectiveAt: future, ArrivalsPerHour: 30},
	}, sequenceInput(change.State, service.deps.Terminal).Policies[0].Rates)

	updateActiveRates(change.State.RunwayGroups, future)
	require.Equal(t, uint32(30), change.State.RunwayGroups[0].ActiveRatePerHour)
	require.Equal(t, future, *change.State.RunwayGroups[0].RateEffectiveAt)
}

func TestRateSelectionMovesOnlyReorderableFlightsAtEffectiveTime(t *testing.T) {
	now := time.Date(2026, time.July, 23, 12, 0, 0, 0, time.UTC)
	service, err := New(Dependencies{
		Repository: &memoryRepository{}, Materializer: unavailableNavigation{}, Geometry: unavailableGeometry{}, Wind: unavailableWind{},
		Publisher: &recordingPublisher{}, Terminal: terminal.Configuration{Airport: "EKCH", ConfigVersion: "test", RunwayGroups: []terminal.RunwayGroup{{ID: "ARRIVAL-04"}, {ID: "ARRIVAL-22"}}},
		Airports: []string{"EKCH"}, Mode: aman.ModeShadow, Now: func() time.Time { return now },
	})
	require.NoError(t, err)
	state := service.initialState("EKCH", now)
	state.Revision = 7
	wake, feeder := "M", "MONAK"
	unstable := operationalFlight("UNSTABLE", "ARRIVAL-04", feeder, wake, now.Add(20*time.Minute))
	stable := operationalFlight("STABLE", "ARRIVAL-04", feeder, wake, now.Add(23*time.Minute))
	stable.State = aman.StateStable
	state.Flights = []aman.AMANFlight{unstable, stable}
	future := now.Add(15 * time.Minute)

	mutation, err := service.SetRate(aman.CommandContext{ReceivedAt: now}, aman.SetRateCommand{
		Metadata:      aman.CommandMetadata{CommandID: "select-22", ExpectedRevision: 7},
		RunwayGroupID: "ARRIVAL-22", ArrivalsPerHour: 30, EffectiveAt: future,
	})
	require.NoError(t, err)
	change, err := mutation(state)
	require.NoError(t, err)
	require.True(t, change.State.RunwayGroups[0].Selected)
	require.False(t, change.State.RunwayGroups[1].Selected)
	require.Equal(t, aman.RunwayGroupID("ARRIVAL-04"), *change.State.Flights[0].SelectedRunwayGroup)

	selected, changed := updateSelectedRunwayGroup(change.State.RunwayGroups, future)
	require.True(t, changed)
	require.Equal(t, aman.RunwayGroupID("ARRIVAL-22"), selected)
	reassignFlightsToGroup(&change.State, selected)
	require.Equal(t, aman.RunwayGroupID("ARRIVAL-22"), *change.State.Flights[0].SelectedRunwayGroup)
	require.Equal(t, aman.RunwayGroupID("ARRIVAL-04"), *change.State.Flights[1].SelectedRunwayGroup)
}

func TestRunwayConfigurationUpgradeReleasesObsoleteGroupState(t *testing.T) {
	now := time.Date(2026, time.July, 23, 12, 0, 0, 0, time.UTC)
	oldGroups := []aman.RunwayGroupPolicy{{ID: "ARRIVAL-22"}}
	selected := aman.RunwayGroupID("ARRIVAL-22")
	flight := aman.AMANFlight{
		State: aman.StateStable, SelectedRunwayGroup: &selected, FreezeReason: aman.FreezeSuperstable,
		Slot: &aman.Slot{Time: now.Add(time.Minute), RunwayGroupID: selected, Sequence: 1},
	}
	state := aman.AirportState{RunwayGroups: oldGroups, Flights: []aman.AMANFlight{flight}}
	configured := []terminal.RunwayGroup{{ID: "ARRIVAL-22L"}, {ID: "ARRIVAL-22R"}}

	require.False(t, runwayGroupsMatchTerminal(state.RunwayGroups, configured))
	state.RunwayGroups = []aman.RunwayGroupPolicy{{ID: "ARRIVAL-22L"}, {ID: "ARRIVAL-22R"}}
	resetFlightsForRunwayConfiguration(&state)

	require.Nil(t, state.Flights[0].SelectedRunwayGroup)
	require.Nil(t, state.Flights[0].Slot)
	require.Equal(t, aman.FreezeNone, state.Flights[0].FreezeReason)
}

func TestSessionRunwaySelectsItsExactAMANGroup(t *testing.T) {
	now := time.Date(2026, time.July, 23, 12, 0, 0, 0, time.UTC)
	service, err := New(Dependencies{
		Repository: &memoryRepository{}, Materializer: unavailableNavigation{}, Geometry: unavailableGeometry{}, Wind: unavailableWind{}, Publisher: &recordingPublisher{},
		Runways: staticArrivalRunway{runway: "22L"},
		Terminal: terminal.Configuration{Airport: "EKCH", ConfigVersion: "test", RunwayGroups: []terminal.RunwayGroup{
			{ID: "ARRIVAL-04L", Aliases: []aman.RunwayGroupID{"04L"}},
			{ID: "ARRIVAL-22L", Aliases: []aman.RunwayGroupID{"22L"}},
		}},
		Airports: []string{"EKCH"}, Mode: aman.ModeShadow, Now: func() time.Time { return now },
	})
	require.NoError(t, err)
	state := service.initialState("EKCH", now)
	unstable := operationalFlight("UNSTABLE", "ARRIVAL-04L", "MONAK", "M", now.Add(20*time.Minute))
	stable := protectedOperationalFlight("STABLE", "ARRIVAL-04L", "MONAK", "M", now.Add(23*time.Minute), 1, aman.FreezeManual)
	state.Flights = []aman.AMANFlight{unstable, stable}

	selected, changed, err := service.selectSessionRunwayGroup(context.Background(), "EKCH", state.RunwayGroups)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, aman.RunwayGroupID("ARRIVAL-22L"), selected)
	reassignFlightsToGroup(&state, selected)
	require.Equal(t, aman.RunwayGroupID("ARRIVAL-22L"), *state.Flights[0].SelectedRunwayGroup)
	require.Equal(t, aman.RunwayGroupID("ARRIVAL-04L"), *state.Flights[1].SelectedRunwayGroup, "protected flights retain their committed runway")
}

func TestFutureRateCommandsPreserveRunwaySelectionHistory(t *testing.T) {
	now := time.Date(2026, time.July, 23, 12, 0, 0, 0, time.UTC)
	service, err := New(Dependencies{
		Repository: &memoryRepository{}, Materializer: unavailableNavigation{}, Geometry: unavailableGeometry{}, Wind: unavailableWind{},
		Publisher: &recordingPublisher{}, Terminal: terminal.Configuration{Airport: "EKCH", ConfigVersion: "test", RunwayGroups: []terminal.RunwayGroup{{ID: "ARRIVAL-04"}, {ID: "ARRIVAL-22"}}},
		Airports: []string{"EKCH"}, Mode: aman.ModeShadow, Now: func() time.Time { return now },
	})
	require.NoError(t, err)
	state := service.initialState("EKCH", now)
	state.Revision = 7

	applyRate := func(group aman.RunwayGroupID, effectiveAt time.Time) {
		mutation, mutationErr := service.SetRate(aman.CommandContext{ReceivedAt: now}, aman.SetRateCommand{
			Metadata: aman.CommandMetadata{
				CommandID:        "select-" + string(group) + "-" + effectiveAt.Format("1504"),
				ExpectedRevision: state.Revision,
			},
			RunwayGroupID: group, ArrivalsPerHour: 20, EffectiveAt: effectiveAt,
		})
		require.NoError(t, mutationErr)
		change, changeErr := mutation(state)
		require.NoError(t, changeErr)
		state = change.State
		state.Revision++
	}

	first, second, third := now.Add(10*time.Minute), now.Add(20*time.Minute), now.Add(30*time.Minute)
	applyRate("ARRIVAL-22", first)
	applyRate("ARRIVAL-04", second)
	applyRate("ARRIVAL-22", third)

	selected, changed := updateSelectedRunwayGroup(state.RunwayGroups, first)
	require.True(t, changed)
	require.Equal(t, aman.RunwayGroupID("ARRIVAL-22"), selected)
	selected, changed = updateSelectedRunwayGroup(state.RunwayGroups, second)
	require.True(t, changed)
	require.Equal(t, aman.RunwayGroupID("ARRIVAL-04"), selected)
	selected, changed = updateSelectedRunwayGroup(state.RunwayGroups, third)
	require.True(t, changed)
	require.Equal(t, aman.RunwayGroupID("ARRIVAL-22"), selected)
}

func TestGoAroundUpdatesOperationalTETABeforeCascading(t *testing.T) {
	now := time.Date(2026, time.July, 23, 12, 0, 0, 0, time.UTC)
	service, err := New(Dependencies{
		Repository: &memoryRepository{}, Materializer: unavailableNavigation{}, Geometry: unavailableGeometry{}, Wind: unavailableWind{},
		Publisher: &recordingPublisher{}, Terminal: terminal.Configuration{Airport: "EKCH", ConfigVersion: "test", RunwayGroups: []terminal.RunwayGroup{{ID: "ARRIVAL-22"}}},
		Airports: []string{"EKCH"}, Mode: aman.ModeShadow, Now: func() time.Time { return now },
	})
	require.NoError(t, err)
	state := service.initialState("EKCH", now)
	state.Revision = 4
	flight := operationalFlight("GO-AROUND", "ARRIVAL-22", "MONAK", "M", now.Add(3*time.Minute))
	flight.State = aman.StateStable
	flight.Slot = &aman.Slot{
		Time: now.Add(3 * time.Minute), RunwayGroupID: "ARRIVAL-22",
		Sequence: 1, Revision: state.Revision, Reason: string(sequence.ReasonRateWTC),
	}
	state.Flights = []aman.AMANFlight{flight}

	command := aman.ReportGoAroundCommand{
		Metadata:   aman.CommandMetadata{CommandID: "go-around", ExpectedRevision: state.Revision},
		FlightID:   flight.ID,
		DetectedAt: now,
	}
	mutation, err := service.ReportGoAround(aman.CommandContext{ReceivedAt: now}, command)
	require.NoError(t, err)
	change, err := mutation(state)
	require.NoError(t, err)
	updated := change.State.Flights[0]
	require.Equal(t, now.Add(DefaultGoAroundDelay), updated.Prediction.OperationalTETA)
	require.Equal(t, aman.OperationalReasonGoAround, updated.Prediction.OperationalReason)
	require.True(t, updated.Prediction.Publishable)
	require.NotNil(t, updated.Slot)
	require.False(t, updated.Slot.Time.Before(now.Add(DefaultGoAroundDelay)))
	require.NotNil(t, change.QueueOffers)
}

func TestRateChangeRejectsProtectedSameSTARConflict(t *testing.T) {
	now := time.Date(2026, time.July, 23, 12, 0, 0, 0, time.UTC)
	service, err := New(Dependencies{
		Repository: &memoryRepository{}, Materializer: unavailableNavigation{}, Geometry: unavailableGeometry{}, Wind: unavailableWind{},
		Publisher: &recordingPublisher{}, Terminal: terminal.Configuration{Airport: "EKCH", ConfigVersion: "test", RunwayGroups: []terminal.RunwayGroup{{
			ID: "ARRIVAL-22", SameSTARSpacing: &terminal.SameSTARSpacing{Enabled: true, ActivationRatePerHour: 20, MinimumEmptySlots: 1},
		}}},
		Airports: []string{"EKCH"}, Mode: aman.ModeShadow, Now: func() time.Time { return now },
	})
	require.NoError(t, err)
	state := service.initialState("EKCH", now)
	state.Revision = 4
	state.RunwayGroups[0].ActiveRatePerHour = 19
	state.RunwayGroups[0].RateSchedule = []aman.RunwayGroupRatePoint{{EffectiveAt: now.Add(-time.Hour), ArrivalsPerHour: 19}}
	wake, feeder := "M", "MONAK"
	lead := protectedOperationalFlight("LEAD", "ARRIVAL-22", feeder, wake, now, 1, aman.FreezeManual)
	trail := protectedOperationalFlight("TRAIL", "ARRIVAL-22", feeder, wake, now.Add(3*time.Minute), 2, aman.FreezeSuperstable)
	state.Flights = []aman.AMANFlight{lead, trail}

	mutation, err := service.SetRate(aman.CommandContext{ReceivedAt: now}, aman.SetRateCommand{
		Metadata:      aman.CommandMetadata{CommandID: "activate-spacing", ExpectedRevision: 4},
		RunwayGroupID: "ARRIVAL-22", ArrivalsPerHour: 20, EffectiveAt: now,
	})
	require.NoError(t, err)
	_, err = mutation(state)
	var domain *aman.DomainError
	require.ErrorAs(t, err, &domain)
	require.Equal(t, aman.ErrorInvalidTransition, domain.Class)
	require.ErrorContains(t, err, string(sequence.WarningProtectedSameSTAR))
	require.Equal(t, uint32(19), state.RunwayGroups[0].ActiveRatePerHour)
}

func TestActiveRouteReuseIncludesNavigationDatasetVersion(t *testing.T) {
	now := time.Date(2026, time.July, 23, 12, 0, 0, 0, time.UTC)
	version := navdata.DatasetVersion{
		Cycle: "2607", SourceRevision: "revision-a",
		EffectiveFrom: now.Add(-24 * time.Hour), EffectiveUntil: now.Add(24 * time.Hour),
	}
	key, group := "route-key", aman.RunwayGroupID("ARRIVAL-22")
	datasetID := navigationDatasetID(version)
	flight := aman.AMANFlight{
		ActiveRouteKey: &key, ActiveRouteDatasetID: &datasetID,
		RouteProgress: &aman.RouteProgress{FlightPlanRevision: 7, RunwayGroupID: group},
	}
	require.True(t, canReuseActiveRoute(flight, 7, group, datasetID))
	version.SourceRevision = "revision-b"
	require.False(t, canReuseActiveRoute(flight, 7, group, navigationDatasetID(version)))
}

func TestTMAFreezeKeepsRouteProjectionRevision(t *testing.T) {
	progress := &aman.RouteProgress{FlightPlanRevision: 2}
	flight := aman.AMANFlight{FreezeReason: aman.FreezeTMA, RouteProgress: progress}

	require.Equal(t, uint64(2), routeProjectionRevision(flight, 3), "an administrative FPL revision must not reset terminal progress")

	flight.FreezeReason = aman.FreezeNone
	require.Equal(t, uint64(3), routeProjectionRevision(flight, 3), "outside TMA the new route revision remains authoritative")
}

func TestSyntheticDestinationClosingLegRequiresRouteRematerialization(t *testing.T) {
	from, destination := navdata.FixID("TUDLO"), navdata.FixID("EKCH")
	route := navdata.RouteGeometry{Legs: []navdata.ProcedureLeg{{ID: "ROUTE-0012", PathTerminator: navdata.PathTF, FromFix: &from, ToFix: &destination}}}

	require.True(t, hasSyntheticDestinationClosingLeg(route, "EKCH"))
	require.False(t, hasSyntheticDestinationClosingLeg(route, "EGLL"))
}

func TestHoldingETAUsesPerLegDurations(t *testing.T) {
	now := time.Date(2026, time.July, 23, 12, 0, 0, 0, time.UTC)
	legs := []trajectory.RemainingLeg{
		{To: "HOLD", DistanceNM: 90},
		{To: "RUNWAY", DistanceNM: 10},
	}
	got := holdingETA(now, []time.Duration{20 * time.Minute, 2 * time.Minute}, legs, "HOLD")
	require.NotNil(t, got)
	require.Equal(t, now.Add(20*time.Minute), *got)
	require.Nil(t, holdingETA(now, []time.Duration{20 * time.Minute}, legs, "HOLD"))
}

func TestHoldingPlanKeepsSlotFixedAndRecalculatesDelayFromLatestTrajectory(t *testing.T) {
	now := time.Date(2026, time.July, 24, 12, 0, 0, 0, time.UTC)
	holdingEntry := now.Add(8 * time.Minute)
	slot := &aman.Slot{Time: now.Add(32 * time.Minute)}
	prediction := aman.Prediction{Publishable: true, RawTETA: now.Add(20 * time.Minute), HoldingFixETA: &holdingEntry}

	first := holdingPlan(prediction, slot)
	require.NotNil(t, first)
	require.Equal(t, now.Add(20*time.Minute), first.ApproachReleaseTime)
	require.Equal(t, 12*time.Minute, first.ExpectedHoldingDuration)
	require.Equal(t, 12*time.Minute, first.PostHoldingTransit)

	// A later physical ETA, such as one recalculated from a lower observed
	// altitude, reduces the hold but leaves the controller's slot untouched.
	laterEntry := now.Add(10 * time.Minute)
	prediction.RawTETA, prediction.HoldingFixETA = now.Add(24*time.Minute), &laterEntry
	second := holdingPlan(prediction, slot)
	require.NotNil(t, second)
	require.Equal(t, now.Add(18*time.Minute), second.ApproachReleaseTime)
	require.Equal(t, 8*time.Minute, second.ExpectedHoldingDuration)
	require.Equal(t, now.Add(32*time.Minute), slot.Time)

	prediction.RawTETA = now.Add(34 * time.Minute)
	require.Nil(t, holdingPlan(prediction, slot), "an infeasible fixed slot must not invent a hold plan")
}

func TestOffRouteFallbackReasonLowersPredictionConfidenceWithoutHidingWaypoint(t *testing.T) {
	reason := offRouteFallbackReason([]string{"UNRESOLVED_LEG:X", "OFF_ROUTE", "OFF_ROUTE_NEXT_WAYPOINT:TESPI"})
	require.Equal(t, "off_route_next_waypoint:tespi", reason)
	require.Empty(t, offRouteFallbackReason([]string{"OFF_ROUTE"}))
}

func TestNavigationHealthFailsClosedWhenNoActiveCacheSnapshotExists(t *testing.T) {
	now := time.Date(2026, time.July, 23, 12, 0, 0, 0, time.UTC)
	service, err := New(Dependencies{
		Repository: &memoryRepository{}, Materializer: unavailableNavigation{}, Geometry: unavailableGeometry{}, Wind: unavailableWind{},
		Publisher: &recordingPublisher{}, Terminal: terminal.Configuration{Airport: "EKCH", ConfigVersion: "test", RunwayGroups: []terminal.RunwayGroup{{ID: "ARRIVAL-22"}}},
		Airports: []string{"EKCH"}, Mode: aman.ModeAuthoritative, Now: func() time.Time { return now },
	})
	require.NoError(t, err)
	require.False(t, service.TechnicalHealth(context.Background()).AuthorityAllowed)
	require.NoError(t, service.ObserveSourceHealth(context.Background(), aman.DataFresh, now))
	service.observeNavigationCache(context.Background(), "EKCH")
	health := service.TechnicalHealth(context.Background())
	require.Equal(t, aman.HealthReady, health.VATSIM.Status)
	require.Equal(t, aman.HealthUnavailable, health.Navigation.Status)
	require.False(t, health.AuthorityAllowed)
}

func TestReconcileAllObservesAirportWeatherWithoutFlights(t *testing.T) {
	now := time.Date(2026, time.July, 23, 12, 0, 0, 0, time.UTC)
	wind := &observedWind{now: now}
	service, err := New(Dependencies{
		Repository: &memoryRepository{}, Materializer: readyNavigation{}, Geometry: unavailableGeometry{}, Wind: wind,
		Publisher: &recordingPublisher{}, Terminal: terminal.Configuration{Airport: "EKCH", ConfigVersion: "test", RunwayGroups: []terminal.RunwayGroup{{
			ID: "ARRIVAL-22", FinalApproaches: []terminal.FinalApproachDefinition{{Runway: "22L", Threshold: terminal.ThresholdDefinition{Position: terminal.CoordinateDefinition{LatitudeDeg: 55.6254, LongitudeDeg: 12.6676}}}},
		}}}, Airports: []string{"EKCH"}, Mode: aman.ModeShadow, Now: func() time.Time { return now },
	})
	require.NoError(t, err)

	service.reconcileAll(context.Background())

	require.Len(t, wind.requests, 1)
	require.Equal(t, 55.6254, wind.requests[0].Samples[0].Position.LatitudeDegrees)
	require.Equal(t, 12.6676, wind.requests[0].Samples[0].Position.LongitudeDegrees)
	require.Equal(t, aman.HealthReady, service.TechnicalHealth(context.Background()).Weather.Status)
}

func TestWeatherRefreshIsRateLimitedAfterSuccessAndFailure(t *testing.T) {
	now := time.Date(2026, time.July, 23, 12, 0, 0, 0, time.UTC)
	wind := &countingUnavailableWind{}
	service, err := New(Dependencies{
		Repository: &memoryRepository{}, Materializer: readyNavigation{}, Geometry: unavailableGeometry{}, Wind: wind,
		Publisher: &recordingPublisher{}, Terminal: terminal.Configuration{Airport: "EKCH", ConfigVersion: "test", RunwayGroups: []terminal.RunwayGroup{{
			ID: "ARRIVAL-22", FinalApproaches: []terminal.FinalApproachDefinition{{Runway: "22L", Threshold: terminal.ThresholdDefinition{Position: terminal.CoordinateDefinition{LatitudeDeg: 55.6254, LongitudeDeg: 12.6676}}}},
		}}}, Airports: []string{"EKCH"}, Mode: aman.ModeShadow, Now: func() time.Time { return now },
	})
	require.NoError(t, err)

	service.reconcileAll(context.Background())
	now = now.Add(weatherRefreshEvery - time.Second)
	service.reconcileAll(context.Background())
	now = now.Add(time.Second)
	service.reconcileAll(context.Background())

	require.Equal(t, 2, wind.requests)
	require.Equal(t, aman.HealthUnavailable, service.TechnicalHealth(context.Background()).Weather.Status)
}

func TestServiceCommitsInitialEmptyAirportState(t *testing.T) {
	now := time.Date(2026, time.July, 23, 12, 0, 0, 0, time.UTC)
	repository := &memoryRepository{}
	publisher := &recordingPublisher{}
	service, err := New(Dependencies{
		Repository: repository, Materializer: unavailableNavigation{}, Geometry: unavailableGeometry{}, Wind: unavailableWind{},
		Publisher: publisher, Terminal: terminal.Configuration{Airport: "EKCH", ConfigVersion: "test", RunwayGroups: []terminal.RunwayGroup{{ID: "ARRIVAL-22"}}},
		Airports: []string{"EKCH"}, Mode: aman.ModeShadow, Now: func() time.Time { return now },
	})
	require.NoError(t, err)
	require.NoError(t, service.reconcileAirport(context.Background(), "EKCH"))
	require.True(t, repository.has)
	require.Equal(t, aman.SequenceRevision(1), repository.state.Revision)
	require.Empty(t, repository.state.Flights)
	require.Len(t, publisher.states, 1)
}

func TestObservedAtNormalizesSourcePrecisionToWholeSeconds(t *testing.T) {
	value := time.Date(2026, time.July, 23, 12, 0, 1, 987654321, time.UTC)
	actual := observedAt(&aman.SurveillanceFact{ObservedAt: &value}, value)
	require.Equal(t, time.Date(2026, time.July, 23, 12, 0, 1, 0, time.UTC), actual)
}

func operationalFlight(id string, group aman.RunwayGroupID, feeder, wake string, teta time.Time) aman.AMANFlight {
	observation := aman.FlightObservation{WakeCategory: &wake}
	return aman.AMANFlight{
		ID: aman.FlightID(id), State: aman.StateUnstable, SelectedRunwayGroup: &group, SelectedFeeder: &feeder,
		LatestObservation: &observation, FreezeReason: aman.FreezeNone,
		Prediction: &aman.Prediction{OperationalTETA: teta, Publishable: true},
	}
}

func protectedOperationalFlight(id string, group aman.RunwayGroupID, feeder, wake string, slotAt time.Time, sequenceNumber int, reason aman.FreezeReason) aman.AMANFlight {
	flight := operationalFlight(id, group, feeder, wake, slotAt)
	flight.State = aman.StateStable
	flight.FreezeReason = reason
	flight.FrozenAt = &slotAt
	flight.FrozenOperationalTETA = &slotAt
	flight.FrozenSlot = &aman.Slot{Time: slotAt, RunwayGroupID: group, Sequence: sequenceNumber, Revision: 4, Reason: "protected"}
	return flight
}

type memoryRepository struct {
	state aman.AirportState
	has   bool
}

type staticArrivalRunway struct {
	runway string
	err    error
}

func (s staticArrivalRunway) ActiveArrivalRunway(context.Context, string) (string, error) {
	return s.runway, s.err
}

func (r *memoryRepository) LoadAirportState(context.Context, string) (aman.AirportState, error) {
	if !r.has {
		return aman.AirportState{}, &aman.DomainError{Class: aman.ErrorNotFound, Message: "missing"}
	}
	return r.state, nil
}
func (r *memoryRepository) Commit(_ context.Context, commit aman.StateCommit) (aman.CommitResult, error) {
	if err := commit.Validate(); err != nil {
		return aman.CommitResult{}, err
	}
	r.state, r.has = commit.State, true
	return aman.CommitResult{State: commit.State}, nil
}

type recordingPublisher struct{ states []aman.AirportState }

func (p *recordingPublisher) PublishAMANState(_ context.Context, state aman.AirportState) error {
	p.states = append(p.states, state)
	return nil
}

type unavailableNavigation struct{}

func (unavailableNavigation) MaterializeRoute(context.Context, navdata.RouteQuery, string) (navdata.RouteKey, error) {
	return "", errors.New("offline")
}

type readyNavigation struct{}

func (readyNavigation) MaterializeRoute(context.Context, navdata.RouteQuery, string) (navdata.RouteKey, error) {
	return "", errors.New("not implemented")
}

type unavailableGeometry struct{}

func (unavailableGeometry) ActiveVersion(context.Context, navdata.AirportID) (navdata.DatasetVersion, error) {
	return navdata.DatasetVersion{}, errors.New("offline")
}
func (unavailableGeometry) Route(context.Context, navdata.RouteKey) (navdata.RouteGeometry, error) {
	return navdata.RouteGeometry{}, errors.New("offline")
}
func (unavailableGeometry) TerminalPath(context.Context, navdata.AirportID, navdata.FeederID, aman.RunwayGroupID) (navdata.TerminalPath, error) {
	return navdata.TerminalPath{}, errors.New("offline")
}
func (unavailableGeometry) ActiveGeometrySnapshot(context.Context, navdata.AirportID) (navdata.ActiveGeometrySnapshot, error) {
	return navdata.ActiveGeometrySnapshot{}, errors.New("offline")
}

type unavailableWind struct{}

func (unavailableWind) WindProfile(context.Context, predictor.WindProfileRequest) (predictor.WindProfile, error) {
	return predictor.WindProfile{}, errors.New("offline")
}

type countingUnavailableWind struct{ requests int }

func (w *countingUnavailableWind) WindProfile(context.Context, predictor.WindProfileRequest) (predictor.WindProfile, error) {
	w.requests++
	return predictor.WindProfile{}, errors.New("offline")
}

type observedWind struct {
	now      time.Time
	requests []predictor.WindProfileRequest
}

func (w *observedWind) WindProfile(_ context.Context, request predictor.WindProfileRequest) (predictor.WindProfile, error) {
	w.requests = append(w.requests, request)
	return predictor.WindProfile{
		SourceID: "test-weather", SourceRevision: "test", ObservedAt: w.now, ExpiresAt: w.now.Add(time.Hour),
		Samples: []predictor.WindSample{{
			Position: request.Samples[0].Position, At: request.Samples[0].At,
			Levels: []predictor.WindLevel{{AltitudeFeet: 10000}},
		}},
	}, nil
}
