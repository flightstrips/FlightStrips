package cluster

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"FlightStrips/internal/natsresources"
	"FlightStrips/internal/testing/natscluster"
	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
)

func TestAiracCandidateTwoReplicaManifestAndUncertainCallNATS(t *testing.T) {
	if os.Getenv("NATS_INTEGRATION") != "1" {
		t.Skip("requires pinned three-node NATS fixture")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
	defer cancel()
	port := 4222
	if raw := os.Getenv("NATS_TEST_PORT_BASE"); raw != "" {
		var err error
		port, err = strconv.Atoi(raw)
		if err != nil || port < 1024 || port > 65533 {
			t.Fatalf("invalid NATS_TEST_PORT_BASE %q", raw)
		}
	}
	url := func(user string, offset int) string {
		return fmt.Sprintf("nats://%s:%s-local-only@127.0.0.1:%d", user, user, port+offset)
	}
	cfg := natsresources.Config{URLs: []string{url("bootstrap", 0), url("bootstrap", 1), url("bootstrap", 2)}, ConnectTimeout: 3 * time.Second, RequestTimeout: 3 * time.Second, Names: natsresources.RequiredNames}
	admin, err := natsresources.Connect(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	if err := natscluster.WaitForQuorum(ctx, admin); err != nil {
		t.Fatal(err)
	}
	if err := natsresources.Bootstrap(ctx, admin, cfg); err != nil {
		t.Fatal(err)
	}
	cfg.URLs = []string{url("backend", 0), url("backend", 1), url("backend", 2)}
	seed := uuid.New()
	icao := string([]byte{'A' + seed[0]%26, 'A' + seed[1]%26, 'A' + seed[2]%26, 'A' + seed[3]%26})
	ref := airportRef(icao)
	type replica struct {
		nc    *nats.Conn
		owner *OwnerRuntime
		state NavigationWeather
		stop  context.CancelFunc
	}
	var nodes [2]replica
	for i := range nodes {
		nc, err := natsresources.Connect(cfg)
		if err != nil {
			t.Fatal(err)
		}
		projection := startProjection(t, ctx, nc, cfg)
		store := NATSStore{JS: projection.JS}
		owner, err := NewOwnerRuntime(nc, projection, store)
		if err != nil {
			t.Fatal(err)
		}
		if err := owner.Track(ref); err != nil {
			t.Fatal(err)
		}
		objects, err := projection.JS.ObjectStore("FS_OBJECTS")
		if err != nil {
			t.Fatal(err)
		}
		runCtx, stop := context.WithCancel(ctx)
		nodes[i] = replica{nc, owner, NavigationWeather{Writer: Writer{Store: store, NodeID: owner.NodeID, Projection: projection, Lease: owner}, Objects: NATSObjects{Store: objects}}, stop}
		go func() { _ = owner.Run(runCtx) }()
	}
	defer func() {
		for _, node := range nodes {
			node.stop()
			node.nc.Close()
		}
	}()
	waitOwner := func(exclude string) int {
		t.Helper()
		for ctx.Err() == nil {
			for i := range nodes {
				if nodes[i].owner.NodeID != exclude && nodes[i].owner.CanWrite(ref) {
					return i
				}
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Fatal("AIRAC owner handoff timed out")
		return -1
	}
	first := waitOwner("")
	second := 1 - first
	makePage := func(cycle string) *pb.AiracPage {
		page := airacTestPage(cycle)
		for _, fragment := range page.Fragments {
			fragment.Airport = icao
			if airport := fragment.GetAirportFragment(); airport != nil {
				airport.Airport.Icao = icao
			}
			if terminal := fragment.GetTerminalFragment(); terminal != nil {
				terminal.Airport = icao
			}
		}
		return page
	}
	var calls atomic.Int32
	fetch := func(_ context.Context, _ string, _ *pb.ProviderCheckpoint, _ *pb.ProviderPage) (*pb.AiracPage, *pb.ProviderCheckpoint, error) {
		calls.Add(1)
		return makePage("2609"), &pb.ProviderCheckpoint{Etag: "cycle-2609"}, nil
	}
	makeWorker := func(i int, callback func(context.Context, string, *pb.ProviderCheckpoint, *pb.ProviderPage) (*pb.AiracPage, *pb.ProviderCheckpoint, error)) AiracCandidateWorker {
		return AiracCandidateWorker{State: nodes[i].state, Worker: ExternalCallWorker{Writer: nodes[i].state.Writer}, Fetch: callback}
	}
	deadline := time.Now().UTC()
	if sent, err := makeWorker(second, fetch).Import(ctx, icao, deadline); err == nil || sent || calls.Load() != 0 {
		t.Fatalf("nonowner import: %v %v calls=%d", sent, err, calls.Load())
	}
	if sent, err := makeWorker(first, fetch).Import(ctx, icao, deadline); err != nil || !sent || calls.Load() != 1 {
		t.Fatalf("owner import: %v %v calls=%d", sent, err, calls.Load())
	}
	waitManifest := func(cycle string) (*pb.NavManifest, error) {
		for ctx.Err() == nil {
			value, err := nodes[second].state.ActiveManifest(ctx, icao)
			if err == nil && value != nil && value.Cycle == cycle {
				return value, nil
			}
			time.Sleep(50 * time.Millisecond)
		}
		return nil, ctx.Err()
	}
	manifest, err := waitManifest("2609")
	if err != nil || manifest == nil || manifest.Cycle != "2609" || len(manifest.Objects) != 3 {
		t.Fatalf("verified manifest: %v %v", manifest, err)
	}
	first = waitOwner("")
	second = 1 - first
	uncertainFetch := func(_ context.Context, _ string, _ *pb.ProviderCheckpoint, _ *pb.ProviderPage) (*pb.AiracPage, *pb.ProviderCheckpoint, error) {
		calls.Add(1)
		nodes[first].stop()
		nodes[first].nc.Close()
		return makePage("2610"), &pb.ProviderCheckpoint{Etag: "cycle-2610"}, nil
	}
	if sent, err := makeWorker(first, uncertainFetch).Import(ctx, icao, deadline.Add(time.Second)); !sent || err == nil || calls.Load() != 2 {
		t.Fatalf("post-call uncertainty: %v %v calls=%d", sent, err, calls.Load())
	}
	if owner := waitOwner(nodes[first].owner.NodeID); owner != second {
		t.Fatal("wrong AIRAC takeover")
	}
	if err := makeWorker(second, fetch).Resume(ctx, icao); err != nil {
		t.Fatal(err)
	}
	if sent, err := makeWorker(second, uncertainFetch).Import(ctx, icao, deadline.Add(time.Second)); err != nil || sent || calls.Load() != 2 {
		t.Fatalf("uncertain import repeated: %v %v calls=%d", sent, err, calls.Load())
	}
	manifest, err = nodes[second].state.ActiveManifest(ctx, icao)
	if err != nil || manifest.Cycle != "2609" {
		t.Fatalf("uncertain cycle activated: %v %v", manifest, err)
	}
	lastFetch := func(_ context.Context, _ string, _ *pb.ProviderCheckpoint, _ *pb.ProviderPage) (*pb.AiracPage, *pb.ProviderCheckpoint, error) {
		calls.Add(1)
		return makePage("2610"), &pb.ProviderCheckpoint{Etag: "cycle-2610"}, nil
	}
	if sent, err := makeWorker(second, lastFetch).Import(ctx, icao, deadline.Add(2*time.Second)); err != nil || !sent || calls.Load() != 3 {
		t.Fatalf("new source import: %v %v calls=%d", sent, err, calls.Load())
	}
	manifest, err = waitManifest("2610")
	if err != nil || manifest.Cycle != "2610" || manifest.SourceRevision < 2 {
		t.Fatalf("new cycle not active: %v %v", manifest, err)
	}
	stale := proto.Clone(manifest).(*pb.NavManifest)
	stale.SourceRevision = 1
	stale.SourceSha256 = fmt.Sprintf("%064x", 1)
	stale.Digest = manifestDigest(stale)
	if _, err := nodes[second].state.ActivateManifest(ctx, uuid.NewString(), stale); err == nil {
		t.Fatal("older source replaced active manifest")
	}
}
