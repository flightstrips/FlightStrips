package euroscope_test

import (
	"FlightStrips/internal/aman"
	"FlightStrips/pkg/events/euroscope"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestAMANGainLossGoldenFixture(t *testing.T) {
	now := time.Date(2026, time.September, 8, 12, 0, 0, 0, time.UTC)
	slot := aman.Slot{Time: now.Add(10 * time.Minute), RunwayGroupID: "22", Sequence: 1, Revision: 42, Reason: "sequence"}
	starFamily, feederFix, holdingFix := "TESPI", "TNO", "ROSBI"
	feederETA := now.Add(5 * time.Minute)
	state := aman.AirportState{
		Airport: "EKCH", Revision: 42, GeneratedAt: now, PolicyVersion: "test", Mode: aman.ModeAuthoritative,
		Authoritative: true,
		Flights: []aman.AMANFlight{
			{ID: "flight-1", VATSIMCID: "1", CurrentCallsign: "SAS123", State: aman.StateStable, DataStatus: aman.DataFresh, FreezeReason: aman.FreezeNone, Slot: &slot,
				SelectedSTARFamily: &starFamily, SelectedFeederFix: &feederFix, SelectedHolding: &holdingFix,
				FeederETA:  &aman.FeederETAState{ETA: &feederETA, Source: aman.FeederETASourceHolding},
				Prediction: &aman.Prediction{OperationalTETA: slot.Time.Add(90 * time.Second), Publishable: true, Calculation: &aman.PredictionCalculation{Legs: []aman.PredictionLeg{{To: "ILS-22L-RUNWAY"}}}}},
			{ID: "flight-2", VATSIMCID: "2", CurrentCallsign: "DAT456", State: aman.StateUnstable, DataStatus: aman.DataStale, FreezeReason: aman.FreezeNone},
			{ID: "flight-3", VATSIMCID: "3", CurrentCallsign: "SAS789", State: aman.StateStable, DataStatus: aman.DataFresh, FreezeReason: aman.FreezeNone,
				SelectedSTARFamily: &starFamily, SelectedFeederFix: &feederFix,
				FeederETA: &aman.FeederETAState{Source: aman.FeederETASourcePassed, Passed: true}},
		},
	}
	event, err := euroscope.NewAMANGainLossEvent(state)
	require.NoError(t, err)
	require.Equal(t, "ILS-22L-RUNWAY", *event.Values[0].ReferencePoint)
	targetTime, err := time.Parse(time.RFC3339Nano, *event.Values[0].TargetTime)
	require.NoError(t, err)
	predictedTime, err := time.Parse(time.RFC3339Nano, *event.Values[0].PredictedTime)
	require.NoError(t, err)
	require.Equal(t, *event.Values[0].GainLossSeconds, int64(predictedTime.Sub(targetTime)/time.Second))
	payload, err := event.Marshal()
	require.NoError(t, err)

	eventType, inner, err := euroscope.UnmarshalEnvelope(payload)
	require.NoError(t, err)
	require.Equal(t, euroscope.AMANGainLoss, eventType)
	var decoded euroscope.AMANGainLossEvent
	require.NoError(t, proto.Unmarshal(inner, &decoded))
	require.EqualValues(t, 1, decoded.Version)
	require.EqualValues(t, 42, decoded.Revision)
	require.True(t, decoded.Authoritative)
	require.Len(t, decoded.Values, 3)
	require.Equal(t, "flight-1", decoded.Values[0].FlightId)
	require.Equal(t, "TESPI", decoded.Values[0].GetStarFamily())
	require.Equal(t, "TNO", decoded.Values[0].GetFeederFix())
	require.Equal(t, "ROSBI", decoded.Values[0].GetHoldingFix())
	require.Equal(t, "2026-09-08T12:05:00.000Z", decoded.Values[0].GetFeederFixEta())
	require.Equal(t, "holding", decoded.Values[0].GetFeederFixEtaSource())
	require.False(t, decoded.Values[0].GetFeederFixPassed())
	require.Nil(t, decoded.Values[1].StarFamily, "old payloads omit additive identity fields")
	require.Nil(t, decoded.Values[1].FeederFix, "old payloads omit additive identity fields")
	require.Nil(t, decoded.Values[1].HoldingFix, "old payloads omit additive identity fields")
	require.Nil(t, decoded.Values[1].FeederFixEta, "old payloads omit additive feeder timing fields")
	require.Nil(t, decoded.Values[1].FeederFixEtaSource, "old payloads omit additive feeder timing fields")
	require.Nil(t, decoded.Values[1].FeederFixPassed, "old payloads omit additive feeder timing fields")
	require.Nil(t, decoded.Values[2].FeederFixEta, "passed state must not invent an ETA")
	require.Equal(t, "passed", decoded.Values[2].GetFeederFixEtaSource())
	require.True(t, decoded.Values[2].GetFeederFixPassed())
}

func TestAMANGainLossIsAlwaysAuthoritativeForEuroScope(t *testing.T) {
	event, err := euroscope.NewAMANGainLossEvent(aman.AirportState{
		Airport: "EKCH", GeneratedAt: time.Date(2026, time.September, 8, 12, 0, 0, 0, time.UTC),
		Authoritative: false,
	})

	require.NoError(t, err)
	require.True(t, event.Authoritative)
}

func TestAMANGainLossExcludesRemovedIdentitiesFromReplacement(t *testing.T) {
	now := time.Date(2026, time.September, 14, 18, 0, 0, 0, time.UTC)
	live := aman.AMANFlight{
		ID: "live", CurrentCallsign: "SAS123", State: aman.StateStable, DataStatus: aman.DataFresh,
		Slot: &aman.Slot{Time: now.Add(10 * time.Minute)},
		Prediction: &aman.Prediction{
			OperationalTETA: now.Add(11 * time.Minute), Publishable: true,
			Calculation: &aman.PredictionCalculation{Legs: []aman.PredictionLeg{{To: "ILS-22L-RUNWAY"}}},
		},
	}
	removed := live
	removed.ID, removed.State, removed.Slot = "retired", aman.StateRemoved, nil
	for _, flights := range [][]aman.AMANFlight{{removed, live}, {live, removed}, {removed}} {
		event, err := euroscope.NewAMANGainLossEvent(aman.AirportState{
			Airport: "EKCH", Revision: 42, GeneratedAt: now, Flights: flights,
		})
		require.NoError(t, err)
		payload, err := event.Marshal()
		require.NoError(t, err)
		_, inner, err := euroscope.UnmarshalEnvelope(payload)
		require.NoError(t, err)
		var decoded euroscope.AMANGainLossEvent
		require.NoError(t, proto.Unmarshal(inner, &decoded))
		if len(flights) == 1 {
			require.Empty(t, decoded.Values, "a complete empty replacement clears retired tags")
			continue
		}
		// EuroScope rejects the entire replacement on duplicate callsigns,
		// even when the retired identity has no gain/loss value.
		require.Len(t, decoded.Values, 1)
		require.Equal(t, "live", decoded.Values[0].FlightId)
		require.EqualValues(t, 60, decoded.Values[0].GetGainLossSeconds())
	}
}

func TestAMANGainLossPublishesOneCurrentRowPerNormalizedCallsign(t *testing.T) {
	now := time.Date(2026, time.September, 14, 18, 0, 0, 0, time.UTC)
	old := aman.AMANFlight{ID: "old", CurrentCallsign: "SAS123", State: aman.StateStable, DataStatus: aman.DataFresh, UpdatedAt: now.Add(-time.Minute)}
	current := old
	current.ID, current.CurrentCallsign, current.UpdatedAt = "current", " sas123 ", now
	other := old
	other.ID, other.CurrentCallsign = "other", "DAT456"
	for _, flights := range [][]aman.AMANFlight{{old, current, other}, {current, old, other}} {
		event, err := euroscope.NewAMANGainLossEvent(aman.AirportState{Airport: "EKCH", GeneratedAt: now, Flights: flights})
		require.NoError(t, err)
		require.Len(t, event.Values, 2)
		require.Equal(t, "SAS123", event.Values[0].Callsign)
		require.Equal(t, "current", event.Values[0].FlightId)
		require.Equal(t, "DAT456", event.Values[1].Callsign)
	}
}
