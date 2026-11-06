package frontend

import (
	events "FlightStrips/pkg/events/frontend"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSessionCleanupKeepsOtherAirportAndEnvironmentPartitions(t *testing.T) {
	hub := &Hub{messages: map[int32][]events.MessageReceivedEvent{}, metarCache: map[int32]string{}, arrAtisCodeCache: map[int32]string{}, depAtisCodeCache: map[int32]string{}, clxOverrides: map[int32]map[string]bool{}}
	// Different airport LIVE and same airport SWEATBOX are both independent.
	for _, session := range []int32{1, 2, 3} {
		hub.storeMessage(session, events.MessageReceivedEvent{})
		hub.metarCache[session] = "weather"
		hub.arrAtisCodeCache[session] = "A"
		hub.depAtisCodeCache[session] = "B"
		hub.setClxOverride(session, "SAS101")
	}
	hub.RemoveSession(1)
	hub.RemoveSession(1)
	// Delayed session work cannot resurrect an expired partition.
	hub.storeMessage(1, events.MessageReceivedEvent{})
	hub.SendAtisUpdate(1, "late", "X", "Y")
	hub.setClxOverride(1, "late")
	require.NotContains(t, hub.messages, int32(1))
	require.NotContains(t, hub.metarCache, int32(1))
	require.NotContains(t, hub.clxOverrides, int32(1))
	for _, session := range []int32{2, 3} {
		require.Len(t, hub.messages[session], 1)
		require.Equal(t, "weather", hub.metarCache[session])
		require.Equal(t, "A", hub.arrAtisCodeCache[session])
		require.True(t, hub.clxOverrides[session]["SAS101"])
	}
}
