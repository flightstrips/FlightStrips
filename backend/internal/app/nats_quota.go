package app

import (
	"FlightStrips/internal/cluster"
	pb "FlightStrips/pkg/events/cluster"
	"context"
	"fmt"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	"math"
	"sort"
	"time"
)

// The global owner evaluates the reservation; the airport owner never replaces
// a quota from its private copy. A transport ambiguity grants no provider call.
func planNATSQuota(_ context.Context, req *pb.CommandRequest, state *cluster.Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
	quota := req.GetSystem().GetUpdateEntity().GetValue().GetProviderQuota()
	if req.Actor.GetKind() != pb.Actor_SYSTEM || req.Actor.Id != "provider-quota" || req.Aggregate.GetGlobal() == nil || quota == nil || quota.WindowStart == nil || quota.WindowStart.CheckValid() != nil || quota.Limit == 0 || quota.Used != 0 {
		return nil, pb.CommandReply_INVALID_ARGUMENT, 0, fmt.Errorf("invalid quota request")
	}
	key := fmt.Sprintf("%s.%d", quota.Provider, quota.WindowStart.AsTime().Unix())
	if quota.Provider != "metar" && quota.Provider != "openmeteo" || quota.Limit != 500 || !quota.WindowStart.AsTime().Equal(quota.WindowStart.AsTime().Truncate(time.Minute)) {
		return nil, pb.CommandReply_INVALID_ARGUMENT, 0, fmt.Errorf("unknown provider window")
	}
	if req.GetSystem().GetUpdateEntity().Key != key {
		return nil, pb.CommandReply_INVALID_ARGUMENT, 0, fmt.Errorf("quota key mismatch")
	}
	old := state.Indexes[pb.EntityKind_PROVIDER_QUOTA][key]
	revision, used := uint64(0), uint32(0)
	if old != nil {
		revision = old.Revision
		prior := old.Value.GetProviderQuota()
		if prior.Limit != quota.Limit {
			return nil, pb.CommandReply_INVALID_ARGUMENT, revision, fmt.Errorf("quota limit changed")
		}
		used = prior.Used
	}
	if used >= quota.Limit || used == math.MaxUint32 {
		return nil, pb.CommandReply_INVALID_ARGUMENT, revision, fmt.Errorf("provider quota exhausted")
	}
	value := proto.Clone(quota).(*pb.ProviderQuota)
	value.Used = used + 1
	changes := []*pb.EntityChange{{Key: key, Revision: revision + 1, Operation: &pb.EntityChange_Upsert{Upsert: &pb.EntityRecord{Value: &pb.EntityRecord_ProviderQuota{ProviderQuota: value}}}}}
	if quota.Provider == "openmeteo" {
		for _, window := range []struct {
			period time.Duration
			limit  uint32
			suffix string
		}{{time.Hour, 4000, "hour"}, {24 * time.Hour, 9000, "day"}} {
			at := quota.WindowStart.AsTime().UTC().Truncate(window.period)
			id := fmt.Sprintf("openmeteo-%s.%d", window.suffix, at.Unix())
			old := state.Indexes[pb.EntityKind_PROVIDER_QUOTA][id]
			used := old.GetValue().GetProviderQuota().GetUsed()
			if used >= window.limit {
				return nil, pb.CommandReply_INVALID_ARGUMENT, 0, fmt.Errorf("provider quota exhausted")
			}
			value := &pb.ProviderQuota{Provider: "openmeteo-" + window.suffix, WindowStart: timestamppb.New(at), Limit: window.limit, Used: used + 1}
			changes = append(changes, &pb.EntityChange{Key: id, Revision: old.GetRevision() + 1, Operation: &pb.EntityChange_Upsert{Upsert: &pb.EntityRecord{Value: &pb.EntityRecord_ProviderQuota{ProviderQuota: value}}}})
		}
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].Key < changes[j].Key })
	return &pb.DomainChange{Changes: changes}, pb.CommandReply_COMMITTED, revision, nil
}
func (r *natsRuntime) reserveQuota(ctx context.Context, id, provider string, window time.Time, limit uint32) (bool, error) {
	window = window.UTC().Truncate(time.Second)
	request := &pb.CommandRequest{ProtocolRevision: 1, CommandId: id, Aggregate: globalNATSRef(), Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "provider-quota"}, Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: &pb.UpdateEntity{Key: fmt.Sprintf("%s.%d", provider, window.Unix()), Value: &pb.EntityRecord{Value: &pb.EntityRecord_ProviderQuota{ProviderQuota: &pb.ProviderQuota{Provider: provider, WindowStart: timestamppb.New(window), Limit: limit}}}}}}}}
	state, err := r.projection.Read(request.Aggregate)
	if err != nil {
		return false, err
	}
	if prior := state.Ledger[id]; prior != nil {
		return false, nil
	}
	var reply *pb.CommandReply
	if state.Owner == nil {
		return false, fmt.Errorf("quota owner unavailable")
	}
	if state.Owner.NodeId == r.owner.NodeID {
		reply = r.freshQuota(ctx, request)
	} else {
		data, e := proto.Marshal(request)
		if e != nil {
			return false, e
		}
		hop, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		msg, e := r.nc.RequestWithContext(hop, "fs.v1.quota."+state.Owner.NodeId, data)
		if e != nil {
			return false, e
		}
		reply = &pb.CommandReply{}
		if len(msg.Data) > cluster.MaxStateBytes || pb.UnmarshalStrict(msg.Data, reply) != nil || reply.CommandId != id || reply.ProtocolRevision != 1 {
			return false, fmt.Errorf("invalid quota reply")
		}
	}
	if err = natsReply(reply); err != nil {
		return false, err
	}
	return reply.GetOutcome().GetStatus() == pb.CommandOutcome_SUCCEEDED, nil
}

func (r *natsRuntime) freshQuota(ctx context.Context, request *pb.CommandRequest) *pb.CommandReply {
	if r.closing.Load() || r.projection.Ready() != nil || !r.owner.CanWrite(globalNATSRef()) || request.GetAggregate().GetGlobal() == nil || request.GetActor().GetId() != "provider-quota" {
		return &pb.CommandReply{ProtocolRevision: 1, CommandId: request.GetCommandId(), Status: pb.CommandReply_UNAVAILABLE}
	}
	writer := r.router.Writer
	writer.Plan = planNATSQuota
	reply, fresh := writer.ExecuteFresh(ctx, request)
	if !fresh && reply.GetStatus() == pb.CommandReply_COMMITTED && reply.GetOutcome().GetStatus() == pb.CommandOutcome_SUCCEEDED {
		return &pb.CommandReply{ProtocolRevision: 1, CommandId: request.CommandId, Status: pb.CommandReply_UNAVAILABLE}
	}
	return reply
}
func (r *natsRuntime) serveQuota(ctx context.Context) error {
	_, closeSub, err := cluster.SubscribeJoined(r.nc, "fs.v1.quota."+r.owner.NodeID, func(msg *nats.Msg) {
		request := &pb.CommandRequest{}
		if len(msg.Data) == 0 || len(msg.Data) > cluster.MaxStateBytes || pb.UnmarshalStrict(msg.Data, request) != nil {
			return
		}
		work, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		reply := r.freshQuota(work, request)
		data, err := proto.Marshal(reply)
		if err == nil && msg.Reply != "" {
			_ = r.nc.Publish(msg.Reply, data)
		}
	})
	if err != nil {
		return err
	}
	defer closeSub()
	if err = r.nc.FlushWithContext(ctx); err != nil {
		return err
	}
	<-ctx.Done()
	return ctx.Err()
}
