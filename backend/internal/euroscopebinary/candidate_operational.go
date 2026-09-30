package euroscopebinary

import (
	"FlightStrips/internal/cluster"
	"FlightStrips/internal/shared"
	pb "FlightStrips/pkg/events/cluster"
	es "FlightStrips/pkg/events/euroscope"
	"context"
	"fmt"
	"google.golang.org/protobuf/proto"
)

// AdmitOperational applies master observations through the same strip/session
// planner as a full sync. It preserves fields absent from an incremental frame.
func (c *DeadlineCandidate) AdmitOperational(ctx context.Context, id int32, connection, cid string, frame *es.Envelope) error {
	if err := c.Router.Projection.ValidateEuroScopeInbound(id, connection, cid, frame); err != nil {
		return err
	}
	state, err := c.Router.Projection.Read(candidateRef(id))
	if err != nil {
		return err
	}
	if runway := frame.GetRunway(); runway != nil {
		seed := state.Indexes[pb.EntityKind_SESSION][fmt.Sprint(id)]
		session := proto.Clone(seed.Value.GetSession()).(*pb.Session)
		session.Runways = nil
		for _, v := range runway.Runways {
			if v == nil {
				return fmt.Errorf("absent runway")
			}
			session.Runways = append(session.Runways, &pb.Runway{Name: v.Name, Departure: v.Departure, Arrival: v.Arrival})
		}
		return c.executeFrame(ctx, id, connection, cid, frame, "euroscope-session", seed.Key, &pb.EntityRecord{Value: &pb.EntityRecord_Session{Session: session}}, seed.Revision, "runways")
	}
	callsign := ""
	apply := func(*pb.Strip) {}
	switch v := frame.Event.(type) {
	case *es.Envelope_Squawk:
		callsign = v.Squawk.Callsign
		apply = func(s *pb.Strip) { s.Squawk = v.Squawk.Squawk }
	case *es.Envelope_RequestedAltitude:
		callsign = v.RequestedAltitude.Callsign
		apply = func(s *pb.Strip) { s.RequestedAltitude = &v.RequestedAltitude.Altitude }
	case *es.Envelope_ClearedAltitude:
		callsign = v.ClearedAltitude.Callsign
		apply = func(s *pb.Strip) { s.ClearedAltitude = &v.ClearedAltitude.Altitude }
	case *es.Envelope_CommunicationType:
		callsign = v.CommunicationType.Callsign
		apply = func(s *pb.Strip) { s.CommunicationType = v.CommunicationType.CommunicationType }
	case *es.Envelope_GroundState:
		callsign = v.GroundState.Callsign
		apply = func(s *pb.Strip) {
			s.GroundState = v.GroundState.GroundState
			switch s.GroundState {
			case es.GroundStatePush:
				s.Bay = shared.BAY_PUSH
			case es.GroundStateTaxi:
				s.Bay = shared.BAY_TAXI
			case es.GroundStateDepart, es.GroundStateLineup:
				s.Bay = shared.BAY_DEPART
			}
		}
	case *es.Envelope_ClearedFlag:
		callsign = v.ClearedFlag.Callsign
		apply = func(s *pb.Strip) {
			if s.Bay == shared.BAY_NOT_CLEARED && v.ClearedFlag.Cleared {
				s.Bay = shared.BAY_CLEARED
			} else if s.Bay == shared.BAY_CLEARED && !v.ClearedFlag.Cleared {
				s.Bay = shared.BAY_NOT_CLEARED
			}
		}
	case *es.Envelope_Heading:
		callsign = v.Heading.Callsign
		apply = func(s *pb.Strip) { s.Heading = &v.Heading.Heading }
	case *es.Envelope_Stand:
		callsign = v.Stand.Callsign
		apply = func(s *pb.Strip) { s.Stand = v.Stand.Stand }
	case *es.Envelope_Hold:
		callsign = v.Hold.Callsign
		apply = func(s *pb.Strip) { s.Hold = v.Hold.Hold; s.HoldType = v.Hold.HoldType; s.HoldEat = v.Hold.HoldEat }
	case *es.Envelope_Route:
		callsign = v.Route.Callsign
		apply = func(s *pb.Strip) { s.Route = v.Route.Route }
	case *es.Envelope_Remarks:
		callsign = v.Remarks.Callsign
		apply = func(s *pb.Strip) { s.Remarks = v.Remarks.Remarks }
	case *es.Envelope_AircraftInfo:
		callsign = v.AircraftInfo.Callsign
		apply = func(s *pb.Strip) { s.AircraftType = v.AircraftInfo.AircraftType }
	case *es.Envelope_AircraftInfoRemarks:
		callsign = v.AircraftInfoRemarks.Callsign
		apply = func(s *pb.Strip) {
			s.AircraftType = v.AircraftInfoRemarks.AircraftType
			s.Remarks = v.AircraftInfoRemarks.Remarks
		}
	case *es.Envelope_Sid:
		callsign = v.Sid.Callsign
		apply = func(s *pb.Strip) { s.Sid = v.Sid.Sid }
	case *es.Envelope_AircraftRunway:
		callsign = v.AircraftRunway.Callsign
		apply = func(s *pb.Strip) { s.Runway = v.AircraftRunway.Runway }
	case *es.Envelope_TrackingControllerChanged:
		callsign = v.TrackingControllerChanged.Callsign
		apply = func(s *pb.Strip) { s.TrackingController = v.TrackingControllerChanged.TrackingController }
	default:
		return fmt.Errorf("not an inbound operational observation: %T", frame.Event)
	}
	old := state.Indexes[pb.EntityKind_STRIP][callsign]
	if old == nil {
		return fmt.Errorf("observed strip unavailable")
	}
	strip := proto.Clone(old.Value.GetStrip()).(*pb.Strip)
	apply(strip)
	return c.executeFrame(ctx, id, connection, cid, frame, "euroscope-strip", callsign, &pb.EntityRecord{Value: &pb.EntityRecord_Strip{Strip: strip}}, old.Revision, "observation")
}

// ExecuteClient evaluates policy only at the current session owner, and repeats
// socket generation/epoch validation inside the atomic command attempt.
func (c *DeadlineCandidate) ExecuteClient(ctx context.Context, id int32, connection, cid string, frame *es.Envelope, req *pb.CommandRequest, master bool) *pb.CommandReply {
	writer := c.Router.Writer
	writer.Plan = func(ctx context.Context, r *pb.CommandRequest, state *cluster.Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
		if err := c.Router.Projection.ValidateEuroScopeInbound(id, connection, cid, frame); err != nil {
			return nil, pb.CommandReply_UNAUTHORIZED, 0, err
		}
		if master {
			if err := c.Router.Projection.RequireMasterInbound(id, connection, cid, frame.MasterEpoch, frame.GetSync() == nil); err != nil {
				return nil, pb.CommandReply_UNAUTHORIZED, 0, err
			}
		}
		return c.Router.Writer.Plan(ctx, r, state)
	}
	return writer.Execute(ctx, req)
}
