package app

import (
	"FlightStrips/internal/cluster"
	pb "FlightStrips/pkg/events/cluster"
	es "FlightStrips/pkg/events/euroscope"
	"context"
	"fmt"
	"google.golang.org/protobuf/types/known/timestamppb"
	"time"
)

func (r *natsRuntime) inboundAMAN(ctx context.Context, id int32, connection, cid string, controller *pb.Controller, frame *es.Envelope) error {
	if r.aman == nil {
		return fmt.Errorf("AMAN disabled")
	}
	v := frame.GetAmanRouteFact()
	if v.Version != 1 || v.Data == nil {
		return fmt.Errorf("invalid route fact version")
	}
	data := v.Data
	at, err := time.Parse(time.RFC3339Nano, data.ObservedAt)
	if err != nil {
		return err
	}
	state, err := r.projection.Read(sessionNATSRef(id))
	if err != nil {
		return err
	}
	seed := state.Indexes[pb.EntityKind_SESSION][fmt.Sprint(id)].GetValue().GetSession()
	if seed == nil || seed.Tombstoned {
		return fmt.Errorf("route fact session unavailable")
	}
	input := &pb.ReportAmanRouteFact{ConnectionId: connection, OwnerEpoch: frame.OwnerEpoch, MasterEpoch: frame.MasterEpoch, Callsign: data.Callsign, ObservedAt: timestamppb.New(at.UTC())}
	switch data.Kind {
	case "direct_to":
		if data.AssignedSpeed != nil {
			return fmt.Errorf("direct-to contains speed")
		}
		input.Fact = &pb.ReportAmanRouteFact_DirectToFix{DirectToFix: data.GetDirectToFix()}
	case "speed":
		if data.DirectToFix != nil || data.AssignedSpeed == nil {
			return fmt.Errorf("invalid speed fact")
		}
		switch s := data.AssignedSpeed.Value.(type) {
		case *es.AssignedSpeed_Knots:
			if s.Knots == 0 {
				return fmt.Errorf("invalid speed")
			}
			input.Fact = &pb.ReportAmanRouteFact_AssignedSpeed{AssignedSpeed: fmt.Sprintf("%d KT", s.Knots)}
		case *es.AssignedSpeed_MachThousandths:
			if s.MachThousandths == 0 {
				return fmt.Errorf("invalid speed")
			}
			input.Fact = &pb.ReportAmanRouteFact_AssignedSpeed{AssignedSpeed: fmt.Sprintf("M%.3f", float64(s.MachThousandths)/1000)}
		default:
			return fmt.Errorf("speed required")
		}
	default:
		return fmt.Errorf("unknown route fact kind")
	}
	request := &pb.CommandRequest{ProtocolRevision: 1, CommandId: frame.CommandId, Aggregate: airportNATSRef(seed.Airport), Actor: &pb.Actor{Kind: pb.Actor_CONTROLLER, Id: cid, SessionId: &id}, Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_ReportAmanRouteFact{ReportAmanRouteFact: input}}}}
	return natsReply(r.Route(ctx, request))
}
func (r *natsRuntime) planRouteFact(ctx context.Context, req *pb.CommandRequest, state *cluster.Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
	actor := req.GetActor()
	input := req.GetSystem().GetReportAmanRouteFact()
	if r.aman == nil || actor.Kind != pb.Actor_CONTROLLER || actor.SessionId == nil {
		return nil, pb.CommandReply_UNAUTHORIZED, 0, fmt.Errorf("route fact requires authenticated controller")
	}
	if err := r.projection.RequireLiveSocket(*actor.SessionId, input.ConnectionId, actor.Id, pb.ClientPresence_EUROSCOPE); err != nil {
		return nil, pb.CommandReply_UNAUTHORIZED, 0, err
	}
	session, err := r.projection.Read(sessionNATSRef(*actor.SessionId))
	if err != nil {
		return nil, pb.CommandReply_UNAVAILABLE, 0, err
	}
	seed := session.Indexes[pb.EntityKind_SESSION][fmt.Sprint(*actor.SessionId)].GetValue().GetSession()
	controller := session.Indexes[pb.EntityKind_CONTROLLER][actor.Id].GetValue().GetController()
	if seed == nil || seed.Tombstoned || seed.Airport != req.Aggregate.GetAirport().Icao || controller == nil || controller.Observer || input.OwnerEpoch != 0 && input.OwnerEpoch != session.Owner.GetEpoch() {
		return nil, pb.CommandReply_UNAUTHORIZED, 0, fmt.Errorf("route fact session identity changed")
	}
	return r.aman.PlanRouteFact(ctx, req, state, session, controller)
}
