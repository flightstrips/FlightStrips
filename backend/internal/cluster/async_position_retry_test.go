package cluster

import (
	pb "FlightStrips/pkg/events/cluster"
	"context"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
	"sync"
	"testing"
	"time"
)

type unknownPositionAckKV struct {
	*positionKVTest
	lock      sync.Mutex
	attempts  int
	committed bool
	after     func()
}

func (kv *unknownPositionAckKV) publish(key string, data []byte, expected uint64) (uint64, error) {
	kv.lock.Lock()
	defer kv.lock.Unlock()
	kv.attempts++
	if kv.attempts == 1 {
		if kv.committed {
			var err error
			if expected == 0 {
				_, err = kv.positionKVTest.Create(key, data)
			} else {
				_, err = kv.positionKVTest.Update(key, data, expected)
			}
			if err != nil {
				return 0, err
			}
		}
		if kv.after != nil {
			kv.after()
		}
		return 0, nats.ErrNoStreamResponse
	}
	if expected == 0 {
		return kv.positionKVTest.Create(key, data)
	}
	return kv.positionKVTest.Update(key, data, expected)
}
func (kv *unknownPositionAckKV) Create(key string, data []byte) (uint64, error) {
	return kv.publish(key, data, 0)
}
func (kv *unknownPositionAckKV) Update(key string, data []byte, expected uint64) (uint64, error) {
	return kv.publish(key, data, expected)
}

func TestAsyncPositionUnknownAcknowledgement(t *testing.T) {
	for _, tc := range []struct {
		name      string
		baseline  uint64
		committed bool
		change    string
		wantErr   bool
		attempts  int
	}{
		{name: "create acknowledged by exact read", committed: true, attempts: 1},
		{name: "update acknowledged by exact read", baseline: 7, committed: true, attempts: 1},
		{name: "uncommitted write retries original CAS", attempts: 2},
		{name: "foreign advanced value rejected", committed: true, change: "foreign", wantErr: true, attempts: 1},
		{name: "deleted baseline rejected", baseline: 7, change: "delete", wantErr: true, attempts: 1},
		{name: "expired lease forbids retransmission", change: "expire", wantErr: true, attempts: 1},
		{name: "expired lease permits exact read-only receipt", committed: true, change: "expire", attempts: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			owners, p, ref, gate, _ := asyncOwnersFixture(t)
			close(gate)
			defer owners.owner.NC.Close()
			key := positionKey(42, "SAS1", 1)
			kv := &unknownPositionAckKV{positionKVTest: &positionKVTest{values: map[string]positionKVEntry{}, next: tc.baseline}, committed: tc.committed}
			if tc.baseline != 0 {
				kv.values[key] = positionKVEntry{revision: tc.baseline, data: []byte("previous immutable position")}
			}
			kv.after = func() {
				if tc.change == "foreign" {
					kv.positionKVTest.Update(key, []byte("different position"), tc.baseline+1)
				}
				if tc.change == "delete" {
					kv.mu.Lock()
					delete(kv.values, key)
					kv.mu.Unlock()
				}
				if tc.change == "expire" {
					subject, _ := Subject(ref)
					p.mu.Lock()
					p.states[subject].Owner.LeaseUntil = timestamppb.New(time.Now().Add(-time.Second))
					p.mu.Unlock()
				}
			}
			p.asyncPositions = &asyncPositionState{durable: map[string]uint64{key: tc.baseline}}
			w := &PositionWriter{KV: kv, SessionID: 42, OwnerEpoch: 1, Connection: "master", async: owners, projection: p, revisions: map[string]uint64{}}
			value := localPositionValue()
			value.SessionId = 42
			value.OwnerEpoch = 1
			value.SourceConnectionId = "master"
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			rev, err := w.writeAsyncDurable(ctx, key, value)
			if tc.wantErr {
				require.Error(t, err)
				require.Zero(t, rev)
				require.Empty(t, w.revisions)
			} else {
				require.NoError(t, err)
				require.Equal(t, tc.baseline+1, rev)
				require.Equal(t, rev, w.revisions[key])
			}
			require.Equal(t, tc.attempts, kv.attempts)
		})
	}
}

func TestAsyncPositionRetryPreservesCancelledBoundAndIdentity(t *testing.T) {
	owners, p, _, gate, _ := asyncOwnersFixture(t)
	close(gate)
	defer owners.owner.NC.Close()
	key := positionKey(42, "SAS1", 1)
	p.asyncPositions = &asyncPositionState{durable: map[string]uint64{}}
	kv := &unknownPositionAckKV{positionKVTest: &positionKVTest{values: map[string]positionKVEntry{}}}
	w := &PositionWriter{KV: kv, SessionID: 42, OwnerEpoch: 1, Connection: "master", async: owners, projection: p, revisions: map[string]uint64{}}
	value := localPositionValue()
	value.SessionId = 42
	value.OwnerEpoch = 1
	value.SourceConnectionId = "master"
	ctx, cancel := context.WithCancel(context.Background())
	kv.after = cancel
	_, err := w.writeAsyncDurable(ctx, key, value)
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, 1, kv.attempts)
	_, err = w.writeAsyncDurable(context.Background(), key, &pb.PositionValue{SchemaVersion: 1, SessionId: 42, OwnerEpoch: 2})
	require.ErrorContains(t, err, "identity")
	require.Equal(t, 1, kv.attempts)
}

func TestAsyncPositionReconciledReceiptRemapsFIFOAliases(t *testing.T) {
	owners, p, ref, gate, _ := asyncOwnersFixture(t)
	close(gate)
	defer owners.owner.NC.Close()
	p.positions = map[string]KVPosition{}
	p.positionCursor.lastProved = time.Now()
	p.asyncPositions = &asyncPositionState{values: map[string]KVPosition{}, durable: map[string]uint64{}, tokens: map[uint64]uint64{}, pending: map[uint64]*pb.PositionValue{}, baselines: map[string]bool{}}
	kv := &unknownPositionAckKV{positionKVTest: &positionKVTest{values: map[string]positionKVEntry{}}, committed: true}
	w, err := NewPositionWriter(kv, 42, 1, "master", func(context.Context, int32, uint64, string) error { return nil }, 1, 8)
	require.NoError(t, err)
	defer w.Close(context.Background())
	w.async, w.projection, w.baselineReady = owners, p, true
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var tokens []uint64
	require.NoError(t, owners.Execute(ctx, ref, func(turn context.Context) error {
		first, err := w.QueuePosition(turn, "SAS1", localPositionValue().GetPosition(), time.Now())
		if err != nil {
			return err
		}
		tokens = append(tokens, (<-first).Revision)
		second, err := w.QueueDisconnect(turn, "SAS1", time.Now())
		if err != nil {
			return err
		}
		tokens = append(tokens, (<-second).Revision)
		return nil
	}))
	owners.BeginDrain()
	require.NoError(t, owners.Drain(ctx))
	p.mu.RLock()
	defer p.mu.RUnlock()
	require.Equal(t, uint64(1), p.asyncPositions.tokens[tokens[0]])
	require.Equal(t, uint64(2), p.asyncPositions.tokens[tokens[1]])
	require.Equal(t, uint64(2), p.asyncPositions.durable[positionKey(42, "SAS1", 1)])
	require.Empty(t, p.asyncPositions.pending)
	require.Equal(t, 2, kv.attempts, "unknown first acknowledgement must not duplicate the immutable position")
}
