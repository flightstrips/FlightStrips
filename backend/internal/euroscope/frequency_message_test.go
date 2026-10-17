package euroscope

import (
	"FlightStrips/internal/shared"
	"FlightStrips/pkg/events"
	wire "FlightStrips/pkg/events/euroscope"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestFrequencyMessagesPinControllerRatherThanMaster(t *testing.T) {
	makeClient := func(cid, position string) *Client {
		c := &Client{session: 42, position: position, user: shared.NewAuthenticatedUser(cid, 0, nil), send: make(chan events.OutgoingMessage, 2), closed: make(chan struct{})}
		c.frequencyMessages.Store(true)
		return c
	}
	owner := makeClient("123", "121.905")
	other := makeClient("456", "118.105")
	hub := &Hub{clients: map[*Client]bool{owner: true, other: true}}
	require.Nil(t, hub.FrequencyMessageSender(41, "123", "121.905"))
	require.Nil(t, hub.FrequencyMessageSender(42, "456", "121.905"))
	owner.frequencyMessages.Store(false)
	require.Nil(t, hub.FrequencyMessageSender(42, "123", "121.905"))
	owner.frequencyMessages.Store(true)
	owner.observer = true
	require.Nil(t, hub.FrequencyMessageSender(42, "123", "121.905"))
	owner.observer = false
	send := hub.FrequencyMessageSender(42, "123", "121.905")
	require.NotNil(t, send)
	require.True(t, send(wire.SendFrequencyMessageEvent{RequestId: "id", Callsign: "SAS123", Message: "Hold position."}))
	require.Len(t, owner.send, 1)
	require.Empty(t, other.send)
	owner.updateIdentity("118.105", "EKCH_TWR", false, "")
	require.False(t, send(wire.SendFrequencyMessageEvent{}))
	owner.updateIdentity("121.905", "EKCH_GND", false, "")
	close(owner.closed)
	// Enqueue uses a select; a closed connection must never be eligible for a new send.
	require.Nil(t, hub.FrequencyMessageSender(42, "123", "121.905"))
}
