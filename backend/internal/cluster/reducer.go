package cluster

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	pb "FlightStrips/pkg/events/cluster"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type AppliedEvent struct {
	Subject                         string
	StreamSequence, SubjectSequence uint64
	ServerTime                      time.Time
	Data                            []byte
}
type Aggregate struct {
	Ref                                       *pb.AggregateRef
	Revision, StreamSequence, SubjectSequence uint64
	Owner                                     *pb.OwnerTerm
	Master                                    *pb.MasterTerm
	Entities                                  map[string]*pb.EntitySnapshot
	Indexes                                   map[pb.EntityKind]map[string]*pb.EntitySnapshot
	StripIDs                                  map[uint64]*pb.EntitySnapshot
	StandAssignmentsByStand                   map[string][]*pb.EntitySnapshot
	Ledger                                    map[string]*pb.CommandOutcome
	Workflows                                 map[string]*pb.WorkflowRecord
	Effects                                   map[string]*pb.EffectRecord
	Sync                                      *pb.SessionSync
}

func NewAggregate(ref *pb.AggregateRef) *Aggregate {
	return &Aggregate{Ref: proto.Clone(ref).(*pb.AggregateRef), Entities: map[string]*pb.EntitySnapshot{}, Indexes: map[pb.EntityKind]map[string]*pb.EntitySnapshot{}, StripIDs: map[uint64]*pb.EntitySnapshot{}, StandAssignmentsByStand: map[string][]*pb.EntitySnapshot{}, Ledger: map[string]*pb.CommandOutcome{}, Workflows: map[string]*pb.WorkflowRecord{}, Effects: map[string]*pb.EffectRecord{}}
}

// Apply validates the entire event before changing a projection. A stale owner
// event still advances the log checkpoint, but has no domain effect.
func (a *Aggregate) Apply(entry AppliedEvent) (bool, error) {
	want, err := Subject(a.Ref)
	if err != nil {
		return false, err
	}
	if entry.Subject != want || entry.StreamSequence <= a.StreamSequence || entry.SubjectSequence <= a.SubjectSequence {
		return false, fmt.Errorf("invalid event checkpoint or subject")
	}
	e := &pb.StateEvent{}
	if len(entry.Data) > MaxStateBytes {
		return false, fmt.Errorf("oversized state event")
	}
	if err := pb.UnmarshalStrict(entry.Data, e); err != nil {
		return false, err
	}
	if err := validateTyped(e.ProtoReflect()); err != nil {
		return false, err
	}
	got, err := Subject(e.GetAggregate())
	if err != nil || got != want || e.GetSchemaVersion() != 1 || !canonicalUUID(e.GetEventId()) || e.GetFact() == nil {
		return false, fmt.Errorf("invalid state event envelope")
	}
	if e.GetOwnerClaimed() != nil || e.GetOwnerRenewed() != nil {
		if e.CommandId != nil || e.GetAggregateRevision() != a.Revision || e.GetActor().GetKind() != pb.Actor_SYSTEM {
			return false, fmt.Errorf("invalid owner control event")
		}
		if e.GetOwnerClaimed().GetLeaseUntil() != nil || e.GetOwnerRenewed().GetLeaseUntil() != nil {
			return false, fmt.Errorf("owner lease is derived from server time")
		}
		accepted := false
		if term := e.GetOwnerClaimed(); term != nil {
			accepted = term.GetNodeId() != "" && term.GetEpoch() == a.ownerEpoch()+1 && (a.Owner == nil || !entry.ServerTime.Before(a.Owner.GetLeaseUntil().AsTime()))
			if accepted {
				a.Owner = &pb.OwnerTerm{NodeId: term.NodeId, Epoch: term.Epoch, LeaseUntil: timestamppb.New(entry.ServerTime.Add(ownerLease))}
			}
		} else if term := e.GetOwnerRenewed(); term != nil {
			accepted = a.Owner != nil && term.GetNodeId() == a.Owner.NodeId && term.GetEpoch() == a.Owner.Epoch && entry.ServerTime.Before(a.Owner.GetLeaseUntil().AsTime())
			if accepted {
				a.Owner.LeaseUntil = timestamppb.New(entry.ServerTime.Add(ownerLease))
			}
		}
		a.checkpoint(entry)
		return accepted, nil
	}
	if e.CommandId != nil && !canonicalUUID(e.GetCommandId()) {
		return false, fmt.Errorf("invalid command ID")
	}
	if a.Owner == nil || e.GetOwnerEpoch() != a.Owner.Epoch || entry.ServerTime.After(a.Owner.GetLeaseUntil().AsTime()) {
		a.checkpoint(entry)
		return false, nil
	}
	if e.GetAggregateRevision() != a.Revision+1 {
		return false, fmt.Errorf("aggregate revision gap")
	}
	if e.GetActor() == nil || e.GetActor().GetKind() == pb.Actor_KIND_UNSPECIFIED {
		return false, fmt.Errorf("missing actor")
	}
	if e.CommandId == nil {
		return false, fmt.Errorf("domain event has no command ID")
	}
	// Effect updates belong to the original command. They advance its existing
	// outcome instead of allocating another ledger entry under a fresh ID.
	if effect := e.GetEffectChanged(); effect != nil {
		if e.GetActor().GetKind() != pb.Actor_SYSTEM || e.GetActor().GetId() != a.Owner.GetNodeId() || e.GetCommandId() != effect.CommandId ||
			effect.Status == pb.EffectRecord_DISPATCH_CLAIMED && effect.OwnerEpoch != a.Owner.Epoch {
			return false, fmt.Errorf("invalid effect actor or command ID")
		}
		old, outcome := a.Effects[effect.CommandId], a.Ledger[effect.CommandId]
		if err := validateEffectTransition(old, effect, outcome, entry.ServerTime); err != nil {
			if errors.Is(err, errEffectDeadlineRace) {
				a.checkpoint(entry)
				return false, nil
			}
			return false, err
		}
		updated := proto.Clone(effect).(*pb.EffectRecord)
		if updated.Status == pb.EffectRecord_DISPATCH_CLAIMED {
			updated.ResultDeadline = timestamppb.New(entry.ServerTime.Add(effectResultWindow))
		}
		a.Effects[effect.CommandId] = updated
		a.Revision = e.AggregateRevision
		copy := proto.Clone(outcome).(*pb.CommandOutcome)
		copy.AggregateRevision, copy.CommittedStreamSequence = a.Revision, entry.StreamSequence
		switch updated.Status {
		case pb.EffectRecord_EXECUTED:
			copy.Status = pb.CommandOutcome_SUCCEEDED
			if updated.GetPrivateMessage() != nil {
				copy.Detail = "Accepted by local EuroScope send; pilot receipt is not confirmed"
			}
		case pb.EffectRecord_FAILED:
			copy.Status, copy.ReasonCode = pb.CommandOutcome_FAILED, updated.ReasonCode
		case pb.EffectRecord_EXPIRED:
			copy.Status, copy.ReasonCode = pb.CommandOutcome_EXPIRED, "DISPATCH_DEADLINE"
		case pb.EffectRecord_UNKNOWN:
			copy.Status, copy.ReasonCode = pb.CommandOutcome_UNKNOWN, "RESULT_DEADLINE"
		}
		a.Ledger[effect.CommandId] = copy
		a.checkpoint(entry)
		return true, nil
	}
	changes := e.GetDomainChanged().GetChanges()
	previousKind, previousKey := int32(0), ""
	staged := make(map[string]*pb.EntitySnapshot, len(a.Entities)+len(changes))
	for k, v := range a.Entities {
		staged[k] = v
	}
	for _, change := range changes {
		kind, err := changeKind(change)
		if err != nil {
			return false, err
		}
		if int32(kind) < previousKind || (int32(kind) == previousKind && change.GetKey() <= previousKey) {
			return false, fmt.Errorf("unsorted or duplicate entity change")
		}
		previousKind, previousKey = int32(kind), change.GetKey()
		slot := entitySlot(staged, kind, change.GetKey())
		if err := validateChange(a.Ref, change, staged[slot]); err != nil {
			return false, err
		}
		if change.GetUpsert() != nil {
			staged[slot] = &pb.EntitySnapshot{Key: change.Key, Revision: change.Revision, Value: proto.Clone(change.GetUpsert()).(*pb.EntityRecord)}
		} else {
			delete(staged, slot)
		}
	}
	if err := validateControllerSectorState(a.Ref, staged); err != nil {
		return false, err
	}
	if err := validateStripState(a.Ref, staged); err != nil {
		return false, err
	}
	if err := validateStripTransition(a, changes, staged); err != nil {
		return false, err
	}
	if err := validateCoordinationTransition(a, changes, staged); err != nil {
		return false, err
	}
	if err := validatePdcTacticalState(a, e.GetDomainChanged(), staged); err != nil {
		return false, err
	}
	if err := validateStandState(a.Ref, staged); err != nil {
		return false, err
	}
	var outcome *pb.CommandOutcome
	if d := e.GetDomainChanged(); d != nil {
		outcome = d.GetOutcome()
	}
	if x := e.GetOutcomeRecorded(); x != nil {
		outcome = x
	}
	if outcome == nil || outcome.GetCommandId() != e.GetCommandId() || len(outcome.GetRequestSha256()) != 64 || outcome.GetActor() == nil || outcome.GetStatus() == pb.CommandOutcome_STATUS_UNSPECIFIED {
		return false, fmt.Errorf("missing command outcome")
	}
	if !proto.Equal(outcome.Actor, e.Actor) || outcome.CommittedStreamSequence != 0 {
		return false, fmt.Errorf("invalid pre-publish outcome")
	}
	if _, err := hex.DecodeString(outcome.RequestSha256); err != nil {
		return false, fmt.Errorf("invalid command digest")
	}
	if _, exists := a.Ledger[e.GetCommandId()]; exists {
		return false, fmt.Errorf("duplicate command outcome")
	}
	if outcome.GetAggregateRevision() != 0 && outcome.GetAggregateRevision() != e.GetAggregateRevision() {
		return false, fmt.Errorf("outcome revision mismatch")
	}
	if d := e.GetDomainChanged(); d != nil {
		if len(d.Effects) > 1 || len(d.Effects) > 0 && outcome.Status != pb.CommandOutcome_ACCEPTED {
			return false, fmt.Errorf("invalid requested effect outcome")
		}
		for _, effect := range d.Effects {
			if err := validateEffectRequest(effect, e.GetCommandId(), a.Effects[effect.CommandId]); err != nil {
				return false, err
			}
		}
	}
	a.Entities = staged
	if session, ok := a.Ref.GetTarget().(*pb.AggregateRef_Session); ok {
		if entity := staged[strconv.FormatInt(int64(session.Session.Id), 10)]; entity != nil {
			if term := entity.GetValue().GetSession().GetMaster(); term != nil {
				a.Master = proto.Clone(term).(*pb.MasterTerm)
			} else {
				a.Master = nil
			}
			if sync := entity.GetValue().GetSession().GetSync(); sync != nil {
				a.Sync = proto.Clone(sync).(*pb.SessionSync)
			} else {
				a.Sync = nil
			}
		} else {
			a.Master = nil
			a.Sync = nil
		}
	}
	a.rebuildIndexes()
	a.Revision = e.AggregateRevision
	copy := proto.Clone(outcome).(*pb.CommandOutcome)
	copy.CommittedStreamSequence, copy.AggregateRevision = entry.StreamSequence, e.AggregateRevision
	a.Ledger[e.GetCommandId()] = copy
	if d := e.GetDomainChanged(); d != nil {
		for _, w := range d.Workflows {
			a.Workflows[w.WorkflowId] = proto.Clone(w).(*pb.WorkflowRecord)
		}
		for _, effect := range d.Effects {
			requested := proto.Clone(effect).(*pb.EffectRecord)
			requested.DispatchDeadline = timestamppb.New(entry.ServerTime.Add(effectDispatchWindow))
			a.Effects[effect.CommandId] = requested
		}
	}
	if w := e.GetWorkflowChanged(); w != nil {
		a.Workflows[w.WorkflowId] = proto.Clone(w).(*pb.WorkflowRecord)
	}
	if effect := e.GetEffectChanged(); effect != nil {
		a.Effects[effect.CommandId] = proto.Clone(effect).(*pb.EffectRecord)
	}
	if sync := e.GetSessionSynced(); sync != nil {
		a.Sync = proto.Clone(sync).(*pb.SessionSync)
	}
	a.checkpoint(entry)
	return true, nil
}

func (a *Aggregate) checkpoint(e AppliedEvent) {
	a.StreamSequence, a.SubjectSequence = e.StreamSequence, e.SubjectSequence
}
func (a *Aggregate) ownerEpoch() uint64 {
	if a.Owner == nil {
		return 0
	}
	return a.Owner.Epoch
}

func (a *Aggregate) rebuildIndexes() {
	indexes := make(map[pb.EntityKind]map[string]*pb.EntitySnapshot)
	stripIDs := make(map[uint64]*pb.EntitySnapshot)
	standAssignments := make(map[string][]*pb.EntitySnapshot)
	keys := make([]string, 0, len(a.Entities))
	for key := range a.Entities {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		entity := a.Entities[key]
		kind, _ := recordKind(entity.Value)
		if indexes[kind] == nil {
			indexes[kind] = make(map[string]*pb.EntitySnapshot)
		}
		indexes[kind][entity.Key] = entity
		if strip := entity.GetValue().GetStrip(); strip != nil {
			stripIDs[strip.Id] = entity
		}
		if assignment := entity.GetValue().GetStandAssignment(); assignment != nil {
			standAssignments[assignment.Stand] = append(standAssignments[assignment.Stand], entity)
		}
	}
	a.Indexes = indexes
	a.StripIDs = stripIDs
	a.StandAssignmentsByStand = standAssignments
}

func (a *Aggregate) EntitiesByKind(kind pb.EntityKind) []*pb.EntitySnapshot {
	keys := make([]string, 0, len(a.Indexes[kind]))
	for key := range a.Indexes[kind] {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]*pb.EntitySnapshot, 0, len(keys))
	for _, key := range keys {
		result = append(result, proto.Clone(a.Indexes[kind][key]).(*pb.EntitySnapshot))
	}
	return result
}

// Snapshot returns a deterministic typed view. Task 03 owns object persistence.
func (a *Aggregate) Snapshot() (*pb.Snapshot, error) {
	s := &pb.Snapshot{SchemaVersion: 1, Aggregate: proto.Clone(a.Ref).(*pb.AggregateRef), AggregateRevision: a.Revision, LastStreamSequence: a.StreamSequence, LastSubjectSequence: a.SubjectSequence}
	if a.Owner != nil {
		s.Owner = proto.Clone(a.Owner).(*pb.OwnerTerm)
	}
	if a.Master != nil {
		s.Master = proto.Clone(a.Master).(*pb.MasterTerm)
	}
	if a.Sync != nil {
		s.Sync = proto.Clone(a.Sync).(*pb.SessionSync)
	}
	for _, entity := range a.Entities {
		s.Entities = append(s.Entities, proto.Clone(entity).(*pb.EntitySnapshot))
	}
	sort.Slice(s.Entities, func(i, j int) bool {
		if s.Entities[i].Key != s.Entities[j].Key {
			return s.Entities[i].Key < s.Entities[j].Key
		}
		left, _ := recordKind(s.Entities[i].Value)
		right, _ := recordKind(s.Entities[j].Value)
		return left < right
	})
	keys := make([]string, 0, len(a.Ledger))
	keys = keys[:0]
	for k := range a.Ledger {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		s.Outcomes = append(s.Outcomes, proto.Clone(a.Ledger[k]).(*pb.CommandOutcome))
	}
	keys = keys[:0]
	for k := range a.Workflows {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		s.Workflows = append(s.Workflows, proto.Clone(a.Workflows[k]).(*pb.WorkflowRecord))
	}
	keys = keys[:0]
	for k := range a.Effects {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		s.Effects = append(s.Effects, proto.Clone(a.Effects[k]).(*pb.EffectRecord))
	}
	b, err := (proto.MarshalOptions{Deterministic: true}).Marshal(s)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(b)
	s.Sha256 = hex.EncodeToString(digest[:])
	return s, nil
}

// Legacy aggregates kept noncolliding records under their plain key. A second
// entity kind with the same canonical key receives a deterministic private
// slot; the wire key and kind remain unchanged. This permits AMAN, policy and
// navigation records for the same airport in one aggregate.
func entitySlot(entities map[string]*pb.EntitySnapshot, kind pb.EntityKind, key string) string {
	slot := fmt.Sprintf("\x00%d/%s", kind, key)
	if entities[slot] != nil {
		return slot
	}
	if old := entities[key]; old == nil {
		return key
	} else if got, _ := recordKind(old.Value); got == kind {
		return key
	}
	return slot
}

func changeKind(change *pb.EntityChange) (pb.EntityKind, error) {
	if change == nil || change.GetOperation() == nil {
		return 0, fmt.Errorf("missing entity operation")
	}
	if d := change.GetDelete(); d != nil {
		if d.Kind < 1 || pb.EntityKind_name[int32(d.Kind)] == "" {
			return 0, fmt.Errorf("invalid entity kind")
		}
		return d.Kind, nil
	}
	r := change.GetUpsert()
	if r == nil || r.GetValue() == nil {
		return 0, fmt.Errorf("missing typed entity")
	}
	oneof := r.ProtoReflect().Descriptor().Oneofs().ByName("value")
	f := r.ProtoReflect().WhichOneof(oneof)
	if f == nil || f.Number() < 1 || pb.EntityKind_name[int32(f.Number())] == "" {
		return 0, fmt.Errorf("invalid entity case")
	}
	return pb.EntityKind(f.Number()), nil
}

func validateChange(ref *pb.AggregateRef, c *pb.EntityChange, old *pb.EntitySnapshot) error {
	kind, err := changeKind(c)
	if err != nil {
		return err
	}
	if c.Key == "" || strings.TrimSpace(c.Key) != c.Key || c.Revision == 0 {
		return fmt.Errorf("invalid entity key/revision")
	}
	if old == nil && c.Revision != 1 {
		return fmt.Errorf("new entity revision must be one")
	}
	if old != nil {
		oldKind, err := recordKind(old.Value)
		if err != nil || oldKind != kind || c.Revision != old.Revision+1 {
			return fmt.Errorf("entity type/revision conflict")
		}
		if c.GetUpsert() != nil && proto.Equal(old.Value, c.GetUpsert()) {
			return fmt.Errorf("unchanged entity replacement")
		}
	}
	if c.GetDelete() != nil {
		if old == nil {
			return fmt.Errorf("delete of absent entity")
		}
		return nil
	}
	key, err := recordKey(kind, c.GetUpsert())
	if err != nil || key != c.Key {
		return fmt.Errorf("entity key mismatch: %s", c.Key)
	}
	if err := validateTyped(c.GetUpsert().ProtoReflect()); err != nil {
		return err
	}
	_, global := ref.GetTarget().(*pb.AggregateRef_Global)
	_, airport := ref.GetTarget().(*pb.AggregateRef_Airport)
	_, session := ref.GetTarget().(*pb.AggregateRef_Session)
	if (kind <= 3 && !global) || (kind == 4 && !airport) || (kind >= 5 && kind <= 18 && !session) || (kind >= 19 && kind <= 28 && kind != pb.EntityKind_PROVIDER_CHECKPOINT && !airport) || (kind == pb.EntityKind_PROVIDER_CHECKPOINT && !airport && !global && !(session && (c.GetUpsert().GetProviderCheckpoint().Provider == "viff" || c.GetUpsert().GetProviderCheckpoint().Provider == "hoppie"))) || ((kind == 29 || kind == 30 || kind == 31) && !session) {
		return fmt.Errorf("entity in wrong aggregate")
	}
	return nil
}

func recordKind(r *pb.EntityRecord) (pb.EntityKind, error) {
	if r == nil || r.GetValue() == nil {
		return 0, fmt.Errorf("missing typed entity")
	}
	f := r.ProtoReflect().WhichOneof(r.ProtoReflect().Descriptor().Oneofs().ByName("value"))
	if f == nil || f.Number() < 1 || pb.EntityKind_name[int32(f.Number())] == "" {
		return 0, fmt.Errorf("invalid entity case")
	}
	return pb.EntityKind(f.Number()), nil
}

func recordKey(kind pb.EntityKind, r *pb.EntityRecord) (string, error) {
	if got, err := recordKind(r); err != nil || got != kind {
		return "", fmt.Errorf("entity case mismatch")
	}
	m := r.ProtoReflect()
	f := m.WhichOneof(m.Descriptor().Oneofs().ByName("value"))
	v := m.Get(f).Message()
	field := func(name string) string {
		d := v.Descriptor().Fields().ByName(protoreflect.Name(name))
		if d == nil {
			return ""
		}
		x := v.Get(d)
		if d.Kind() == protoreflect.StringKind {
			return x.String()
		}
		switch d.Kind() {
		case protoreflect.Int32Kind, protoreflect.Int64Kind, protoreflect.Sint32Kind, protoreflect.Sint64Kind:
			return strconv.FormatInt(x.Int(), 10)
		default:
			return strconv.FormatUint(x.Uint(), 10)
		}
	}
	switch kind {
	case 1, 4, 19, 25:
		return field("icao") + field("airport"), nil
	case 2, 5:
		return field("id"), nil
	case 3:
		start := v.Descriptor().Fields().ByName("window_start")
		if !v.Has(start) {
			return "", fmt.Errorf("missing quota window")
		}
		return field("provider") + "." + strconv.FormatInt(v.Get(start).Message().Interface().(*timestamppb.Timestamp).AsTime().Unix(), 10), nil
	case 6:
		return field("cid"), nil
	case 7:
		return field("sector"), nil
	case 8, 13, 14, 15, 17, 20, 24:
		if kind == 24 {
			return field("provider_id"), nil
		}
		if kind == 17 {
			return field("callsign") + "." + field("key"), nil
		}
		return field("callsign"), nil
	case 9, 10, 18, 21, 22, 23, 29:
		return field("id"), nil
	case 11:
		return field("callsign"), nil
	case 12:
		return field("stand"), nil
	case 16:
		return field("airport"), nil
	case 26:
		return field("route_key"), nil
	case 27:
		return field("provider") + "." + field("resource"), nil
	case 28:
		return field("airport") + "." + field("provider"), nil
	case 30:
		return field("provider"), nil
	case 31:
		return field("message_id"), ValidatePdcProviderMessage(r.GetPdcProviderMessage())
	}
	return "", fmt.Errorf("unknown entity kind")
}
