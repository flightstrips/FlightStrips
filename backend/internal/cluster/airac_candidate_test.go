package cluster

import (
	"context"
	"testing"
	"time"

	pb "FlightStrips/pkg/events/cluster"
	"google.golang.org/protobuf/proto"
)

func airacTestPage(cycle string) *pb.AiracPage {
	airport := testNav(cycle, "Copenhagen")
	fix := proto.Clone(airport).(*pb.NavData)
	fix.Fragment = &pb.NavData_FixFragment{FixFragment: &pb.NavFixFragment{Coverage: "complete", Fixes: []*pb.NavFix{{Id: "TESPI", Position: &pb.NavCoordinate{LatitudeDegrees: 55, LongitudeDegrees: 12}}}}}
	terminal := proto.Clone(airport).(*pb.NavData)
	terminal.Fragment = &pb.NavData_TerminalFragment{TerminalFragment: &pb.NavTerminalFragment{Airport: "EKCH", ConfigVersion: "v1"}}
	return &pb.AiracPage{Fragments: []*pb.NavData{airport, fix, terminal}}
}

func TestAiracCandidateActivatesOnlyCommittedTypedPage(t *testing.T) {
	ctx := context.Background()
	store, _, state := navFixture(t)
	calls := 0
	worker := AiracCandidateWorker{State: state, Worker: ExternalCallWorker{Writer: state.Writer}, Fetch: func(_ context.Context, airport string, _ *pb.ProviderCheckpoint, _ *pb.ProviderPage) (*pb.AiracPage, *pb.ProviderCheckpoint, error) {
		calls++
		return airacTestPage("2609"), &pb.ProviderCheckpoint{Etag: "e1"}, nil
	}}
	deadline := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	if sent, err := worker.Import(ctx, "EKCH", deadline); err != nil || !sent || calls != 1 {
		t.Fatalf("import: %v %v calls=%d", sent, err, calls)
	}
	manifest, err := state.ActiveManifest(ctx, "EKCH")
	if err != nil || manifest == nil || manifest.Cycle != "2609" || len(manifest.Objects) != 3 || manifest.SourceRevision == 0 || manifest.SourceSha256 == "" {
		t.Fatalf("active manifest: %v %v", manifest, err)
	}
	before := store.commits
	if sent, err := worker.Import(ctx, "EKCH", deadline); err != nil || sent || calls != 1 || store.commits != before {
		t.Fatalf("replay: %v %v calls=%d", sent, err, calls)
	}
	if err := worker.Resume(ctx, "EKCH"); err != nil || store.commits != before {
		t.Fatalf("resume duplicated activation: %v", err)
	}
}
