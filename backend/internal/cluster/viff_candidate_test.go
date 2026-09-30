package cluster

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"FlightStrips/internal/cdm"
	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
)

type viffStub struct {
	calls int
	call  func() error
	last  string
}

func seedViffSession(t *testing.T, writer Writer, sessionID int32, airport string) {
	t.Helper()
	key := strconv.FormatInt(int64(sessionID), 10)
	zero := uint64(0)
	request := &pb.CommandRequest{ProtocolRevision: 1, CommandId: uuid.NewString(), Aggregate: sessionRef(sessionID), Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "viff-test"}, ExpectedEntityRevision: &zero, Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: &pb.UpdateEntity{Key: key, Value: &pb.EntityRecord{Value: &pb.EntityRecord_Session{Session: &pb.Session{Id: sessionID, Airport: airport, Name: "LIVE"}}}}}}}}
	if reply := writer.Execute(context.Background(), request); reply.Status != pb.CommandReply_COMMITTED || reply.GetOutcome().GetStatus() != pb.CommandOutcome_SUCCEEDED {
		t.Fatalf("seed vIFF session: %v", reply)
	}
}

func (s *viffStub) invoke() error {
	s.calls++
	if s.call != nil {
		return s.call()
	}
	return nil
}
func (s *viffStub) SetMasterAirport(context.Context, string, string) error {
	s.last = "set-master"
	return s.invoke()
}
func (s *viffStub) ClearMasterAirport(context.Context, string, string) error {
	s.last = "clear-master"
	return s.invoke()
}
func (s *viffStub) IFPSDpi(context.Context, string, string) error { s.last = "dpi"; return s.invoke() }
func (s *viffStub) IFPSSetCdmData(context.Context, cdm.SetCdmDataParams) error {
	s.last = "set-cdm-data"
	return s.invoke()
}
func (s *viffStub) IFPSSetTobt(context.Context, string, string, int) error {
	s.last = "set-tobt"
	return s.invoke()
}

func TestViffWriteCommitAndTakeover(t *testing.T) {
	store, _, nav := navFixture(t)
	ctx := context.Background()
	client := &viffStub{}
	spec := ViffWriteSpec{OperationID: uuid.NewString(), Kind: ViffSetMaster, Airport: "EKCH", Position: "EKCH_TWR"}
	adapter := ViffWriteAdapter{Worker: ExternalCallWorker{Writer: nav.Writer}, Client: client}
	if sent, err := adapter.Run(ctx, spec); !sent || err != nil || client.calls != 1 {
		t.Fatalf("first vIFF write: %v %v calls=%d", sent, err, client.calls)
	}
	if sent, err := adapter.Run(ctx, spec); sent || err != nil || client.calls != 1 {
		t.Fatalf("replayed vIFF write: %v %v calls=%d", sent, err, client.calls)
	}
	changed := spec
	changed.Position = "EKCH_APP"
	if sent, err := adapter.Run(ctx, changed); sent || err == nil || client.calls != 1 {
		t.Fatalf("changed command identity accepted: %v %v calls=%d", sent, err, client.calls)
	}
	handoffAirport(t, store)
	other := ViffWriteAdapter{Worker: ExternalCallWorker{Writer: Writer{Store: store, NodeID: "node-b"}}, Client: client}
	if err := other.Resume(ctx, "EKCH", 0); err != nil {
		t.Fatal(err)
	}
	if sent, err := other.Run(ctx, spec); sent || err != nil || client.calls != 1 {
		t.Fatalf("takeover repeated vIFF write: %v %v calls=%d", sent, err, client.calls)
	}
	state, err := other.Worker.Writer.load(ctx, "fs.v1.state.airport.EKCH", airportRef("EKCH"))
	if err != nil || state.Workflows[spec.OperationID].Status != pb.WorkflowRecord_COMPLETED {
		t.Fatalf("result not durable: %v %v", state.Workflows[spec.OperationID], err)
	}
}

func TestViffWriteUncertainResultDoesNotRetry(t *testing.T) {
	store, _, nav := navFixture(t)
	ctx := context.Background()
	client := &viffStub{call: func() error { return errors.New("response lost after POST") }}
	spec := ViffWriteSpec{OperationID: uuid.NewString(), Kind: ViffSetMaster, Airport: "EKCH", Position: "EKCH_TWR"}
	adapter := ViffWriteAdapter{Worker: ExternalCallWorker{Writer: nav.Writer}, Client: client}
	if sent, err := adapter.Run(ctx, spec); !sent || err == nil || client.calls != 1 {
		t.Fatalf("uncertain vIFF write: %v %v calls=%d", sent, err, client.calls)
	}
	handoffAirport(t, store)
	other := ViffWriteAdapter{Worker: ExternalCallWorker{Writer: Writer{Store: store, NodeID: "node-b"}}, Client: client}
	if err := other.Resume(ctx, "EKCH", 0); err != nil {
		t.Fatal(err)
	}
	if sent, err := other.Run(ctx, spec); sent || err != nil || client.calls != 1 {
		t.Fatalf("uncertain vIFF write retried: %v %v calls=%d", sent, err, client.calls)
	}
	state, err := other.Worker.Writer.load(ctx, "fs.v1.state.airport.EKCH", airportRef("EKCH"))
	if err != nil || state.Workflows[spec.OperationID].Status != pb.WorkflowRecord_FAILED || state.Workflows[spec.OperationID].ReasonCode != "CALL_UNCERTAIN" {
		t.Fatalf("uncertain state: %v %v", state.Workflows[spec.OperationID], err)
	}
}

func TestViffWriteLostResultAckRecoversCommittedCall(t *testing.T) {
	store, _, nav := navFixture(t)
	ctx := context.Background()
	client := &viffStub{call: func() error {
		store.mu.Lock()
		store.loseAck, store.failAfterAck = true, true
		store.mu.Unlock()
		return nil
	}}
	spec := ViffWriteSpec{OperationID: uuid.NewString(), Kind: ViffSetMaster, Airport: "EKCH", Position: "EKCH_TWR"}
	first := ViffWriteAdapter{Worker: ExternalCallWorker{Writer: nav.Writer}, Client: client}
	_, _ = first.Run(ctx, spec)
	if client.calls != 1 {
		t.Fatalf("vIFF call count=%d", client.calls)
	}
	handoffAirport(t, store)
	other := ViffWriteAdapter{Worker: ExternalCallWorker{Writer: Writer{Store: store, NodeID: "node-b"}}, Client: client}
	if err := other.Resume(ctx, "EKCH", 0); err != nil {
		t.Fatal(err)
	}
	if sent, err := other.Run(ctx, spec); sent || err != nil || client.calls != 1 {
		t.Fatalf("result replay resent vIFF: %v %v calls=%d", sent, err, client.calls)
	}
	state, err := other.Worker.Writer.load(ctx, "fs.v1.state.airport.EKCH", airportRef("EKCH"))
	if err != nil || state.Workflows[spec.OperationID].Status != pb.WorkflowRecord_COMPLETED {
		t.Fatalf("committed result not recovered: %v %v", state.Workflows[spec.OperationID], err)
	}
}

func TestViffWriteRoutesEveryOperationalMethod(t *testing.T) {
	store, _, nav := navFixture(t)
	ref := sessionRef(123)
	subject, _ := Subject(ref)
	claim := &pb.StateEvent{SchemaVersion: 1, EventId: uuid.NewString(), Aggregate: ref, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "node-a"}, Fact: &pb.StateEvent_OwnerClaimed{OwnerClaimed: &pb.OwnerTerm{NodeId: "node-a", Epoch: 1}}}
	data, _ := proto.Marshal(claim)
	if _, err := store.Publish(context.Background(), subject, 0, data); err != nil {
		t.Fatal(err)
	}
	seedViffSession(t, nav.Writer, 123, "EKCH")
	client := &viffStub{}
	adapter := ViffWriteAdapter{Worker: ExternalCallWorker{Writer: nav.Writer}, Client: client}
	for _, spec := range []ViffWriteSpec{
		{Kind: ViffClearMaster, Airport: "EKCH", Position: "EKCH_TWR"},
		{Kind: ViffDpi, Airport: "EKCH", SessionID: 123, Callsign: "SAS123", Value: "REA/1"},
		{Kind: ViffSetCdmData, Airport: "EKCH", SessionID: 123, Callsign: "SAS123", Data: cdm.SetCdmDataParams{Callsign: "SAS123", Tobt: "1200"}},
		{Kind: ViffSetTobt, Airport: "EKCH", SessionID: 123, Callsign: "SAS123", Value: "1200", TaxiMinutes: 10},
	} {
		spec.OperationID = uuid.NewString()
		if sent, err := adapter.Run(context.Background(), spec); !sent || err != nil || client.last != string(spec.Kind) {
			t.Fatalf("vIFF %s: %v %v last=%s", spec.Kind, sent, err, client.last)
		}
	}
	if client.calls != 4 {
		t.Fatalf("vIFF method dispatch calls=%d", client.calls)
	}
}
