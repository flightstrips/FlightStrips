package trafficprediction

import (
	"strings"
	"testing"
	"time"

	"FlightStrips/internal/aman"
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
	api.CurrentCallsign = "SAS42"
	amanFlight := airborne("aman", "42", now.Add(35*time.Minute), aman.StateStable, aman.DataFresh)
	amanFlight.CurrentCallsign = "SAS42"
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
	api.CurrentCallsign = "SAS42"
	amanFlight := airborne("aman", "42", time.Time{}, aman.StateStable, aman.DataFresh)
	amanFlight.CurrentCallsign = "SAS42"
	amanFlight.Prediction = nil
	state.Flights = []aman.AMANFlight{api, amanFlight}

	model := Build(state, readyHealth())
	require.Contains(t, model.DegradedReasons, "missing_timing:SAS42")
	for _, bucket := range model.Buckets {
		require.Zero(t, bucket.Count, "the duplicate API prediction must remain suppressed")
	}
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
	state.Flights = []aman.AMANFlight{planned("stale", "1", now.Add(time.Minute), aman.DataStale), {ID: "unknown", CurrentCallsign: "UNKNOWN", State: aman.StatePlanned, DataStatus: aman.DataDisconnected}}

	model := Build(state, readyHealth())
	require.Equal(t, StatusDisconnected, model.Status)
	require.Contains(t, model.DegradedReasons, "stale_flight_data")
	require.Contains(t, model.DegradedReasons, "vatsim_disconnected")
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
	require.Contains(t, model.DegradedReasons, "vatsim_disconnected")
}

func baseState(now time.Time, rate uint32) aman.AirportState {
	effective := now.Add(-time.Hour)
	return aman.AirportState{GeneratedAt: now, RunwayGroups: []aman.RunwayGroupPolicy{{ID: "22", Selected: true, ActiveRatePerHour: rate, RateEffectiveAt: &effective, RateSchedule: []aman.RunwayGroupRatePoint{{EffectiveAt: effective, ArrivalsPerHour: rate}}, SelectionSchedule: []aman.RunwayGroupSelectionPoint{{EffectiveAt: effective, CommandRevision: 1}}}}}
}

func readyHealth() aman.ComponentHealth { return aman.ComponentHealth{Status: aman.HealthReady} }

func planned(id, cid string, at time.Time, status aman.DataStatus) aman.AMANFlight {
	duration := time.Hour
	eobt := at.Add(-duration)
	return aman.AMANFlight{ID: aman.FlightID(id), VATSIMCID: cid, CurrentCallsign: stringsUpper(id), State: aman.StatePlanned, DataStatus: status, LatestObservation: &aman.FlightObservation{PlannedTiming: &aman.PlannedTiming{EstimatedOffBlockTime: &eobt, EstimatedEnrouteTime: &duration}}, UpdatedAt: at.Add(-time.Hour)}
}

func plannedID(index int, at time.Time) aman.AMANFlight {
	return planned(string(rune('A'+index)), string(rune('a'+index)), at, aman.DataFresh)
}

func airborne(id, cid string, at time.Time, state aman.FlightState, status aman.DataStatus) aman.AMANFlight {
	return aman.AMANFlight{ID: aman.FlightID(id), VATSIMCID: cid, CurrentCallsign: stringsUpper(id), State: state, DataStatus: status, Prediction: &aman.Prediction{OperationalTETA: at, Publishable: true}, UpdatedAt: at}
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
