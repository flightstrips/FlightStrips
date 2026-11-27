package frontend

import (
	"context"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/coordinationrequest"
	internalModels "FlightStrips/internal/models"
	"FlightStrips/internal/testutil"
	"FlightStrips/pkg/events"
	frontendEvents "FlightStrips/pkg/events/frontend"
	"github.com/stretchr/testify/require"
)

const forcedFMPTestSession int32 = 42

type memoryForcedFMPStore struct {
	grants map[int32]aman.ForcedFMPGrant
}

func (s *memoryForcedFMPStore) LoadForcedFMP(_ context.Context, session int32) (aman.ForcedFMPGrant, bool, error) {
	grant, ok := s.grants[session]
	return grant, ok, nil
}

func (s *memoryForcedFMPStore) SaveForcedFMP(_ context.Context, grant aman.ForcedFMPGrant) error {
	s.grants[grant.SessionID] = grant
	return nil
}

func (s *memoryForcedFMPStore) ClearForcedFMP(_ context.Context, session int32) error {
	delete(s.grants, session)
	return nil
}

// newForcedFMPTestHub returns a hub whose controllers list is online and a
// client logged on as callsign. Roles resolve from the callsign.
func newForcedFMPTestHub(t *testing.T, callsign string, online *[]*internalModels.Controller) (*Hub, *Client, *recordingAMANCommandService) {
	t.Helper()
	service := &recordingAMANCommandService{}
	hub, client := newAMANCommandTestClient(service, time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC))
	hub.amanRoleForPosition = func(string) string { return "" }
	hub.amanAuthorityRefresh = newSessionRefreshQueue()
	hub.forcedFMP = newForcedFMPRegistry()
	hub.server = &testutil.MockServer{ControllerRepoVal: &testutil.MockControllerRepository{
		ListBySessionFn: func(context.Context, int32) ([]*internalModels.Controller, error) { return *online, nil },
	}}
	client.session = forcedFMPTestSession
	client.callsign = callsign
	client.position = "119.805"
	client.send = make(chan events.OutgoingMessage, 16)
	return hub, client, service
}

func newForcedFMPPeer(hub *Hub, callsign string) *Client {
	_, peer := newAMANCommandTestClient(nil, time.Time{})
	peer.hub = hub
	peer.session = forcedFMPTestSession
	peer.callsign = callsign
	peer.position = "118.455"
	peer.send = make(chan events.OutgoingMessage, 16)
	return peer
}

func sendForcedFMP(t *testing.T, hub *Hub, client *Client, eventType frontendEvents.EventType) {
	t.Helper()
	payload := `{"type":"` + string(eventType) + `","version":1,"data":{"command_id":"force-1"}}`
	require.NoError(t, hub.handlers.Handle(context.Background(), client, Message{Type: eventType, Message: []byte(payload)}))
}

func swapAs(t *testing.T, hub *Hub, client *Client) {
	t.Helper()
	payload := `{"type":"aman.swap_flights","version":1,"data":{"command_id":"swap-1","expected_revision":7,"callsign":"flight-1","other_callsign":"flight-2"}}`
	require.NoError(t, hub.handlers.Handle(context.Background(), client, Message{Type: frontendEvents.AMANSwapFlightsType, Message: []byte(payload)}))
}

func lastRejection(t *testing.T, client *Client) frontendEvents.AMANCommandRejectedEvent {
	t.Helper()
	for {
		select {
		case message := <-client.send:
			if rejection, ok := message.(frontendEvents.AMANCommandRejectedEvent); ok {
				return rejection
			}
		default:
			t.Fatal("expected an AMAN command rejection")
		}
	}
}

func TestForceFMPGivesApproachControllerFMPAuthority(t *testing.T) {
	online := []*internalModels.Controller{}
	hub, client, service := newForcedFMPTestHub(t, "EKCH_W_APP", &online)
	hub.amanStateProvider = fixedAMANState{state: sequencingState()}

	swapAs(t, hub, client)
	require.Empty(t, service.operation, "without FMP authority a controller cannot swap flights outside its STAR families")
	require.Equal(t, string(aman.ErrorUnauthorized), lastRejection(t, client).Data.Code)

	sendForcedFMP(t, hub, client, frontendEvents.AMANForceFMPType)
	require.True(t, hub.hasAMANFMPAuthority(client))
	require.Equal(t, []int32{forcedFMPTestSession}, hub.amanAuthorityRefresh.drain())

	swapAs(t, hub, client)
	require.Equal(t, "swap", service.operation)
	require.Equal(t, "EKCH_W_APP", service.auth.Role, "audit keeps the controller's own role")
	require.True(t, service.auth.FMPAuthority)
	require.True(t, service.auth.HasFMPAuthority())
}

func TestForceFMPIsLimitedToApproachAndAreaControllers(t *testing.T) {
	for callsign, allowed := range map[string]bool{"EKCH_W_APP": true, "EKDK_V_CTR": true, "EKCH_A_TWR": false, "EKCH_DEL": false, "EKDK_FMP": false} {
		t.Run(callsign, func(t *testing.T) {
			online := []*internalModels.Controller{}
			hub, client, _ := newForcedFMPTestHub(t, callsign, &online)
			sendForcedFMP(t, hub, client, frontendEvents.AMANForceFMPType)
			_, held := hub.forcedFMP.get(context.Background(), forcedFMPTestSession)
			require.Equal(t, allowed, held)
			if !allowed {
				lastRejection(t, client)
			}
		})
	}
}

func TestForceFMPRejectedWhileFMPOnline(t *testing.T) {
	online := []*internalModels.Controller{{Session: forcedFMPTestSession, Callsign: "EKDK_FMP", Position: "131.040"}}
	hub, client, _ := newForcedFMPTestHub(t, "EKCH_W_APP", &online)

	sendForcedFMP(t, hub, client, frontendEvents.AMANForceFMPType)

	require.Equal(t, string(aman.ErrorInvalidTransition), lastRejection(t, client).Data.Code)
	require.False(t, hub.hasAMANFMPAuthority(client))
	event := hub.amanFMPAuthorityEvent(context.Background(), client, true, true)
	require.False(t, event.Data.CanForce)
	require.True(t, event.Data.FMPOnline)
}

func TestForceFMPCanBeTakenOverByAnotherController(t *testing.T) {
	online := []*internalModels.Controller{}
	hub, first, _ := newForcedFMPTestHub(t, "EKCH_W_APP", &online)
	second := newForcedFMPPeer(hub, "EKDK_V_CTR")

	sendForcedFMP(t, hub, first, frontendEvents.AMANForceFMPType)
	require.True(t, hub.hasAMANFMPAuthority(first))
	sendForcedFMP(t, hub, second, frontendEvents.AMANForceFMPType)

	require.False(t, hub.hasAMANFMPAuthority(first), "only one controller holds forced FMP")
	require.True(t, hub.hasAMANFMPAuthority(second))
	holder := hub.amanFMPAuthorityEvent(context.Background(), first, false, true).Data
	require.Equal(t, "EKDK_V_CTR", *holder.ForcedHolder)
	require.True(t, holder.CanForce, "the previous holder can take it back")
}

func TestForcedFMPReleasedByHolderOnly(t *testing.T) {
	online := []*internalModels.Controller{}
	hub, holder, _ := newForcedFMPTestHub(t, "EKCH_W_APP", &online)
	other := newForcedFMPPeer(hub, "EKCH_O_APP")
	sendForcedFMP(t, hub, holder, frontendEvents.AMANForceFMPType)

	sendForcedFMP(t, hub, other, frontendEvents.AMANReleaseForcedFMPType)
	require.Equal(t, string(aman.ErrorInvalidTransition), lastRejection(t, other).Data.Code)
	require.True(t, hub.hasAMANFMPAuthority(holder))

	sendForcedFMP(t, hub, holder, frontendEvents.AMANReleaseForcedFMPType)
	require.False(t, hub.hasAMANFMPAuthority(holder))
}

func TestForcedFMPEndsWhenFMPLogsOnOrHolderLogsOff(t *testing.T) {
	online := []*internalModels.Controller{}
	hub, holder, _ := newForcedFMPTestHub(t, "EKCH_W_APP", &online)

	sendForcedFMP(t, hub, holder, frontendEvents.AMANForceFMPType)
	hub.resetForcedFMPOnControllerOnline(forcedFMPTestSession, "EKCH_O_APP", "118.455")
	require.True(t, hub.hasAMANFMPAuthority(holder), "a non-FMP logon keeps the grant")
	hub.resetForcedFMPOnControllerOnline(forcedFMPTestSession, "EKDK_FMP", "131.040")
	require.False(t, hub.hasAMANFMPAuthority(holder), "FMP logon resets forced FMP")

	sendForcedFMP(t, hub, holder, frontendEvents.AMANForceFMPType)
	hub.resetForcedFMPOnControllerOffline(forcedFMPTestSession, "EKCH_O_APP")
	require.True(t, hub.hasAMANFMPAuthority(holder), "another controller's logoff keeps the grant")
	hub.resetForcedFMPOnControllerOffline(forcedFMPTestSession, "ekch_w_app")
	require.False(t, hub.hasAMANFMPAuthority(holder), "holder logoff resets forced FMP")
}

func TestForcedFMPIsTiedToCallsignAndSession(t *testing.T) {
	online := []*internalModels.Controller{}
	hub, holder, _ := newForcedFMPTestHub(t, "EKCH_W_APP", &online)
	sendForcedFMP(t, hub, holder, frontendEvents.AMANForceFMPType)

	otherSession := newForcedFMPPeer(hub, "EKCH_W_APP")
	otherSession.session = forcedFMPTestSession + 1
	require.False(t, hub.hasAMANFMPAuthority(otherSession))

	observer := newForcedFMPPeer(hub, "EKCH_W_APP")
	observer.readOnly = true
	require.False(t, hub.hasAMANFMPAuthority(observer))
}

func TestForcedFMPGrantSurvivesRestartThroughStore(t *testing.T) {
	store := &memoryForcedFMPStore{grants: map[int32]aman.ForcedFMPGrant{}}
	// The holder is still logged on, so the stored grant is kept.
	online := []*internalModels.Controller{{ID: 7, Session: forcedFMPTestSession, Callsign: "EKCH_W_APP", Position: "119.805"}}
	hub, holder, _ := newForcedFMPTestHub(t, "EKCH_W_APP", &online)
	hub.SetAMANForcedFMPStore(store)
	sendForcedFMP(t, hub, holder, frontendEvents.AMANForceFMPType)
	require.Contains(t, store.grants, forcedFMPTestSession)

	restarted, client, _ := newForcedFMPTestHub(t, "EKCH_W_APP", &online)
	restarted.SetAMANForcedFMPStore(store)
	require.True(t, restarted.hasAMANFMPAuthority(client))
}

func TestForcedFMPAuthorityReachesCoordinationContext(t *testing.T) {
	online := []*internalModels.Controller{}
	hub, holder, _ := newForcedFMPTestHub(t, "EKCH_W_APP", &online)
	sendForcedFMP(t, hub, holder, frontendEvents.AMANForceFMPType)

	auth, err := hub.coordinationContext(holder)
	require.NoError(t, err)
	require.Equal(t, coordinationrequest.CommandContext{
		SessionID: forcedFMPTestSession, Airport: "EKCH", Actor: holder.GetCid(), Role: "EKCH_W_APP",
		ReceivedAt: auth.ReceivedAt, FMPAuthority: true,
	}, auth)
	require.True(t, auth.HasFMPAuthority())
}
