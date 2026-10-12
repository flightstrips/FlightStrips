package frontend

import (
	"FlightStrips/internal/config"
	"FlightStrips/internal/shared"
	frontendEvents "FlightStrips/pkg/events/frontend"
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSendMessagePreservesFormatAndSnapshot(t *testing.T) {
	for _, format := range []string{"", "AUTO", "SMALL", "LARGE"} {
		t.Run(format, func(t *testing.T) {
			hub := &Hub{clients: map[*Client]bool{}, messages: map[int32][]frontendEvents.MessageReceivedEvent{}, send: make(chan internalMessage, 1)}
			client := startQueuedTestClient(&Client{hub: hub, session: 42, position: "119.905", user: shared.NewAuthenticatedUser("123", 0, nil)})
			hub.clients[client] = true
			data, err := json.Marshal(frontendEvents.SendMessageEvent{Text: "TEST MESSAGE", Format: format})
			require.NoError(t, err)
			err = handleSendMessage(context.Background(), client, Message{Message: data})
			require.NoError(t, err)
			require.Len(t, hub.send, 1)
			event, ok := (<-hub.send).message.(frontendEvents.MessageReceivedEvent)
			require.True(t, ok)
			assert.Equal(t, format, event.Format)
			assert.True(t, event.IsBroadcast)
			assert.Equal(t, "TEST MESSAGE", event.Text)
			require.Len(t, hub.snapshotMessages(42), 1)
			assert.Equal(t, format, hub.snapshotMessages(42)[0].Format)
		})
	}
}

func TestSendMessageRejectsInvalidFormat(t *testing.T) {
	data, err := json.Marshal(frontendEvents.SendMessageEvent{Text: "TEST", Format: "INVALID"})
	require.NoError(t, err)
	err = handleSendMessage(context.Background(), &Client{}, Message{Message: data})
	require.ErrorContains(t, err, "invalid message format")
}

func TestMessageRecipientsResolvePositionNamesToFrequencies(t *testing.T) {
	t.Chdir("../..")
	require.NoError(t, config.InitConfig())
	hub := &Hub{clients: map[*Client]bool{}}
	sender := startQueuedTestClient(&Client{hub: hub, session: 42, position: "119.905", user: shared.NewAuthenticatedUser("123", 0, nil)})
	recipient := startQueuedTestClient(&Client{hub: hub, session: 42, position: "121.630", user: shared.NewAuthenticatedUser("456", 0, nil)})
	observer := startQueuedTestClient(&Client{hub: hub, session: 42, position: "121.630", readOnly: true, user: shared.NewAuthenticatedUser("789", 0, nil)})
	otherSession := startQueuedTestClient(&Client{hub: hub, session: 43, position: "121.630", user: shared.NewAuthenticatedUser("987", 0, nil)})
	for _, client := range []*Client{sender, recipient, observer, otherSession} {
		hub.clients[client] = true
	}
	event := frontendEvents.MessageReceivedEvent{Text: "TEST", Recipients: []string{"APRON ARR"}, Format: "LARGE"}
	hub.dispatchMessage(42, event, sender.GetCid())
	assert.Equal(t, event, waitForOutgoingMessage(t, sender.send))
	assert.Equal(t, event, waitForOutgoingMessage(t, recipient.send))
	assert.Empty(t, observer.send)
	assert.Empty(t, otherSession.send)
}
