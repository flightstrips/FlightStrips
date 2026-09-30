package cluster

import (
	"context"
	"testing"
	"time"

	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestVatsimSessionGenerationFencesReplayAndPreservesEuroScope(t *testing.T) {
	ctx := context.Background()
	store, strips := stripFixture(t)
	objects := &memoryObjects{values: map[string][]byte{}}
	writer := Writer{Store: store, NodeID: "node-a"}
	source := NavigationWeather{Writer: writer, Objects: objects}
	adapter := VatsimSessionAdapter{Source: source, Writer: writer}
	at := timestamppb.New(time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC))
	flight := func(callsign, route string) *pb.VatsimFlight {
		return &pb.VatsimFlight{Cid: callsign, Callsign: callsign, State: "prefile", FlightPlan: &pb.VatsimFlightPlan{
			Origin: "EKCH", Destination: "EDDF", AircraftShort: "A320", Route: route, Revision: 1,
		}}
	}
	put := func(flights ...*pb.VatsimFlight) (uint64, string) {
		t.Helper()
		page := &pb.ProviderPage{Provider: "vatsim", Resource: "network-data/v3", Parsed: &pb.ProviderPage_Vatsim{Vatsim: &pb.VatsimPage{SnapshotAt: at, Flights: flights}}}
		name, sha, err := source.PublishProvider(page)
		if err != nil {
			t.Fatal(err)
		}
		if reply, err := source.PutCheckpointFor(ctx, globalRef(), uuid.NewString(), &pb.ProviderCheckpoint{Provider: "vatsim", Resource: "network-data/v3", ObjectName: name, Sha256: sha}); err != nil || reply.Status != pb.CommandReply_COMMITTED {
			t.Fatalf("checkpoint: %v %v", reply, err)
		}
		_, _, revision, err := source.CheckpointRevisionFor(ctx, globalRef(), "vatsim", "network-data/v3")
		if err != nil {
			t.Fatal(err)
		}
		return revision, sha
	}
	firstRevision, firstSHA := put(flight("KLM123", "DCT A"), flight("SAS123", "DCT B"))
	first := adapter.Reconcile(ctx, 1)
	if first.Status != pb.CommandReply_COMMITTED || first.GetOutcome().GetStatus() != pb.CommandOutcome_SUCCEEDED {
		t.Fatalf("first: %v", first)
	}
	state, err := writer.load(ctx, "fs.v1.state.session.1", sessionRef(1))
	if err != nil {
		t.Fatal(err)
	}
	klm := state.Indexes[pb.EntityKind_STRIP]["KLM123"].GetValue().GetStrip()
	sas := state.Indexes[pb.EntityKind_STRIP]["SAS123"].GetValue().GetStrip()
	if klm.Id == sas.Id || klm.Sequence == sas.Sequence || klm.VatsimSourceRevision != firstRevision || sas.Route != "DCT B" {
		t.Fatalf("first generation: KLM=%v SAS=%v", klm, sas)
	}
	commits := store.commits
	if replay := adapter.Reconcile(ctx, 1); replay.Status != pb.CommandReply_COMMITTED || store.commits != commits {
		t.Fatalf("replay mutated session: %v", replay)
	}
	observed := *sas
	observed.Route = "EUROSCOPE ROUTE"
	observed.VatsimOnly = false
	if reply, err := strips.Observe(ctx, 1, &observed, sas.Revision); err != nil || reply.Status != pb.CommandReply_COMMITTED {
		t.Fatalf("EuroScope observation: %v %v", reply, err)
	}
	at = timestamppb.New(at.AsTime().Add(time.Minute))
	put(flight("SAS123", "NEW PROVIDER ROUTE"))
	second := adapter.Reconcile(ctx, 1)
	if second.Status != pb.CommandReply_COMMITTED || second.GetOutcome().GetStatus() != pb.CommandOutcome_SUCCEEDED {
		t.Fatalf("second: %v", second)
	}
	state, err = writer.load(ctx, "fs.v1.state.session.1", sessionRef(1))
	if err != nil {
		t.Fatal(err)
	}
	if state.Indexes[pb.EntityKind_STRIP]["KLM123"] != nil {
		t.Fatal("absent VATSIM-only strip retained")
	}
	sas = state.Indexes[pb.EntityKind_STRIP]["SAS123"].GetValue().GetStrip()
	if sas.Route != "EUROSCOPE ROUTE" || sas.VatsimOnly {
		t.Fatalf("EuroScope strip overwritten: %v", sas)
	}
	if _, status, _, err := planVatsimGeneration(state, 1, firstRevision, firstSHA, &pb.VatsimPage{SnapshotAt: at}); status != pb.CommandReply_REVISION_CONFLICT || err == nil {
		t.Fatalf("stale generation accepted: %v %v", status, err)
	}
}
