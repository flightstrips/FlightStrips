package cluster

import (
	"errors"
	"strings"
	"testing"

	pb "FlightStrips/pkg/events/cluster"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

type immutableSnapshotObjects struct {
	nats.ObjectStore
	values         map[string][]byte
	names          []string
	putErr, getErr error
}

func (o *immutableSnapshotObjects) PutBytes(name string, data []byte, _ ...nats.ObjectOpt) (*nats.ObjectInfo, error) {
	if o.putErr != nil {
		return nil, o.putErr
	}
	if o.values == nil {
		o.values = map[string][]byte{}
	}
	if _, exists := o.values[name]; exists {
		return nil, errors.New("object overwrite forbidden")
	}
	o.values[name] = append([]byte(nil), data...)
	o.names = append(o.names, name)
	return &nats.ObjectInfo{}, nil
}
func (o *immutableSnapshotObjects) GetBytes(name string, _ ...nats.GetObjectOpt) ([]byte, error) {
	if o.getErr != nil {
		return nil, o.getErr
	}
	value, ok := o.values[name]
	if !ok {
		return nil, nats.ErrObjectNotFound
	}
	return append([]byte(nil), value...), nil
}
func snapshotRaceAggregate() *Aggregate {
	a := NewAggregate(sessionRef(1))
	a.StreamSequence = 10
	a.SubjectSequence = 10
	a.Revision = 2
	return a
}
func decodeSnapshotIndex(t *testing.T, k *snapshotIndexKV) *pb.SnapshotIndex {
	t.Helper()
	index := &pb.SnapshotIndex{}
	require.NoError(t, pb.UnmarshalStrict(k.value, index))
	return index
}
func TestSnapshotSaveUsesFreshObjectsAndRetainsVerifiedPointer(t *testing.T) {
	k := &snapshotIndexKV{}
	o := &immutableSnapshotObjects{}
	store := SnapshotStore{Index: k, Objects: o}
	a := snapshotRaceAggregate()
	require.NoError(t, store.Save(a))
	first := append([]byte(nil), k.value...)
	firstIndex := decodeSnapshotIndex(t, k)
	require.NoError(t, store.Save(a))
	require.Equal(t, first, k.value)
	require.Len(t, o.names, 1, "sequential duplicate must reuse its verified pointer")
	require.True(t, canonicalUUID(strings.TrimPrefix(firstIndex.ObjectName, "snapshot/session/1/10/")))
	a.Revision++
	require.ErrorIs(t, store.Save(a), ErrImmutableSnapshotCollision)
	require.Equal(t, first, k.value)
	require.Len(t, o.names, 1, "collision must preserve the existing object")
	o.values[firstIndex.ObjectName] = []byte("corrupt")
	require.NoError(t, store.Save(snapshotRaceAggregate()))
	repaired := decodeSnapshotIndex(t, k)
	require.NotEqual(t, firstIndex.ObjectName, repaired.ObjectName)
	_, err := verifySnapshotObject(o, repaired)
	require.NoError(t, err)
	// A verified newer pointer remains intact when an older replay checkpoints.
	repairedBytes := append([]byte(nil), k.value...)
	older := snapshotRaceAggregate()
	older.StreamSequence = 5
	older.SubjectSequence = 5
	require.NoError(t, store.Save(older))
	require.Equal(t, repairedBytes, k.value)

}
func TestSnapshotObjectNamesAcceptLegacyAndCanonicalUniqueSuffix(t *testing.T) {
	a := snapshotRaceAggregate()
	snapshot, err := a.Snapshot()
	require.NoError(t, err)
	object := &pb.ObjectValue{SchemaVersion: 1, Content: &pb.ObjectValue_Snapshot{Snapshot: snapshot}}
	data, err := proto.Marshal(object)
	require.NoError(t, err)
	prefix := "snapshot/session/1/10"
	suffix := "11111111-2222-4333-8aaa-555555555555"
	for _, name := range []string{prefix, prefix + "/" + suffix, prefix + "/" + strings.ToUpper(suffix), prefix + "/" + suffix + "/extra", "snapshot/session/2/10/" + suffix, "snapshot/session/1/11/" + suffix, prefix + "/garbage"} {
		t.Run(name, func(t *testing.T) {
			o := &immutableSnapshotObjects{values: map[string][]byte{name: data}}
			index := &pb.SnapshotIndex{SchemaVersion: 1, Aggregate: sessionRef(1), ObjectName: name, Sha256: digest(data), AggregateRevision: a.Revision, LastStreamSequence: a.StreamSequence, LastSubjectSequence: a.SubjectSequence}
			_, err := verifySnapshotObject(o, index)
			if name == prefix || name == prefix+"/"+suffix {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, "identity mismatch")
			}
		})
	}
}
func TestSnapshotStageWrappingPreservesNATSErrors(t *testing.T) {
	for _, failure := range []error{nats.ErrTimeout, &nats.APIError{Code: 503, ErrorCode: 10008, Description: "storage unavailable"}} {
		k := &snapshotIndexKV{}
		o := &immutableSnapshotObjects{putErr: failure}
		store := SnapshotStore{Index: k, Objects: o}
		err := store.Save(snapshotRaceAggregate())
		require.ErrorIs(t, err, failure)
		require.ErrorContains(t, err, "object publication")
		require.Zero(t, k.revision)
		o.putErr = nil
		o.getErr = failure
		err = store.Save(snapshotRaceAggregate())
		require.ErrorIs(t, err, failure)
		require.ErrorContains(t, err, "object verification")
		require.Zero(t, k.revision)
	}
}
func TestSnapshotSaveRepairsInvalidPriorIndexAfterVerifiedReplay(t *testing.T) {
	for _, kind := range []string{"malformed index", "invalid identity", "missing object"} {
		t.Run(kind, func(t *testing.T) {
			k := &snapshotIndexKV{}
			o := &immutableSnapshotObjects{}
			store := SnapshotStore{Index: k, Objects: o}
			a := snapshotRaceAggregate()
			require.NoError(t, store.Save(a))
			prior := decodeSnapshotIndex(t, k)
			switch kind {
			case "malformed index":
				k.value = []byte{0xff}
			case "invalid identity":
				prior.SchemaVersion = 99
				prior.LastStreamSequence = 1000
				k.value, _ = proto.Marshal(prior)
			case "missing object":
				delete(o.values, prior.ObjectName)
			}
			require.NoError(t, store.Save(a))
			repaired := decodeSnapshotIndex(t, k)
			require.Equal(t, a.StreamSequence, repaired.LastStreamSequence)
			_, err := verifySnapshotObject(o, repaired)
			require.NoError(t, err)
		})
	}
}
func TestSnapshotSaveDoesNotTreatPriorTransportFailureAsCorruption(t *testing.T) {
	k := &snapshotIndexKV{}
	o := &immutableSnapshotObjects{}
	store := SnapshotStore{Index: k, Objects: o}
	require.NoError(t, store.Save(snapshotRaceAggregate()))
	prior := append([]byte(nil), k.value...)
	o.getErr = nats.ErrTimeout
	require.ErrorIs(t, store.Save(snapshotRaceAggregate()), nats.ErrTimeout)
	require.Equal(t, prior, k.value)
	require.Len(t, o.names, 1)
}
