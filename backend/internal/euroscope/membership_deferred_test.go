package euroscope

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"FlightStrips/internal/shared"
	es "FlightStrips/pkg/events/euroscope"
	front "FlightStrips/pkg/events/frontend"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

type epochDeleteService struct {
	noOpStripService
	calls   atomic.Int32
	started chan struct{}
	release chan struct{}
}

func (s *epochDeleteService) DeleteStrip(ctx context.Context, _ int32, _ string) error {
	if s.started != nil {
		close(s.started)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.release:
		}
	}
	s.calls.Add(1)
	return nil
}
func TestOldMasterAbsenceCannotDeleteAfterElection(t *testing.T) {
	hub := membershipTestHub()
	hub.aircraftDisconnectTimers = map[string]*aircraftDisconnectEntry{}
	service := &epochDeleteService{}
	hub.stripService = service
	root := membershipTestRoot(hub, "123", "a")
	other := membershipTestRoot(hub, "456", "b")
	a := membershipTestMember(hub, root, 1, "AAAA", "LIVE", "a")
	b := membershipTestMember(hub, other, 1, "AAAA", "LIVE", "b")
	hub.master[1] = a
	hub.scheduleAircraftDisconnect(1, "SAS123", 40*time.Millisecond)
	hub.aircraftDisconnectMu.Lock()
	old := hub.aircraftDisconnectTimers["1:SAS123"]
	hub.aircraftDisconnectMu.Unlock()
	hub.setMasterClient(b)
	select {
	case <-old.done:
	case <-time.After(time.Second):
		t.Fatal("old absence worker did not stop")
	}
	require.Zero(t, service.calls.Load())
	require.False(t, hub.IsAircraftDisconnectPending(1, "SAS123"))
	hub.scheduleAircraftDisconnect(1, "SAS123", time.Millisecond)
	require.Eventually(t, func() bool { return service.calls.Load() == 1 }, time.Second, time.Millisecond, "replacement master's fresh absence should retain normal cleanup")
}

func TestAbsenceDatabaseWorkIsOrderedBeforeItsSessionElection(t *testing.T) {
	hub := membershipTestHub()
	hub.aircraftDisconnectTimers = map[string]*aircraftDisconnectEntry{}
	service := &epochDeleteService{started: make(chan struct{}), release: make(chan struct{})}
	hub.stripService = service
	root := membershipTestRoot(hub, "123", "a")
	other := membershipTestRoot(hub, "456", "b")
	a := membershipTestMember(hub, root, 1, "AAAA", "LIVE", "a")
	b := membershipTestMember(hub, other, 1, "AAAA", "LIVE", "b")
	hub.master[1] = a
	hub.scheduleAircraftDisconnect(1, "SAS123", time.Millisecond)
	<-service.started
	changed := make(chan struct{})
	go func() { hub.setMasterClient(b); close(changed) }()
	select {
	case <-changed:
		t.Fatal("election overtook already applying absence")
	case <-time.After(20 * time.Millisecond):
	}
	close(service.release)
	<-changed
	require.Equal(t, int32(1), service.calls.Load())
}

func TestRegisteredElectionWaitDoesNotHoldGlobalClientRegistry(t *testing.T) {
	hub := membershipTestHub()
	root := membershipTestRoot(hub, "123", "a")
	other := membershipTestRoot(hub, "456", "b")
	a := membershipTestMember(hub, root, 1, "AAAA", "LIVE", "a")
	b := membershipTestMember(hub, other, 1, "AAAA", "LIVE", "b")
	hub.master[1] = a
	b.callsign = "EKDK_FMP"
	a.callsign = "EKCH_A_TWR"
	entered := make(chan struct{})
	validate := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- a.ApplyScopedMessage(context.Background(), a.wireContext(), "sync", nil, es.CommandOrigin_COMMAND_ORIGIN_UNSPECIFIED, "", func(context.Context) error {
			close(entered)
			<-validate
			return (&commandBinding{hub: hub, client: a, session: 1, cid: "123"}).Validate()
		})
	}()
	<-entered
	elected := make(chan struct{})
	go func() { hub.promoteRegisteredMasterIfPreferred(b); close(elected) }()
	registered := make(chan struct{})
	go func() {
		hub.clientsMu.Lock()
		hub.clients[authorityClient(hub, 2, "789")] = true
		hub.clientsMu.Unlock()
		close(registered)
	}()
	select {
	case <-registered:
	case <-time.After(time.Second):
		close(validate)
		t.Fatal("another airport registration waited behind session A election")
	}
	close(validate)
	require.NoError(t, <-done)
	<-elected
}

func TestWarmupMembershipIsNotEligibleForDisconnectPromotion(t *testing.T) {
	hub := membershipTestHub()
	root := membershipTestRoot(hub, "123", "a")
	cold := membershipTestMember(hub, root, 1, "AAAA", "LIVE", "cold")
	cold.roleEligible.Store(false)
	hub.master[1] = nil
	require.Nil(t, preferredMasterClient(hub.clientsSnapshot(), 1, nil))
	_, registered := hub.promoteRegisteredMasterIfPreferred(cold)
	require.True(t, registered)
	require.Nil(t, hub.getMasterClient(1))
	cold.roleEligible.Store(true)
	master, registered := hub.promoteRegisteredMasterIfPreferred(cold)
	require.True(t, registered)
	require.True(t, master)
}

func TestFrequencyResultMatchesExactRequestMembershipAndCallback(t *testing.T) {
	hub := membershipTestHub()
	one := membershipTestRoot(hub, "123", "one")
	two := membershipTestRoot(hub, "123", "two")
	a := membershipTestMember(hub, one, 1, "AAAA", "LIVE", "a")
	b := membershipTestMember(hub, two, 1, "AAAA", "LIVE", "b")
	a.position = "121.905"
	a.frequencyMessages.Store(true)
	delivered := 0
	binding := &commandBinding{hub: hub, client: a, session: 1, cid: "123"}
	sender := binding.FrequencyMessageSender("121.905", func(event front.FrequencyMessageResultEvent) {
		require.Equal(t, "request-a", event.RequestID)
		delivered++
	})
	require.True(t, sender(es.SendFrequencyMessageEvent{RequestId: "request-a", Callsign: "SAS123", Message: "Hold position"}))
	handlers := shared.NewMessageHandlers[es.EventType, *Client]()
	handlers.Add(es.FrequencyMessageResult, handleFrequencyMessageResult)
	result := &es.FrequencyMessageResultEvent{RequestId: "request-a", Callsign: "SAS123", Submitted: true}
	require.Error(t, handlers.Handle(t.Context(), b, authorityMessage(t, es.FrequencyMessageResult, result, b.wireContext())), "same CID/currentmaster cannot forge another instance's result")
	old := a.wireContext()
	hub.setMasterClient(a)
	hub.setMasterClient(b)
	require.NoError(t, handlers.Handle(t.Context(), a, authorityMessage(t, es.FrequencyMessageResult, result, old)))
	require.Equal(t, 1, delivered)
	require.Error(t, handlers.Handle(t.Context(), a, authorityMessage(t, es.FrequencyMessageResult, result, old)))
}

func TestStaleRunwaysDoNotWriteAfterLosingAndRegainingMaster(t *testing.T) {
	hub := membershipTestHub()
	one := membershipTestRoot(hub, "123", "one")
	two := membershipTestRoot(hub, "456", "two")
	a := membershipTestMember(hub, one, 1, "AAAA", "LIVE", "a")
	b := membershipTestMember(hub, two, 1, "AAAA", "LIVE", "b")
	hub.master[1] = a
	old := a.wireContext()
	hub.setMasterClient(b)
	hub.setMasterClient(a)
	handlers := shared.NewMessageHandlers[es.EventType, *Client]()
	handlers.Add(es.RunwayType, handleRunways)
	require.Error(t, handlers.Handle(t.Context(), a, authorityMessage(t, es.RunwayType, &es.RunwayEvent{Runways: []*es.Runway{{Name: "22", Departure: true}}}, old)), "stale runway event must fail before touching the session repository")
}

func TestPrivateMessageIngressUsesOwnMembershipOnly(t *testing.T) {
	hub := membershipTestHub()
	one := membershipTestRoot(hub, "123", "one")
	two := membershipTestRoot(hub, "456", "two")
	own := membershipTestMember(hub, one, 1, "AAAA", "LIVE", "own")
	membershipTestMember(hub, two, 1, "AAAA", "LIVE", "master")
	handlers := shared.NewMessageHandlers[es.EventType, *Client]()
	handlers.AddAction(es.SendPrivateMessage, handleSendPrivateMessage)
	require.NoError(t, handlers.Handle(t.Context(), own, authorityMessage(t, es.SendPrivateMessage, &es.SendPrivateMessageEvent{Callsign: "SAS123", Message: "CONTACT OPS"}, own.wireContext())))
	require.Len(t, one.send, 1)
	require.Empty(t, two.send)
}

func TestHubQueueCapturesEpochBeforeFanout(t *testing.T) {
	hub := membershipTestHub()
	hub.send = make(chan internalMessage, 1)
	root := membershipTestRoot(hub, "123", "one")
	client := membershipTestMember(hub, root, 1, "AAAA", "LIVE", "a")
	hub.Broadcast(1, es.GroundStateEvent{Callsign: "SAS123", GroundState: "TAXI"})
	queued := <-hub.send
	hub.election(1).epoch.Add(1)
	require.True(t, client.enqueueAtEpoch(queued.message, queued.epoch, queued.fixedEpoch))
	data, err := (<-root.send).Marshal()
	require.NoError(t, err)
	var envelope es.Envelope
	require.NoError(t, proto.Unmarshal(data, &envelope))
	require.Zero(t, envelope.Context.MasterEpoch)
	require.Equal(t, uint64(1), client.wireContext().MasterEpoch)
}

func TestTypedResultValueAndSquawkDeadlineStayNarrow(t *testing.T) {
	hub := membershipTestHub()
	root := membershipTestRoot(hub, "123", "one")
	client := membershipTestMember(hub, root, 1, "AAAA", "LIVE", "a")
	id, err := client.registerExecution(es.GroundStateEvent{Callsign: "SAS123", GroundState: "PUSH"}, es.CommandOrigin_COMMAND_ORIGIN_HUMAN, 0)
	require.NoError(t, err)
	wrong, _ := proto.Marshal(&es.GroundStateEvent{Callsign: "SAS123", GroundState: "TAXI"})
	require.False(t, client.validateExecutionEcho("ground_state", wrong, es.CommandOrigin_COMMAND_ORIGIN_HUMAN, id))
	require.Less(t, time.Until(client.executions[id].deadline), 31*time.Second)
	id, err = client.registerExecution(es.GenerateSquawkEvent{Callsign: "SAS123"}, es.CommandOrigin_COMMAND_ORIGIN_HUMAN, 0)
	require.NoError(t, err)
	require.Greater(t, time.Until(client.executions[id].deadline), 119*time.Second)
	require.Less(t, time.Until(client.executions[id].deadline), 121*time.Second)
}
