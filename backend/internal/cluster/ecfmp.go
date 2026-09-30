package cluster

import (
	"context"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"

	pb "FlightStrips/pkg/events/cluster"
	"google.golang.org/protobuf/proto"
)

// EcfmpSessionAdapter applies a committed global feed generation on the
// session owner. No provider request is made by this adapter.
type EcfmpSessionAdapter struct{ Writer Writer }

func (a EcfmpSessionAdapter) Strips(ctx context.Context, sessionID int32) ([]*pb.Strip, error) {
	ref := sessionRef(sessionID)
	subject, err := Subject(ref)
	if err != nil {
		return nil, err
	}
	state, err := a.Writer.load(ctx, subject, ref)
	if err != nil {
		return nil, err
	}
	var strips []*pb.Strip
	for _, entry := range state.EntitiesByKind(pb.EntityKind_STRIP) {
		strips = append(strips, proto.Clone(entry.GetValue().GetStrip()).(*pb.Strip))
	}
	sort.Slice(strips, func(i, j int) bool { return strips[i].Callsign < strips[j].Callsign })
	return strips, nil
}

func ecfmpRevision(version string) (uint64, error) {
	parts := strings.SplitN(version, ":", 2)
	if len(parts) != 2 || len(parts[0]) != 20 || len(parts[1]) != 64 {
		return 0, fmt.Errorf("invalid ECFMP source revision")
	}
	if _, err := hex.DecodeString(parts[1]); err != nil {
		return 0, fmt.Errorf("invalid ECFMP source digest")
	}
	value, err := strconv.ParseUint(parts[0], 10, 64)
	if err != nil || value == 0 {
		return 0, fmt.Errorf("invalid ECFMP source revision")
	}
	return value, nil
}

// Apply rejects an evaluated result when its strip changed, and rejects a
// source older than the session's last applied global checkpoint revision.
func (a EcfmpSessionAdapter) Apply(ctx context.Context, sessionID int32, strip *pb.Strip, sourceRevision string, restrictions []*pb.EcfmpRestriction) *pb.CommandReply {
	if strip == nil || strip.Callsign == "" || strip.Revision == 0 {
		return &pb.CommandReply{Status: pb.CommandReply_INVALID_ARGUMENT}
	}
	if _, err := ecfmpRevision(sourceRevision); err != nil {
		return &pb.CommandReply{Status: pb.CommandReply_INVALID_ARGUMENT, Detail: err.Error()}
	}
	id, err := ProviderEventCommandID("ecfmp-application", "ecfmp", fmt.Sprintf("%d/%s/%d/%s", sessionID, strip.Callsign, strip.Revision, sourceRevision))
	if err != nil {
		return &pb.CommandReply{Status: pb.CommandReply_INVALID_ARGUMENT}
	}
	value := &pb.EcfmpState{Callsign: strip.Callsign, SourceRevision: sourceRevision, Restrictions: restrictions}
	request := &pb.CommandRequest{ProtocolRevision: 1, CommandId: id, Aggregate: sessionRef(sessionID), Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "ecfmp-application"}, Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: &pb.UpdateEntity{Key: strip.Callsign, Value: &pb.EntityRecord{Value: &pb.EntityRecord_EcfmpState{EcfmpState: value}}}}}}}
	w := a.Writer
	w.Plan = func(_ context.Context, _ *pb.CommandRequest, state *Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
		currentStrip := state.Indexes[pb.EntityKind_STRIP][strip.Callsign]
		if currentStrip == nil || currentStrip.Revision != strip.Revision || !proto.Equal(currentStrip.GetValue().GetStrip(), strip) {
			return nil, pb.CommandReply_REVISION_CONFLICT, 0, fmt.Errorf("strip changed during ECFMP application")
		}
		old := state.Indexes[pb.EntityKind_ECFMP_STATE][strip.Callsign]
		if old != nil {
			oldVersion, err := ecfmpRevision(old.GetValue().GetEcfmpState().SourceRevision)
			newVersion, _ := ecfmpRevision(sourceRevision)
			if err != nil {
				return nil, pb.CommandReply_UNAVAILABLE, old.Revision, fmt.Errorf("corrupt ECFMP source revision")
			}
			if oldVersion > newVersion || oldVersion == newVersion && old.GetValue().GetEcfmpState().SourceRevision != sourceRevision {
				return nil, pb.CommandReply_REVISION_CONFLICT, old.Revision, fmt.Errorf("newer ECFMP source already applied")
			}
			if proto.Equal(old.GetValue().GetEcfmpState(), value) {
				return &pb.DomainChange{}, pb.CommandReply_COMMITTED, old.Revision, nil
			}
		}
		return &pb.DomainChange{Changes: []*pb.EntityChange{candidateUpsert(strip.Callsign, old, &pb.EntityRecord{Value: &pb.EntityRecord_EcfmpState{EcfmpState: value}})}}, pb.CommandReply_COMMITTED, 0, nil
	}
	return w.Execute(ctx, request)
}
