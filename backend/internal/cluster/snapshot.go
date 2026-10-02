package cluster

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
)

const MaxObjectBytes = 32 << 20

var errInvalidSnapshotObject = errors.New("invalid snapshot object")

var ErrImmutableSnapshotCollision = errors.New("immutable snapshot name already contains different data")
var ErrSnapshotIndexContended = errors.New("snapshot index update contended; retained log remains authoritative")

var ErrSnapshotTooLarge = errors.New("history exceeds bounded snapshot size; retained log remains authoritative")

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
		return fmt.Errorf("snapshot materialization: %w", err)
	}
	object := &pb.ObjectValue{SchemaVersion: 1, Content: &pb.ObjectValue_Snapshot{Snapshot: snapshot}}
	data, err := (proto.MarshalOptions{Deterministic: true}).Marshal(object)
	if err != nil {
		return fmt.Errorf("snapshot encoding: %w", err)
	}
	if len(data) > MaxObjectBytes {
		return ErrSnapshotTooLarge
	}
	// Each publication owns a fresh object namespace. ObjectStore.Put cannot
	// conditionally create and may purge old chunks on overwrite; neither replica
	// can overwrite another replica's object, even when checkpoints are identical.
	name := fmt.Sprintf("snapshot/%s/%d/%s", strings.ReplaceAll(key, ".", "/"), a.StreamSequence, uuid.NewString())
	index := &pb.SnapshotIndex{SchemaVersion: 1, Aggregate: proto.Clone(a.Ref).(*pb.AggregateRef), ObjectName: name, Sha256: digest(data), AggregateRevision: a.Revision, LastStreamSequence: a.StreamSequence, LastSubjectSequence: a.SubjectSequence}
	verifyPrior := func(prior *pb.SnapshotIndex) error { _, err := verifySnapshotObject(s.Objects, prior); return err }
	if previous, readErr := s.Index.Get(key); readErr == nil {
		prior := &pb.SnapshotIndex{}
		reusable := false
		var priorErr error
		if pb.UnmarshalStrict(previous.Value(), prior) == nil {
			reusable, priorErr = snapshotIndexSupersedes(prior, index, verifyPrior)
		}
		err = priorErr
		if err != nil {
			return err
		}
		if reusable {
			return nil
		}
	} else if !errors.Is(readErr, nats.ErrKeyNotFound) {
		return fmt.Errorf("snapshot index lookup: %w", readErr)
	}
	if _, err = s.Objects.PutBytes(name, data); err != nil {
		return fmt.Errorf("snapshot object publication: %w", err)
	}
	if _, err = verifySnapshotObject(s.Objects, index); err != nil {
		return fmt.Errorf("snapshot object verification: %w", err)
	}
	encoded, err := (proto.MarshalOptions{Deterministic: true}).Marshal(index)
	if err != nil {
		return fmt.Errorf("snapshot index encoding: %w", err)
	}
	return publishSnapshotIndex(s.Index, key, index, encoded, verifyPrior)
}

// Replica checkpoint publishers may race. A failed CAS leaves the previous
// verified pointer and authoritative log untouched; it is safe to defer only
// genuine contention. Storage and transport failures remain readiness errors.
func publishSnapshotIndex(kv nats.KeyValue, key string, index *pb.SnapshotIndex, encoded []byte, verifyPrevious func(*pb.SnapshotIndex) error) error {
	var conflict error
	for attempts := 0; attempts < 5; attempts++ {
		previous, err := kv.Get(key)
		if errors.Is(err, nats.ErrKeyNotFound) {
			_, err = kv.Create(key, encoded)
		} else if err != nil {
			return fmt.Errorf("snapshot index lookup: %w", err)
		} else {
			prior := &pb.SnapshotIndex{}
			reusable := false
			var priorErr error
			if pb.UnmarshalStrict(previous.Value(), prior) == nil {
				reusable, priorErr = snapshotIndexSupersedes(prior, index, verifyPrevious)
			}
			if priorErr != nil {
				return priorErr
			}
			if reusable {
				return nil
			}
			_, err = kv.Update(key, encoded, previous.Revision())
		}
		if err == nil {
			return nil
		}
		var api *nats.APIError
		if !errors.Is(err, nats.ErrKeyExists) && !(errors.As(err, &api) && (api.ErrorCode == nats.JSErrCodeStreamWrongLastSequence || api.ErrorCode == 10164)) {
			return fmt.Errorf("snapshot index publication failed: %w", err)
		}
		conflict = err
		if attempts < 4 {
			time.Sleep(time.Duration(attempts+1) * 5 * time.Millisecond)
		}
	}
	return fmt.Errorf("%w: %w", ErrSnapshotIndexContended, conflict)
}

// A reused pointer is always verified before it can suppress publication.
// Matching stream checkpoints must describe exactly the same immutable data.
// Corrupt pointers cannot suppress a newly verified checkpoint; transport and
// storage errors are not evidence of corruption and still fail closed.
func snapshotIndexSupersedes(prior, index *pb.SnapshotIndex, verifyPrevious func(*pb.SnapshotIndex) error) (bool, error) {
	if err := validateTyped(prior.ProtoReflect()); err != nil {
		return false, nil
	}
	if prior.SchemaVersion != 1 || prior.LastStreamSequence == 0 || prior.LastSubjectSequence == 0 {
		return false, nil
	}
	if !proto.Equal(prior.Aggregate, index.Aggregate) {
		return false, nil
	}
	if prior.LastStreamSequence < index.LastStreamSequence {
		return false, nil
	}
	if verifyPrevious == nil {
		return false, fmt.Errorf("snapshot previous pointer verifier unavailable")
	}
	if err := verifyPrevious(prior); err != nil {
		if errors.Is(err, errInvalidSnapshotObject) || errors.Is(err, nats.ErrObjectNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("snapshot previous pointer verification: %w", err)
	}
	if prior.LastStreamSequence == index.LastStreamSequence && (prior.Sha256 != index.Sha256 || prior.AggregateRevision != index.AggregateRevision || prior.LastSubjectSequence != index.LastSubjectSequence) {
		return false, ErrImmutableSnapshotCollision
	}
	return true, nil
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

func verifySnapshotObject(objects nats.ObjectStore, index *pb.SnapshotIndex) (verified *pb.Snapshot, failure error) {
	invalidContent := true
	defer func() {
		if failure != nil && invalidContent {
			failure = fmt.Errorf("%w: %w", errInvalidSnapshotObject, failure)
		}
	}()
	if index.SchemaVersion != 1 || index.LastStreamSequence == 0 || index.LastSubjectSequence != index.LastStreamSequence {
		return nil, fmt.Errorf("invalid snapshot index")
	}
	key, err := snapshotKey(index.Aggregate)
	if err != nil {
		return nil, err
	}
	want := fmt.Sprintf("snapshot/%s/%d", strings.ReplaceAll(key, ".", "/"), index.LastStreamSequence)
	suffix := strings.TrimPrefix(index.ObjectName, want+"/")
	if index.ObjectName != want && (!strings.HasPrefix(index.ObjectName, want+"/") || !canonicalUUID(suffix)) {
		return nil, fmt.Errorf("snapshot object identity mismatch")
	}
	// Legacy exact checkpoint names remain readable. New names have exactly
	// one canonical UUID segment; wrong aggregate/sequence/path stays rejected.
	data, err := objects.GetBytes(index.ObjectName)
	if err != nil {
		invalidContent = errors.Is(err, nats.ErrObjectNotFound) || errors.Is(err, nats.ErrDigestMismatch) || errors.Is(err, nats.ErrBadObjectMeta)
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
		if (kind <= 3 && !global) || (kind == 4 && !airport) || (kind >= 5 && kind <= 18 && !session) || (kind >= 19 && kind <= 28 && kind != pb.EntityKind_PROVIDER_CHECKPOINT && !airport) || (kind == pb.EntityKind_PROVIDER_CHECKPOINT && !airport && !global && !(session && (entity.Value.GetProviderCheckpoint().Provider == "viff" || entity.Value.GetProviderCheckpoint().Provider == "hoppie"))) || ((kind == 29 || kind == 30 || kind == 31 || kind == 32) && !session) {
			return nil, fmt.Errorf("snapshot entity in wrong aggregate")
		}
		key, err := recordKey(kind, entity.Value)
		if err != nil || key != entity.Key {
			return nil, fmt.Errorf("snapshot entity key mismatch")
		}
		if throttle := entity.Value.GetSessionSquawkThrottle(); throttle != nil {
			if throttle.SessionId != snapshot.Aggregate.GetSession().GetId() || throttle.NextAllowedAt == nil || throttle.NextAllowedAt.CheckValid() != nil {
				return nil, fmt.Errorf("invalid snapshot squawk throttle")
			}
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
