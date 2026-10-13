package frontend

import (
	"FlightStrips/internal/config"
	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
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

func TestMessageRecipientsUseCurrentSectorCoverage(t *testing.T) {
	t.Chdir("../..")
	require.NoError(t, config.InitConfig())
	for _, tc := range []struct {
		name       string
		area       string
		sector     string
		planner    string
		direct     string
		wantDirect bool
		withApron  bool
	}{
		{name: "delivery covers planner with apron online", area: "SEQ PLN", sector: "SQ", planner: "119.905", withApron: true},
		{name: "delivery covers planner without apron", area: "SEQ PLN", sector: "SQ", planner: "119.905"},
		{name: "direct frequency takes priority", area: "SEQ PLN", sector: "SQ", planner: "119.905", direct: "121.905", wantDirect: true},
		{name: "west arrival coverage", area: "GND WEST", sector: "GWA", planner: "118.105"},
		{name: "west departure coverage", area: "GND WEST", sector: "GWD", planner: "119.355"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sectorRepo := &testutil.MockSectorOwnerRepository{
				ListBySessionFn: func(_ context.Context, session int32) ([]*models.SectorOwner, error) {
					require.EqualValues(t, 42, session)
					return []*models.SectorOwner{{Position: tc.planner, Sector: []string{tc.sector}}}, nil
				},
			}
			hub := &Hub{clients: map[*Client]bool{}, server: &testutil.MockServer{SectorRepoVal: sectorRepo}}
			newClient := func(cid, position, callsign string, session int32, readOnly bool) *Client {
				client := startQueuedTestClient(&Client{
					hub: hub, session: session, position: position, callsign: callsign, readOnly: readOnly,
					user: shared.NewAuthenticatedUser(cid, 0, nil),
				})
				hub.clients[client] = true
				return client
			}
			sender := newClient("1", "121.830", "EKCH_GE_TWR", 42, false)
			planner := newClient("2", tc.planner, "EKCH_DEL", 42, false)
			observer := newClient("3", tc.planner, "EKCH_OBS", 42, false)
			readOnly := newClient("4", tc.planner, "EKCH_DEL", 42, true)
			otherSession := newClient("5", tc.planner, "EKCH_DEL", 43, false)
			var apron, direct *Client
			if tc.withApron {
				apron = newClient("6", "121.630", "EKCH_A_GND", 42, false)
			}
			if tc.direct != "" {
				direct = newClient("7", tc.direct, "EKCH_B_GND", 42, false)
			}
			event := frontendEvents.MessageReceivedEvent{Text: "TEST", Recipients: []string{tc.area}}
			hub.dispatchMessage(42, event, sender.GetCid())
			assert.Equal(t, event, waitForOutgoingMessage(t, sender.send))
			if tc.wantDirect {
				assert.Equal(t, event, waitForOutgoingMessage(t, direct.send))
				assert.Empty(t, planner.send)
			} else {
				assert.Equal(t, event, waitForOutgoingMessage(t, planner.send))
			}
			assert.Empty(t, observer.send)
			assert.Empty(t, readOnly.send)
			assert.Empty(t, otherSession.send)
			if apron != nil {
				assert.Empty(t, apron.send)
			}
		})
	}
}
