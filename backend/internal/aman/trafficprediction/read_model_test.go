package trafficprediction

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/predictor"
	"github.com/stretchr/testify/require"
)

func TestBuildUsesDeterministicHalfOpenQuarterHourRangeAcrossMidnight(t *testing.T) {
	now := utc(2026, time.July, 22, 23, 44)
	state := baseState(now, 20)
	state.Flights = []aman.AMANFlight{
		planned("before", "1", utc(2026, time.July, 22, 23, 29), aman.DataFresh),
		planned("start", "2", utc(2026, time.July, 22, 23, 30), aman.DataFresh),
		planned("boundary", "3", utc(2026, time.July, 22, 23, 45), aman.DataFresh),
		planned("end", "4", utc(2026, time.July, 23, 2, 30), aman.DataFresh),
	}

	model := Build(state, readyHealth())
	require.Equal(t, utc(2026, time.July, 22, 23, 30), model.RangeStart)
	require.Equal(t, utc(2026, time.July, 23, 2, 30), model.RangeEnd)
	require.Len(t, model.Buckets, 12)
	require.Equal(t, []string{"START"}, callsigns(model.Buckets[0]))
	require.Equal(t, []string{"BOUNDARY"}, callsigns(model.Buckets[1]))
	for _, bucket := range model.Buckets {
		require.NotContains(t, callsigns(bucket), "END")
	}
}

func TestBuildDeduplicatesAMANOverVATSIMPrediction(t *testing.T) {
	now := utc(2026, time.July, 22, 20, 44)
	state := baseState(now, 20)
	api := planned("api", "42", now.Add(20*time.Minute), aman.DataFresh)
	api.Callsign = "SAS42"
	amanFlight := airborne("aman", "42", now.Add(35*time.Minute), aman.StateStable, aman.DataFresh)
	amanFlight.Callsign = "SAS42"
	state.Flights = []aman.AMANFlight{api, amanFlight}

	model := Build(state, readyHealth())
	require.Equal(t, 0, model.Buckets[2].Count, "discarded API time must not remain")
	require.Equal(t, 1, model.Buckets[3].Count)
	require.Equal(t, SourceAMAN, model.Buckets[3].Flights[0].TimingSource)
}

func TestBuildDoesNotFallBackToVATSIMWhenAuthoritativeAMANTimingIsMissing(t *testing.T) {
	now := utc(2026, time.July, 22, 20, 44)
	state := baseState(now, 20)
	api := planned("api", "42", now.Add(20*time.Minute), aman.DataFresh)
	api.Callsign = "SAS42"
	amanFlight := airborne("aman", "42", time.Time{}, aman.StateStable, aman.DataFresh)
	amanFlight.Callsign = "SAS42"
	amanFlight.Prediction = nil
	state.Flights = []aman.AMANFlight{api, amanFlight}

	model := Build(state, readyHealth())
	require.Contains(t, model.DegradedReasons, "missing_timing:SAS42")
	for _, bucket := range model.Buckets {
		require.Zero(t, bucket.Count, "the duplicate API prediction must remain suppressed")
	}
}

func TestBuildUsesFreshLiveArrivalForNearbyAirborneFlightWithoutFiledTiming(t *testing.T) {
	now := utc(2026, time.July, 22, 20, 44)
	state := baseState(now, 20)
	altitude, groundspeed := 18000, 420.0
	airport := AirportPosition{LatitudeDegrees: 55.618, LongitudeDegrees: 12.656}
	flight := aman.AMANFlight{
		Callsign: "SAS202", State: aman.StateAirborne, DataStatus: aman.DataFresh,
		LatestObservation: &aman.FlightObservation{
			Surveillance: &aman.SurveillanceFact{
				LatitudeDegrees: 55.1, LongitudeDegrees: 12.1, AltitudeFeet: &altitude,
				GroundspeedKnots: &groundspeed, ObservedAt: &now,
			},
		},
	}
	state.Flights = []aman.AMANFlight{flight}

	model := BuildWithAirportPosition(state, readyHealth(), airport)
	require.NotContains(t, model.DegradedReasons, "missing_timing:SAS202")
	require.Equal(t, 1, model.Buckets[1].Count)
	require.Equal(t, SourceAirbornePosition, model.Buckets[1].Flights[0].TimingSource)
	require.Contains(t, model.DegradedReasons, "position_estimate")

	flight.Prediction = &aman.Prediction{OperationalTETA: now.Add(90 * time.Minute), Publishable: true, ModelVersion: "aman-airborne-takeoff-eet-v1"}
	state.Flights[0] = flight
	model = BuildWithAirportPosition(state, readyHealth(), airport)
	require.Equal(t, 1, model.Buckets[1].Count, "current position must supersede an airborne flight-plan estimate")

	flight.State = aman.StateUnstable
	state.Flights[0] = flight
	model = BuildWithAirportPosition(state, readyHealth(), airport)
	require.Equal(t, SourceAirbornePosition, model.Buckets[1].Flights[0].TimingSource, "a preliminary estimate is not authoritative AMAN timing")
	flight.State = aman.StateAirborne

	// An accepted route prediction still wins; an expired position observation
	// cannot stand in for current airborne timing.
	flight.Prediction = &aman.Prediction{OperationalTETA: now.Add(20 * time.Minute), Publishable: true, Basis: aman.PredictionBasisPerformanceWind}
	state.Flights[0] = flight
	model = BuildWithAirportPosition(state, readyHealth(), airport)
	require.Equal(t, 0, model.Buckets[1].Count)
	require.Equal(t, 1, model.Buckets[2].Count)

	flight.Prediction = nil
	staleObserved := now.Add(-3 * time.Minute)
	flight.LatestObservation.Surveillance.ObservedAt = &staleObserved
	state.Flights[0] = flight
	model = BuildWithAirportPosition(state, readyHealth(), airport)
	require.Contains(t, model.DegradedReasons, "missing_timing:SAS202")
}

func TestBuildNeverUsesEOBTForAnAirborneFlight(t *testing.T) {
	now := utc(2026, time.July, 22, 20, 44)
	eobt, eet := now.Add(-time.Hour), 90*time.Minute
	flight := aman.AMANFlight{
		Callsign: "SAS202", State: aman.StateAirborne, DataStatus: aman.DataFresh,
		Prediction:        &aman.Prediction{OperationalTETA: eobt.Add(eet), Publishable: true, ModelVersion: "aman-planned-eobt-exot-eet-v1"},
		LatestObservation: &aman.FlightObservation{PlannedTiming: &aman.PlannedTiming{EstimatedOffBlockTime: &eobt, EstimatedEnrouteTime: &eet}},
	}
	state := baseState(now, 20)
	state.Flights = []aman.AMANFlight{flight}

	model := BuildWithAirportPosition(state, readyHealth(), AirportPosition{LatitudeDegrees: 55.618, LongitudeDegrees: 12.656})
	require.Contains(t, model.DegradedReasons, "missing_timing:SAS202")
	for _, bucket := range model.Buckets {
		require.Zero(t, bucket.Count)
	}
}

func TestBuildTreatsMovingPositionAsAirborneEvenWithPlannedLifecycle(t *testing.T) {
	now := utc(2026, time.July, 22, 20, 44)
	eobt, eet := now.Add(-time.Hour), 90*time.Minute
	altitude, groundspeed := 3000, 70.0
	flight := aman.AMANFlight{
		Callsign: "SAS202", State: aman.StatePlanned, DataStatus: aman.DataFresh,
		LatestObservation: &aman.FlightObservation{
			PlannedTiming: &aman.PlannedTiming{EstimatedOffBlockTime: &eobt, EstimatedEnrouteTime: &eet},
			Surveillance: &aman.SurveillanceFact{
				LatitudeDegrees: 55.5, LongitudeDegrees: 12.5, AltitudeFeet: &altitude,
				GroundspeedKnots: &groundspeed, ObservedAt: &now,
			},
		},
	}
	state := baseState(now, 20)
	state.Flights = []aman.AMANFlight{flight}

	model := BuildWithAirportPosition(state, readyHealth(), AirportPosition{LatitudeDegrees: 55.618, LongitudeDegrees: 12.656})
	require.NotContains(t, model.DegradedReasons, "missing_timing:SAS202")
	count := 0
	for _, bucket := range model.Buckets {
		count += bucket.AirborneCount
		for _, entry := range bucket.Flights {
			require.Equal(t, SourceAirbornePosition, entry.TimingSource)
			require.True(t, entry.Airborne)
		}
	}
	require.Equal(t, 1, count)
}

func TestBuildThresholdEqualityAndOneHourWindow(t *testing.T) {
	now := utc(2026, time.July, 22, 20, 30)
	state := baseState(now, 40)
	// 44 aircraft is exactly 110% of a rate of 40 for a one-hour window: not high.
	for index := 0; index < 44; index++ {
		state.Flights = append(state.Flights, plannedID(index, now.Add(time.Duration(index%4)*15*time.Minute+time.Minute)))
	}
	model := Build(state, readyHealth())
	require.False(t, model.Buckets[1].WindowHigh)
	require.Equal(t, AlertNone, model.Buckets[1].Alert)

	state.Flights = append(state.Flights, plannedID(50, now.Add(time.Minute)))
	model = Build(state, readyHealth())
	require.True(t, model.Buckets[1].WindowHigh)
	require.False(t, model.Buckets[1].BucketHigh)
	require.Equal(t, AlertYellow, model.Buckets[1].Alert)

	// Move the extra aircraft into the current bucket: its load and the window are both high.
	state.Flights[len(state.Flights)-1] = plannedID(50, now.Add(16*time.Minute))
	model = Build(state, readyHealth())
	require.True(t, model.Buckets[1].BucketHigh)
	require.True(t, model.Buckets[1].WindowHigh)
	require.Equal(t, AlertRed, model.Buckets[1].Alert)
}

func TestBuildPublishesStaleMissingTimingAndMissingRateDegradation(t *testing.T) {
	now := utc(2026, time.July, 22, 20, 44)
	state := baseState(now, 20)
	state.RunwayGroups = nil
	state.Flights = []aman.AMANFlight{planned("stale", "1", now.Add(time.Minute), aman.DataStale), {Callsign: "UNKNOWN", State: aman.StatePlanned, DataStatus: aman.DataDisconnected}}

	model := Build(state, readyHealth())
	require.Equal(t, StatusDegraded, model.Status)
	require.Equal(t, aman.DataFresh, model.SourceStatus)
	require.Contains(t, model.DegradedReasons, "stale_flight_data")
	require.Contains(t, model.DegradedReasons, "disconnected_flight_data")
	require.NotContains(t, model.DegradedReasons, "source_disconnected")
	require.Contains(t, model.DegradedReasons, "missing_selected_rate")
	require.Contains(t, model.DegradedReasons, "missing_timing:UNKNOWN")
	require.Nil(t, model.Buckets[0].SelectedRate)
}

func TestBuildUsesSelectedGroupAndScheduledRateAtEachBucket(t *testing.T) {
	now := utc(2026, time.July, 22, 20, 44)
	state := baseState(now, 20)
	other := aman.RunwayGroupPolicy{ID: "04", ActiveRatePerHour: 30, RateEffectiveAt: timePtr(now.Add(-time.Hour)), RateSchedule: []aman.RunwayGroupRatePoint{{EffectiveAt: now.Add(-time.Hour), ArrivalsPerHour: 30}}, SelectionSchedule: []aman.RunwayGroupSelectionPoint{{EffectiveAt: utc(2026, time.July, 22, 21, 0), CommandRevision: 2}}}
	state.RunwayGroups[0].RateSchedule = append(state.RunwayGroups[0].RateSchedule, aman.RunwayGroupRatePoint{EffectiveAt: utc(2026, time.July, 22, 20, 45), ArrivalsPerHour: 24})
	state.RunwayGroups = append(state.RunwayGroups, other)

	model := Build(state, readyHealth())
	require.Equal(t, aman.RunwayGroupID("22"), model.Buckets[0].SelectedRate.RunwayGroupID)
	require.EqualValues(t, 20, model.Buckets[0].SelectedRate.ArrivalsPerHour)
	require.EqualValues(t, 24, model.Buckets[1].SelectedRate.ArrivalsPerHour)
	require.Equal(t, aman.RunwayGroupID("04"), model.Buckets[2].SelectedRate.RunwayGroupID)
	require.EqualValues(t, 30, model.Buckets[2].SelectedRate.ArrivalsPerHour)
}

func TestBuildUsesEveryScheduledQuarterToCalculateWindowCapacity(t *testing.T) {
	now := utc(2026, time.July, 22, 20, 30)
	state := baseState(now, 20)
	state.RunwayGroups[0].RateSchedule = append(state.RunwayGroups[0].RateSchedule, aman.RunwayGroupRatePoint{EffectiveAt: utc(2026, time.July, 22, 21, 0), ArrivalsPerHour: 40})
	for index := 0; index < 33; index++ {
		state.Flights = append(state.Flights, plannedID(index, now.Add(time.Duration(index%4)*15*time.Minute+time.Minute)))
	}

	model := Build(state, readyHealth())
	require.False(t, model.Buckets[1].WindowHigh, "33 is equal to 110% of the mixed-rate capacity of 30")
	state.Flights = append(state.Flights, plannedID(50, now.Add(time.Minute)))
	model = Build(state, readyHealth())
	require.True(t, model.Buckets[1].WindowHigh)
}

func TestBuildWindowIncludesPrecedingBucketOutsidePublishedRange(t *testing.T) {
	now := utc(2026, time.July, 22, 20, 44)
	state := baseState(now, 4)
	state.Flights = []aman.AMANFlight{plannedID(0, utc(2026, time.July, 22, 20, 31))}
	for index := 1; index <= 4; index++ {
		state.Flights = append(state.Flights, plannedID(index, utc(2026, time.July, 22, 20, 29-index)))
	}
	model := Build(state, readyHealth())
	require.Equal(t, 1, model.Buckets[0].Count)
	require.False(t, model.Buckets[0].BucketHigh)
	require.True(t, model.Buckets[0].WindowHigh)
	require.Equal(t, AlertYellow, model.Buckets[0].Alert)
}

func TestBuildPublishesDisconnectedSourceWithoutFlights(t *testing.T) {
	state := baseState(utc(2026, time.July, 22, 20, 44), 20)
	model := Build(state, aman.ComponentHealth{Status: aman.HealthUnavailable})
	require.Equal(t, aman.DataDisconnected, model.SourceStatus)
	require.Equal(t, StatusDisconnected, model.Status)
	require.Contains(t, model.DegradedReasons, "source_disconnected")
}

func TestDisconnectedAircraftRetainsTimingWithoutDisconnectingHealthySource(t *testing.T) {
	now := utc(2026, time.July, 22, 20, 44)
	state := baseState(now, 20)
	state.Flights = []aman.AMANFlight{airborne("retained", "1", now.Add(time.Minute), aman.StateStable, aman.DataDisconnected)}
	model := Build(state, readyHealth())
	require.Equal(t, aman.DataFresh, model.SourceStatus)
	require.Equal(t, StatusDegraded, model.Status)
	require.Contains(t, model.DegradedReasons, "disconnected_flight_data")
	require.NotContains(t, model.DegradedReasons, "source_disconnected")
	require.Equal(t, 1, model.Buckets[1].Count)
	require.Equal(t, aman.DataDisconnected, model.Buckets[1].Flights[0].DataStatus)
}

func TestBuildDeduplicatesSameCallsign(t *testing.T) {
	now := utc(2026, time.July, 22, 20, 44)
	state := baseState(now, 20)
	first := airborne("session-10-flight", "", now.Add(20*time.Minute), aman.StateAirborne, aman.DataFresh)
	second := airborne("session-11-flight", "", now.Add(35*time.Minute), aman.StateAirborne, aman.DataFresh)
	first.Callsign, second.Callsign = "SAS42", "SAS42"
	state.Flights = []aman.AMANFlight{first, second}

	model := Build(state, readyHealth())
	count := 0
	for _, bucket := range model.Buckets {
		count += bucket.Count
	}
	require.Equal(t, 1, count)
}

func baseState(now time.Time, rate uint32) aman.AirportState {
	effective := now.Add(-time.Hour)
	return aman.AirportState{GeneratedAt: now, RunwayGroups: []aman.RunwayGroupPolicy{{ID: "22", Selected: true, ActiveRatePerHour: rate, RateEffectiveAt: &effective, RateSchedule: []aman.RunwayGroupRatePoint{{EffectiveAt: effective, ArrivalsPerHour: rate}}, SelectionSchedule: []aman.RunwayGroupSelectionPoint{{EffectiveAt: effective, CommandRevision: 1}}}}}
}

func readyHealth() aman.ComponentHealth { return aman.ComponentHealth{Status: aman.HealthReady} }

func planned(id, cid string, at time.Time, status aman.DataStatus) aman.AMANFlight {
	duration := time.Hour
	eobt := at.Add(-duration - predictor.DefaultEXOT)
	return aman.AMANFlight{Callsign: stringsUpper(id), State: aman.StatePlanned, DataStatus: status, LatestObservation: &aman.FlightObservation{PlannedTiming: &aman.PlannedTiming{EstimatedOffBlockTime: &eobt, EstimatedEnrouteTime: &duration}}, UpdatedAt: at.Add(-time.Hour)}
}

func plannedID(index int, at time.Time) aman.AMANFlight {
	return planned(fmt.Sprintf("TEST%03d", index), "", at, aman.DataFresh)
}

func airborne(id, cid string, at time.Time, state aman.FlightState, status aman.DataStatus) aman.AMANFlight {
	return aman.AMANFlight{Callsign: stringsUpper(id), State: state, DataStatus: status, Prediction: &aman.Prediction{OperationalTETA: at, Publishable: true}, UpdatedAt: at}
}

func callsigns(bucket Bucket) []string {
	values := make([]string, len(bucket.Flights))
	for index, flight := range bucket.Flights {
		values[index] = flight.Callsign
	}
	return values
}
func utc(year int, month time.Month, day, hour, minute int) time.Time {
	return time.Date(year, month, day, hour, minute, 0, 0, time.UTC)
}
func timePtr(value time.Time) *time.Time { return &value }
func stringsUpper(value string) string   { return strings.ToUpper(value) }

func TestBuildRetainsCommittedAssignmentsWhenPredictionIsUnavailable(t *testing.T) {
	now := utc(2026, time.October, 4, 12, 0)
	for _, status := range []aman.DataStatus{aman.DataFresh, aman.DataStale, aman.DataDisconnected} {
		for _, kind := range []string{"missing", "unpublishable", "retained"} {
			t.Run(string(status)+"/"+kind, func(t *testing.T) {
				state := baseState(now, 40)
				at := now.Add(20 * time.Minute)
				flight := aman.AMANFlight{Callsign: "AFR15", State: aman.StateAirborne, DataStatus: status, Slot: &aman.Slot{Time: at}}
				if kind != "missing" {
					flight.Prediction = &aman.Prediction{OperationalTETA: now.Add(35 * time.Minute), Publishable: kind == "retained"}
				}
				state.Flights = []aman.AMANFlight{flight}
				model := Build(state, readyHealth())
				require.NotContains(t, model.DegradedReasons, "missing_timing:AFR15")
				index := 1
				if kind == "retained" {
					index = 2
					at = flight.Prediction.OperationalTETA
				}
				require.Len(t, model.Buckets[index].Flights, 1)
				got := model.Buckets[index].Flights[0]
				require.Equal(t, at, got.LandingAt)
				require.Equal(t, SourceAMAN, got.TimingSource)
				require.Equal(t, status, got.DataStatus)
			})
		}
	}
}

func TestBuildDoesNotReportPastCommittedSlotAsMissingTiming(t *testing.T) {
	now := utc(2026, time.October, 4, 12, 0)
	state := baseState(now, 40)
	state.Flights = []aman.AMANFlight{{Callsign: "AFR15", State: aman.StateStable, DataStatus: aman.DataDisconnected, Slot: &aman.Slot{Time: now.Add(-time.Hour)}}}
	model := Build(state, readyHealth())
	require.NotContains(t, model.DegradedReasons, "missing_timing:AFR15")
	for _, bucket := range model.Buckets {
		require.Empty(t, bucket.Flights)
	}
}
