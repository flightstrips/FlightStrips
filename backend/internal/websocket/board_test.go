package websocket

import (
	"FlightStrips/pkg/events"
	fe "FlightStrips/pkg/events/frontend"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestBoardTransportContextStaysOutsideStrictActionSchemas(t *testing.T) {
	board := events.BoardContext{SessionID: 1, Airport: "EKCH", Environment: "LIVE", InstanceID: "own", MembershipID: "m", TransportGeneration: 18446744073709551615, Generation: 18446744073709551614, DefinitionRevision: "revision", SubscriptionID: "tab"}
	raw, err := json.Marshal(map[string]any{"type": "aman.set_holding_eat_writeback", "version": 1, "board": board, "data": map[string]any{"command_id": "command", "expected_revision": 1, "enabled": true}})
	require.NoError(t, err)
	message, err := parseMessage[fe.EventType](raw)
	require.NoError(t, err)
	require.Equal(t, board, *message.Board)
	require.NotContains(t, string(message.Message), "board")
	var action fe.AMANSetHoldingEATWritebackMessage
	// The real strict decoder must see only the unchanged business schema.
	require.NoError(t, json.Unmarshal(message.Message, &action))
	require.Contains(t, string(message.Message), "command_id")
}
