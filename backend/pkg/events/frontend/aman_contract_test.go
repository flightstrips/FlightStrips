package frontend

import (
	"encoding/json"
	"os"
	"strconv"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/navdata"
	"FlightStrips/internal/aman/prediction"

	"github.com/stretchr/testify/require"
)

// Export real publisher output for the TypeScript consumer. The contract job
// exercises the wire, including marshalling, rather than hand-maintained DTOs.
func TestAMANStateConsumerContract(t *testing.T) {
	type scenario struct {
		Name  string          `json:"name"`
		Event json.RawMessage `json:"event"`
	}
	var scenarios []scenario
	add := func(name string, state aman.AirportState, health aman.TechnicalHealth) {
		t.Helper()
		for index := range state.Flights {
			if state.Flights[index].Slot != nil {
				state.Flights[index].Slot.Revision = state.Revision
			}
		}
		event, err := NewAMANStateEvent(state, health.EffectiveMode, health)
		require.NoError(t, err, name)
		// The app transport adds versioned terminal mappings before marshalling.
		left, right := navdata.STARFamilyID("TESPI"), navdata.STARFamilyID("TUDLO")
		event.Data.TimelineConfig = ProjectAMANTimelineConfig("contract-terminal-v1", []navdata.TimelineMapping{{ID: 1, Left: &left, Right: &right}})
		wire, err := event.Marshal()
		require.NoError(t, err, name)
		scenarios = append(scenarios, scenario{Name: name, Event: wire})
	}
	add("healthy", goldenAMANState(), goldenAMANHealth())

	// Reproduce the observations immediately before the reported 19:04 CET
	// outage with the production filter, then publish its resulting warning.
	type filterSample struct {
		LoggedAt           string `json:"logged_at_ns"`
		ObservedAt         string `json:"observed_at_ns"`
		CandidateETA       string `json:"candidate_eta_ns"`
		CandidateFeederETA string `json:"candidate_feeder_eta_ns"`
		Outcome            string `json:"outcome"`
	}
	var incident struct {
		Callsign        string         `json:"callsign"`
		SessionID       int32          `json:"session_id"`
		AcceptedETA     string         `json:"accepted_eta_ns"`
		Samples         []filterSample `json:"samples"`
		RecoverySamples []filterSample `json:"recovery_samples"`
	}
	bytes, err := os.ReadFile("testdata/aman-filter-2026-10-26.json")
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(bytes, &incident))
	nanos := func(value string) time.Time {
		t.Helper()
		n, err := strconv.ParseInt(value, 10, 64)
		require.NoError(t, err)
		return time.Unix(0, n).UTC()
	}
	state := goldenAMANState()
	state.SessionID = incident.SessionID
	flight := state.Flights[0]
	flight.Callsign = incident.Callsign
	flight.Prediction.RawTETA = nanos(incident.AcceptedETA)
	flight.Prediction.OperationalTETA = flight.Prediction.RawTETA
	for index, sample := range incident.Samples {
		raw := *flight.Prediction
		raw.RawTETA = nanos(sample.CandidateETA)
		raw.InputObservedAt, raw.GeneratedAt = nanos(sample.ObservedAt), nanos(sample.LoggedAt)
		feeder := nanos(sample.CandidateFeederETA)
		var accepted bool
		var outcome string
		flight, accepted, outcome = prediction.Filter(flight, raw, &feeder, false, false)
		require.False(t, accepted)
		require.Equal(t, sample.Outcome, outcome)
		state.GeneratedAt, state.Flights[0] = raw.GeneratedAt, flight
		state.Revision++
		if index == 0 {
			add("isolated_spike", state, goldenAMANHealth())
		}
	}
	require.True(t, flight.PredictionFilter.Unreliable)
	require.False(t, flight.Prediction.Publishable)
	add("production_unreliable", state, goldenAMANHealth())

	// A healthy neighbouring aircraft must remain visible in the same replacement.
	neighbour := goldenAMANState().Flights[0]
	shift := state.GeneratedAt.Sub(neighbour.Prediction.GeneratedAt)
	neighbour.UpdatedAt = neighbour.UpdatedAt.Add(shift)
	neighbour.Prediction.GeneratedAt = state.GeneratedAt
	neighbour.Prediction.InputObservedAt = neighbour.Prediction.InputObservedAt.Add(shift)
	neighbour.Prediction.RawTETA = neighbour.Prediction.RawTETA.Add(shift)
	neighbour.Prediction.OperationalTETA = neighbour.Prediction.OperationalTETA.Add(shift)
	neighbour.Prediction.HoldingFixETA = timePointer(neighbour.Prediction.HoldingFixETA.Add(shift))
	neighbour.Prediction.HoldingPlan.HoldingEntryTime = neighbour.Prediction.HoldingPlan.HoldingEntryTime.Add(shift)
	neighbour.Prediction.HoldingPlan.ApproachReleaseTime = neighbour.Prediction.HoldingPlan.ApproachReleaseTime.Add(shift)
	neighbour.Slot.Time = neighbour.Slot.Time.Add(shift)
	neighbour.FeederETA.ETA = timePointer(neighbour.FeederETA.ETA.Add(shift))
	neighbour.ActiveRouteFact.ObservedAt = neighbour.ActiveRouteFact.ObservedAt.Add(shift)
	state.Flights = append(state.Flights, neighbour)
	add("unreliable_with_healthy_neighbour", state, goldenAMANHealth())

	// Source outages also expire pending candidates without another observation.
	stale := goldenAMANState()
	first := *stale.Flights[0].Prediction
	first.RawTETA = first.RawTETA.Add(10 * time.Minute)
	first.InputObservedAt, first.GeneratedAt = stale.GeneratedAt, stale.GeneratedAt
	stale.Flights[0], _, _ = prediction.Filter(stale.Flights[0], first, nil, false, false)
	stale.GeneratedAt = stale.GeneratedAt.Add(2 * time.Minute)
	stale.Flights[0] = prediction.ExpireFilter(stale.Flights[0], stale.GeneratedAt)
	require.True(t, stale.Flights[0].PredictionFilter.Unreliable)
	add("stale_observation_unreliable", stale, goldenAMANHealth())

	// Replay the real next observations too: the first remains inconsistent,
	// then the consistent pair at 19:06 CET restores guidance automatically.
	require.Len(t, incident.RecoverySamples, 2)
	for _, sample := range incident.RecoverySamples {
		raw := *flight.Prediction
		raw.RawTETA = nanos(sample.CandidateETA)
		raw.InputObservedAt, raw.GeneratedAt = nanos(sample.ObservedAt), nanos(sample.LoggedAt)
		raw.Publishable, raw.DegradationReason = true, nil
		feeder := nanos(sample.CandidateFeederETA)
		updated, accepted, outcome := prediction.Filter(flight, raw, &feeder, false, false)
		require.Equal(t, sample.Outcome, outcome)
		flight = updated
		if accepted {
			reduced, err := prediction.Reduce(prediction.DefaultConfig(), flight, prediction.Input{Raw: raw, State: flight.State, Slot: flight.Slot})
			require.NoError(t, err)
			flight = reduced.Flight
		}
		state.GeneratedAt, state.Flights[0] = raw.GeneratedAt, flight
		state.Revision++
	}
	require.False(t, flight.PredictionFilter.Unreliable)
	require.True(t, flight.Prediction.Publishable)
	add("recovered", state, goldenAMANHealth())

	passed := goldenAMANState()
	passed.Flights[0].FeederETA = &aman.FeederETAState{Source: aman.FeederETASourcePassed, Passed: true}
	add("passed_feeder", passed, goldenAMANHealth())
	degraded := goldenAMANHealth()
	degraded.Status = aman.HealthDegraded
	degraded.Weather = aman.ComponentHealth{Status: aman.HealthUnavailable, Reason: "weather_refresh_failed"}
	add("degraded_weather", goldenAMANState(), degraded)
	flightDegraded := goldenAMANState()
	flightDegraded.Flights[0].DataStatus = aman.DataStale
	reason := "missing_essential_data:groundspeed"
	flightDegraded.Flights[0].Prediction.DegradationReason = &reason
	add("single_flight_input_degraded", flightDegraded, goldenAMANHealth())

	// A real airport-wide source outage restricts authority, but the last
	// complete timeline and traffic data must still render with diagnostics.
	outage := goldenAMANHealth()
	outage.Status, outage.Ready, outage.AuthorityAllowed = aman.HealthDegraded, false, false
	outage.EffectiveMode = aman.EffectiveBlocked
	outage.VATSIM = aman.ComponentHealth{Status: aman.HealthUnavailable, Reason: "source_disconnected"}
	outage.BlockedReasons = []string{"observation_source:source_disconnected"}
	outageState := goldenAMANState()
	outageState.Authoritative = false
	add("airport_source_outage", outageState, outage)
	empty := goldenAMANState()
	empty.Flights = nil
	add("empty_airport", empty, goldenAMANHealth())

	if path := os.Getenv("AMAN_CONTRACT_OUTPUT"); path != "" {
		encoded, err := json.MarshalIndent(scenarios, "", "  ")
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(path, encoded, 0600))
	}
}
