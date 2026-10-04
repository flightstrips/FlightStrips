package operational

import (
	"context"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/sequence"
	"FlightStrips/internal/models"
	"github.com/stretchr/testify/require"
)

func TestEuroScopeAircraftFactsReachBothObserversAndMerge(t *testing.T) {
	now := time.Date(2026, 10, 4, 8, 0, 0, 0, time.UTC)
	for _, test := range []struct{ item, aircraft, wake string }{
		{"A320/M-SDE3FGHIRWY/LB1", "A320", "M"},
		{"b772/h-sde2/lb1", "B772", "H"},
		{"C172/L-S/C", "C172", "L"},
		{"A388/J-S/LB1", "A388", "J"},
		{"A320", "A320", ""},
		{"A320/X-S/C", "A320", ""},
	} {
		t.Run(test.item, func(t *testing.T) {
			sink := &euroScopeObservationSink{}
			strip := &models.Strip{Session: 1, Callsign: "SAS123", Origin: "ESSA", Destination: "EKCH", AircraftType: &test.item}
			strips, err := NewEuroScopeStripObserver(EuroScopeStripObserverDependencies{Sink: sink, EnabledAirports: []string{"EKCH"}, Now: func() time.Time { return now }})
			require.NoError(t, err)
			require.NoError(t, strips.ObserveEuroScopeStrip(context.Background(), strip))
			positions, err := NewEuroScopePositionObserver(EuroScopePositionObserverDependencies{Sink: sink, EnabledAirports: []string{"EKCH"}, Now: func() time.Time { return now }})
			require.NoError(t, err)
			require.NoError(t, positions.ObserveEuroScopePosition(context.Background(), 1, strip, 55, 12, 9000))
			for _, observation := range sink.observations {
				require.Equal(t, test.aircraft, stringValue(observation.AircraftType))
				require.Equal(t, test.wake, stringValue(observation.WakeCategory))
				merged := mergeSurveillanceObservation(aman.FlightObservation{}, observation)
				require.Equal(t, test.wake, stringValue(merged.WakeCategory))
			}
		})
	}
}

func TestUnclearedRouteHoldingDoesNotPropagateOutlierSlots(t *testing.T) {
	now := time.Date(2026, 10, 4, 8, 0, 0, 0, time.UTC)
	hold := "LUGAS"
	state := aman.AirportState{RunwayGroups: []aman.RunwayGroupPolicy{{ID: "22L", RateSchedule: []aman.RunwayGroupRatePoint{{EffectiveAt: now, ArrivalsPerHour: 40}}}}}
	group := aman.RunwayGroupID("22L")
	// The first route ETA is corrupt but reaches the optional holding before
	// the normal flight. Neither aircraft has been cleared to hold.
	for i, callsign := range []string{"OUTLIER", "NORMAL"} {
		landing := now.Add(time.Duration(16-i*15) * time.Hour)
		entry := now.Add(time.Duration(i+1) * time.Minute)
		state.Flights = append(state.Flights, aman.AMANFlight{Callsign: callsign, State: aman.StateAirborne, FreezeReason: aman.FreezeNone, DataStatus: aman.DataFresh, SelectedRunwayGroup: &group, SelectedHolding: &hold, Prediction: &aman.Prediction{Publishable: true, RawTETA: landing, OperationalTETA: landing, HoldingFixETA: &entry}, LatestObservation: &aman.FlightObservation{WakeCategory: stringPointer("M")}})
	}
	service := &Service{}
	input := service.sequenceInput(state)
	result, err := sequence.Generate(input)
	require.NoError(t, err)
	require.Equal(t, "NORMAL", result.Entries[0].Callsign)
	require.Equal(t, now.Add(time.Hour), result.Entries[0].Time)
	flight := state.Flights[0]
	flight.HoldingClearance = &aman.HoldingClearance{HoldType: aman.HoldingClearanceEnroute, Hold: "LUGAS"}
	require.NotNil(t, holdingQueueTime(flight, []aman.AMANFlight{flight}), "explicit holding still enforces the queue")
}

func TestEuroScopeMediumArrivalsUseFortyPerHourGrid(t *testing.T) {
	now := time.Date(2026, 10, 4, 8, 0, 0, 0, time.UTC)
	group := aman.RunwayGroupID("22L")
	aircraft, wake := euroScopeAircraft(stringPointer("A320/M-SDE3/LB1"))
	state := aman.AirportState{RunwayGroups: []aman.RunwayGroupPolicy{{ID: group, RateSchedule: []aman.RunwayGroupRatePoint{{EffectiveAt: now, ArrivalsPerHour: 40}}}}}
	for _, callsign := range []string{"FIRST", "SECOND"} {
		state.Flights = append(state.Flights, aman.AMANFlight{Callsign: callsign, State: aman.StateAirborne, FreezeReason: aman.FreezeNone, DataStatus: aman.DataFresh, SelectedRunwayGroup: &group, Prediction: &aman.Prediction{Publishable: true, RawTETA: now, OperationalTETA: now}, LatestObservation: &aman.FlightObservation{AircraftType: aircraft, WakeCategory: wake}})
	}
	result, err := sequence.Generate((&Service{}).sequenceInput(state))
	require.NoError(t, err)
	require.Empty(t, result.Warnings)
	require.Equal(t, 90*time.Second, result.Entries[1].Time.Sub(result.Entries[0].Time))
}

func TestStableSlotProtectionDoesNotDependOnPredictionDrift(t *testing.T) {
	now := time.Date(2026, 10, 4, 8, 0, 0, 0, time.UTC)
	group := aman.RunwayGroupID("22L")
	for _, drift := range []time.Duration{2 * time.Minute, 2*time.Minute + time.Second, 10 * time.Minute, 30 * time.Minute} {
		state := aman.AirportState{Flights: []aman.AMANFlight{{Callsign: "SAS123", State: aman.StateStable, FreezeReason: aman.FreezeNone, SelectedRunwayGroup: &group,
			Slot: &aman.Slot{Time: now, RunwayGroupID: group, Sequence: 1}, Prediction: &aman.Prediction{Publishable: true, OperationalTETA: now.Add(drift)}}}}
		input := (&Service{}).sequenceInput(state)
		require.Len(t, input.Flights, 1)
		require.True(t, input.Flights[0].ProtectCurrentSlot)
	}
}
