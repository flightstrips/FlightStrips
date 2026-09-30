package cluster

import (
	"context"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	pb "FlightStrips/pkg/events/cluster"
	"google.golang.org/protobuf/proto"
)

// AtisSessionAdapter publishes one session presentation derived from a typed
// global ATIS feed and an airport-owned METAR cache.
type AtisSessionAdapter struct{ Writer Writer }

func atisRevision(value string) (uint64, int64, error) {
	parts := strings.Split(value, ":")
	if len(parts) != 3 || len(parts[0]) != 20 || len(parts[1]) != 64 || len(parts[2]) != 20 {
		return 0, 0, fmt.Errorf("invalid ATIS source revision")
	}
	if _, err := hex.DecodeString(parts[1]); err != nil {
		return 0, 0, err
	}
	feed, err := strconv.ParseUint(parts[0], 10, 64)
	if err != nil || feed == 0 {
		return 0, 0, fmt.Errorf("invalid ATIS feed revision")
	}
	metar, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil || metar < 0 {
		return 0, 0, fmt.Errorf("invalid METAR source time")
	}
	return feed, metar, nil
}

func (a AtisSessionAdapter) Apply(ctx context.Context, sessionID int32, value *pb.Atis) *pb.CommandReply {
	if value == nil || value.Airport == "" || value.ObservedAt == nil || value.ObservedAt.CheckValid() != nil {
		return &pb.CommandReply{Status: pb.CommandReply_INVALID_ARGUMENT}
	}
	if _, _, err := atisRevision(value.SourceRevision); err != nil {
		return &pb.CommandReply{Status: pb.CommandReply_INVALID_ARGUMENT, Detail: err.Error()}
	}
	id, err := ProviderEventCommandID("atis-presentation", "afv", fmt.Sprintf("%d/%s/%s", sessionID, value.Airport, value.SourceRevision))
	if err != nil {
		return &pb.CommandReply{Status: pb.CommandReply_INVALID_ARGUMENT}
	}
	request := &pb.CommandRequest{ProtocolRevision: 1, CommandId: id, Aggregate: sessionRef(sessionID), Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "atis-presentation"},
		Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: &pb.UpdateEntity{Key: value.Airport, Value: &pb.EntityRecord{Value: &pb.EntityRecord_Atis{Atis: value}}}}}}}
	w := a.Writer
	w.Plan = func(_ context.Context, _ *pb.CommandRequest, state *Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
		session := state.Indexes[pb.EntityKind_SESSION][strconv.FormatInt(int64(sessionID), 10)]
		if session == nil || session.GetValue().GetSession().Airport != value.Airport {
			return nil, pb.CommandReply_INVALID_ARGUMENT, 0, fmt.Errorf("ATIS session airport mismatch")
		}
		old := state.Indexes[pb.EntityKind_ATIS][value.Airport]
		if old != nil {
			oldValue := old.GetValue().GetAtis()
			oldFeed, oldMetar, err := atisRevision(oldValue.SourceRevision)
			if err != nil {
				return nil, pb.CommandReply_UNAVAILABLE, old.Revision, fmt.Errorf("corrupt ATIS source revision")
			}
			feed, metar, _ := atisRevision(value.SourceRevision)
			if oldFeed > feed || oldFeed == feed && oldMetar > metar || oldFeed == feed && oldMetar == metar && oldValue.SourceRevision != value.SourceRevision {
				return nil, pb.CommandReply_REVISION_CONFLICT, old.Revision, fmt.Errorf("newer ATIS source already applied")
			}
			if proto.Equal(oldValue, value) {
				return &pb.DomainChange{}, pb.CommandReply_COMMITTED, old.Revision, nil
			}
		}
		return &pb.DomainChange{Changes: []*pb.EntityChange{candidateUpsert(value.Airport, old, &pb.EntityRecord{Value: &pb.EntityRecord_Atis{Atis: value}})}}, pb.CommandReply_COMMITTED, 0, nil
	}
	return w.Execute(ctx, request)
}
