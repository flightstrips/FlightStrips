package operational

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/lifecycle"
	"FlightStrips/internal/aman/navdata"
	"FlightStrips/internal/aman/prediction"
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
	explicitFamily := "TUDLO"
	for index := range state.Flights {
		state.Flights[index].SelectedSTARFamily = &explicitFamily
	}
	config := terminal.Configuration{
		RunwayGroups: []terminal.RunwayGroup{{ID: group}},
		STARFamilyPolicies: []terminal.STARFamilyPolicy{
			{STARFamily: "TUDLO", SameSTARSpacing: terminal.SameSTARSpacing{Enabled: true, ActivationRatePerHour: 20, MinimumEmptySlots: 1}, HoldingSequencePolicy: navdata.HoldingSequenceLowestAltitudeFirst},
			{STARFamily: "MONAK", SameSTARSpacing: terminal.SameSTARSpacing{ActivationRatePerHour: 18, MinimumEmptySlots: 2}},
		},
	}
	input := sequenceInput(state, config)
	require.Len(t, input.Policies, 1)
	require.Equal(t, sequence.SameSTARSpacing{Enabled: true, ActivationRatePerHour: 20, MinimumEmptySlots: 1}, input.Policies[0].SameSTARSpacing)
	require.Equal(t, []sequence.STARFamilyPolicy{
		{STARFamily: "MONAK", SameSTARSpacing: sequence.SameSTARSpacing{ActivationRatePerHour: 18, MinimumEmptySlots: 2}, HoldingSequencePolicy: navdata.HoldingSequenceDisabled},
		{STARFamily: "TUDLO", SameSTARSpacing: sequence.SameSTARSpacing{Enabled: true, ActivationRatePerHour: 20, MinimumEmptySlots: 1}, HoldingSequencePolicy: navdata.HoldingSequenceLowestAltitudeFirst},
	}, input.STARFamilyPolicies)
	require.Equal(t, explicitFamily, input.Flights[0].STARFamily)
	require.Equal(t, explicitFamily, input.Flights[0].SelectedSTARFamily)

	result, err := sequence.Generate(input)
	require.NoError(t, err)
	require.Len(t, result.Entries, 2)
	require.Equal(t, 6*time.Minute, result.Entries[1].Time.Sub(result.Entries[0].Time))
}

func TestSequenceInputDoesNotInferHoldingPolicyFamilyFromLegacyIdentity(t *testing.T) {
	start := time.Date(2026, time.September, 11, 12, 0, 0, 0, time.UTC)
	effective := start
	group := aman.RunwayGroupID("ARRIVAL-22")
	flight := operationalFlight("LEGACY", group, "TESPI", "M", start)
	flight.SelectedSTARFamily = nil
	state := aman.AirportState{
		RunwayGroups: []aman.RunwayGroupPolicy{{ID: group, ActiveRatePerHour: 20, RateEffectiveAt: &effective}},
		Flights:      []aman.AMANFlight{flight},
	}
	config := terminal.Configuration{
		RunwayGroups: []terminal.RunwayGroup{{ID: group}},
		STARFamilyPolicies: []terminal.STARFamilyPolicy{{
			STARFamily: "TESPI", HoldingSequencePolicy: navdata.HoldingSequenceLowestAltitudeFirst,
		}},
	}

	input := sequenceInput(state, config)
	require.Len(t, input.Flights, 1)
	require.Equal(t, "TESPI", input.Flights[0].STARFamily, "legacy identity remains available to same-STAR spacing")
	require.Empty(t, input.Flights[0].SelectedSTARFamily, "holding policy requires explicit family identity")
}

func TestResequencePersistsAndResolvesProtectedSameSTARWarnings(t *testing.T) {
	start := time.Date(2026, time.July, 23, 12, 0, 0, 0, time.UTC)
	group := aman.RunwayGroupID("ARRIVAL-22")
	spacing := &aman.SameSTARSpacingPolicy{Enabled: true, ActivationRatePerHour: 20, MinimumEmptySlots: 1}
	effective := start
	service := &Service{deps: Dependencies{Terminal: terminal.Configuration{RunwayGroups: []terminal.RunwayGroup{{ID: group}}}}}
	state := aman.AirportState{RunwayGroups: []aman.RunwayGroupPolicy{{ID: group, ActiveRatePerHour: 20, RateEffectiveAt: &effective, SameSTARSpacing: spacing}}}
	state.Flights = []aman.AMANFlight{
		protectedOperationalFlight("LEAD", group, "MONAK", "M", start, 1, aman.FreezeManual),
		protectedOperationalFlight("TRAIL", group, "MONAK", "M", start.Add(3*time.Minute), 2, aman.FreezeSuperstable),
	}
	want := []aman.RunwayGroupSequenceWarning{{
		Code: string(sequence.WarningProtectedSameSTAR), FlightID: "TRAIL", RelatedFlightID: "LEAD", STARFamily: "MONAK",
	}}

	service.resequence(&state, start)
	require.Equal(t, want, state.RunwayGroups[0].SequenceWarnings)
	require.Equal(t, start, state.Flights[0].FrozenSlot.Time)
	require.Equal(t, start.Add(3*time.Minute), state.Flights[1].FrozenSlot.Time)
	service.resequence(&state, start.Add(time.Minute))
	require.Equal(t, want, state.RunwayGroups[0].SequenceWarnings, "reconciliation replaces rather than duplicates warnings")

	state.RunwayGroups[0].ActiveRatePerHour = 19
	service.resequence(&state, start.Add(2*time.Minute))
	require.Empty(t, state.RunwayGroups[0].SequenceWarnings, "falling below the activation rate resolves the warning")
	state.RunwayGroups[0].ActiveRatePerHour = 20
	state.Flights[1].SelectedFeeder = stringPointer("TUDLO")
	service.resequence(&state, start.Add(3*time.Minute))
	require.Empty(t, state.RunwayGroups[0].SequenceWarnings, "different STAR families do not conflict")
	state.Flights[1].SelectedFeeder = stringPointer("MONAK")
	state.Flights[1].FrozenSlot.Time = start.Add(6 * time.Minute)
	service.resequence(&state, start.Add(4*time.Minute))
	require.Empty(t, state.RunwayGroups[0].SequenceWarnings, "exact spacing resolves the warning")
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

func TestReconciliationPopulatesResolvedTerminalIdentities(t *testing.T) {
	now := time.Date(2026, time.September, 11, 12, 0, 0, 0, time.UTC)
	group := aman.RunwayGroupID("ARRIVAL-22L")
	version := navdata.DatasetVersion{Cycle: "2609", SourceRevision: "test", EffectiveFrom: now.Add(-24 * time.Hour), EffectiveUntil: now.Add(24 * time.Hour)}
	origin, star, feederFix := navdata.FixID("ORIGIN"), navdata.FixID("TESPI"), navdata.FixID("TNO")
	path := navdata.TerminalPath{
		Version: version, Airport: "EKCH", Feeder: "TESPI", STARFamily: "TESPI", FeederFix: feederFix, RunwayGroup: group,
		Legs: []navdata.ProcedureLeg{{ID: "TERMINAL", PathTerminator: navdata.PathTF, FromFix: &star, ToFix: &feederFix}},
	}
	service := Service{deps: Dependencies{
		Materializer: fixedNavigation{key: "route"},
		Geometry: terminalIdentityGeometry{
			version: version, path: path,
			route: navdata.RouteGeometry{Version: version, Digest: "route-digest", Coverage: navdata.CoverageComplete, Legs: []navdata.ProcedureLeg{{ID: "ROUTE", PathTerminator: navdata.PathTF, FromFix: &origin, ToFix: &star}}},
			fixes: []navdata.Fix{{ID: origin, Position: navdata.Coordinate{LatitudeDeg: 55, LongitudeDeg: 12}}, {ID: star, Position: navdata.Coordinate{LatitudeDeg: 55.1, LongitudeDeg: 12.1}}, {ID: feederFix, Position: navdata.Coordinate{LatitudeDeg: 55.2, LongitudeDeg: 12.2}}},
		},
		Terminal: terminal.Configuration{
			ConfigVersion: "test-v1",
			Feeders:       []terminal.Feeder{{ID: "TESPI"}},
			Paths:         []terminal.Path{{Feeder: "TESPI", RunwayGroup: group}},
		},
	}}
	altitude, groundspeed, route, wake := 10_000, 300.0, "DCT TESPI", "L"
	observation := aman.FlightObservation{
		FlightID: "flight-identity", VATSIMCID: "123", Callsign: "SAS123", Origin: "ENGM", Destination: "EKCH",
		FiledRoute: &route, WakeCategory: &wake, ReconciledAt: now, SourceStatus: aman.DataFresh,
		Surveillance: &aman.SurveillanceFact{LatitudeDegrees: 55.01, LongitudeDegrees: 12.01, AltitudeFeet: &altitude, GroundspeedKnots: &groundspeed, ObservedAt: &now},
	}
	state := aman.AirportState{RunwayGroups: []aman.RunwayGroupPolicy{{ID: group, Selected: true}}}

	updated, err := service.reconcileFlight(context.Background(), state, newFlight(observation, now), observation, now)

	require.NoError(t, err)
	require.Equal(t, "TESPI", *updated.SelectedFeeder)
	require.Equal(t, "TESPI", *updated.SelectedSTARFamily)
	require.Equal(t, "TNO", *updated.SelectedFeederFix)
}

func TestResolvedLegacyTerminalPathDoesNotFabricateFeederFix(t *testing.T) {
	flight := aman.AMANFlight{}
	applyResolvedTerminalIdentity(&flight, navdata.TerminalPath{Feeder: "TESPI"})
	require.Equal(t, "TESPI", *flight.SelectedFeeder)
	require.Equal(t, "TESPI", *flight.SelectedSTARFamily)
	require.Nil(t, flight.SelectedFeederFix)
}

func TestSequenceInputIncludesEligibleLightAircraftRegardlessOfEngine(t *testing.T) {
	start := time.Date(2026, time.July, 23, 12, 0, 0, 0, time.UTC)
	effective, group, wake := start, aman.RunwayGroupID("ARRIVAL-22"), "L"
	config := terminal.Configuration{RunwayGroups: []terminal.RunwayGroup{{ID: group}}}
	for _, test := range []struct {
		name, aircraftType string
		engine             sat.EngineType
	}{{"piston", "C172", sat.EnginePiston}, {"non-piston", "LJ35", sat.EngineJet}} {
		t.Run(test.name, func(t *testing.T) {
			flight := operationalFlight("LIGHT", group, "MONAK", wake, start.Add(10*time.Minute))
			flight.LatestObservation.AircraftType = &test.aircraftType
			state := aman.AirportState{Revision: 1, RunwayGroups: []aman.RunwayGroupPolicy{{ID: group, ActiveRatePerHour: 20, RateEffectiveAt: &effective}}, Flights: []aman.AMANFlight{flight}}
			input := sequenceInputWithAircraft(state, config, testAircraftEngines{engine: test.engine, wtc: "L"})
			require.Len(t, input.Flights, 1)
			require.Equal(t, sequence.WakeCategory("L"), input.Flights[0].WakeCategory)
		})
	}
}

func TestLightFollowerKeepsThreeMinuteSeparation(t *testing.T) {
	start := time.Date(2026, time.July, 23, 12, 0, 0, 0, time.UTC)
	group := aman.RunwayGroupID("ARRIVAL-22")
	result, err := sequence.Generate(sequence.Input{
		Policies: []sequence.Policy{{RunwayGroupID: group, Rates: []sequence.RatePoint{{EffectiveAt: start, ArrivalsPerHour: 60}}, EarlyTolerance: 30 * time.Second, SeparationRules: amanCPHSeparations(), UnknownSeparation: 3 * time.Minute}},
		Flights:  []sequence.Flight{{ID: "LEADER", RunwayGroupID: group, State: aman.StateUnstable, OperationalTETA: start, WakeCategory: "M", FreezeReason: aman.FreezeNone}, {ID: "LIGHT", RunwayGroupID: group, State: aman.StateUnstable, OperationalTETA: start, WakeCategory: "L", FreezeReason: aman.FreezeNone}},
	})
	require.NoError(t, err)
	require.Len(t, result.Entries, 2)
	require.Equal(t, 180*time.Second, result.Entries[1].Time.Sub(result.Entries[0].Time))
}

func TestMissingLightRETARemainsExplicitlyDegradedAndUnsequenced(t *testing.T) {
	now := time.Date(2026, time.July, 23, 12, 0, 0, 0, time.UTC)
	service, err := New(Dependencies{
		Repository: &memoryRepository{}, Materializer: unavailableNavigation{}, Geometry: unavailableGeometry{}, Wind: unavailableWind{}, Publisher: &recordingPublisher{},
		Terminal: terminal.Configuration{Airport: "EKCH", ConfigVersion: "test", RunwayGroups: []terminal.RunwayGroup{{ID: "ARRIVAL-22"}}},
		Airports: []string{"EKCH"}, Mode: aman.ModeShadow, Now: func() time.Time { return now },
	})
	require.NoError(t, err)
	takeoff, eet, wake := now.Add(-time.Minute), 30*time.Minute, "L"
	observation := aman.FlightObservation{
		FlightID: "LIGHT", Callsign: "OYABC", Origin: "EKOD", Destination: "EKCH", WakeCategory: &wake,
		PlannedTiming: &aman.PlannedTiming{EstimatedEnrouteTime: &eet}, TakeoffDetected: &takeoff,
		ReconciledAt: now, SourceStatus: aman.DataFresh,
	}
	state := service.initialState("EKCH", now)
	flight, err := service.reconcileFlight(context.Background(), state, newFlight(observation, now), observation, now)
	require.NoError(t, err)
	require.NotNil(t, flight.Prediction)
	require.False(t, flight.Prediction.Publishable)
	require.Equal(t, "wtc_light_reta_unavailable:missing_essential_data:surveillance,filed_route", *flight.Prediction.DegradationReason)
	state.Flights = []aman.AMANFlight{flight}
	require.Empty(t, sequenceInput(state, service.deps.Terminal).Flights)
}

func TestLightRETAProvenanceSurvivesPersistedReplay(t *testing.T) {
	now := time.Date(2026, time.July, 23, 12, 0, 0, 0, time.UTC)
	reta, reason := now.Add(15*time.Minute), wtcLightRETAPolicyReason
	state := aman.AirportState{Flights: []aman.AMANFlight{{Prediction: &aman.Prediction{
		RawTETA: reta, RawRETA: &reta, OperationalTETA: reta, OperationalReason: aman.OperationalReasonPredicted,
		GeneratedAt: now, InputObservedAt: now, Confidence: aman.ConfidenceLow, Publishable: true, DegradationReason: &reason,
		DatasetVersion: "2609", GeometryDigest: "geometry", ModelVersion: "aman-cph-reta-v1", ConfigVersion: "test", Basis: aman.PredictionBasisRETA,
		Sources: []string{"vatsim:surveillance-groundspeed", "airacnet:route-distance"},
	}}}}
	encoded, err := json.Marshal(state)
	require.NoError(t, err)
	var replayed aman.AirportState
	require.NoError(t, json.Unmarshal(encoded, &replayed))
	prediction := replayed.Flights[0].Prediction
	require.NoError(t, prediction.Validate())
	require.Equal(t, aman.PredictionBasisRETA, prediction.Basis)
	require.Equal(t, prediction.RawTETA, *prediction.RawRETA)
	require.Equal(t, wtcLightRETAPolicyReason, *prediction.DegradationReason)
	require.Equal(t, state.Flights[0].Prediction.Sources, prediction.Sources)
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

func TestResequenceKeepsLateSuperstableFlightLockedAcrossReplay(t *testing.T) {
	start := time.Date(2026, time.July, 23, 12, 0, 0, 0, time.UTC)
	effective, group, wake := start, aman.RunwayGroupID("ARRIVAL-22"), "M"
	slot := aman.Slot{Time: start.Add(3 * time.Minute), RunwayGroupID: group, Sequence: 1, Revision: 7, Reason: string(sequence.ReasonFreezeSuperstable)}
	frozenAt := start.Add(-time.Minute)
	flight := operationalFlight("SUPERSTABLE", group, "MONAK", wake, slot.Time)
	flight.State = aman.StateStable
	flight.FreezeReason = aman.FreezeSuperstable
	flight.FrozenAt = &frozenAt
	flight.FrozenOperationalTETA = &slot.Time
	flight.FrozenSlot = &slot
	flight.Slot = &slot
	flight.Prediction.RawTETA = slot.Time.Add(gainResequenceThreshold + time.Second)
	flight.Prediction.OperationalTETA = slot.Time
	flight.Prediction.OperationalReason = aman.OperationalReasonSuperstableFreeze
	state := aman.AirportState{
		Revision: 7,
		RunwayGroups: []aman.RunwayGroupPolicy{{
			ID: group, ActiveRatePerHour: 20, RateEffectiveAt: &effective,
		}},
		Flights: []aman.AMANFlight{flight},
	}

	persisted, err := json.Marshal(state)
	require.NoError(t, err)
	var replayed aman.AirportState
	require.NoError(t, json.Unmarshal(persisted, &replayed))

	service := &Service{deps: Dependencies{Terminal: terminal.Configuration{RunwayGroups: []terminal.RunwayGroup{{ID: group}}}}}
	service.resequence(&replayed, start)
	updated := replayed.Flights[0]

	require.Equal(t, aman.FreezeSuperstable, updated.FreezeReason)
	require.Equal(t, slot.Time, updated.Prediction.OperationalTETA)
	require.Equal(t, slot.Time.Add(gainResequenceThreshold+time.Second), updated.Prediction.RawTETA, "raw drift must remain visible")
	require.Equal(t, slot, *updated.Slot)
	require.Equal(t, slot, *updated.FrozenSlot)
	require.Equal(t, slot.Time, *updated.FrozenOperationalTETA)
	require.Equal(t, frozenAt, *updated.FrozenAt)
}

func TestResequencePromotesVacancyAndBuildsSameRevisionAudit(t *testing.T) {
	start := time.Date(2026, time.July, 23, 12, 0, 0, 0, time.UTC)
	group, wake := aman.RunwayGroupID("ARRIVAL-22"), "M"
	service := &Service{deps: Dependencies{Terminal: terminal.Configuration{RunwayGroups: []terminal.RunwayGroup{{ID: group}}}}}
	lead := operationalFlight("LEAD", group, "MONAK", wake, start)
	lead.State = aman.StateStable
	lead.Slot = &aman.Slot{Time: start, RunwayGroupID: group, Sequence: 1, Revision: 7, Reason: "rate_wtc"}
	removed := operationalFlight("REMOVED", group, "MONAK", wake, start.Add(3*time.Minute))
	removed.State = aman.StateRemoved
	removed.Slot = &aman.Slot{Time: start.Add(3 * time.Minute), RunwayGroupID: group, Sequence: 2, Revision: 7, Reason: "rate_wtc"}
	target := operationalFlight("TARGET", group, "MONAK", wake, start.Add(3*time.Minute))
	target.State = aman.StateStable
	target.Slot = &aman.Slot{Time: start.Add(6 * time.Minute), RunwayGroupID: group, Sequence: 3, Revision: 7, Reason: "rate_wtc"}
	target.QueueOffers = []aman.QueueOffer{{
		FlightID: target.ID, RunwayGroupID: group,
		CandidateSlot: aman.Slot{Time: start.Add(3 * time.Minute), RunwayGroupID: group, Sequence: 2, Revision: 7, Reason: "rate_wtc"},
		QueuePosition: 1, ExpiresAt: start.Add(2 * time.Minute), AirportRevision: 7, Reason: aman.QueueOfferEarlierOccupiedSlot,
	}}
	state := aman.AirportState{
		Airport: "EKCH", Revision: 7,
		RunwayGroups: []aman.RunwayGroupPolicy{{ID: group, ActiveRatePerHour: 20, RateEffectiveAt: &start}},
		Flights:      []aman.AMANFlight{lead, removed, target},
	}

	promotions := service.resequence(&state, start)
	require.Len(t, promotions, 1)
	require.Equal(t, target.ID, promotions[0].FlightID)
	require.Equal(t, start.Add(3*time.Minute), state.Flights[2].Slot.Time)
	require.Equal(t, string(sequence.ReasonQueuePromotion), state.Flights[2].Slot.Reason)

	state.Revision++
	records := vacancyPromotionAuditRecords(state, promotions, start)
	require.Len(t, records, 1)
	require.Equal(t, state.Revision, records[0].Revision)
	require.Equal(t, "aman.queue_promotion", records[0].Category)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(records[0].Payload, &payload))
	require.Equal(t, "TARGET", payload["flight_id"])
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
		ActiveRouteFact: &aman.RouteFact{ID: "direct-to", Fix: "MONAK", State: aman.RouteFactActive},
	}

	updated := applyGroundedObservation(flight, observation, now)

	require.Equal(t, aman.StateLanded, updated.State)
	require.Equal(t, aman.LifecycleReasonLandingConfirmed, updated.Lifecycle.Reason)
	require.False(t, updated.Prediction.Publishable)
	require.Equal(t, "landed", *updated.Prediction.DegradationReason)
	require.Nil(t, updated.Slot)
	require.Nil(t, updated.SelectedHolding)
	require.Equal(t, aman.FreezeNone, updated.FreezeReason)
	require.Equal(t, aman.RouteFactExpired, updated.ActiveRouteFact.State)
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

func TestSetRateDoesNotChangeRunwaySelectionOrFlightAssignments(t *testing.T) {
	now := time.Date(2026, time.July, 23, 12, 0, 0, 0, time.UTC)
	service, err := New(Dependencies{
		Repository: &memoryRepository{}, Materializer: unavailableNavigation{}, Geometry: unavailableGeometry{}, Wind: unavailableWind{},
		Publisher: &recordingPublisher{}, Terminal: terminal.Configuration{Airport: "EKCH", ConfigVersion: "test", RunwayGroups: []terminal.RunwayGroup{{ID: "ARRIVAL-04"}, {ID: "ARRIVAL-22"}}},
		Airports: []string{"EKCH"}, Mode: aman.ModeShadow, Now: func() time.Time { return now },
	})
	require.NoError(t, err)
	state := service.initialState("EKCH", now)
	state.Revision = 7
	flight := operationalFlight("MOVABLE", "ARRIVAL-04", "MONAK", "M", now.Add(20*time.Minute))
	state.Flights = []aman.AMANFlight{flight}
	selectionBefore := append([]aman.RunwayGroupSelectionPoint(nil), state.RunwayGroups[0].SelectionSchedule...)

	mutation, err := service.SetRate(aman.CommandContext{ReceivedAt: now}, aman.SetRateCommand{
		Metadata: aman.CommandMetadata{CommandID: "rate-22", ExpectedRevision: 7}, RunwayGroupID: "ARRIVAL-22",
		ArrivalsPerHour: 30, EffectiveAt: now.Add(15 * time.Minute),
	})
	require.NoError(t, err)
	change, err := mutation(state)
	require.NoError(t, err)
	require.True(t, change.State.RunwayGroups[0].Selected)
	require.False(t, change.State.RunwayGroups[1].Selected)
	require.Equal(t, selectionBefore, change.State.RunwayGroups[0].SelectionSchedule)
	require.Empty(t, change.State.RunwayGroups[1].SelectionSchedule)
	require.Equal(t, aman.RunwayGroupID("ARRIVAL-04"), *change.State.Flights[0].SelectedRunwayGroup)
}

func TestSetActiveRunwayGroupsIsAtomicRevisionedAndIdempotent(t *testing.T) {
	now := time.Date(2026, time.July, 23, 12, 0, 0, 0, time.UTC)
	config := terminal.Configuration{
		Airport: "EKCH", ConfigVersion: "test",
		RunwayGroups:          []terminal.RunwayGroup{{ID: "ARRIVAL-04L"}, {ID: "ARRIVAL-04R"}, {ID: "ARRIVAL-22L"}},
		ActiveRunwayGroupSets: [][]aman.RunwayGroupID{{"ARRIVAL-04L"}, {"ARRIVAL-04L", "ARRIVAL-04R"}, {"ARRIVAL-22L"}},
		Paths: []terminal.Path{
			{Feeder: "MONAK", RunwayGroup: "ARRIVAL-04L"},
			{Feeder: "MONAK", RunwayGroup: "ARRIVAL-04R"},
		},
	}
	repository := &memoryRepository{}
	publisher := &recordingPublisher{}
	service, err := New(Dependencies{
		Repository: repository, Materializer: unavailableNavigation{}, Geometry: unavailableGeometry{}, Wind: unavailableWind{},
		Publisher: publisher, Terminal: config, Airports: []string{"EKCH"}, Mode: aman.ModeShadow, Now: func() time.Time { return now },
	})
	require.NoError(t, err)
	repository.state, repository.has = service.initialState("EKCH", now), true
	repository.state.Revision = 7
	coordinator, err := sequence.NewCoordinator(sequence.CoordinatorDependencies{
		States: repository, Outcomes: repository, Committer: repository, Publisher: publisher, Now: func() time.Time { return now },
	})
	require.NoError(t, err)
	actions, err := sequence.NewActionService(coordinator, service)
	require.NoError(t, err)
	auth := aman.CommandContext{Airport: "EKCH", Actor: "1234567", Role: "EKCH_FMH", ReceivedAt: now}
	command := aman.SetActiveRunwayGroupsCommand{
		Metadata:       aman.CommandMetadata{CommandID: "set-runways", ExpectedRevision: 7},
		RunwayGroupIDs: []aman.RunwayGroupID{"ARRIVAL-04R", "ARRIVAL-04L"},
	}

	first, err := actions.SetActiveRunwayGroups(context.Background(), auth, command)
	require.NoError(t, err)
	require.True(t, first.Changed)
	require.Equal(t, aman.SequenceRevision(8), first.CurrentRevision)
	require.Equal(t, []aman.RunwayGroupID{"ARRIVAL-04L", "ARRIVAL-04R"}, repository.state.ActiveRunwayGroups)
	require.Empty(t, repository.state.Flights)
	require.Contains(t, string(first.Outcome.Payload), `"actor":"1234567"`)
	require.Contains(t, string(first.Outcome.Payload), `"role":"EKCH_FMH"`)

	retry, err := actions.SetActiveRunwayGroups(context.Background(), auth, command)
	require.NoError(t, err)
	require.True(t, retry.Duplicate)
	require.False(t, retry.Changed)
	require.Equal(t, aman.SequenceRevision(8), retry.CurrentRevision)
	require.Len(t, repository.commits, 1)

	stale := command
	stale.Metadata.CommandID = "stale-set"
	_, err = actions.SetActiveRunwayGroups(context.Background(), auth, stale)
	requireDomainClass(t, err, aman.ErrorRevisionConflict)
	require.Equal(t, []aman.RunwayGroupID{"ARRIVAL-04L", "ARRIVAL-04R"}, repository.state.ActiveRunwayGroups)
}

func TestSetActiveRunwayGroupsDeterministicallyUsesEarliestOpportunityAndRetainsProtectedIncompatibility(t *testing.T) {
	now := time.Date(2026, time.July, 23, 12, 0, 0, 0, time.UTC)
	config := terminal.Configuration{
		Airport: "EKCH", RunwayGroups: []terminal.RunwayGroup{{ID: "A"}, {ID: "B"}, {ID: "C"}},
		ActiveRunwayGroupSets: [][]aman.RunwayGroupID{{"A", "B"}, {"C"}},
		Paths: []terminal.Path{
			{Feeder: "MONAK", RunwayGroup: "A"},
			{Feeder: "MONAK", RunwayGroup: "B"},
		},
	}
	service, err := New(Dependencies{
		Repository: &memoryRepository{}, Materializer: unavailableNavigation{}, Geometry: unavailableGeometry{}, Wind: unavailableWind{},
		Publisher: &recordingPublisher{}, Terminal: config, Airports: []string{"EKCH"}, Mode: aman.ModeShadow, Now: func() time.Time { return now },
	})
	require.NoError(t, err)
	state := service.initialState("EKCH", now)
	state.Revision = 7
	state.ActiveRunwayGroups = []aman.RunwayGroupID{"C"}
	for i := range state.RunwayGroups {
		state.RunwayGroups[i].Selected = state.RunwayGroups[i].ID == "C"
	}
	compatible := protectedOperationalFlight("PROTECTED-B", "B", "MONAK", "M", now.Add(6*time.Minute), 1, aman.FreezeManual)
	compatible.Slot = compatible.FrozenSlot
	incompatible := protectedOperationalFlight("PROTECTED-C", "C", "MONAK", "M", now.Add(9*time.Minute), 1, aman.FreezeManual)
	incompatible.Slot = incompatible.FrozenSlot
	state.Flights = []aman.AMANFlight{
		operationalFlight("MOVABLE", "C", "MONAK", "M", now.Add(6*time.Minute)),
		compatible,
		incompatible,
	}

	mutation, err := service.SetActiveRunwayGroups(aman.CommandContext{Airport: "EKCH", ReceivedAt: now}, aman.SetActiveRunwayGroupsCommand{
		RunwayGroupIDs: []aman.RunwayGroupID{"B", "A"},
	})
	require.NoError(t, err)
	change, err := mutation(state)
	require.NoError(t, err)
	require.Equal(t, []aman.RunwayGroupID{"A", "B"}, change.State.ActiveRunwayGroups)
	require.Equal(t, aman.RunwayGroupID("A"), *change.State.Flights[0].SelectedRunwayGroup)
	require.Equal(t, now.Add(6*time.Minute), change.State.Flights[0].Slot.Time)
	require.Equal(t, aman.RunwayGroupID("B"), *change.State.Flights[1].SelectedRunwayGroup)
	require.Equal(t, compatible.FrozenSlot, change.State.Flights[1].FrozenSlot)
	require.Equal(t, aman.RunwayGroupID("C"), *change.State.Flights[2].SelectedRunwayGroup)
	require.Equal(t, incompatible.FrozenSlot, change.State.Flights[2].FrozenSlot)
	require.Contains(t, string(change.Outcome), `"protected_incompatible_flight_ids":["PROTECTED-C"]`)

	reordered := state
	reordered.Flights = []aman.AMANFlight{incompatible, compatible, state.Flights[0]}
	reorderedChange, err := mutation(reordered)
	require.NoError(t, err)
	require.Equal(t, aman.RunwayGroupID("C"), *reorderedChange.State.Flights[0].SelectedRunwayGroup)
	require.Equal(t, aman.RunwayGroupID("B"), *reorderedChange.State.Flights[1].SelectedRunwayGroup)
	require.Equal(t, aman.RunwayGroupID("A"), *reorderedChange.State.Flights[2].SelectedRunwayGroup)
	require.Equal(t, change.State.Flights[0].Slot.Time, reorderedChange.State.Flights[2].Slot.Time)
}

func TestSetActiveRunwayGroupsRejectsUnassignableMovableFlightAtomically(t *testing.T) {
	now := time.Date(2026, time.July, 23, 12, 0, 0, 0, time.UTC)
	config := terminal.Configuration{
		Airport: "EKCH", RunwayGroups: []terminal.RunwayGroup{{ID: "A"}, {ID: "B"}},
		ActiveRunwayGroupSets: [][]aman.RunwayGroupID{{"A"}, {"B"}},
		Paths:                 []terminal.Path{{Feeder: "MONAK", RunwayGroup: "A"}},
	}
	service, err := New(Dependencies{
		Repository: &memoryRepository{}, Materializer: unavailableNavigation{}, Geometry: unavailableGeometry{}, Wind: unavailableWind{},
		Publisher: &recordingPublisher{}, Terminal: config, Airports: []string{"EKCH"}, Mode: aman.ModeShadow, Now: func() time.Time { return now },
	})
	require.NoError(t, err)
	state := service.initialState("EKCH", now)
	state.Flights = []aman.AMANFlight{operationalFlight("MOVABLE", "A", "MONAK", "M", now.Add(6*time.Minute))}
	before := state

	mutation, err := service.SetActiveRunwayGroups(aman.CommandContext{Airport: "EKCH", ReceivedAt: now}, aman.SetActiveRunwayGroupsCommand{RunwayGroupIDs: []aman.RunwayGroupID{"B"}})
	require.NoError(t, err)
	_, err = mutation(state)
	requireDomainClass(t, err, aman.ErrorInvalidTransition)
	require.Equal(t, before, state)
}

func TestSetActiveRunwayGroupsRejectsUnknownIncompatibleAndMismatchedConfiguration(t *testing.T) {
	now := time.Date(2026, time.July, 23, 12, 0, 0, 0, time.UTC)
	config := terminal.Configuration{
		Airport: "EKCH", RunwayGroups: []terminal.RunwayGroup{{ID: "A"}, {ID: "B"}, {ID: "C"}},
		ActiveRunwayGroupSets: [][]aman.RunwayGroupID{{"A"}, {"A", "B"}, {"C"}},
	}
	service := &Service{deps: Dependencies{Terminal: config}}
	base := aman.AirportState{
		Airport: "EKCH", ActiveRunwayGroups: []aman.RunwayGroupID{"A"},
		RunwayGroups: []aman.RunwayGroupPolicy{{ID: "A", Selected: true}, {ID: "B"}, {ID: "C"}},
	}
	auth := aman.CommandContext{Airport: "EKCH", Actor: "1234567", Role: "EKCH_FMH", ReceivedAt: now}

	for _, test := range []struct {
		name   string
		groups []aman.RunwayGroupID
		state  aman.AirportState
		class  aman.ErrorClass
	}{
		{name: "unknown", groups: []aman.RunwayGroupID{"MISSING"}, state: base, class: aman.ErrorNotFound},
		{name: "incompatible", groups: []aman.RunwayGroupID{"A", "C"}, state: base, class: aman.ErrorInvalidArgument},
		{name: "configuration mismatch", groups: []aman.RunwayGroupID{"A"}, state: func() aman.AirportState { value := base; value.RunwayGroups = value.RunwayGroups[:2]; return value }(), class: aman.ErrorInvalidArgument},
	} {
		t.Run(test.name, func(t *testing.T) {
			before := test.state
			mutation, err := service.SetActiveRunwayGroups(auth, aman.SetActiveRunwayGroupsCommand{RunwayGroupIDs: test.groups})
			require.NoError(t, err)
			_, err = mutation(test.state)
			requireDomainClass(t, err, test.class)
			require.Equal(t, before, test.state)
		})
	}
}

func TestImmediateRunwaySelectionMovesOnlyReorderableFlights(t *testing.T) {
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
	effectiveAt := now
	rateSchedules := [][]aman.RunwayGroupRatePoint{
		append([]aman.RunwayGroupRatePoint(nil), state.RunwayGroups[0].RateSchedule...),
		append([]aman.RunwayGroupRatePoint(nil), state.RunwayGroups[1].RateSchedule...),
	}

	mutation, err := service.SelectRunwayGroup(aman.CommandContext{ReceivedAt: now}, aman.SelectRunwayGroupCommand{
		Metadata:      aman.CommandMetadata{CommandID: "select-22", ExpectedRevision: 7},
		RunwayGroupID: "ARRIVAL-22", EffectiveAt: effectiveAt,
	})
	require.NoError(t, err)
	change, err := mutation(state)
	require.NoError(t, err)
	require.False(t, change.State.RunwayGroups[0].Selected)
	require.True(t, change.State.RunwayGroups[1].Selected)
	require.Equal(t, aman.RunwayGroupID("ARRIVAL-22"), *change.State.Flights[0].SelectedRunwayGroup)
	require.Equal(t, aman.RunwayGroupID("ARRIVAL-04"), *change.State.Flights[1].SelectedRunwayGroup)
	require.Equal(t, rateSchedules[0], change.State.RunwayGroups[0].RateSchedule)
	require.Equal(t, rateSchedules[1], change.State.RunwayGroups[1].RateSchedule)
	require.Contains(t, string(change.Outcome), `"protected_flight_ids":["STABLE"]`)
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

func TestLegacyRateCreatedSelectionSchedulesAreDiscarded(t *testing.T) {
	now := time.Date(2026, time.July, 23, 12, 0, 0, 0, time.UTC)
	groups := []aman.RunwayGroupPolicy{
		{ID: "ARRIVAL-04", SelectionSchedule: []aman.RunwayGroupSelectionPoint{{EffectiveAt: now, CommandRevision: 7}}},
		{ID: "ARRIVAL-22", SelectionSchedule: []aman.RunwayGroupSelectionPoint{{
			EffectiveAt: now.Add(time.Hour), CommandRevision: 8, Source: aman.RunwayGroupSelectionSourceFMPCommand,
		}}},
	}

	_, reset := discardLegacyRunwayGroupSelections(groups)

	require.Empty(t, groups[0].SelectionSchedule)
	require.Len(t, groups[1].SelectionSchedule, 1)
	require.True(t, hasRunwayGroupSelectionSchedule(groups))
	require.False(t, reset)

	legacyOnly := []aman.RunwayGroupPolicy{
		{ID: "ARRIVAL-04"},
		{ID: "ARRIVAL-22", Selected: true, SelectionSchedule: []aman.RunwayGroupSelectionPoint{{EffectiveAt: now, CommandRevision: 7}}},
	}
	selected, reset := discardLegacyRunwayGroupSelections(legacyOnly)
	require.True(t, reset)
	require.Equal(t, aman.RunwayGroupID("ARRIVAL-04"), selected)
	require.True(t, legacyOnly[0].Selected)
	require.False(t, legacyOnly[1].Selected)
}

func TestScheduledRunwaySelectionConflictRetainsPreviousStateAndReportsConflict(t *testing.T) {
	now := time.Date(2026, time.July, 23, 12, 0, 0, 0, time.UTC)
	service, err := New(Dependencies{
		Repository: &memoryRepository{}, Materializer: unavailableNavigation{}, Geometry: unavailableGeometry{}, Wind: unavailableWind{},
		Publisher: &recordingPublisher{}, Terminal: terminal.Configuration{Airport: "EKCH", ConfigVersion: "test", RunwayGroups: []terminal.RunwayGroup{{ID: "ARRIVAL-04"}, {ID: "ARRIVAL-22"}}},
		Airports: []string{"EKCH"}, Mode: aman.ModeShadow, Now: func() time.Time { return now },
	})
	require.NoError(t, err)
	state := service.initialState("EKCH", now)
	state.Revision = 7
	state.RunwayGroups[1].SelectionSchedule = []aman.RunwayGroupSelectionPoint{{
		EffectiveAt: now, CommandRevision: 7, Source: aman.RunwayGroupSelectionSourceFMPCommand,
	}}
	movable := operationalFlight("MOVABLE", "ARRIVAL-04", "MONAK", "M", now.Add(20*time.Minute))
	movable.Slot = &aman.Slot{Time: now.Add(20 * time.Minute), RunwayGroupID: "ARRIVAL-04", Sequence: 1, Revision: 7, Reason: "rate_wtc"}
	lead := protectedOperationalFlight("LEAD", "ARRIVAL-22", "MONAK", "M", now.Add(10*time.Minute), 1, aman.FreezeManual)
	trail := protectedOperationalFlight("TRAIL", "ARRIVAL-22", "MONAK", "M", now.Add(11*time.Minute), 2, aman.FreezeSuperstable)
	state.Flights = []aman.AMANFlight{movable, lead, trail}

	selected, changed := selectedRunwayGroupAt(state.RunwayGroups, now)
	require.True(t, changed)
	err = service.activateRunwayGroup(&state, selected, now)
	require.Error(t, err)
	setRunwayGroupSelectionConflict(state.RunwayGroups, selected, err.Error())

	require.True(t, state.RunwayGroups[0].Selected)
	require.False(t, state.RunwayGroups[1].Selected)
	require.NotNil(t, state.RunwayGroups[1].SelectionConflict)
	require.Equal(t, aman.RunwayGroupID("ARRIVAL-04"), *state.Flights[0].SelectedRunwayGroup)
	require.NotNil(t, state.Flights[0].Slot)
}

func TestFutureRunwaySelectionCommandsPreserveSelectionHistory(t *testing.T) {
	now := time.Date(2026, time.July, 23, 12, 0, 0, 0, time.UTC)
	service, err := New(Dependencies{
		Repository: &memoryRepository{}, Materializer: unavailableNavigation{}, Geometry: unavailableGeometry{}, Wind: unavailableWind{},
		Publisher: &recordingPublisher{}, Terminal: terminal.Configuration{Airport: "EKCH", ConfigVersion: "test", RunwayGroups: []terminal.RunwayGroup{{ID: "ARRIVAL-04"}, {ID: "ARRIVAL-22"}}},
		Airports: []string{"EKCH"}, Mode: aman.ModeShadow, Now: func() time.Time { return now },
	})
	require.NoError(t, err)
	state := service.initialState("EKCH", now)
	state.Revision = 7

	applySelection := func(group aman.RunwayGroupID, effectiveAt time.Time) {
		mutation, mutationErr := service.SelectRunwayGroup(aman.CommandContext{ReceivedAt: now}, aman.SelectRunwayGroupCommand{
			Metadata: aman.CommandMetadata{
				CommandID:        "select-" + string(group) + "-" + effectiveAt.Format("1504"),
				ExpectedRevision: state.Revision,
			},
			RunwayGroupID: group, EffectiveAt: effectiveAt,
		})
		require.NoError(t, mutationErr)
		change, changeErr := mutation(state)
		require.NoError(t, changeErr)
		state = change.State
		state.Revision++
	}

	first, second, third := now.Add(10*time.Minute), now.Add(20*time.Minute), now.Add(30*time.Minute)
	applySelection("ARRIVAL-22", first)
	applySelection("ARRIVAL-04", second)
	applySelection("ARRIVAL-22", third)

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
	manualFeederETA := now.Add(2 * time.Minute)
	derivedFeederETA := now.Add(4 * time.Minute)
	flight.FeederETA = &aman.FeederETAState{ETA: &manualFeederETA, Source: aman.FeederETASourceManual}
	flight.DerivedFeederETA = &aman.FeederETAState{ETA: &derivedFeederETA, Source: aman.FeederETASourceRoute}
	flight.ActiveRouteFact = &aman.RouteFact{ID: "direct-to", Fix: "MONAK", State: aman.RouteFactActive}
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
	require.Equal(t, aman.RouteFactExpired, updated.ActiveRouteFact.State)
	require.Nil(t, updated.FeederETA)
	require.Nil(t, updated.DerivedFeederETA)
	require.True(t, updated.GoAroundDetection.AwaitingReset, "manual reporting suppresses a duplicate detector prompt")
	require.NotNil(t, change.QueueOffers)
}

func TestRemovedFlightExpiresActiveRouteFact(t *testing.T) {
	now := time.Date(2026, time.July, 23, 12, 0, 0, 0, time.UTC)
	flight := aman.AMANFlight{
		State:           aman.StateAirborne,
		ActiveRouteFact: &aman.RouteFact{ID: "direct-to", Fix: "MONAK", State: aman.RouteFactActive},
		Lifecycle: &aman.LifecycleState{
			Absence: &aman.AbsenceState{MissingSince: now.Add(-time.Minute), RemovalDueAt: &now},
		},
	}

	markMissing(&flight, now)

	require.Equal(t, aman.StateRemoved, flight.State)
	require.Equal(t, aman.RouteFactExpired, flight.ActiveRouteFact.State)
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

func TestRouteFeederETASumsAcceptedLegDurations(t *testing.T) {
	now := time.Date(2026, time.September, 11, 18, 0, 0, 0, time.UTC)
	legs := []trajectory.RemainingLeg{{To: "BEFORE"}, {To: "TNO"}, {To: "RUNWAY"}}

	got := routeFeederETA(now, []time.Duration{2 * time.Minute, 3 * time.Minute, 4 * time.Minute}, legs, "TNO", trajectory.FeederProgressAhead)

	require.NotNil(t, got)
	require.Equal(t, now.Add(5*time.Minute), *got.ETA)
	require.Equal(t, aman.FeederETASourceRoute, got.Source)
	require.False(t, got.Passed)
}

func TestRouteFeederETAMissingFeederLegIsUnavailable(t *testing.T) {
	now := time.Date(2026, time.September, 11, 18, 0, 0, 0, time.UTC)
	legs := []trajectory.RemainingLeg{{To: "BEFORE"}, {To: "RUNWAY"}}

	require.Nil(t, routeFeederETA(now, []time.Duration{2 * time.Minute, 4 * time.Minute}, legs, "TNO", trajectory.FeederProgressUnknown))
	require.Nil(t, routeFeederETA(now, []time.Duration{2 * time.Minute}, legs, "TNO", trajectory.FeederProgressAhead))
}

func TestRouteFeederETAPassedUsesExplicitTerminalState(t *testing.T) {
	now := time.Date(2026, time.September, 11, 18, 0, 0, 0, time.UTC)

	got := routeFeederETA(now, []time.Duration{4 * time.Minute}, []trajectory.RemainingLeg{{To: "RUNWAY"}}, "TNO", trajectory.FeederProgressPassed)

	require.Equal(t, &aman.FeederETAState{Source: aman.FeederETASourcePassed, Passed: true}, got)
}

func TestRouteFeederETAUsesGeometryIdentityAndModelDurations(t *testing.T) {
	now := time.Date(2026, time.September, 11, 18, 0, 0, 0, time.UTC)
	legs := []trajectory.RemainingLeg{
		{ID: "ACTIVE-GEOMETRY-1", To: "TNO", DistanceNM: 900},
		{ID: "ACTIVE-GEOMETRY-2", To: "RUNWAY", DistanceNM: 1},
	}

	got := routeFeederETA(now, []time.Duration{90 * time.Second, 12 * time.Hour}, legs, "TNO", trajectory.FeederProgressAhead)

	require.Equal(t, now.Add(90*time.Second), *got.ETA, "ETA must retain the accepted model duration rather than estimate from distance")
	require.Equal(t, aman.FeederETASourceRoute, got.Source)
}

func TestRouteFeederETARecalculatesDeterministically(t *testing.T) {
	now := time.Date(2026, time.September, 11, 18, 0, 0, 0, time.UTC)
	legs := []trajectory.RemainingLeg{{To: "TNO"}, {To: "RUNWAY"}}

	first := routeFeederETA(now, []time.Duration{3 * time.Minute, 5 * time.Minute}, legs, "TNO", trajectory.FeederProgressAhead)
	replay := routeFeederETA(now, []time.Duration{3 * time.Minute, 5 * time.Minute}, legs, "TNO", trajectory.FeederProgressAhead)
	recalculated := routeFeederETA(now.Add(time.Minute), []time.Duration{2 * time.Minute, 5 * time.Minute}, legs, "TNO", trajectory.FeederProgressAhead)

	require.Equal(t, first, replay)
	require.Equal(t, *first.ETA, *recalculated.ETA, "equivalent accepted inputs must reproduce the same absolute ETA")
}

func TestLifecycleStateUsesFeederETAAfterPersistedUnstableDwell(t *testing.T) {
	now := time.Date(2026, time.September, 11, 18, 0, 0, 0, time.UTC)
	config := lifecycle.DefaultConfig()
	flight := operationalFlight("SAS123", "ARRIVAL-22", "TESPI", "M", now.Add(5*time.Minute))
	flight.Lifecycle = &aman.LifecycleState{EnteredAt: now.Add(-config.MinimumUnstableDwell), Reason: aman.LifecycleReasonUnstableHorizon}

	require.Equal(t, aman.StateUnstable, lifecycleState(config, flight, now.Add(5*time.Minute), now), "landing TETA must not substitute for missing feeder ETA")
	feederETA := now.Add(config.StableHorizon)
	flight.FeederETA = &aman.FeederETAState{ETA: &feederETA, Source: aman.FeederETASourceHolding}
	require.Equal(t, aman.StateStable, lifecycleState(config, flight, now.Add(time.Hour), now), "the feeder boundary controls Stable independently of landing TETA")
}

func TestApplySuperstableUsesInclusiveFeederBoundaryAndAuthoritativePassedState(t *testing.T) {
	now := time.Date(2026, time.September, 11, 19, 0, 0, 0, time.UTC)
	config := lifecycle.DefaultConfig()
	tests := []struct {
		name        string
		feeder      *aman.FeederETAState
		withoutSlot bool
		want        bool
	}{
		{name: "above threshold", feeder: feederETAAt(now.Add(config.SuperstableHorizon + time.Nanosecond))},
		{name: "exact threshold", feeder: feederETAAt(now.Add(config.SuperstableHorizon)), want: true},
		{name: "below threshold", feeder: feederETAAt(now.Add(config.SuperstableHorizon - time.Nanosecond)), want: true},
		{name: "authoritatively passed", feeder: &aman.FeederETAState{Source: aman.FeederETASourcePassed, Passed: true}, want: true},
		{name: "missing feeder"},
		{name: "missing slot", feeder: feederETAAt(now.Add(config.SuperstableHorizon)), withoutSlot: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flight := superstableCandidate(now, tt.feeder)
			if tt.withoutSlot {
				flight.Slot = nil
			}
			require.Equal(t, tt.want, applySuperstable(config, &flight, aman.FreezeNone, now))
			if !tt.want {
				require.Equal(t, aman.FreezeNone, flight.FreezeReason)
				return
			}
			require.Equal(t, aman.StateStable, flight.State)
			require.Equal(t, aman.FreezeSuperstable, flight.FreezeReason)
			require.Equal(t, now, *flight.FrozenAt)
			require.Equal(t, flight.Prediction.OperationalTETA, *flight.FrozenOperationalTETA)
			require.Equal(t, *flight.Slot, *flight.FrozenSlot)
			require.Equal(t, aman.OperationalReasonSuperstableFreeze, flight.Prediction.OperationalReason)
		})
	}
}

func TestLateFeederEntryCompletesDwellAndCapturesStableInOneResult(t *testing.T) {
	now := time.Date(2026, time.September, 11, 20, 0, 0, 0, time.UTC)
	config := lifecycle.DefaultConfig()
	flight := superstableCandidate(now, feederETAAt(now.Add(config.SuperstableHorizon)))
	flight.State = aman.StateUnstable
	flight.Lifecycle = &aman.LifecycleState{
		EnteredAt: now.Add(-config.MinimumUnstableDwell), Reason: aman.LifecycleReasonUnstableHorizon,
		LastEventID: "unstable", LastEventFingerprint: "unstable", LastEventAt: now.Add(-config.MinimumUnstableDwell),
	}

	previousState := flight.State
	nextState := lifecycleState(config, flight, now.Add(30*time.Minute), now)
	reduced, err := prediction.Reduce(prediction.DefaultConfig(), flight, prediction.Input{
		Raw: acceptedRawPrediction(now, now.Add(18*time.Minute)), State: nextState, Slot: flight.Slot,
	})
	require.NoError(t, err)
	updated := reduced.Flight
	updateLifecycle(&updated, previousState, nextState, now)
	require.True(t, applySuperstable(config, &updated, aman.FreezeNone, now))

	require.Equal(t, aman.StateStable, updated.State)
	require.Equal(t, aman.LifecycleReasonStableHorizon, updated.Lifecycle.Reason)
	require.Equal(t, aman.FreezeSuperstable, updated.FreezeReason)
	require.Equal(t, updated.Prediction.OperationalTETA, *updated.FrozenOperationalTETA)
	require.Equal(t, *updated.Slot, *updated.FrozenSlot)
}

func TestSuperstableResultIsAuditedAndIdempotentAcrossRestartReplay(t *testing.T) {
	now := time.Date(2026, time.September, 11, 21, 0, 0, 0, time.UTC)
	config := lifecycle.DefaultConfig()
	flight := superstableCandidate(now, feederETAAt(now.Add(config.SuperstableHorizon)))
	require.True(t, applySuperstable(config, &flight, aman.FreezeNone, now))

	record, err := superstableAuditRecord("EKCH", flight, now)
	require.NoError(t, err)
	record.Revision = 7
	require.Equal(t, "aman.superstable_applied", record.Category)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(record.Payload, &payload))
	require.Equal(t, string(flight.ID), payload["flight_id"])
	require.Equal(t, string(aman.StateStable), payload["state"])
	require.Equal(t, string(aman.FreezeSuperstable), payload["freeze_reason"])
	repository := &memoryRepository{}
	state := aman.AirportState{
		Airport: "EKCH", Revision: 7, GeneratedAt: now, PolicyVersion: policyVersion, Mode: aman.ModeShadow,
		Flights: []aman.AMANFlight{flight}, RunwayGroups: []aman.RunwayGroupPolicy{{ID: "ARRIVAL-22", Selected: true}},
		ActiveRunwayGroups: []aman.RunwayGroupID{"ARRIVAL-22"},
	}
	_, err = repository.Commit(context.Background(), aman.StateCommit{
		ExpectedRevision: 6, State: state, AuditRecords: []aman.AuditRecord{record},
	})
	require.NoError(t, err)
	require.Len(t, repository.commits, 1)
	require.Equal(t, aman.FreezeSuperstable, repository.commits[0].State.Flights[0].FreezeReason)
	require.Equal(t, record, repository.commits[0].AuditRecords[0])

	persisted, err := json.Marshal(repository.state.Flights[0])
	require.NoError(t, err)
	var restarted aman.AMANFlight
	require.NoError(t, json.Unmarshal(persisted, &restarted))
	replayed := restarted
	require.False(t, applySuperstable(config, &replayed, restarted.FreezeReason, now), "an already committed freeze must not emit a second result")
	require.Equal(t, restarted, replayed)
}

func feederETAAt(at time.Time) *aman.FeederETAState {
	return &aman.FeederETAState{ETA: &at, Source: aman.FeederETASourceRoute}
}

func superstableCandidate(now time.Time, feeder *aman.FeederETAState) aman.AMANFlight {
	group := aman.RunwayGroupID("ARRIVAL-22")
	slot := aman.Slot{Time: now.Add(20 * time.Minute), RunwayGroupID: group, Sequence: 2, Revision: 7, Reason: "spacing"}
	flight := operationalFlight("SAS123", group, "TESPI", "M", now.Add(18*time.Minute))
	feederFix := "TNO"
	prediction := acceptedRawPrediction(now, now.Add(18*time.Minute))
	prediction.OperationalTETA, prediction.OperationalReason = prediction.RawTETA, aman.OperationalReasonPredicted
	flight.VATSIMCID, flight.CurrentCallsign, flight.DataStatus = "1234567", "SAS123", aman.DataFresh
	flight.State, flight.SelectedSTARFamily, flight.SelectedFeederFix, flight.FeederETA = aman.StateStable, flight.SelectedFeeder, &feederFix, feeder
	flight.Slot, flight.Prediction, flight.LatestObservation, flight.UpdatedAt = &slot, &prediction, nil, now
	return flight
}

func acceptedRawPrediction(generatedAt, rawTETA time.Time) aman.Prediction {
	return aman.Prediction{
		RawTETA: rawTETA, GeneratedAt: generatedAt, InputObservedAt: generatedAt,
		Confidence: aman.ConfidenceHigh, Publishable: true, DatasetVersion: "2609",
		GeometryDigest: "geometry", ModelVersion: "performance-wind-v1", ConfigVersion: "ekch-v1",
		Sources: []string{"vatsim", "airacnet"},
	}
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

func TestRefreshHoldingPlansDerivesFeederETAFromAuthoritativeReleaseAndConfiguredTransit(t *testing.T) {
	now := time.Date(2026, time.September, 11, 18, 0, 0, 0, time.UTC)
	service, state := holdingFeederETAServiceState(now, "TESPI", "TNO", "EKCH-TESPI-PRIMARY", "ARRIVAL-22L", int64Pointer(195))
	state.Flights[0].Prediction.ModelVersion = "performance-wind-v7"

	service.refreshHoldingPlans(&state)

	flight := state.Flights[0]
	require.NotNil(t, flight.Prediction.HoldingPlan)
	require.Equal(t, now.Add(20*time.Minute), flight.Prediction.HoldingPlan.ApproachReleaseTime)
	require.Equal(t, &aman.FeederETAState{
		ETA: timePointer(now.Add(23*time.Minute + 15*time.Second)), Source: aman.FeederETASourceHolding,
	}, flight.FeederETA)
	require.Equal(t, "performance-wind-v7", flight.Prediction.ModelVersion)
	require.Equal(t, "ekch-test-v1", flight.Prediction.ConfigVersion)

	first := flight.FeederETA
	service.refreshHoldingPlans(&state)
	require.Equal(t, first, state.Flights[0].FeederETA, "replaying the same release and configuration must be deterministic")
}

func TestRefreshHoldingPlansAllowsZeroDurationWhenHoldingAndFeederAreERNOV(t *testing.T) {
	now := time.Date(2026, time.September, 11, 18, 0, 0, 0, time.UTC)
	service, state := holdingFeederETAServiceState(now, "ERNOV", "ERNOV", "EKCH-ERNOV-PRIMARY", "ARRIVAL-22L", int64Pointer(0))

	service.refreshHoldingPlans(&state)

	require.Equal(t, now.Add(20*time.Minute), *state.Flights[0].FeederETA.ETA)
	require.Equal(t, aman.FeederETASourceHolding, state.Flights[0].FeederETA.Source)
}

func TestRefreshHoldingPlansExposesAbsentTransitAsUnavailable(t *testing.T) {
	now := time.Date(2026, time.September, 11, 18, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		family, feederFix, holding string
		group                      aman.RunwayGroupID
	}{
		{family: "MONAK", feederFix: "KUBIS", holding: "EKCH-MONAK-PRIMARY", group: "ARRIVAL-30"},
		{family: "TIDVU", feederFix: "WUPJA", holding: "EKCH-TIDVU-PRIMARY", group: "ARRIVAL-12"},
	} {
		t.Run(test.feederFix, func(t *testing.T) {
			service, state := holdingFeederETAServiceState(now, test.family, test.feederFix, test.holding, test.group, nil)
			routeETA := now.Add(7 * time.Minute)
			state.Flights[0].FeederETA = &aman.FeederETAState{ETA: &routeETA, Source: aman.FeederETASourceRoute}

			service.refreshHoldingPlans(&state)

			require.NotNil(t, state.Flights[0].Prediction.HoldingPlan)
			require.Nil(t, state.Flights[0].FeederETA, "an active hold without approved transit must not retain route ETA or serialize zero")
		})
	}
}

func TestRefreshHoldingPlansRejectsInactiveAndNonAuthoritativeHoldingETA(t *testing.T) {
	now := time.Date(2026, time.September, 11, 18, 0, 0, 0, time.UTC)
	service, authoritative := holdingFeederETAServiceState(now, "TESPI", "TNO", "EKCH-TESPI-PRIMARY", "ARRIVAL-22L", int64Pointer(195))
	staleETA := now.Add(time.Minute)
	authoritative.Flights[0].FeederETA = &aman.FeederETAState{ETA: &staleETA, Source: aman.FeederETASourceHolding}
	authoritative.Flights[0].Slot = nil

	service.refreshHoldingPlans(&authoritative)

	require.Nil(t, authoritative.Flights[0].Prediction.HoldingPlan)
	require.Nil(t, authoritative.Flights[0].FeederETA)

	_, shadow := holdingFeederETAServiceState(now, "TESPI", "TNO", "EKCH-TESPI-PRIMARY", "ARRIVAL-22L", int64Pointer(195))
	shadow.Authoritative = false
	routeETA := now.Add(6 * time.Minute)
	shadow.Flights[0].FeederETA = &aman.FeederETAState{ETA: &routeETA, Source: aman.FeederETASourceRoute}

	service.refreshHoldingPlans(&shadow)

	require.NotNil(t, shadow.Flights[0].Prediction.HoldingPlan)
	require.Equal(t, aman.FeederETASourceRoute, shadow.Flights[0].FeederETA.Source)
	require.Equal(t, routeETA, *shadow.Flights[0].FeederETA.ETA)
}

func TestHoldingFeederETARequiresMatchingPredictionConfigAndPreservesPassedProvenance(t *testing.T) {
	now := time.Date(2026, time.September, 11, 18, 0, 0, 0, time.UTC)
	service, state := holdingFeederETAServiceState(now, "TESPI", "TNO", "EKCH-TESPI-PRIMARY", "ARRIVAL-22L", int64Pointer(195))
	state.Flights[0].Prediction.ConfigVersion = "stale-config"

	service.refreshHoldingPlans(&state)
	require.Nil(t, state.Flights[0].FeederETA)

	state.Flights[0].Prediction.ConfigVersion = service.deps.Terminal.ConfigVersion
	state.Flights[0].FeederETA = &aman.FeederETAState{Source: aman.FeederETASourcePassed, Passed: true}
	service.refreshHoldingPlans(&state)
	require.Equal(t, &aman.FeederETAState{Source: aman.FeederETASourcePassed, Passed: true}, state.Flights[0].FeederETA)
}

func holdingFeederETAServiceState(now time.Time, family, feederFix, holding string, group aman.RunwayGroupID, transit *int64) (*Service, aman.AirportState) {
	config := terminal.Configuration{
		ConfigVersion: "ekch-test-v1",
		Paths: []terminal.Path{{
			Feeder: navdata.FeederID(family), STARFamily: navdata.STARFamilyID(family), FeederFix: navdata.FixID(feederFix),
			HoldingToFeederSeconds: transit, RunwayGroup: group, SelectedHolding: navdata.HoldingID(holding),
		}},
	}
	service := &Service{deps: Dependencies{Terminal: config}}
	entry := now.Add(8 * time.Minute)
	return service, aman.AirportState{
		Authoritative: true,
		Flights: []aman.AMANFlight{{
			SelectedFeeder: &family, SelectedSTARFamily: &family, SelectedFeederFix: &feederFix,
			SelectedHolding: &holding, SelectedRunwayGroup: &group, Slot: &aman.Slot{Time: now.Add(32 * time.Minute)},
			Prediction: &aman.Prediction{
				Publishable: true, RawTETA: now.Add(20 * time.Minute), HoldingFixETA: &entry,
				ModelVersion: "test-model", ConfigVersion: config.ConfigVersion,
			},
		}},
	}
}

func int64Pointer(value int64) *int64 { return &value }

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
	state    aman.AirportState
	has      bool
	commits  []aman.StateCommit
	outcomes map[string]aman.CommandOutcome
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
	if commit.CommandOutcome != nil && r.outcomes != nil {
		if outcome, exists := r.outcomes[commit.CommandOutcome.CommandID]; exists {
			return aman.CommitResult{State: r.state, CommandOutcome: &outcome, DuplicateCommand: true}, nil
		}
	}
	if err := commit.Validate(); err != nil {
		return aman.CommitResult{}, err
	}
	r.state, r.has = commit.State, true
	r.commits = append(r.commits, commit)
	result := aman.CommitResult{State: commit.State}
	if commit.CommandOutcome != nil {
		if r.outcomes == nil {
			r.outcomes = make(map[string]aman.CommandOutcome)
		}
		outcome := *commit.CommandOutcome
		r.outcomes[outcome.CommandID] = outcome
		result.CommandOutcome = &outcome
	}
	return result, nil
}

func (r *memoryRepository) LoadCommandOutcome(_ context.Context, commandID string) (aman.CommandOutcome, error) {
	if outcome, exists := r.outcomes[commandID]; exists {
		return outcome, nil
	}
	return aman.CommandOutcome{}, &aman.DomainError{Class: aman.ErrorNotFound, Message: "missing"}
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

type fixedNavigation struct{ key navdata.RouteKey }

func (n fixedNavigation) MaterializeRoute(context.Context, navdata.RouteQuery, string) (navdata.RouteKey, error) {
	return n.key, nil
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

type terminalIdentityGeometry struct {
	version navdata.DatasetVersion
	path    navdata.TerminalPath
	route   navdata.RouteGeometry
	fixes   []navdata.Fix
}

func (g terminalIdentityGeometry) ActiveVersion(context.Context, navdata.AirportID) (navdata.DatasetVersion, error) {
	return g.version, nil
}

func (g terminalIdentityGeometry) Route(context.Context, navdata.RouteKey) (navdata.RouteGeometry, error) {
	return g.route, nil
}

func (g terminalIdentityGeometry) TerminalPath(context.Context, navdata.AirportID, navdata.FeederID, aman.RunwayGroupID) (navdata.TerminalPath, error) {
	return g.path, nil
}

func (g terminalIdentityGeometry) ActiveGeometrySnapshot(context.Context, navdata.AirportID) (navdata.ActiveGeometrySnapshot, error) {
	return navdata.ActiveGeometrySnapshot{Manifest: navdata.ManifestCandidate{Version: g.version}, ManifestRevision: 1, Fixes: g.fixes, TerminalPaths: []navdata.TerminalPath{g.path}}, nil
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

func requireDomainClass(t *testing.T, err error, class aman.ErrorClass) {
	t.Helper()
	var domain *aman.DomainError
	require.ErrorAs(t, err, &domain)
	require.Equal(t, class, domain.Class)
}
