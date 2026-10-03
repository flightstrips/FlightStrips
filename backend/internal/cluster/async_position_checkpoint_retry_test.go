package cluster

import (
	pb "FlightStrips/pkg/events/cluster"
	"context"
	"errors"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
	"sync/atomic"
	"testing"
	"time"
)

func staleAsyncMetadata(p *Projection) {
	p.mu.Lock()
	p.checked = time.Now().Add(-3 * time.Second)
	p.mu.Unlock()
}
func restoreAsyncMetadata(p *Projection) { p.mu.Lock(); p.checked = time.Now(); p.mu.Unlock() }

func TestAsyncPositionWorkerAwaitsFreshMetadataBeforePersistence(t *testing.T) {
	for _, mode := range []string{"recover", "expired lease", "stream identity failure"} {
		t.Run(mode, func(t *testing.T) {
			m, p, ref, gate, _ := asyncOwnersFixture(t)
			close(gate)
			defer m.owner.NC.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			var persisted atomic.Int32
			require.NoError(t, m.Execute(ctx, ref, func(turn context.Context) error {
				return m.Append(turn, ref, func(context.Context) error {
					staleAsyncMetadata(p)
					if mode != "recover" {
						subject, _ := Subject(ref)
						p.mu.Lock()
						if mode == "expired lease" {
							next := copyAggregateForApply(p.states[subject])
							next.Owner.LeaseUntil = timestamppb.New(time.Now().Add(-time.Second))
							p.states[subject] = next
						} else {
							p.healthErr = errors.New("FS_STATE stream identity changed")
						}
						p.mu.Unlock()
					}
					return nil
				}, func(context.Context) error { persisted.Add(1); return nil })
			}))
			if mode == "recover" {
				time.Sleep(120 * time.Millisecond)
				require.NoError(t, m.Err())
				require.Zero(t, persisted.Load(), "stale proof cannot authorize a publication")
				require.ErrorContains(t, p.Ready(), "state metadata is stale")
				restoreAsyncMetadata(p)
				require.NoError(t, m.Drain(ctx))
				require.Equal(t, int32(1), persisted.Load())
			} else {
				require.Error(t, m.Drain(ctx))
				require.Error(t, m.Err())
				require.Zero(t, persisted.Load())
			}
		})
	}
}

func TestAsyncPositionWorkerAwaitsFreshMetadataAfterExactReceipt(t *testing.T) {
	for _, expired := range []bool{false, true} {
		t.Run(map[bool]string{false: "same owner resumes", true: "expired final receipt retires"}[expired], func(t *testing.T) {
			m, p, ref, gate, _ := asyncOwnersFixture(t)
			close(gate)
			defer m.owner.NC.Close()
			p.positions = map[string]KVPosition{}
			p.positionCursor.lastProved = time.Now()
			p.asyncPositions = &asyncPositionState{values: map[string]KVPosition{}, durable: map[string]uint64{}, tokens: map[uint64]uint64{}, pending: map[uint64]*pb.PositionValue{}, baselines: map[string]bool{}}
			receipt := make(chan struct{})
			kv := &unknownPositionAckKV{positionKVTest: &positionKVTest{values: map[string]positionKVEntry{}}, committed: true, after: func() {
				staleAsyncMetadata(p)
				if expired {
					subject, _ := Subject(ref)
					p.mu.Lock()
					next := copyAggregateForApply(p.states[subject])
					next.Owner.LeaseUntil = timestamppb.New(time.Now().Add(-time.Second))
					p.states[subject] = next
					p.mu.Unlock()
				}
				close(receipt)
			}}
			w, err := NewPositionWriter(kv, 42, 1, "master", func(context.Context, int32, uint64, string) error { return nil }, 1, 8)
			require.NoError(t, err)
			defer w.Close(context.Background())
			w.async, w.projection, w.baselineReady = m, p, true
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			var token uint64
			require.NoError(t, m.Execute(ctx, ref, func(turn context.Context) error {
				ch, err := w.QueuePosition(turn, "SAS1", localPositionValue().GetPosition(), time.Now())
				if err != nil {
					return err
				}
				token = (<-ch).Revision
				return nil
			}))
			select {
			case <-receipt:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			time.Sleep(120 * time.Millisecond)
			require.NoError(t, m.Err())
			require.True(t, m.Pending(ref))
			require.ErrorContains(t, p.Ready(), "state metadata is stale")
			restoreAsyncMetadata(p)
			require.NoError(t, m.Drain(ctx))
			require.NoError(t, m.Err())
			require.False(t, m.Pending(ref))
			if expired {
				require.False(t, m.Active(ref))
			}
			p.mu.RLock()
			require.Equal(t, uint64(1), p.asyncPositions.tokens[token])
			p.mu.RUnlock()
			require.Equal(t, 1, kv.attempts, "receipt refresh must not republish the position")
		})
	}
}

type staleConfirmationStore struct {
	*asyncReceiptStore
	p         *Projection
	entered   chan struct{}
	failure   error
	receipts  atomic.Int32
	publishes atomic.Int32
}

func (s *staleConfirmationStore) Publish(ctx context.Context, subject string, expected uint64, data []byte) (uint64, error) {
	s.publishes.Add(1)
	return s.asyncReceiptStore.Publish(ctx, subject, expected, data)
}
func (s *staleConfirmationStore) Committed(ctx context.Context, seq uint64) (AppliedEvent, error) {
	entry, err := s.asyncReceiptStore.Committed(ctx, seq)
	if err == nil && s.receipts.Add(1) == 1 {
		// Independent ordered replay confirms the receipt before metadata expires.
		if err = s.p.apply(entry); err != nil {
			return AppliedEvent{}, err
		}
		staleAsyncMetadata(s.p)
		if s.failure != nil {
			s.p.mu.Lock()
			s.p.healthErr = s.failure
			s.p.mu.Unlock()
		}
		close(s.entered)
	}
	return entry, err
}
func TestAsyncStateConfirmationAwaitsMetadataWithoutRepublishing(t *testing.T) {
	m, p, ref, gate, store := asyncOwnersFixture(t)
	defer m.owner.NC.Close()
	wrapped := &staleConfirmationStore{asyncReceiptStore: store, p: p, entered: make(chan struct{})}
	m.store = wrapped
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, m.Execute(ctx, ref, func(turn context.Context) error {
		base, err := m.Read(ref)
		if err != nil {
			return err
		}
		_, err = m.AcceptState(turn, base, asyncDomainEvent(ref, 1))
		return err
	}))
	close(gate)
	select {
	case <-wrapped.entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	time.Sleep(120 * time.Millisecond)
	require.NoError(t, m.Err())
	require.True(t, m.Pending(ref))
	require.ErrorContains(t, p.Ready(), "state metadata is stale")
	restoreAsyncMetadata(p)
	require.NoError(t, m.Drain(ctx))
	require.NoError(t, m.Err())
	require.Equal(t, int32(1), wrapped.publishes.Load())
	require.Equal(t, int32(1), wrapped.receipts.Load())
}

func TestAsyncStateConfirmationRejectsStreamIntegrityFailure(t *testing.T) {
	m, p, ref, gate, store := asyncOwnersFixture(t)
	defer m.owner.NC.Close()
	integrity := errors.New("FS_STATE stream identity changed")
	wrapped := &staleConfirmationStore{asyncReceiptStore: store, p: p, entered: make(chan struct{}), failure: integrity}
	m.store = wrapped
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, m.Execute(ctx, ref, func(turn context.Context) error {
		base, err := m.Read(ref)
		if err != nil {
			return err
		}
		_, err = m.AcceptState(turn, base, asyncDomainEvent(ref, 1))
		return err
	}))
	close(gate)
	require.ErrorIs(t, m.Drain(ctx), integrity)
	require.ErrorIs(t, m.Err(), integrity)
	require.Equal(t, int32(1), wrapped.publishes.Load())
}
