package euroscope

import (
	"context"
	"sync"
	"testing"
	"time"

	"FlightStrips/internal/shared"
	es "FlightStrips/pkg/events/euroscope"
	"FlightStrips/pkg/models"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

type membershipWrites struct {
	noOpStripService
	mu       sync.Mutex
	sessions []int32
	human    []bool
	block    func(int32)
}

func (s *membershipWrites) UpdateGroundState(ctx context.Context, session int32, _, _, _ string) error {
	if s.block != nil {
		s.block(session)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions = append(s.sessions, session)
	_, human := shared.HumanCommand(ctx)
	s.human = append(s.human, human)
	return nil
}
func (s *membershipWrites) UpdateAssignedSquawk(ctx context.Context, session int32, _, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions = append(s.sessions, session)
	_, human := shared.HumanCommand(ctx)
	s.human = append(s.human, human)
	return nil
}
func authorityMessage(t *testing.T, kind es.EventType, event proto.Message, scope *es.MembershipContext) Message {
	t.Helper()
	payload, err := proto.Marshal(event)
	require.NoError(t, err)
	return Message{Type: kind, Message: payload, Scope: scope}
}
func authorityHandlers() shared.MessageHandlers[es.EventType, *Client] {
	handlers := shared.NewMessageHandlers[es.EventType, *Client]()
	handlers.Add(es.GroundState, handleGroundState)
	handlers.Add(es.AssignedSquawk, handleAssignedSquawk)
	handlers.Add(es.Sync, handleSync)
	return handlers
}

func TestMasterEpochsFenceProductionHandlersIndependently(t *testing.T) {
	hub := membershipTestHub()
	writes := &membershipWrites{}
	hub.stripService = writes
	root := membershipTestRoot(hub, "123", "one")
	a := membershipTestMember(hub, root, 1, "AAAA", "LIVE", "a")
	b := membershipTestMember(hub, root, 2, "BBBB", "LIVE", "b")
	c := membershipTestMember(hub, root, 3, "CCCC", "LIVE", "c")
	other := membershipTestRoot(hub, "123", "two")
	d := membershipTestMember(hub, other, 3, "CCCC", "LIVE", "d")
	// One physical instance is master for A/B and non-master for C, even with
	// another compatible instance authenticating as the same CID.
	require.Same(t, a, hub.getMasterClient(1))
	require.Same(t, b, hub.getMasterClient(2))
	require.Same(t, d, hub.getMasterClient(3))
	handlers := authorityHandlers()
	old := a.wireContext()
	event := &es.GroundStateEvent{Callsign: "SAS123", GroundState: "TAXI"}
	require.NoError(t, handlers.Handle(t.Context(), a, authorityMessage(t, es.GroundState, event, old)))
	require.NoError(t, handlers.Handle(t.Context(), b, authorityMessage(t, es.GroundState, event, b.wireContext())))
	require.Error(t, handlers.Handle(t.Context(), c, authorityMessage(t, es.GroundState, event, c.wireContext())))
	replacement := membershipTestMember(hub, other, 1, "AAAA", "LIVE", "replacement")
	hub.master[1] = a
	hub.setMasterClient(replacement)
	require.False(t, hub.MembershipReady(a))
	require.True(t, hub.MembershipReady(b))
	require.True(t, hub.IsSessionSynced(3))
	require.Error(t, handlers.Handle(t.Context(), a, authorityMessage(t, es.Sync, &es.SyncEvent{}, old)), "stale snapshot must be rejected before any inventory/DB access")
	hub.setMasterClient(a)
	require.Greater(t, a.wireContext().MasterEpoch, old.MasterEpoch)
	require.Error(t, handlers.Handle(t.Context(), a, authorityMessage(t, es.GroundState, event, old)), "regaining master must not revive queued work")
	require.NoError(t, handlers.Handle(t.Context(), a, authorityMessage(t, es.GroundState, event, a.wireContext())))
	require.Equal(t, []int32{1, 2, 1}, writes.sessions)
	foreign := a.wireContext()
	foreign.SessionId = 2
	require.Error(t, handlers.Handle(t.Context(), a, authorityMessage(t, es.GroundState, event, foreign)))
}

func TestSlowSessionApplicationDoesNotBlockAnotherAirportElection(t *testing.T) {
	hub := membershipTestHub()
	root := membershipTestRoot(hub, "123", "one")
	other := membershipTestRoot(hub, "456", "two")
	a := membershipTestMember(hub, root, 1, "AAAA", "LIVE", "a")
	b := membershipTestMember(hub, root, 2, "BBBB", "LIVE", "b")
	nextA := membershipTestMember(hub, other, 1, "AAAA", "LIVE", "next-a")
	nextB := membershipTestMember(hub, other, 2, "BBBB", "LIVE", "next-b")
	hub.master[1] = a
	hub.master[2] = b
	started := make(chan struct{})
	release := make(chan struct{})
	writes := &membershipWrites{block: func(session int32) {
		if session == 1 {
			close(started)
			<-release
		}
	}}
	hub.stripService = writes
	handlers := authorityHandlers()
	applied := make(chan error, 1)
	message := authorityMessage(t, es.GroundState, &es.GroundStateEvent{Callsign: "SAS123"}, a.wireContext())
	go func() { applied <- handlers.Handle(context.Background(), a, message) }()
	<-started
	changedA := make(chan struct{})
	go func() { hub.setMasterClient(nextA); close(changedA) }()
	changedB := make(chan struct{})
	go func() { hub.setMasterClient(nextB); close(changedB) }()
	select {
	case <-changedB:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("airport B election waited for airport A database work")
	}
	select {
	case <-changedA:
		t.Fatal("airport A changed authority during application")
	default:
	}
	close(release)
	require.NoError(t, <-applied)
	<-changedA
	require.Equal(t, uint64(1), hub.election(1).epoch.Load())
	require.Equal(t, uint64(1), hub.election(2).epoch.Load())
}

func TestOwnNonMasterTypedResultUsesExactBoundExecution(t *testing.T) {
	hub := membershipTestHub()
	writes := &membershipWrites{}
	hub.stripService = writes
	ownRoot := membershipTestRoot(hub, "123", "own")
	otherRoot := membershipTestRoot(hub, "123", "other")
	own := membershipTestMember(hub, ownRoot, 1, "AAAA", "LIVE", "own")
	master := membershipTestMember(hub, otherRoot, 1, "AAAA", "LIVE", "master")
	binding, err := hub.CaptureMembershipBinding(1, "123", "own", "own")
	require.NoError(t, err)
	require.NoError(t, binding.(*commandBinding).enqueue(es.GenerateSquawkEvent{Callsign: "SAS123"}))
	data, err := (<-ownRoot.send).Marshal()
	require.NoError(t, err)
	var command es.Envelope
	require.NoError(t, proto.Unmarshal(data, &command))
	require.NotEmpty(t, command.ExecutionId)
	require.Equal(t, es.CommandOrigin_COMMAND_ORIGIN_HUMAN, command.CommandOrigin)
	// The SDK result can follow an election. It remains the same human's exact
	// instance and membership; the nonce cannot authorize another flight/type.
	hub.setMasterClient(own)
	hub.setMasterClient(master)
	handlers := authorityHandlers()
	message := authorityMessage(t, es.AssignedSquawk, &es.AssignedSquawkEvent{Callsign: "OTHER", Squawk: "1234"}, command.Context)
	message.Origin = command.CommandOrigin
	message.ExecutionID = command.ExecutionId
	require.Error(t, handlers.Handle(t.Context(), own, message))
	message = authorityMessage(t, es.AssignedSquawk, &es.AssignedSquawkEvent{Callsign: "SAS123", Squawk: "1234"}, command.Context)
	message.Origin = command.CommandOrigin
	message.ExecutionID = "forged"
	require.Error(t, handlers.Handle(t.Context(), own, message))
	message.ExecutionID = command.ExecutionId
	require.NoError(t, handlers.Handle(t.Context(), own, message))
	require.Equal(t, []int32{1}, writes.sessions)
	require.Equal(t, []bool{true}, writes.human)
	require.Error(t, handlers.Handle(t.Context(), own, message), "nonce is a single typed result, not blanket write authority")
	// A fresh physical instance for this CID cannot inherit the old execution.
	replacement := membershipTestMember(hub, otherRoot, 1, "AAAA", "LIVE", "new")
	message.Scope = replacement.wireContext()
	require.Error(t, handlers.Handle(t.Context(), replacement, message))
}

func TestAutomaticExecutionAndExpiredNonceDoNotSurviveAuthorityChange(t *testing.T) {
	hub := membershipTestHub()
	root := membershipTestRoot(hub, "123", "one")
	other := membershipTestRoot(hub, "456", "two")
	client := membershipTestMember(hub, root, 1, "AAAA", "LIVE", "a")
	next := membershipTestMember(hub, other, 1, "AAAA", "LIVE", "next")
	hub.master[1] = client
	id, err := client.registerExecution(es.GenerateSquawkEvent{Callsign: "SAS123"}, es.CommandOrigin_COMMAND_ORIGIN_AUTOMATIC, 0)
	require.NoError(t, err)
	payload, _ := proto.Marshal(&es.AssignedSquawkEvent{Callsign: "SAS123", Squawk: "1234"})
	hub.setMasterClient(next)
	require.False(t, client.validateExecutionEcho("assigned_squawk", payload, es.CommandOrigin_COMMAND_ORIGIN_AUTOMATIC, id))
	id, err = client.registerExecution(es.GenerateSquawkEvent{Callsign: "SAS123"}, es.CommandOrigin_COMMAND_ORIGIN_HUMAN, 1)
	require.NoError(t, err)
	request := client.executions[id]
	request.deadline = time.Now().Add(-time.Second)
	client.executions[id] = request
	require.False(t, client.validateExecutionEcho("assigned_squawk", payload, es.CommandOrigin_COMMAND_ORIGIN_HUMAN, id))
	binding, err := hub.ResolveAutomaticCommandBinding(1, "")
	require.NoError(t, err)
	hub.setMasterClient(client)
	require.Error(t, binding.Validate(), "automatic deferred work must retain its election")
}

func TestRunwayStateAndSyncMasterStatusSeparateSameCIDInstances(t *testing.T) {
	hub := membershipTestHub()
	hub.runwayStates = map[string]*clientRunwayState{}
	one := membershipTestRoot(hub, "123", "one")
	two := membershipTestRoot(hub, "123", "two")
	master := membershipTestMember(hub, one, 1, "AAAA", "LIVE", "master")
	slave := membershipTestMember(hub, two, 1, "AAAA", "LIVE", "slave")
	hub.master[1] = master
	master.callsign = "SAME_TWR"
	slave.callsign = "SAME_TWR"
	current := models.ActiveRunways{DepartureRunways: []string{"04"}}
	different := models.ActiveRunways{DepartureRunways: []string{"22"}}
	hub.evaluateMembershipRunwayState(master, current, current, true)
	mismatch := hub.evaluateMembershipRunwayState(slave, different, current, false)
	require.True(t, mismatch.DepartureMismatch)
	require.Same(t, slave, mismatch.Target)
	hub.resyncSessionRunwayMismatchTargets(1, "123", current)
	require.True(t, hub.runwayStates[runwayStateKey(1, "slave")].DepartureMismatch)
	runtime := euroscopeSyncHubRuntime{hub: hub, client: slave}
	has, isMaster := runtime.CurrentMasterStatus(1, "123", "SAME_TWR")
	require.True(t, has)
	require.False(t, isMaster)
	hub.clearMembershipRunwayState(master)
	require.Len(t, hub.runwayStates, 1)
	require.Contains(t, hub.runwayStates, runwayStateKey(1, "slave"))
}
