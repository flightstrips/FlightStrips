package euroscopebinary

import (
	"FlightStrips/internal/diagnostics"
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"FlightStrips/internal/cluster"
	"FlightStrips/internal/shared"
	pb "FlightStrips/pkg/events/cluster"
	euroscope "FlightStrips/pkg/events/euroscope"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// DeadlineCandidate binds actual binary observations, scheduling, shared
// retention and reconciliation. Construction starts no loops or sockets.
// Task 20 binds Planner, Serve, Handler.Inbound and SessionWork together.
type DeadlineCandidate struct {
	Router        *cluster.CommandRouter
	Source        cluster.NavigationWeather
	Now           func() time.Time
	Next          cluster.Planner
	NextInbound   func(context.Context, int32, string, string, *euroscope.Envelope) error
	ObservedStrip func(context.Context, int32, string, string, *euroscope.Envelope, *euroscope.Strip) error
	// Full sync admits many observations; allow a bounded longer owner hop.
	SyncAdmissionTimeout time.Duration
	mu                   sync.Mutex
	writers              map[int32]*cluster.PositionWriter
	// Coverage is an optional typed transceiver lookup, used by the same sector
	// and route policy as the SQL runtime; it never receives serialized data.
	Coverage func(string) []string
}

func NewDeadlineCandidate(router *cluster.CommandRouter, source cluster.NavigationWeather, next cluster.Planner) (*DeadlineCandidate, error) {
	if router == nil || router.NC == nil || router.Lease == nil || router.Projection == nil || router.Writer.Store == nil || source.Objects == nil || next == nil {
		return nil, fmt.Errorf("EuroScope deadlines require router, owner, projection, accepted provider source and planner")
	}
	return &DeadlineCandidate{Router: router, Source: source, Next: next, writers: map[int32]*cluster.PositionWriter{}}, nil
}

func (c *DeadlineCandidate) clock() time.Time {
	if c.Now != nil {
		return c.Now().UTC()
	}
	return time.Now().UTC()
}
func candidateRef(id int32) *pb.AggregateRef {
	return &pb.AggregateRef{Target: &pb.AggregateRef_Session{Session: &pb.SessionRef{Id: id}}}
}
func globalCandidateRef() *pb.AggregateRef {
	return &pb.AggregateRef{Target: &pb.AggregateRef_Global{Global: &pb.GlobalRef{}}}
}

func (c *DeadlineCandidate) deadlines() cluster.EuroScopeDeadlinePlanner {
	return cluster.EuroScopeDeadlinePlanner{Projection: c.Router.Projection, Now: c.clock, Next: c.basePlan}
}
func (c *DeadlineCandidate) Planner(ctx context.Context, req *pb.CommandRequest, state *cluster.Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
	return (cluster.SquawkPlanner{Projection: c.Router.Projection, Next: c.deadlines().Plan}).Plan(ctx, req, state)
}
func (c *DeadlineCandidate) basePlan(ctx context.Context, req *pb.CommandRequest, state *cluster.Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
	if proposed := req.GetSystem().GetUpdateEntity().GetValue().GetSession(); proposed != nil && req.Actor.GetId() == "euroscope-session" {
		old := state.Indexes[pb.EntityKind_SESSION][fmt.Sprint(req.Aggregate.GetSession().Id)].GetValue().GetSession()
		if old == nil {
			return nil, pb.CommandReply_NOT_FOUND, 0, fmt.Errorf("session observation seed unavailable")
		}
		copy := proto.Clone(old).(*pb.Session)
		copy.Runways = proposed.Runways
		copy.AvailableSids = proposed.AvailableSids
		if !proto.Equal(copy, proposed) {
			return nil, pb.CommandReply_INVALID_ARGUMENT, 0, fmt.Errorf("EuroScope session may only replace runways and SIDs")
		}
		return cluster.PlanSystemEntity(ctx, req, state)
	}
	if req.Actor.GetId() == "euroscope-controller" || req.Actor.GetId() == "euroscope-controller-offline" {
		controller := req.GetSystem().GetUpdateEntity().GetValue().GetController()
		if controller == nil || state.Master == nil || controller.Callsign == "" || strings.ContainsAny(controller.Callsign+controller.Position, "/ \t\r\n") {
			return nil, pb.CommandReply_UNAUTHORIZED, 0, fmt.Errorf("controller observation requires master and scalar identity")
		}
		change, status, revision := &pb.DomainChange{}, pb.CommandReply_COMMITTED, uint64(0)
		if controller.Cid != "" {
			var err error
			change, status, revision, err = cluster.PlanControllerSector(ctx, req, state)
			if err != nil || status != pb.CommandReply_COMMITTED {
				return change, status, revision, err
			}
		}
		workflow := &pb.WorkflowRecord{WorkflowId: cluster.ControllerObservationID(req.Aggregate.GetSession().Id, controller.Callsign), Source: state.Ref, Destination: state.Ref,
			SourceRevision: &state.Master.Epoch, Step: "euroscope-controller/" + controller.Callsign + "/" + controller.Position, Status: pb.WorkflowRecord_PENDING}
		if controller.Cid == "" && req.Actor.Id == "euroscope-controller" {
			if old := state.Indexes[pb.EntityKind_SESSION_DEADLINE]["controller-offline."+controller.Callsign]; old != nil {
				change.Changes = append(change.Changes, &pb.EntityChange{Key: old.Key, Revision: old.Revision + 1, Operation: &pb.EntityChange_Delete{Delete: &pb.DeleteEntity{Kind: pb.EntityKind_SESSION_DEADLINE}}})
			}
		}
		if req.Actor.Id == "euroscope-controller-offline" {
			workflow.Status = pb.WorkflowRecord_COMPLETED
			key := "controller-offline." + controller.Callsign
			old := state.Indexes[pb.EntityKind_SESSION_DEADLINE][key]
			source := controller.Revision
			if controller.Cid == "" {
				source = state.Master.Epoch
			}
			change.Changes = append(change.Changes, replaceEntity(old, key, &pb.EntityRecord{Value: &pb.EntityRecord_SessionDeadline{SessionDeadline: &pb.SessionDeadline{
				Id: key, Kind: "controller-offline", Callsign: controller.Callsign, SourceRevision: source, DueAt: timestamppb.New(c.clock().Add(cluster.ControllerOfflineGrace))}}}))
		}
		change.Workflows = append(change.Workflows, workflow)
		return change, status, revision, nil
	}
	if req.Actor.GetId() == "euroscope-reconcile" {
		if req.ExpectedEntityRevision == nil || *req.ExpectedEntityRevision != state.Revision || req.GetSystem().GetUpdateEntity().GetValue().GetSessionDeadline() == nil {
			return nil, pb.CommandReply_UNAVAILABLE, state.Revision, fmt.Errorf("session reconciliation source changed")
		}
		changes, err := c.reconcileChanges(ctx, state)
		if err != nil {
			return nil, pb.CommandReply_UNAVAILABLE, 0, err
		}
		return &pb.DomainChange{Changes: changes}, pb.CommandReply_COMMITTED, state.Revision, nil
	}
	return (cluster.SessionWorkerPlanner{Projection: c.Router.Projection, Next: c.Next, Now: c.clock, RetainedAircraft: c.RetainedAircraft}).Plan(ctx, req, state)
}

func candidateReply(reply *pb.CommandReply) error {
	if reply == nil || reply.Status != pb.CommandReply_COMMITTED && reply.Status != pb.CommandReply_PENDING || reply.GetOutcome().GetStatus() == pb.CommandOutcome_FAILED {
		return fmt.Errorf("EuroScope admission failed: status=%s outcome=%s reason=%s detail=%s %s", reply.GetStatus(), reply.GetOutcome().GetStatus(), reply.GetOutcome().GetReasonCode(), reply.GetDetail(), reply.GetOutcome().GetDetail())
	}
	return nil
}
func (c *DeadlineCandidate) sweep(ctx context.Context, id int32, actor string) error {
	for i := 0; i < 8; i++ {
		state, err := c.Router.Projection.Read(candidateRef(id))
		if err != nil {
			return err
		}
		var changes []*pb.EntityChange
		if actor == "euroscope-deadlines" {
			changes, err = c.deadlines().Recover(state)
		} else {
			changes, err = c.reconcileChanges(ctx, state)
		}
		if err != nil {
			return err
		}
		if len(changes) == 0 {
			return nil
		}
		commandID, _ := cluster.ProviderEventCommandID(actor, "euroscope", fmt.Sprintf("%d/%d", id, state.Revision))
		req := &pb.CommandRequest{ProtocolRevision: 1, CommandId: commandID, Aggregate: state.Ref, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: actor}, ExpectedEntityRevision: &state.Revision,
			Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: &pb.UpdateEntity{
				Key: "session-update", Value: &pb.EntityRecord{Value: &pb.EntityRecord_SessionDeadline{SessionDeadline: &pb.SessionDeadline{Id: "session-update", Kind: "session-update", SourceRevision: state.Revision}}}}}}}}
		reply := c.Router.Route(ctx, req)
		if reply.GetStreamSequence() > 0 {
			if err := c.Router.Projection.WaitApplied(ctx, reply.GetStreamSequence()); err != nil {
				return err
			}
		}
		if err = candidateReply(reply); err == nil {
			return nil
		}
		if reply.GetStatus() != pb.CommandReply_UNAVAILABLE && reply.GetStatus() != pb.CommandReply_REVISION_CONFLICT {
			return err
		}
		subject, _ := cluster.Subject(state.Ref)
		wait, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
		_ = c.Router.Projection.WaitSubjectAdvance(wait, subject, state.SubjectSequence)
		cancel()
	}
	return fmt.Errorf("EuroScope sweep source did not stabilize")
}
func (c *DeadlineCandidate) Recover(ctx context.Context, id int32) error {
	if err := c.sweep(ctx, id, "euroscope-deadlines"); err != nil {
		return err
	}
	state, err := c.Router.Projection.Read(candidateRef(id))
	if err != nil {
		return err
	}
	seed := state.Indexes[pb.EntityKind_SESSION][fmt.Sprint(id)].GetValue().GetSession()
	for _, entity := range state.EntitiesByKind(pb.EntityKind_STRIP) {
		s := entity.Value.GetStrip()
		if s.EuroscopeObservedAt == nil || s.Departure != seed.GetAirport() || s.Bay != "NOT_CLEARED" || s.GroundState != "" || cluster.ValidAssignedSquawk(s.AssignedSquawk) {
			continue
		}
		commandID := automaticSquawkID(id, s)
		// An accepted intent, including claimed uncertainty or cancellation, is
		// never recreated by recovery. A subsequent observation has a new ID.
		prior, err := state.LookupOutcome(commandID)
		if err != nil {
			return err
		}
		if prior != nil {
			continue
		}
		effect, err := state.LookupEffect(commandID)
		if err != nil {
			return err
		}
		if effect != nil {
			continue // An effect can outlive the command receipt retention window.
		}
		reply := c.RequestSquawk(ctx, id, commandID, s.Callsign)
		if reply.GetStatus() != pb.CommandReply_UNAVAILABLE && reply.GetOutcome().GetReasonCode() != "SQUAWK_ALREADY_PENDING" {
			if err := candidateReply(reply); err != nil {
				return err
			}
		}
	}
	return nil
}

func automaticSquawkID(id int32, strip *pb.Strip) string {
	commandID, _ := cluster.ProviderEventCommandID("euroscope-auto-squawk", "euroscope", fmt.Sprintf("%d/%s/%d", id, strip.Callsign, strip.EuroscopeObservedAt.AsTime().UnixNano()))
	return commandID
}
func (c *DeadlineCandidate) SessionUpdate(ctx context.Context, id int32) error {
	return c.sweep(ctx, id, "euroscope-reconcile")
}
func (c *DeadlineCandidate) SessionDisconnect(ctx context.Context, id int32) error {
	return c.SessionUpdate(ctx, id)
}
func (c *DeadlineCandidate) BindWorker(worker *cluster.SessionWork) {
	worker.EuroScope, worker.SessionUpdate, worker.SessionDisconnect = c.Recover, c.SessionUpdate, c.SessionDisconnect
	worker.DeadlineCommit = c.commitDeadline
}

func (c *DeadlineCandidate) commitDeadline(ctx context.Context, req *pb.CommandRequest, deadline *pb.SessionDeadline) error {
	if owners := c.Router.Projection.Async; owners != nil {
		// Position admission uses this same owner turn. The deadline planner
		// validates the current observation and source atomically with expiry;
		// cleanup must not require the disconnected source socket to be live.
		return owners.Execute(ctx, req.Aggregate, func(runCtx context.Context) error {
			positions, _, err := c.Router.Projection.ObservationSnapshot(req.Aggregate.GetSession().Id)
			if err != nil {
				return err
			}
			found := false
			for _, position := range positions {
				if position.Value.AircraftKey == deadline.Callsign {
					if position.Stale {
						return nil // Preserve expiry until the new master sync proves its source.
					}
					found = true
					break
				}
			}
			if !found {
				return nil // Replay or the new master sync has not supplied this aircraft yet.
			}
			reply := c.Router.Route(runCtx, req)
			if reply.GetStatus() == pb.CommandReply_COMMITTED && reply.GetOutcome().GetStatus() == pb.CommandOutcome_FAILED &&
				(reply.GetOutcome().GetReasonCode() == "REVISION_CONFLICT" || reply.GetOutcome().GetReasonCode() == "NOT_FOUND") {
				return nil // Reconsider the current deadline on the next pass.
			}
			return candidateReply(reply)
		})
	}
	id := req.Aggregate.GetSession().Id
	state, err := c.Router.Projection.Read(req.Aggregate)
	if err != nil {
		return err
	}
	w, err := c.positionWriter(ctx, id, state.Master.GetConnectionId())
	if err != nil {
		return err
	}
	_, err = w.ExecuteDisconnectContext(ctx, deadline.Callsign, deadline.SourceRevision, func(runCtx context.Context) (*pb.CommandReply, error) {
		reply := c.Router.Route(runCtx, req)
		return reply, candidateReply(reply)
	})
	return err
}

// Inbound forwards one raw Protobuf envelope to the accepted owner. Headers
// contain only authenticated routing identities, rechecked on the owner.
// Ambiguous observation delivery requires a fresh sync, never a blind retry.
func (c *DeadlineCandidate) Inbound(ctx context.Context, id int32, connectionID, cid string, frame *euroscope.Envelope) error {
	if err := c.Router.Projection.ValidateEuroScopeInbound(id, connectionID, cid, frame); err != nil {
		return err
	}
	owner, err := c.Router.Projection.ReadOwner(candidateRef(id))
	if err != nil || owner == nil {
		return fmt.Errorf("socket owner unavailable")
	}
	if owner.NodeId == c.Router.Lease.NodeID {
		return c.admit(ctx, id, connectionID, cid, frame)
	}
	data, err := proto.Marshal(frame)
	if err != nil || len(data) > cluster.MaxStateBytes {
		return fmt.Errorf("invalid socket admission size")
	}
	msg := nats.NewMsg("fs.v1.euroscope." + owner.NodeId)
	msg.Data = data
	msg.Header.Set("FS-Session", strconv.Itoa(int(id)))
	msg.Header.Set("FS-Connection", connectionID)
	msg.Header.Set("FS-CID", cid)
	reply, err := c.Router.NC.RequestMsgWithContext(ctx, msg)
	if err != nil {
		return err
	}
	result := &pb.EffectDeliveryReply{}
	if pb.UnmarshalStrict(reply.Data, result) != nil || !result.Accepted {
		return fmt.Errorf("owner refused socket admission")
	}
	return nil
}
func (c *DeadlineCandidate) Serve(ctx context.Context) error {
	positions := shared.NewPositionDispatcher(32, 1024, nil)
	stop := context.AfterFunc(ctx, positions.Cancel)
	defer stop()
	defer func() {
		drain, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = positions.Close(drain)
	}()
	_, closeSub, err := cluster.SubscribeJoined(c.Router.NC, "fs.v1.euroscope."+c.Router.Lease.NodeID, func(msg *nats.Msg) {
		run := func(runCtx context.Context) {
			result := &pb.EffectDeliveryReply{}
			id, err := strconv.ParseInt(msg.Header.Get("FS-Session"), 10, 32)
			frame := &euroscope.Envelope{}
			if err == nil && id > 0 && len(msg.Data) > 0 && len(msg.Data) <= cluster.MaxStateBytes && pb.UnmarshalStrict(msg.Data, frame) == nil {
				timeout := 5 * time.Second
				if frame.GetSync() != nil {
					timeout = c.SyncAdmissionTimeout
					if timeout <= 0 {
						timeout = 2 * time.Minute
					}
				}
				hop, cancel := context.WithTimeout(runCtx, timeout)
				admitErr := c.admit(hop, int32(id), msg.Header.Get("FS-Connection"), msg.Header.Get("FS-CID"), frame)
				result.Accepted = admitErr == nil
				if admitErr != nil {
					slog.WarnContext(hop, "EuroScope owner admission failed", "session_id", id, "event_type", diagnostics.OneofName(frame, "event"), "error", diagnostics.Message(admitErr), "error_reason", inboundFailureReason(admitErr))
				}
				cancel()
			}
			data, _ := proto.Marshal(result)
			if msg.Reply != "" {
				_ = msg.Respond(data)
			}
		}
		frame := &euroscope.Envelope{}
		if pb.UnmarshalStrict(msg.Data, frame) == nil && independentInboundKey(frame) != "" {
			key := msg.Header.Get("FS-Session") + "/" + independentInboundKey(frame)
			if positions.Submit(ctx, key, run) == nil {
				return
			}
			data, _ := proto.Marshal(&pb.EffectDeliveryReply{})
			_ = msg.Respond(data)
			return
		}
		if positions.Barrier(ctx) != nil {
			return
		}
		run(ctx)
	})
	if err != nil {
		return err
	}
	defer closeSub()
	if err = cluster.FlushSubscription(ctx, c.Router.NC); err != nil {
		return err
	}
	<-ctx.Done()
	return ctx.Err()
}

func (c *DeadlineCandidate) executeFrame(ctx context.Context, id int32, connection, cid string, frame *euroscope.Envelope, actor, key string, record *pb.EntityRecord, revision uint64, operation string) error {
	commandID, _ := cluster.ProviderEventCommandID("euroscope-event", connection, fmt.Sprintf("%s/%s/%s/%d", frame.CommandId, operation, key, revision))
	req := &pb.CommandRequest{ProtocolRevision: 1, CommandId: commandID, Aggregate: candidateRef(id), Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: actor}, ExpectedEntityRevision: &revision,
		Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: &pb.UpdateEntity{Key: key, Value: record}}}}}
	w := c.Router.Writer
	w.Plan = func(ctx context.Context, req *pb.CommandRequest, state *cluster.Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
		if err := c.Router.Projection.ValidateEuroScopeInbound(id, connection, cid, frame); err != nil {
			return nil, pb.CommandReply_UNAUTHORIZED, 0, err
		}
		if operation == "assigned-squawk" {
			return planStripObservation(ctx, req, state, key, func(strip *pb.Strip) { strip.AssignedSquawk = record.GetStrip().AssignedSquawk }, c.Planner)
		}
		return planFrameObservation(ctx, req, state, c.Planner)
	}
	if err := candidateReply(w.Execute(ctx, req)); err != nil {
		return fmt.Errorf("%s %s: %w", operation, key, err)
	}
	return nil
}

func (c *DeadlineCandidate) RequestSquawk(ctx context.Context, id int32, commandID, callsign string) *pb.CommandReply {
	state, err := c.Router.Projection.Read(candidateRef(id))
	if err != nil {
		return &pb.CommandReply{Status: pb.CommandReply_UNAVAILABLE}
	}
	_, presence, err := c.Router.Projection.ObservationSnapshot(id)
	if err != nil {
		return &pb.CommandReply{Status: pb.CommandReply_UNAVAILABLE, Detail: err.Error()}
	}
	controllers, err := (cluster.ControllerSector{Store: cluster.RoutedLifecycleStore{Projection: c.Router.Projection}}).OperationalControllers(ctx, id, presence, c.clock())
	if err != nil {
		return &pb.CommandReply{Status: pb.CommandReply_UNAVAILABLE, Detail: err.Error()}
	}
	target := ""
	effect, err := state.LookupEffect(commandID)
	if err != nil {
		return &pb.CommandReply{Status: pb.CommandReply_UNAVAILABLE, Detail: err.Error()}
	}
	if effect != nil && effect.GetGenerateSquawk().GetCallsign() == strings.ToUpper(strings.TrimSpace(callsign)) {
		target = effect.TargetCid
	}
	for _, controller := range controllers {
		if target != "" {
			break
		}
		for _, sector := range state.EntitiesByKind(pb.EntityKind_SECTOR_OWNER) {
			s := sector.Value.GetSectorOwner()
			if s.Sector == "DEL" && s.Position == controller.Position && !controller.Observer {
				target = controller.Cid
				break
			}
		}
		if target != "" {
			break
		}
	}
	if target == "" {
		target = state.Master.GetCid()
	}
	if target == "" {
		return &pb.CommandReply{Status: pb.CommandReply_UNAVAILABLE, Detail: "squawk awaits operational target"}
	}
	return c.Router.Route(ctx, &pb.CommandRequest{ProtocolRevision: 1, CommandId: commandID, Aggregate: state.Ref, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "euroscope-squawk"},
		Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_RequestSquawk{RequestSquawk: &pb.RequestSquawk{Callsign: strings.ToUpper(strings.TrimSpace(callsign)), TargetCid: target}}}}})
}

func sortChanges(changes []*pb.EntityChange) {
	sort.Slice(changes, func(i, j int) bool {
		kind := func(c *pb.EntityChange) int {
			if c.GetDelete() != nil {
				return int(c.GetDelete().Kind)
			}
			m := c.GetUpsert().ProtoReflect()
			return int(m.WhichOneof(m.Descriptor().Oneofs().ByName("value")).Number())
		}
		ki, kj := kind(changes[i]), kind(changes[j])
		if ki != kj {
			return ki < kj
		}
		return changes[i].Key < changes[j].Key
	})
}
func replaceEntity(old *pb.EntitySnapshot, key string, record *pb.EntityRecord) *pb.EntityChange {
	rev := uint64(1)
	if old != nil {
		rev = old.Revision + 1
	}
	return &pb.EntityChange{Key: key, Revision: rev, Operation: &pb.EntityChange_Upsert{Upsert: record}}
}

func (c *DeadlineCandidate) Close(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for id, w := range c.writers {
		if err := w.Close(ctx); err != nil {
			return err
		}
		delete(c.writers, id)
	}
	return nil
}
