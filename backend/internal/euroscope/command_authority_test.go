package euroscope

import (
	"FlightStrips/internal/shared"
	"FlightStrips/pkg/events"
	es "FlightStrips/pkg/events/euroscope"
	"context"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func authorityClient(hub *Hub, session int32, cid string) *Client {
	token := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{"exp": float64(time.Now().Add(time.Hour).Unix())})
	return &Client{hub: hub, session: session, user: shared.NewAuthenticatedUser(cid, 0, token), send: make(chan events.OutgoingMessage, 16), closed: make(chan struct{})}
}

type boundPDCService struct {
	shared.PdcService
	hub      *Hub
	observed shared.CommandBinding
}

func (s *boundPDCService) IssueClearance(ctx context.Context, callsign, remarks, cid string, session int32) error {
	s.observed, _ = shared.HumanCommand(ctx)
	return shared.DispatchEuroscopeCommand(ctx, s.hub, session, "MASTER", es.SidEvent{Callsign: callsign, Sid: "TEST"}, nil)
}

func TestEuroScopeControllerActionCapturesNonMasterOwnBinding(t *testing.T) {
	hub := &Hub{}
	clients := registerAuthorityClients(hub, 7, "OWN", "MASTER")
	hub.master[7] = clients["MASTER"]
	pdc := &boundPDCService{hub: hub}
	hub.pdcService = pdc
	handlers := shared.NewMessageHandlers[es.EventType, *Client]()
	handlers.AddAction(es.IssuePdcClearance, handleIssuePdcClearance)
	payload, err := proto.Marshal(&es.IssuePdcClearanceEvent{Callsign: "SAS123"})
	require.NoError(t, err)
	require.NoError(t, handlers.Handle(context.Background(), clients["OWN"], Message{Type: es.IssuePdcClearance, Message: payload}))
	require.NotNil(t, pdc.observed)
	require.Equal(t, "OWN", pdc.observed.CID())
	require.IsType(t, es.SidEvent{}, <-clients["OWN"].send)
	require.Empty(t, clients["MASTER"].send)
	handlers.Add(es.GroundState, func(ctx context.Context, _ *Client, _ Message) error {
		_, human := shared.HumanCommand(ctx)
		require.False(t, human, "observations must remain reconciliation")
		return nil
	})
	require.NoError(t, handlers.Handle(context.Background(), clients["OWN"], Message{Type: es.GroundState}))
}

func registerAuthorityClients(hub *Hub, session int32, cids ...string) map[string]*Client {
	if hub.clients == nil {
		hub.clients = map[*Client]bool{}
	}
	if hub.master == nil {
		hub.master = map[int32]*Client{}
	}
	masterCID := ""
	if existing := hub.master[session]; existing != nil {
		masterCID = existing.GetCid()
	}
	result := map[string]*Client{}
	for _, cid := range cids {
		client := authorityClient(hub, session, cid)
		hub.clients[client] = true
		result[cid] = client
		if cid == masterCID {
			hub.master[session] = client
		}
	}
	hub.syncedSessions.Store(session, struct{}{})
	return result
}

func TestHumanCommandNeverRebindsAfterDisconnect(t *testing.T) {
	hub := &Hub{}
	clients := registerAuthorityClients(hub, 7, "OWN", "MASTER")
	hub.master[7] = clients["MASTER"]
	binding, err := hub.CaptureCommandBinding(7, "OWN")
	require.NoError(t, err)
	ctx, err := shared.WithHumanCommand(context.Background(), binding)
	require.NoError(t, err)
	ctx = context.WithoutCancel(ctx)
	fallback := false
	event := es.EobtEvent{Callsign: "SAS123", Eobt: "1234"}
	require.NoError(t, shared.DispatchCommand(ctx, 7, event, func() { fallback = true }))
	require.IsType(t, es.EobtEvent{}, <-clients["OWN"].send)
	require.Empty(t, clients["MASTER"].send)
	require.False(t, fallback)
	hub.clientsMu.Lock()
	delete(hub.clients, clients["OWN"])
	replacement := authorityClient(hub, 7, "OWN")
	hub.clients[replacement] = true
	hub.clientsMu.Unlock()
	require.Error(t, shared.DispatchEuroscopeCommand(ctx, hub, 7, "MASTER", event, func() { fallback = true }))
	require.Error(t, shared.DispatchCommand(ctx, 8, event, func() { fallback = true }))
	require.Empty(t, replacement.send)
	require.Empty(t, clients["MASTER"].send)
	require.False(t, fallback)
}

func TestQueuedHumanSquawkRetainsConnectionAndSource(t *testing.T) {
	hub := &Hub{}
	clients := registerAuthorityClients(hub, 7, "OWN", "MASTER")
	hub.master[7] = clients["MASTER"]
	hub.squawkThrottle = newSquawkThrottle(time.Hour, nil, hub.dispatchGenerateSquawkRequest)
	t.Cleanup(func() {
		hub.squawkThrottle.mu.Lock()
		defer hub.squawkThrottle.mu.Unlock()
		for _, state := range hub.squawkThrottle.states {
			if state.timer != nil {
				state.timer.Stop()
			}
		}
	})
	hub.SendGenerateSquawk(7, "OWN", "FIRST")
	require.IsType(t, es.GenerateSquawkEvent{}, <-clients["OWN"].send)
	hub.SendGenerateSquawk(7, "OWN", "QUEUED")
	hub.clientsMu.Lock()
	delete(hub.clients, clients["OWN"])
	replacement := authorityClient(hub, 7, "OWN")
	hub.clients[replacement] = true
	hub.clientsMu.Unlock()
	hub.squawkThrottle.flush(7)
	require.Empty(t, replacement.send)
	require.Empty(t, clients["MASTER"].send)
	// An explicit missing human CID cannot enter the automatic resolver.
	hub.SendGenerateSquawk(7, "DISCONNECTED", "MISSING")
	require.Empty(t, clients["MASTER"].send)
}

func TestAutomaticExecutorRequiresEligibleSameSessionConnection(t *testing.T) {
	hub := &Hub{}
	clients := registerAuthorityClients(hub, 7, "MASTER", "ASSUMING")
	hub.master[7] = clients["MASTER"]
	binding, err := hub.ResolveAutomaticCommandBinding(7, "")
	require.NoError(t, err)
	require.Equal(t, "MASTER", binding.CID())
	binding, err = hub.ResolveAutomaticCommandBinding(7, "ASSUMING")
	require.NoError(t, err)
	require.Equal(t, "ASSUMING", binding.CID())
	clients["ASSUMING"].observer = true
	_, err = hub.ResolveAutomaticCommandBinding(7, "ASSUMING")
	require.Error(t, err)
	foreign := authorityClient(hub, 8, "FOREIGN")
	hub.clients[foreign] = true
	hub.master[7] = foreign
	_, err = hub.ResolveAutomaticCommandBinding(7, "")
	require.Error(t, err)
	_, err = hub.CaptureCommandBinding(8, "MASTER")
	require.Error(t, err)
}
