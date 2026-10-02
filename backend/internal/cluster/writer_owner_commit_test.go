package cluster

import (
	"context"
	"testing"
	"time"

	pb "FlightStrips/pkg/events/cluster"
	"github.com/stretchr/testify/require"
)

type ownerCommitReceiptStore struct {
	*memoryStore
	projection *Projection
	alter      func(*AppliedEvent)
	reads      int
}

func (s *ownerCommitReceiptStore) Publish(ctx context.Context, subject string, expected uint64, data []byte) (uint64, error) {
	sequence, err := s.memoryStore.Publish(ctx, subject, expected, data)
	if err == nil {
		s.projection.mu.Lock()
		s.projection.highWater = sequence
		s.projection.mu.Unlock()
	}
	return sequence, err
}

func (s *ownerCommitReceiptStore) Committed(_ context.Context, sequence uint64) (AppliedEvent, error) {
	s.reads++
	s.mu.Lock()
	entry := s.entries[sequence-1]
	entry.Data = append([]byte(nil), entry.Data...)
	s.mu.Unlock()
	if s.alter != nil {
		s.alter(&entry)
	}
	return entry, nil
}

func TestWriterOwnerReceiptReturnsWithoutOrderedReplay(t *testing.T) {
	store, writer, ref, p := ownerCommitProjectionFixture(t)
	receipts := &ownerCommitReceiptStore{memoryStore: store, projection: p}
	writer.Store, writer.Projection = receipts, p
	request := command(ref, "owner RAM", 0)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	reply, fresh := writer.ExecuteFresh(ctx, request)
	require.True(t, fresh)
	require.Equal(t, pb.CommandReply_COMMITTED, reply.Status)
	require.Equal(t, uint64(1), p.applied, "reply must succeed while raw replay remains paused")
	require.Equal(t, uint64(2), p.highWater)
	require.Equal(t, 1, receipts.reads)
	require.NoError(t, p.apply(store.entries[1]))
	// After replay catches up, retry reads the same durable outcome and never
	// publishes the command a second time.
	again, fresh := writer.ExecuteFresh(ctx, request)
	require.False(t, fresh)
	require.Equal(t, reply.GetStreamSequence(), again.GetStreamSequence())
	require.Equal(t, 2, store.commits)
}

func TestWriterOwnerReceiptMustProveIdentityAndReducerEffectiveness(t *testing.T) {
	for _, test := range []struct {
		name  string
		alter func(*AppliedEvent, *Projection)
	}{
		{"expired timestamp", func(e *AppliedEvent, p *Projection) {
			e.ServerTime = p.states[e.Subject].Owner.LeaseUntil.AsTime().Add(time.Nanosecond)
		}},
		{"wrong subject", func(e *AppliedEvent, _ *Projection) { e.Subject = "fs.v1.state.airport.EKCH" }},
		{"wrong sequence", func(e *AppliedEvent, _ *Projection) { e.StreamSequence++ }},
		{"wrong payload", func(e *AppliedEvent, _ *Projection) { e.Data = []byte("different") }},
	} {
		t.Run(test.name, func(t *testing.T) {
			store, writer, ref, p := ownerCommitProjectionFixture(t)
			receipts := &ownerCommitReceiptStore{memoryStore: store, projection: p,
				alter: func(e *AppliedEvent) { test.alter(e, p) }}
			writer.Store, writer.Projection = receipts, p
			request := command(ref, "unproved", 0)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
			defer cancel()
			reply := writer.Execute(ctx, request)
			require.NotEqual(t, pb.CommandReply_COMMITTED, reply.Status)
			require.NotEqual(t, pb.CommandReply_PENDING, reply.Status)
			require.Nil(t, p.states["fs.v1.state.global"].Ledger[request.CommandId])
			require.Equal(t, uint64(1), p.applied)
			if test.name != "expired timestamp" {
				require.Equal(t, pb.CommandReply_UNAVAILABLE, reply.Status, "identity failure must fall back to blocked raw replay")
				require.NoError(t, p.apply(store.entries[1]), "authoritative replay recovers a receipt-read failure")
				require.NotNil(t, p.states["fs.v1.state.global"].Ledger[request.CommandId])
			}
		})
	}
}
