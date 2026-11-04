package euroscope

import (
	"context"
	"testing"
	"time"

	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	es "FlightStrips/pkg/events/euroscope"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func membershipTestHub() *Hub {
	return &Hub{clients: map[*Client]bool{}, master: map[int32]*Client{}, airportClientCount: map[string]int{}, server: &testutil.MockServer{FrontendHubVal: &testutil.MockFrontendHub{}, ControllerRepoVal: &testutil.MockControllerRepository{SetCidFn: func(context.Context, int32, string, *string) (int64, error) { return 1, nil }}}}
}
func membershipTestRoot(hub *Hub, cid, instance string) *Client {
	root := authorityClient(hub, 0, cid)
	root.transport = &pluginTransport{root: root, instance: instance, generation: newGeneration(), members: map[string]*Client{}}
	if hub.transports == nil {
		hub.transports = map[*Client]bool{}
	}
	hub.transports[root] = true
	return root
}
func membershipTestMember(hub *Hub, root *Client, session int32, airport, environment, id string) *Client {
	c := authorityClient(hub, session, root.GetCid())
	c.transport = root.transport
	c.airport = airport
	c.sessionName = environment
	c.announced.Store(true)
	c.roleEligible.Store(true)
	c.membership = &es.MembershipContext{InstanceId: root.transport.instance, TransportGeneration: root.transport.generation, MembershipId: id, SessionId: session, Airport: airport, Environment: environment, Generation: newGeneration()}
	root.transport.members[id] = c
	hub.master[session] = c
	hub.clients[c] = true
	hub.syncedSessions.Store(session, struct{}{})
	return c
}

func TestMultiplexedRoutingRejectsWrongOrRemovedIncarnations(t *testing.T) {
	hub := membershipTestHub()
	root := membershipTestRoot(hub, "123", "instance-a")
	a := membershipTestMember(hub, root, 1, "AAAA", "LIVE", "a")
	b := membershipTestMember(hub, root, 2, "BBBB", "SWEATBOX_123", "b")
	for _, member := range []*Client{a, b} {
		got, err := hub.ResolveMessage(root, member.wireContext(), "sync")
		require.NoError(t, err)
		require.Same(t, member, got)
	}
	scope := a.wireContext()
	for _, mutate := range []func(*es.MembershipContext){func(c *es.MembershipContext) { c.InstanceId = "other" }, func(c *es.MembershipContext) { c.TransportGeneration++ }, func(c *es.MembershipContext) { c.SessionId = 2 }, func(c *es.MembershipContext) { c.Airport = "BBBB" }, func(c *es.MembershipContext) { c.Environment = "PLAYBACK_other" }, func(c *es.MembershipContext) { c.Generation++ }} {
		wrong := proto.Clone(scope).(*es.MembershipContext)
		mutate(wrong)
		_, err := hub.ResolveMessage(root, wrong, "sync")
		require.Error(t, err)
	}
	hub.RemoveMembership(a, "coverage removed")
	require.False(t, root.isClosed())
	require.False(t, b.isClosed())
	require.Len(t, root.transport.snapshot(), 1)
	applied := false
	require.Error(t, a.ApplyScopedMessage(t.Context(), scope, "sync", nil, es.CommandOrigin_COMMAND_ORIGIN_UNSPECIFIED, "", func(context.Context) error { applied = true; return nil }))
	require.False(t, applied)
	_, err := hub.ResolveMessage(root, scope, "sync")
	require.Error(t, err)
	require.NoError(t, b.ApplyScopedMessage(t.Context(), b.wireContext(), "sync", nil, es.CommandOrigin_COMMAND_ORIGIN_UNSPECIFIED, "", func(context.Context) error { applied = true; return nil }))
	require.True(t, applied)
}

func TestBindingsAddressExactInstanceMembershipAndNeverReconnect(t *testing.T) {
	hub := membershipTestHub()
	first := membershipTestRoot(hub, "123", "first")
	second := membershipTestRoot(hub, "123", "second")
	a := membershipTestMember(hub, first, 1, "AAAA", "LIVE", "a")
	b := membershipTestMember(hub, second, 1, "AAAA", "LIVE", "b")
	_, err := hub.CaptureCommandBinding(1, "123")
	require.Error(t, err)
	binding, err := hub.CaptureMembershipBinding(1, "123", "second", "b")
	require.NoError(t, err)
	_, err = hub.CaptureMembershipBinding(1, "unauthorized", "second", "b")
	require.Error(t, err)
	require.NoError(t, binding.Send(es.EobtEvent{Callsign: "SAS123", Eobt: "1200"}))
	require.Empty(t, first.send)
	wire, err := (<-second.send).Marshal()
	require.NoError(t, err)
	var envelope es.Envelope
	require.NoError(t, proto.Unmarshal(wire, &envelope))
	require.True(t, proto.Equal(b.membership, envelope.Context))
	hub.RemoveMembership(b, "removed")
	replacement := membershipTestMember(hub, second, 1, "AAAA", "LIVE", "replacement")
	require.Error(t, binding.Send(es.EobtEvent{Callsign: "SAS123"}))
	require.False(t, replacement.isClosed())
	require.False(t, a.isClosed())
	first.Close()
	require.Error(t, (&commandBinding{hub: hub, client: a, session: 1, cid: "123"}).Validate())
}

func TestMembershipApplicationSerializesRemoval(t *testing.T) {
	hub := membershipTestHub()
	root := membershipTestRoot(hub, "123", "instance")
	a := membershipTestMember(hub, root, 1, "AAAA", "LIVE", "a")
	started := make(chan struct{})
	release := make(chan struct{})
	applied := make(chan error, 1)
	go func() {
		applied <- a.ApplyScopedMessage(context.Background(), a.wireContext(), "sync", nil, es.CommandOrigin_COMMAND_ORIGIN_UNSPECIFIED, "", func(context.Context) error { close(started); <-release; return nil })
	}()
	<-started
	removed := make(chan struct{})
	go func() { hub.RemoveMembership(a, "removed"); close(removed) }()
	select {
	case <-removed:
		t.Fatal("membership removed during application")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	require.NoError(t, <-applied)
	<-removed
	require.Error(t, a.ApplyScopedMessage(t.Context(), a.wireContext(), "sync", nil, es.CommandOrigin_COMMAND_ORIGIN_UNSPECIFIED, "", func(context.Context) error { return nil }))
}

var _ shared.CommandBindingProvider = (*Hub)(nil)

func TestDirectSendCapturesOnlyPresentInstance(t *testing.T) {
	hub := membershipTestHub()
	hub.send = make(chan internalMessage, 4)
	hub.Send(1, "123", es.EobtEvent{Callsign: "SAS123"})
	require.Empty(t, hub.send, "absent CID must not enqueue a wildcard")
	root := membershipTestRoot(hub, "123", "instance")
	member := membershipTestMember(hub, root, 1, "AAAA", "LIVE", "a")
	hub.Send(1, "123", es.EobtEvent{Callsign: "SAS123"})
	queued := <-hub.send
	require.Same(t, member, queued.target)
	hub.RemoveMembership(member, "removed")
	replacement := membershipTestMember(hub, root, 1, "AAAA", "LIVE", "new")
	require.NotSame(t, replacement, queued.target)
	_, err := hub.ResolveMessage(root, replacement.wireContext(), "login")
	require.Error(t, err)
}

func TestDrainClosesZeroMembershipTransport(t *testing.T) {
	hub := membershipTestHub()
	root := membershipTestRoot(hub, "123", "instance")
	require.NoError(t, hub.DrainPositions(t.Context()))
	require.True(t, root.isClosed())
	require.True(t, hub.draining.Load())
}

type membershipDisconnectFrontend struct {
	testutil.MockFrontendHub
	disconnected []int32
}

func (f *membershipDisconnectFrontend) MembershipDisconnected(session int32, _ string) {
	f.disconnected = append(f.disconnected, session)
}
func TestObserverMasterLossIsSessionScoped(t *testing.T) {
	hub := membershipTestHub()
	frontend := &membershipDisconnectFrontend{}
	hub.server = &testutil.MockServer{FrontendHubVal: frontend}
	root := membershipTestRoot(hub, "123", "instance")
	a := membershipTestMember(hub, root, 1, "AAAA", "LIVE", "a")
	a.observer = true
	b := membershipTestMember(hub, root, 2, "BBBB", "LIVE", "b")
	b.observer = true
	hub.notifyObserverFrontendsOffline(1)
	require.Equal(t, []int32{1}, frontend.disconnected)
	require.False(t, b.isClosed())
}
