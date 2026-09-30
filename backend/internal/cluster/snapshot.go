package cluster

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"

	pb "FlightStrips/pkg/events/cluster"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
)

const MaxObjectBytes = 32 << 20

var ErrImmutableSnapshotCollision = errors.New("immutable snapshot name already contains different data")

// SnapshotStore publishes an index only after reading back and validating the
// immutable typed object. KV history retains the previous verified pointer.
type SnapshotStore struct {
	Index   nats.KeyValue
	Objects nats.ObjectStore
}

func snapshotKey(ref *pb.AggregateRef) (string, error) {
	subject, err := Subject(ref)
	if err != nil {
		return "", err
	}
	return strings.TrimPrefix(subject, "fs.v1.state."), nil
}

func (s SnapshotStore) Save(a *Aggregate) error {
	if s.Index == nil || s.Objects == nil {
		return fmt.Errorf("snapshot resources unavailable")
	}
	key, err := snapshotKey(a.Ref)
	if err != nil {
		return err
	}
	snapshot, err := a.Snapshot()
	if err != nil {
		return err
	}
	object := &pb.ObjectValue{SchemaVersion: 1, Content: &pb.ObjectValue_Snapshot{Snapshot: snapshot}}
	data, err := (proto.MarshalOptions{Deterministic: true}).Marshal(object)
	if err != nil {
		return err
	}
	if len(data) > MaxObjectBytes {
		return fmt.Errorf("oversized snapshot")
	}
	name := fmt.Sprintf("snapshot/%s/%d", strings.ReplaceAll(key, ".", "/"), a.StreamSequence)
	if previous, err := s.Objects.GetBytes(name); err == nil {
		if string(previous) != string(data) {
			return ErrImmutableSnapshotCollision
		}
	} else {
		if !errors.Is(err, nats.ErrObjectNotFound) {
			return err
		}
		if _, err := s.Objects.PutBytes(name, data); err != nil {
			return err
		}
	}
	index := &pb.SnapshotIndex{SchemaVersion: 1, Aggregate: proto.Clone(a.Ref).(*pb.AggregateRef), ObjectName: name, Sha256: digest(data), AggregateRevision: a.Revision, LastStreamSequence: a.StreamSequence, LastSubjectSequence: a.SubjectSequence}
	if _, err := verifySnapshotObject(s.Objects, index); err != nil {
		return err
	}
	encoded, err := (proto.MarshalOptions{Deterministic: true}).Marshal(index)
	if err != nil {
		return err
	}
	for attempts := 0; attempts < 5; attempts++ {
		previous, err := s.Index.Get(key)
		if errors.Is(err, nats.ErrKeyNotFound) {
			if _, err = s.Index.Create(key, encoded); err == nil {
				return nil
			}
		} else if err != nil {
			return err
		} else {
			prior := &pb.SnapshotIndex{}
			if pb.UnmarshalStrict(previous.Value(), prior) == nil && prior.LastStreamSequence >= index.LastStreamSequence {
				return nil
			}
			if _, err = s.Index.Update(key, encoded, previous.Revision()); err == nil {
				return nil
			}
		}
	}
	return fmt.Errorf("snapshot index changed during update")
}

// Load tries both retained index revisions. A corrupt newest object cannot
// poison the projection; an invalid older index simply falls back to the log.
func (s SnapshotStore) Load(ref *pb.AggregateRef) (*Aggregate, error) {
	if s.Index == nil || s.Objects == nil {
		return nil, fmt.Errorf("snapshot resources unavailable")
	}
	key, err := snapshotKey(ref)
	if err != nil {
		return nil, err
	}
	entries, err := s.Index.History(key)
	if err != nil {
		if err == nats.ErrKeyNotFound {
			return NewAggregate(ref), nil
		}
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Revision() > entries[j].Revision() })
	for _, entry := range entries {
		index := &pb.SnapshotIndex{}
		if pb.UnmarshalStrict(entry.Value(), index) != nil {
			continue
		}
		if got, e := snapshotKey(index.Aggregate); e != nil || got != key {
			continue
		}
		snapshot, e := verifySnapshotObject(s.Objects, index)
		if e != nil {
			continue
		}
		state, e := aggregateFromSnapshot(snapshot)
		if e == nil {
			return state, nil
		}
	}
	return NewAggregate(ref), nil
}

func verifySnapshotObject(objects nats.ObjectStore, index *pb.SnapshotIndex) (*pb.Snapshot, error) {
	if index.SchemaVersion != 1 || index.LastStreamSequence == 0 || index.LastSubjectSequence != index.LastStreamSequence {
		return nil, fmt.Errorf("invalid snapshot index")
	}
	key, err := snapshotKey(index.Aggregate)
	if err != nil {
		return nil, err
	}
	want := fmt.Sprintf("snapshot/%s/%d", strings.ReplaceAll(key, ".", "/"), index.LastStreamSequence)
	if index.ObjectName != want {
		return nil, fmt.Errorf("snapshot object identity mismatch")
	}
	data, err := objects.GetBytes(index.ObjectName)
	if err != nil {
		return nil, err
	}
	if len(data) > MaxObjectBytes || digest(data) != index.Sha256 {
		return nil, fmt.Errorf("snapshot object digest mismatch")
	}
	object := &pb.ObjectValue{}
	if err := pb.UnmarshalStrict(data, object); err != nil {
		return nil, err
	}
	snapshot := object.GetSnapshot()
	if object.SchemaVersion != 1 || snapshot == nil || snapshot.SchemaVersion != 1 || !proto.Equal(snapshot.Aggregate, index.Aggregate) || snapshot.AggregateRevision != index.AggregateRevision || snapshot.LastStreamSequence != index.LastStreamSequence || snapshot.LastSubjectSequence != index.LastSubjectSequence {
		return nil, fmt.Errorf("snapshot checkpoint mismatch")
	}
	copy := proto.Clone(snapshot).(*pb.Snapshot)
	copy.Sha256 = ""
	b, err := (proto.MarshalOptions{Deterministic: true}).Marshal(copy)
	if err != nil || digest(b) != snapshot.Sha256 {
		return nil, fmt.Errorf("snapshot internal digest mismatch")
	}
	if err := validateTyped(snapshot.ProtoReflect()); err != nil {
		return nil, err
	}
	return snapshot, nil
}

func aggregateFromSnapshot(snapshot *pb.Snapshot) (*Aggregate, error) {
	state := NewAggregate(snapshot.Aggregate)
	state.Revision, state.StreamSequence, state.SubjectSequence = snapshot.AggregateRevision, snapshot.LastStreamSequence, snapshot.LastSubjectSequence
	if snapshot.Owner != nil {
		state.Owner = proto.Clone(snapshot.Owner).(*pb.OwnerTerm)
	}
	if snapshot.Master != nil {
		state.Master = proto.Clone(snapshot.Master).(*pb.MasterTerm)
	}
	if snapshot.Sync != nil {
		state.Sync = proto.Clone(snapshot.Sync).(*pb.SessionSync)
	}
	last := ""
	lastKind := pb.EntityKind(0)
	for _, entity := range snapshot.Entities {
		if entity == nil || entity.Revision == 0 || entity.Value == nil {
			return nil, fmt.Errorf("invalid snapshot entity ordering")
		}
		kind, err := recordKind(entity.Value)
		if err != nil {
			return nil, err
		}
		if entity.Key < last || (entity.Key == last && kind <= lastKind) {
			return nil, fmt.Errorf("invalid snapshot entity ordering")
		}
		_, global := snapshot.Aggregate.GetTarget().(*pb.AggregateRef_Global)
		_, airport := snapshot.Aggregate.GetTarget().(*pb.AggregateRef_Airport)
		_, session := snapshot.Aggregate.GetTarget().(*pb.AggregateRef_Session)
		if (kind <= 3 && !global) || (kind == 4 && !airport) || (kind >= 5 && kind <= 18 && !session) || (kind >= 19 && kind <= 28 && kind != pb.EntityKind_PROVIDER_CHECKPOINT && !airport) || (kind == pb.EntityKind_PROVIDER_CHECKPOINT && !airport && !global) || ((kind == 29 || kind == 30) && !session) {
			return nil, fmt.Errorf("snapshot entity in wrong aggregate")
		}
		key, err := recordKey(kind, entity.Value)
		if err != nil || key != entity.Key {
			return nil, fmt.Errorf("snapshot entity key mismatch")
		}
		state.Entities[entitySlot(state.Entities, kind, entity.Key)] = proto.Clone(entity).(*pb.EntitySnapshot)
		lastKind = kind
		last = entity.Key
	}
	if err := validateStripState(snapshot.Aggregate, state.Entities); err != nil {
		return nil, err
	}
	last = ""
	for _, outcome := range snapshot.Outcomes {
		if outcome == nil || !canonicalUUID(outcome.CommandId) || outcome.CommandId <= last || len(outcome.RequestSha256) != 64 || outcome.Actor == nil || outcome.Status == pb.CommandOutcome_STATUS_UNSPECIFIED || outcome.CommittedStreamSequence == 0 || outcome.CommittedStreamSequence > state.StreamSequence || outcome.AggregateRevision > state.Revision || state.Ledger[outcome.CommandId] != nil {
			return nil, fmt.Errorf("invalid snapshot outcome")
		}
		if _, err := hex.DecodeString(outcome.RequestSha256); err != nil {
			return nil, fmt.Errorf("invalid snapshot command digest")
		}
		state.Ledger[outcome.CommandId] = proto.Clone(outcome).(*pb.CommandOutcome)
		last = outcome.CommandId
	}
	last = ""
	for _, workflow := range snapshot.Workflows {
		if workflow == nil || workflow.WorkflowId == "" || workflow.WorkflowId <= last || state.Workflows[workflow.WorkflowId] != nil {
			return nil, fmt.Errorf("invalid snapshot workflow")
		}
		state.Workflows[workflow.WorkflowId] = proto.Clone(workflow).(*pb.WorkflowRecord)
		last = workflow.WorkflowId
	}
	last = ""
	for _, effect := range snapshot.Effects {
		if effect == nil || !canonicalUUID(effect.CommandId) || effect.CommandId <= last || state.Effects[effect.CommandId] != nil {
			return nil, fmt.Errorf("invalid snapshot effect")
		}
		state.Effects[effect.CommandId] = proto.Clone(effect).(*pb.EffectRecord)
		last = effect.CommandId
	}
	state.rebuildIndexes()
	return state, nil
}

func digest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
