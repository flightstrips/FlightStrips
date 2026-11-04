package frontend

import (
	"context"
	"sync"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	es "FlightStrips/pkg/events/euroscope"
	fe "FlightStrips/pkg/events/frontend"
	"github.com/stretchr/testify/require"
)

func TestFrontendIdentityTransitionIsAtomicForSocketReaders(t *testing.T) {
	client := &Client{}
	client.setIdentity(1, "LIVE", "position-a", "AAAA", "controller-a", false)
	var writer sync.WaitGroup
	writer.Add(1)
	go func() {
		defer writer.Done()
		for i := 0; i < 1000; i++ {
			client.setIdentity(2, "SWEATBOX_test", "position-b", "BBBB", "controller-b", true)
			client.setIdentity(1, "LIVE", "position-a", "AAAA", "controller-a", false)
		}
	}()
	for i := 0; i < 1000; i++ {
		identity := client.identitySnapshot()
		if identity.session == 1 {
			require.Equal(t, clientIdentity{session: 1, sessionName: "LIVE", position: "position-a", airport: "AAAA", callsign: "controller-a"}, identity)
		} else {
			require.Equal(t, clientIdentity{session: 2, sessionName: "SWEATBOX_test", position: "position-b", airport: "BBBB", callsign: "controller-b", readOnly: true}, identity)
		}
		client.GetSession()
		client.GetSessionName()
		client.GetAirport()
		client.GetPosition()
		client.GetCallsign()
		_ = client.CanHandleMessage("move")
	}
	writer.Wait()
}

type identityTestBinding struct {
	session int32
	cid     string
}

func (b *identityTestBinding) Session() int32                        { return b.session }
func (b *identityTestBinding) CID() string                           { return b.cid }
func (b *identityTestBinding) Same(other shared.CommandBinding) bool { return other == b }
func (b *identityTestBinding) Validate() error                       { return nil }
func (b *identityTestBinding) Send(es.OutgoingMessage) error         { return nil }

type identityCaptureHub struct {
	testutil.MockEuroscopeHub
	capture func(int32, string) (shared.CommandBinding, error)
}

func (h *identityCaptureHub) CaptureCommandBinding(session int32, cid string) (shared.CommandBinding, error) {
	return h.capture(session, cid)
}

type requestIdentityOrderService struct {
	noOpStripService
	t      *testing.T
	called bool
}

func (s *requestIdentityOrderService) MoveStripBetween(ctx context.Context, session int32, _ string, _ *fe.StripRef, _ string) error {
	require.Equal(s.t, int32(1), session)
	binding, ok := shared.HumanCommand(ctx)
	require.True(s.t, ok)
	require.Equal(s.t, int32(1), binding.Session())
	s.called = true
	return nil
}
func TestDispatchKeepsCapturedIdentityWhenBindingResolutionAssociatesAnotherSession(t *testing.T) {
	for _, nested := range []bool{false, true} {
		t.Run(map[bool]string{false: "direct", true: "nested"}[nested], func(t *testing.T) {
			service := &requestIdentityOrderService{t: t}
			owner := "position-a"
			repo := &testutil.MockStripRepository{GetByCallsignFn: func(_ context.Context, session int32, _ string) (*models.Strip, error) {
				require.Equal(t, int32(1), session)
				return &models.Strip{Bay: "TAXI", Owner: &owner}, nil
			}}
			server := &testutil.MockServer{StripRepoVal: repo}
			hub := buildFrontendTestHub(server, service)
			client := buildFrontendTestClient(hub, 1, "AAAA")
			client.position = "position-a"
			client.SetUser(shared.NewAuthenticatedUser("123", 0, nil))
			captures := 0
			server.EuroscopeHubVal = &identityCaptureHub{capture: func(session int32, cid string) (shared.CommandBinding, error) {
				captures++
				require.Equal(t, int32(1), session)
				client.setIdentity(2, "SWEATBOX_test", "position-b", "BBBB", "controller-b", false)
				return &identityTestBinding{session: session, cid: cid}, nil
			}}
			hub.handlers = shared.NewMessageHandlers[fe.EventType, *Client]()
			hub.handlers.Add(fe.UpdateOrder, handleUpdateOrder)
			message := marshalMessage(t, fe.UpdateOrderEvent{Callsign: "SAS123"})
			message.Type = fe.UpdateOrder
			if nested {
				hub.handlers.Add("nested", func(ctx context.Context, client *Client, _ Message) error {
					return hub.handlers.Handle(ctx, client, message)
				})
				require.NoError(t, hub.handlers.Handle(t.Context(), client, Message{Type: "nested"}))
			} else {
				require.NoError(t, hub.handlers.Handle(t.Context(), client, message))
			}
			require.True(t, service.called)
			require.Equal(t, 1, captures)
		})
	}
}

func TestAMANDispatchUsesOneCapturedSessionAirportAndRole(t *testing.T) {
	service := &recordingAMANCommandService{}
	hub, client := newAMANCommandTestClient(service, time.Now().UTC())
	client.session = 1
	hub.amanRoleForPosition = func(position string) string {
		if position == "131.040" {
			return "EKDK_FMP"
		}
		return "EKCH_APP"
	}
	initial := client.identitySnapshot()
	server := hub.server.(*testutil.MockServer)
	server.EuroscopeHubVal = &identityCaptureHub{capture: func(session int32, cid string) (shared.CommandBinding, error) {
		require.Equal(t, initial.session, session)
		client.setIdentity(2, "SWEATBOX_test", "position-b", "BBBB", "controller-b", true)
		return &identityTestBinding{session: session, cid: cid}, nil
	}}
	payload := []byte(`{"type":"aman.lock_flight","version":1,"data":{"command_id":"command-1","expected_revision":7,"callsign":"flight-1"}}`)
	require.NoError(t, hub.handlers.Handle(t.Context(), client, Message{Type: fe.AMANLockFlightType, Message: payload}))
	require.Equal(t, "lock", service.operation)
	require.Equal(t, initial.session, service.auth.SessionID)
	require.Equal(t, initial.airport, service.auth.Airport)
	require.Equal(t, "EKDK_FMP", service.auth.Role)
}

func TestObserverAMANRequestCannotAcquireAuthorityThroughLaterAssociation(t *testing.T) {
	service := &recordingAMANCommandService{}
	hub, client := newAMANCommandTestClient(service, time.Now().UTC())
	client.setIdentity(1, "LIVE", "131.040", "EKCH", "observer", true)
	hub.handlers.Add(fe.AMANLockFlightType, func(ctx context.Context, client *Client, message Message) error {
		client.setIdentity(2, "SWEATBOX_test", "131.040", "BBBB", "controller", false)
		return handleAMANLockFlight(ctx, client, message)
	})
	payload := []byte(`{"type":"aman.lock_flight","version":1,"data":{"command_id":"command-1","expected_revision":7,"callsign":"flight-1"}}`)
	require.NoError(t, hub.handlers.Handle(t.Context(), client, Message{Type: fe.AMANLockFlightType, Message: payload}))
	require.Empty(t, service.operation)
	require.Equal(t, string(aman.ErrorUnauthorized), (<-client.send).(fe.AMANCommandRejectedEvent).Data.Code)
}

type identityTransitionMoveService struct {
	noOpStripService
	client  *Client
	t       *testing.T
	ordered bool
}

func (s *identityTransitionMoveService) MoveFrontendStrip(_ context.Context, session int32, _ string, _ string, _ string, airport, position string, _ bool, _ bool) error {
	require.Equal(s.t, int32(1), session)
	require.Equal(s.t, "AAAA", airport)
	require.Equal(s.t, "position-a", position)
	s.client.setIdentity(2, "SWEATBOX_test", "position-b", "BBBB", "controller-b", false)
	return nil
}
func (s *identityTransitionMoveService) MoveStripBetween(_ context.Context, session int32, _ string, _ *fe.StripRef, _ string) error {
	require.Equal(s.t, int32(1), session, "ordered continuation must keep the initiating identity")
	s.ordered = true
	return nil
}
func TestOrderedMoveKeepsIdentityAcrossAssociationChange(t *testing.T) {
	service := &identityTransitionMoveService{t: t}
	hub := buildFrontendTestHub(&testutil.MockServer{StripRepoVal: &testutil.MockStripRepository{GetByCallsignFn: func(_ context.Context, session int32, _ string) (*models.Strip, error) {
		require.Equal(t, int32(1), session)
		return &models.Strip{Bay: "TAXI"}, nil
	}}}, service)
	client := buildFrontendTestClient(hub, 1, "AAAA")
	client.position = "position-a"
	service.client = client
	require.NoError(t, handleMove(t.Context(), client, marshalMessage(t, fe.MoveEvent{Callsign: "SAS123", Bay: "TAXI", Ordered: true})))
	require.True(t, service.ordered)
}
