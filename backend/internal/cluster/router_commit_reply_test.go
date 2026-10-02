package cluster

import (
	"context"
	"testing"

	pb "FlightStrips/pkg/events/cluster"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestCommittedOwnerReplyRequiresExactRequestAndOutcomeBinding(t *testing.T) {
	_, writer, ref, _ := ownerCommitProjectionFixture(t)
	request := command(ref, "committed", 0)
	hash, err := RequestHash(request)
	require.NoError(t, err)
	reply := writer.Execute(context.Background(), request)
	require.True(t, validCommittedReply(request, hash, reply))
	for _, test := range []struct {
		name string
		edit func(*pb.CommandReply)
	}{
		{"missing outcome", func(r *pb.CommandReply) { r.Outcome = nil }},
		{"foreign actor", func(r *pb.CommandReply) { r.Outcome.Actor.Id = "other" }},
		{"changed hash", func(r *pb.CommandReply) { r.Outcome.RequestSha256 = "other" }},
		{"foreign aggregate", func(r *pb.CommandReply) {
			r.Outcome.Aggregate = &pb.AggregateRef{Target: &pb.AggregateRef_Session{Session: &pb.SessionRef{Id: 99}}}
		}},
		{"foreign outcome command", func(r *pb.CommandReply) { r.Outcome.CommandId = "other" }},
		{"foreign reply command", func(r *pb.CommandReply) { r.CommandId = "other" }},
		{"unsupported protocol", func(r *pb.CommandReply) { r.ProtocolRevision = 2 }},
		{"missing sequence", func(r *pb.CommandReply) { r.StreamSequence = nil }},
		{"zero sequence", func(r *pb.CommandReply) { r.StreamSequence = proto.Uint64(0) }},
		{"changed sequence", func(r *pb.CommandReply) { r.StreamSequence = proto.Uint64(r.GetStreamSequence() + 1) }},
		{"missing revision", func(r *pb.CommandReply) { r.AggregateRevision = nil }},
		{"changed revision", func(r *pb.CommandReply) { r.AggregateRevision = proto.Uint64(r.GetAggregateRevision() + 1) }},
		{"pending mismatch", func(r *pb.CommandReply) { r.Status = pb.CommandReply_PENDING }},
		{"unknown outcome", func(r *pb.CommandReply) { r.Outcome.Status = pb.CommandOutcome_UNKNOWN }},
	} {
		t.Run(test.name, func(t *testing.T) {
			changed := proto.Clone(reply).(*pb.CommandReply)
			test.edit(changed)
			require.False(t, validCommittedReply(request, hash, changed))
		})
	}
}

func TestProjectedCommittedOutcomeCanReturnWhileGlobalReplayLags(t *testing.T) {
	store, writer, ref, p := ownerCommitProjectionFixture(t)
	request := command(ref, "local", 0)
	require.Equal(t, pb.CommandReply_COMMITTED, writer.Execute(context.Background(), request).Status)
	entry := store.entries[1]
	require.NoError(t, p.applyCommitted(entry, 1))
	p.highWater = 100
	hash, err := RequestHash(request)
	require.NoError(t, err)
	router := &CommandRouter{Projection: p}
	reply := router.projectedOutcome(request, hash)
	require.True(t, validCommittedReply(request, hash, reply))
	require.Equal(t, uint64(1), p.applied, "outcome retrieval must not advance raw replay")
	foreign := proto.Clone(request).(*pb.CommandRequest)
	foreign.Actor.Id = "other"
	require.Equal(t, pb.CommandReply_UNAUTHORIZED, router.projectedOutcome(foreign, hash).Status)
	p.positionReady = false
	require.Nil(t, router.projectedOutcome(request, hash), "known outcome must not bypass projection health")
}
