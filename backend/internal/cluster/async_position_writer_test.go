package cluster

import (
	pb "FlightStrips/pkg/events/cluster"
	"context"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	"testing"
	"time"
)

func TestPositionDeadlineTranslationPreservesOtherSourceRevisions(t *testing.T) {
	token := localPositionBit | 17
	deadline := func(kind string, revision uint64) *pb.EntityChange {
		return &pb.EntityChange{Operation: &pb.EntityChange_Upsert{Upsert: &pb.EntityRecord{Value: &pb.EntityRecord_SessionDeadline{SessionDeadline: &pb.SessionDeadline{Kind: kind, SourceRevision: revision}}}}}
	}

	original := &pb.StateEvent{Fact: &pb.StateEvent_DomainChanged{DomainChanged: &pb.DomainChange{Changes: []*pb.EntityChange{deadline("aircraft-disconnect", token), deadline("controller-offline", token), deadline("aircraft-disconnect", 3)}}}}
	p := &Projection{asyncPositions: &asyncPositionState{tokens: map[uint64]uint64{token: 91}}}
	detached := proto.Clone(original).(*pb.StateEvent)
	require.NoError(t, p.TranslatePositionSources(detached))
	changes := detached.GetDomainChanged().Changes
	require.Equal(t, uint64(91), changes[0].GetUpsert().GetSessionDeadline().SourceRevision)
	require.Equal(t, token, changes[1].GetUpsert().GetSessionDeadline().SourceRevision)
	require.Equal(t, uint64(3), changes[2].GetUpsert().GetSessionDeadline().SourceRevision)
	require.Equal(t, token, original.GetDomainChanged().Changes[0].GetUpsert().GetSessionDeadline().SourceRevision)
	delete(p.asyncPositions.tokens, token)
	require.ErrorContains(t, p.TranslatePositionSources(proto.Clone(original).(*pb.StateEvent)), "not been synchronized")
}

func TestPendingPositionOverlayDoesNotAdvanceRawCursor(t *testing.T) {
	p := readyPositionWaitFixture()
	value := localPositionValue()
	token := localPositionBit | 29
	p.positions["1.SAS1.2"] = KVPosition{Value: value, Revision: 7}
	newer := proto.Clone(value).(*pb.PositionValue)
	newer.GetPosition().Latitude = 56
	p.asyncPositions = &asyncPositionState{values: map[string]KVPosition{"1.SAS1.2": {Value: newer, Revision: token}}, durable: map[string]uint64{"1.SAS1.2": 7}, tokens: map[uint64]uint64{}, pending: map[uint64]*pb.PositionValue{token: newer}}
	require.Equal(t, token, p.positionViewLocked()["1.SAS1.2"].Revision)
	require.Equal(t, uint64(7), p.positions["1.SAS1.2"].Revision)
	require.Zero(t, p.positionCursor.appliedStream)
	require.NoError(t, p.checkAsyncPositionReplayLocked("1.SAS1.2", newer, 8), "queued accepted publication is valid before callback records its ack")
	require.NoError(t, p.checkAsyncPositionReplayLocked("1.SAS1.2", value, 7), "older baseline replay is valid")
	require.Error(t, p.checkAsyncPositionReplayLocked("1.SAS1.2", nil, 9), "foreign delete cannot be hidden by pending overlay")
	require.Error(t, p.observationErr)
}

type asyncPositionKVGate struct {
	*positionKVTest
	started chan struct{}
	release chan struct{}
}

func (kv *asyncPositionKVGate) Create(key string, data []byte) (uint64, error) {
	close(kv.started)
	<-kv.release
	return kv.positionKVTest.Create(key, data)
}

func TestAsyncPositionsAcknowledgeRAMBeforeFIFOReplicationAndReenterDerivedTurn(t *testing.T) {
	owners, p, ref, stateGate, _ := asyncOwnersFixture(t)
	close(stateGate)
	p.positions = map[string]KVPosition{}
	p.positionCursor.lastProved = time.Now()
	p.asyncPositions = &asyncPositionState{values: map[string]KVPosition{}, durable: map[string]uint64{}, tokens: map[uint64]uint64{}, pending: map[uint64]*pb.PositionValue{}, baselines: map[string]bool{}}
	kv := &asyncPositionKVGate{positionKVTest: &positionKVTest{values: map[string]positionKVEntry{}}, started: make(chan struct{}), release: make(chan struct{})}
	w, err := NewPositionWriter(kv, 42, 1, "master", func(context.Context, int32, uint64, string) error { return nil }, 1, 8)
	require.NoError(t, err)
	defer w.Close(context.Background())
	w.async, w.projection, w.baselineReady = owners, p, true
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var first, second uint64
	require.NoError(t, owners.Execute(ctx, ref, func(turn context.Context) error {
		result, err := w.QueuePosition(turn, "SAS1", localPositionValue().GetPosition(), time.Now())
		if err != nil {
			return err
		}
		receipt := <-result
		first = receipt.Revision
		if receipt.Err != nil {
			return receipt.Err
		}
		_, err = w.ExecuteDerivedContext(turn, "SAS1", first, func(derived context.Context) (*pb.CommandReply, error) {
			err := owners.Execute(derived, ref, func(context.Context) error { return nil })
			return nil, err
		})
		return err
	}))
	<-kv.started
	require.NoError(t, owners.Execute(ctx, ref, func(turn context.Context) error {
		result, err := w.QueueDisconnect(turn, "SAS1", time.Now())
		if err != nil {
			return err
		}
		receipt := <-result
		second = receipt.Revision
		return receipt.Err
	}))
	require.NotEqual(t, first, second)
	require.True(t, first&localPositionBit != 0)
	p.mu.RLock()
	require.Equal(t, second, p.asyncPositions.values["42.SAS1.1"].Revision)
	require.Empty(t, p.positions)
	require.Zero(t, p.positionCursor.appliedStream)
	p.mu.RUnlock()
	close(kv.release)
	require.NoError(t, owners.Drain(ctx))
	p.mu.RLock()
	require.Equal(t, uint64(1), p.asyncPositions.tokens[first])
	require.Equal(t, uint64(2), p.asyncPositions.tokens[second])
	p.mu.RUnlock()
	entry, err := kv.Get("42.SAS1.1")
	require.NoError(t, err)
	value := &pb.PositionValue{}
	require.NoError(t, pb.UnmarshalStrict(entry.Value(), value))
	require.NotNil(t, value.GetTombstone())
}

func TestAsyncPositionAdmissionRejectsStaleBackgroundIntegrityProof(t *testing.T) {
	p := readyPositionWaitFixture()
	require.ErrorContains(t, p.asyncPositionHealthLocked(), "proof is stale")
	p.positionCursor.lastProved = time.Now()
	require.NoError(t, p.asyncPositionHealthLocked())
	p.positionCursor.lastProved = time.Now().Add(-3 * time.Second)
	require.ErrorContains(t, p.asyncPositionHealthLocked(), "proof is stale")
}

func TestPositionSourceAliasesPreservePendingAndDurableDeadlineGuards(t *testing.T) {
	token, other := localPositionBit|71, localPositionBit|72
	p := &Projection{asyncPositions: &asyncPositionState{tokens: map[uint64]uint64{token: 9}}}
	require.True(t, p.PositionSourceMatches(token, 9))
	require.True(t, p.PositionSourceMatches(9, token))
	require.True(t, p.PositionSourceMatches(other, other))
	require.False(t, p.PositionSourceMatches(token, other))
	require.False(t, p.PositionSourceMatches(other, 0))
	require.False(t, p.PositionSourceMatches(token, 8))
}

func TestAsyncDisconnectSourceAliasExpiresAndRejectsNewerObservation(t *testing.T) {
	_, _, _, p, now, _ := workerFixture(t)
	setWorkerPresence(p, *now, now.Add(-time.Hour), pb.ClientPresence_EUROSCOPE)
	state := NewAggregate(sessionRef(1))
	state.Owner = &pb.OwnerTerm{NodeId: "node-a", Epoch: 1}
	state.Master = &pb.MasterTerm{ConnectionId: "client-a", Cid: "cid-a", Epoch: 1, OwnerEpoch: 1}
	state.Sync = &pb.SessionSync{ConnectionId: "client-a", MasterEpoch: 1, CompletedAt: timestamppb.New(*now)}
	seed := &pb.EntitySnapshot{Key: "1", Revision: 1, Value: &pb.EntityRecord{Value: &pb.EntityRecord_Session{Session: &pb.Session{Id: 1, Airport: "EKCH", Name: "LIVE"}}}}
	strip := &pb.EntitySnapshot{Key: "SAS101", Revision: 2, Value: &pb.EntityRecord{Value: &pb.EntityRecord_Strip{Strip: &pb.Strip{Id: 1, Callsign: "SAS101", Revision: 2}}}}
	deadline := &pb.EntitySnapshot{Key: "aircraft-disconnect.SAS101", Revision: 1, Value: &pb.EntityRecord{Value: &pb.EntityRecord_SessionDeadline{SessionDeadline: &pb.SessionDeadline{Id: "aircraft-disconnect.SAS101", Kind: "aircraft-disconnect", Callsign: "SAS101", DueAt: timestamppb.New(now.Add(-4 * time.Minute)), SourceRevision: 8}}}}
	for _, e := range []*pb.EntitySnapshot{seed, strip, deadline} {
		state.Entities[e.Key] = e
	}
	state.Indexes[pb.EntityKind_SESSION] = map[string]*pb.EntitySnapshot{"1": seed}
	state.Indexes[pb.EntityKind_STRIP] = map[string]*pb.EntitySnapshot{"SAS101": strip}
	state.Indexes[pb.EntityKind_SESSION_DEADLINE] = map[string]*pb.EntitySnapshot{deadline.Key: deadline}
	p.states = map[string]*Aggregate{"fs.v1.state.session.1": state}
	token := localPositionBit | 99
	value := &pb.PositionValue{SchemaVersion: 1, SessionId: 1, AircraftKey: "SAS101", OwnerEpoch: 1, SourceConnectionId: "client-a", Observation: &pb.PositionValue_Tombstone{Tombstone: &pb.PositionTombstone{}}}
	p.asyncPositions = &asyncPositionState{values: map[string]KVPosition{"1.SAS101.1": {Value: value, Revision: token}}, tokens: map[uint64]uint64{token: 8}}
	value.ObservedAt = timestamppb.New(now.Add(-5 * time.Minute))
	strip.Value.GetStrip().EuroscopeObservedAt = timestamppb.New(now.Add(-6 * time.Minute))
	recovery := EuroScopeDeadlinePlanner{Projection: p, Now: func() time.Time { return *now }}
	recovered, err := recovery.Recover(state)
	require.NoError(t, err)
	for _, change := range recovered {
		require.NotEqual(t, deadline.Key, change.Key, "acknowledged source alias must not reset disconnect grace clock")
	}
	planner := SessionWorkerPlanner{Next: PlanSystemEntity, Projection: p, Now: func() time.Time { return *now }, RetainedAircraft: func(*Aggregate, string) (bool, error) { return false, nil }}
	req := sessionWorkerRequest(1, uuid.NewString(), 1, &pb.SystemCommand{Action: &pb.SystemCommand_RemoveEntity{RemoveEntity: &pb.RemoveEntity{Key: deadline.Key, Kind: pb.EntityKind_SESSION_DEADLINE}}})
	change, status, _, err := planner.Plan(context.Background(), req, state)
	require.NoError(t, err)
	require.Equal(t, pb.CommandReply_COMMITTED, status)
	require.Len(t, change.Changes, 2, "durable alias must expire the RAM tombstone and deadline together")
	newer := proto.Clone(value).(*pb.PositionValue)
	newer.Observation = &pb.PositionValue_Position{Position: &pb.AircraftPosition{Latitude: 55, Longitude: 12}}
	p.asyncPositions.values["1.SAS101.1"] = KVPosition{Value: newer, Revision: token + 1}
	change, status, _, err = planner.Plan(context.Background(), req, state)
	require.NoError(t, err)
	require.Equal(t, pb.CommandReply_COMMITTED, status)
	require.Len(t, change.Changes, 1, "newer live observation cancels only deadline")
}
