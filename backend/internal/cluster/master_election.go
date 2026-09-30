package cluster

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"FlightStrips/internal/config"
	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
)

// MasterElection runs only on the accepted session owner. ConnectionId is the
// socket generation: a reconnect gets a new UUID even when its CID is the same.
type MasterElection struct {
	Projection *Projection
	Router     interface {
		Route(context.Context, *pb.CommandRequest) *pb.CommandReply
	}
	Lease interface{ CanWrite(*pb.AggregateRef) bool }
}

func (e MasterElection) candidate(ctx context.Context, sessionID int32, airport string) (*pb.ClientPresence, error) {
	_, entries, err := e.Projection.ObservationSnapshot(sessionID)
	if err != nil {
		return nil, err
	}
	nodes := make(map[string]bool)
	for _, entry := range entries {
		if node := entry.Value.GetNode(); node != nil && node.Ready {
			nodes[node.NodeId] = true
		}
	}
	order := []string{"EKDK_FMP", "EKDK_B_CTR", "EKDK_D_CTR", "EKDK_UC_CTR", "EKCH_A_TWR"}
	if policy, err := e.Projection.Read(&pb.AggregateRef{Target: &pb.AggregateRef_Airport{Airport: &pb.AirportRef{Icao: airport}}}); err == nil {
		if record := policy.Entities[airport]; record != nil && record.GetValue().GetAirportPolicy() != nil {
			if configured := record.GetValue().GetAirportPolicy().MasterPositionOrder; len(configured) > 0 {
				order = configured
			}
		}
	}
	var best *pb.ClientPresence
	priority := func(client *pb.ClientPresence) int {
		name := strings.ToUpper(strings.TrimSpace(client.Callsign))
		position := strings.ToUpper(strings.TrimSpace(client.Position))
		positionName := position
		if configured, err := config.GetPositionBasedOnFrequency(client.Position); err == nil {
			positionName = strings.ToUpper(strings.TrimSpace(configured.Name))
		}
		for i, preferred := range order {
			preferred = strings.ToUpper(strings.TrimSpace(preferred))
			if preferred == name || preferred == position || preferred == positionName {
				return i
			}
		}
		// Preserve the current FMP frequency priority if the airport policy
		// has not named the position explicitly.
		if position == "131.040" {
			return 0
		}
		return len(order)
	}
	for _, entry := range entries {
		client := entry.Value.GetClient()
		if client == nil || client.Kind != pb.ClientPresence_EUROSCOPE || client.Observer ||
			client.SessionId != sessionID || client.Cid == "" || client.ConnectionId == "" ||
			client.ConnectedAt == nil || client.ConnectedAt.CheckValid() != nil || !nodes[client.NodeId] {
			continue
		}
		if best == nil || priority(client) < priority(best) ||
			(priority(client) == priority(best) && (client.ConnectedAt.AsTime().Before(best.ConnectedAt.AsTime()) ||
				(client.ConnectedAt.AsTime().Equal(best.ConnectedAt.AsTime()) && client.ConnectionId < best.ConnectionId))) {
			best = client
		}
	}
	if best == nil {
		return nil, nil
	}
	return proto.Clone(best).(*pb.ClientPresence), nil
}

// Reconcile commits a new epoch only when the selected socket or owner term
// changes. A vacant term keeps the epoch so reconnect never inherits sync.
func (e MasterElection) Reconcile(ctx context.Context, sessionID int32) (*pb.MasterTerm, error) {
	if e.Projection == nil || e.Router == nil || e.Lease == nil || !e.Lease.CanWrite(sessionRef(sessionID)) {
		return nil, fmt.Errorf("session owner is unavailable")
	}
	state, err := e.Projection.Read(sessionRef(sessionID))
	if err != nil {
		return nil, err
	}
	entry := state.Entities[strconv.Itoa(int(sessionID))]
	if entry == nil || entry.GetValue().GetSession() == nil || entry.GetValue().GetSession().Tombstoned {
		return nil, fmt.Errorf("session is not active")
	}
	selected, err := e.candidate(ctx, sessionID, entry.GetValue().GetSession().Airport)
	if err != nil {
		return nil, err
	}
	old := state.Master
	connection, cid := "", ""
	if selected != nil {
		connection, cid = selected.ConnectionId, selected.Cid
	}
	if old != nil && old.ConnectionId == connection && old.Cid == cid && old.OwnerEpoch == state.Owner.Epoch {
		return proto.Clone(old).(*pb.MasterTerm), nil
	}
	epoch := uint64(1)
	if old != nil {
		epoch = old.Epoch + 1
		if epoch == 0 {
			return nil, fmt.Errorf("master epoch exhausted")
		}
	}
	master := &pb.MasterTerm{ConnectionId: connection, Cid: cid, Epoch: epoch, OwnerEpoch: state.Owner.Epoch}
	request := &pb.CommandRequest{ProtocolRevision: 1, CommandId: uuid.NewString(), Aggregate: sessionRef(sessionID),
		Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "session-master-election"},
		Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_ElectSessionMaster{
			ElectSessionMaster: &pb.ElectSessionMaster{Master: master}}}}}
	reply := e.Router.Route(ctx, request)
	if reply == nil || reply.Status != pb.CommandReply_COMMITTED || reply.GetOutcome().GetStatus() != pb.CommandOutcome_SUCCEEDED {
		if reply == nil {
			return nil, fmt.Errorf("master election received no reply")
		}
		return nil, fmt.Errorf("master election: %s: %s", reply.Status, reply.Detail)
	}
	return master, nil
}

// Run follows presence and owner changes. All nodes may run it; only the
// current owner can commit an election. CAS and the planner settle races.
func (e MasterElection) Run(ctx context.Context, sessionID int32) error {
	if e.Projection == nil || e.Lease == nil || sessionID < 1 {
		return fmt.Errorf("invalid master election runner")
	}
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		if e.Lease.CanWrite(sessionRef(sessionID)) {
			_, _ = e.Reconcile(ctx, sessionID)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// RunAll watches the active global session registry. It keeps elections alive
// even when the accepted owner has no local EuroScope sockets.
func (e MasterElection) RunAll(ctx context.Context, registry SessionRegistry) error {
	if e.Projection == nil || e.Lease == nil || registry.Store == nil {
		return fmt.Errorf("invalid master election supervisor")
	}
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		if e.Projection.Ready() == nil {
			if sessions, err := registry.ActiveSessions(ctx); err == nil {
				for _, session := range sessions {
					if e.Lease.CanWrite(sessionRef(session.Id)) {
						_, _ = e.Reconcile(ctx, session.Id)
					}
				}
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// MasterElectionPlanner is installed on the owner writer's planner chain.
// It rechecks live presence at the commit point, before the subject CAS.
func MasterElectionPlanner(projection *Projection, next Planner) Planner {
	return func(ctx context.Context, request *pb.CommandRequest, state *Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
		action := request.GetSystem().GetElectSessionMaster()
		if action == nil {
			if next == nil {
				return nil, pb.CommandReply_INVALID_ARGUMENT, 0, fmt.Errorf("no planner for command")
			}
			if sync := request.GetSystem().GetRecordSync().GetSync(); sync != nil {
				if state == nil || state.Master == nil || request.GetAggregate().GetSession() == nil || projection == nil {
					return nil, pb.CommandReply_UNAUTHORIZED, 0, fmt.Errorf("master sync has no session authority")
				}
				if err := projection.RequireMasterInbound(request.Aggregate.GetSession().Id,
					sync.ConnectionId, state.Master.Cid, sync.MasterEpoch, false); err != nil {
					return nil, pb.CommandReply_UNAUTHORIZED, 0, err
				}
			}
			return next(ctx, request, state)
		}
		entry, err := observationSession(request, state)
		if err != nil {
			return nil, pb.CommandReply_NOT_FOUND, 0, err
		}
		current := entry.Revision
		master := action.GetMaster()
		if projection == nil || request.Actor.GetKind() != pb.Actor_SYSTEM || request.Actor.Id != "session-master-election" ||
			request.ExpectedEntityRevision != nil || state.Owner == nil || master == nil || master.Epoch == 0 ||
			master.OwnerEpoch != state.Owner.Epoch || (master.ConnectionId == "") != (master.Cid == "") {
			return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("invalid master election")
		}
		if state.Master != nil && (state.Master.Epoch == ^uint64(0) || master.Epoch != state.Master.Epoch+1) ||
			state.Master == nil && master.Epoch != 1 {
			return nil, pb.CommandReply_REVISION_CONFLICT, current, fmt.Errorf("master epoch changed")
		}
		session := entry.GetValue().GetSession()
		selected, err := (MasterElection{Projection: projection}).candidate(ctx, session.Id, session.Airport)
		if err != nil {
			return nil, pb.CommandReply_UNAVAILABLE, current, err
		}
		if selected == nil && (master.ConnectionId != "" || master.Cid != "") ||
			selected != nil && (master.ConnectionId != selected.ConnectionId || master.Cid != selected.Cid) {
			return nil, pb.CommandReply_REVISION_CONFLICT, current, fmt.Errorf("master candidate changed")
		}
		if state.Master != nil && state.Master.ConnectionId == master.ConnectionId &&
			state.Master.Cid == master.Cid && state.Master.OwnerEpoch == master.OwnerEpoch {
			return nil, pb.CommandReply_REVISION_CONFLICT, current, fmt.Errorf("master is already current")
		}
		copy := proto.Clone(session).(*pb.Session)
		copy.Master = proto.Clone(master).(*pb.MasterTerm)
		copy.Sync = nil
		return &pb.DomainChange{Changes: []*pb.EntityChange{{Key: entry.Key, Revision: current + 1,
				Operation: &pb.EntityChange_Upsert{Upsert: &pb.EntityRecord{Value: &pb.EntityRecord_Session{Session: copy}}}}}},
			pb.CommandReply_COMMITTED, current, nil
	}
}

// RequireMasterInbound fences every master-origin frame with the authenticated
// socket identity and its committed term. A sync frame uses requireSync=false;
// operational frames require a fresh completed sync for this generation.
func (p *Projection) RequireMasterInbound(sessionID int32, connectionID, cid string, epoch uint64, requireSync bool) error {
	if p == nil || sessionID < 1 || connectionID == "" || cid == "" || epoch == 0 {
		return fmt.Errorf("invalid socket authority")
	}
	ref := sessionRef(sessionID)
	if err := p.Ready(); err != nil {
		return err
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	subject, _ := Subject(ref)
	state := p.states[subject]
	if state == nil || state.Owner == nil || state.Master == nil || state.Master.ConnectionId != connectionID ||
		state.Master.Cid != cid || state.Master.Epoch != epoch || state.Master.OwnerEpoch != state.Owner.Epoch {
		p.staleEpochs.Add(1)
		return fmt.Errorf("stale session master")
	}
	if err := p.liveSocketLocked(sessionID, connectionID, cid, pb.ClientPresence_EUROSCOPE); err != nil {
		return err
	}
	if requireSync && p.operationalSyncLocked(state, sessionID) == nil {
		return fmt.Errorf("current master has not synced")
	}
	return nil
}

func (p *Projection) RequireLiveSocket(sessionID int32, connectionID, cid string, kind pb.ClientPresence_Kind) error {
	if p == nil {
		return fmt.Errorf("projection unavailable")
	}
	if err := p.Ready(); err != nil {
		return err
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.liveSocketLocked(sessionID, connectionID, cid, kind)
}

func (p *Projection) liveSocketLocked(sessionID int32, connectionID, cid string, kind pb.ClientPresence_Kind) error {
	presence := p.presence["client."+connectionID]
	client := presence.Value.GetClient()
	if client == nil || client.ConnectionId != connectionID || client.Cid != cid || client.SessionId != sessionID ||
		client.Kind != kind || presence.Observed.Before(p.startedAt) ||
		time.Since(presence.Observed) >= 10*time.Second {
		return fmt.Errorf("socket generation is not live")
	}
	node := p.presence["node."+client.NodeId]
	if node.Value.GetNode() == nil || !node.Value.GetNode().Ready || node.Observed.Before(p.startedAt) ||
		time.Since(node.Observed) >= 10*time.Second {
		return fmt.Errorf("socket node is not live")
	}
	return nil
}
