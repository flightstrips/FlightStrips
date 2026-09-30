package vatsimcandidate

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

	"FlightStrips/internal/cluster"
	"FlightStrips/internal/natsresources"
	"FlightStrips/internal/testing/natscluster"
	"FlightStrips/internal/vatsim"
	pb "FlightStrips/pkg/events/cluster"
	"github.com/nats-io/nats.go"
)

func TestVatsimFetchTwoReplicaNATS(t *testing.T) {
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
	ref := &pb.AggregateRef{Target: &pb.AggregateRef_Global{Global: &pb.GlobalRef{}}}
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
			t.Fatal("projection not ready")
		}
		store := cluster.NATSStore{JS: projection.JS}
		owner, err := cluster.NewOwnerRuntime(nc, projection, store)
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
		nodes[i] = replica{nc: nc, owner: owner, state: cluster.NavigationWeather{Writer: cluster.Writer{Store: store, NodeID: owner.NodeID, Projection: projection, Lease: owner}, Objects: cluster.NATSObjects{Store: objects}}, stop: stop}
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
		t.Fatal("VATSIM owner handoff timed out")
		return -1
	}
	first := waitOwner("")
	second := 1 - first
	var dataCalls atomic.Int32
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/status" {
			_, _ = fmt.Fprintf(w, `{"data":{"v3":[%q]}}`, serverURL+"/data")
			return
		}
		if r.URL.Path != "/data" {
			http.NotFound(w, r)
			return
		}
		if dataCalls.Add(1) == 2 {
			nodes[first].stop()
			nodes[first].nc.Close()
		}
		_, _ = w.Write([]byte(`{"general":{"update_timestamp":"2026-09-30T12:00:00Z"},"pilots":[{"cid":123456,"callsign":"SAS123","flight_plan":{"departure":"EKCH","arrival":"EDDF","revision_id":9}}],"prefiles":[]}`))
	}))
	defer server.Close()
	serverURL = server.URL
	cache := vatsim.NewCache(server.URL+"/status", 0, server.Client())
	makeFetch := func(i int) Fetch {
		return Fetch{Cache: cache, State: nodes[i].state, Worker: cluster.ExternalCallWorker{Writer: nodes[i].state.Writer}}
	}
	deadline := time.Now().UTC().Truncate(time.Millisecond)
	if sent, err := makeFetch(second).Global(ctx, deadline); err == nil || sent || dataCalls.Load() != 0 {
		t.Fatalf("nonowner VATSIM fetch: %v %v calls=%d", sent, err, dataCalls.Load())
	}
	if sent, err := makeFetch(first).Global(ctx, deadline); err != nil || !sent || dataCalls.Load() != 1 {
		t.Fatalf("global VATSIM fetch: %v %v calls=%d", sent, err, dataCalls.Load())
	}
	_, page, _, err := nodes[second].state.CheckpointRevisionFor(ctx, ref, "vatsim", "network-data/v3")
	if err != nil || page == nil || len(page.GetVatsim().Flights) != 1 || page.GetVatsim().Flights[0].FlightPlan.Revision != 9 {
		t.Fatalf("typed VATSIM checkpoint: %v %v", page, err)
	}
	if sent, err := makeFetch(first).Global(ctx, deadline.Add(time.Second)); !sent || err == nil || dataCalls.Load() != 2 {
		t.Fatalf("post-call owner failure: %v %v calls=%d", sent, err, dataCalls.Load())
	}
	_ = waitOwner(nodes[first].owner.NodeID)
	if err := (cluster.ExternalCallWorker{Writer: nodes[second].state.Writer}).Resume(ctx, ref); err != nil {
		t.Fatal(err)
	}
	if sent, err := makeFetch(second).Global(ctx, deadline.Add(time.Second)); err != nil || sent || dataCalls.Load() != 2 {
		t.Fatalf("uncertain VATSIM refetch: %v %v calls=%d", sent, err, dataCalls.Load())
	}
}
