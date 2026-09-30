package cluster

import (
	"FlightStrips/internal/shared"
	pb "FlightStrips/pkg/events/cluster"
	"context"
	"fmt"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	"math"
	"strconv"
	"strings"
	"time"
)

// PlanStripCommands fills the validation and manual-FPL boundary using the
// same accepted strip revisions and conditional acknowledgement policy as the
// existing services. It delegates all other actions to the composed planners.
func PlanStripCommands(next Planner) Planner {
	return func(ctx context.Context, req *pb.CommandRequest, state *Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
		validation, fpl := req.GetClient().GetValidation(), req.GetClient().GetFlightPlan()
		if validation == nil && fpl == nil {
			return next(ctx, req, state)
		}
		session, err := stripSession(req, state)
		if err != nil {
			return nil, pb.CommandReply_NOT_FOUND, 0, err
		}
		actor := req.GetActor()
		controller := state.Indexes[pb.EntityKind_CONTROLLER][actor.Id].GetValue().GetController()
		if actor.Kind != pb.Actor_CONTROLLER || actor.GetSessionId() != req.Aggregate.GetSession().Id || controller == nil || controller.Observer {
			return nil, pb.CommandReply_UNAUTHORIZED, 0, fmt.Errorf("active controller required")
		}
		callsign := validation.GetCallsign()
		if fpl != nil {
			callsign = fpl.Callsign
		}
		key := strings.ToUpper(strings.TrimSpace(callsign))
		old := state.Indexes[pb.EntityKind_STRIP][key]
		if old == nil {
			return nil, pb.CommandReply_NOT_FOUND, 0, fmt.Errorf("strip not found")
		}
		current := old.Revision
		if req.ExpectedEntityRevision == nil || *req.ExpectedEntityRevision != current {
			return nil, pb.CommandReply_REVISION_CONFLICT, current, fmt.Errorf("stale strip revision")
		}
		strip := proto.Clone(old.Value.GetStrip()).(*pb.Strip)
		change := &pb.DomainChange{}
		if validation != nil {
			switch v := validation.Change.(type) {
			case *pb.ValidationActionCommand_Acknowledge:
				status := strip.Validation
				if status == nil || !status.Active || status.ActivationKey != v.Acknowledge.ActivationKey {
					return change, pb.CommandReply_COMMITTED, current, nil
				}
				if status.OwningPosition != controller.Position && status.IssueType != "PDC INVALID" && status.IssueType != "CUSTOM PDC" {
					return nil, pb.CommandReply_UNAUTHORIZED, current, fmt.Errorf("validation belongs to another position")
				}
				status.Active = false
			case *pb.ValidationActionCommand_AcknowledgeUnexpectedChange:
				field := v.AcknowledgeUnexpectedChange.FieldName
				if strings.TrimSpace(field) == "" {
					return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("field required")
				}
				out := strip.UnexpectedChangeFields[:0]
				for _, f := range strip.UnexpectedChangeFields {
					if f != field {
						out = append(out, f)
					}
				}
				strip.UnexpectedChangeFields = out
				markStripField(strip, field)
			case *pb.ValidationActionCommand_ClxOverride:
				key := v.ClxOverride.OverrideKey
				if key == "" {
					return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("override key required")
				}
				id := strip.Callsign + "." + key
				previous := state.Indexes[pb.EntityKind_CLX_OVERRIDE][id]
				value := &pb.ClxOverride{Callsign: strip.Callsign, Key: key, Actor: actor.Id, CreatedAt: timestamppb.Now()}
				change.Changes = append(change.Changes, candidateUpsert(id, previous, &pb.EntityRecord{Value: &pb.EntityRecord_ClxOverride{ClxOverride: value}}))
				return change, pb.CommandReply_COMMITTED, current, nil
			default:
				return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("unsupported validation command")
			}
		} else {
			if strip.OwnerCid != "" && strip.OwnerCid != actor.Id {
				return nil, pb.CommandReply_UNAUTHORIZED, current, fmt.Errorf("strip owned by another controller")
			}
			effect := &pb.CreateFlightPlanEffect{Callsign: strip.Callsign, Origin: strip.Departure}
			switch v := fpl.Create.(type) {
			case *pb.FlightPlanAction_Manual:
				p := v.Manual
				if p == nil || p.Destination == "" {
					return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("destination required")
				}
				if strip.Departure == "" {
					strip.Departure = session.Value.GetSession().Airport
				}
				strip.Destination = p.Destination
				if p.Sid != "" {
					strip.Sid = p.Sid
				}
				if p.Squawk != "" {
					if !validSquawk(p.Squawk) {
						return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("invalid squawk")
					}
					strip.AssignedSquawk = p.Squawk
				}
				if p.Eobt != nil {
					if p.Eobt.CheckValid() != nil {
						return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("invalid EOBT")
					}
					strip.Eobt = p.Eobt
				}
				if p.AircraftType != "" {
					strip.AircraftType = p.AircraftType
				}
				if p.Route != "" {
					strip.Route = p.Route
				}
				if p.Stand != "" {
					strip.Stand = p.Stand
				}
				if p.DepartureRunway != "" {
					strip.Runway = p.DepartureRunway
				}
				if p.FlightLevel != "" {
					level, e := strconv.ParseInt(p.FlightLevel, 10, 32)
					if e != nil || level < 0 || level > math.MaxInt32/100 {
						return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("invalid flight level")
					}
					alt := int32(level * 100)
					strip.RequestedAltitude = &alt
				}
				strip.Bay = shared.BAY_NOT_CLEARED
				effect.Origin = strip.Departure
				effect.Destination = p.Destination
				effect.Sid = p.Sid
				effect.AssignedSquawk = p.Squawk
				effect.AircraftType = p.AircraftType
				effect.Route = p.Route
				effect.Stand = p.Stand
				effect.Runway = p.DepartureRunway
				if p.Eobt != nil {
					effect.Eobt = p.Eobt.AsTime().UTC().Format("1504")
				}
				if strip.RequestedAltitude != nil {
					effect.RequestedAltitude = *strip.RequestedAltitude
				}
			case *pb.FlightPlanAction_Vfr:
				p := v.Vfr
				if p == nil || p.PersonsOnBoard > math.MaxInt32 {
					return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("invalid VFR flight plan")
				}
				ssr := p.Squawk
				if ssr == "" {
					ssr = "7000"
				}
				if !validSquawk(ssr) {
					return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("invalid squawk")
				}
				strip.AssignedSquawk = ssr
				if p.AircraftType != "" {
					strip.AircraftType = p.AircraftType
				}
				if p.PersonsOnBoard > 0 {
					strip.PersonsOnBoard = &p.PersonsOnBoard
				}
				if p.FlightPlanType != "" {
					strip.FlightPlanType = p.FlightPlanType
				}
				if p.Language != "" {
					strip.Language = p.Language
				}
				if p.Remarks != "" {
					strip.Remarks = p.Remarks
				}
				strip.Bay = shared.BAY_CONTROLZONE
				effect.AssignedSquawk = ssr
				effect.AircraftType = p.AircraftType
				effect.PersonsOnBoard = int32(p.PersonsOnBoard)
				effect.FplType = p.FlightPlanType
				effect.Language = p.Language
				effect.Remarks = p.Remarks
			default:
				return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("flight plan kind required")
			}
			strip.Manual = true
			strip.HasFlightPlan = true
			strip.Sequence, err = endOfStripBay(state, strip.Bay, key)
			if err != nil {
				return nil, pb.CommandReply_INVALID_ARGUMENT, current, err
			}
			master := state.Master
			if master == nil {
				return nil, pb.CommandReply_UNAVAILABLE, current, fmt.Errorf("flight plan master unavailable")
			}
			change.Effects = []*pb.EffectRecord{{CommandId: req.CommandId, TargetCid: master.Cid, TargetConnectionId: &master.ConnectionId, OwnerEpoch: state.ownerEpoch(), MasterEpoch: master.Epoch, Status: pb.EffectRecord_WAITING, Payload: &pb.EffectRecord_CreateFlightPlan{CreateFlightPlan: effect}, DispatchDeadline: timestamppb.New(time.Now().UTC().Add(30 * time.Second))}}
		}
		if !equalStripWithoutRevision(old.Value.GetStrip(), strip) {
			change.Changes = append(change.Changes, stripChange(old, strip))
		}
		return change, pb.CommandReply_COMMITTED, current, nil
	}
}
