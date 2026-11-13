package operational

import (
	"encoding/json"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/sequence"
	"FlightStrips/internal/aman/terminal"
	"github.com/stretchr/testify/require"
)

func TestSequenceSTARProgressUsesFreshRouteEvidence(t *testing.T) {
	start := time.Date(2026, time.October, 5, 12, 0, 0, 0, time.UTC)
	family, feeder := "MONAK", "FEEDER"
	flight := aman.AMANFlight{
		DataStatus: aman.DataFresh, SelectedSTARFamily: &family, SelectedFeederFix: &feeder,
		RouteProgress:    &aman.RouteProgress{TerminalDigest: "terminal"},
		Prediction:       &aman.Prediction{Publishable: true},
		DerivedFeederETA: &aman.FeederETAState{Source: aman.FeederETASourceRoute, ETA: &start},
		FeederETA:        &aman.FeederETAState{Source: aman.FeederETASourceManual, ETA: &start},
	}
	progress := sequenceSTARProgress(flight)
	require.NotNil(t, progress)
	require.Equal(t, feeder, progress.Fix)
	require.Equal(t, start, *progress.ETA)
	require.NotSame(t, flight.DerivedFeederETA.ETA, progress.ETA)

	flight.DerivedFeederETA = nil
	require.Nil(t, sequenceSTARProgress(flight), "a manual feeder ETA is not physical order evidence")
	flight.FeederETA = &aman.FeederETAState{Source: aman.FeederETASourceHolding, ETA: &start}
	require.Nil(t, sequenceSTARProgress(flight), "a holding release is not free-flight order evidence")
	flight.FeederETA = &aman.FeederETAState{Source: aman.FeederETASourceRoute, ETA: &start}
	flight.Prediction.Publishable = false
	require.Nil(t, sequenceSTARProgress(flight))
	flight.Prediction.Publishable = true
	flight.DataStatus = aman.DataStale
	require.Nil(t, sequenceSTARProgress(flight))
}

func TestHoldingDerivedFeederTimingCannotAuthorizeSameSTARInsertionAheadOfStableArrival(t *testing.T) {
	now := time.Date(2026, time.November, 13, 18, 0, 0, 0, time.UTC)
	group := aman.RunwayGroupID("ARRIVAL-22L")
	family, feeder := "TESPI", "ROSBI"
	first := gapCommandFlight("SAS1778", group, now.Add(23*time.Minute), 1, aman.StateStable, aman.FreezeNone)
	second := gapCommandFlight("SAS46R", group, now.Add(26*time.Minute), 2, aman.StateUnstable, aman.FreezeNone)
	for _, f := range []*aman.AMANFlight{&first, &second} {
		f.SelectedSTARFamily, f.SelectedFeederFix = &family, &feeder
		f.RouteProgress = &aman.RouteProgress{TerminalDigest: "terminal"}
		eta := f.Slot.Time.Add(-5 * time.Minute)
		f.FeederETA = &aman.FeederETAState{Source: aman.FeederETASourceHolding, ETA: &eta}
		f.DerivedFeederETA = f.FeederETA
	}
	first.Prediction.OperationalTETA, first.Prediction.RawTETA = now.Add(17*time.Minute), now.Add(17*time.Minute)
	first.Prediction.DegradationReason = stringPointer("basic_terminal_estimate:degraded_or_incomplete_geometry: sector route fix missing or ambiguous: ODEVA")
	second.Prediction.OperationalTETA, second.Prediction.RawTETA = now.Add(19*time.Minute), now.Add(19*time.Minute)
	state := aman.AirportState{Revision: 39, Flights: []aman.AMANFlight{second, first}, RunwayGroups: []aman.RunwayGroupPolicy{{
		ID: group, ActiveRatePerHour: 40, RateSchedule: []aman.RunwayGroupRatePoint{{EffectiveAt: now, ArrivalsPerHour: 40}},
	}}}
	input := sequenceInput(state, terminal.Configuration{})
	for _, f := range input.Flights {
		require.Nil(t, f.STARProgress, "holding-derived clocks do not establish physical order")
		require.Nil(t, f.HoldingQueueTime, "a prediction is not an actual holding queue")
	}
	got, err := sequence.Generate(input)
	require.NoError(t, err)
	require.False(t, got.HasConflicts())
	require.Equal(t, aman.Callsign("SAS1778"), got.Entries[0].Callsign)
	require.Equal(t, first.Slot.Time, got.Entries[0].Time)
	require.Equal(t, aman.Callsign("SAS46R"), got.Entries[1].Callsign)
	require.False(t, got.Entries[1].Time.Before(first.Slot.Time.Add(90*time.Second)))
	compacted, _, err := sequence.GenerateWithVacancyPromotions(input, nil, now)
	require.NoError(t, err)
	require.Equal(t, aman.Callsign("SAS1778"), compacted.Entries[0].Callsign)
	require.Equal(t, aman.Callsign("SAS46R"), compacted.Entries[1].Callsign)
	require.Equal(t, aman.FeederETASourceHolding, first.DerivedFeederETA.Source)
}

func TestSTAROrderAuditRecordsMissingEvidenceForInitialInsertion(t *testing.T) {
	now := time.Date(2026, time.November, 13, 18, 0, 0, 0, time.UTC)
	input := sequence.Input{Flights: []sequence.Flight{
		{Callsign: "SAS1778", RunwayGroupID: "A", State: aman.StateAirborne, FreezeReason: aman.FreezeNone, SelectedSTARFamily: "TESPI", OperationalTETA: now, WakeCategory: "M"},
		{Callsign: "SAS46R", RunwayGroupID: "A", State: aman.StateAirborne, FreezeReason: aman.FreezeNone, SelectedSTARFamily: "TESPI", OperationalTETA: now.Add(time.Minute), WakeCategory: "M"},
	}, Policies: []sequence.Policy{{RunwayGroupID: "A", Rates: []sequence.RatePoint{{EffectiveAt: now, ArrivalsPerHour: 40}}, SeparationRules: sequence.CPHSeparationRules(), UnknownSeparation: 3 * time.Minute}}}
	result, err := sequence.Generate(input)
	require.NoError(t, err)
	snapshots := sequence.STAROrderDiagnostics(input, result)
	require.Len(t, snapshots, 1)
	require.False(t, snapshots[0].Comparable)
	require.Nil(t, snapshots[0].PreviousOrder)
	require.True(t, snapshots[0].CurrentOrder)
	records := vacancyPromotionAuditEntries(sequenceChanges{STAROrder: snapshots})
	require.Len(t, records, 1)
	require.Equal(t, "aman.star_order_evaluated", records[0].Category)
	var payload struct {
		Pairs []sequence.STAROrderSnapshot `json:"pairs"`
	}
	require.NoError(t, json.Unmarshal(records[0].Payload, &payload))
	require.Equal(t, snapshots, payload.Pairs)
	require.Contains(t, string(records[0].Payload), `"Progress":null`)
}

func TestSequenceSTARProgressAfterPassingFeeder(t *testing.T) {
	family, feeder := "MONAK", "FEEDER"
	flight := aman.AMANFlight{
		DataStatus: aman.DataFresh, SelectedSTARFamily: &family, SelectedFeederFix: &feeder,
		RouteProgress:    &aman.RouteProgress{TerminalDigest: "terminal"},
		DerivedFeederETA: &aman.FeederETAState{Source: aman.FeederETASourcePassed, Passed: true},
		Prediction: &aman.Prediction{Publishable: true, Calculation: &aman.PredictionCalculation{Legs: []aman.PredictionLeg{
			{From: "FEEDER", To: "FINAL", DistanceNM: 20, EndLatitude: 55, EndLongitude: 12},
			{From: "FINAL", To: "RWY", DistanceNM: 5, EndLatitude: 55.1, EndLongitude: 12.1},
		}}},
	}
	progress := sequenceSTARProgress(flight)
	require.NotNil(t, progress)
	require.True(t, progress.Passed)
	require.Nil(t, progress.ETA)
	require.Equal(t, float64(25), progress.DistanceToGoNM)
	require.Equal(t, []string{"FEEDER>FINAL:55:12", "FINAL>RWY:55.1:12.1"}, progress.RemainingFixes)
	flight.Prediction.Calculation = nil
	require.Nil(t, sequenceSTARProgress(flight), "passed progress needs a comparable remaining route")
}
