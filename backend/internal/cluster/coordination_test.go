package cluster

import (
	"context"
	"slices"
	"strconv"
	"sync"
	"testing"

	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
)

func coordinationFixture(t *testing.T) (*memoryStore, CoordinationState, StripState) {
	t.Helper()
	store, strips := stripFixture(t)
	ctx := context.Background()
	controllers := ControllerSector{Store: strips.Store}
	for _, cid := range []string{"cid-a", "cid-b", "cid-c"} {
		if reply, err := controllers.PutController(ctx, 1, &pb.Controller{Cid: cid, Callsign: "EKCH_" + cid, Position: cid}, 0); err != nil {
			t.Fatalf("controller: %v %v", reply, err)
		}
	}
	if reply, err := strips.Put(ctx, 1, &pb.Strip{Callsign: "SAS101", Bay: "CLEARED", OwnerCid: "cid-a", NextControllers: []string{"cid-b", "cid-c"}}, 0); err != nil {
		t.Fatalf("strip: %v %v", reply, err)
	}
	return store, CoordinationState{Store: strips.Store}, strips
}

// Concrete protobuf oneof wrappers are used directly by the tests below.
func actTransfer(to string) *pb.CoordinationAction {
	return &pb.CoordinationAction{Callsign: "SAS101", Change: &pb.CoordinationAction_Transfer{Transfer: &pb.TransferCoordination{ToCid: to}}}
}
func actAssume() *pb.CoordinationAction {
	return &pb.CoordinationAction{Callsign: "SAS101", Change: &pb.CoordinationAction_Assume{Assume: &pb.AssumeCoordination{}}}
}
func actForce(from string) *pb.CoordinationAction {
	return &pb.CoordinationAction{Callsign: "SAS101", Change: &pb.CoordinationAction_ForceAssume{ForceAssume: &pb.ForceAssumeCoordination{FromCid: from}}}
}
func actFree() *pb.CoordinationAction {
	return &pb.CoordinationAction{Callsign: "SAS101", Change: &pb.CoordinationAction_Free{Free: &pb.FreeCoordination{}}}
}
func actCancel(id string) *pb.CoordinationAction {
	return &pb.CoordinationAction{Callsign: "SAS101", Change: &pb.CoordinationAction_Cancel{Cancel: &pb.CancelCoordination{TransferId: id}}}
}
func actTag(owner string) *pb.CoordinationAction {
	return &pb.CoordinationAction{Callsign: "SAS101", Change: &pb.CoordinationAction_Tag{Tag: &pb.TagCoordination{ToCid: owner}}}
}
func actAcceptTag(id string) *pb.CoordinationAction {
	return &pb.CoordinationAction{Callsign: "SAS101", Change: &pb.CoordinationAction_AcceptTag{AcceptTag: &pb.AcceptTagCoordination{RequestId: id}}}
}

func TestCoordinationAtomicTransferAssumeAndReplay(t *testing.T) {
	store, coordination, strips := coordinationFixture(t)
	ctx := context.Background()
	strip, _, err := strips.ByCallsign(ctx, 1, "SAS101")
	if err != nil {
		t.Fatal(err)
	}
	id := uuid.NewString()
	request := actTransfer("cid-b")
	if reply, err := coordination.Action(ctx, 1, "cid-c", uuid.NewString(), strip.Revision, request); err == nil || reply.Status != pb.CommandReply_UNAUTHORIZED {
		t.Fatalf("nonowner transfer: %v %v", reply, err)
	}
	first, err := coordination.Action(ctx, 1, "cid-a", id, strip.Revision, request)
	if err != nil {
		t.Fatal(err)
	}
	commits := store.commits
	again, err := coordination.Action(ctx, 1, "cid-a", id, strip.Revision, request)
	if err != nil || store.commits != commits || first.GetStreamSequence() != again.GetStreamSequence() {
		t.Fatalf("duplicate transfer: %v %v", again, err)
	}
	changed, err := coordination.Action(ctx, 1, "cid-a", id, strip.Revision, actTransfer("cid-c"))
	if err == nil || changed.Status != pb.CommandReply_INVALID_ARGUMENT || store.commits != commits {
		t.Fatalf("changed command ID: %v %v", changed, err)
	}
	c, _, err := coordination.ByStrip(ctx, 1, "SAS101")
	if err != nil || c.Id != 1 || c.ToCid != "cid-b" {
		t.Fatalf("transfer: %v %v", c, err)
	}
	byStripID, _, err := coordination.ByStripID(ctx, 1, strip.Id)
	if err != nil || !proto.Equal(c, byStripID) {
		t.Fatalf("strip ID projection: %v %v", byStripID, err)
	}
	byID, _, err := coordination.ByID(ctx, 1, c.Id)
	if err != nil || !proto.Equal(c, byID) {
		t.Fatalf("coordination ID projection: %v %v", byID, err)
	}
	if reply, err := coordination.Action(ctx, 1, "cid-c", uuid.NewString(), strip.Revision, actAssume()); err == nil || reply.Status != pb.CommandReply_UNAUTHORIZED {
		t.Fatalf("wrong target: %v %v", reply, err)
	}
	strip, _, _ = strips.ByCallsign(ctx, 1, "SAS101")
	if reply, err := coordination.Action(ctx, 1, "cid-b", uuid.NewString(), strip.Revision, actAssume()); err != nil {
		t.Fatalf("assume: %v %v", reply, err)
	}
	strip, _, _ = strips.ByCallsign(ctx, 1, "SAS101")
	if strip.OwnerCid != "cid-b" || len(strip.NextControllers) != 1 || strip.NextControllers[0] != "cid-c" || len(strip.PreviousControllers) != 1 || strip.PreviousControllers[0] != "cid-a" {
		t.Fatalf("owner route: %v", strip)
	}
	if _, _, err := coordination.ByStrip(ctx, 1, "SAS101"); err == nil {
		t.Fatal("assumed coordination remained active")
	}
	entries, err := store.Replay(ctx, "fs.v1.state.session.1")
	if err != nil {
		t.Fatal(err)
	}
	last := &pb.StateEvent{}
	if err := pb.UnmarshalStrict(entries[len(entries)-1].Data, last); err != nil {
		t.Fatal(err)
	}
	if len(last.GetDomainChanged().Changes) != 2 || last.GetDomainChanged().Changes[0].GetUpsert().GetStrip() == nil || last.GetDomainChanged().Changes[1].GetDelete().Kind != pb.EntityKind_COORDINATION {
		t.Fatalf("assume was not one strip+coordination event: %v", last)
	}
	replayed := CoordinationState{Store: LocalLifecycleStore{Writer: Writer{Store: store}}}
	if list, _, err := replayed.List(ctx, 1); err != nil || len(list) != 0 {
		t.Fatalf("replay: %v %v", list, err)
	}
}

func TestCoordinationTagForceCancelAndVersionRules(t *testing.T) {
	_, coordination, strips := coordinationFixture(t)
	ctx := context.Background()
	strip, _, _ := strips.ByCallsign(ctx, 1, "SAS101")
	if reply, err := coordination.Action(ctx, 1, "cid-b", uuid.NewString(), strip.Revision, actTag("cid-a")); err != nil {
		t.Fatalf("tag: %v %v", reply, err)
	}
	c, _, _ := coordination.ByStrip(ctx, 1, "SAS101")
	if reply, err := coordination.Action(ctx, 1, "cid-a", uuid.NewString(), strip.Revision-1, actAcceptTag(strconv.FormatUint(c.Id, 10))); err == nil || reply.GetOutcome().GetReasonCode() != "REVISION_CONFLICT" {
		t.Fatalf("stale strip version: %v %v", reply, err)
	}
	if reply, err := coordination.Action(ctx, 1, "cid-a", uuid.NewString(), strip.Revision, actAcceptTag(strconv.FormatUint(c.Id, 10))); err != nil {
		t.Fatalf("accept tag: %v %v", reply, err)
	}
	strip, _, _ = strips.ByCallsign(ctx, 1, "SAS101")
	if strip.OwnerCid != "cid-b" {
		t.Fatalf("tag owner: %v", strip)
	}
	if reply, err := coordination.Action(ctx, 1, "cid-b", uuid.NewString(), strip.Revision, actTransfer("cid-c")); err != nil {
		t.Fatalf("transfer: %v %v", reply, err)
	}
	c, _, _ = coordination.ByStrip(ctx, 1, "SAS101")
	if reply, err := coordination.Action(ctx, 1, "cid-a", uuid.NewString(), strip.Revision, actCancel(strconv.FormatUint(c.Id, 10))); err == nil || reply.Status != pb.CommandReply_UNAUTHORIZED {
		t.Fatalf("foreign cancel: %v %v", reply, err)
	}
	reply, err := coordination.Action(ctx, 1, "cid-a", uuid.NewString(), strip.Revision, actForce("cid-b"))
	if err != nil {
		t.Fatalf("force assume: %v %v", reply, err)
	}
	result, _, err := strips.ByCallsign(ctx, 1, "SAS101")
	if err != nil {
		t.Fatal(err)
	}
	if result.OwnerCid != "cid-a" || !slices.Equal(result.NextControllers, []string{"cid-c"}) {
		t.Fatalf("force-assume result: %v", result)
	}
	strip, _, _ = strips.ByCallsign(ctx, 1, "SAS101")
	if strip.OwnerCid != "cid-a" {
		t.Fatalf("force owner: %v", strip)
	}
	if _, _, err := coordination.ByStrip(ctx, 1, "SAS101"); err == nil {
		t.Fatal("force assume left transfer active")
	}
	if reply, err := coordination.Action(ctx, 1, "cid-a", uuid.NewString(), strip.Revision, actFree()); err != nil {
		t.Fatalf("free: %v %v", reply, err)
	}
	strip, _, _ = strips.ByCallsign(ctx, 1, "SAS101")
	if strip.OwnerCid != "" {
		t.Fatalf("free owner: %v", strip)
	}
}

func TestCoordinationConflictingControllersAcrossBackends(t *testing.T) {
	store, _, strips := coordinationFixture(t)
	ctx := context.Background()
	strip, _, _ := strips.ByCallsign(ctx, 1, "SAS101")
	read := Writer{Store: store}
	plan := SessionLifecyclePlanner(func(ctx context.Context, ref *pb.AggregateRef) (*Aggregate, error) {
		subject, _ := Subject(ref)
		return read.load(ctx, subject, ref)
	})
	backends := []CoordinationState{{Store: LocalLifecycleStore{Writer: Writer{Store: store, NodeID: "node-a", Plan: plan}}}, {Store: LocalLifecycleStore{Writer: Writer{Store: store, NodeID: "node-a", Plan: plan}}}}
	var wg sync.WaitGroup
	replies := make(chan *pb.CommandReply, 2)
	for i, cid := range []string{"cid-b", "cid-c"} {
		wg.Add(1)
		go func(i int, cid string) {
			defer wg.Done()
			reply, _ := backends[i].Action(ctx, 1, cid, uuid.NewString(), strip.Revision, actForce("cid-a"))
			replies <- reply
		}(i, cid)
	}
	wg.Wait()
	close(replies)
	succeeded, conflicted := 0, 0
	for reply := range replies {
		if reply.GetOutcome().GetStatus() == pb.CommandOutcome_SUCCEEDED {
			succeeded++
		} else if reply.GetOutcome().GetReasonCode() == "REVISION_CONFLICT" {
			conflicted++
		} else {
			t.Fatalf("unexpected result: %v", reply)
		}
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("success=%d conflict=%d", succeeded, conflicted)
	}
	state, err := read.load(ctx, "fs.v1.state.session.1", sessionRef(1))
	if err != nil {
		t.Fatal(err)
	}
	if state.Entities["SAS101"].GetValue().GetStrip().OwnerCid == "cid-a" {
		t.Fatal("owner was not changed")
	}
	if !proto.Equal(state.Entities["SAS101"].GetValue().GetStrip(), state.Indexes[pb.EntityKind_STRIP]["SAS101"].GetValue().GetStrip()) {
		t.Fatal("projection indexes disagree")
	}
}

func TestCoordinationGenericEntityWriteCannotBypassSessionAllocator(t *testing.T) {
	store, _, _ := coordinationFixture(t)
	zero := uint64(0)
	request := &pb.CommandRequest{ProtocolRevision: 1, CommandId: uuid.NewString(), Aggregate: sessionRef(1), Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "test"}, ExpectedEntityRevision: &zero, Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: &pb.UpdateEntity{Key: "1", Value: &pb.EntityRecord{Value: &pb.EntityRecord_Coordination{Coordination: &pb.Coordination{Id: 1, Callsign: "SAS101", FromCid: "cid-a", ToCid: "cid-b", Status: "TRANSFER"}}}}}}}}
	reply := (Writer{Store: store, NodeID: "node-a", Plan: PlanSystemEntity}).Execute(context.Background(), request)
	if reply.Status != pb.CommandReply_INVALID_ARGUMENT {
		t.Fatalf("generic write bypassed allocator: %v", reply)
	}
}

func TestCoordinationMissedApproachReturnMovesBayAtomically(t *testing.T) {
	store, coordination, strips := coordinationFixture(t)
	ctx := context.Background()
	controllers := ControllerSector{Store: strips.Store}
	for _, item := range []struct{ cid, section string }{{"cid-a", "TWR"}, {"cid-b", "APP"}} {
		old, _, err := controllers.ControllerByCID(ctx, 1, item.cid)
		if err != nil {
			t.Fatal(err)
		}
		old.Section = item.section
		if reply, err := controllers.PutController(ctx, 1, old, old.Revision); err != nil {
			t.Fatalf("section: %v %v", reply, err)
		}
	}
	strip, _, _ := strips.ByCallsign(ctx, 1, "SAS101")
	strip.Bay = "AIRBORNE"
	if reply, err := strips.Put(ctx, 1, strip, strip.Revision); err != nil {
		t.Fatalf("airborne: %v %v", reply, err)
	}
	strip, _, _ = strips.ByCallsign(ctx, 1, "SAS101")
	if reply, err := coordination.Action(ctx, 1, "cid-a", uuid.NewString(), strip.Revision, actTransfer("cid-b")); err != nil {
		t.Fatalf("transfer: %v %v", reply, err)
	}
	if reply, err := coordination.Action(ctx, 1, "cid-b", uuid.NewString(), strip.Revision, actAssume()); err != nil {
		t.Fatalf("assume: %v %v", reply, err)
	}
	strip, _, _ = strips.ByCallsign(ctx, 1, "SAS101")
	if strip.OwnerCid != "cid-b" || strip.Bay != "FINAL" || len(strip.NextControllers) == 0 || strip.NextControllers[0] != "cid-a" || slices.Contains(strip.PreviousControllers, "cid-a") {
		t.Fatalf("missed approach owner route: %v", strip)
	}
	entries, err := store.Replay(ctx, "fs.v1.state.session.1")
	if err != nil {
		t.Fatal(err)
	}
	event := &pb.StateEvent{}
	if err := pb.UnmarshalStrict(entries[len(entries)-1].Data, event); err != nil {
		t.Fatal(err)
	}
	if len(event.GetDomainChanged().Changes) != 2 {
		t.Fatalf("missed approach was split across events: %v", event)
	}
}

func TestCoordinationEuroScopeArrivalPreservesHandoverAndCorrectsStrip(t *testing.T) {
	store, coordination, strips := coordinationFixture(t)
	ctx := context.Background()
	strip, _, _ := strips.ByCallsign(ctx, 1, "SAS101")
	strip.Bay = "ARR_HIDDEN"
	if reply, err := strips.Put(ctx, 1, strip, strip.Revision); err != nil {
		t.Fatalf("arrival bay: %v %v", reply, err)
	}
	strip, _, _ = strips.ByCallsign(ctx, 1, "SAS101")
	id := uuid.NewString()
	candidate := &pb.Coordination{Callsign: "SAS101", FromCid: "cid-b", ToCid: "cid-a", Status: "TRANSFER", FromEuroscope: true, EuroscopeHandoverCid: "cid-a"}
	first, err := coordination.ObserveTransfer(ctx, 1, id, strip.Revision, candidate)
	if err != nil {
		t.Fatalf("EuroScope transfer: %v %v", first, err)
	}
	commits := store.commits
	second, err := coordination.ObserveTransfer(ctx, 1, id, strip.Revision, candidate)
	if err != nil || second.GetStreamSequence() != first.GetStreamSequence() || store.commits != commits {
		t.Fatalf("duplicate observation: %v %v", second, err)
	}
	strip, _, _ = strips.ByCallsign(ctx, 1, "SAS101")
	c, _, err := coordination.ByStrip(ctx, 1, "SAS101")
	if err != nil || strip.Bay != "FINAL" || strip.OwnerCid != "" || !c.FromEuroscope || c.EuroscopeHandoverCid != "cid-a" {
		t.Fatalf("arrival transition: strip=%v coordination=%v err=%v", strip, c, err)
	}
	entries, err := store.Replay(ctx, "fs.v1.state.session.1")
	if err != nil {
		t.Fatal(err)
	}
	event := &pb.StateEvent{}
	if err := pb.UnmarshalStrict(entries[len(entries)-1].Data, event); err != nil {
		t.Fatal(err)
	}
	if len(event.GetDomainChanged().Changes) != 3 {
		t.Fatalf("arrival transition was split across events: %v", event)
	}
}
