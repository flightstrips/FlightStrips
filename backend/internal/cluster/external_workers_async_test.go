package cluster

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestExternalCallFlushesPriorSessionRAMAndCallsProviderOnce(t *testing.T) {
	owners, projection, ref, gate, store := asyncOwnersFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	require.NoError(t, owners.Execute(ctx, ref, func(turn context.Context) error {
		base, err := owners.Read(ref)
		if err != nil {
			return err
		}
		_, err = owners.AcceptState(turn, base, asyncDomainEvent(ref, 1))
		return err
	}))
	<-store.started
	require.True(t, owners.Pending(ref))
	writer := Writer{Store: store, Projection: projection, Lease: owners.owner, NodeID: owners.owner.NodeID}
	worker := ExternalCallWorker{Writer: writer}
	var calls atomic.Int32
	spec := ExternalCallSpec{Source: ref, Destination: ref, WorkflowID: uuid.NewString(), Step: "external/metar",
		Fetch: func(context.Context) (proto.Message, error) {
			calls.Add(1)
			// The irreversible call must follow both the earlier RAM tail and
			// the newly persisted intent, even with independent replay paused.
			durable, err := projection.readOwnedDurable(ref, owners.owner.NodeID)
			if err != nil {
				return nil, err
			}
			if len(durable.Workflows) == 0 || owners.Pending(ref) {
				return nil, fmt.Errorf("provider call preceded durable intent or prior RAM flush")
			}
			return &pb.WeatherObservation{Metar: "EKCH CAVOK"}, nil
		},
		Commit: func(run context.Context, id string, _ proto.Message) *pb.CommandReply {
			resultWriter := writer
			resultWriter.Plan = func(context.Context, *pb.CommandRequest, *Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
				return &pb.DomainChange{}, pb.CommandReply_COMMITTED, 0, nil
			}
			request := command(ref, "provider result", 0)
			request.CommandId = id
			reply := resultWriter.Execute(run, request)
			if reply.MemoryAccepted || reply.StreamSequence == nil {
				t.Error("provider completion returned before its durable result")
			}
			return reply
		}}
	type result struct {
		called bool
		err    error
	}
	done := make(chan result, 1)
	go func() { called, err := worker.Run(ctx, spec); done <- result{called, err} }()
	select {
	case completed := <-done:
		t.Fatalf("provider path passed blocked persistence: %+v", completed)
	case <-time.After(20 * time.Millisecond):
	}
	require.Zero(t, calls.Load(), "an unflushed RAM acceptance cannot authorize a provider call")
	close(gate)
	var completed result
	select {
	case completed = <-done:
	case <-ctx.Done():
		t.Fatal("provider path stalled after persistence resumed")
	}
	require.NoError(t, completed.err)
	require.True(t, completed.called)
	require.Equal(t, int32(1), calls.Load())
	called, err := worker.Run(ctx, spec)
	require.NoError(t, err)
	require.False(t, called)
	require.Equal(t, int32(1), calls.Load(), "retry of the same durable intent must not call the provider twice")
	require.NoError(t, owners.Drain(ctx))
}

func TestExternalCallFreshPermissionSurvivesLaterPendingRAM(t *testing.T) {
	owners, _, ref, gate, store := asyncOwnersFixture(t)
	close(gate)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	hold := make(chan struct{})
	writer := Writer{Store: store, Projection: owners.projection, Lease: owners.owner, NodeID: owners.owner.NodeID}
	worker := ExternalCallWorker{Writer: writer}
	var calls int
	spec := ExternalCallSpec{Source: ref, Destination: ref, WorkflowID: uuid.NewString(), Step: "external/hoppie/poll",
		Reserve: func(run context.Context) (bool, error) {
			err := owners.Execute(run, ref, func(turn context.Context) error {
				return owners.Append(turn, ref, func(context.Context) error { return nil }, func(work context.Context) error {
					select {
					case <-hold:
						return nil
					case <-work.Done():
						return work.Err()
					}
				})
			})
			return err == nil, err
		},
		Fetch: func(context.Context) (proto.Message, error) {
			calls++
			require.True(t, owners.Pending(ref), "test must exercise a later accepted RAM tail")
			require.False(t, owners.owner.CanWrite(ref), "pending work still gates new unsynchronized durable writes")
			close(hold)
			return &pb.WeatherObservation{Metar: "EKCH CAVOK"}, nil
		},
		Commit: func(run context.Context, id string, _ proto.Message) *pb.CommandReply {
			resultWriter := writer
			resultWriter.Plan = func(context.Context, *pb.CommandRequest, *Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
				return &pb.DomainChange{}, pb.CommandReply_COMMITTED, 0, nil
			}
			request := command(ref, "provider result", 0)
			request.CommandId = id
			return resultWriter.Execute(run, request)
		}}
	called, err := worker.Run(ctx, spec)
	require.NoError(t, err)
	require.True(t, called)
	called, err = worker.Run(ctx, spec)
	require.NoError(t, err)
	require.False(t, called)
	require.Equal(t, 1, calls)
	require.NoError(t, owners.Drain(ctx))
}

func TestDurableDispatchPermissionRetainsAuthorityAndHealthFences(t *testing.T) {
	for _, fault := range []string{"stale-renewal", "expired-lease", "foreign-owner", "disconnected", "replay-lag", "queue-failure"} {
		t.Run(fault, func(t *testing.T) {
			owners, p, ref, gate, _ := asyncOwnersFixture(t)
			close(gate)
			require.True(t, owners.owner.CanDispatchDurable(ref))
			key, _ := Subject(ref)
			switch fault {
			case "stale-renewal":
				owners.owner.lastRenew[key] = time.Now().Add(-ownerLease)
			case "expired-lease":
				p.states[key].Owner.LeaseUntil = timestamppb.New(time.Now().Add(-time.Second))
			case "foreign-owner":
				p.states[key].Owner.NodeId = "node-b"
				p.states[key].Owner.Epoch++
			case "disconnected":
				owners.owner.NC.Close()
			case "replay-lag":
				p.highWater++
			case "queue-failure":
				owners.Invalidate(ref, fmt.Errorf("queue failed"))
			}
			require.False(t, owners.owner.CanDispatchDurable(ref), "fresh permission cannot bypass %s", fault)
		})
	}
}
