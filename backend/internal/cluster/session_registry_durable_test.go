package cluster

import (
	"context"
	"testing"
	"time"

	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestSessionActivationWaitsForDurableSeedWithoutPoisoningIdentity(t *testing.T) {
	_, registry := registryFixture(t, 1)
	ctx := context.Background()
	id := uuid.NewString()
	require.NoError(t, registry.execute(ctx, lifecycleRequest(globalRef(), id, &pb.SystemCommand_CreateSession{CreateSession: &pb.CreateSession{Airport: "EKCH", Name: "LIVE", WorkflowId: id}})))
	entry := registryState(t, registry).Entities["1"].GetValue().GetSessionRegistry()
	observed := NewAggregate(sessionRef(1))
	writer := registry.Store.(LocalLifecycleStore).Writer
	writer.Plan = SessionLifecyclePlanner(func(context.Context, *pb.AggregateRef) (*Aggregate, error) { return observed, nil })
	request := lifecycleRequest(globalRef(), lifecycleID(entry.WorkflowId, "active"), &pb.SystemCommand_CreateSession{CreateSession: &pb.CreateSession{Id: 1, Airport: "EKCH", Name: "LIVE", WorkflowId: entry.WorkflowId}})
	reply := writer.Execute(ctx, request)
	require.Equal(t, pb.CommandReply_UNAVAILABLE, reply.Status)
	require.Nil(t, registryState(t, registry).Ledger[request.CommandId], "an unseen seed must not permanently fail the deterministic activation ID")
	require.NoError(t, registry.execute(ctx, lifecycleRequest(sessionRef(1), lifecycleID(entry.WorkflowId, "seed"), &pb.SystemCommand_CreateSession{CreateSession: &pb.CreateSession{Id: 1, Airport: "EKCH", Name: "LIVE", WorkflowId: entry.WorkflowId}})))
	var err error
	observed, err = registry.readDurable(ctx, sessionRef(1))
	require.NoError(t, err)
	reply = writer.Execute(ctx, request)
	require.Equal(t, pb.CommandOutcome_SUCCEEDED, reply.GetOutcome().GetStatus())
	require.Equal(t, pb.SessionRegistry_ACTIVE, registryState(t, registry).Entities["1"].GetValue().GetSessionRegistry().State)
}

func TestSessionDeletionWaitsForReplayedTombstoneWithoutPoisoningIdentity(t *testing.T) {
	_, registry := registryFixture(t, 1)
	ctx := context.Background()
	_, err := registry.GetOrCreateSession(ctx, "EKCH", "LIVE")
	require.NoError(t, err)
	entry := registryState(t, registry).Entities["1"].GetValue().GetSessionRegistry()
	observed, err := registry.readDurable(ctx, sessionRef(1))
	require.NoError(t, err)
	require.NoError(t, registry.TombstoneSession(ctx, 1))
	writer := registry.Store.(LocalLifecycleStore).Writer
	writer.Plan = SessionLifecyclePlanner(func(context.Context, *pb.AggregateRef) (*Aggregate, error) { return observed, nil })
	request := lifecycleRequest(globalRef(), lifecycleID(entry.WorkflowId, "deleting"), &pb.SystemCommand_DeleteSession{DeleteSession: &pb.DeleteSession{Id: 1, WorkflowId: entry.WorkflowId}})
	reply := writer.Execute(ctx, request)
	require.Equal(t, pb.CommandReply_UNAVAILABLE, reply.Status)
	require.Nil(t, registryState(t, registry).Ledger[request.CommandId])
	observed, err = registry.readDurable(ctx, sessionRef(1))
	require.NoError(t, err)
	reply = writer.Execute(ctx, request)
	require.Equal(t, pb.CommandOutcome_SUCCEEDED, reply.GetOutcome().GetStatus())
	require.Equal(t, pb.SessionRegistry_DELETING, registryState(t, registry).Entities["1"].GetValue().GetSessionRegistry().State)
}

func TestSessionRegistrySeedCannotFinishOnRAMAcceptance(t *testing.T) {
	owners, projection, ref, gate, store := asyncOwnersFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	writer := Writer{Store: store, Projection: projection, Lease: owners.owner, NodeID: owners.owner.NodeID,
		Plan: SessionLifecyclePlanner(func(context.Context, *pb.AggregateRef) (*Aggregate, error) { return nil, nil })}
	registry := SessionRegistry{Store: LocalLifecycleStore{Writer: writer}}
	id := uuid.NewString()
	request := lifecycleRequest(ref, id, &pb.SystemCommand_CreateSession{CreateSession: &pb.CreateSession{Id: 42, Airport: "EKCH", Name: "LIVE", WorkflowId: uuid.NewString()}})
	done := make(chan error, 1)
	go func() { done <- registry.execute(ctx, request) }()
	<-store.started
	select {
	case err := <-done:
		t.Fatalf("seed returned before broker persistence: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(gate)
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-ctx.Done():
		t.Fatal("seed did not finish after broker persistence resumed")
	}
	durable, err := projection.readOwnedDurable(ref, owners.owner.NodeID)
	require.NoError(t, err)
	require.NotNil(t, durable.Indexes[pb.EntityKind_SESSION]["42"])
	require.NotZero(t, durable.Ledger[id].CommittedStreamSequence)
	require.NoError(t, owners.Drain(ctx))
}

func TestSessionActivationStillRejectsWrongOrTombstonedIdentity(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(*pb.Session)
	}{
		{"different airport", func(s *pb.Session) { s.Airport = "ESSA" }},
		{"different name", func(s *pb.Session) { s.Name = "OTHER" }},
		{"tombstoned", func(s *pb.Session) { s.Tombstoned = true }},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, registry := registryFixture(t, 1)
			ctx := context.Background()
			id := uuid.NewString()
			require.NoError(t, registry.execute(ctx, lifecycleRequest(globalRef(), id, &pb.SystemCommand_CreateSession{CreateSession: &pb.CreateSession{Airport: "EKCH", Name: "LIVE", WorkflowId: id}})))
			entry := registryState(t, registry).Entities["1"].GetValue().GetSessionRegistry()
			session := &pb.Session{Id: 1, Airport: "EKCH", Name: "LIVE"}
			test.edit(session)
			observed := NewAggregate(sessionRef(1))
			observed.Entities["1"] = &pb.EntitySnapshot{Key: "1", Revision: 1, Value: &pb.EntityRecord{Value: &pb.EntityRecord_Session{Session: session}}}
			writer := registry.Store.(LocalLifecycleStore).Writer
			writer.Plan = SessionLifecyclePlanner(func(context.Context, *pb.AggregateRef) (*Aggregate, error) { return observed, nil })
			request := lifecycleRequest(globalRef(), lifecycleID(entry.WorkflowId, "active"), &pb.SystemCommand_CreateSession{CreateSession: &pb.CreateSession{Id: 1, Airport: "EKCH", Name: "LIVE", WorkflowId: entry.WorkflowId}})
			reply := writer.Execute(ctx, request)
			require.Equal(t, pb.CommandOutcome_FAILED, reply.GetOutcome().GetStatus())
			require.Equal(t, "INVALID_ARGUMENT", reply.GetOutcome().GetReasonCode())
			require.Equal(t, pb.SessionRegistry_INITIALIZING, registryState(t, registry).Entities["1"].GetValue().GetSessionRegistry().State)
		})
	}
}
