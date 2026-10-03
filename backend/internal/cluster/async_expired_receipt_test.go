package cluster

import (
	pb "FlightStrips/pkg/events/cluster"
	"context"
	"errors"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type expiredReceiptStore struct {
	*asyncReceiptStore
	p                   *Projection
	mode                string
	entered, release    chan struct{}
	publishes, receipts atomic.Int32
}

func (s *expiredReceiptStore) Publish(ctx context.Context, subject string, expected uint64, data []byte) (uint64, error) {
	n := s.publishes.Add(1)
	if n == 1 && s.mode == "foreign" {
		event := decodeAsyncEvent(data)
		foreign := asyncDomainEvent(event.Aggregate, event.AggregateRevision)
		var err error
		data, err = proto.Marshal(foreign)
		if err != nil {
			return 0, err
		}
	}
	seq, err := s.asyncReceiptStore.Publish(ctx, subject, expected, data)
	if n == 1 && err == nil {
		if s.mode == "cas" {
			return 0, nats.ErrNoStreamResponse
		}
		close(s.entered)
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-s.release:
		}
		return 0, nats.ErrNoStreamResponse
	}
	if n == 2 && s.mode == "cas" && errors.Is(err, ErrCAS) {
		staleAsyncMetadata(s.p)
		close(s.entered)
	}
	return seq, err
}
func (s *expiredReceiptStore) Committed(ctx context.Context, seq uint64) (AppliedEvent, error) {
	s.receipts.Add(1)
	return s.asyncReceiptStore.Committed(ctx, seq)
}

func TestAsyncExpiredReceiptReconcilesOnlyProvenFinalTail(t *testing.T) {
	for _, mode := range []string{"cas", "unknown-ack", "foreign", "remaining-tail"} {
		t.Run(mode, func(t *testing.T) {
			m, p, ref, gate, original := asyncOwnersFixture(t)
			defer m.owner.NC.Close()
			store := &expiredReceiptStore{asyncReceiptStore: original, p: p, mode: mode, entered: make(chan struct{}), release: make(chan struct{})}
			m.store = store
			var released sync.Once
			t.Cleanup(func() { released.Do(func() { close(store.release) }) })
			subject, _ := Subject(ref)
			leaseEnd := time.Now().Add(500 * time.Millisecond)
			p.mu.Lock()
			next := copyAggregateForApply(p.states[subject])
			next.Owner.LeaseUntil = timestamppb.New(leaseEnd)
			p.states[subject] = next
			p.mu.Unlock()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
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
			var remaining *pb.StateEvent
			if mode == "remaining-tail" {
				remaining = asyncDomainEvent(ref, 2)
				require.NoError(t, m.Execute(ctx, ref, func(turn context.Context) error {
					base, err := m.Read(ref)
					if err != nil {
						return err
					}
					_, err = m.AcceptState(turn, base, remaining)
					return err
				}))
			}
			staleAsyncMetadata(p)
			time.Sleep(time.Until(leaseEnd.Add(20 * time.Millisecond)))
			released.Do(func() { close(store.release) })
			time.Sleep(120 * time.Millisecond)
			require.NoError(t, m.Err(), "an uncertain receipt can be awaited without reviving the expired owner")
			require.True(t, m.Pending(ref))
			require.False(t, m.Active(ref))
			entry, err := original.Committed(ctx, 2)
			require.NoError(t, err)
			require.True(t, entry.ServerTime.Before(leaseEnd))
			require.NoError(t, p.apply(entry))
			restoreAsyncMetadata(p)
			err = m.Drain(ctx)
			if mode == "foreign" || mode == "remaining-tail" {
				require.Error(t, err)
				require.Error(t, m.Err())
			} else {
				require.NoError(t, err)
				require.NoError(t, m.Err())
				require.False(t, m.Pending(ref))
			}
			require.False(t, m.Active(ref), "proof of past persistence cannot authorize a new old-term mutation")
			attempts := int32(1)
			if mode == "cas" {
				attempts = 2
			}
			require.Equal(t, attempts, store.publishes.Load())
			if mode == "foreign" {
				require.ErrorIs(t, err, nats.ErrNoStreamResponse)
				require.Zero(t, store.receipts.Load())
				p.mu.RLock()
				outcome, lookupErr := p.states[subject].LookupOutcome(event.GetCommandId())
				p.mu.RUnlock()
				require.NoError(t, lookupErr)
				require.Nil(t, outcome)
			} else {
				require.Equal(t, int32(1), store.receipts.Load())
			}
			if remaining != nil {
				p.mu.RLock()
				outcome, lookupErr := p.states[subject].LookupOutcome(remaining.GetCommandId())
				p.mu.RUnlock()
				require.NoError(t, lookupErr)
				require.Nil(t, outcome, "remaining accepted tail must not be persisted under expired authority")
			}
		})
	}
}

func TestAsyncRetryGuardReportsSafeStructuralReason(t *testing.T) {
	for _, reason := range []string{"owner_missing", "owner_node_changed", "owner_epoch_changed", "owner_lease_expired", "observation_integrity", "metadata_integrity", "snapshot_integrity"} {
		t.Run(reason, func(t *testing.T) {
			m, p, ref, gate, _ := asyncOwnersFixture(t)
			close(gate)
			defer m.owner.NC.Close()
			subject, _ := Subject(ref)
			const secret = "private-command-or-credential"
			p.mu.Lock()
			next := copyAggregateForApply(p.states[subject])
			p.states[subject] = next
			switch reason {
			case "owner_missing":
				next.Owner = nil
			case "owner_node_changed":
				next.Owner.NodeId = "other-node"
			case "owner_epoch_changed":
				next.Owner.Epoch++
			case "owner_lease_expired":
				next.Owner.LeaseUntil = timestamppb.New(time.Now().Add(-time.Second))
			case "observation_integrity":
				p.observationErr = errors.New(secret)
			case "metadata_integrity":
				p.healthErr = errors.New(secret)
			case "snapshot_integrity":
				p.snapshotErrors = map[string]error{subject: errors.New(secret)}
			}
			p.mu.Unlock()
			err := m.waitPersistenceRetry(context.Background(), ref, 1)
			var rejected *asyncRetryRejected
			require.ErrorAs(t, err, &rejected)
			require.Equal(t, reason, rejected.reason)
			require.NotContains(t, err.Error(), secret)
		})
	}
}
