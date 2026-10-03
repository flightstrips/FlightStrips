package euroscopebinary

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"FlightStrips/internal/cluster"
	"FlightStrips/internal/shared"
	pb "FlightStrips/pkg/events/cluster"
	euroscope "FlightStrips/pkg/events/euroscope"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Positions supplies Task 19a with the very same owner/master dispatcher used
// by binary admission and expiry. Nil means that generation awaits its sync.
func (c *DeadlineCandidate) Positions(id int32) *cluster.PositionWriter {
	state, err := c.Router.Projection.Read(candidateRef(id))
	if err != nil || !c.canWritePosition(id) {
		return nil
	}
	if synced, err := c.Router.Projection.OperationalSync(state.Ref); err != nil || synced == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	w := c.writers[id]
	if w == nil || w.OwnerEpoch != state.Owner.GetEpoch() || w.Connection != state.Master.GetConnectionId() {
		return nil
	}
	return w
}

func (c *DeadlineCandidate) positionWriter(ctx context.Context, id int32, connection string) (*cluster.PositionWriter, error) {
	owner, master, err := c.Router.Projection.ReadSessionTerms(id)
	if err != nil || owner == nil || master == nil {
		return nil, fmt.Errorf("position owner/master unavailable")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	prior := c.writers[id]
	if prior != nil && prior.OwnerEpoch == owner.Epoch && prior.Connection == connection {
		return prior, nil
	}
	if prior != nil {
		if err := prior.Close(ctx); err != nil {
			return nil, err
		}
	}
	masterEpoch, cid := master.Epoch, master.Cid
	w, err := cluster.NewPositionWriter(c.Router.Projection.Positions, id, owner.Epoch, connection,
		func(_ context.Context, session int32, ownerEpoch uint64, generation string) error {
			if !c.canWritePosition(session) {
				return fmt.Errorf("position owner unavailable")
			}
			a, err := c.Router.Projection.ReadOwner(candidateRef(session))
			if err != nil || a.GetEpoch() != ownerEpoch {
				return fmt.Errorf("position owner changed")
			}
			return c.Router.Projection.RequireMasterInbound(session, generation, cid, masterEpoch, false)
		}, 32, 1024)
	if err == nil {
		err = w.SetLifecycleFence(c.Router.Projection.FenceLifecyclePositions)
		if err == nil {
			err = w.SetCommittedApply(c.Router.Projection.ApplyCommittedPosition(c.Router.Lease.NodeID))
		}
		if err != nil {
			_ = w.Close(ctx)
			return nil, err
		}
		if c.Router.Projection.Async != nil {
			if err = w.SetAsyncOwners(ctx, c.Router.Projection.Async, c.Router.Projection, c.Router.Lease.NodeID); err != nil {
				_ = w.Close(ctx)
				return nil, err
			}
		}
		c.writers[id] = w
	}
	return w, err
}

func (c *DeadlineCandidate) position(ctx context.Context, id int32, connection, key string, value *pb.AircraftPosition) error {
	if owners := c.Router.Projection.Async; owners != nil {
		return owners.Execute(ctx, candidateRef(id), func(runCtx context.Context) error { return c.positionAccepted(runCtx, id, connection, key, value) })
	}
	return c.positionAccepted(ctx, id, connection, key, value)
}

func (c *DeadlineCandidate) positionAccepted(ctx context.Context, id int32, connection, key string, value *pb.AircraftPosition) error {

	ctx, span := otel.Tracer("euroscopebinary").Start(ctx, "euroscope.position.processing")
	defer span.End()
	stage := time.Now()
	w, err := c.positionWriter(ctx, id, connection)
	span.SetAttributes(attribute.Float64("position.writer_lookup_ms", float64(time.Since(stage))/float64(time.Millisecond)))
	if err != nil {
		return err
	}
	stage = time.Now()
	var result <-chan cluster.PositionWriteResult
	if value == nil {
		result, err = w.QueueDisconnect(ctx, key, c.clock())
	} else {
		result, err = w.QueuePosition(ctx, key, value, c.clock())
	}
	if errors.Is(err, cluster.ErrAircraftDisconnected) {
		// Duplicate disconnects and late positions are already superseded. Keep
		// the tombstone and socket; authority is still checked before accepting.
		return w.Authority(ctx, id, w.OwnerEpoch, connection)
	}
	if err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case receipt := <-result:
		span.SetAttributes(attribute.Float64("position.queue_to_accept_ms", float64(time.Since(stage))/float64(time.Millisecond)))
		if receipt.Err != nil {
			return receipt.Err
		}
		// The writer materializes the accepted value on its owner before
		// returning success. In memory mode persistence follows independently;
		// the raw fleet cursor remains a separate integrity fence.
		if value != nil {
			stage = time.Now()
			deriveErr := c.derivePosition(ctx, id, w, key, receipt.Revision, value)
			span.SetAttributes(attribute.Float64("position.derive_ms", float64(time.Since(stage))/float64(time.Millisecond)))
			if deriveErr != nil {
				return deriveErr
			}
			// A live position only cancels its own disconnect deadline. Full
			// controller/squawk recovery runs at sync and in the owner worker;
			// copying every completed command and every aircraft on each report
			// adds unrelated work to high-rate observation acceptance.
			deadline, err := c.Router.Projection.ReadEntity(candidateRef(id), pb.EntityKind_SESSION_DEADLINE, "aircraft-disconnect."+key)
			if err != nil {
				return err
			}
			if deadline == nil {
				return nil
			}
		}
		return c.Recover(ctx, id)
	}
}

func (c *DeadlineCandidate) admit(ctx context.Context, id int32, connection, cid string, original *euroscope.Envelope) error {
	if !c.canWritePosition(id) {
		return fmt.Errorf("socket session not owned")
	}
	if err := c.Router.Projection.ValidateEuroScopeInbound(id, connection, cid, original); err != nil {
		return err
	}
	frame := proto.Clone(original).(*euroscope.Envelope)
	if frame.CommandId == "" {
		frame.CommandId = uuid.NewString()
	}
	switch event := frame.GetEvent().(type) {
	case *euroscope.Envelope_Sync:
		c.mu.Lock()
		if w := c.writers[id]; w != nil {
			if err := w.Close(ctx); err != nil {
				c.mu.Unlock()
				return err
			}
			delete(c.writers, id)
		}
		c.mu.Unlock()
		seenControllers := map[string]bool{}
		for _, controller := range event.Sync.Controllers {
			if controller == nil || seenControllers[controller.Callsign] {
				return fmt.Errorf("invalid sync controller set")
			}
			seenControllers[controller.Callsign] = true
			if err := c.controller(ctx, id, connection, cid, frame, controller.Callsign, controller.Position, true); err != nil {
				return err
			}
		}
		before, err := c.Router.Projection.Read(candidateRef(id))
		if err != nil {
			return err
		}
		for _, workflow := range before.Workflows {
			parts := strings.Split(workflow.Step, "/")
			if len(parts) == 3 && parts[0] == "euroscope-controller" && workflow.Status == pb.WorkflowRecord_PENDING && !seenControllers[parts[1]] {
				if err := c.controller(ctx, id, connection, cid, frame, parts[1], parts[2], false); err != nil {
					return err
				}
			}
		}
		seen := map[string]bool{}
		for _, strip := range event.Sync.Strips {
			if strip == nil || seen[strip.Callsign] {
				return fmt.Errorf("duplicate or absent sync strip")
			}
			seen[strip.Callsign] = true
			if err := c.strip(ctx, id, connection, cid, frame, strip); err != nil {
				return err
			}
		}
		state, err := c.Router.Projection.Read(candidateRef(id))
		if err != nil {
			return err
		}
		for _, old := range state.EntitiesByKind(pb.EntityKind_STRIP) {
			if old.Value.GetStrip().EuroscopeObservedAt != nil && !seen[old.Key] {
				if err := c.position(ctx, id, connection, old.Key, nil); err != nil {
					return err
				}
			}
		}
		seed := state.Indexes[pb.EntityKind_SESSION][fmt.Sprint(id)]
		session := proto.Clone(seed.Value.GetSession()).(*pb.Session)
		session.Runways = nil
		for _, runway := range event.Sync.Runways {
			if runway == nil {
				return fmt.Errorf("missing sync runway")
			}
			session.Runways = append(session.Runways, &pb.Runway{Name: runway.Name, Departure: runway.Departure, Arrival: runway.Arrival})
		}
		session.AvailableSids = nil
		for _, sid := range event.Sync.Sids {
			if sid == nil {
				return fmt.Errorf("missing sync SID")
			}
			session.AvailableSids = append(session.AvailableSids, &pb.SidInfo{Name: sid.Name, Runway: sid.Runway})
		}
		if !proto.Equal(session, seed.Value.GetSession()) {
			if err := c.executeFrame(ctx, id, connection, cid, frame, "euroscope-session", seed.Key, &pb.EntityRecord{Value: &pb.EntityRecord_Session{Session: session}}, seed.Revision, "sync-session"); err != nil {
				return err
			}
		}
		// Handler commits the sync marker after this complete admission. Shared
		// recovery will cancel/rearm positions once that marker is operational.
		return nil
	case *euroscope.Envelope_StripUpdate:
		return c.strip(ctx, id, connection, cid, frame, event.StripUpdate.Strip)
	case *euroscope.Envelope_AircraftPositionUpdate:
		v := event.AircraftPositionUpdate
		if v.Altitude < math.MinInt32 || v.Altitude > math.MaxInt32 {
			return fmt.Errorf("invalid position altitude")
		}
		return c.position(ctx, id, connection, v.Callsign, &pb.AircraftPosition{Latitude: v.Lat, Longitude: v.Lon, AltitudeFeet: int32(v.Altitude)})
	case *euroscope.Envelope_AircraftDisconnect:
		return c.position(ctx, id, connection, event.AircraftDisconnect.Callsign, nil)
	case *euroscope.Envelope_ControllerOnline:
		return c.controller(ctx, id, connection, cid, frame, event.ControllerOnline.Callsign, event.ControllerOnline.Position, true)
	case *euroscope.Envelope_ControllerOffline:
		// The shared presence check at recovery/expiry prevents an observation
		// from deleting coverage still connected through another backend.
		return c.controller(ctx, id, connection, cid, frame, event.ControllerOffline.Callsign, "", false)
	case *euroscope.Envelope_AssignedSquawk:
		state, err := c.Router.Projection.Read(candidateRef(id))
		if err != nil {
			return err
		}
		old := state.Indexes[pb.EntityKind_STRIP][event.AssignedSquawk.Callsign]
		if old == nil {
			return fmt.Errorf("assigned squawk strip unavailable")
		}
		strip := proto.Clone(old.Value.GetStrip()).(*pb.Strip)
		strip.AssignedSquawk = event.AssignedSquawk.Squawk
		return c.executeFrame(ctx, id, connection, cid, frame, "euroscope-strip", old.Key, &pb.EntityRecord{Value: &pb.EntityRecord_Strip{Strip: strip}}, old.Revision, "assigned-squawk")
	default:
		if c.NextInbound != nil {
			return c.NextInbound(ctx, id, connection, cid, frame)
		}
		return fmt.Errorf("EuroScope event adapter unavailable for %T", event)
	}
}

func (c *DeadlineCandidate) controller(ctx context.Context, id int32, connection, cid string, frame *euroscope.Envelope, callsign, position string, online bool) error {
	state, err := c.Router.Projection.Read(candidateRef(id))
	if err != nil {
		return err
	}
	if err := c.Router.Projection.RequireMasterInbound(id, connection, cid, frame.MasterEpoch, frame.GetSync() == nil); err != nil {
		return err
	}
	var prior *pb.EntitySnapshot
	// Reports for authenticated controllers are admitted under their durable
	// CID. Socket presence remains the operational online authority.
	for _, old := range state.EntitiesByKind(pb.EntityKind_CONTROLLER) {
		if old.Value.GetController().Callsign != callsign {
			continue
		}
		prior = old
		break
	}
	if prior == nil && !online {
		workflow, err := state.LookupWorkflow(cluster.ControllerObservationID(id, callsign))
		if err != nil {
			return err
		}
		if workflow == nil {
			return nil
		}
		parts := strings.Split(workflow.Step, "/")
		if len(parts) == 3 {
			position = parts[2]
		}
	}
	controller := &pb.Controller{Callsign: callsign, Position: position, Revision: 1}
	if prior != nil {
		controller = proto.Clone(prior.Value.GetController()).(*pb.Controller)
		controller.Revision = prior.Revision + 1
		if position != "" {
			controller.Position = position
		}
	}
	actor := "euroscope-controller"
	if !online {
		actor = "euroscope-controller-offline"
	}
	key := controller.Cid
	if key == "" {
		key = callsign
	}
	return c.executeFrame(ctx, id, connection, cid, frame, actor, key, &pb.EntityRecord{Value: &pb.EntityRecord_Controller{Controller: controller}}, prior.GetRevision(), actor)
}

func (c *DeadlineCandidate) strip(ctx context.Context, id int32, connection, cid string, frame *euroscope.Envelope, observed *euroscope.Strip) error {
	if observed == nil || observed.Callsign == "" || observed.Callsign != strings.ToUpper(strings.TrimSpace(observed.Callsign)) {
		return fmt.Errorf("invalid observed strip")
	}
	ref := candidateRef(id)
	old, err := c.Router.Projection.ReadEntity(ref, pb.EntityKind_STRIP, observed.Callsign)
	if err != nil {
		return err
	}
	seed, err := c.Router.Projection.ReadEntity(ref, pb.EntityKind_SESSION, fmt.Sprint(id))
	if err != nil {
		return err
	}
	if seed.GetValue().GetSession() == nil {
		return fmt.Errorf("observed session unavailable")
	}
	revision := old.GetRevision()
	bay := shared.BAY_UNKNOWN
	if observed.Destination == seed.GetValue().GetSession().Airport {
		bay = shared.BAY_ARR_HIDDEN
	}
	if observed.Origin == seed.GetValue().GetSession().Airport {
		bay = shared.BAY_NOT_CLEARED
		if observed.Cleared {
			bay = shared.BAY_CLEARED
		}
		switch observed.GroundState {
		case euroscope.GroundStatePush:
			bay = shared.BAY_PUSH
		case euroscope.GroundStateTaxi:
			bay = shared.BAY_TAXI
		case euroscope.GroundStateDepart, euroscope.GroundStateLineup:
			bay = shared.BAY_DEPART
		}
	}
	strip := &pb.Strip{Callsign: observed.Callsign, Departure: observed.Origin, Destination: observed.Destination, Alternate: observed.Alternate, Route: observed.Route, Remarks: observed.Remarks,
		Runway: observed.Runway, AssignedSquawk: observed.AssignedSquawk, Squawk: observed.Squawk, Sid: observed.Sid, Star: observed.Star, Stand: observed.Stand, Bay: bay,
		Heading: &observed.Heading, ClearedAltitude: &observed.ClearedAltitude, RequestedAltitude: &observed.RequestedAltitude, AircraftType: observed.AircraftType, AircraftCategory: observed.AircraftCategory, SpokenCallsign: &observed.SpokenCallsign, Capabilities: observed.Capabilities, CommunicationType: observed.CommunicationType, HasFlightPlan: observed.HasFp,
		GroundState: observed.GroundState, EngineType: observed.EngineType, EuroscopeObservedAt: timestamppb.New(c.clock()), Hold: observed.Hold, HoldType: observed.HoldType, HoldEat: observed.HoldEat}
	// The CDM adapter owns EOBT/ELDT admission; a socket observation must not
	// erase its accepted timestamps while the adapters are composed at cutover.
	// Preserve CDM values from the current owner attempt, not this advisory read.
	if observed.Eldt != "" {
		parsed, err := time.Parse("1504", observed.Eldt)
		if err != nil {
			return fmt.Errorf("invalid observed ELDT")
		}
		now := c.clock()
		at := time.Date(now.Year(), now.Month(), now.Day(), parsed.Hour(), parsed.Minute(), 0, 0, time.UTC)
		if at.Before(now.Add(-12 * time.Hour)) {
			at = at.Add(24 * time.Hour)
		}
		strip.Eldt = timestamppb.New(at)
	}
	strip.TrackingController = observed.TrackingController
	if err := c.executeFrame(ctx, id, connection, cid, frame, "euroscope-strip", observed.Callsign, &pb.EntityRecord{Value: &pb.EntityRecord_Strip{Strip: strip}}, revision, "strip"); err != nil {
		return err
	}
	if c.ObservedStrip != nil {
		if err := c.ObservedStrip(ctx, id, connection, cid, frame, observed); err != nil {
			return fmt.Errorf("CDM observation %s: %w", observed.Callsign, err)
		}
	}
	ctx, span := otel.Tracer("euroscopebinary").Start(ctx, "euroscope.position.processing")
	defer span.End()
	stage := time.Now()
	w, err := c.positionWriter(ctx, id, connection)
	span.SetAttributes(attribute.Float64("position.writer_lookup_ms", float64(time.Since(stage))/float64(time.Millisecond)))
	if err != nil {
		return err
	}
	if err = w.ReopenAircraft(ctx, observed.Callsign); err != nil {
		return err
	}
	if observed.Position != nil {
		if err := c.position(ctx, id, connection, observed.Callsign, &pb.AircraftPosition{Latitude: observed.Position.Lat, Longitude: observed.Position.Lon, AltitudeFeet: observed.Position.Altitude}); err != nil {
			return fmt.Errorf("position observation %s: %w", observed.Callsign, err)
		}
	}
	if observed.Origin == seed.GetValue().GetSession().Airport && !observed.Cleared && observed.GroundState == "" && !cluster.ValidAssignedSquawk(observed.AssignedSquawk) {
		commandID := automaticSquawkID(id, strip)
		reply := c.RequestSquawk(ctx, id, commandID, observed.Callsign)
		if reply.GetOutcome().GetReasonCode() != "SQUAWK_ALREADY_PENDING" {
			return candidateReply(reply)
		}
	}
	return nil
}

func (c *DeadlineCandidate) canWritePosition(id int32) bool {
	if c.Router.Projection.Async != nil {
		return c.Router.Lease.CanCommitLocal(candidateRef(id))
	}
	return c.Router.Lease.CanWrite(candidateRef(id))
}
