package services

import (
	"context"
	"fmt"
	"strings"

	"FlightStrips/internal/cluster"
	pb "FlightStrips/pkg/events/cluster"
	"google.golang.org/protobuf/proto"
)

// Browser EOBT edits arrive with other strip fields. Plan both policies under
// the original command, preserving its strip revision and accompanying edits.
func (c *CdmCandidate) planStripEobt(ctx context.Context, request *pb.CommandRequest, state *cluster.Aggregate, next cluster.Planner) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
	if next == nil {
		return nil, pb.CommandReply_UNAVAILABLE, 0, fmt.Errorf("strip planner unavailable")
	}
	base, status, current, err := next(ctx, request, state)
	if err != nil || status != pb.CommandReply_COMMITTED || base == nil {
		return base, status, current, err
	}
	action := request.GetClient().GetStrip()
	key := strings.ToUpper(strings.TrimSpace(action.Callsign))
	old := state.Indexes[pb.EntityKind_STRIP][key]
	seed, err := cdmSeed(state, request.Aggregate.GetSession().Id)
	if err != nil {
		return nil, pb.CommandReply_UNAVAILABLE, current, err
	}
	if old.GetValue().GetStrip().Departure != seed.Airport {
		return base, status, current, nil
	}
	staged := *state
	staged.Entities = make(map[string]*pb.EntitySnapshot, len(state.Entities))
	for k, v := range state.Entities {
		staged.Entities[k] = v
	}
	staged.Indexes = make(map[pb.EntityKind]map[string]*pb.EntitySnapshot, len(state.Indexes))
	for k, v := range state.Indexes {
		staged.Indexes[k] = v
	}
	staged.Indexes[pb.EntityKind_STRIP] = make(map[string]*pb.EntitySnapshot, len(state.Indexes[pb.EntityKind_STRIP]))
	for k, v := range state.Indexes[pb.EntityKind_STRIP] {
		staged.Indexes[pb.EntityKind_STRIP][k] = v
	}
	for _, change := range base.Changes {
		if change.Key != key || change.GetUpsert().GetStrip() == nil {
			continue
		}
		copy := proto.Clone(change.GetUpsert().GetStrip()).(*pb.Strip)
		copy.Revision = old.Revision
		copy.Eobt = old.GetValue().GetStrip().Eobt // confirmation policy needs the preceding EOBT.
		entry := &pb.EntitySnapshot{Key: key, Revision: old.Revision, Value: &pb.EntityRecord{Value: &pb.EntityRecord_Strip{Strip: copy}}}
		staged.Entities[key], staged.Indexes[pb.EntityKind_STRIP][key] = entry, entry
	}
	cdmRequest := proto.Clone(request).(*pb.CommandRequest)
	cdmRequest.ExpectedEntityRevision = proto.Uint64(standRevision(state.Indexes[pb.EntityKind_CDM_STATE][key]))
	cdmRequest.Command = &pb.CommandRequest_Client{Client: &pb.ClientCommand{Action: &pb.ClientCommand_Cdm{Cdm: &pb.CdmAction{Callsign: key, Change: &pb.CdmAction_SetEobt{SetEobt: &pb.SetTobt{Value: action.GetUpdateData().Eobt}}}}}}
	change, status, _, err := c.planAction(ctx, cdmRequest, &staged)
	if err != nil || status != pb.CommandReply_COMMITTED {
		return change, status, current, err
	}
	replacedStrip := false
	for _, item := range change.Changes {
		if item.Key == key && item.GetUpsert().GetStrip() != nil {
			replacedStrip = true
		}
	}
	for _, item := range base.Changes {
		if item.Key != key || item.GetUpsert().GetStrip() == nil || !replacedStrip {
			change.Changes = append(change.Changes, item)
		}
	}
	change.Effects = append(change.Effects, base.Effects...)
	change.Workflows = append(change.Workflows, base.Workflows...)
	cdmSort(change.Changes)
	return change, pb.CommandReply_COMMITTED, current, nil
}
