package cluster

import (
	"context"
	"sync"
	"testing"
	"time"

	pb "FlightStrips/pkg/events/cluster"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestCommandTransportRequiresRAMReplyOptIn(t *testing.T) {
	for _, test := range []struct {
		name, capability, durable string
		memory                    bool
	}{
		{name: "legacy ingress"},
		{name: "unrecognized capability", capability: "future"},
		{name: "RAM capable ingress", capability: "1", memory: true},
		{name: "durable overrides RAM capability", capability: "1", durable: "1"},
	} {
		t.Run(test.name, func(t *testing.T) {
			owners, projection, ref, gate, store := asyncOwnersFixture(t)
			var release sync.Once
			t.Cleanup(func() { release.Do(func() { close(gate) }) })
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			writer := Writer{Store: store, Projection: projection, Lease: owners.owner, NodeID: owners.owner.NodeID,
				Plan: func(context.Context, *pb.CommandRequest, *Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
					return &pb.DomainChange{}, pb.CommandReply_COMMITTED, 0, nil
				}}
			router := &CommandRouter{NC: owners.owner.NC, Projection: projection, Lease: owners.owner, Writer: writer}
			request := command(ref, "transport compatibility", 0)
			data, err := proto.Marshal(request)
			require.NoError(t, err)
			msg := nats.NewMsg("unit-command")
			msg.Data = data
			msg.Header.Set("FS-Memory-Accepted", test.capability)
			msg.Header.Set("FS-Durable", test.durable)
			done := make(chan *pb.CommandReply, 1)
			go func() { done <- router.handleMessage(ctx, msg) }()
			var reply *pb.CommandReply
			if test.memory {
				select {
				case reply = <-done:
				case <-ctx.Done():
					t.Fatal("RAM-capable caller waited for persistence")
				}
				require.True(t, reply.MemoryAccepted)
				require.Nil(t, reply.StreamSequence)
			} else {
				select {
				case <-store.started:
				case <-ctx.Done():
					t.Fatal("durable command did not reach persistence")
				}
				select {
				case reply = <-done:
					t.Fatalf("legacy/durable caller received a reply before persistence: %v", reply)
				case <-time.After(20 * time.Millisecond):
				}
			}
			release.Do(func() { close(gate) })
			if reply == nil {
				select {
				case reply = <-done:
				case <-ctx.Done():
					t.Fatal("durable caller did not complete after persistence")
				}
				require.False(t, reply.MemoryAccepted)
				require.NotZero(t, reply.GetStreamSequence())
			}
			require.Equal(t, pb.CommandReply_COMMITTED, reply.Status)
			require.Equal(t, pb.CommandOutcome_SUCCEEDED, reply.GetOutcome().GetStatus())
			require.NoError(t, owners.Drain(ctx))
		})
	}
}
