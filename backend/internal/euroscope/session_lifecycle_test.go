package euroscope

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSessionExpiryKeepsControlTransportAndOtherMembershipDemand(t *testing.T) {
	hub := membershipTestHub()
	root := membershipTestRoot(hub, "123", "waiting")
	root.transport.evidence.Environment = "LIVE"
	membershipTestMember(hub, root, 1, "AAAA", "LIVE", "a")
	membershipTestMember(hub, root, 2, "BBBB", "LIVE", "b")
	a, cancelA := context.WithCancel(t.Context())
	b, cancelB := context.WithCancel(t.Context())
	defer cancelB()
	hub.offlineTimers = map[string]*offlineTimerEntry{"a": {session: 1, cancel: cancelA}, "b": {session: 2, cancel: cancelB}}
	hub.syncedSessions.Store(int32(1), true)
	hub.syncedSessions.Store(int32(2), true)
	hub.RemoveSession(1)
	require.Len(t, root.transport.snapshot(), 1)
	require.True(t, hub.HasLiveSourceDemand())
	require.ErrorIs(t, a.Err(), context.Canceled)
	require.NoError(t, b.Err())
	_, present := hub.syncedSessions.Load(int32(2))
	require.True(t, present)
	_, present = hub.syncedSessions.Load(int32(1))
	require.False(t, present)
	hub.RemoveSession(2)
	require.Empty(t, root.transport.snapshot())
	require.True(t, hub.HasLiveSourceDemand(), "zero-member authenticated LIVE control still demands shared acquisition")
	root.Close()
	require.False(t, hub.HasLiveSourceDemand())
}
