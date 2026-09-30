package metar

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
	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
)

func TestCandidateMetarTwoReplicaNATSQuotaAndUncertainty(t *testing.T) {
	if os.Getenv("NATS_INTEGRATION") != "1" {
		t.Skip("requires pinned three-node NATS fixture")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
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
	cfg := natsresources.Config{URLs: []string{url("bootstrap", 0), url("bootstrap", 1), url("bootstrap", 2)},
		ConnectTimeout: 3 * time.Second, RequestTimeout: 3 * time.Second, Names: natsresources.RequiredNames}
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
	random := uuid.New()
	airport := string([]byte{'A' + random[0]%26, 'A' + random[1]%26, 'A' + random[2]%26, 'A' + random[3]%26})
	airportRef := &pb.AggregateRef{Target: &pb.AggregateRef_Airport{Airport: &pb.AirportRef{Icao: airport}}}
	globalRef := &pb.AggregateRef{Target: &pb.AggregateRef_Global{Global: &pb.GlobalRef{}}}
	type replica struct {
		nc         *nats.Conn
		projection *cluster.Projection
		owner      *cluster.OwnerRuntime
		state      cluster.NavigationWeather
		stop       context.CancelFunc
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
			t.Fatal("projection did not become ready")
		}
		store := cluster.NATSStore{JS: projection.JS}
		objects, err := projection.JS.ObjectStore("FS_OBJECTS")
		if err != nil {
			t.Fatal(err)
		}
		owner, err := cluster.NewOwnerRuntime(nc, projection, store)
		if err != nil {
			t.Fatal(err)
		}
		for _, ref := range []*pb.AggregateRef{globalRef, airportRef} {
			if err := owner.Track(ref); err != nil {
				t.Fatal(err)
			}
		}
		nodes[i] = replica{nc: nc, projection: projection, owner: owner,
			state: cluster.NavigationWeather{Writer: cluster.Writer{Store: store, NodeID: owner.NodeID, Projection: projection, Lease: owner}, Objects: cluster.NATSObjects{Store: objects}}, stop: stop}
		go func() { _ = owner.Run(runCtx) }()
	}
	defer func() {
		for _, node := range nodes {
			node.stop()
			node.nc.Close()
		}
	}()
	waitOwner := func(ref *pb.AggregateRef, excluded string) int {
		t.Helper()
		for ctx.Err() == nil {
			for i := range nodes {
				if nodes[i].owner.NodeID != excluded && nodes[i].owner.CanWrite(ref) {
					return i
				}
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Fatalf("owner handoff timed out: %v", ref)
		return -1
	}
	globalOwner := waitOwner(globalRef, "")
	first := waitOwner(airportRef, "")
	second := 1 - first
	var hits atomic.Int32
	var atisHits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/atis" {
			atisHits.Add(1)
			_, _ = w.Write([]byte(fmt.Sprintf(`[{"callsign":"%s_ATIS","atis_code":"A","text_atis":["INFO ALPHA"],"last_updated":"2026-09-30T12:00:00Z"}]`, airport)))
			return
		}
		if r.URL.Path != "/"+airport {
			http.NotFound(w, r)
			return
		}
		if hits.Add(1) == 2 {
			nodes[first].stop()
			nodes[first].nc.Close()
		}
		_, _ = w.Write([]byte(airport + " 301200Z 27005KT CAVOK"))
	}))
	defer server.Close()
	provider := NewCandidateProvider(server.Client(), server.URL, server.URL+"/atis")
	makeCandidate := func(i int) Candidate {
		return Candidate{Provider: provider, State: nodes[i].state,
			AirportWorker: cluster.ExternalCallWorker{Writer: nodes[i].state.Writer}, GlobalWorker: cluster.ExternalCallWorker{Writer: nodes[i].state.Writer},
			Session: cluster.AtisSessionAdapter{Writer: nodes[i].state.Writer}}
	}
	window := time.Now().UTC().Truncate(time.Second).Add(time.Duration(random[4]) * time.Second)
	reserve := func(at time.Time) func(context.Context, string) (bool, error) {
		return func(ctx context.Context, id string) (bool, error) {
			return nodes[globalOwner].state.ReserveQuota(ctx, id, "metar", at, 1)
		}
	}
	if sent, err := makeCandidate(second).FetchMetar(ctx, airport, window, time.Minute, reserve(window)); err == nil || sent || hits.Load() != 0 {
		t.Fatalf("nonowner METAR request: %v %v hits=%d", sent, err, hits.Load())
	}
	if sent, err := makeCandidate(first).FetchMetar(ctx, airport, window, time.Minute, reserve(window)); err != nil || !sent || hits.Load() != 1 {
		t.Fatalf("owner METAR request: %v %v hits=%d", sent, err, hits.Load())
	}
	quota, err := nodes[globalOwner].state.Quota(ctx, "metar", window)
	if err != nil || quota == nil || quota.Used != 1 {
		t.Fatalf("first quota: %v %v", quota, err)
	}
	if sent, err := makeCandidate(1-globalOwner).FetchAtisFeed(ctx, window); err == nil || sent || atisHits.Load() != 0 {
		t.Fatalf("nonowner ATIS fetch: %v %v hits=%d", sent, err, atisHits.Load())
	}
	if sent, err := makeCandidate(globalOwner).FetchAtisFeed(ctx, window); err != nil || !sent || atisHits.Load() != 1 {
		t.Fatalf("global ATIS fetch: %v %v hits=%d", sent, err, atisHits.Load())
	}
	_, typedAtis, _, err := nodes[1-globalOwner].state.CheckpointRevisionFor(ctx, globalRef, "afv-atis", "feed")
	if err != nil || typedAtis == nil || len(typedAtis.GetAtisFeed().Airports) != 1 || typedAtis.GetAtisFeed().Airports[0].Arrival.Code != "A" {
		t.Fatalf("typed ATIS page: %v %v", typedAtis, err)
	}
	sessionID := int32(200000 + int(random[5])*1000 + int(random[6]))
	sessionRef := &pb.AggregateRef{Target: &pb.AggregateRef_Session{Session: &pb.SessionRef{Id: sessionID}}}
	if err := nodes[first].owner.Track(sessionRef); err != nil {
		t.Fatal(err)
	}
	for !nodes[first].owner.CanWrite(sessionRef) && ctx.Err() == nil {
		time.Sleep(50 * time.Millisecond)
	}
	if ctx.Err() != nil {
		t.Fatal("session owner did not become writable")
	}
	if err := nodes[second].owner.Track(sessionRef); err != nil {
		t.Fatal(err)
	}
	seedWriter := nodes[first].state.Writer
	seedWriter.Plan = func(context.Context, *pb.CommandRequest, *cluster.Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
		return &pb.DomainChange{Changes: []*pb.EntityChange{{Key: strconv.FormatInt(int64(sessionID), 10), Revision: 1,
			Operation: &pb.EntityChange_Upsert{Upsert: &pb.EntityRecord{Value: &pb.EntityRecord_Session{Session: &pb.Session{Id: sessionID, Airport: airport, Name: "LIVE"}}}}}}}, pb.CommandReply_COMMITTED, 0, nil
	}
	seed := &pb.CommandRequest{ProtocolRevision: 1, CommandId: uuid.NewString(), Aggregate: sessionRef,
		Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "atis-test"}, Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: &pb.UpdateEntity{Key: strconv.FormatInt(int64(sessionID), 10), Value: &pb.EntityRecord{Value: &pb.EntityRecord_Session{Session: &pb.Session{Id: sessionID, Airport: airport, Name: "LIVE"}}}}}}}}
	if reply := seedWriter.Execute(ctx, seed); reply.GetOutcome().GetStatus() != pb.CommandOutcome_SUCCEEDED {
		t.Fatalf("seed session: %v", reply)
	}
	applyTime := time.Now().UTC()
	if err := makeCandidate(first).ApplySession(ctx, sessionID, airport, applyTime); err != nil {
		t.Fatal(err)
	}
	var initialSession *cluster.Aggregate
	for ctx.Err() == nil {
		initialSession, err = nodes[second].projection.Read(sessionRef)
		if err == nil && initialSession.Indexes[pb.EntityKind_ATIS][airport] != nil {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if err != nil || initialSession == nil || initialSession.Indexes[pb.EntityKind_ATIS][airport].GetValue().GetAtis().ArrivalCode != "A" {
		t.Fatalf("ATIS presentation on second replica: %v %v", initialSession, err)
	}
	secondWindow := window.Add(time.Minute)
	if sent, err := makeCandidate(first).FetchMetar(ctx, airport, secondWindow, time.Minute, reserve(secondWindow)); !sent || err == nil || hits.Load() != 2 {
		t.Fatalf("post-call failure: %v %v hits=%d", sent, err, hits.Load())
	}
	if globalOwner == first {
		globalOwner = second
	}
	_ = waitOwner(globalRef, nodes[first].owner.NodeID)
	_ = waitOwner(airportRef, nodes[first].owner.NodeID)
	_ = waitOwner(sessionRef, nodes[first].owner.NodeID)
	if err := makeCandidate(second).ApplySession(ctx, sessionID, airport, applyTime); err != nil {
		t.Fatalf("ATIS takeover application: %v", err)
	}
	replayedSession, err := nodes[second].projection.Read(sessionRef)
	if err != nil || replayedSession.Indexes[pb.EntityKind_ATIS][airport].Revision != initialSession.Indexes[pb.EntityKind_ATIS][airport].Revision {
		t.Fatalf("ATIS duplicate after takeover: %v %v", replayedSession, err)
	}
	if err := (cluster.ExternalCallWorker{Writer: nodes[second].state.Writer}).Resume(ctx, airportRef); err != nil {
		t.Fatal(err)
	}
	if sent, err := makeCandidate(second).FetchMetar(ctx, airport, secondWindow, time.Minute, reserve(secondWindow)); err != nil || sent || hits.Load() != 2 {
		t.Fatalf("uncertain METAR refetched: %v %v hits=%d", sent, err, hits.Load())
	}
	quota, err = nodes[globalOwner].state.Quota(ctx, "metar", secondWindow)
	if err != nil || quota == nil || quota.Used != 1 {
		t.Fatalf("uncertain quota reservation: %v %v", quota, err)
	}
	if sent, err := makeCandidate(globalOwner).FetchAtisFeed(ctx, window); err != nil || sent || atisHits.Load() != 1 {
		t.Fatalf("takeover repeated ATIS request: %v %v hits=%d", sent, err, atisHits.Load())
	}
}
