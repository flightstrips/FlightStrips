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
	if runway := frame.GetRunway(); runway != nil {
		if err := c.Router.Projection.RequireMasterInbound(id, connection, cid, frame.MasterEpoch, false); err != nil {
			return err
		}
		patch := &pb.Session{Id: id}
		for _, v := range runway.Runways {
			if v == nil {
				return fmt.Errorf("absent runway")
			}
			patch.Runways = append(patch.Runways, &pb.Runway{Name: v.Name, Departure: v.Departure, Arrival: v.Arrival})
		}
		key := fmt.Sprint(id)
		commandID, _ := cluster.ProviderEventCommandID("euroscope-event", connection, fmt.Sprintf("%s/runways/%s", frame.CommandId, key))
		req := &pb.CommandRequest{ProtocolRevision: 1, CommandId: commandID, Aggregate: candidateRef(id), Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "euroscope-session"},
			Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: &pb.UpdateEntity{Key: key, Value: &pb.EntityRecord{Value: &pb.EntityRecord_Session{Session: patch}}}}}}}
		writer := c.Router.Writer
		writer.Plan = func(ctx context.Context, req *pb.CommandRequest, state *cluster.Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
			if err := c.Router.Projection.ValidateEuroScopeInbound(id, connection, cid, frame); err != nil {
				return nil, pb.CommandReply_UNAUTHORIZED, 0, err
			}
			if err := c.Router.Projection.RequireMasterInbound(id, connection, cid, frame.MasterEpoch, false); err != nil {
				return nil, pb.CommandReply_UNAUTHORIZED, 0, err
			}
			return planRunwayObservation(ctx, req, state, key, c.Planner)
		}
		return candidateReply(writer.Execute(ctx, req))
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
	// Tracking callbacks can precede the master's first report of an aircraft.
	// They are advisory; the complete strip carries the current tracker.
	if frame.GetTrackingControllerChanged() != nil {
		strip, err := c.Router.Projection.ReadEntity(candidateRef(id), pb.EntityKind_STRIP, callsign)
		if err != nil {
			return err
		}
		if strip == nil {
			return nil
		}
	}
	// Hash the incremental observation, then apply its named fields to the
	// latest strip inside every subject-CAS attempt. Position derivation and
	// provider reconciliation may legitimately change other strip fields.
	req := stripObservationRequest(frame, id, connection, callsign, apply)
	writer := c.Router.Writer
	writer.Plan = func(ctx context.Context, req *pb.CommandRequest, state *cluster.Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
		if err := c.Router.Projection.ValidateEuroScopeInbound(id, connection, cid, frame); err != nil {
			return nil, pb.CommandReply_UNAUTHORIZED, 0, err
		}
		return planStripObservation(ctx, req, state, callsign, apply, c.Planner)
	}
	return candidateReply(writer.Execute(ctx, req))
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

// planStripObservation never carries a previous attempt's whole strip or
// revision into a retry. The stable original request remains the ledger hash.
func planStripObservation(ctx context.Context, req *pb.CommandRequest, state *cluster.Aggregate, callsign string, apply func(*pb.Strip), next cluster.Planner) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
	old := state.Indexes[pb.EntityKind_STRIP][callsign]
	if old == nil || old.Value.GetStrip() == nil {
		return nil, pb.CommandReply_NOT_FOUND, 0, fmt.Errorf("observed strip unavailable")
	}
	strip := proto.Clone(old.Value.GetStrip()).(*pb.Strip)
	apply(strip)
	planned := proto.Clone(req).(*pb.CommandRequest)
	// Planning keeps the established EuroScope merge/deadline policy. The
	// original request retains its event-specific identity in the outcome hash.
	planned.Actor.Id = "euroscope-strip"
	planned.ExpectedEntityRevision = &old.Revision
	planned.GetSystem().GetUpdateEntity().Value = &pb.EntityRecord{Value: &pb.EntityRecord_Strip{Strip: strip}}
	return next(ctx, planned, state)
}

// The hash patch expresses the original observation even when its operation
// depends on the latest domain state.
func stripObservationPatch(frame *es.Envelope, callsign string, apply func(*pb.Strip)) *pb.Strip {
	patch := &pb.Strip{Callsign: callsign}
	apply(patch)
	// Clearance is conditional on the current bay. Its hash still must carry
	// the original boolean when applying it to an empty patch is a no-op.
	if cleared := frame.GetClearedFlag(); cleared != nil {
		patch.Bay = shared.BAY_NOT_CLEARED
		if cleared.Cleared {
			patch.Bay = shared.BAY_CLEARED
		}
	}
	return patch
}

// Runway frames replace runways alone. Fresh planning preserves concurrent
// session settings and SID observations when a subject-CAS attempt retries.
func planRunwayObservation(ctx context.Context, req *pb.CommandRequest, state *cluster.Aggregate, key string, next cluster.Planner) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
	old := state.Indexes[pb.EntityKind_SESSION][key]
	if old == nil || old.Value.GetSession() == nil {
		return nil, pb.CommandReply_NOT_FOUND, 0, fmt.Errorf("runway session unavailable")
	}
	planned := proto.Clone(req).(*pb.CommandRequest)
	session := proto.Clone(old.Value.GetSession()).(*pb.Session)
	session.Runways = planned.GetSystem().GetUpdateEntity().GetValue().GetSession().Runways
	planned.ExpectedEntityRevision = &old.Revision
	planned.GetSystem().GetUpdateEntity().Value = &pb.EntityRecord{Value: &pb.EntityRecord_Session{Session: session}}
	return next(ctx, planned, state)
}

// Sparse typed patches can have identical bytes for distinct clear operations.
// The trusted system actor records the original closed wire event kind so a
// reused command ID cannot silently replay another operation's outcome.
func stripObservationRequest(frame *es.Envelope, id int32, connection, callsign string, apply func(*pb.Strip)) *pb.CommandRequest {
	patch := stripObservationPatch(frame, callsign, apply)
	message := frame.ProtoReflect()
	event := message.WhichOneof(message.Descriptor().Oneofs().ByName("event"))
	commandID, _ := cluster.ProviderEventCommandID("euroscope-event", connection, fmt.Sprintf("%s/observation/%s", frame.CommandId, callsign))
	return &pb.CommandRequest{ProtocolRevision: 1, CommandId: commandID, Aggregate: candidateRef(id), Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "euroscope-strip/" + string(event.Name())},
		Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: &pb.UpdateEntity{Key: callsign, Value: &pb.EntityRecord{Value: &pb.EntityRecord_Strip{Strip: patch}}}}}}}
}

// Full sync observations are replanned against each owner attempt just like
// incremental observations. The original request remains the immutable hash
// input; provider/position work may advance entity revisions in between.
func planFrameObservation(ctx context.Context, req *pb.CommandRequest, state *cluster.Aggregate, next cluster.Planner) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
	planned := proto.Clone(req).(*pb.CommandRequest)
	update := planned.GetSystem().GetUpdateEntity()
	if update == nil {
		return next(ctx, req, state)
	}
	var old *pb.EntitySnapshot
	switch req.GetActor().GetId() {
	case "euroscope-strip":
		old = state.Indexes[pb.EntityKind_STRIP][update.Key]
		incoming := update.GetValue().GetStrip()
		if incoming == nil {
			return next(ctx, req, state)
		}
		incoming.Eobt = old.GetValue().GetStrip().GetEobt()
		if incoming.Eldt == nil {
			incoming.Eldt = old.GetValue().GetStrip().GetEldt()
		}
	case "euroscope-session":
		old = state.Indexes[pb.EntityKind_SESSION][update.Key]
		if old.GetValue().GetSession() == nil {
			return next(ctx, req, state)
		}
		incoming := update.GetValue().GetSession()
		latest := proto.Clone(old.GetValue().GetSession()).(*pb.Session)
		latest.Runways, latest.AvailableSids = incoming.Runways, incoming.AvailableSids
		update.Value = &pb.EntityRecord{Value: &pb.EntityRecord_Session{Session: latest}}
	case "euroscope-controller", "euroscope-controller-offline":
		old = state.Indexes[pb.EntityKind_CONTROLLER][update.Key]
		if latest := old.GetValue().GetController(); latest != nil {
			incoming := update.GetValue().GetController()
			current := proto.Clone(latest).(*pb.Controller)
			current.Position = incoming.Position
			current.Revision = old.Revision + 1
			update.Value = &pb.EntityRecord{Value: &pb.EntityRecord_Controller{Controller: current}}
		}
	default:
		return next(ctx, req, state)
	}
	revision := old.GetRevision()
	planned.ExpectedEntityRevision = &revision
	return next(ctx, planned, state)
}
