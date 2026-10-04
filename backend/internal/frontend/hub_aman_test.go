package frontend

import (
	"context"
	"testing"
	"time"

	"FlightStrips/pkg/events"
	frontendEvents "FlightStrips/pkg/events/frontend"

	"github.com/stretchr/testify/require"
)

func TestPublishAMANStateEventSendsCompleteReplacementOnlyToMatchingAirport(t *testing.T) {
	ekch := startQueuedTestClient(&Client{airport: "EKCH", send: make(chan events.OutgoingMessage, 1)})
	essa := startQueuedTestClient(&Client{airport: "ESSA", send: make(chan events.OutgoingMessage, 1)})
	hub := &Hub{
		clients: map[*Client]bool{ekch: true, essa: true},
		send:    make(chan internalMessage, 1),
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.Run(ctx)

	expected := frontendEvents.AMANStateEvent{Version: 1, Data: frontendEvents.AMANState{Airport: "EKCH", Revision: 12}}
	hub.PublishAMANStateEvent(expected)

	require.Equal(t, expected, waitForOutgoingMessage(t, ekch.send))
	select {
	case message := <-essa.send:
		t.Fatalf("unexpected cross-airport AMAN message: %#v", message)
	case <-time.After(20 * time.Millisecond):
	}
}

func TestAMANPublicationDoesNotCrossSessionsAtSameAirport(t *testing.T) {
	first := startQueuedTestClient(&Client{session: 1, airport: "EKCH", send: make(chan events.OutgoingMessage, 1)})
	second := startQueuedTestClient(&Client{session: 2, airport: "EKCH", send: make(chan events.OutgoingMessage, 1)})
	hub := &Hub{clients: map[*Client]bool{first: true, second: true}, send: make(chan internalMessage, 1)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.Run(ctx)
	event := frontendEvents.AMANStateEvent{Version: 1, Data: frontendEvents.AMANState{SessionID: 1, Airport: "EKCH", Revision: 42}}
	hub.PublishAMANStateEvent(event, 1)
	require.Equal(t, event, waitForOutgoingMessage(t, first.send))
	select {
	case unexpected := <-second.send:
		t.Fatalf("AMAN crossed sessions: %v", unexpected)
	case <-time.After(20 * time.Millisecond):
	}
}
