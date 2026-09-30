package cluster

import (
	"context"
	"testing"
	"time"

	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestAmanCandidateConsumesTypedVatsimAndReplaysTick(t *testing.T) {
	ctx := context.Background()
	store, objects, source := navFixture(t)
	_ = objects
	worker := AmanCandidateWorker{Source: source, State: AmanAdapter{Writer: Writer{Store: store, NodeID: "node-a"}}}
	at := timestamppb.New(time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC))
	page := &pb.ProviderPage{Provider: "vatsim", Resource: "network-data/v3", Parsed: &pb.ProviderPage_Vatsim{Vatsim: &pb.VatsimPage{SnapshotAt: at,
		Flights: []*pb.VatsimFlight{{Cid: "12345", Callsign: "SAS123", State: "online", FlightPlan: &pb.VatsimFlightPlan{Origin: "EDDF", Destination: "EKCH", Revision: 3}}}}}}
	name, sha, err := source.PublishProvider(page)
	if err != nil {
		t.Fatal(err)
	}
	if reply, err := source.PutCheckpointFor(ctx, globalRef(), uuid.NewString(), &pb.ProviderCheckpoint{Provider: "vatsim", Resource: "network-data/v3", ObjectName: name, Sha256: sha}); err != nil || reply.Status != pb.CommandReply_COMMITTED {
		t.Fatalf("source checkpoint: %v %v", reply, err)
	}
	var observed string
	evaluate := func(_ context.Context, board AmanBoard, flight *pb.VatsimFlight, observation *pb.VatsimObservation, request *pb.CommandRequest) (AmanTransition, error) {
		observed = flight.Callsign
		return AmanTransition{Airport: &pb.AmanAirport{Airport: "EKCH", Revision: 1, PolicyVersion: "v1", GeneratedAt: at},
			Flights: []*pb.AmanFlight{{Callsign: flight.Callsign, State: "planned", FreezeReason: "none", UpdatedAt: at}}}, nil
	}
	first := worker.ObserveVatsim(ctx, "EKCH", "SAS123", evaluate)
	if first.Status != pb.CommandReply_COMMITTED || first.GetOutcome().GetStatus() != pb.CommandOutcome_SUCCEEDED || observed != "SAS123" {
		t.Fatalf("observation: %v", first)
	}
	commits := store.commits
	observed = ""
	replay := worker.ObserveVatsim(ctx, "EKCH", "SAS123", evaluate)
	if replay.GetStreamSequence() != first.GetStreamSequence() || observed != "" || store.commits != commits {
		t.Fatalf("observation replay: %v", replay)
	}
	tick := worker.Reconcile(ctx, "EKCH", at.AsTime().Add(time.Minute), func(_ context.Context, board AmanBoard, _ time.Time) (AmanTransition, error) {
		return AmanTransition{Airport: &pb.AmanAirport{Airport: "EKCH", Revision: board.Airport.Revision + 1, PolicyVersion: "v1", GeneratedAt: at}, Flights: board.Flights}, nil
	})
	if tick.GetOutcome().GetStatus() != pb.CommandOutcome_SUCCEEDED {
		t.Fatalf("tick: %v", tick)
	}
	commits = store.commits
	replay = worker.Reconcile(ctx, "EKCH", at.AsTime().Add(time.Minute), func(context.Context, AmanBoard, time.Time) (AmanTransition, error) {
		t.Fatal("replayed tick evaluated")
		return AmanTransition{}, nil
	})
	if replay.GetStreamSequence() != tick.GetStreamSequence() || store.commits != commits {
		t.Fatalf("tick replay: %v", replay)
	}
}
