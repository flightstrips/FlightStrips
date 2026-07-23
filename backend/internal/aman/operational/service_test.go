package operational

import (
	"context"
	"errors"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/materializer"
	"FlightStrips/internal/aman/navdata"
	"FlightStrips/internal/aman/predictor"
	"FlightStrips/internal/aman/sequence"
	"FlightStrips/internal/aman/terminal"
	"FlightStrips/internal/aman/trajectory"
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

func TestPreliminaryPredictionsUseDocumentedPlannedAndAirborneTimes(t *testing.T) {
	now := time.Date(2026, time.July, 23, 12, 0, 0, 0, time.UTC)
	eobt := now.Add(time.Hour)
	eet := 90 * time.Minute
	observation := aman.FlightObservation{PlannedTiming: &aman.PlannedTiming{EstimatedOffBlockTime: &eobt, EstimatedEnrouteTime: &eet}}
	planned := aman.AMANFlight{State: aman.StatePlanned}
	applyPreliminaryPrediction(&planned, observation, now)
	require.Equal(t, eobt.Add(15*time.Minute+eet), planned.Prediction.RawTETA)
	require.Equal(t, "aman-planned-eobt-exot-eet-v1", planned.Prediction.ModelVersion)

	takeoff := now.Add(5 * time.Minute)
	observation.TakeoffDetected = &takeoff
	airborne := aman.AMANFlight{State: aman.StateAirborne}
	applyPreliminaryPrediction(&airborne, observation, now)
	require.Equal(t, takeoff.Add(eet), airborne.Prediction.RawTETA)
	require.Equal(t, "aman-airborne-takeoff-eet-v1", airborne.Prediction.ModelVersion)

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
	require.Equal(t, uint32(20), group.ActiveRatePerHour)
	require.Len(t, group.RateSchedule, 2)
	require.Equal(t, []sequence.RatePoint{
		{EffectiveAt: now, ArrivalsPerHour: 20},
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
	require.Equal(t, now.Add(10*time.Minute), updated.Prediction.OperationalTETA)
	require.Equal(t, aman.OperationalReasonGoAround, updated.Prediction.OperationalReason)
	require.True(t, updated.Prediction.Publishable)
	require.NotNil(t, updated.Slot)
	require.False(t, updated.Slot.Time.Before(now.Add(10*time.Minute)))
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

func TestFailedNavigationRefreshRetriesAndHealthFailsClosed(t *testing.T) {
	now := time.Date(2026, time.July, 23, 12, 0, 0, 0, time.UTC)
	navigation := &countingNavigation{}
	service, err := New(Dependencies{
		Repository: &memoryRepository{}, Materializer: navigation, Geometry: unavailableGeometry{}, Wind: unavailableWind{},
		Publisher: &recordingPublisher{}, Terminal: terminal.Configuration{Airport: "EKCH", ConfigVersion: "test", RunwayGroups: []terminal.RunwayGroup{{ID: "ARRIVAL-22"}}},
		Airports: []string{"EKCH"}, Mode: aman.ModeAuthoritative, Now: func() time.Time { return now },
	})
	require.NoError(t, err)
	require.False(t, service.TechnicalHealth(context.Background()).AuthorityAllowed)
	require.NoError(t, service.ObserveSourceHealth(context.Background(), aman.DataFresh, now))
	require.Error(t, service.refreshNavigation(context.Background(), "EKCH"))
	require.Error(t, service.refreshNavigation(context.Background(), "EKCH"))
	require.Equal(t, 2, navigation.attempts)
	health := service.TechnicalHealth(context.Background())
	require.Equal(t, aman.HealthReady, health.VATSIM.Status)
	require.Equal(t, aman.HealthUnavailable, health.Navigation.Status)
	require.False(t, health.AuthorityAllowed)
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

func (unavailableNavigation) Refresh(context.Context, materializer.Request) error {
	return errors.New("offline")
}
func (unavailableNavigation) MaterializeRoute(context.Context, navdata.RouteQuery, string) (navdata.RouteKey, error) {
	return "", errors.New("offline")
}

type countingNavigation struct{ attempts int }

func (n *countingNavigation) Refresh(context.Context, materializer.Request) error {
	n.attempts++
	return errors.New("offline")
}
func (*countingNavigation) MaterializeRoute(context.Context, navdata.RouteQuery, string) (navdata.RouteKey, error) {
	return "", errors.New("offline")
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
