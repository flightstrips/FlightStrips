package cluster

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"
	"time"

	pb "FlightStrips/pkg/events/cluster"
	euroscope "FlightStrips/pkg/events/euroscope"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type effectObjects struct {
	values map[string][]byte
	times  map[string]time.Time
}

func (m *effectObjects) GetBytes(name string, _ ...nats.GetObjectOpt) ([]byte, error) {
	if value, ok := m.values[name]; ok {
		return append([]byte(nil), value...), nil
	}
	return nil, nats.ErrObjectNotFound
}
func (m *effectObjects) PutBytes(name string, value []byte, _ ...nats.ObjectOpt) (*nats.ObjectInfo, error) {
	if m.values == nil {
		m.values, m.times = map[string][]byte{}, map[string]time.Time{}
	}
	m.values[name], m.times[name] = append([]byte(nil), value...), time.Now()
	return &nats.ObjectInfo{ObjectMeta: nats.ObjectMeta{Name: name}, ModTime: m.times[name]}, nil
}
func (m *effectObjects) List(_ ...nats.ListObjectsOpt) ([]*nats.ObjectInfo, error) {
	out := []*nats.ObjectInfo{}
	for name := range m.values {
		out = append(out, &nats.ObjectInfo{ObjectMeta: nats.ObjectMeta{Name: name}, ModTime: m.times[name]})
	}
	return out, nil
}
func (m *effectObjects) Delete(name string) error { delete(m.values, name); return nil }

func TestPrivateMessageCiphertextAndCIDBinding(t *testing.T) {
	objects := &effectObjects{}
	secrets := EffectSecrets{Objects: objects, ActiveKeyID: "v1", Keys: map[string][]byte{"v1": bytesOf(32, 7)}}
	id := uuid.NewString()
	ref, err := secrets.StagePrivateMessage(id, "controller-1", "SAS123", "Proceed to stand A12")
	if err != nil {
		t.Fatal(err)
	}
	data := objects.values[ref.ObjectName]
	if strings.Contains(string(data), "Proceed to stand") || ref.ObjectName != "effect/"+id || len(ref.Sha256) != 64 {
		t.Fatal("private message leaked into object reference or ciphertext")
	}
	if _, err := secrets.StagePrivateMessage(id, "controller-1", "SAS123", "Proceed to stand A12"); err != nil {
		t.Fatalf("same logical request must reuse staged secret: %v", err)
	}
	if body, err := secrets.OpenPrivateMessage(id, "controller-1", ref.ObjectName, ref.Sha256); err != nil || body != "Proceed to stand A12" {
		t.Fatalf("private message readback: %q %v", body, err)
	}
	for _, cid := range []string{"controller-2", "SAS123"} {
		if _, err := secrets.OpenPrivateMessage(id, cid, ref.ObjectName, ref.Sha256); err == nil {
			t.Fatal("wrong target CID decrypted ciphertext")
		}
	}
	objects.values[ref.ObjectName][len(data)-1] ^= 1
	if _, err := secrets.OpenPrivateMessage(id, "controller-1", ref.ObjectName, ref.Sha256); err == nil {
		t.Fatal("modified ciphertext passed digest check")
	}
}

func TestPrivateMessagePlannerKeepsBodyOutOfStateEvent(t *testing.T) {
	objects := &effectObjects{}
	secrets := EffectSecrets{Objects: objects, ActiveKeyID: "v1", Keys: map[string][]byte{"v1": bytesOf(32, 7)}}
	ref := sessionRef(74)
	state := NewAggregate(ref)
	state.Owner = &pb.OwnerTerm{NodeId: "node-a", Epoch: 2}
	state.Indexes[pb.EntityKind_CONTROLLER] = map[string]*pb.EntitySnapshot{"controller-1": {}}
	id := uuid.NewString()
	request := &pb.CommandRequest{ProtocolRevision: 1, CommandId: id, Aggregate: ref,
		Actor: &pb.Actor{Kind: pb.Actor_CONTROLLER, Id: "controller-1", SessionId: proto.Int32(74)},
		Command: &pb.CommandRequest_Client{Client: &pb.ClientCommand{Action: &pb.ClientCommand_Message{Message: &pb.MessageAction{
			Send: &pb.MessageAction_PrivateMessage{PrivateMessage: &pb.PrivateMessage{TargetCid: "SAS123", Text: "secret body"}}}}}}}
	change, status, _, err := (PrivateMessagePlanner{Secrets: secrets}).Plan(context.Background(), request, state)
	if err != nil || status != pb.CommandReply_COMMITTED || len(change.Effects) != 1 {
		t.Fatalf("private message was not planned: %v %v", status, err)
	}
	effect := change.Effects[0]
	if effect.TargetCid != "controller-1" || effect.GetPrivateMessage().Recipient != "SAS123" {
		t.Fatalf("effect target was not immutable sender CID: %+v", effect)
	}
	encoded, _ := proto.Marshal(change)
	if strings.Contains(string(encoded), "secret body") {
		t.Fatal("private message body appeared in durable state change")
	}
}

func bytesOf(n int, b byte) []byte {
	value := make([]byte, n)
	for i := range value {
		value[i] = b
	}
	return value
}

func effectState(t *testing.T) (*Aggregate, time.Time, string) {
	t.Helper()
	start := time.Now().UTC().Truncate(time.Second)
	id := uuid.NewString()
	ref := sessionRef(74)
	state := NewAggregate(ref)
	state.Owner = &pb.OwnerTerm{NodeId: "node-a", Epoch: 1, LeaseUntil: timestamppb.New(start.Add(3 * time.Minute))}
	request := &pb.EffectRecord{CommandId: id, TargetCid: "controller-1", OwnerEpoch: 1, Status: pb.EffectRecord_WAITING,
		Payload:          &pb.EffectRecord_Pdc{Pdc: &pb.PdcEffect{Callsign: "SAS123", Action: "ISSUE"}},
		DispatchDeadline: timestamppb.New(start.Add(30 * time.Second))}
	outcome := &pb.CommandOutcome{CommandId: id, RequestSha256: fmt.Sprintf("%x", sha256.Sum256([]byte("request"))),
		Actor: &pb.Actor{Kind: pb.Actor_CONTROLLER, Id: "controller-1"}, Status: pb.CommandOutcome_ACCEPTED,
		Aggregate: ref, AggregateRevision: 1}
	event := &pb.StateEvent{SchemaVersion: 1, EventId: uuid.NewString(), CommandId: &id, Aggregate: ref,
		AggregateRevision: 1, OwnerEpoch: 1, Actor: outcome.Actor,
		Fact: &pb.StateEvent_DomainChanged{DomainChanged: &pb.DomainChange{Effects: []*pb.EffectRecord{request}, Outcome: outcome}}}
	applyEffectTestEvent(t, state, event, start)
	if got := state.Effects[id].DispatchDeadline.AsTime(); !got.Equal(start.Add(effectDispatchWindow)) {
		t.Fatalf("request deadline not based on server time: %v", got)
	}
	return state, start, id
}

func applyEffectTestEvent(t *testing.T, state *Aggregate, event *pb.StateEvent, at time.Time) {
	t.Helper()
	data, err := proto.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	subject, _ := Subject(state.Ref)
	if _, err := state.Apply(AppliedEvent{Subject: subject, StreamSequence: state.StreamSequence + 1,
		SubjectSequence: state.SubjectSequence + 1, ServerTime: at, Data: data}); err != nil {
		t.Fatal(err)
	}
}

func transitionEvent(state *Aggregate, id string, status pb.EffectRecord_Status, connectionID string) *pb.StateEvent {
	next := proto.Clone(state.Effects[id]).(*pb.EffectRecord)
	next.Status = status
	if status == pb.EffectRecord_DISPATCH_CLAIMED {
		next.DispatchConnectionId = &connectionID
	}
	return &pb.StateEvent{SchemaVersion: 1, EventId: uuid.NewString(), CommandId: &id, Aggregate: state.Ref,
		AggregateRevision: state.Revision + 1, OwnerEpoch: 1, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "node-a"},
		Fact: &pb.StateEvent_EffectChanged{EffectChanged: next}}
}

func TestEffectOfflineBeforeClaimExpiresForSameCID(t *testing.T) {
	state, start, id := effectState(t)
	applyEffectTestEvent(t, state, transitionEvent(state, id, pb.EffectRecord_EXPIRED, ""), start.Add(31*time.Second))
	if state.Effects[id].TargetCid != "controller-1" || state.Effects[id].DispatchConnectionId != nil || state.Ledger[id].Status != pb.CommandOutcome_EXPIRED {
		t.Fatalf("offline effect was claimed or lost its CID: %+v", state.Effects[id])
	}
}

func TestEarlyExpiryIsNoOpUntilServerDeadline(t *testing.T) {
	state, start, id := effectState(t)
	event := transitionEvent(state, id, pb.EffectRecord_EXPIRED, "")
	data, _ := proto.Marshal(event)
	subject, _ := Subject(state.Ref)
	effective, err := state.Apply(AppliedEvent{Subject: subject, StreamSequence: state.StreamSequence + 1,
		SubjectSequence: state.SubjectSequence + 1, ServerTime: start.Add(29 * time.Second), Data: data})
	if err != nil || effective || state.Effects[id].Status != pb.EffectRecord_WAITING {
		t.Fatalf("early expiry damaged projection: effective=%v err=%v", effective, err)
	}
	applyEffectTestEvent(t, state, transitionEvent(state, id, pb.EffectRecord_EXPIRED, ""), start.Add(31*time.Second))
}

func TestClaimedEffectBecomesUnknownWithoutResend(t *testing.T) {
	state, start, id := effectState(t)
	applyEffectTestEvent(t, state, transitionEvent(state, id, pb.EffectRecord_DISPATCH_CLAIMED, "socket-1"), start.Add(time.Second))
	if !state.Effects[id].ResultDeadline.AsTime().Equal(start.Add(31 * time.Second)) {
		t.Fatal("result deadline did not start at claim")
	}
	applyEffectTestEvent(t, state, transitionEvent(state, id, pb.EffectRecord_UNKNOWN, ""), start.Add(32*time.Second))
	if state.Ledger[id].Status != pb.CommandOutcome_UNKNOWN || *state.Effects[id].DispatchConnectionId != "socket-1" {
		t.Fatal("ambiguous execution did not retain its one claimed generation")
	}
	late := transitionEvent(state, id, pb.EffectRecord_EXECUTED, "")
	data, _ := proto.Marshal(late)
	subject, _ := Subject(state.Ref)
	if _, err := state.Apply(AppliedEvent{Subject: subject, StreamSequence: state.StreamSequence + 1,
		SubjectSequence: state.SubjectSequence + 1, ServerTime: start.Add(33 * time.Second), Data: data}); err == nil {
		t.Fatal("late plugin result rewrote terminal unknown")
	}
}

func TestPluginResultTerminalOnlyOnce(t *testing.T) {
	state, start, id := effectState(t)
	applyEffectTestEvent(t, state, transitionEvent(state, id, pb.EffectRecord_DISPATCH_CLAIMED, "socket-1"), start.Add(time.Second))
	applyEffectTestEvent(t, state, transitionEvent(state, id, pb.EffectRecord_EXECUTED, ""), start.Add(2*time.Second))
	if state.Ledger[id].Status != pb.CommandOutcome_SUCCEEDED || len(state.Ledger) != 1 {
		t.Fatal("plugin result did not update original command outcome")
	}
	duplicate := transitionEvent(state, id, pb.EffectRecord_EXECUTED, "")
	data, _ := proto.Marshal(duplicate)
	subject, _ := Subject(state.Ref)
	if _, err := state.Apply(AppliedEvent{Subject: subject, StreamSequence: state.StreamSequence + 1,
		SubjectSequence: state.SubjectSequence + 1, ServerTime: start.Add(3 * time.Second), Data: data}); err == nil {
		t.Fatal("duplicate terminal event was appended")
	}
}

func TestPrivateMessageSuccessMeansLocalAcceptance(t *testing.T) {
	state, start, id := effectState(t)
	state.Effects[id].Payload = &pb.EffectRecord_PrivateMessage{PrivateMessage: &pb.PrivateMessageEffect{
		Recipient: "SAS123", ObjectName: "effect/" + id, Sha256: strings.Repeat("a", 64)}}
	applyEffectTestEvent(t, state, transitionEvent(state, id, pb.EffectRecord_DISPATCH_CLAIMED, "socket-1"), start.Add(time.Second))
	applyEffectTestEvent(t, state, transitionEvent(state, id, pb.EffectRecord_EXECUTED, ""), start.Add(2*time.Second))
	if !strings.Contains(state.Ledger[id].Detail, "local EuroScope send") || !strings.Contains(state.Ledger[id].Detail, "pilot receipt is not confirmed") {
		t.Fatalf("private message success claimed pilot receipt: %+v", state.Ledger[id])
	}
}

func TestLaterSyncReconcilesStateWithoutRewritingUnknown(t *testing.T) {
	state, _, id := effectState(t)
	state.Effects[id].Status = pb.EffectRecord_UNKNOWN
	state.Effects[id].Payload = &pb.EffectRecord_SetFlightPlan{SetFlightPlan: &pb.SetFlightPlanEffect{
		Callsign: "SAS123", Field: "ROUTE", Value: "NEXEN P60"}}
	state.Ledger[id].Status = pb.CommandOutcome_UNKNOWN
	matched := ReconciledUnknownEffects(state, &euroscope.SyncEvent{Strips: []*euroscope.Strip{{Callsign: "SAS123", Route: "NEXEN P60"}}})
	if len(matched) != 1 || matched[0] != id || state.Ledger[id].Status != pb.CommandOutcome_UNKNOWN {
		t.Fatal("sync either missed the observed state or rewrote unknown execution")
	}
	if got := ReconciledUnknownEffects(state, &euroscope.SyncEvent{Strips: []*euroscope.Strip{{Callsign: "SAS123", Route: "OTHER"}}}); len(got) != 0 {
		t.Fatal("different observed state was falsely reconciled")
	}
}

func TestEffectObjectsWaitUntilTerminalPlus24Hours(t *testing.T) {
	for _, cold := range []bool{false, true} {
		t.Run(fmt.Sprintf("cold=%t", cold), func(t *testing.T) { effectObjectRetentionCase(t, cold) })
	}
}

func effectObjectRetentionCase(t *testing.T, cold bool) {
	state, terminalAt, id := effectState(t)
	if cold {
		oldID := id
		id = "00000000-0000-4000-8000-000000000001"
		state.Effects[id] = state.Effects[oldID]
		state.Effects[id].CommandId = id
		delete(state.Effects, oldID)
		state.Ledger[id] = state.Ledger[oldID]
		state.Ledger[id].CommandId = id
		delete(state.Ledger, oldID)
	}
	objects := &effectObjects{}
	secrets := EffectSecrets{Objects: objects, ActiveKeyID: "v1", Keys: map[string][]byte{"v1": bytesOf(32, 9)}}
	secret, err := secrets.StagePrivateMessage(id, "controller-1", "SAS123", "stand A12")
	if err != nil {
		t.Fatal(err)
	}
	state.Effects[id].Payload = &pb.EffectRecord_PrivateMessage{PrivateMessage: secret}
	state.Effects[id].Status = pb.EffectRecord_EXECUTED
	state.Ledger[id].Status = pb.CommandOutcome_SUCCEEDED
	terminal := &pb.StateEvent{SchemaVersion: 1, EventId: uuid.NewString(), CommandId: &id, Aggregate: state.Ref,
		Fact: &pb.StateEvent_EffectChanged{EffectChanged: state.Effects[id]}}
	data, _ := proto.Marshal(terminal)
	if cold {
		state.history = testHistoryCache(t)
		for i := 0; i < historyWorkingSet; i++ {
			extra := fmt.Sprintf("ffffffff-ffff-4fff-8fff-%012x", i)
			state.Effects[extra] = &pb.EffectRecord{CommandId: extra, Status: pb.EffectRecord_EXPIRED}
		}
		if err := state.boundHistory(); err != nil {
			t.Fatal(err)
		}
		if state.Effects[id] != nil {
			t.Fatal("terminal secret reference must be cold in this case")
		}
	}
	subject, _ := Subject(state.Ref)
	store := &memoryStore{entries: []AppliedEvent{{Subject: subject, StreamSequence: 1, SubjectSequence: 1, ServerTime: terminalAt, Data: data}}}
	projection := &Projection{states: map[string]*Aggregate{subject: state}, started: true, checked: time.Now(),
		positionReady: true, presenceReady: true}
	collector := Effects{Owner: &OwnerRuntime{Projection: projection, Store: store}, Secrets: secrets}
	if n, err := collector.GarbageCollect(context.Background(), terminalAt.Add(23*time.Hour)); err != nil || n != 0 {
		t.Fatalf("deleted before terminal retention: %d %v", n, err)
	}
	if n, err := collector.GarbageCollect(context.Background(), terminalAt.Add(25*time.Hour)); err != nil || n != 1 {
		t.Fatalf("terminal object not collected: %d %v", n, err)
	}
	orphan := "effect/" + uuid.NewString()
	objects.values[orphan], objects.times[orphan] = []byte("orphan"), terminalAt
	if n, err := collector.GarbageCollect(context.Background(), terminalAt.Add(25*time.Hour)); err != nil || n != 1 {
		t.Fatalf("old orphan object not collected: %d %v", n, err)
	}
}
