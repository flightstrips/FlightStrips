package cluster

import (
	"context"
	"testing"
	"time"

	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Effect transitions retain their replay barrier while the ordinary async
// store materializes domain receipts locally. Model the independent watcher.
type replayingEffectStore struct {
	*asyncReceiptStore
	projection *Projection
}

func (s replayingEffectStore) Publish(ctx context.Context, subject string, expected uint64, data []byte) (uint64, error) {
	sequence, err := s.asyncReceiptStore.Publish(ctx, subject, expected, data)
	if err != nil {
		return 0, err
	}
	event := &pb.StateEvent{}
	if err = proto.Unmarshal(data, event); err != nil {
		return 0, err
	}
	if event.GetEffectChanged() != nil {
		entry, err := s.Committed(ctx, sequence)
		if err != nil {
			return 0, err
		}
		if err = s.projection.apply(entry); err != nil {
			return 0, err
		}
	}
	return sequence, nil
}

func seedAsyncEffect(t *testing.T, p *Projection, ref *pb.AggregateRef, claimed bool) *pb.EffectRecord {
	t.Helper()
	id, connection := uuid.NewString(), uuid.NewString()
	effect := &pb.EffectRecord{CommandId: id, TargetCid: "111111", OwnerEpoch: 1, Status: pb.EffectRecord_WAITING,
		DispatchDeadline: timestamppb.New(time.Now().Add(time.Minute)), Payload: &pb.EffectRecord_Pdc{Pdc: &pb.PdcEffect{Callsign: "SAS123", Action: "ISSUE"}}}
	if claimed {
		effect.Status = pb.EffectRecord_DISPATCH_CLAIMED
		effect.DispatchConnectionId = &connection
		effect.ResultDeadline = timestamppb.New(time.Now().Add(time.Minute))
	}
	key, _ := Subject(ref)
	p.mu.Lock()
	p.states[key].Effects[id] = effect
	p.states[key].Ledger[id] = &pb.CommandOutcome{CommandId: id, Status: pb.CommandOutcome_ACCEPTED}
	p.presence = map[string]KVPresence{}
	p.mu.Unlock()
	putTestPresence(p, &pb.ClientPresence{SessionId: ref.GetSession().Id, ConnectionId: connection, NodeId: "node-a", Cid: effect.TargetCid, Kind: pb.ClientPresence_EUROSCOPE, ConnectedAt: timestamppb.Now()})
	return effect
}

func TestEffectDurableTransitionSerializesConcurrentRAMAdmission(t *testing.T) {
	m, p, ref, gate, store := asyncOwnersFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	effect := seedAsyncEffect(t, p, ref, false)
	m.owner.Store = replayingEffectStore{store, p}
	var target *pb.ClientPresence
	for _, entry := range p.presence {
		if client := entry.Value.GetClient(); client != nil {
			target = client
		}
	}
	var sends int
	fanout := &SessionFanout{NC: m.owner.NC, Projection: p, NodeID: "node-a", sockets: map[string]*socketEntry{target.ConnectionId: {
		presence: target, delivered: map[string]bool{}, socket: LocalSessionSocket{OnEffect: func(*pb.EffectRecord) error { sends++; return nil }},
	}}}
	effects := Effects{Owner: m.owner, Fanout: fanout}
	done := make(chan error, 1)
	go func() {
		done <- effects.claimAndDispatch(ctx, ref, effect, target.ConnectionId)
	}()
	<-store.started
	admitted := make(chan struct{})
	accepted := make(chan error, 1)
	go func() {
		accepted <- m.Execute(ctx, ref, func(turn context.Context) error {
			close(admitted)
			base, err := m.Read(ref)
			if err != nil {
				return err
			}
			_, err = m.AcceptState(turn, base, asyncDomainEvent(ref, base.Revision+1))
			return err
		})
	}()
	select {
	case <-admitted:
		t.Fatal("RAM admission overtook blocked durable effect transition")
	case <-time.After(20 * time.Millisecond):
	}
	close(gate)
	require.NoError(t, <-done)
	require.Equal(t, 1, sends, "durable claim must reach the socket before the next RAM turn")
	require.NoError(t, <-accepted)
	require.NoError(t, m.Drain(ctx))
	require.NoError(t, m.Err())
	view, err := m.Read(ref)
	require.NoError(t, err)
	require.Equal(t, uint64(2), view.Revision)
	require.Equal(t, pb.EffectRecord_DISPATCH_CLAIMED, view.Effects[effect.CommandId].Status)
}

func TestForwardedEffectResultWaitsForPendingRAMBeforeDurableCommit(t *testing.T) {
	m, p, ref, gate, store := asyncOwnersFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	effect := seedAsyncEffect(t, p, ref, true)
	m.owner.Store = replayingEffectStore{store, p}
	require.NoError(t, m.Execute(ctx, ref, func(turn context.Context) error {
		base, err := m.Read(ref)
		if err != nil {
			return err
		}
		_, err = m.AcceptState(turn, base, asyncDomainEvent(ref, base.Revision+1))
		return err
	}))
	<-store.started
	terminal := proto.Clone(effect).(*pb.EffectRecord)
	terminal.Status = pb.EffectRecord_EXECUTED
	request := &pb.EffectDeliveryRequest{SessionId: ref.GetSession().Id, ConnectionId: *effect.DispatchConnectionId, Effect: terminal, ClaimStreamSequence: 1}
	effects := Effects{Owner: m.owner}
	done := make(chan error, 1)
	go func() { done <- effects.recordForwardedResult(ctx, request) }()
	select {
	case err := <-done:
		t.Fatalf("result skipped pending durability barrier: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(gate)
	require.NoError(t, <-done)
	require.NoError(t, effects.recordForwardedResult(ctx, request), "result retry must acknowledge its existing durable outcome")
	require.Len(t, store.entries, 3, "duplicate plugin result must not publish twice")
	require.NoError(t, m.Drain(ctx))
	require.NoError(t, m.Err())
}
