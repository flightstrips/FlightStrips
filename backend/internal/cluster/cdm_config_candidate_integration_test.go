package cluster

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"FlightStrips/internal/cdm"
	"FlightStrips/internal/natsresources"
	"FlightStrips/internal/testing/natscluster"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
)

func TestCdmConfigTwoReplicaNATS(t *testing.T) {
	if os.Getenv("NATS_INTEGRATION") != "1" {
		t.Skip("requires pinned three-node NATS fixture")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
	defer cancel()
	port := 4222
	if raw := os.Getenv("NATS_TEST_PORT_BASE"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1024 || parsed > 65533 {
			t.Fatalf("invalid NATS_TEST_PORT_BASE %q", raw)
		}
		port = parsed
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
		t.Fatal("CDM configuration owner handoff timed out")
		return -1
	}
	first := waitOwner("")
	second := 1 - first
	deadline := time.Now().UTC()
	calls := 0
	fetch := func(_ context.Context, airport string) (*cdm.CdmAirportConfig, error) {
		calls++
		return &cdm.CdmAirportConfig{Airport: airport, DefaultRate: 20, DefaultRateLvo: 14, DefaultTaxiMinutes: 10}, nil
	}
	worker := func(i int, callback func(context.Context, string) (*cdm.CdmAirportConfig, error)) CdmConfigCandidateWorker {
		return CdmConfigCandidateWorker{State: nodes[i].state, Worker: ExternalCallWorker{Writer: nodes[i].state.Writer}, Fetch: callback}
	}
	if sent, err := worker(second, fetch).Refresh(ctx, icao, deadline); sent || err == nil || calls != 0 {
		t.Fatalf("nonowner CDM fetch: %v %v calls=%d", sent, err, calls)
	}
	if sent, err := worker(first, fetch).Refresh(ctx, icao, deadline); !sent || err != nil || calls != 1 {
		t.Fatalf("owner CDM fetch: %v %v calls=%d", sent, err, calls)
	}
	for ctx.Err() == nil {
		page, revision, err := worker(second, fetch).Read(ctx, icao)
		if err == nil && page != nil && revision == 1 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if ctx.Err() != nil {
		t.Fatal(ctx.Err())
	}
	first = waitOwner("")
	second = 1 - first
	uncertain := func(_ context.Context, airport string) (*cdm.CdmAirportConfig, error) {
		calls++
		nodes[first].stop()
		nodes[first].nc.Close()
		return &cdm.CdmAirportConfig{Airport: airport, DefaultRate: 18, DefaultRateLvo: 12, DefaultTaxiMinutes: 10}, nil
	}
	if sent, err := worker(first, uncertain).Refresh(ctx, icao, deadline.Add(time.Second)); !sent || err == nil || calls != 2 {
		t.Fatalf("post-call owner death: %v %v calls=%d", sent, err, calls)
	}
	if waitOwner(nodes[first].owner.NodeID) != second {
		t.Fatal("wrong CDM configuration takeover")
	}
	if err := worker(second, fetch).Resume(ctx, icao); err != nil {
		t.Fatal(err)
	}
	if sent, err := worker(second, uncertain).Refresh(ctx, icao, deadline.Add(time.Second)); sent || err != nil || calls != 2 {
		t.Fatalf("uncertain CDM fetch repeated: %v %v calls=%d", sent, err, calls)
	}
	page, revision, err := worker(second, fetch).Read(ctx, icao)
	if err != nil || page.DefaultRate != 20 || revision != 1 {
		t.Fatalf("uncertain CDM candidate published: %v revision=%d err=%v", page, revision, err)
	}
}
