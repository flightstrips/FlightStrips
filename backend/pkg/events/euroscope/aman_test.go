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
	state := aman.AirportState{
		Airport: "EKCH", Revision: 42, GeneratedAt: now, PolicyVersion: "test", Mode: aman.ModeAuthoritative,
		Authoritative: true,
		Flights: []aman.AMANFlight{
			{ID: "flight-1", VATSIMCID: "1", CurrentCallsign: "SAS123", State: aman.StateStable, DataStatus: aman.DataFresh, FreezeReason: aman.FreezeNone, Slot: &slot,
				Prediction: &aman.Prediction{OperationalTETA: slot.Time.Add(90 * time.Second), Publishable: true, Calculation: &aman.PredictionCalculation{Legs: []aman.PredictionLeg{{To: "ILS-22L-RUNWAY"}}}}},
			{ID: "flight-2", VATSIMCID: "2", CurrentCallsign: "DAT456", State: aman.StateUnstable, DataStatus: aman.DataStale, FreezeReason: aman.FreezeNone},
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
	require.Len(t, decoded.Values, 2)
	require.Equal(t, "flight-1", decoded.Values[0].FlightId)
}
