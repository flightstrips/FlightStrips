package cluster

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const sessionCleanupGrace = 5 * time.Minute

// SessionWork runs session policy through accepted ownership.
// Every mutation goes through the session owner and its subject-CAS writer.
// Tick is deliberately short: no local timer is an authority or a deadline.
type SessionWork struct {
	Registry   SessionRegistry
	Store      LifecycleStore
	Projection *Projection
	Owner      interface {
		CanWrite(*pb.AggregateRef) bool
		Track(*pb.AggregateRef) error
	}
	Now      func() time.Time
	Interval time.Duration

	// These reconcilers may read external observations, but must submit every
	// derived mutation as an owner-routed typed command. They are not invoked
	// while the lease or projection is unhealthy.
	PDC               func(context.Context, int32) error
	Departure         func(context.Context, int32) error
	Arrival           func(context.Context, int32) error
	CDM               func(context.Context, int32) error
	Traffic           func(context.Context, int32) error
	SessionUpdate     func(context.Context, int32) error
	SessionDisconnect func(context.Context, int32) error
	// Rebuild socket deadlines from accepted identities and observations before
	// reading due work. Task 18c binds its concrete recovery adapter here.
	EuroScope func(context.Context, int32) error
	// Task18c holds the owner position barrier while an aircraft deadline fires.
	DeadlineCommit func(context.Context, *pb.CommandRequest, *pb.SessionDeadline) error

	mu             sync.Mutex
	unhealthySince time.Time
	recoveredAt    time.Time
}

func (w *SessionWork) clock() time.Time {
	if w.Now != nil {
		return w.Now().UTC()
	}
	return time.Now().UTC()
}

// ReconcileSession runs the same healthy owner pass as the registry supervisor.
// Callers already holding an admitted session identity can use this entry point
// without constructing a second timer or a separate deadline implementation.
func (w *SessionWork) ReconcileSession(ctx context.Context, id int32) error {
	if w.Store == nil || w.Projection == nil || w.Owner == nil || id <= 0 {
		return fmt.Errorf("session worker is incomplete")
	}
	if err := w.Projection.Ready(); err != nil {
		return err
	}
	if !w.Owner.CanWrite(sessionRef(id)) {
		return fmt.Errorf("session lease unavailable")
	}
	return w.stepSession(ctx, &pb.SessionRegistry{Id: id}, time.Time{}, time.Time{}, 0)
}

func (w *SessionWork) Run(ctx context.Context) error {
	if w.Store == nil || w.Registry.Store == nil || w.Projection == nil || w.Owner == nil {
		return fmt.Errorf("session worker requires registry, store, projection and owner")
	}
	interval := w.Interval
	if interval <= 0 {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	lastError := ""
	for {
		if err := w.Step(ctx); err != nil {
			if err.Error() != lastError && ctx.Err() == nil {
				slog.WarnContext(ctx, "session worker pass failed", slog.String("error_type", fmt.Sprintf("%T", err)))
			}
			lastError = err.Error()
		} else if lastError != "" {
			slog.InfoContext(ctx, "session worker resumed after failure")
			lastError = ""
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// Step is also the deterministic test seam. It rebuilds due work from the
// current projection on every pass, including the first pass after takeover.
func (w *SessionWork) Step(ctx context.Context) error {
	if w.Store == nil || w.Registry.Store == nil || w.Projection == nil || w.Owner == nil {
		return fmt.Errorf("session worker is incomplete")
	}
	if err := w.Projection.Ready(); err != nil {
		w.mu.Lock()
		if w.unhealthySince.IsZero() {
			w.unhealthySince = w.clock()
		}
		// A brief metadata recovery can precede ownership/pause persistence.
		// Renewed failure must extend that incomplete recovery interval.
		w.recoveredAt = time.Time{}
		w.mu.Unlock()
		return err
	}
	w.mu.Lock()
	paused := time.Duration(0)
	if !w.unhealthySince.IsZero() {
		if w.recoveredAt.IsZero() {
			w.recoveredAt = w.clock()
		}
		paused = w.recoveredAt.Sub(w.unhealthySince)
	}
	unhealthySince, recoveredAt := w.unhealthySince, w.recoveredAt
	w.mu.Unlock()
	sessions, err := w.Registry.ActiveSessions(ctx)
	if err != nil {
		w.mu.Lock()
		if w.unhealthySince.IsZero() {
			w.unhealthySince = w.clock()
		}
		w.recoveredAt = time.Time{}
		w.mu.Unlock()
		return err
	}
	var first error
	for _, session := range sessions {
		ref := sessionRef(session.Id)
		if err := w.Owner.Track(ref); err != nil {
			if first == nil {
				first = err
			}
			continue
		}
		if !w.Owner.CanWrite(ref) {
			continue
		}
		if err := w.stepSession(ctx, session, unhealthySince, recoveredAt, paused); err != nil && first == nil {
			first = err
		}
	}
	// Keep the frozen recovery interval until every live session has either
	// recorded the pause or acquired a controller. A later owner can then
	// recover a session without counting the outage as healthy time.
	// Reconciler errors unrelated to cleanup must not retain an interval whose
	// pause is already durable for every session.
	allRecorded := true
	if paused > 0 {
		for _, session := range sessions {
			state, err := w.Store.Read(ctx, sessionRef(session.Id))
			if err != nil {
				allRecorded = false
				break
			}
			seed := state.Indexes[pb.EntityKind_SESSION][strconv.Itoa(int(session.Id))]
			if seed == nil {
				allRecorded = false
				break
			}
			value := seed.GetValue().GetSession()
			if value != nil && !value.Tombstoned && value.FirstNoControllerAt != nil && (value.CleanupPausedAt == nil || value.CleanupPausedAt.AsTime().Before(recoveredAt)) {
				allRecorded = false
				break
			}
		}
	}
	if allRecorded {
		w.mu.Lock()
		w.unhealthySince = time.Time{}
		w.recoveredAt = time.Time{}
		w.mu.Unlock()
	}
	return first
}

func (w *SessionWork) stepSession(ctx context.Context, registry *pb.SessionRegistry, unhealthySince, recoveredAt time.Time, paused time.Duration) error {
	id, now := registry.Id, w.clock()
	var deferred []error
	if w.EuroScope != nil {
		if err := w.EuroScope(ctx, id); err != nil {
			deferred = append(deferred, err)
		}
	}
	state, err := w.Store.Read(ctx, sessionRef(id))
	if err != nil {
		return err
	}
	seed := state.Indexes[pb.EntityKind_SESSION][strconv.Itoa(int(id))]
	if seed == nil || seed.GetValue().GetSession() == nil {
		return fmt.Errorf("session %d has no seed", id)
	}
	if seed.GetValue().GetSession().Tombstoned {
		return w.Registry.FinalizeDeletion(ctx, id)
	}
	_, entries, err := w.Projection.ObservationSnapshot(id)
	if err != nil {
		return err
	}
	presence, err := operationalSessionPresence(entries, id, now)
	if err != nil {
		return err
	}
	session := seed.GetValue().GetSession()
	marker := session.FirstNoControllerAt
	fullRestart := marker != nil && presence.oldestNode.After(marker.AsTime())
	alreadyPaused := !unhealthySince.IsZero() && session.CleanupPausedAt != nil && !session.CleanupPausedAt.AsTime().Before(recoveredAt)
	if presence.controllers || marker == nil || fullRestart || paused > 0 && !alreadyPaused {
		var next *timestamppb.Timestamp
		var pausedAt *timestamppb.Timestamp
		if !presence.controllers {
			at := now
			if fullRestart {
				pausedAt = timestamppb.New(now)
			}
			if marker == nil && paused > 0 {
				pausedAt = timestamppb.New(recoveredAt)
			}
			if marker != nil && !fullRestart && paused > 0 && !alreadyPaused {
				remaining := paused
				if session.CleanupPausedAt != nil && session.CleanupPausedAt.AsTime().After(unhealthySince) {
					// Another session/replica may have persisted the earlier portion
					// before recovery failed again. Extend only the missing portion.
					remaining = recoveredAt.Sub(session.CleanupPausedAt.AsTime())
				}
				at = marker.AsTime().Add(remaining)
				if at.After(now) {
					at = now
				}
				pausedAt = timestamppb.New(recoveredAt)
			}
			next = timestamppb.New(at)
		}
		if marker == nil && next != nil || marker != nil && next == nil || marker != nil && next != nil && !marker.AsTime().Equal(next.AsTime()) {
			if err := w.writeMarker(ctx, id, seed, next, pausedAt); err != nil {
				return err
			}
		}
	}
	cleanupDue := !presence.controllers && marker != nil && !fullRestart && (paused == 0 || alreadyPaused) && !now.Before(marker.AsTime().Add(sessionCleanupGrace))
	// Re-read each entity at dispatch time. The planner repeats revision and
	// source checks immediately before the CAS publish.
	for _, deadline := range state.EntitiesByKind(pb.EntityKind_SESSION_DEADLINE) {
		value := deadline.GetValue().GetSessionDeadline()
		if value != nil && (value.Kind == "pdc-poll" || value.Kind == "pdc-response" && w.PDC != nil) {
			continue // The concrete PDC callback owns provider and response transitions.
		}
		// CDM consumes and rearms its deadlines atomically with its domain
		// result. The generic expiry path must never delete that work first.
		if value != nil && strings.HasPrefix(value.Kind, "cdm-") {
			if w.CDM == nil {
				return fmt.Errorf("CDM reconciler is not configured")
			}
			continue
		}
		if value == nil || value.DueAt == nil || now.Before(value.DueAt.AsTime()) {
			continue
		}
		if value.Kind == "session-update" || value.Kind == "session-disconnect" {
			run := w.SessionUpdate
			if value.Kind == "session-disconnect" {
				run = w.SessionDisconnect
			}
			if run == nil {
				return fmt.Errorf("%s reconciler is not configured", value.Kind)
			}
			if err := run(ctx, id); err != nil {
				return err
			}
		}
		if err := w.fireDeadline(ctx, id, deadline); err != nil {
			return err
		}
	}
	for _, kind := range []pb.EntityKind{pb.EntityKind_STAND_ASSIGNMENT, pb.EntityKind_STAND_BLOCK} {
		for _, entity := range state.EntitiesByKind(kind) {
			// Concrete lifecycle callbacks own their persisted expiry policy,
			// including wrong-stand episodes and retained physical occupancy.
			if kind == pb.EntityKind_STAND_ASSIGNMENT {
				assignment := entity.GetValue().GetStandAssignment()
				if assignment.Direction == "DEPARTURE" && w.Departure != nil || assignment.Direction == "ARRIVAL" && w.Arrival != nil {
					continue
				}
			}
			if kind == pb.EntityKind_STAND_BLOCK && w.Arrival != nil {
				continue
			}
			var due *timestamppb.Timestamp
			if kind == pb.EntityKind_STAND_ASSIGNMENT {
				due = entity.GetValue().GetStandAssignment().ExpiresAt
			} else {
				due = entity.GetValue().GetStandBlock().ExpiresAt
			}
			if due != nil && !now.Before(due.AsTime()) {
				if err := w.expireStand(ctx, id, kind, entity); err != nil {
					return err
				}
			}
		}
	}
	for _, run := range []func(context.Context, int32) error{w.PDC, w.Departure, w.Arrival, w.CDM, w.Traffic} {
		if run == nil {
			continue
		}
		if !w.Owner.CanWrite(sessionRef(id)) {
			return fmt.Errorf("session %d lease lost", id)
		}
		if err := run(ctx, id); err != nil {
			deferred = append(deferred, err)
		}
	}
	if cleanupDue {
		// A deadline or reconciler may have changed the session revision in
		// this pass. Recheck the source before asking the owner to tombstone.
		fresh, err := w.Store.Read(ctx, sessionRef(id))
		if err != nil {
			return err
		}
		current := fresh.Indexes[pb.EntityKind_SESSION][strconv.Itoa(int(id))]
		if current == nil {
			return fmt.Errorf("session %d disappeared before cleanup", id)
		}
		if err := w.tombstone(ctx, id, registry.WorkflowId, current.Revision); err != nil {
			return err
		}
		return w.Registry.FinalizeDeletion(ctx, id)
	}
	return errors.Join(deferred...)
}

type sessionPresence struct {
	controllers bool
	oldestNode  time.Time
}

// Frontend connections and persisted Controller rows do not count. Both a
// EuroScope client lease and its hosting node lease must be fresh and ready.
func operationalSessionPresence(entries []KVPresence, sessionID int32, now time.Time) (sessionPresence, error) {
	nodes := map[string]time.Time{}
	for _, item := range entries {
		node := item.Value.GetNode()
		if node != nil && node.Ready && node.StartedAt != nil && !item.Observed.After(now) && now.Sub(item.Observed) < nodeTTL {
			nodes[node.NodeId] = node.StartedAt.AsTime()
		}
	}
	if len(nodes) == 0 {
		return sessionPresence{}, fmt.Errorf("no fresh ready node presence")
	}
	result := sessionPresence{}
	for _, started := range nodes {
		if result.oldestNode.IsZero() || started.Before(result.oldestNode) {
			result.oldestNode = started
		}
	}
	for _, item := range entries {
		client := item.Value.GetClient()
		if client != nil && client.SessionId == sessionID && client.Kind == pb.ClientPresence_EUROSCOPE && !client.Observer && !item.Observed.After(now) && now.Sub(item.Observed) < nodeTTL {
			if _, ok := nodes[client.NodeId]; ok {
				result.controllers = true
				break
			}
		}
	}
	return result, nil
}

func sessionWorkerRequest(id int32, commandID string, expected uint64, action *pb.SystemCommand) *pb.CommandRequest {
	return &pb.CommandRequest{ProtocolRevision: 1, CommandId: commandID, Aggregate: sessionRef(id),
		Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "session-worker"}, ExpectedEntityRevision: &expected,
		Command: &pb.CommandRequest_System{System: action}}
}

func workerCommandID(id int32, kind, key string, revision uint64) string {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(fmt.Sprintf("flightstrips/session-worker/%d/%s/%s/%d", id, kind, key, revision))).String()
}

func (w *SessionWork) execute(ctx context.Context, request *pb.CommandRequest) error {
	if !w.Owner.CanWrite(request.Aggregate) {
		return fmt.Errorf("session lease lost")
	}
	reply := w.Store.Execute(ctx, request)
	if reply == nil || reply.Status != pb.CommandReply_COMMITTED || reply.GetOutcome().GetStatus() == pb.CommandOutcome_FAILED {
		if reply == nil {
			return fmt.Errorf("session worker received no reply")
		}
		return fmt.Errorf("session worker %s: %s: %s %s", request.CommandId, reply.Status, reply.Detail, reply.GetOutcome().GetDetail())
	}
	return nil
}

func (w *SessionWork) writeMarker(ctx context.Context, id int32, seed *pb.EntitySnapshot, marker, pausedAt *timestamppb.Timestamp) error {
	copy := proto.Clone(seed.GetValue().GetSession()).(*pb.Session)
	copy.FirstNoControllerAt = marker
	copy.CleanupPausedAt = pausedAt
	key := seed.Key
	record := &pb.EntityRecord{Value: &pb.EntityRecord_Session{Session: copy}}
	action := &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: &pb.UpdateEntity{Key: key, Value: record}}}
	request := sessionWorkerRequest(id, workerCommandID(id, "presence", key+"/"+w.clock().Format(time.RFC3339Nano), seed.Revision), seed.Revision,
		action)
	return w.execute(ctx, request)
}

func (w *SessionWork) tombstone(ctx context.Context, id int32, workflow string, revision uint64) error {
	request := sessionWorkerRequest(id, workerCommandID(id, "cleanup", workflow, revision), revision,
		&pb.SystemCommand{Action: &pb.SystemCommand_DeleteSession{DeleteSession: &pb.DeleteSession{Id: id, WorkflowId: workflow}}})
	return w.execute(ctx, request)
}

func (w *SessionWork) fireDeadline(ctx context.Context, id int32, entity *pb.EntitySnapshot) error {
	d := entity.GetValue().GetSessionDeadline()
	identity := fmt.Sprintf("%s/%s/%d", entity.Key, d.GetDueAt().AsTime().UTC().Format(time.RFC3339Nano), d.SourceRevision)
	request := sessionWorkerRequest(id, workerCommandID(id, "deadline", identity, entity.Revision), entity.Revision,
		&pb.SystemCommand{Action: &pb.SystemCommand_RemoveEntity{RemoveEntity: &pb.RemoveEntity{Key: entity.Key, Kind: pb.EntityKind_SESSION_DEADLINE}}})
	if d.Kind == "aircraft-disconnect" && w.DeadlineCommit != nil {
		return w.DeadlineCommit(ctx, request, d)
	}
	return w.execute(ctx, request)
}

func (w *SessionWork) expireStand(ctx context.Context, id int32, kind pb.EntityKind, entity *pb.EntitySnapshot) error {
	identity := entity.Key
	if kind == pb.EntityKind_STAND_ASSIGNMENT {
		v := entity.GetValue().GetStandAssignment()
		identity += "/" + v.GetCreatedAt().AsTime().UTC().Format(time.RFC3339Nano)
	} else {
		v := entity.GetValue().GetStandBlock()
		identity += "/" + v.GetCreatedAt().AsTime().UTC().Format(time.RFC3339Nano)
	}
	request := sessionWorkerRequest(id, workerCommandID(id, "stand", identity, entity.Revision), entity.Revision,
		&pb.SystemCommand{Action: &pb.SystemCommand_RemoveEntity{RemoveEntity: &pb.RemoveEntity{Key: entity.Key, Kind: kind}}})
	return w.execute(ctx, request)
}

// ScheduleDeadline persists a timer with the exact source entity revision.
// Replacing the source or deadline makes an old firing command ineffective.
func (w *SessionWork) ScheduleDeadline(ctx context.Context, sessionID int32, deadline *pb.SessionDeadline) error {
	if deadline == nil || deadline.SourceRevision == 0 || deadline.DueAt == nil || deadline.DueAt.CheckValid() != nil {
		return fmt.Errorf("deadline needs due time and source revision")
	}
	state, err := w.Store.Read(ctx, sessionRef(sessionID))
	if err != nil {
		return err
	}
	switch deadline.Kind {
	case "pdc-response":
		source := state.Indexes[pb.EntityKind_PDC_SEQUENCE][deadline.Callsign]
		if source == nil || source.Revision != deadline.SourceRevision {
			return fmt.Errorf("PDC source revision changed")
		}
	case "controller-offline":
		found := false
		for _, source := range state.EntitiesByKind(pb.EntityKind_CONTROLLER) {
			if source.GetValue().GetController().Callsign == deadline.Callsign && source.Revision == deadline.SourceRevision {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("controller source revision changed")
		}
	case "aircraft-disconnect":
		if w.Projection == nil {
			return fmt.Errorf("position projection unavailable")
		}
		positions, _, err := w.Projection.ObservationSnapshot(sessionID)
		if err != nil {
			return err
		}
		found := false
		for _, source := range positions {
			if source.Value.AircraftKey == deadline.Callsign && source.Revision == deadline.SourceRevision && source.Value.GetTombstone() != nil && !source.Stale {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("aircraft disconnect source revision changed")
		}
	case "session-update", "session-disconnect", "session-cleanup":
		source := state.Indexes[pb.EntityKind_SESSION][strconv.Itoa(int(sessionID))]
		if source == nil || source.Revision != deadline.SourceRevision {
			return fmt.Errorf("session source revision changed")
		}
	default:
		return fmt.Errorf("unsupported deadline kind %q", deadline.Kind)
	}
	old := state.Indexes[pb.EntityKind_SESSION_DEADLINE][deadline.Id]
	revision := uint64(0)
	if old != nil {
		if proto.Equal(old.GetValue().GetSessionDeadline(), deadline) {
			return nil
		}
		revision = old.Revision
	}
	record := &pb.EntityRecord{Value: &pb.EntityRecord_SessionDeadline{SessionDeadline: proto.Clone(deadline).(*pb.SessionDeadline)}}
	action := &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: &pb.UpdateEntity{Key: deadline.Id, Value: record}}}
	identity := fmt.Sprintf("%s/%s/%d", deadline.Id, deadline.DueAt.AsTime().UTC().Format(time.RFC3339Nano), deadline.SourceRevision)
	return w.execute(ctx, sessionWorkerRequest(sessionID, workerCommandID(sessionID, "schedule", identity, revision), revision,
		action))
}

// sortedSessionIDs helps callers build deterministic worker diagnostics.
func sortedSessionIDs(sessions []*pb.SessionRegistry) []int32 {
	ids := make([]int32, 0, len(sessions))
	for _, session := range sessions {
		ids = append(ids, session.Id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}
