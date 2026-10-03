package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"FlightStrips/internal/cluster"
	"FlightStrips/internal/natsresources"
	pb "FlightStrips/pkg/events/cluster"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
)

type snapshotLookupGate struct {
	calls atomic.Int64
	ready chan struct{}
}
type gatedSnapshotIndex struct {
	nats.KeyValue
	ctx  context.Context
	gate *snapshotLookupGate
}

func (k gatedSnapshotIndex) Get(key string) (nats.KeyValueEntry, error) {
	entry, err := k.KeyValue.Get(key)
	count := k.gate.calls.Add(1)
	if count <= 16 {
		if count == 16 {
			close(k.gate.ready)
		}
		select {
		case <-k.gate.ready:
		case <-k.ctx.Done():
			return nil, k.ctx.Err()
		}
	}
	return entry, err
}

// Concurrent independent replica checkpoints must never overwrite an object.
// This uses the unchanged backend ACL, which denies object-stream purge APIs.
func TestServerNATSConcurrentImmutableSnapshots(t *testing.T) {
	if os.Getenv("NATS_TASK22") != "1" {
		t.Skip("requires explicit disposable Task22 fixture")
	}
	f := newEntrypointFixture(t, true)
	_, ref, _ := f.seededSession()
	state := f.state(ref)
	nc, err := natsresources.Connect(f.projection.Config)
	require.NoError(t, err)
	defer nc.Close()
	js, err := nc.JetStream(nats.MaxWait(2 * time.Second))
	require.NoError(t, err)
	index, err := js.KeyValue(f.projection.Config.Names.SnapshotIndex)
	require.NoError(t, err)
	objects, err := js.ObjectStore(f.projection.Config.Names.Objects)
	require.NoError(t, err)
	// Start without a retained pointer, then hold all initial lookups until
	// every publisher observed the same missing pointer. This reproduces the
	// cross-replica check-then-put race under the real backend permissions.
	require.NoError(t, index.Delete(fmt.Sprintf("session.%d", ref.GetSession().Id)))
	gate := &snapshotLookupGate{ready: make(chan struct{})}
	stores := []cluster.SnapshotStore{
		{Index: gatedSnapshotIndex{KeyValue: f.projection.Snapshots.Index, ctx: f.ctx, gate: gate}, Objects: f.projection.Snapshots.Objects},
		{Index: gatedSnapshotIndex{KeyValue: index, ctx: f.ctx, gate: gate}, Objects: objects},
	}
	start := make(chan struct{})
	failures := make(chan error, 16)
	var jobs sync.WaitGroup
	for i := 0; i < 16; i++ {
		jobs.Add(1)
		go func(store cluster.SnapshotStore) { defer jobs.Done(); <-start; failures <- store.Save(state) }(stores[i%len(stores)])
	}
	close(start)
	jobs.Wait()
	close(failures)
	for err := range failures {
		require.NoError(t, err)
	}
	retained, err := index.Get(fmt.Sprintf("session.%d", ref.GetSession().Id))
	require.NoError(t, err)
	pointer := &pb.SnapshotIndex{}
	require.NoError(t, pb.UnmarshalStrict(retained.Value(), pointer))
	require.Contains(t, pointer.ObjectName, fmt.Sprintf("snapshot/session/%d/", ref.GetSession().Id))
	require.Equal(t, state.StreamSequence, pointer.LastStreamSequence)
	restored, err := stores[1].Load(ref)
	require.NoError(t, err)
	require.Equal(t, state.StreamSequence, restored.StreamSequence)
	require.Equal(t, state.Revision, restored.Revision)
	for _, broker := range f.brokers {
		require.NotContains(t, broker.log.text(), "Publish Violation - Subject \"$JS.API.STREAM.PURGE.OBJ_FS_OBJECTS\"")
	}
	for _, app := range f.apps {
		require.NotContains(t, app.log.text(), "Permissions Violation for Publish to \"$JS.API.STREAM.PURGE.OBJ_FS_OBJECTS\"")
	}
	require.False(t, strings.HasSuffix(pointer.ObjectName, "/"))
}
