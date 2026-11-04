package frontend

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	"FlightStrips/pkg/events"
	es "FlightStrips/pkg/events/euroscope"
	fe "FlightStrips/pkg/events/frontend"
	"github.com/stretchr/testify/require"
)

type selectedPluginHub struct {
	testutil.MockEuroscopeHub
	selected *models.Controller
	binding  shared.CommandBinding
	captures []string
}

func (h *selectedPluginHub) GetFrontendController(string) *models.Controller { return nil } // multiple own instances
func (h *selectedPluginHub) GetMembershipController(cid string, session int32, instance, membership string) *models.Controller {
	if cid == "123" && session == 1 && instance == "second-instance" && membership == "selected" {
		return h.selected
	}
	return nil
}
func (h *selectedPluginHub) CaptureMembershipBinding(session int32, cid, instance, membership string) (shared.CommandBinding, error) {
	if h.GetMembershipController(cid, session, instance, membership) == nil {
		return nil, errors.New("unavailable")
	}
	h.captures = append(h.captures, instance+"/"+membership)
	return h.binding, nil
}

func TestExplicitMembershipSelectionRetainsExactSourceAndRejectsPreboundReplacement(t *testing.T) {
	own := &identityTestBinding{session: 1, cid: "123"}
	source := &selectedPluginHub{selected: &models.Controller{Session: 1, Callsign: "SECOND", Position: "118.000"}, binding: own}
	hub := &Hub{server: &testutil.MockServer{EuroscopeHubVal: source}}
	client := &Client{hub: hub, user: shared.NewAuthenticatedUser("123", 0, nil), session: 1, selection: membershipSelection{session: 1, instance: "second-instance", membership: "selected"}}
	ctx, err := client.PrepareMessageContext(t.Context(), "update_order")
	require.NoError(t, err)
	captured, ok := shared.HumanCommand(ctx)
	require.True(t, ok)
	require.Same(t, own, captured)
	_, err = client.PrepareMessageContext(ctx, "move")
	require.NoError(t, err)
	for _, wrong := range []*identityTestBinding{{session: 2, cid: "123"}, {session: 1, cid: "456"}, {session: 1, cid: "123"}} {
		bound, err := shared.WithHumanCommand(context.Background(), wrong)
		require.NoError(t, err)
		_, err = client.PrepareMessageContext(bound, "update_order")
		require.Error(t, err, "another instance binding must never replace selected source")
	}
	require.Equal(t, "second-instance/selected", source.captures[0])
	source.selected = nil
	_, err = client.PrepareMessageContext(t.Context(), "move")
	require.Error(t, err)
}

func TestWaitingExplicitSelectionAssociatesWithoutGlobalCIDLookup(t *testing.T) {
	source := &selectedPluginHub{selected: &models.Controller{Session: 1, Callsign: "SECOND", Position: "118.000"}}
	hub := &Hub{server: &testutil.MockServer{EuroscopeHubVal: source}, clients: map[*Client]bool{}}
	client := &Client{hub: hub, user: shared.NewAuthenticatedUser("123", 0, nil), session: WaitingForEuroscopeConnectionSessionId, selection: membershipSelection{session: 1, instance: "second-instance", membership: "selected"}}
	require.True(t, hub.associateClientWithSession(client, 1, source.selected, &models.Session{ID: 1, Name: "LIVE", Airport: "AAAA"}, false))
	require.Equal(t, int32(1), client.GetSession())
	require.Equal(t, "SECOND", client.GetCallsign())
	_, err := hub.controllerForSelection("other-user", client.selection)
	require.Error(t, err)
	_, err = selectionFromAuthentication(events.AuthenticationEvent{SessionID: 1})
	require.Error(t, err)
}

func TestExactMembershipRemovalKeepsOtherInstanceFrontendAssociated(t *testing.T) {
	hub := &Hub{server: &testutil.MockServer{EuroscopeHubVal: &testutil.MockEuroscopeHub{}}, clients: map[*Client]bool{}}
	makeClient := func(member string) *Client {
		return startQueuedTestClient(&Client{hub: hub, user: shared.NewAuthenticatedUser("123", 0, nil), session: 1, sessionName: "LIVE", airport: "AAAA", callsign: member, selection: membershipSelection{session: 1, instance: member, membership: member}})
	}
	removed, retained := makeClient("removed"), makeClient("retained")
	hub.clients[removed] = true
	hub.clients[retained] = true
	hub.handlePluginMembershipDisconnected(cidDisconnectMessage{cid: "123", session: 1, instance: "removed", membership: "removed"})
	require.Equal(t, WaitingForEuroscopeConnectionSessionId, removed.GetSession())
	require.Equal(t, int32(1), retained.GetSession())
	require.IsType(t, fe.DisconnectEvent{}, <-removed.send)
	require.Empty(t, retained.send)
}

type selectedFrequencyBinding struct {
	identityTestBinding
	deliver     func(fe.FrequencyMessageResultEvent)
	sent        []es.SendFrequencyMessageEvent
	unavailable bool
}

func (b *selectedFrequencyBinding) FrequencyMessageSender(_ string, deliver func(fe.FrequencyMessageResultEvent)) func(es.SendFrequencyMessageEvent) bool {
	if b.unavailable {
		return nil
	}
	b.deliver = deliver
	return func(event es.SendFrequencyMessageEvent) bool { b.sent = append(b.sent, event); return true }
}
func TestFrequencyMessageUsesCapturedBindingAndReturnsOnlyToInitiatingFrontend(t *testing.T) {
	text := "T"
	source := &frequencyTestRouter{}
	server := mockServerWithStripRepo(&testutil.MockStripRepository{GetByCallsignFn: func(context.Context, int32, string) (*models.Strip, error) {
		return &models.Strip{Callsign: "SAS123", CommunicationType: &text}, nil
	}})
	server.EuroscopeHubVal = source
	hub := buildFrontendTestHub(server, &spyStripService{})
	client := startQueuedTestClient(buildFrontendTestClient(hub, 1, "AAAA"))
	client.SetUser(shared.NewAuthenticatedUser("123", 0, nil))
	other := startQueuedTestClient(buildFrontendTestClient(hub, 1, "AAAA"))
	other.SetUser(shared.NewAuthenticatedUser("123", 0, nil))
	binding := &selectedFrequencyBinding{identityTestBinding: identityTestBinding{session: 1, cid: "123"}}
	ctx, err := shared.WithHumanCommand(t.Context(), binding)
	require.NoError(t, err)
	req := fe.SendFrequencyMessageEvent{RequestID: "exact-request", Callsign: "SAS123", Message: "Taxi approved"}
	require.NoError(t, handleSendFrequencyMessage(ctx, client, marshalMessage(t, req)))
	require.Len(t, binding.sent, 1)
	require.Empty(t, source.sent, "captured human work cannot use another CID router")
	binding.deliver(fe.FrequencyMessageResultEvent{RequestID: req.RequestID, Callsign: req.Callsign, Submitted: true})
	require.IsType(t, fe.FrequencyMessageResultEvent{}, <-client.send)
	require.Empty(t, other.send)
}

func TestUnavailableCapturedFrequencySenderRejectsBeforeBundledActions(t *testing.T) {
	text := "T"
	strip := &models.Strip{Callsign: "SAS123", CommunicationType: &text}
	server := mockServerWithStripRepo(&testutil.MockStripRepository{GetByCallsignFn: func(context.Context, int32, string) (*models.Strip, error) { return strip, nil }})
	service := &spyStripService{}
	hub := buildFrontendTestHub(server, service)
	hub.handlers = shared.NewMessageHandlers[fe.EventType, *Client]()
	hub.handlers.Add(fe.Move, handleMove)
	client := startQueuedTestClient(buildFrontendTestClient(hub, 1, "AAAA"))
	client.SetUser(shared.NewAuthenticatedUser("123", 0, nil))
	binding := &selectedFrequencyBinding{identityTestBinding: identityTestBinding{session: 1, cid: "123"}, unavailable: true}
	ctx, err := shared.WithHumanCommand(t.Context(), binding)
	require.NoError(t, err)
	req := fe.SendFrequencyMessageEvent{RequestID: "unavailable-exact", Callsign: "SAS123", Message: "Taxi approved", Actions: []json.RawMessage{[]byte(`{"type":"move","callsign":"SAS123","bay":"TAXI"}`)}}
	require.NoError(t, handleSendFrequencyMessage(ctx, client, marshalMessage(t, req)))
	require.Empty(t, service.moveFrontendStripCalls)
	require.Empty(t, binding.sent)
	result := (<-client.send).(fe.FrequencyMessageResultEvent)
	require.Zero(t, result.AppliedActions)
	require.Contains(t, result.Error, "connect an updated EuroScope plugin")
}
