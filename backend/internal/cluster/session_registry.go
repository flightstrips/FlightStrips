package cluster

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
)

// LifecycleStore is the boundary for task 13's owner router. The local writer
// is sufficient for the isolated runtime; production routing can implement the
// same two operations without changing the registry workflow.
type LifecycleStore interface {
	Execute(context.Context, *pb.CommandRequest) *pb.CommandReply
	Read(context.Context, *pb.AggregateRef) (*Aggregate, error)
}

type LocalLifecycleStore struct{ Writer Writer }

func (s LocalLifecycleStore) Execute(ctx context.Context, request *pb.CommandRequest) *pb.CommandReply {
	return s.Writer.Execute(ctx, request)
}
func (s LocalLifecycleStore) ExecuteDurable(ctx context.Context, request *pb.CommandRequest) *pb.CommandReply {
	reply, _ := s.Writer.ExecuteFresh(ctx, request)
	return reply
}
func (s LocalLifecycleStore) ReadDurable(ctx context.Context, ref *pb.AggregateRef) (*Aggregate, error) {
	if s.Writer.Projection != nil {
		return s.Writer.Projection.ReadDurable(ref)
	}
	subject, err := Subject(ref)
	if err != nil {
		return nil, err
	}
	return s.Writer.load(context.WithValue(ctx, durableExecutionKey{}, true), subject, ref)
}
func (s LocalLifecycleStore) Read(ctx context.Context, ref *pb.AggregateRef) (*Aggregate, error) {
	subject, err := Subject(ref)
	if err != nil {
		return nil, err
	}
	return s.Writer.load(ctx, subject, ref)
}

// RoutedLifecycleStore is the candidate multi-node adapter used by socket
// handlers. Commands route to the accepted owner; reads use this node's own
// applied projection.
type RoutedLifecycleStore struct {
	Router interface {
		Route(context.Context, *pb.CommandRequest) *pb.CommandReply
	}
	Projection *Projection
}

func (s RoutedLifecycleStore) Execute(ctx context.Context, request *pb.CommandRequest) *pb.CommandReply {
	if s.Router == nil {
		return unavailable(request.GetCommandId())
	}
	return s.Router.Route(ctx, request)
}
func (s RoutedLifecycleStore) ExecuteDurable(ctx context.Context, request *pb.CommandRequest) *pb.CommandReply {
	if router, ok := s.Router.(interface {
		RouteDurable(context.Context, *pb.CommandRequest) *pb.CommandReply
	}); ok {
		return router.RouteDurable(ctx, request)
	}
	return s.Execute(ctx, request)
}
func (s RoutedLifecycleStore) ReadDurable(_ context.Context, ref *pb.AggregateRef) (*Aggregate, error) {
	if s.Projection == nil {
		return nil, fmt.Errorf("projection unavailable")
	}
	return s.Projection.ReadDurable(ref)
}

func (s RoutedLifecycleStore) Read(_ context.Context, ref *pb.AggregateRef) (*Aggregate, error) {
	if s.Projection == nil {
		return nil, fmt.Errorf("projection unavailable")
	}
	return s.Projection.Read(ref)
}

func (s RoutedLifecycleStore) ReadEntities(_ context.Context, ref *pb.AggregateRef, kind pb.EntityKind) ([]*pb.EntitySnapshot, error) {
	if s.Projection == nil {
		return nil, fmt.Errorf("projection unavailable")
	}
	return s.Projection.ReadEntities(ref, kind)
}

func (s RoutedLifecycleStore) ReadDurableEntity(_ context.Context, ref *pb.AggregateRef, kind pb.EntityKind, key string) (*pb.EntitySnapshot, error) {
	if s.Projection == nil {
		return nil, fmt.Errorf("projection unavailable")
	}
	return s.Projection.ReadDurableEntity(ref, kind, key)
}

func (s LocalLifecycleStore) ReadDurableEntity(ctx context.Context, ref *pb.AggregateRef, kind pb.EntityKind, key string) (*pb.EntitySnapshot, error) {
	if s.Writer.Projection != nil {
		return s.Writer.Projection.ReadDurableEntity(ref, kind, key)
	}
	state, err := s.ReadDurable(ctx, ref)
	if err != nil {
		return nil, err
	}
	return state.Indexes[kind][key], nil
}

func (r SessionRegistry) persistedSeed(ctx context.Context, id int32) (*pb.EntitySnapshot, error) {
	key := strconv.Itoa(int(id))
	if reader, ok := r.Store.(interface {
		ReadDurableEntity(context.Context, *pb.AggregateRef, pb.EntityKind, string) (*pb.EntitySnapshot, error)
	}); ok {
		return reader.ReadDurableEntity(ctx, sessionRef(id), pb.EntityKind_SESSION, key)
	}
	state, err := r.read(ctx, sessionRef(id))
	if err != nil {
		return nil, err
	}
	return state.Indexes[pb.EntityKind_SESSION][key], nil
}

func (r SessionRegistry) registryEntities(ctx context.Context) ([]*pb.EntitySnapshot, error) {
	if reader, ok := r.Store.(interface {
		ReadEntities(context.Context, *pb.AggregateRef, pb.EntityKind) ([]*pb.EntitySnapshot, error)
	}); ok {
		return reader.ReadEntities(ctx, globalRef(), pb.EntityKind_SESSION_REGISTRY)
	}
	state, err := r.read(ctx, globalRef())
	if err != nil {
		return nil, err
	}
	return state.EntitiesByKind(pb.EntityKind_SESSION_REGISTRY), nil
}

// SessionRegistry is an opt-in candidate adapter. The SQL-backed Server does
// not construct it until the coordinated cutover.
type SessionRegistry struct{ Store LifecycleStore }

func globalRef() *pb.AggregateRef {
	return &pb.AggregateRef{Target: &pb.AggregateRef_Global{Global: &pb.GlobalRef{}}}
}
func sessionRef(id int32) *pb.AggregateRef {
	return &pb.AggregateRef{Target: &pb.AggregateRef_Session{Session: &pb.SessionRef{Id: id}}}
}
func lifecycleID(workflow, step string) string {
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte("flightstrips/session/"+workflow+"/"+step)).String()
}
func lifecycleRequest(ref *pb.AggregateRef, id string, action any) *pb.CommandRequest {
	system := &pb.SystemCommand{}
	switch x := action.(type) {
	case *pb.SystemCommand_CreateSession:
		system.Action = x
	case *pb.SystemCommand_DeleteSession:
		system.Action = x
	}
	return &pb.CommandRequest{ProtocolRevision: 1, CommandId: id, Aggregate: ref,
		Actor:   &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "session-registry"},
		Command: &pb.CommandRequest_System{System: system}}
}

func (r SessionRegistry) read(ctx context.Context, ref *pb.AggregateRef) (*Aggregate, error) {
	return r.readDurable(ctx, ref)
}
func (r SessionRegistry) readDurable(ctx context.Context, ref *pb.AggregateRef) (*Aggregate, error) {
	if r.Store == nil {
		return nil, fmt.Errorf("session registry store is unavailable")
	}
	if reader, ok := r.Store.(interface {
		ReadDurable(context.Context, *pb.AggregateRef) (*Aggregate, error)
	}); ok {
		return reader.ReadDurable(ctx, ref)
	}
	return r.Store.Read(ctx, ref)
}
func (r SessionRegistry) execute(ctx context.Context, request *pb.CommandRequest) error {
	if r.Store == nil {
		return fmt.Errorf("session registry store is unavailable")
	}
	var reply *pb.CommandReply
	if store, ok := r.Store.(interface {
		ExecuteDurable(context.Context, *pb.CommandRequest) *pb.CommandReply
	}); ok {
		reply = store.ExecuteDurable(ctx, request)
	} else {
		reply = r.Store.Execute(ctx, request)
	}
	if reply != nil && reply.MemoryAccepted {
		return fmt.Errorf("session lifecycle prerequisite is not durable")
	}
	if reply == nil || reply.Status != pb.CommandReply_COMMITTED || reply.GetOutcome().GetStatus() == pb.CommandOutcome_FAILED {
		if reply == nil {
			return fmt.Errorf("session lifecycle command received no reply")
		}
		return fmt.Errorf("session lifecycle command %s: %s: %s (%s: %s)", request.CommandId, reply.Status, reply.Detail, reply.GetOutcome().GetReasonCode(), reply.GetOutcome().GetDetail())
	}
	return nil
}

func registryByName(a *Aggregate, airport, name string) *pb.SessionRegistry {
	for _, entity := range a.EntitiesByKind(pb.EntityKind_SESSION_REGISTRY) {
		entry := entity.GetValue().GetSessionRegistry()
		if entry.Airport == airport && entry.Name == name && entry.State != pb.SessionRegistry_DELETING && entry.State != pb.SessionRegistry_DELETED {
			return entry
		}
	}
	return nil
}

// ActiveSessions is derived from the retained global entities on every read.
func (r SessionRegistry) ActiveSessions(ctx context.Context) ([]*pb.SessionRegistry, error) {
	entities, err := r.registryEntities(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]*pb.SessionRegistry, 0)
	for _, entity := range entities {
		entry := entity.GetValue().GetSessionRegistry()
		if entry.State == pb.SessionRegistry_ACTIVE {
			result = append(result, proto.Clone(entry).(*pb.SessionRegistry))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Id < result[j].Id })
	return result, nil
}

func validateSessionName(airport, name string) (string, string, error) {
	airport = strings.ToUpper(strings.TrimSpace(airport))
	name = strings.TrimSpace(name)
	if _, err := Subject(&pb.AggregateRef{Target: &pb.AggregateRef_Airport{Airport: &pb.AirportRef{Icao: airport}}}); err != nil || name == "" {
		return "", "", fmt.Errorf("invalid airport or session name")
	}
	return airport, name, nil
}

// GetOrCreateSession waits for the durable session seed and active registry
// marker before returning. A retry may use a new transport ID: the retained
// initializing record supplies the original workflow ID.
func (r SessionRegistry) GetOrCreateSession(ctx context.Context, airport, name string) (*pb.Session, error) {
	airport, name, err := validateSessionName(airport, name)
	if err != nil {
		return nil, err
	}
	for ctx.Err() == nil {
		global, err := r.read(ctx, globalRef())
		if err != nil {
			return nil, err
		}
		entry := registryByName(global, airport, name)
		if entry == nil {
			id := uuid.NewString()
			request := lifecycleRequest(globalRef(), id, &pb.SystemCommand_CreateSession{CreateSession: &pb.CreateSession{Airport: airport, Name: name, WorkflowId: id}})
			if err := r.execute(ctx, request); err != nil {
				return nil, err
			}
			continue
		}
		if entry.State == pb.SessionRegistry_INITIALIZING {
			if err := r.completeCreate(ctx, entry); err != nil {
				return nil, err
			}
			continue
		}
		session, err := r.read(ctx, sessionRef(entry.Id))
		if err != nil {
			return nil, err
		}
		seed := session.Entities[strconv.Itoa(int(entry.Id))]
		if seed != nil && seed.GetValue().GetSession().GetTombstoned() {
			if err := r.FinalizeDeletion(ctx, entry.Id); err != nil {
				return nil, err
			}
			continue
		}
		if seed == nil || seed.GetValue().GetSession() == nil {
			return nil, fmt.Errorf("active session %d has no live seed", entry.Id)
		}
		return proto.Clone(seed.GetValue().GetSession()).(*pb.Session), nil
	}
	return nil, ctx.Err()
}

func (r SessionRegistry) completeCreate(ctx context.Context, entry *pb.SessionRegistry) error {
	ref := sessionRef(entry.Id)
	session, err := r.read(ctx, ref)
	if err != nil {
		return err
	}
	if seed := session.Entities[strconv.Itoa(int(entry.Id))]; seed != nil && seed.GetValue().GetSession().GetTombstoned() {
		return r.FinalizeDeletion(ctx, entry.Id)
	}
	if err := r.execute(ctx, lifecycleRequest(ref, lifecycleID(entry.WorkflowId, "seed"), &pb.SystemCommand_CreateSession{CreateSession: &pb.CreateSession{Id: entry.Id, Airport: entry.Airport, Name: entry.Name, WorkflowId: entry.WorkflowId}})); err != nil {
		return err
	}
	return r.execute(ctx, lifecycleRequest(globalRef(), lifecycleID(entry.WorkflowId, "active"), &pb.SystemCommand_CreateSession{CreateSession: &pb.CreateSession{Id: entry.Id, Airport: entry.Airport, Name: entry.Name, WorkflowId: entry.WorkflowId}}))
}

// TombstoneSession records a terminal session marker. Task 18 decides when
// its five-minute healthy-service condition permits calling this method.
func (r SessionRegistry) TombstoneSession(ctx context.Context, id int32) error {
	global, err := r.read(ctx, globalRef())
	if err != nil {
		return err
	}
	entry := global.Entities[strconv.Itoa(int(id))]
	if entry == nil || entry.GetValue().GetSessionRegistry() == nil {
		return fmt.Errorf("session %d is absent from registry", id)
	}
	workflow := entry.GetValue().GetSessionRegistry().WorkflowId
	return r.execute(ctx, lifecycleRequest(sessionRef(id), lifecycleID(workflow, "tombstone"), &pb.SystemCommand_DeleteSession{DeleteSession: &pb.DeleteSession{Id: id, WorkflowId: workflow}}))
}

// FinalizeDeletion only releases the name after reading a durable session
// tombstone. Deleting and deleted are separate retained global commits.
func (r SessionRegistry) FinalizeDeletion(ctx context.Context, id int32) error {
	if id < 1 {
		return fmt.Errorf("invalid session ID")
	}
	session, err := r.read(ctx, sessionRef(id))
	if err != nil {
		return err
	}
	seed := session.Entities[strconv.Itoa(int(id))]
	if seed == nil || !seed.GetValue().GetSession().GetTombstoned() {
		return fmt.Errorf("session %d has no durable tombstone", id)
	}
	global, err := r.read(ctx, globalRef())
	if err != nil {
		return err
	}
	entity := global.Entities[strconv.Itoa(int(id))]
	if entity == nil || entity.GetValue().GetSessionRegistry() == nil {
		return fmt.Errorf("session %d is absent from registry", id)
	}
	entry := entity.GetValue().GetSessionRegistry()
	if entry.State == pb.SessionRegistry_DELETED {
		return nil
	}
	if entry.State != pb.SessionRegistry_DELETING {
		if err := r.execute(ctx, lifecycleRequest(globalRef(), lifecycleID(entry.WorkflowId, "deleting"), &pb.SystemCommand_DeleteSession{DeleteSession: &pb.DeleteSession{Id: id, WorkflowId: entry.WorkflowId}})); err != nil {
			return err
		}
	}
	return r.execute(ctx, lifecycleRequest(globalRef(), lifecycleID(entry.WorkflowId, "deleted"), &pb.SystemCommand_DeleteSession{DeleteSession: &pb.DeleteSession{Id: id, WorkflowId: entry.WorkflowId}}))
}

// Recover resumes incomplete durable steps after a process or owner restart.
func (r SessionRegistry) Recover(ctx context.Context) error {
	entities, err := r.registryEntities(ctx)
	if err != nil {
		return err
	}
	for _, entity := range entities {
		entry := entity.GetValue().GetSessionRegistry()
		switch entry.State {
		case pb.SessionRegistry_INITIALIZING:
			if err := r.completeCreate(ctx, entry); err != nil {
				return err
			}
		case pb.SessionRegistry_ACTIVE, pb.SessionRegistry_DELETING:
			seed, err := r.persistedSeed(ctx, entry.Id)
			if err != nil {
				return err
			}
			if seed != nil && seed.GetValue().GetSession().GetTombstoned() {
				if err := r.FinalizeDeletion(ctx, entry.Id); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// SessionLifecyclePlanner checks the other aggregate before a cross-aggregate
// transition. The session seed and tombstone are monotonic, so a confirmed
// record remains valid while the global subject CAS is retried.
func SessionLifecyclePlanner(read func(context.Context, *pb.AggregateRef) (*Aggregate, error)) Planner {
	return func(ctx context.Context, request *pb.CommandRequest, state *Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
		if read == nil {
			return nil, pb.CommandReply_UNAVAILABLE, 0, fmt.Errorf("lifecycle reader is unavailable")
		}
		if request.GetAggregate().GetGlobal() != nil {
			id := int32(0)
			if c := request.GetSystem().GetCreateSession(); c != nil {
				id = c.Id
			}
			if d := request.GetSystem().GetDeleteSession(); d != nil {
				id = d.Id
			}
			if id > 0 {
				session, err := read(ctx, sessionRef(id))
				if err != nil {
					return nil, pb.CommandReply_UNAVAILABLE, 0, err
				}
				seed := session.Entities[strconv.Itoa(int(id))]
				value := (*pb.Session)(nil)
				if seed != nil {
					value = seed.GetValue().GetSession()
				}
				if value == nil {
					return nil, pb.CommandReply_UNAVAILABLE, 0, fmt.Errorf("session prerequisite has not been replayed")
				}
				if c := request.GetSystem().GetCreateSession(); c != nil && (value.Tombstoned || value.Airport != c.Airport || value.Name != c.Name) {
					return nil, pb.CommandReply_INVALID_ARGUMENT, 0, fmt.Errorf("session seed is tombstoned or has a different identity")
				}
				if request.GetSystem().GetDeleteSession() != nil && !value.Tombstoned {
					return nil, pb.CommandReply_UNAVAILABLE, 0, fmt.Errorf("session tombstone has not been replayed")
				}
			}
		}
		return planSessionLifecycle(ctx, request, state)
	}
}

// planSessionLifecycle runs only in the candidate NATS writer. The global
// subject CAS serializes allocation; the retained registry ID is its high-water mark.
func planSessionLifecycle(ctx context.Context, request *pb.CommandRequest, state *Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
	ref := request.GetAggregate()
	create := request.GetSystem().GetCreateSession()
	remove := request.GetSystem().GetDeleteSession()
	if create == nil && remove == nil {
		if update := request.GetSystem().GetUpdateEntity(); update != nil {
			kind, _ := recordKind(update.Value)
			if kind == pb.EntityKind_SESSION_REGISTRY || kind == pb.EntityKind_SESSION {
				return nil, pb.CommandReply_INVALID_ARGUMENT, 0, fmt.Errorf("session lifecycle entity requires a lifecycle command")
			}
		}
		if deletion := request.GetSystem().GetRemoveEntity(); deletion != nil && (deletion.Kind == pb.EntityKind_SESSION_REGISTRY || deletion.Kind == pb.EntityKind_SESSION) {
			return nil, pb.CommandReply_INVALID_ARGUMENT, 0, fmt.Errorf("session lifecycle entity cannot be removed")
		}
		return PlanStrip(ctx, request, state)
	}
	if request.GetActor().GetKind() != pb.Actor_SYSTEM || request.ExpectedEntityRevision != nil {
		return nil, pb.CommandReply_INVALID_ARGUMENT, 0, fmt.Errorf("invalid lifecycle command")
	}
	if create != nil {
		airport, name, err := validateSessionName(create.Airport, create.Name)
		if err != nil || airport != create.Airport || name != create.Name || !canonicalUUID(create.WorkflowId) {
			return nil, pb.CommandReply_INVALID_ARGUMENT, 0, fmt.Errorf("invalid session identity or workflow")
		}
		if ref.GetGlobal() != nil {
			if create.Id == 0 {
				if registryByName(state, airport, name) != nil {
					return &pb.DomainChange{}, pb.CommandReply_COMMITTED, 0, nil
				}
				maxID := int32(0)
				for _, e := range state.EntitiesByKind(pb.EntityKind_SESSION_REGISTRY) {
					if id := e.GetValue().GetSessionRegistry().Id; id > maxID {
						maxID = id
					}
				}
				if maxID == math.MaxInt32 {
					return nil, pb.CommandReply_INVALID_ARGUMENT, 0, fmt.Errorf("session ID space exhausted")
				}
				id := maxID + 1
				entry := &pb.SessionRegistry{Id: id, Airport: airport, Name: name, State: pb.SessionRegistry_INITIALIZING, WorkflowId: create.WorkflowId}
				return lifecycleChange(nil, entry, "initializing", pb.WorkflowRecord_PENDING), pb.CommandReply_COMMITTED, 0, nil
			}
			old := state.Entities[strconv.Itoa(int(create.Id))]
			if old == nil || !matchingRegistry(old.GetValue().GetSessionRegistry(), create) {
				return nil, pb.CommandReply_NOT_FOUND, 0, fmt.Errorf("initializing registry entry missing")
			}
			entry := old.GetValue().GetSessionRegistry()
			if entry.State == pb.SessionRegistry_ACTIVE {
				return &pb.DomainChange{}, pb.CommandReply_COMMITTED, 0, nil
			}
			if entry.State != pb.SessionRegistry_INITIALIZING {
				return nil, pb.CommandReply_INVALID_ARGUMENT, 0, fmt.Errorf("registry entry cannot be activated")
			}
			copy := proto.Clone(entry).(*pb.SessionRegistry)
			copy.State = pb.SessionRegistry_ACTIVE
			return lifecycleChange(old, copy, "active", pb.WorkflowRecord_COMPLETED), pb.CommandReply_COMMITTED, 0, nil
		}
		if ref.GetSession().GetId() != create.Id || create.Id < 1 {
			return nil, pb.CommandReply_INVALID_ARGUMENT, 0, fmt.Errorf("session aggregate mismatch")
		}
		old := state.Entities[strconv.Itoa(int(create.Id))]
		if old != nil {
			s := old.GetValue().GetSession()
			if s == nil || s.Airport != airport || s.Name != name || s.Tombstoned {
				return nil, pb.CommandReply_INVALID_ARGUMENT, 0, fmt.Errorf("session identity is immutable")
			}
			return &pb.DomainChange{}, pb.CommandReply_COMMITTED, 0, nil
		}
		s := &pb.Session{Id: create.Id, Airport: airport, Name: name, NextStripId: 1, NextCoordinationId: 1, NextTacticalId: 1, NextMessageId: 1}
		change := &pb.EntityChange{Key: strconv.Itoa(int(s.Id)), Revision: 1, Operation: &pb.EntityChange_Upsert{Upsert: &pb.EntityRecord{Value: &pb.EntityRecord_Session{Session: s}}}}
		return &pb.DomainChange{Changes: []*pb.EntityChange{change}, Workflows: []*pb.WorkflowRecord{lifecycleWorkflow(create.WorkflowId, s.Id, "seeded", pb.WorkflowRecord_COMPLETED)}}, pb.CommandReply_COMMITTED, 0, nil
	}
	if remove.Id < 1 || !canonicalUUID(remove.WorkflowId) {
		return nil, pb.CommandReply_INVALID_ARGUMENT, 0, fmt.Errorf("invalid deletion identity")
	}
	old := state.Entities[strconv.Itoa(int(remove.Id))]
	if old == nil {
		return nil, pb.CommandReply_NOT_FOUND, 0, fmt.Errorf("session entry missing")
	}
	if ref.GetSession() != nil {
		if ref.GetSession().GetId() != remove.Id || old.GetValue().GetSession() == nil {
			return nil, pb.CommandReply_INVALID_ARGUMENT, 0, fmt.Errorf("session aggregate mismatch")
		}
		s := old.GetValue().GetSession()
		if s.Tombstoned {
			return &pb.DomainChange{}, pb.CommandReply_COMMITTED, 0, nil
		}
		for _, effect := range state.Effects {
			if effect.Status == pb.EffectRecord_WAITING || effect.Status == pb.EffectRecord_DISPATCH_CLAIMED {
				// This is a temporary admission barrier. Retaining a failed outcome
				// would poison the deterministic tombstone command after effects finish.
				return nil, pb.CommandReply_UNAVAILABLE, 0, fmt.Errorf("session has nonterminal effects")
			}
		}
		copy := proto.Clone(s).(*pb.Session)
		copy.Tombstoned = true
		change := &pb.EntityChange{Key: strconv.Itoa(int(remove.Id)), Revision: old.Revision + 1, Operation: &pb.EntityChange_Upsert{Upsert: &pb.EntityRecord{Value: &pb.EntityRecord_Session{Session: copy}}}}
		return &pb.DomainChange{Changes: []*pb.EntityChange{change}, Workflows: []*pb.WorkflowRecord{lifecycleWorkflow(remove.WorkflowId, remove.Id, "tombstoned", pb.WorkflowRecord_COMPLETED)}}, pb.CommandReply_COMMITTED, 0, nil
	}
	if ref.GetGlobal() == nil || old.GetValue().GetSessionRegistry() == nil || old.GetValue().GetSessionRegistry().WorkflowId != remove.WorkflowId {
		return nil, pb.CommandReply_INVALID_ARGUMENT, 0, fmt.Errorf("registry workflow mismatch")
	}
	entry := old.GetValue().GetSessionRegistry()
	if entry.State == pb.SessionRegistry_DELETED {
		return &pb.DomainChange{}, pb.CommandReply_COMMITTED, 0, nil
	}
	copy := proto.Clone(entry).(*pb.SessionRegistry)
	step, status := "deleting", pb.WorkflowRecord_PENDING
	if entry.State == pb.SessionRegistry_DELETING {
		copy.State, step, status = pb.SessionRegistry_DELETED, "deleted", pb.WorkflowRecord_COMPLETED
	} else {
		copy.State = pb.SessionRegistry_DELETING
	}
	return lifecycleChange(old, copy, step, status), pb.CommandReply_COMMITTED, 0, nil
}

func matchingRegistry(entry *pb.SessionRegistry, create *pb.CreateSession) bool {
	return entry != nil && entry.Id == create.Id && entry.Airport == create.Airport && entry.Name == create.Name && entry.WorkflowId == create.WorkflowId
}
func lifecycleWorkflow(workflow string, id int32, step string, status pb.WorkflowRecord_Status) *pb.WorkflowRecord {
	commandID := lifecycleID(workflow, step)
	if step == "initializing" {
		commandID = workflow
	}
	if step == "seeded" {
		commandID = lifecycleID(workflow, "seed")
	}
	return &pb.WorkflowRecord{WorkflowId: workflow, Source: globalRef(), Destination: sessionRef(id), Step: step, DerivedCommandId: commandID, Status: status}
}
func lifecycleChange(old *pb.EntitySnapshot, entry *pb.SessionRegistry, step string, status pb.WorkflowRecord_Status) *pb.DomainChange {
	revision := uint64(1)
	if old != nil {
		revision = old.Revision + 1
	}
	change := &pb.EntityChange{Key: strconv.Itoa(int(entry.Id)), Revision: revision, Operation: &pb.EntityChange_Upsert{Upsert: &pb.EntityRecord{Value: &pb.EntityRecord_SessionRegistry{SessionRegistry: entry}}}}
	return &pb.DomainChange{Changes: []*pb.EntityChange{change}, Workflows: []*pb.WorkflowRecord{lifecycleWorkflow(entry.WorkflowId, entry.Id, step, status)}}
}

// WaitActive is useful at connection setup when a different owner is advancing
// initialization. It never exposes an initializing or tombstoned session.
func (r SessionRegistry) WaitActive(ctx context.Context, airport, name string) (*pb.Session, error) {
	airport, name, err := validateSessionName(airport, name)
	if err != nil {
		return nil, err
	}
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		global, err := r.read(ctx, globalRef())
		if err != nil {
			return nil, err
		}
		if entry := registryByName(global, airport, name); entry != nil && entry.State == pb.SessionRegistry_ACTIVE {
			session, err := r.read(ctx, sessionRef(entry.Id))
			if err != nil {
				return nil, err
			}
			if seed := session.Entities[strconv.Itoa(int(entry.Id))]; seed != nil && seed.GetValue().GetSession() != nil && !seed.GetValue().GetSession().Tombstoned {
				return proto.Clone(seed.GetValue().GetSession()).(*pb.Session), nil
			}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}
