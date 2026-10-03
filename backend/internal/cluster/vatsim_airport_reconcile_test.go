package cluster

import (
	"context"
	"testing"
	"time"

	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestVatsimAirportReconcilerRetractsMissingArrivalAndReplays(t *testing.T) {
	ctx := context.Background()
	store, _, source := navFixture(t)
	worker := AmanCandidateWorker{Source: source, State: AmanAdapter{Writer: Writer{Store: store, NodeID: "node-a"}}}
	at := timestamppb.New(time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC))
	checkpoint := func(flights ...*pb.VatsimFlight) {
		t.Helper()
		page := &pb.ProviderPage{Provider: "vatsim", Resource: "network-data/v3", Parsed: &pb.ProviderPage_Vatsim{Vatsim: &pb.VatsimPage{SnapshotAt: at, Flights: flights}}}
		name, sha, err := source.PublishProvider(page)
		if err != nil {
			t.Fatal(err)
		}
		if reply, err := source.PutCheckpointFor(ctx, globalRef(), uuid.NewString(), &pb.ProviderCheckpoint{Provider: "vatsim", Resource: "network-data/v3", ObjectName: name, Sha256: sha}); err != nil || reply.Status != pb.CommandReply_COMMITTED {
			t.Fatalf("checkpoint: %v %v", reply, err)
		}
	}
	checkpoint(&pb.VatsimFlight{Cid: "12345", Callsign: "SAS123", State: "online", FlightPlan: &pb.VatsimFlightPlan{Origin: "EDDF", Destination: "EKCH", Revision: 3}})
	presentCalls, missingCalls := 0, 0
	reconciler := VatsimAirportReconciler{Worker: worker}
	reconciler.EvaluatePresent = func(_ context.Context, board AmanBoard, flight *pb.VatsimFlight, _ *pb.VatsimObservation, _ *pb.CommandRequest) (AmanTransition, error) {
		presentCalls++
		if flight == nil {
			t.Fatal("present evaluator received nil flight")
		}
		revision := uint64(1)
		if board.Airport != nil {
			revision = board.Airport.Revision + 1
		}
		return AmanTransition{Airport: &pb.AmanAirport{Airport: "EKCH", Revision: revision, PolicyVersion: "v1", GeneratedAt: at}, Flights: []*pb.AmanFlight{{Callsign: flight.Callsign, State: "planned", FreezeReason: "none", UpdatedAt: at, LatestObservation: &pb.AmanFlightObservation{Callsign: flight.Callsign, Provider: "vatsim", SourceStatus: "fresh", ReconciledAt: at}}}}, nil
	}
	reconciler.EvaluateMissing = func(_ context.Context, board AmanBoard, flight *pb.VatsimFlight, _ *pb.VatsimObservation, _ *pb.CommandRequest) (AmanTransition, error) {
		missingCalls++
		if flight != nil {
			t.Fatal("missing evaluator received flight")
		}
		flights := make([]*pb.AmanFlight, 0, len(board.Flights))
		for _, old := range board.Flights {
			copy := proto.Clone(old).(*pb.AmanFlight)
			copy.LatestObservation.Missing = true
			flights = append(flights, copy)
		}
		return AmanTransition{Airport: &pb.AmanAirport{Airport: "EKCH", Revision: board.Airport.Revision + 1, PolicyVersion: "v1", GeneratedAt: at}, Flights: flights}, nil
	}
	if err := reconciler.Reconcile(ctx, "EKCH"); err != nil {
		t.Fatal(err)
	}
	// A completed observation is answered from durable state even when its
	// immutable provider payload cannot be read. New generations still read it.
	withoutObjects := worker
	withoutObjects.Source.Objects = nil
	reply := withoutObjects.ObserveVatsim(ctx, "EKCH", "sas123", reconciler.EvaluatePresent)
	if reply.Status != pb.CommandReply_COMMITTED || presentCalls != 1 {
		t.Fatalf("completed observation read object storage: %v calls=%d", reply, presentCalls)
	}
	if err := reconciler.Reconcile(ctx, "EKCH"); err != nil {
		t.Fatal(err)
	}
	if presentCalls != 1 {
		t.Fatalf("present evaluated %d times", presentCalls)
	}
	checkpoint()
	if err := reconciler.Reconcile(ctx, "EKCH"); err != nil {
		t.Fatal(err)
	}
	if err := reconciler.Reconcile(ctx, "EKCH"); err != nil {
		t.Fatal(err)
	}
	if missingCalls != 1 {
		t.Fatalf("missing evaluated %d times", missingCalls)
	}
	board, err := worker.State.Read(ctx, "EKCH")
	if err != nil || board.Airport.Revision != 2 || len(board.Flights) != 1 || !board.Flights[0].LatestObservation.Missing {
		t.Fatalf("retracted board: %+v %v", board, err)
	}
}
