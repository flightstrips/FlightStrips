package euroscope

import (
	"github.com/stretchr/testify/require"
	"testing"
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
