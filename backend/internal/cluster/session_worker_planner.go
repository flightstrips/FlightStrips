package cluster

import (
	"context"
	"fmt"
	"strconv"
	"time"

	pb "FlightStrips/pkg/events/cluster"
	"google.golang.org/protobuf/proto"
)

// SessionWorkerPlanner is installed around the candidate session planner at
// the Task 20 cutover. The worker's read is advisory; these checks run again
// on the owner against the fresh state before subject CAS.
type SessionWorkerPlanner struct {
	Next       Planner
	Projection *Projection
	Now        func() time.Time
	// RetainedAircraft must derive its answer from shared, revisioned NATS
	// observations. A nil implementation fails closed on disconnect deletion.
	RetainedAircraft func(*Aggregate, string) (bool, error)
}

func (p SessionWorkerPlanner) clock() time.Time {
	if p.Now != nil {
		return p.Now().UTC()
	}
	return time.Now().UTC()
}

func (p SessionWorkerPlanner) Plan(ctx context.Context, req *pb.CommandRequest, state *Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
	if p.Next == nil {
		return nil, pb.CommandReply_UNAVAILABLE, 0, fmt.Errorf("session planner unavailable")
	}
	if req.GetAggregate().GetSession() == nil || req.GetActor().GetKind() != pb.Actor_SYSTEM || req.GetActor().GetId() != "session-worker" {
		return p.Next(ctx, req, state)
	}
	if req.GetSystem().GetRemoveEntity().GetKind() == pb.EntityKind_SESSION_DEADLINE {
		return p.planDeadline(ctx, req, state)
	}
	if u := req.GetSystem().GetUpdateEntity(); u != nil && u.GetValue().GetSession() != nil {
		return p.planMarker(req, state, u)
	}
	if req.GetSystem().GetDeleteSession() != nil {
		return p.planCleanup(ctx, req, state)
	}
	return p.Next(ctx, req, state)
}

func (p SessionWorkerPlanner) presence(sessionID int32) (sessionPresence, error) {
	if p.Projection == nil {
		return sessionPresence{}, fmt.Errorf("session presence unavailable")
	}
	_, entries, err := p.Projection.ObservationSnapshot(sessionID)
	if err != nil {
		return sessionPresence{}, err
	}
	return operationalSessionPresence(entries, sessionID, p.clock())
}

func workerReject(status pb.CommandReply_Status, revision uint64, message string) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
	return nil, status, revision, fmt.Errorf("%s", message)
}

func (p SessionWorkerPlanner) planMarker(req *pb.CommandRequest, state *Aggregate, update *pb.UpdateEntity) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
	id := req.Aggregate.GetSession().Id
	old := state.Indexes[pb.EntityKind_SESSION][strconv.Itoa(int(id))]
	if old == nil || old.GetValue().GetSession() == nil || old.GetValue().GetSession().Tombstoned {
		return workerReject(pb.CommandReply_NOT_FOUND, 0, "session is not active")
	}
	if req.ExpectedEntityRevision == nil || *req.ExpectedEntityRevision != old.Revision {
		return workerReject(pb.CommandReply_REVISION_CONFLICT, old.Revision, "stale session marker")
	}
	if update.Key != old.Key {
		return workerReject(pb.CommandReply_INVALID_ARGUMENT, old.Revision, "session marker key mismatch")
	}
	proposed := update.Value.GetSession()
	copy := proto.Clone(old.Value.GetSession()).(*pb.Session)
	copy.FirstNoControllerAt = proposed.FirstNoControllerAt
	copy.CleanupPausedAt = proposed.CleanupPausedAt
	if !proto.Equal(copy, proposed) {
		return workerReject(pb.CommandReply_INVALID_ARGUMENT, old.Revision, "session worker may only change the no-controller marker")
	}
	presence, err := p.presence(id)
	if err != nil {
		return nil, pb.CommandReply_UNAVAILABLE, old.Revision, err
	}
	marker := proposed.FirstNoControllerAt
	if presence.controllers && marker != nil || !presence.controllers && marker == nil {
		return workerReject(pb.CommandReply_REVISION_CONFLICT, old.Revision, "controller presence changed")
	}
	if marker != nil && (marker.CheckValid() != nil || marker.AsTime().After(p.clock().Add(time.Second)) || old.GetValue().GetSession().FirstNoControllerAt != nil && marker.AsTime().Before(old.GetValue().GetSession().FirstNoControllerAt.AsTime())) {
		return workerReject(pb.CommandReply_INVALID_ARGUMENT, old.Revision, "invalid no-controller observation time")
	}
	if proposed.CleanupPausedAt != nil && (marker == nil || proposed.CleanupPausedAt.CheckValid() != nil || proposed.CleanupPausedAt.AsTime().After(p.clock().Add(time.Second))) {
		return workerReject(pb.CommandReply_INVALID_ARGUMENT, old.Revision, "invalid cleanup pause marker")
	}
	if proto.Equal(old.Value.GetSession(), proposed) {
		return &pb.DomainChange{}, pb.CommandReply_COMMITTED, old.Revision, nil
	}
	return &pb.DomainChange{Changes: []*pb.EntityChange{candidateUpsert(old.Key, old, &pb.EntityRecord{Value: &pb.EntityRecord_Session{Session: proposed}})}}, pb.CommandReply_COMMITTED, old.Revision, nil
}

func (p SessionWorkerPlanner) planCleanup(ctx context.Context, req *pb.CommandRequest, state *Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
	id := req.Aggregate.GetSession().Id
	old := state.Indexes[pb.EntityKind_SESSION][strconv.Itoa(int(id))]
	if old == nil || old.GetValue().GetSession() == nil {
		return workerReject(pb.CommandReply_NOT_FOUND, 0, "session is missing")
	}
	if req.ExpectedEntityRevision == nil || *req.ExpectedEntityRevision != old.Revision {
		return workerReject(pb.CommandReply_REVISION_CONFLICT, old.Revision, "stale cleanup source")
	}
	session := old.GetValue().GetSession()
	if session.Tombstoned {
		return &pb.DomainChange{}, pb.CommandReply_COMMITTED, old.Revision, nil
	}
	presence, err := p.presence(id)
	if err != nil {
		return nil, pb.CommandReply_UNAVAILABLE, old.Revision, err
	}
	if presence.controllers || session.FirstNoControllerAt == nil || p.clock().Before(session.FirstNoControllerAt.AsTime().Add(sessionCleanupGrace)) || presence.oldestNode.After(session.FirstNoControllerAt.AsTime()) {
		return workerReject(pb.CommandReply_REVISION_CONFLICT, old.Revision, "healthy no-controller window is incomplete")
	}
	// LifecyclePlanner rejects an expected revision because its normal global
	// workflow has none. This wrapper has checked it against the session seed.
	copy := proto.Clone(req).(*pb.CommandRequest)
	copy.ExpectedEntityRevision = nil
	return p.Next(ctx, copy, state)
}

func (p SessionWorkerPlanner) planDeadline(ctx context.Context, req *pb.CommandRequest, state *Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
	remove := req.GetSystem().GetRemoveEntity()
	old := state.Indexes[pb.EntityKind_SESSION_DEADLINE][remove.Key]
	if old == nil {
		return workerReject(pb.CommandReply_NOT_FOUND, 0, "deadline is absent")
	}
	if req.ExpectedEntityRevision == nil || *req.ExpectedEntityRevision != old.Revision {
		return workerReject(pb.CommandReply_REVISION_CONFLICT, old.Revision, "deadline was rearmed")
	}
	d := old.GetValue().GetSessionDeadline()
	if d == nil || d.DueAt == nil || p.clock().Before(d.DueAt.AsTime()) {
		return workerReject(pb.CommandReply_REVISION_CONFLICT, old.Revision, "deadline has not elapsed")
	}
	changes := []*pb.EntityChange{candidateDelete(old.Key, old, pb.EntityKind_SESSION_DEADLINE)}
	switch d.Kind {
	case "pdc-response":
		sequence := state.Indexes[pb.EntityKind_PDC_SEQUENCE][d.Callsign]
		if sequence != nil && sequence.Revision == d.SourceRevision {
			value := sequence.GetValue().GetPdcSequence()
			if value != nil && value.State == "CLEARED" && value.Deadline != nil && value.Deadline.AsTime().Equal(d.DueAt.AsTime()) {
				copy := proto.Clone(value).(*pb.PdcSequence)
				copy.State, copy.Deadline = "NO_RESPONSE", nil
				changes = append(changes, candidateUpsert(sequence.Key, sequence, &pb.EntityRecord{Value: &pb.EntityRecord_PdcSequence{PdcSequence: copy}}))
			}
		}
	case "strip-auto-hide":
		strip := state.Indexes[pb.EntityKind_STRIP][d.Callsign]
		if strip != nil && strip.Revision == d.SourceRevision && strip.GetValue().GetStrip().Bay == "STAND" {
			seed := state.Indexes[pb.EntityKind_SESSION][strconv.Itoa(int(req.Aggregate.GetSession().Id))]
			if seed == nil {
				return nil, pb.CommandReply_UNAVAILABLE, old.Revision, fmt.Errorf("session seed is unavailable")
			}
			copy := proto.Clone(strip.GetValue().GetStrip()).(*pb.Strip)
			if err := moveStripBay(state, copy, "HIDDEN", seed.GetValue().GetSession().Airport, false, true); err != nil {
				return workerReject(pb.CommandReply_REVISION_CONFLICT, old.Revision, err.Error())
			}
			changes = append(changes, stripChange(strip, copy))
		}
	case "controller-offline":
		connected, err := p.controllerPresent(req.Aggregate.GetSession().Id, d.Callsign)
		if err != nil {
			return nil, pb.CommandReply_UNAVAILABLE, old.Revision, err
		}
		if !connected {
			for _, controller := range state.EntitiesByKind(pb.EntityKind_CONTROLLER) {
				if controller.GetValue().GetController().Callsign == d.Callsign && controller.Revision == d.SourceRevision {
					changes = append(changes, candidateDelete(controller.Key, controller, pb.EntityKind_CONTROLLER))
					break
				}
			}
		}
	case "aircraft-disconnect":
		if p.Projection == nil || p.RetainedAircraft == nil {
			return nil, pb.CommandReply_UNAVAILABLE, old.Revision, fmt.Errorf("aircraft retention projection is unavailable")
		}
		positions, _, err := p.Projection.ObservationSnapshot(req.Aggregate.GetSession().Id)
		if err != nil {
			return nil, pb.CommandReply_UNAVAILABLE, old.Revision, err
		}
		var disconnected bool
		var observed bool
		for _, position := range positions {
			if position.Value.AircraftKey != d.Callsign {
				continue
			}
			observed = true
			if position.Stale {
				return nil, pb.CommandReply_UNAVAILABLE, old.Revision, fmt.Errorf("aircraft position awaits new owner sync")
			}
			disconnected = position.Revision == d.SourceRevision && position.Value.GetTombstone() != nil
			break
		}
		if !observed {
			return nil, pb.CommandReply_UNAVAILABLE, old.Revision, fmt.Errorf("aircraft position has not replayed")
		}
		if disconnected {
			retained, err := p.RetainedAircraft(state, d.Callsign)
			if err != nil {
				return nil, pb.CommandReply_UNAVAILABLE, old.Revision, err
			}
			if retained {
				strip := state.Indexes[pb.EntityKind_STRIP][d.Callsign]
				if strip != nil && strip.Value.GetStrip().EuroscopeObservedAt != nil {
					copy := proto.Clone(strip.Value.GetStrip()).(*pb.Strip)
					copy.EuroscopeObservedAt = nil
					changes = append(changes, stripChange(strip, copy))
				}
			} else {
				strip := state.Indexes[pb.EntityKind_STRIP][d.Callsign]
				if strip != nil {
					copy := proto.Clone(req).(*pb.CommandRequest)
					copy.ExpectedEntityRevision = proto.Uint64(strip.Revision)
					copy.GetSystem().Action = &pb.SystemCommand_RemoveEntity{RemoveEntity: &pb.RemoveEntity{Key: d.Callsign, Kind: pb.EntityKind_STRIP}}
					deleted, status, _, err := planStripDelete(copy, state, copy.GetSystem().GetRemoveEntity())
					if err != nil {
						return nil, status, old.Revision, err
					}
					changes = append(changes, deleted.Changes...)
				}
			}
		}
	case "session-update", "session-disconnect", "session-cleanup":
		// These are superseded by the owner projection sweep and the durable
		// no-controller marker. No independent local timer may mutate state.
	default:
		return workerReject(pb.CommandReply_INVALID_ARGUMENT, old.Revision, "unsupported deadline kind")
	}
	sortCandidateChanges(changes)
	return &pb.DomainChange{Changes: changes}, pb.CommandReply_COMMITTED, old.Revision, nil
}

func (p SessionWorkerPlanner) controllerPresent(id int32, callsign string) (bool, error) {
	if p.Projection == nil {
		return false, fmt.Errorf("session presence unavailable")
	}
	state, err := p.Projection.Read(sessionRef(id))
	if err != nil {
		return false, err
	}
	shared, err := SharedEuroScopeControllers(p.Projection, state, p.clock())
	if err != nil {
		return false, err
	}
	for _, controller := range shared {
		if controller.Callsign == callsign {
			return true, nil
		}
	}
	_, entries, err := p.Projection.ObservationSnapshot(id)
	if err != nil {
		return false, err
	}
	now := p.clock()
	nodes := map[string]bool{}
	for _, item := range entries {
		if node := item.Value.GetNode(); node != nil && node.Ready && !item.Observed.After(now) && now.Sub(item.Observed) < nodeTTL {
			nodes[node.NodeId] = true
		}
	}
	if len(nodes) == 0 {
		return false, fmt.Errorf("no fresh ready node presence")
	}
	for _, item := range entries {
		client := item.Value.GetClient()
		if client != nil && client.SessionId == id && client.Kind == pb.ClientPresence_EUROSCOPE && client.Callsign == callsign && !client.Observer && nodes[client.NodeId] && !item.Observed.After(now) && now.Sub(item.Observed) < nodeTTL {
			return true, nil
		}
	}
	return false, nil
}
