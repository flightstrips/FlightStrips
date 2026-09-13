package euroscope

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMarshalEnvelopeUsesOneofFieldAsEventDiscriminator(t *testing.T) {
	encoded, err := MarshalEnvelope(&LoginEvent{
		Airport:  "EKCH",
		Callsign: "EKCH_TWR",
		Range:    100,
	}, Login)
	require.NoError(t, err)

	eventType, payload, err := UnmarshalEnvelope(encoded)
	require.NoError(t, err)
	require.Equal(t, Login, eventType)

	var login LoginEvent
	require.NoError(t, UnmarshalEvent(encoded, Login, &login))
	require.Equal(t, "EKCH", login.Airport)
	require.Equal(t, "EKCH_TWR", login.Callsign)
	require.EqualValues(t, 100, login.Range)
	require.NotEmpty(t, payload)
}

func TestOutgoingMessageMarshalWrapsPayloadInEnvelope(t *testing.T) {
	encoded, err := (SessionInfoEvent{Role: SessionInfoMaster}).Marshal()
	require.NoError(t, err)

	eventType, _, err := UnmarshalEnvelope(encoded)
	require.NoError(t, err)
	require.Equal(t, SessionInfo, eventType)
}

func TestHoldEventCanBeSentToEuroScope(t *testing.T) {
	encoded, err := (HoldEvent{Callsign: "SAS123", Hold: "OLPIB", HoldType: "enroute", HoldEat: "1422"}).Marshal()
	require.NoError(t, err)

	var hold HoldEvent
	require.NoError(t, UnmarshalEvent(encoded, Hold, &hold))
	require.Equal(t, "SAS123", hold.Callsign)
	require.Equal(t, "OLPIB", hold.Hold)
	require.Equal(t, "1422", hold.HoldEat)
}

func TestMarshalEnvelopeRejectsMismatchedOneofField(t *testing.T) {
	_, err := MarshalEnvelope(&LoginEvent{}, Authentication)
	require.ErrorContains(t, err, "does not match envelope field")
}

func TestUnmarshalEnvelopeRejectsMissingEvent(t *testing.T) {
	_, _, err := UnmarshalEnvelope(nil)
	require.ErrorContains(t, err, "contains no event")
}
