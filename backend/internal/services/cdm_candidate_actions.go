package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"FlightStrips/internal/cdm"
	"FlightStrips/internal/cluster"
	pb "FlightStrips/pkg/events/cluster"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// CdmActionService submits the boundary-assigned UUID and actor unchanged.
// Its store may be the owner router; only the owner's Planner evaluates policy.
type CdmActionService struct{ Store cluster.LifecycleStore }

func NewCdmActionService(store cluster.LifecycleStore) (*CdmActionService, error) {
	if store == nil {
		return nil, fmt.Errorf("CDM actions require typed owner store")
	}
	return &CdmActionService{Store: store}, nil
}
func (s *CdmActionService) Execute(ctx context.Context, request *pb.CommandRequest) *pb.CommandReply {
	if request == nil || (request.GetClient().GetCdm() == nil && request.GetClient().GetValidation().GetUpdateTobt() == nil && request.GetClient().GetStrip().GetUpdateData().GetEobt() == nil) {
		return &pb.CommandReply{Status: pb.CommandReply_INVALID_ARGUMENT, Detail: "typed CDM action required"}
	}
	return s.Store.Execute(ctx, request)
}

// Planner wraps the candidate domain chain at Task 20; it handles every CDM
// browser, HTTP, and authenticated socket operation through the same policy.
func (c *CdmCandidate) Planner(next cluster.Planner) cluster.Planner {
	return func(ctx context.Context, request *pb.CommandRequest, state *cluster.Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
		if request.GetClient().GetStrip().GetUpdateData().GetEobt() != nil {
			return c.planStripEobt(ctx, request, state, next)
		}
		if request.GetClient().GetCdm() == nil && request.GetClient().GetValidation().GetUpdateTobt() == nil {
			if next == nil {
				return nil, pb.CommandReply_UNAVAILABLE, 0, fmt.Errorf("CDM next planner unavailable")
			}
			return next(ctx, request, state)
		}
		return c.planAction(ctx, request, state)
	}
}
func (c *CdmCandidate) planAction(ctx context.Context, request *pb.CommandRequest, state *cluster.Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
	id := request.GetAggregate().GetSession().GetId()
	seed, err := cdmSeed(state, id)
	if err != nil {
		return nil, pb.CommandReply_NOT_FOUND, 0, err
	}
	a := request.GetClient().GetCdm()
	clx := request.GetClient().GetValidation().GetUpdateTobt()
	if clx != nil {
		a = &pb.CdmAction{Callsign: request.GetClient().GetValidation().Callsign, Change: &pb.CdmAction_SetTobt{SetTobt: &pb.SetTobt{Value: clx.Value}}}
	}
	key := strings.ToUpper(strings.TrimSpace(a.Callsign))
	strip := state.Indexes[pb.EntityKind_STRIP][key]
	if strip == nil {
		return nil, pb.CommandReply_NOT_FOUND, 0, fmt.Errorf("CDM strip unavailable")
	}
	old := state.Indexes[pb.EntityKind_CDM_STATE][key]
	expected := standRevision(old)
	inputRevision := expected
	if clx != nil {
		inputRevision = strip.Revision
	}
	if request.ExpectedEntityRevision == nil || *request.ExpectedEntityRevision != inputRevision {
		return nil, pb.CommandReply_REVISION_CONFLICT, expected, fmt.Errorf("CDM state changed")
	}
	actor := request.Actor
	position, role := actor.Id, "ATC"
	if actor.GetSessionId() != id {
		return nil, pb.CommandReply_UNAUTHORIZED, expected, fmt.Errorf("CDM actor session mismatch")
	}
	switch actor.Kind {
	case pb.Actor_CONTROLLER:
		controller := state.Indexes[pb.EntityKind_CONTROLLER][actor.Id].GetValue().GetController()
		if controller == nil || controller.Observer {
			return nil, pb.CommandReply_UNAUTHORIZED, expected, fmt.Errorf("operational controller required")
		}
		if owner := strip.GetValue().GetStrip().OwnerCid; owner != "" && owner != actor.Id {
			return nil, pb.CommandReply_UNAUTHORIZED, expected, fmt.Errorf("CDM strip has another owner")
		}
		position = controller.Position
	case pb.Actor_PILOT:
		if a.GetSetTobt() == nil || clx != nil {
			return nil, pb.CommandReply_UNAUTHORIZED, expected, fmt.Errorf("pilot may only change TOBT")
		}
		role = "pilot"
		if seed.Master == nil || seed.Master.Cid == "" {
			return nil, pb.CommandReply_UNAVAILABLE, expected, fmt.Errorf("CDM master unavailable")
		}
	case pb.Actor_SYSTEM:
		if actor.Id != "cdm-operations" && actor.Id != "euroscope-cdm" {
			return nil, pb.CommandReply_UNAUTHORIZED, expected, fmt.Errorf("authenticated CDM adapter required")
		}
	default:
		return nil, pb.CommandReply_UNAUTHORIZED, expected, fmt.Errorf("CDM actor unavailable")
	}
	if strip.GetValue().GetStrip().Departure != seed.Airport {
		return nil, pb.CommandReply_INVALID_ARGUMENT, expected, fmt.Errorf("CDM departure required")
	}
	page, configRevision, err := c.Config.Read(ctx, seed.Airport)
	if err != nil {
		return nil, pb.CommandReply_UNAVAILABLE, expected, err
	}
	config, err := clusterCdmConfig(page, seed)
	if err != nil {
		return nil, pb.CommandReply_UNAVAILABLE, expected, err
	}
	operation := cdm.CandidateAction{Position: position, Role: role}
	invalidClock := false
	clock := func(v *pb.SetTobt) string {
		if v.GetValue() != nil && v.GetHhmmUtc() != "" {
			invalidClock = true
		}
		if v.GetValue() != nil {
			return v.Value.AsTime().UTC().Format("1504")
		}
		return v.GetHhmmUtc()
	}
	switch x := a.Change.(type) {
	case *pb.CdmAction_SetTobt:
		operation.Kind, operation.Value = "tobt", clock(x.SetTobt)
	case *pb.CdmAction_SetReady:
		if !x.SetReady.Ready {
			return nil, pb.CommandReply_INVALID_ARGUMENT, expected, fmt.Errorf("READY request must be true")
		}
		operation.Kind = "ready"
	case *pb.CdmAction_SetDeice:
		operation.Kind, operation.Value = "deice", x.SetDeice.Code
	case *pb.CdmAction_SetCtot:
		operation.Kind = "ctot"
		if x.SetCtot.Value != nil {
			operation.Value = x.SetCtot.Value.AsTime().UTC().Format("1504")
		}
	case *pb.CdmAction_RemoveCtot:
		operation.Kind = "remove-ctot"
	case *pb.CdmAction_SetEobt:
		operation.Kind, operation.Value = "eobt", clock(x.SetEobt)
	case *pb.CdmAction_SetAsrt:
		operation.Kind, operation.Value = "asrt", clock(x.SetAsrt)
	case *pb.CdmAction_SetTsac:
		operation.Kind, operation.Value = "tsac", clock(x.SetTsac)
	case *pb.CdmAction_ClearanceTobt:
		operation.Kind = "clearance-tobt"
	case *pb.CdmAction_GroundState:
		operation.Kind, operation.Value = "ground-state", x.GroundState.State
	case *pb.CdmAction_RecordAobt:
		operation.Kind = "aobt"
	case *pb.CdmAction_RecordAtot:
		operation.Kind = "atot"
	case *pb.CdmAction_PreparePushback:
		operation.Kind = "pushback"
	case *pb.CdmAction_BetterTobt:
		operation.Kind = "better-tobt"
	case *pb.CdmAction_LogonEobt:
		operation.Kind, operation.Value = "logon", clock(x.LogonEobt)
	default:
		return nil, pb.CommandReply_INVALID_ARGUMENT, expected, fmt.Errorf("unsupported CDM action")
	}
	if clx != nil {
		operation.Kind = "clx-tobt"
	}
	if invalidClock {
		return nil, pb.CommandReply_INVALID_ARGUMENT, expected, fmt.Errorf("one CDM clock value required")
	}
	if actor.Kind != pb.Actor_SYSTEM && (operation.Kind == "ground-state" || operation.Kind == "aobt" || operation.Kind == "atot" || operation.Kind == "logon") {
		return nil, pb.CommandReply_UNAUTHORIZED, expected, fmt.Errorf("CDM observation requires authenticated adapter")
	}
	now := c.clock()
	m := cdmModel(state, strip, id)
	positions, _, err := c.Writer.Projection.ObservationSnapshot(id)
	if err != nil {
		return nil, pb.CommandReply_UNAVAILABLE, expected, err
	}
	for _, p := range positions {
		if p.Value.AircraftKey == key && p.Value.GetPosition() != nil {
			if p.Stale {
				return nil, pb.CommandReply_UNAVAILABLE, expected, fmt.Errorf("CDM position stale")
			}
			pos := p.Value.GetPosition()
			m.PositionLatitude, m.PositionLongitude = &pos.Latitude, &pos.Longitude
		}
	}
	d, err := cdm.PlanAction(m, config, operation, now)
	if err != nil {
		return nil, pb.CommandReply_INVALID_ARGUMENT, expected, err
	}
	if c.usesViff(seed) {
		if operation.Kind == "ready" {
			d.ReadySyncPending = true
		}
		if operation.Kind == "atot" && old.GetValue().GetCdmState().GetAtot() == nil && cdmValue(d.Atot) != "" {
			d.AtotViffPending = true
		}
	}
	s, data, err := cdmRecords(strip.GetValue().GetStrip(), old.GetValue().GetCdmState(), d, now)
	if err != nil {
		return nil, pb.CommandReply_INVALID_ARGUMENT, expected, err
	}
	m.CdmData = d
	change := &pb.DomainChange{}
	if operation.Kind == "pushback" {
		data.Pushback = &pb.CdmState_PushbackVerification{OperationId: request.CommandId, ExpectedTobt: s.Tobt}
		if !c.usesViff(seed) {
			data.Pushback.Completed = true
		}
		if c.usesViff(seed) {
			deadline := cdmDeadline(id, "cdm-pushback", now.Add(500*time.Millisecond), state.Revision+1)
			deadline.Id = "cdm-pushback/" + key
			deadline.CommandId = lifecycleID(request.CommandId, "pushback/probe/1")
			change.Changes = append(change.Changes, candidateUpsert(deadline.Id, state.Indexes[pb.EntityKind_SESSION_DEADLINE][deadline.Id], &pb.EntityRecord{Value: &pb.EntityRecord_SessionDeadline{SessionDeadline: deadline}}))
		}
	}
	if clx != nil && s.Validation.GetIssueType() == "CLX" {
		s.Validation = nil
		m.ValidationStatus = nil
	}
	if operation.Kind == "ground-state" {
		s.GroundState = operation.Value
	}
	validation := PlanCtotValidation(m, now, lifecycleID(request.CommandId, "validation"), false)
	if validation == nil {
		s.Validation = nil
	} else if validation.IssueType == ctotValidationIssueType {
		s.Validation = cdmValidation(validation)
	}
	if !equalStripWithoutRevision(strip.GetValue().GetStrip(), s) {
		change.Changes = append(change.Changes, stripChange(strip, s))
	}
	if d.NeedsLocalRecalculation() {
		deadline := cdmDeadline(id, "cdm-debounce", now.Add(500*time.Millisecond), state.Revision+1)
		deadline.CommandId = lifecycleID(request.CommandId, "debounce")
		prior := state.Indexes[pb.EntityKind_SESSION_DEADLINE][deadline.Id]
		change.Changes = append(change.Changes, candidateUpsert(deadline.Id, prior, &pb.EntityRecord{Value: &pb.EntityRecord_SessionDeadline{SessionDeadline: deadline}}))
	}
	if c.usesViff(seed) {
		if !proto.Equal(data, old.GetValue().GetCdmState()) {
			data.SourceRevision = request.CommandId
		}
		add := func(label, value string, kind pb.CdmState_ExportIntent_Kind, taxi int) {
			data.PendingExports = append(data.PendingExports, &pb.CdmState_ExportIntent{OperationId: lifecycleID(request.CommandId, "viff/"+label), Kind: kind, Value: value, TaxiMinutes: int32(taxi), InputRevision: expected + 1})
		}
		if (operation.Kind == "tobt" || operation.Kind == "eobt" || operation.Kind == "logon") && cdmValue(d.Tobt) != "" && cdmValue(d.Tobt) != cdmValue(cdmModel(state, strip, id).CdmData.Tobt) {
			add("tobt", cdmValue(d.Tobt), pb.CdmState_ExportIntent_SET_TOBT, cdm.CandidateTaxiMinutes(m, config))
		}
		if operation.Kind == "better-tobt" {
			add("better-tobt", cdmValue(d.Status), pb.CdmState_ExportIntent_DPI, 0)
		}
		if cdmValue(d.Aobt) != "" && cdmValue(m.CdmData.Aobt) != cdmValue(cdmModel(state, strip, id).CdmData.Aobt) {
			add("aobt", "AOBT/"+cdmValue(d.Aobt), pb.CdmState_ExportIntent_DPI, 0)
		}
		if operation.Kind == "atot" && old.GetValue().GetCdmState().GetAtot() == nil {
			add("atot", "ATOT/"+cdmValue(d.Atot), pb.CdmState_ExportIntent_DPI, 0)
		}
		if !d.NeedsLocalRecalculation() {
			if err := c.queueExport(state, data, request.CommandId, m, expected+1, config); err != nil {
				return nil, pb.CommandReply_UNAVAILABLE, 0, err
			}
		}
	}
	if !proto.Equal(data, old.GetValue().GetCdmState()) {
		change.Changes = append(change.Changes, candidateUpsert(key, old, &pb.EntityRecord{Value: &pb.EntityRecord_CdmState{CdmState: data}}))
	}
	// Corrected EOBT is an immutable plugin effect targeted at the accepted
	// master, never a detached hub send. Controller/pilot TOBT uses the same path.
	if actor.Id != "euroscope-cdm" && (operation.Kind == "eobt" || operation.Kind == "tobt" || operation.Kind == "clx-tobt") && seed.Master != nil && seed.Master.Cid != "" {
		field, value := "TOBT", cdmValue(d.Tobt)
		if operation.Kind == "eobt" {
			field, value = "EOBT", cdmValue(d.Eobt)
		}
		effect := &pb.EffectRecord{CommandId: request.CommandId, TargetCid: seed.Master.Cid, TargetConnectionId: &seed.Master.ConnectionId, OwnerEpoch: state.Owner.Epoch, MasterEpoch: seed.Master.Epoch, Status: pb.EffectRecord_WAITING, DispatchDeadline: timestamppb.New(now.Add(30 * time.Second))}
		if field == "TOBT" {
			effect.Payload = &pb.EffectRecord_Cdm{Cdm: &pb.CdmEffect{Callsign: key, Action: "SET_TOBT", Value: value}}
		} else {
			effect.Payload = &pb.EffectRecord_SetFlightPlan{SetFlightPlan: &pb.SetFlightPlanEffect{Callsign: key, Field: field, Value: value}}
		}
		change.Effects = append(change.Effects, effect)
	}
	// Cross-aggregate and observation inputs are checked again after planning.
	currentConfig, rev, err := c.Config.Read(ctx, seed.Airport)
	if err != nil || rev != configRevision || !proto.Equal(currentConfig, page) {
		return nil, pb.CommandReply_UNAVAILABLE, expected, fmt.Errorf("CDM config changed during action")
	}
	freshPositions, _, err := c.Writer.Projection.ObservationSnapshot(id)
	if err != nil || lifecyclePositionTag(freshPositions) != lifecyclePositionTag(positions) {
		return nil, pb.CommandReply_UNAVAILABLE, expected, fmt.Errorf("CDM position changed during action")
	}
	cdmSort(change.Changes)
	return change, pb.CommandReply_COMMITTED, expected, nil
}
