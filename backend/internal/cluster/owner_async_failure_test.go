package cluster

import (
	"context"
	"errors"
	"testing"
	"time"

	pb "FlightStrips/pkg/events/cluster"
	"github.com/stretchr/testify/require"
)

func TestOwnerAsyncFailureRevokesRenewalHealthForEveryTrackedSession(t *testing.T) {
	failure := errors.New("durable session queue frozen")
	queue := &AsyncSessionOwners{failure: failure}
	projection := &Projection{Async: queue}
	store := &memoryStore{}
	owner := &OwnerRuntime{Projection: projection, Store: store, NodeID: "node-a",
		tracked: map[string]*pb.AggregateRef{}, lastRenew: map[string]time.Time{}, failed: map[string]bool{}}
	for _, id := range []int32{1, 2} {
		ref := sessionRef(id)
		subject, err := Subject(ref)
		require.NoError(t, err)
		require.NoError(t, owner.Track(ref))
		owner.lastRenew[subject] = time.Now()
	}
	// Queue failure must be checked before touching transport/projection state.
	// The node must stop extending every session term, allowing a standby to
	// take over its durable prefix after the existing leases expire.
	owner.maintain(context.Background())
	require.Empty(t, owner.lastRenew)
	for subject := range owner.tracked {
		require.True(t, owner.failed[subject])
	}
	require.Zero(t, store.commits, "a failed queue must never publish another lease renewal")
	require.ErrorIs(t, queue.Err(), failure)
}
