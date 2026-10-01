package app

import (
	"context"
	"fmt"
	"testing"
	"time"

	"FlightStrips/internal/cluster"
	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestQuotaRetiresExpiredWindowsWithoutResettingCurrentLimits(t *testing.T) {
	at := time.Now().UTC().Truncate(time.Minute)
	ref := globalNATSRef()
	state := cluster.NewAggregate(ref)
	state.Indexes[pb.EntityKind_PROVIDER_QUOTA] = map[string]*pb.EntitySnapshot{}
	add := func(provider string, at time.Time, limit, used uint32) string {
		key := fmt.Sprintf("%s.%d", provider, at.Unix())
		entity := &pb.EntitySnapshot{Key: key, Revision: 7, Value: &pb.EntityRecord{Value: &pb.EntityRecord_ProviderQuota{ProviderQuota: &pb.ProviderQuota{Provider: provider, WindowStart: timestamppb.New(at), Limit: limit, Used: used}}}}
		state.Entities[key] = entity
		state.Indexes[pb.EntityKind_PROVIDER_QUOTA][key] = entity
		return key
	}
	old := add("openmeteo", at.Add(-72*time.Hour), 500, 499)
	hour := add("openmeteo-hour", at.Truncate(time.Hour), 4000, 3998)
	day := add("openmeteo-day", at.Truncate(24*time.Hour), 9000, 8998)
	key := fmt.Sprintf("openmeteo.%d", at.Unix())
	req := &pb.CommandRequest{ProtocolRevision: 1, CommandId: uuid.NewString(), Aggregate: ref, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "provider-quota"}, Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: &pb.UpdateEntity{Key: key, Value: &pb.EntityRecord{Value: &pb.EntityRecord_ProviderQuota{ProviderQuota: &pb.ProviderQuota{Provider: "openmeteo", WindowStart: timestamppb.New(at), Limit: 500}}}}}}}}
	change, status, _, err := planNATSQuota(context.Background(), req, state)
	require.NoError(t, err)
	require.Equal(t, pb.CommandReply_COMMITTED, status)
	changes := map[string]*pb.EntityChange{}
	for _, entry := range change.Changes {
		changes[entry.Key] = entry
	}
	require.Equal(t, pb.EntityKind_PROVIDER_QUOTA, changes[old].GetDelete().Kind)
	require.Equal(t, uint64(8), changes[old].Revision)
	require.Equal(t, uint32(3999), changes[hour].GetUpsert().GetProviderQuota().Used)
	require.Equal(t, uint32(8999), changes[day].GetUpsert().GetProviderQuota().Used)
	// A new ID cannot reopen a retired window and regain quota.
	req.GetSystem().GetUpdateEntity().Key = old
	req.GetSystem().GetUpdateEntity().GetValue().GetProviderQuota().WindowStart = timestamppb.New(at.Add(-72 * time.Hour))
	_, status, _, err = planNATSQuota(context.Background(), req, state)
	require.Error(t, err)
	require.Equal(t, pb.CommandReply_INVALID_ARGUMENT, status)
}
