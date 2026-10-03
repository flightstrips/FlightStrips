package cluster

import (
	"context"
	"errors"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
	"sync/atomic"
	"testing"
	"time"
)

type staleReplayReceiptStore struct {
	*asyncReceiptStore
	p         *Projection
	mode      string
	entered   chan struct{}
	publishes atomic.Int32
	receipts  atomic.Int32
	integrity error
}

func (s *staleReplayReceiptStore) stale(subject string) {
	staleAsyncMetadata(s.p)
	s.p.mu.Lock()
	if s.integrity != nil {
		s.p.healthErr = s.integrity
	}
	if s.mode == "expired-cas" {
		next := copyAggregateForApply(s.p.states[subject])
		next.Owner.LeaseUntil = timestamppb.New(time.Now().Add(-time.Second))
		s.p.states[subject] = next
	}
	s.p.mu.Unlock()
	close(s.entered)
}
func (s *staleReplayReceiptStore) Publish(ctx context.Context, subject string, expected uint64, data []byte) (uint64, error) {
	n := s.publishes.Add(1)
	seq, err := s.asyncReceiptStore.Publish(ctx, subject, expected, data)
	if s.mode == "cas" || s.mode == "expired-cas" {
		if n == 1 && err == nil {
			return 0, nats.ErrNoStreamResponse
		}
		if n == 2 && errors.Is(err, ErrCAS) {
			s.stale(subject)
		}
	}
	return seq, err
}
func (s *staleReplayReceiptStore) Committed(ctx context.Context, seq uint64) (AppliedEvent, error) {
	n := s.receipts.Add(1)
	entry, err := s.asyncReceiptStore.Committed(ctx, seq)
	if err == nil && n == 1 && s.mode != "cas" && s.mode != "expired-cas" {
		s.stale(entry.Subject)
	}
	return entry, err
}

func TestAsyncReceiptReplayWaitAwaitsFreshProofWithoutRepublishing(t *testing.T) {
	for _, mode := range []string{"puback", "expired-puback", "cas"} {
		t.Run(mode, func(t *testing.T) {
			m, p, ref, gate, original := asyncOwnersFixture(t)
			defer m.owner.NC.Close()
			store := &staleReplayReceiptStore{asyncReceiptStore: original, p: p, mode: mode, entered: make(chan struct{})}
			m.store = store
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			var leaseEnd time.Time
			if mode == "expired-puback" {
				leaseEnd = time.Now().Add(500 * time.Millisecond)
				subject, _ := Subject(ref)
				p.mu.Lock()
				next := copyAggregateForApply(p.states[subject])
				next.Owner.LeaseUntil = timestamppb.New(leaseEnd)
				p.states[subject] = next
				p.mu.Unlock()
			}
			event := asyncDomainEvent(ref, 1)
			require.NoError(t, m.Execute(ctx, ref, func(turn context.Context) error {
				base, err := m.Read(ref)
				if err != nil {
					return err
				}
				_, err = m.AcceptState(turn, base, event)
				return err
			}))
			close(gate)
			select {
			case <-store.entered:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			time.Sleep(120 * time.Millisecond)
			require.NoError(t, m.Err(), "an exact receipt or subject CAS must await the stale replay proof")
			require.True(t, m.Pending(ref))
			require.ErrorContains(t, p.Ready(), "state metadata is stale")
			if mode == "expired-puback" {
				time.Sleep(time.Until(leaseEnd.Add(10 * time.Millisecond)))
				require.False(t, m.Active(ref))
				require.NoError(t, m.Err())
			}
			subject, _ := Subject(ref)
			p.mu.RLock()
			require.Zero(t, p.states[subject].Revision, "raw replay was deliberately left unapplied")
			p.mu.RUnlock()
			entry, err := original.Committed(ctx, 2)
			require.NoError(t, err)
			require.NoError(t, p.apply(entry), "real ordered replay must apply the exact stored event")
			restoreAsyncMetadata(p)
			require.NoError(t, m.Drain(ctx))
			require.NoError(t, m.Err())
			if mode == "expired-puback" {
				require.False(t, m.Active(ref), "read-only receipt retirement cannot revive the expired owner")
			}
			expectedPublishes := int32(1)
			if mode == "cas" {
				expectedPublishes = 2
			}
			require.Equal(t, expectedPublishes, store.publishes.Load())
			require.Equal(t, int32(1), store.receipts.Load())
			p.mu.RLock()
			outcome, err := p.states[subject].LookupOutcome(event.GetCommandId())
			p.mu.RUnlock()
			require.NoError(t, err)
			require.Equal(t, uint64(2), outcome.CommittedStreamSequence)
		})
	}
}

func TestAsyncReceiptReplayWaitRetainsIntegrityAndLeaseFences(t *testing.T) {
	for _, mode := range []string{"integrity", "expired-cas"} {
		t.Run(mode, func(t *testing.T) {
			m, p, ref, gate, original := asyncOwnersFixture(t)
			defer m.owner.NC.Close()
			store := &staleReplayReceiptStore{asyncReceiptStore: original, p: p, mode: mode, entered: make(chan struct{})}
			if mode == "integrity" {
				store.integrity = errors.New("FS_STATE stream identity changed")
			}
			m.store = store
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
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
			require.Error(t, m.Drain(ctx))
			require.Error(t, m.Err())
			expectedPublishes := int32(1)
			if mode == "expired-cas" {
				expectedPublishes = 2
				require.ErrorContains(t, m.Err(), "owner generation or integrity changed")
			} else {
				require.ErrorIs(t, m.Err(), store.integrity)
			}
			require.Equal(t, expectedPublishes, store.publishes.Load(), "a permanent failure cannot trigger another publication")
		})
	}
}

func TestAsyncReceiptReplayWaitKeepsCallerDeadline(t *testing.T) {
	for _, kind := range []string{"receipt", "cas"} {
		t.Run(kind, func(t *testing.T) {
			m, p, ref, gate, _ := asyncOwnersFixture(t)
			close(gate)
			defer m.owner.NC.Close()
			staleAsyncMetadata(p)
			ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
			defer cancel()
			var err error
			if kind == "receipt" {
				err = m.persistenceReplayApplied(ctx, 2)
			} else {
				err = m.persistenceSubjectAdvance(ctx, ref, 1, 1)
			}
			require.ErrorIs(t, err, context.DeadlineExceeded)
			require.NoError(t, m.Err(), "a wait cannot clear or manufacture manager failure")
		})
	}
}
