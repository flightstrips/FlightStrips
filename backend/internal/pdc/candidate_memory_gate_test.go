package pdc

import (
	"context"
	"testing"

	"FlightStrips/internal/cluster"
	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type pendingPolicyOwner struct{ healthy, pending bool }

func (o pendingPolicyOwner) CanWrite(*pb.AggregateRef) bool       { return o.healthy && !o.pending }
func (o pendingPolicyOwner) CanCommitLocal(*pb.AggregateRef) bool { return o.healthy }

type durableOnlyPolicyOwner struct{}

func (durableOnlyPolicyOwner) CanWrite(*pb.AggregateRef) bool { return true }

func TestPDCRAMPolicySchedulesPollWithoutGrantingProviderPermission(t *testing.T) {
	ref := pdcRef(42)
	owner := pendingPolicyOwner{healthy: true, pending: true}
	require.True(t, pdcOwnerCanPlan(owner, true, ref))
	require.False(t, owner.CanWrite(ref), "RAM eligibility cannot authorize an uncommitted provider intent")
	state := cluster.NewAggregate(ref)
	state.Indexes[pb.EntityKind_SESSION] = map[string]*pb.EntitySnapshot{"42": {Key: "42", Value: &pb.EntityRecord{Value: &pb.EntityRecord_Session{Session: &pb.Session{Id: 42, Airport: "EKCH", Name: "LIVE"}}}}}
	key := "pdc-poll.EKCH"
	deadline := &pb.SessionDeadline{Id: key, Kind: "pdc-poll", DueAt: timestamppb.Now(), CommandId: uuid.NewString(), SourceRevision: 1}
	request := pdcSystem(42, uuid.NewString(), "pdc-poll", proto.Uint64(0), pdcUpdate(key, &pb.EntityRecord{Value: &pb.EntityRecord_SessionDeadline{SessionDeadline: deadline}}))
	change, status, _, err := (&Candidate{}).planPoll(context.Background(), request, state)
	require.NoError(t, err)
	require.Equal(t, pb.CommandReply_COMMITTED, status)
	require.Len(t, change.Changes, 1)
	require.Equal(t, key, change.Changes[0].Key)
	require.Nil(t, state.Indexes[pb.EntityKind_SESSION_DEADLINE][key], "planning must leave the accepted view immutable")
	require.Empty(t, state.Workflows, "scheduling a poll must not manufacture a durable external-call permission")
}

func TestPDCPolicyEligibilityRejectsStaleAndUnsupportedOwners(t *testing.T) {
	ref := pdcRef(42)
	require.False(t, pdcOwnerCanPlan(pendingPolicyOwner{healthy: false, pending: true}, true, ref))
	require.False(t, pdcOwnerCanPlan(durableOnlyPolicyOwner{}, true, ref))
	require.False(t, pdcOwnerCanPlan(pendingPolicyOwner{healthy: true, pending: true}, false, ref))
	require.True(t, pdcOwnerCanPlan(durableOnlyPolicyOwner{}, false, ref))
}
