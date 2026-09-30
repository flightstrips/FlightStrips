package pdc

import (
	"context"
	"sort"
	"strings"
	"time"

	"FlightStrips/internal/cluster"
	pb "FlightStrips/pkg/events/cluster"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Deferred route/SID effects refer to the exact committed strip revision and
// the immutable ISSUE target. A later edit supersedes them before dispatch.
func (c *Candidate) processPlugins(ctx context.Context, id int32) error {
	state, err := c.Writer.Read(ctx, pdcRef(id))
	if err != nil {
		return err
	}
	keys := []string{}
	for key, w := range state.Workflows {
		if strings.HasPrefix(w.Step, "pdc/plugin/") && w.Status == pb.WorkflowRecord_PENDING {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		record := proto.Clone(state.Workflows[key]).(*pb.WorkflowRecord)
		record.Status = pb.WorkflowRecord_COMPLETED
		req := pdcSystem(id, record.DerivedCommandId, "pdc-plugin", nil, &pb.SystemCommand{Action: &pb.SystemCommand_AdvanceWorkflow{AdvanceWorkflow: &pb.AdvanceWorkflow{Workflow: record}}})
		if err = pdcReply(c.Writer.Execute(ctx, req)); err != nil {
			return err
		}
	}
	return nil
}
func (c *Candidate) planPlugin(r *pb.CommandRequest, a *cluster.Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
	proposed := r.GetSystem().GetAdvanceWorkflow().GetWorkflow()
	if proposed == nil {
		return pdcReject(pb.CommandReply_INVALID_ARGUMENT, "PDC plugin intent missing")
	}
	old := a.Workflows[proposed.WorkflowId]
	if old == nil || old.Step == "" || old.Status != pb.WorkflowRecord_PENDING || proposed.Status != pb.WorkflowRecord_COMPLETED || r.CommandId != old.DerivedCommandId {
		return pdcReject(pb.CommandReply_INVALID_ARGUMENT, "PDC plugin identity mismatch")
	}
	copy := proto.Clone(proposed).(*pb.WorkflowRecord)
	copy.Status = old.Status
	if !proto.Equal(copy, old) {
		return pdcReject(pb.CommandReply_INVALID_ARGUMENT, "PDC plugin intent changed")
	}
	parts := strings.Split(old.Step, "/")
	if len(parts) != 5 || parts[0] != "pdc" || parts[1] != "plugin" || (parts[2] != "SID" && parts[2] != "ROUTE" && parts[2] != "SET_CLEARED_FLAG" && parts[2] != "STATE_CHANGE") {
		return pdcReject(pb.CommandReply_INVALID_ARGUMENT, "unsupported PDC plugin action")
	}
	terminal := proto.Clone(proposed).(*pb.WorkflowRecord)
	out := &pb.DomainChange{Workflows: []*pb.WorkflowRecord{terminal}}
	source := a.Indexes[pb.EntityKind_STRIP][parts[3]]
	primary := a.Effects[parts[4]]
	if source == nil || old.SourceRevision == nil || source.Revision != *old.SourceRevision || primary == nil {
		terminal.Status, terminal.ReasonCode = pb.WorkflowRecord_SUPERSEDED, "SOURCE_CHANGED"
		return out, pb.CommandReply_COMMITTED, 0, nil
	}
	value := source.Value.GetStrip().Sid
	if parts[2] == "ROUTE" {
		value = source.Value.GetStrip().Route
	}
	out.Effects = []*pb.EffectRecord{{CommandId: r.CommandId, TargetCid: primary.TargetCid, TargetConnectionId: primary.TargetConnectionId, OwnerEpoch: a.Owner.Epoch, MasterEpoch: primary.MasterEpoch, Status: pb.EffectRecord_WAITING, DispatchDeadline: timestamppb.New(c.clock().Add(30 * time.Second)), Payload: &pb.EffectRecord_SetFlightPlan{SetFlightPlan: &pb.SetFlightPlanEffect{Callsign: parts[3], Field: parts[2], Value: value}}}}
	if parts[2] == "SET_CLEARED_FLAG" || parts[2] == "STATE_CHANGE" {
		s := source.Value.GetStrip()
		out.Effects[0].Payload = &pb.EffectRecord_Pdc{Pdc: &pb.PdcEffect{Callsign: s.Callsign, Action: parts[2], Cleared: proto.Bool(false), State: proto.String(s.PdcState), Remarks: proto.String(s.PdcRequestRemarks)}}
	}
	return out, pb.CommandReply_COMMITTED, 0, nil
}
