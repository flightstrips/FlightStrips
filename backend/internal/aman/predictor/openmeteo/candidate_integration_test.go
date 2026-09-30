package openmeteo

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"FlightStrips/internal/aman/predictor"
	"FlightStrips/internal/cluster"
	"FlightStrips/internal/natsresources"
	"FlightStrips/internal/testing/natscluster"
	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
)

func TestOpenMeteoTwoReplicaQuotaAndUncertainNATS(t *testing.T) {
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
	ref := &pb.AggregateRef{Target: &pb.AggregateRef_Airport{Airport: &pb.AirportRef{Icao: "EKCH"}}}
	global := &pb.AggregateRef{Target: &pb.AggregateRef_Global{Global: &pb.GlobalRef{}}}
	type replica struct {
		nc    *nats.Conn
		owner *cluster.OwnerRuntime
		state cluster.NavigationWeather
		stop  context.CancelFunc
	}
	var nodes [2]replica
	for i := range nodes {
		nc, err := natsresources.Connect(cfg)
		if err != nil {
			t.Fatal(err)
		}
		projection, err := cluster.NewProjection(nc, cfg)
		if err != nil {
			t.Fatal(err)
		}
		runCtx, stop := context.WithCancel(ctx)
		go func() { _ = projection.Run(runCtx) }()
		for projection.Ready() != nil && ctx.Err() == nil {
			time.Sleep(25 * time.Millisecond)
		}
		if ctx.Err() != nil {
			t.Fatal("projection unavailable")
		}
		store := cluster.NATSStore{JS: projection.JS}
		owner, err := cluster.NewOwnerRuntime(nc, projection, store)
		if err != nil {
			t.Fatal(err)
		}
		for _, target := range []*pb.AggregateRef{global, ref} {
			if err := owner.Track(target); err != nil {
				t.Fatal(err)
			}
		}
		objects, err := projection.JS.ObjectStore("FS_OBJECTS")
		if err != nil {
			t.Fatal(err)
		}
		nodes[i] = replica{nc, owner, cluster.NavigationWeather{Writer: cluster.Writer{Store: store, NodeID: owner.NodeID, Projection: projection, Lease: owner}, Objects: cluster.NATSObjects{Store: objects}}, stop}
		go func() { _ = owner.Run(runCtx) }()
	}
	defer func() {
		for _, node := range nodes {
			node.stop()
			node.nc.Close()
		}
	}()
	waitOwner := func(target *pb.AggregateRef, exclude string) int {
		t.Helper()
		for ctx.Err() == nil {
			for i := range nodes {
				if nodes[i].owner.NodeID != exclude && nodes[i].owner.CanWrite(target) {
					return i
				}
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Fatal("owner handoff timed out")
		return -1
	}
	first := waitOwner(ref, "")
	second := 1 - first
	window := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	quotaWindow := time.Now().UTC().Add(time.Duration(uuid.New()[0]) * time.Second)
	reserve := func(ctx context.Context, id string) (bool, error) {
		owner := waitOwner(global, "")
		return nodes[owner].state.ReserveQuota(ctx, id, "openmeteo", quotaWindow, 3)
	}
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 2 {
			nodes[first].stop()
			nodes[first].nc.Close()
		}
		_, _ = w.Write([]byte(gfsPayload()))
	}))
	defer server.Close()
	providerNow := window
	provider := New(Config{BaseURL: server.URL, Client: server.Client(), Now: func() time.Time { return providerNow }})
	nonce := uuid.New()
	jitter := float64(uint32(nonce[0])<<16|uint32(nonce[1])<<8|uint32(nonce[2])) / 1e10
	request := predictor.WindProfileRequest{Samples: []predictor.WindSampleRequest{{Position: predictor.WindCoordinate{LatitudeDegrees: 55 + jitter, LongitudeDegrees: 12}, At: window, AltitudeFeet: 10000}}}
	makeCandidate := func(i int) Candidate {
		return Candidate{Provider: provider, State: nodes[i].state, Worker: cluster.ExternalCallWorker{Writer: nodes[i].state.Writer}}
	}
	deadline := window
	if sent, _, err := makeCandidate(second).Fetch(ctx, "EKCH", request, deadline, reserve); err == nil || sent || calls.Load() != 0 {
		t.Fatalf("nonowner fetch: %v %v calls=%d", sent, err, calls.Load())
	}
	sent, resource, err := makeCandidate(first).Fetch(ctx, "EKCH", request, deadline, reserve)
	if err != nil || !sent || calls.Load() != 1 {
		t.Fatalf("owner fetch: %v %v calls=%d", sent, err, calls.Load())
	}
	profile, err := makeCandidate(second).Profile(ctx, "EKCH", resource)
	if err != nil || len(profile.Samples) != 1 || len(profile.Samples[0].Levels) != 8 {
		t.Fatalf("typed checkpoint: %+v %v", profile, err)
	}
	providerNow = providerNow.Add(31 * time.Minute)
	if sent, _, err := makeCandidate(first).Fetch(ctx, "EKCH", request, deadline.Add(time.Minute), reserve); err == nil || !sent || calls.Load() != 2 {
		t.Fatalf("post-call failure: %v %v calls=%d", sent, err, calls.Load())
	}
	if owner := waitOwner(ref, nodes[first].owner.NodeID); owner != second {
		t.Fatal("wrong airport takeover")
	}
	if err := (cluster.ExternalCallWorker{Writer: nodes[second].state.Writer}).Resume(ctx, ref); err != nil {
		t.Fatal(err)
	}
	if sent, _, err := makeCandidate(second).Fetch(ctx, "EKCH", request, deadline.Add(time.Minute), reserve); err != nil || sent || calls.Load() != 2 {
		t.Fatalf("uncertain request repeated: %v %v calls=%d", sent, err, calls.Load())
	}
	globalOwner := waitOwner(global, "")
	quota, err := nodes[globalOwner].state.Quota(ctx, "openmeteo", quotaWindow)
	if err != nil || quota == nil || quota.Used != 2 {
		t.Fatalf("durable quota: %v %v", quota, err)
	}
}
