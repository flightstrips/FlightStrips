package euroscope

import (
	"FlightStrips/pkg/events"
	euroscopeEvents "FlightStrips/pkg/events/euroscope"
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type gainLossProviderStub struct {
	event   euroscopeEvents.AMANGainLossEvent
	airport string
	err     error
}

type holdingEATProviderStub struct {
	events  []euroscopeEvents.HoldEvent
	airport string
	err     error
}

func (s *holdingEATProviderStub) CurrentAMANHoldingEAT(_ context.Context, airport string) ([]euroscopeEvents.HoldEvent, error) {
	s.airport = airport
	return s.events, s.err
}

func (s *gainLossProviderStub) CurrentAMANGainLoss(_ context.Context, airport string) (euroscopeEvents.AMANGainLossEvent, error) {
	s.airport = airport
	return s.event, s.err
}

func TestSendInitialAMANGainLossUsesAuthenticatedClientsAirport(t *testing.T) {
	want := euroscopeEvents.AMANGainLossEvent{Airport: "EKCH", Revision: 7}
	provider := &gainLossProviderStub{event: want}
	hub := &Hub{amanGainLoss: provider}
	client := &Client{airport: "EKCH", send: make(chan events.OutgoingMessage, 1), closed: make(chan struct{})}

	hub.sendInitialAMANGainLoss(client)

	require.Equal(t, "EKCH", provider.airport)
	require.Equal(t, want, <-client.send)
}

func TestPublishAMANGainLossTargetsAirportAcrossSessions(t *testing.T) {
	hub := &Hub{send: make(chan internalMessage, 1), clients: make(map[*Client]bool)}
	ekch1 := &Client{airport: "EKCH", session: 1, send: make(chan events.OutgoingMessage, 1), closed: make(chan struct{})}
	ekch2 := &Client{airport: "ekch", session: 2, send: make(chan events.OutgoingMessage, 1), closed: make(chan struct{})}
	ekbi := &Client{airport: "EKBI", session: 1, send: make(chan events.OutgoingMessage, 1), closed: make(chan struct{})}
	hub.clients[ekch1], hub.clients[ekch2], hub.clients[ekbi] = true, true, true

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.Run(ctx)
	want := euroscopeEvents.AMANGainLossEvent{Airport: "EKCH", Revision: 8}
	hub.PublishAMANGainLoss(want)

	require.Equal(t, want, <-ekch1.send)
	require.Equal(t, want, <-ekch2.send)
	select {
	case unexpected := <-ekbi.send:
		t.Fatalf("unexpected cross-airport event: %#v", unexpected)
	default:
	}
}

func TestSendInitialAMANHoldingEATRepairsOperationalClientOnly(t *testing.T) {
	want := euroscopeEvents.HoldEvent{Callsign: "SAS123", Hold: "OLPIB", HoldType: "enroute", HoldEat: "1422"}
	provider := &holdingEATProviderStub{events: []euroscopeEvents.HoldEvent{want}}
	hub := &Hub{amanHoldingEAT: provider}
	client := &Client{airport: "EKCH", send: make(chan events.OutgoingMessage, 1), closed: make(chan struct{})}

	hub.sendInitialAMANHoldingEAT(client)

	require.Equal(t, "EKCH", provider.airport)
	require.Equal(t, want, <-client.send)

	observer := &Client{airport: "EKCH", observer: true, send: make(chan events.OutgoingMessage, 1), closed: make(chan struct{})}
	hub.sendInitialAMANHoldingEAT(observer)
	require.Empty(t, observer.send)
}

func TestPublishAMANHoldingEATTargetsAirportAcrossSessions(t *testing.T) {
	hub := &Hub{send: make(chan internalMessage, 1), clients: make(map[*Client]bool)}
	ekch := &Client{airport: "EKCH", session: 1, send: make(chan events.OutgoingMessage, 1), closed: make(chan struct{})}
	ekbi := &Client{airport: "EKBI", session: 1, send: make(chan events.OutgoingMessage, 1), closed: make(chan struct{})}
	hub.clients[ekch], hub.clients[ekbi] = true, true

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.Run(ctx)
	want := euroscopeEvents.HoldEvent{Callsign: "SAS123", Hold: "OLPIB", HoldType: "enroute", HoldEat: "1422"}
	hub.PublishAMANHoldingEAT("EKCH", []euroscopeEvents.HoldEvent{want})

	require.Equal(t, want, <-ekch.send)
	select {
	case unexpected := <-ekbi.send:
		t.Fatalf("unexpected cross-airport event: %#v", unexpected)
	default:
	}
}
