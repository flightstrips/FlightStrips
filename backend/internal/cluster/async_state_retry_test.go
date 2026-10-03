package cluster

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	pb "FlightStrips/pkg/events/cluster"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type retryStateStore struct {
	*asyncReceiptStore
	p         *Projection
	ref       *pb.AggregateRef
	mode      string
	t         *testing.T
	publishes atomic.Int32
	receipts  atomic.Int32
}

func (s *retryStateStore) Publish(ctx context.Context, subject string, expected uint64, data []byte) (uint64, error) {
	n := s.publishes.Add(1)
	if n == 1 && (s.mode == "not-committed" || s.mode == "expired") {
		if s.mode == "expired" {
			s.p.mu.Lock()
			copy := copyAggregateForApply(s.p.states[subject])
			copy.Owner.LeaseUntil = timestamppb.New(time.Now().Add(-time.Second))
			s.p.states[subject] = copy
			s.p.mu.Unlock()
		}
		return 0, nats.ErrNoStreamResponse
	}
	seq, err := s.asyncReceiptStore.Publish(ctx, subject, expected, data)
	if err != nil {
		if s.mode == "committed-late-replay" && n == 2 {
			entry, readErr := s.asyncReceiptStore.Committed(ctx, 2)
			if readErr != nil {
				return 0, readErr
			}
			if readErr = s.p.apply(entry); readErr != nil {
				return 0, readErr
			}
		}
		return 0, err
	}
	if n == 1 && s.mode == "committed-late-replay" {
		return 0, nats.ErrNoStreamResponse
	}
	if n == 1 && (s.mode == "committed" || s.mode == "committed-new-epoch" || s.mode == "identity-mismatch") {
		entry, err := s.asyncReceiptStore.Committed(ctx, seq)
		if err != nil {
			return 0, err
		}
		if err = s.p.apply(entry); err != nil {
			return 0, err
		}
		if s.mode == "committed-new-epoch" {
			reclaimAsyncFixtureEpoch(s.t, s.p, s.ref)
		}
		return 0, nats.ErrNoStreamResponse
	}
	return seq, nil
}

func (s *retryStateStore) Committed(ctx context.Context, seq uint64) (AppliedEvent, error) {
	if s.receipts.Add(1) == 1 && s.mode == "receipt" {
		return AppliedEvent{}, nats.ErrTimeout
	}
	entry, err := s.asyncReceiptStore.Committed(ctx, seq)
	if err == nil && s.mode == "identity-mismatch" {
		event := decodeAsyncEvent(entry.Data)
		event.Actor.Id = "foreign"
		entry.Data, err = proto.Marshal(event)
	}
	return entry, err
}

func TestAsyncStateRetryReconcilesDurabilityWithoutDuplicateFacts(t *testing.T) {
	for _, mode := range []string{"not-committed", "committed", "committed-late-replay", "committed-new-epoch", "receipt", "expired", "identity-mismatch"} {
		t.Run(mode, func(t *testing.T) {
			m, p, ref, gate, original := asyncOwnersFixture(t)
			close(gate)
			store := &retryStateStore{asyncReceiptStore: original, p: p, ref: ref, mode: mode, t: t}
			m.store = store
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
			err := m.Drain(ctx)
			if mode == "expired" || mode == "identity-mismatch" {
				require.Error(t, err)
				require.NotNil(t, m.Err())
				require.Nil(t, m.Control(ref))
				require.Equal(t, int32(1), store.publishes.Load())
				return
			}
			require.NoError(t, err)
			require.NoError(t, m.Err())
			require.Len(t, original.entries, 2, "one owner claim and exactly one domain fact")
			raw, err := p.readOwnedDurable(ref, "node-a")
			require.NoError(t, err)
			require.Equal(t, uint64(1), raw.Revision)
			require.Equal(t, uint64(2), raw.Ledger[event.GetCommandId()].CommittedStreamSequence)
			if mode == "committed-new-epoch" {
				view, err := m.Read(ref)
				require.NoError(t, err)
				require.Equal(t, uint64(2), view.Owner.Epoch)
			}
			if mode == "not-committed" || mode == "committed-late-replay" {
				require.Equal(t, int32(2), store.publishes.Load())
			} else {
				require.Equal(t, int32(1), store.publishes.Load())
			}
		})
	}
}
