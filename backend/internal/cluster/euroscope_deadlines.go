package cluster

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	pb "FlightStrips/pkg/events/cluster"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const ControllerOfflineGrace = 15 * time.Second
const AircraftDisconnectGrace = 60 * time.Second
const MasterTransferGrace = 45 * time.Second
const SessionUpdateDebounce = 300 * time.Millisecond

// EuroScopeDeadlinePlanner couples admissions to their scheduling and repairs
// the FS_STATE/FS_POSITIONS boundary from shared accepted observations. It is
// never constructed by the SQL runtime.
type EuroScopeDeadlinePlanner struct {
	Projection *Projection
	Next       Planner
	Now        func() time.Time
}

func (p EuroScopeDeadlinePlanner) clock() time.Time {
	if p.Now != nil {
		return p.Now().UTC()
	}
	return time.Now().UTC()
}

func deadlineChange(state *Aggregate, id, kind, callsign string, source uint64, due time.Time) *pb.EntityChange {
	old := state.Indexes[pb.EntityKind_SESSION_DEADLINE][id]
	value := &pb.EntityRecord{Value: &pb.EntityRecord_SessionDeadline{SessionDeadline: &pb.SessionDeadline{
		Id: id, Kind: kind, Callsign: callsign, SourceRevision: source, DueAt: timestamppb.New(due)}}}
	if old != nil && proto.Equal(old.Value, value) {
		return nil
	}
	return candidateUpsert(id, old, value)
}

func (p EuroScopeDeadlinePlanner) Plan(ctx context.Context, req *pb.CommandRequest, state *Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
	if req.Aggregate.GetSession() == nil {
		return p.Next(ctx, req, state)
	}
	if req.GetActor().GetKind() == pb.Actor_SYSTEM && req.GetActor().GetId() == "euroscope-deadlines" {
		if req.GetSystem().GetUpdateEntity().GetValue().GetSessionDeadline() == nil || req.ExpectedEntityRevision == nil || *req.ExpectedEntityRevision != state.Revision {
			return workerReject(pb.CommandReply_UNAVAILABLE, state.Revision, "deadline recovery state changed")
		}
		changes, err := p.Recover(state)
		if err != nil {
			return nil, pb.CommandReply_UNAVAILABLE, 0, err
		}
		return &pb.DomainChange{Changes: changes}, pb.CommandReply_COMMITTED, state.Revision, nil
	}
	change, status, revision, err := p.Next(ctx, req, state)
	if err != nil || status != pb.CommandReply_COMMITTED || change == nil {
		return change, status, revision, err
	}
	// Work is persisted with controller/session changes, including login, sync,
	// master election and worker offline removal. Reconciliation therefore
	// survives death between committing an admission and returning to a socket.
	needsUpdate := req.Actor.Id == "euroscope-controller"
	if remove := req.GetSystem().GetRemoveEntity(); remove != nil && remove.Kind == pb.EntityKind_SESSION_DEADLINE {
		if old := state.Indexes[pb.EntityKind_SESSION_DEADLINE][remove.Key]; old != nil && old.Value.GetSessionDeadline().Kind == "controller-offline" {
			needsUpdate = true
		}
	}
	for _, c := range change.Changes {
		kind, _ := changeKind(c)
		if (kind == pb.EntityKind_CONTROLLER || kind == pb.EntityKind_SESSION || kind == pb.EntityKind_SECTOR_OWNER) && req.Actor.Id != "euroscope-controller-offline" {
			needsUpdate = true
		}
		if controller := c.GetUpsert().GetController(); controller != nil && req.Actor.Id != "euroscope-controller-offline" {
			id := "controller-offline." + controller.Callsign
			if old := state.Indexes[pb.EntityKind_SESSION_DEADLINE][id]; old != nil {
				change.Changes = append(change.Changes, candidateDelete(id, old, pb.EntityKind_SESSION_DEADLINE))
			}
		}
		if strip := c.GetUpsert().GetStrip(); strip != nil && req.Actor.Id == "euroscope-strip" {
			id := "aircraft-disconnect." + strip.Callsign
			if old := state.Indexes[pb.EntityKind_SESSION_DEADLINE][id]; old != nil {
				change.Changes = append(change.Changes, candidateDelete(id, old, pb.EntityKind_SESSION_DEADLINE))
			}
		}
	}
	if needsUpdate {
		source := state.Indexes[pb.EntityKind_SESSION][strconv.Itoa(int(req.Aggregate.GetSession().Id))].GetRevision()
		for _, c := range change.Changes {
			if session := c.GetUpsert().GetSession(); session != nil {
				source = c.Revision
			}
		}
		if c := deadlineChange(state, "session-update", "session-update", "", source, p.clock().Add(SessionUpdateDebounce)); c != nil {
			change.Changes = append(change.Changes, c)
		}
	}
	if req.GetSystem().GetElectSessionMaster() != nil {
		for _, e := range state.EntitiesByKind(pb.EntityKind_SESSION_DEADLINE) {
			d := e.Value.GetSessionDeadline()
			if d.Kind == "controller-offline" || d.Kind == "aircraft-disconnect" {
				if c := deadlineChange(state, d.Id, d.Kind, d.Callsign, d.SourceRevision, p.clock().Add(MasterTransferGrace)); c != nil {
					change.Changes = append(change.Changes, c)
				}
			}
		}
	}
	sortCandidateChanges(change.Changes)
	return change, status, revision, nil
}

// Recover is a pure plan from durable identities and fresh shared observations.
// Existing due times are preserved, and newer observations cancel/rearm work.
func (p EuroScopeDeadlinePlanner) Recover(state *Aggregate) ([]*pb.EntityChange, error) {
	if p.Projection == nil {
		return nil, fmt.Errorf("deadline observations unavailable")
	}
	id := state.Ref.GetSession().GetId()
	positions, entries, err := p.Projection.ObservationSnapshot(id)
	if err != nil {
		return nil, err
	}
	presence, err := operationalSessionPresence(entries, id, p.clock())
	if err != nil {
		return nil, err
	}
	live, err := SharedEuroScopeControllers(p.Projection, state, p.clock())
	if err != nil {
		return nil, err
	}
	online := map[string]bool{}
	for _, controller := range live {
		if !controller.Observer {
			online[controller.Callsign] = true
		}
	}
	changes := []*pb.EntityChange{}
	for _, workflow := range state.Workflows {
		parts := strings.Split(workflow.Step, "/")
		if len(parts) == 3 && parts[0] == "euroscope-controller" && workflow.Status == pb.WorkflowRecord_PENDING && online[parts[1]] {
			found := false
			for _, entity := range state.EntitiesByKind(pb.EntityKind_CONTROLLER) {
				if entity.Value.GetController().Callsign == parts[1] {
					found = true
					break
				}
			}
			if !found {
				if old := state.Indexes[pb.EntityKind_SESSION_DEADLINE]["controller-offline."+parts[1]]; old != nil {
					changes = append(changes, candidateDelete(old.Key, old, pb.EntityKind_SESSION_DEADLINE))
				}
			}
		}
	}
	pendingOffline := false
	for _, e := range state.EntitiesByKind(pb.EntityKind_CONTROLLER) {
		c := e.Value.GetController()
		key := "controller-offline." + c.Callsign
		old := state.Indexes[pb.EntityKind_SESSION_DEADLINE][key]
		if online[c.Callsign] {
			if old != nil {
				changes = append(changes, candidateDelete(key, old, pb.EntityKind_SESSION_DEADLINE))
			}
		} else if !c.Observer && (old == nil || old.Value.GetSessionDeadline().SourceRevision != e.Revision) {
			changes = append(changes, deadlineChange(state, key, "controller-offline", c.Callsign, e.Revision, p.clock().Add(ControllerOfflineGrace)))
		}
		if !c.Observer && !online[c.Callsign] {
			pendingOffline = true
		}
	}
	for _, pos := range positions {
		key := "aircraft-disconnect." + pos.Value.AircraftKey
		old := state.Indexes[pb.EntityKind_SESSION_DEADLINE][key]
		if pos.Stale {
			continue
		}
		strip := state.Indexes[pb.EntityKind_STRIP][pos.Value.AircraftKey].GetValue().GetStrip()
		if pos.Value.GetPosition() != nil || strip == nil || strip.EuroscopeObservedAt == nil || strip.EuroscopeObservedAt.AsTime().After(pos.Value.GetObservedAt().AsTime()) {
			if old != nil {
				changes = append(changes, candidateDelete(key, old, pb.EntityKind_SESSION_DEADLINE))
			}
		} else if old == nil || !p.Projection.PositionSourceMatches(old.Value.GetSessionDeadline().SourceRevision, pos.Revision) {
			// The KV entry remains after a crash before this FS_STATE commit.
			changes = append(changes, deadlineChange(state, key, "aircraft-disconnect", pos.Value.AircraftKey, pos.Revision, p.clock().Add(AircraftDisconnectGrace)))
		}
	}
	key := "session-disconnect"
	old := state.Indexes[pb.EntityKind_SESSION_DEADLINE][key]
	seed := state.Indexes[pb.EntityKind_SESSION][strconv.Itoa(int(id))]
	if seed == nil {
		return nil, fmt.Errorf("deadline session unavailable")
	}
	if presence.controllers {
		if old != nil {
			changes = append(changes, candidateDelete(key, old, pb.EntityKind_SESSION_DEADLINE))
		}
	} else if old == nil && pendingOffline {
		changes = append(changes, deadlineChange(state, key, "session-disconnect", "", seed.Revision, p.clock().Add(ControllerOfflineGrace+SessionUpdateDebounce)))
	}
	sortCandidateChanges(changes)
	return changes, nil
}
