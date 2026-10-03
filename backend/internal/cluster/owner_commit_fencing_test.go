package cluster

import (
	"strings"
	"testing"
	"time"

	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// A successful conditional publish is not proof that its domain fact is
// effective. Local PubAck application must use this same server-time reducer.
func TestOwnerCommittedFactLeaseBoundaryAndTakeoverOrder(t *testing.T) {
	for _, takeoverFirst := range []bool{false, true} {
		name := "command-before-takeover"
		if takeoverFirst {
			name = "takeover-before-command"
		}
		t.Run(name, func(t *testing.T) {
			ref := &pb.AggregateRef{Target: &pb.AggregateRef_Global{Global: &pb.GlobalRef{}}}
			a := NewAggregate(ref)
			at := time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC)
			apply := func(event *pb.StateEvent, serverTime time.Time) bool {
				t.Helper()
				data, err := proto.Marshal(event)
				require.NoError(t, err)
				sequence := a.StreamSequence + 1
				effective, err := a.Apply(AppliedEvent{Subject: "fs.v1.state.global", StreamSequence: sequence,
					SubjectSequence: sequence, ServerTime: serverTime, Data: data})
				require.NoError(t, err)
				require.Equal(t, sequence, a.StreamSequence, "even fenced facts advance the durable subject checkpoint")
				return effective
			}
			control := func(node string, epoch uint64, renew bool) *pb.StateEvent {
				e := &pb.StateEvent{SchemaVersion: 1, EventId: uuid.NewString(), Aggregate: ref,
					AggregateRevision: a.Revision, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: node}}
				term := &pb.OwnerTerm{NodeId: node, Epoch: epoch}
				if renew {
					e.Fact = &pb.StateEvent_OwnerRenewed{OwnerRenewed: term}
				} else {
					e.Fact = &pb.StateEvent_OwnerClaimed{OwnerClaimed: term}
				}
				return e
			}
			domain := func(epoch uint64) *pb.StateEvent {
				id := uuid.NewString()
				actor := &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "test"}
				return &pb.StateEvent{SchemaVersion: 1, EventId: uuid.NewString(), Aggregate: ref, CommandId: &id,
					AggregateRevision: a.Revision + 1, OwnerEpoch: epoch, Actor: actor,
					Fact: &pb.StateEvent_DomainChanged{DomainChanged: &pb.DomainChange{Outcome: &pb.CommandOutcome{
						CommandId: id, RequestSha256: strings.Repeat("a", 64), Actor: actor,
						Status: pb.CommandOutcome_SUCCEEDED, Aggregate: ref, AggregateRevision: a.Revision + 1}}}}
			}
			require.True(t, apply(control("node-a", 1, false), at))
			boundary := a.Owner.LeaseUntil.AsTime()
			require.False(t, apply(control("node-b", 2, false), boundary.Add(-time.Nanosecond)))
			require.False(t, apply(control("node-a", 1, true), boundary), "renewal cannot resurrect an expired lease")
			command := domain(1)
			if takeoverFirst {
				require.True(t, apply(control("node-b", 2, false), boundary))
				require.False(t, apply(command, boundary), "old epoch is fenced even with the same server timestamp")
				require.Nil(t, a.Ledger[command.GetCommandId()])
				require.Zero(t, a.Revision)
			} else {
				require.True(t, apply(command, boundary))
				require.NotNil(t, a.Ledger[command.GetCommandId()])
				require.True(t, apply(control("node-b", 2, false), boundary))
			}
			require.Equal(t, "node-b", a.Owner.NodeId)
			require.False(t, apply(control("node-a", 1, true), boundary.Add(time.Second)))
			stale := domain(1)
			require.False(t, apply(stale, boundary.Add(time.Second)))
			require.Nil(t, a.Ledger[stale.GetCommandId()])
			fresh := domain(2)
			require.True(t, apply(fresh, boundary.Add(time.Second)))
			require.NotNil(t, a.Ledger[fresh.GetCommandId()])
		})
	}
}
