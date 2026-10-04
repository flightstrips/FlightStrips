package trafficprediction

import (
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/predictor"
	"github.com/stretchr/testify/require"
)

func TestTrafficWarningsExplainMissingDepartureInputsAndResolveWithTiming(t *testing.T) {
	now := utc(2026, time.October, 4, 12, 0)
	duration := 90 * time.Minute
	departure := now.Add(10 * time.Minute)
	for _, test := range []struct {
		name, code string
		timing     *aman.PlannedTiming
		missing    []string
	}{
		{"both", "missing_departure_timing", nil, []string{"off-block time (EOBT)", "enroute duration (EET)"}},
		{"departure", "missing_departure_time", &aman.PlannedTiming{EstimatedEnrouteTime: &duration}, []string{"off-block time (EOBT)"}},
		{"duration", "missing_flight_duration", &aman.PlannedTiming{EstimatedOffBlockTime: &departure}, []string{"enroute duration (EET)"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := baseState(now, 40)
			state.Flights = []aman.AMANFlight{{Callsign: "BAW822", State: aman.StatePlanned, DataStatus: aman.DataFresh,
				LatestObservation: &aman.FlightObservation{Origin: "EGLL", Provider: aman.ObservationProviderEuroScope,
					FlightPlan: aman.FlightPlanFact{ObservedAt: &now}, PlannedTiming: test.timing}}}
			model := Build(state, readyHealth())
			require.Len(t, model.Warnings, 1)
			warning := model.Warnings[0]
			require.Equal(t, aman.WarningSourceTrafficPrediction, warning.Source)
			require.Equal(t, aman.Callsign("BAW822"), *warning.Callsign)
			require.Equal(t, test.code, warning.Code)
			for _, missing := range test.missing {
				require.Contains(t, warning.Message, missing)
			}
			require.Contains(t, warning.Message, "planned at EGLL")
			require.Contains(t, warning.Message, "Last EuroScope observation")
			state.Flights[0].LatestObservation.PlannedTiming = &aman.PlannedTiming{EstimatedOffBlockTime: &departure, EstimatedEnrouteTime: &duration}
			model = Build(state, readyHealth())
			require.Empty(t, model.Warnings)
			require.NotContains(t, model.DegradedReasons, "missing_timing:BAW822")
			landing, _, _, ok := landingTime(state.Flights[0], now, AirportPosition{})
			require.True(t, ok)
			require.Equal(t, departure.Add(predictor.DefaultEXOT+duration), landing)
		})
	}
}

func TestTrafficWarningsExplainMissingAirborneTimingAndRetainCommittedSlots(t *testing.T) {
	now := utc(2026, time.October, 4, 12, 0)
	old := now.Add(-5 * time.Minute)
	state := baseState(now, 40)
	state.Flights = []aman.AMANFlight{{Callsign: "SAS101", State: aman.StateAirborne, DataStatus: aman.DataDisconnected,
		LatestObservation: &aman.FlightObservation{Provider: aman.ObservationProviderEuroScope, Surveillance: &aman.SurveillanceFact{ObservedAt: &old}}}}
	model := Build(state, readyHealth())
	require.Len(t, model.Warnings, 1)
	require.Contains(t, model.Warnings[0].Message, "aircraft updates are disconnected")
	require.Contains(t, model.Warnings[0].Message, "more than two minutes old")
	require.Contains(t, model.Warnings[0].Message, "groundspeed is missing")
	require.Contains(t, model.Warnings[0].Message, "omitted from arrival counts")
	state.Flights[0].Slot = &aman.Slot{Time: now.Add(20 * time.Minute)}
	model = Build(state, readyHealth())
	require.NotContains(t, model.DegradedReasons, "missing_timing:SAS101")
	require.Len(t, model.Warnings, 1)
	require.Equal(t, "disconnected_flight_data", model.Warnings[0].Code)
	require.Contains(t, model.Warnings[0].Message, "retains its last known arrival time")
	require.Equal(t, 1, model.Buckets[1].Count)
	state.Flights[0].Slot.Time = now.Add(-time.Hour)
	model = Build(state, readyHealth())
	require.Empty(t, model.Warnings, "retained traffic outside the chart must not add unrelated warnings")
}

func TestTrafficWarningsDeduplicateRateAndFlightConditions(t *testing.T) {
	now := utc(2026, time.October, 4, 12, 0)
	state := baseState(now, 40)
	state.RunwayGroups = nil
	state.Flights = []aman.AMANFlight{{Callsign: "SAS101", State: aman.StatePlanned}, {Callsign: "SAS101", State: aman.StatePlanned}}
	model := Build(state, readyHealth())
	require.Len(t, model.Warnings, 2)
	require.Equal(t, model.Warnings[0].ID, model.Warnings[0].Identity())
	require.Equal(t, model.Warnings[1].ID, model.Warnings[1].Identity())
}

func TestTrafficWarningsDistinguishFreshMessagesFromRetainedArrivalEstimates(t *testing.T) {
	now := utc(2026, time.October, 4, 12, 0)
	for _, test := range []struct{ reason, detail string }{
		{"missing_essential_data:groundspeed", "missing required data: groundspeed"},
		{"wtc_light_reta_unavailable:missing_essential_data:surveillance,filed_route", "missing required data: aircraft position, filed route"},
		{"unknown_star_family", "does not match a configured STAR entry"},
		{"raw prediction: invalid_argument: prediction segment is invalid", "calculated flight segment failed validation"},
		{"route geometry is not publishable: unresolved", "usable remaining route geometry (unresolved)"},
	} {
		t.Run(test.reason, func(t *testing.T) {
			state := retainedDiagnosticState(now, test.reason, aman.DataFresh, now)
			model := Build(state, readyHealth())
			require.Len(t, model.Warnings, 1)
			warning := model.Warnings[0]
			require.Equal(t, "arrival_prediction_not_updated", warning.Code)
			require.Contains(t, warning.Message, "Aircraft messages are arriving")
			require.Contains(t, warning.Message, test.detail)
			require.Contains(t, warning.Message, "Keeping assigned arrival time 12:20 UTC")
			require.Contains(t, warning.Message, "Retained estimate calculated at 2026-10-04 11:55:00 UTC")
			require.Contains(t, warning.Message, "source status: fresh")
			require.NotContains(t, warning.Message, "stale aircraft updates")
			require.NotContains(t, warning.Message, "data status: stale")
			require.Equal(t, 1, model.Buckets[1].Count, "diagnostic changes must retain the assigned arrival")
			require.Equal(t, now.Add(20*time.Minute), state.Flights[0].Slot.Time)

			// Fresh accepted timing resolves the warning, even when wind accuracy
			// remains degraded and the assigned arrival itself is unchanged.
			state.Flights[0].DataStatus = aman.DataFresh
			fallback := "WEATHER_ESTIMATED_FROM_SURVEILLANCE"
			state.Flights[0].Prediction.DegradationReason = &fallback
			require.Empty(t, Build(state, readyHealth()).Warnings)
		})
	}
}

func TestTrafficWarningsDescribeRealSourceStalenessAndPositionClockProblems(t *testing.T) {
	now := utc(2026, time.October, 4, 12, 0)
	stale := retainedDiagnosticState(now, "WEATHER_ESTIMATED_FROM_SURVEILLANCE", aman.DataStale, now.Add(-3*time.Minute))
	model := Build(stale, readyHealth())
	require.Len(t, model.Warnings, 1)
	require.Equal(t, "stale_flight_data", model.Warnings[0].Code)
	require.Contains(t, model.Warnings[0].Message, "aircraft source reports stale data")
	require.Contains(t, model.Warnings[0].Message, "3m0s before this AMAN update")
	require.NotContains(t, model.Warnings[0].Message, "WEATHER_ESTIMATED_FROM_SURVEILLANCE", "a previous accepted wind fallback is not a current failure")

	for _, test := range []struct {
		at     time.Time
		detail string
		age    string
	}{
		{now.Add(-3 * time.Minute), "position is more than two minutes old", "3m0s before"},
		{now.Add(5 * time.Second), "position timestamp is ahead of the AMAN update time", "timestamp is 5s ahead"},
		{now.Add(100 * time.Millisecond), "position timestamp is ahead of the AMAN update time", "timestamp is less than 1s ahead"},
	} {
		state := retainedDiagnosticState(now, "tma_entry_surveillance_stale", aman.DataFresh, test.at)
		model = Build(state, readyHealth())
		require.Len(t, model.Warnings, 1)
		require.Equal(t, "arrival_prediction_not_updated", model.Warnings[0].Code)
		require.Contains(t, model.Warnings[0].Message, test.detail)
		require.Contains(t, model.Warnings[0].Message, test.age)
		if test.at.After(now) {
			require.NotContains(t, model.Warnings[0].Message, "0s before")
		}
	}
}

func TestMissingTimingWarningDoesNotCallFreshIncompleteMessagesStale(t *testing.T) {
	now := utc(2026, time.October, 4, 12, 0)
	state := retainedDiagnosticState(now, "missing_essential_data:groundspeed", aman.DataFresh, now)
	state.Flights[0].Slot = nil
	state.Flights[0].Prediction.Publishable = false
	state.Flights[0].LatestObservation.Surveillance.GroundspeedKnots = nil
	model := BuildWithAirportPosition(state, readyHealth(), AirportPosition{LatitudeDegrees: 55.618, LongitudeDegrees: 12.656})
	require.Len(t, model.Warnings, 1)
	require.Equal(t, "arrival_prediction_not_updated", model.Warnings[0].Code)
	require.Contains(t, model.Warnings[0].Message, "missing required data: groundspeed")
	require.NotContains(t, model.Warnings[0].Message, "aircraft updates are stale")
}

func TestObservationDetailsIdentifyWhichTimestampIsAvailable(t *testing.T) {
	now := utc(2026, time.October, 4, 12, 0)
	flight := aman.AMANFlight{DataStatus: aman.DataStale, LatestObservation: &aman.FlightObservation{
		Provider: aman.ObservationProviderEuroScope, SourceStatus: aman.DataFresh, ReconciledAt: now,
		FlightPlan: aman.FlightPlanFact{ObservedAt: &now},
	}}
	require.Contains(t, observationDetail(flight, now), "observation (flight plan)")
	flight.LatestObservation.FlightPlan.ObservedAt = nil
	require.Contains(t, observationDetail(flight, now), "observation (received message)")
	require.Contains(t, observationDetail(flight, now), "source status: fresh")
	flight.LatestObservation.ReconciledAt = time.Time{}
	require.Equal(t, "Last observation time is unavailable.", observationDetail(flight, now))
}

func retainedDiagnosticState(now time.Time, reason string, source aman.DataStatus, observedAt time.Time) aman.AirportState {
	altitude, speed := 12000, 250.0
	state := baseState(now, 40)
	state.Flights = []aman.AMANFlight{{Callsign: "NJE634Y", State: aman.StateStable, DataStatus: aman.DataStale,
		Slot: &aman.Slot{Time: now.Add(20 * time.Minute)},
		Prediction: &aman.Prediction{RawTETA: now.Add(20 * time.Minute), OperationalTETA: now.Add(20 * time.Minute),
			GeneratedAt: now.Add(-5 * time.Minute), Publishable: true, DegradationReason: &reason},
		LatestObservation: &aman.FlightObservation{Provider: aman.ObservationProviderEuroScope, SourceStatus: source,
			Surveillance: &aman.SurveillanceFact{LatitudeDegrees: 55.1, LongitudeDegrees: 12.1, AltitudeFeet: &altitude, GroundspeedKnots: &speed, ObservedAt: &observedAt}},
	}}
	return state
}
