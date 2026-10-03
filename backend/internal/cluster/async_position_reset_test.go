package cluster

import (
	pb "FlightStrips/pkg/events/cluster"
	"context"
	"errors"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func resetPositionFixture() (*Projection, *AsyncSessionOwners, *asyncPositionState) {
	m := NewAsyncSessionOwners(nil, nil, nil)
	state := &asyncPositionState{values: map[string]KVPosition{"1.SAS1.2": {Value: localPositionValue(), Revision: localPositionBit | 1}}, tokens: map[uint64]uint64{localPositionBit | 1: 7}, durable: map[string]uint64{"1.SAS1.2": 7}, pending: map[uint64]*pb.PositionValue{}, baselines: map[string]bool{"1.2": true}}
	p := readyPositionWaitFixture()
	p.Async = m
	p.asyncPositions = state
	return p, m, state
}
func TestIdlePositionConsumerResetRecoversThroughCompleteReplay(t *testing.T) {
	p, m, manager := resetPositionFixture()
	p.mu.Lock()
	p.positionReady = false
	p.resetAsyncPositionOverlayLocked()
	p.mu.Unlock()
	require.NoError(t, m.Err())
	require.Same(t, manager, p.asyncPositions, "bound writers retain the manager")
	require.Empty(t, manager.values)
	require.Empty(t, manager.tokens)
	require.Empty(t, manager.baselines)
	require.Empty(t, manager.durable)
	require.ErrorContains(t, p.Ready(), "replay incomplete")
	p.mu.Lock()
	require.NoError(t, p.materializePositionLocked("1.SAS1.2", localPositionValue(), 8, time.Now()))
	p.mu.Unlock()
	require.Equal(t, uint64(8), manager.durable["1.SAS1.2"], "full replay seeds the next CAS precondition")
	require.False(t, p.positionReady, "a single raw record cannot reopen admission")
}
func TestPositionResetRejectsActiveTurnOrPendingTail(t *testing.T) {
	for _, name := range []string{"turn", "job", "tail", "failure"} {
		t.Run(name, func(t *testing.T) {
			p, m, _ := resetPositionFixture()
			m.sessions["fs.v1.state.session.1"] = &asyncSession{}
			switch name {
			case "turn":
				m.turns = 1
			case "job":
				m.sessions["fs.v1.state.session.1"].pending = 1
			case "tail":
				m.sessions["fs.v1.state.session.1"].tail = []*pb.StateEvent{{}}
			case "failure":
				m.failure = errors.New("existing failure")
			}
			p.mu.Lock()
			p.positionReady = false
			p.resetAsyncPositionOverlayLocked()
			p.mu.Unlock()
			require.Error(t, m.Err())
			require.Error(t, p.observationErr)
			require.False(t, p.positionReady)
		})
	}
}
func TestPositionAliasesPruneOnlyAfterEveryLiveReferenceDisappears(t *testing.T) {
	p, m, manager := resetPositionFixture()
	state := NewAggregate(sessionRef(1))
	state.Owner = &pb.OwnerTerm{Epoch: 2}
	p.states = map[string]*Aggregate{"fs.v1.state.session.1": state}
	tokens := []uint64{localPositionBit | 1, localPositionBit | 2, localPositionBit | 3, localPositionBit | 4, localPositionBit | 5}
	for _, token := range tokens {
		manager.tokens[token] = token &^ localPositionBit
	}
	manager.pending[tokens[1]] = localPositionValue()
	ram := NewAggregate(sessionRef(1))
	ram.Indexes[pb.EntityKind_SESSION_DEADLINE] = map[string]*pb.EntitySnapshot{"d": {Value: &pb.EntityRecord{Value: &pb.EntityRecord_SessionDeadline{SessionDeadline: &pb.SessionDeadline{Kind: "aircraft-disconnect", SourceRevision: tokens[2]}}}}}
	tail := &pb.StateEvent{Fact: &pb.StateEvent_DomainChanged{DomainChanged: &pb.DomainChange{Changes: []*pb.EntityChange{{Operation: &pb.EntityChange_Upsert{Upsert: &pb.EntityRecord{Value: &pb.EntityRecord_SessionDeadline{SessionDeadline: &pb.SessionDeadline{Kind: "aircraft-disconnect", SourceRevision: tokens[3]}}}}}}}}}
	m.sessions["fs.v1.state.session.1"] = &asyncSession{ram: ram, tail: []*pb.StateEvent{tail}}
	manager.values["1.OLD.1"] = KVPosition{Value: &pb.PositionValue{SessionId: 1, OwnerEpoch: 1}, Revision: tokens[4]}
	manager.baselines["1.1"] = true
	manager.durable["1.OLD.1"] = 99
	p.mu.Lock()
	p.pruneAsyncPositionAliasesLocked()
	p.mu.Unlock()
	for _, token := range tokens[:4] {
		require.Contains(t, manager.tokens, token)
	}
	require.NotContains(t, manager.tokens, tokens[4])
	require.NotContains(t, manager.values, "1.OLD.1")
	require.NotContains(t, manager.baselines, "1.1")
	require.NotContains(t, manager.durable, "1.OLD.1")
	delete(manager.pending, tokens[1])
	ram.Indexes[pb.EntityKind_SESSION_DEADLINE] = map[string]*pb.EntitySnapshot{}
	m.sessions["fs.v1.state.session.1"].tail = nil
	p.mu.Lock()
	p.pruneAsyncPositionAliasesLocked()
	p.mu.Unlock()
	require.Len(t, manager.tokens, 1, "only the current overlay source remains")
	require.Contains(t, manager.tokens, tokens[0])
}

func TestResetReadinessRejectsNewOwnerTurn(t *testing.T) {
	m, p, ref, gate, _ := asyncOwnersFixture(t)
	close(gate)
	require.NoError(t, m.Execute(context.Background(), ref, func(context.Context) error { return nil }))
	p.mu.Lock()
	p.positionReady = false
	require.True(t, m.IdleForPositionReset())
	p.mu.Unlock()
	called := false
	require.Error(t, m.Execute(context.Background(), ref, func(context.Context) error { called = true; return nil }))
	require.False(t, called)
}

func TestPositionAliasPruningPreservesPendingOldEpochCAS(t *testing.T) {
	p, _, manager := resetPositionFixture()
	state := NewAggregate(sessionRef(1))
	state.Owner = &pb.OwnerTerm{Epoch: 3}
	p.states = map[string]*Aggregate{"fs.v1.state.session.1": state}
	token := localPositionBit | 2
	manager.pending[token] = &pb.PositionValue{SessionId: 1, AircraftKey: "SAS1", OwnerEpoch: 2}
	manager.tokens[token] = 7
	p.mu.Lock()
	p.pruneAsyncPositionAliasesLocked()
	p.mu.Unlock()
	require.Contains(t, manager.tokens, token)
	require.Contains(t, manager.durable, "1.SAS1.2")
	require.Contains(t, manager.baselines, "1.2")
	delete(manager.pending, token)
	p.mu.Lock()
	p.pruneAsyncPositionAliasesLocked()
	p.mu.Unlock()
	require.NotContains(t, manager.tokens, token)
	require.NotContains(t, manager.durable, "1.SAS1.2")
	require.NotContains(t, manager.baselines, "1.2")
}
