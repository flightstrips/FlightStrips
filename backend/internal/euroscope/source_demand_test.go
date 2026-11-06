package euroscope

import (
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestLiveControlDemandSurvivesZeroMemberships(t *testing.T) {
	hub := membershipTestHub()
	root := membershipTestRoot(hub, "123", "waiting")
	require.False(t, hub.HasLiveSourceDemand())
	root.transport.evidence.Environment = "SWEATBOX_123"
	require.False(t, hub.HasLiveSourceDemand())
	root.transport.evidence.Environment = "LIVE"
	a := membershipTestMember(hub, root, 1, "AAAA", "LIVE", "a")
	b := membershipTestMember(hub, root, 2, "BBBB", "LIVE", "b")
	require.True(t, hub.HasLiveSourceDemand())
	hub.RemoveMembership(a, "coverage removed")
	hub.RemoveMembership(b, "coverage removed")
	require.Empty(t, root.transport.snapshot())
	require.True(t, hub.HasLiveSourceDemand())
	root.Close()
	require.False(t, hub.HasLiveSourceDemand())
}

func TestLiveDemandWaitDoesNotHoldRegistryAgainstMembershipJoin(t *testing.T) {
	hub := membershipTestHub()
	root := membershipTestRoot(hub, "123", "joining")
	root.transport.evidence.Environment = "LIVE"
	root.transport.mu.Lock()
	demand := make(chan bool, 1)
	go func() { demand <- hub.HasLiveSourceDemand() }()
	registered := make(chan struct{})
	go func() { hub.clientsMu.Lock(); hub.clientsMu.Unlock(); close(registered) }()
	select {
	case <-registered:
	case <-time.After(time.Second):
		root.transport.mu.Unlock()
		t.Fatal("source demand holds the global registry while waiting for a joining transport")
	}
	root.transport.mu.Unlock()
	require.True(t, <-demand)
}
