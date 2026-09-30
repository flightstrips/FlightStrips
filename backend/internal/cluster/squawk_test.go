package cluster

import (
	"testing"
	"time"

	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
)

func squawkReducerState(t *testing.T) (*Aggregate, time.Time, string) {
	t.Helper()
	a, at, id := effectState(t)
	a.Effects[id].Payload = &pb.EffectRecord_GenerateSquawk{GenerateSquawk: &pb.GenerateSquawkEffect{Callsign: "SAS123"}}
	a.Entities["74"] = &pb.EntitySnapshot{Key: "74", Revision: 1, Value: &pb.EntityRecord{Value: &pb.EntityRecord_Session{Session: &pb.Session{Id: 74, Airport: "EKCH", Name: "LIVE", NextStripId: 3}}}}
	a.Entities["SAS123"] = &pb.EntitySnapshot{Key: "SAS123", Revision: 1, Value: &pb.EntityRecord{Value: &pb.EntityRecord_Strip{Strip: &pb.Strip{Id: 1, Revision: 1, Callsign: "SAS123", Bay: "NOT_CLEARED", Sequence: 1000}}}}
	a.rebuildIndexes()
	return a, at, id
}

func TestSquawkThrottleServerTimeAtomicReplayAndSnapshot(t *testing.T) {
	a, at, id := squawkReducerState(t)
	applyEffectTestEvent(t, a, transitionEvent(a, id, pb.EffectRecord_DISPATCH_CLAIMED, "socket-1"), at.Add(time.Second))
	throttle := a.Indexes[pb.EntityKind_SESSION_SQUAWK_THROTTLE]["74"]
	if throttle == nil || !throttle.Value.GetSessionSquawkThrottle().NextAllowedAt.AsTime().Equal(at.Add(6*time.Second)) || a.Effects[id].Status != pb.EffectRecord_DISPATCH_CLAIMED {
		t.Fatal("claim/throttle not derived atomically from JetStream server timestamp")
	}
	snapshot, err := a.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	restored, err := aggregateFromSnapshot(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(throttle, restored.Indexes[pb.EntityKind_SESSION_SQUAWK_THROTTLE]["74"]) {
		t.Fatal("snapshot lost throttle entity beside the same-key session seed")
	}
	second := uuid.NewString()
	effect := proto.Clone(a.Effects[id]).(*pb.EffectRecord)
	effect.CommandId = second
	effect.Status = pb.EffectRecord_WAITING
	effect.ResultDeadline = nil
	effect.DispatchConnectionId = nil
	effect.Payload = &pb.EffectRecord_GenerateSquawk{GenerateSquawk: &pb.GenerateSquawkEffect{Callsign: "SAS124"}}
	a.Entities["SAS124"] = &pb.EntitySnapshot{Key: "SAS124", Revision: 1, Value: &pb.EntityRecord{Value: &pb.EntityRecord_Strip{Strip: &pb.Strip{Id: 2, Revision: 1, Callsign: "SAS124", Bay: "NOT_CLEARED", Sequence: 2000}}}}
	a.rebuildIndexes()
	a.Effects[second] = effect
	outcome := proto.Clone(a.Ledger[id]).(*pb.CommandOutcome)
	outcome.CommandId = second
	outcome.CommittedStreamSequence++
	a.Ledger[second] = outcome
	// A proposer clock ahead of server time cannot shorten the throttle.
	applyEffectTestEvent(t, a, transitionEvent(a, second, pb.EffectRecord_DISPATCH_CLAIMED, "socket-2"), at.Add(5999*time.Millisecond))
	if a.Effects[second].Status != pb.EffectRecord_WAITING || !proto.Equal(throttle, a.Indexes[pb.EntityKind_SESSION_SQUAWK_THROTTLE]["74"]) {
		t.Fatal("early server claim shortened throttle")
	}
	applyEffectTestEvent(t, a, transitionEvent(a, second, pb.EffectRecord_DISPATCH_CLAIMED, "socket-2"), at.Add(6*time.Second))
	if a.Effects[second].Status != pb.EffectRecord_DISPATCH_CLAIMED {
		t.Fatal("claim did not admit at five-second boundary")
	}
}

func TestSquawkClaimSuppressesValidAssignedRemovedAndOutOfOrder(t *testing.T) {
	for _, scenario := range []string{"assigned", "removed", "queue"} {
		t.Run(scenario, func(t *testing.T) {
			a, at, id := squawkReducerState(t)
			switch scenario {
			case "assigned":
				a.Entities["SAS123"].Value.GetStrip().AssignedSquawk = "2101"
			case "removed":
				delete(a.Entities, "SAS123")
			case "queue":
				other := uuid.NewString()
				effect := proto.Clone(a.Effects[id]).(*pb.EffectRecord)
				effect.CommandId = other
				a.Effects[other] = effect
				outcome := proto.Clone(a.Ledger[id]).(*pb.CommandOutcome)
				outcome.CommandId = other
				a.Ledger[other] = outcome
				a.Ledger[id].CommittedStreamSequence++
			}
			a.rebuildIndexes()
			applyEffectTestEvent(t, a, transitionEvent(a, id, pb.EffectRecord_DISPATCH_CLAIMED, "socket-1"), at.Add(time.Second))
			if a.Effects[id].Status != pb.EffectRecord_WAITING || a.Indexes[pb.EntityKind_SESSION_SQUAWK_THROTTLE]["74"] != nil {
				t.Fatal("invalid squawk dispatch claimed")
			}
		})
	}
}

func TestSquawkCancellationAndUnknownCannotRequeue(t *testing.T) {
	a, at, id := squawkReducerState(t)
	a.Entities["SAS123"].Value.GetStrip().AssignedSquawk = "2101"
	e := transitionEvent(a, id, pb.EffectRecord_FAILED, "")
	e.GetEffectChanged().ReasonCode = "SQUAWK_ASSIGNED"
	applyEffectTestEvent(t, a, e, at.Add(time.Second))
	if a.Ledger[id].ReasonCode != "SQUAWK_ASSIGNED" {
		t.Fatal("valid assigned squawk not cancelled")
	}
	a, at, id = squawkReducerState(t)
	applyEffectTestEvent(t, a, transitionEvent(a, id, pb.EffectRecord_DISPATCH_CLAIMED, "socket-1"), at.Add(time.Second))
	applyEffectTestEvent(t, a, transitionEvent(a, id, pb.EffectRecord_UNKNOWN, ""), at.Add(32*time.Second))
	if a.Ledger[id].Status != pb.CommandOutcome_UNKNOWN || a.Effects[id].GetDispatchConnectionId() != "socket-1" {
		t.Fatal("squawk uncertainty lost generation/outcome")
	}
	e = transitionEvent(a, id, pb.EffectRecord_WAITING, "")
	data, _ := proto.Marshal(e)
	subject, _ := Subject(a.Ref)
	if _, err := a.Apply(AppliedEvent{Subject: subject, StreamSequence: a.StreamSequence + 1, SubjectSequence: a.SubjectSequence + 1, ServerTime: at.Add(33 * time.Second), Data: data}); err == nil {
		t.Fatal("UNKNOWN squawk requeued")
	}
}
