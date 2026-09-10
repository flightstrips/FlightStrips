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

func TestMarshalEnvelopeRejectsMismatchedOneofField(t *testing.T) {
	_, err := MarshalEnvelope(&LoginEvent{}, Authentication)
	require.ErrorContains(t, err, "does not match envelope field")
}

func TestUnmarshalEnvelopeRejectsMissingEvent(t *testing.T) {
	_, _, err := UnmarshalEnvelope(nil)
	require.ErrorContains(t, err, "contains no event")
}
