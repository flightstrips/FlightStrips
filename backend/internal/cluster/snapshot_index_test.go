package cluster

import (
	pb "FlightStrips/pkg/events/cluster"
	"errors"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"testing"
	"time"
)

type snapshotIndexKV struct {
	nats.KeyValue
	value         []byte
	revision      uint64
	conflictCount int
	failure       error
	attempts      int
}

func (k *snapshotIndexKV) Get(string) (nats.KeyValueEntry, error) {
	if k.revision == 0 {
		return nil, nats.ErrKeyNotFound
	}
	return positionKVEntry{data: k.value, revision: k.revision}, nil
}
func (k *snapshotIndexKV) Create(_ string, data []byte) (uint64, error) { return k.Update("", data, 0) }
func (k *snapshotIndexKV) Update(_ string, data []byte, expected uint64) (uint64, error) {
	k.attempts++
	if k.failure != nil {
		return 0, k.failure
	}
	if k.conflictCount > 0 {
		k.conflictCount--
		return 0, &nats.APIError{ErrorCode: 10164, Code: 400}
	}
	if expected != k.revision {
		return 0, nats.ErrKeyExists
	}
	k.revision++
	k.value = append([]byte(nil), data...)
	return k.revision, nil
}
func TestSnapshotIndexContentionDoesNotRegressVerifiedPointer(t *testing.T) {
	prior := &pb.SnapshotIndex{LastStreamSequence: 10}
	newer := &pb.SnapshotIndex{LastStreamSequence: 20}
	oldData, err := proto.Marshal(prior)
	require.NoError(t, err)
	newData, err := proto.Marshal(newer)
	require.NoError(t, err)
	k := &snapshotIndexKV{value: oldData, revision: 1, conflictCount: 2}
	require.NoError(t, publishSnapshotIndex(k, "session.1", newer, newData))
	require.Equal(t, 3, k.attempts)
	require.NoError(t, publishSnapshotIndex(k, "session.1", prior, oldData))
	require.Equal(t, newData, k.value)
	require.Equal(t, 3, k.attempts)
	k.conflictCount = 5
	newest := &pb.SnapshotIndex{LastStreamSequence: 30}
	data, err := proto.Marshal(newest)
	require.NoError(t, err)
	require.ErrorIs(t, publishSnapshotIndex(k, "session.1", newest, data), ErrSnapshotIndexContended)
	require.Equal(t, newData, k.value)
}
func TestSnapshotIndexNonCASFailureIsNotContention(t *testing.T) {
	failure := errors.New("storage unavailable")
	k := &snapshotIndexKV{failure: failure}
	err := publishSnapshotIndex(k, "session.1", &pb.SnapshotIndex{LastStreamSequence: 1}, []byte{1})
	require.ErrorIs(t, err, failure)
	require.NotErrorIs(t, err, ErrSnapshotIndexContended)
	require.Equal(t, 1, k.attempts)
}

func TestDeferredSnapshotDoesNotDisableReadyProjection(t *testing.T) {
	p := &Projection{started: true, checked: time.Now(), positionReady: true, presenceReady: true, snapshotErrors: map[string]error{}, lastSnapshot: map[string]time.Time{}}
	p.finishSnapshot("session.1", ErrSnapshotIndexContended)
	require.Equal(t, uint64(1), p.snapshotIndexContentions.Load())
	require.NoError(t, p.Ready())
	p.finishSnapshot("session.1", errors.New("snapshot storage unavailable"))
	require.Error(t, p.Ready())
}
