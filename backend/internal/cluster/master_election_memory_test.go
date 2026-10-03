package cluster

import (
	pb "FlightStrips/pkg/events/cluster"
	"context"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	"testing"
	"time"
)

type memoryElectionLease struct{ healthy, pending bool }

func (o memoryElectionLease) CanWrite(*pb.AggregateRef) bool       { return o.healthy && !o.pending }
func (o memoryElectionLease) CanCommitLocal(*pb.AggregateRef) bool { return o.healthy }

func TestMasterElectionMemoryEligibility(t *testing.T) {
	ref := sessionRef(42)
	projection := &Projection{Async: NewAsyncSessionOwners(nil, nil, nil)}
	require.True(t, (MasterElection{Projection: projection, Lease: memoryElectionLease{healthy: true, pending: true}}).canPlan(ref))
	require.False(t, (MasterElection{Projection: projection, Lease: memoryElectionLease{healthy: false, pending: true}}).canPlan(ref))
	require.False(t, (MasterElection{Projection: projection, Lease: writableLease(true)}).canPlan(ref), "memory owner must support fresh local authority")
	require.False(t, (MasterElection{Projection: &Projection{}, Lease: memoryElectionLease{healthy: true, pending: true}}).canPlan(ref), "legacy election keeps durable guard")
	require.True(t, (MasterElection{Projection: &Projection{}, Lease: writableLease(true)}).canPlan(ref))
}

func TestMasterReconnectElectionDuringPendingMemoryReplication(t *testing.T) {
	const id int32 = 42
	ref := sessionRef(id)
	subject, _ := Subject(ref)
	store, p, _ := testSessionReplicas(t, id)
	gate := make(chan struct{})
	receipt := &asyncReceiptStore{memoryStore: store, gate: gate, started: make(chan struct{})}
	owner := &OwnerRuntime{NC: asyncConnectedTransport(t), Projection: p, NodeID: "node-a", lastRenew: map[string]time.Time{subject: time.Now()}, failed: map[string]bool{}}
	core := NewAsyncSessionOwners(p, owner, receipt)
	p.Async = core
	t.Cleanup(func() {
		close(gate)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		require.NoError(t, core.Drain(ctx))
	})
	writer := Writer{Store: receipt, Projection: p, Lease: owner, NodeID: "node-a", Plan: MasterElectionPlanner(p, PlanSessionObservations)}
	election := MasterElection{Projection: p, Router: electionTestRouter{writer: writer, store: store}, Lease: owner}
	original := &pb.ClientPresence{ConnectionId: uuid.NewString(), NodeId: "node-a", SessionId: id, Cid: "tower", Callsign: "EKCH_A_TWR", Kind: pb.ClientPresence_EUROSCOPE, ConnectedAt: timestamppb.Now()}
	putTestPresence(p, original)
	first, err := election.Reconcile(context.Background(), id)
	require.NoError(t, err)
	require.Equal(t, uint64(1), first.Epoch)
	require.True(t, core.Pending(ref))
	require.False(t, owner.CanWrite(ref), "durable publication remains blocked by queued election")
	reconnect := proto.Clone(original).(*pb.ClientPresence)
	reconnect.ConnectionId = uuid.NewString()
	reconnect.ConnectedAt = timestamppb.Now()
	p.mu.Lock()
	delete(p.presence, "client."+original.ConnectionId)
	p.mu.Unlock()
	putTestPresence(p, reconnect)
	next, err := election.Reconcile(context.Background(), id)
	require.NoError(t, err)
	require.Equal(t, uint64(2), next.Epoch)
	require.Equal(t, reconnect.ConnectionId, next.ConnectionId)
	require.True(t, core.Pending(ref))
	require.Equal(t, reconnect.ConnectionId, core.Control(ref).Master.ConnectionId)
	require.Nil(t, core.Control(ref).Sync, "new connection must not inherit prior sync")
	require.Error(t, p.RequireMasterInbound(id, original.ConnectionId, original.Cid, first.Epoch, false), "old connection is fenced immediately in RAM")
	require.Error(t, p.RequireMasterInbound(id, reconnect.ConnectionId, reconnect.Cid, next.Epoch, true), "new master must provide its own fresh sync")
	owner.mu.Lock()
	owner.failed[subject] = true
	owner.mu.Unlock()
	_, err = election.Reconcile(context.Background(), id)
	require.Error(t, err, "queued memory election cannot bypass stale owner authority")
	owner.mu.Lock()
	owner.failed[subject] = false
	owner.mu.Unlock()
}
