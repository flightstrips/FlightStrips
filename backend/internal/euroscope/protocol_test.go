package euroscope

import (
	"testing"

	"FlightStrips/pkg/events/euroscope"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

func TestDecodeAuthenticationRequiresRevisionTwo(t *testing.T) {
	hub := &Hub{}
	for _, revision := range []uint32{0, 1, 3} {
		frame, err := euroscope.MarshalEnvelope(&euroscope.TokenEvent{Token: "token", ProtocolRevision: revision}, euroscope.Authentication)
		require.NoError(t, err)
		_, err = hub.DecodeAuthentication(websocket.BinaryMessage, frame)
		require.ErrorContains(t, err, "unsupported EuroScope protocol revision")
	}
	frame, err := euroscope.MarshalEnvelope(&euroscope.TokenEvent{Token: "token", ProtocolRevision: 2}, euroscope.Authentication)
	require.NoError(t, err)
	_, err = hub.DecodeAuthentication(websocket.TextMessage, frame)
	require.ErrorContains(t, err, "binary websocket frame")
	auth, err := hub.DecodeAuthentication(websocket.BinaryMessage, frame)
	require.NoError(t, err)
	require.Equal(t, "token", auth.Token)
}
