package euroscopebinary

import (
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"FlightStrips/internal/cluster"
	"FlightStrips/internal/config"
	"FlightStrips/internal/natsresources"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testing/natscluster"
	pb "FlightStrips/pkg/events/cluster"
	es "FlightStrips/pkg/events/euroscope"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type deadlineAuth struct{}

func (deadlineAuth) Validate(cid string) (shared.AuthenticatedUser, error) {
	return shared.NewAuthenticatedUser(cid, 1, &jwt.Token{Claims: jwt.MapClaims{"exp": float64(time.Now().Add(time.Hour).Unix())}}), nil
}

type admittedSession struct {
	projection *cluster.Projection
	id         int32
}

func (s admittedSession) GetOrCreateSession(ctx context.Context, airport, name string) (*pb.Session, error) {
	state, err := s.projection.Read(candidateRef(s.id))
	if err != nil {
		return nil, err
	}
	return state.Indexes[pb.EntityKind_SESSION][fmt.Sprint(s.id)].Value.GetSession(), nil
}

type deadlineNode struct {
	nc      *nats.Conn
	p       *cluster.Projection
	owner   *cluster.OwnerRuntime
	router  *cluster.CommandRouter
	c       *DeadlineCandidate
	fanout  *cluster.SessionFanout
	effects cluster.Effects
	work    cluster.SessionWork
	server  *httptest.Server
	stop    context.CancelFunc
	dead    bool
}
type deadlineHarness struct {
	t       *testing.T
	ctx     context.Context
	cfg     natsresources.Config
	id      int32
	nodes   [2]*deadlineNode
	sockets []*deadlineSocket
}
type deadlineSocket struct {
	conn   *websocket.Conn
	cid    string
	node   int
	mu     sync.Mutex
	frames []*es.Envelope
	err    error
}

func (s *deadlineSocket) send(t *testing.T, frame *es.Envelope) {
	t.Helper()
	data, err := proto.Marshal(frame)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
		t.Fatal(err)
	}
}
func (s *deadlineSocket) generated() []*es.Envelope {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []*es.Envelope{}
	for _, f := range s.frames {
		if f.GetGenerateSquawk() != nil {
			out = append(out, f)
		}
	}
	return out
}

func newDeadlineHarness(t *testing.T) *deadlineHarness {
	t.Helper()
	if os.Getenv("NATS_INTEGRATION") != "1" {
		t.Skip("requires pinned three-node NATS fixture")
	}
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chdir(filepath.Join(root, "../..")); err != nil {
		t.Fatal(err)
	}
	err = config.InitConfig()
	_ = os.Chdir(root)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	t.Cleanup(cancel)
	port := 4222
	if raw := os.Getenv("NATS_TEST_PORT_BASE"); raw != "" {
		port, err = strconv.Atoi(raw)
		if err != nil {
			t.Fatal(err)
		}
	}
	urls := func(user string) []string {
		out := []string{}
		for i := 0; i < 3; i++ {
			out = append(out, fmt.Sprintf("nats://%s:%s-local-only@127.0.0.1:%d", user, user, port+i))
		}
		return out
	}
	cfg := natsresources.Config{URLs: urls("bootstrap"), ConnectTimeout: 3 * time.Second, RequestTimeout: 3 * time.Second, Names: natsresources.RequiredNames}
	admin, err := natsresources.Connect(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	if err = natscluster.WaitForQuorum(ctx, admin); err != nil {
		t.Fatal(err)
	}
	if err = natsresources.Bootstrap(ctx, admin, cfg); err != nil {
		t.Fatal(err)
	}
	cfg.URLs = urls("backend")
	h := &deadlineHarness{t: t, ctx: ctx, cfg: cfg, id: int32(2000000 + time.Now().UnixNano()%1000000)}
	for i := range h.nodes {
		nc, err := natsresources.Connect(cfg)
		if err != nil {
			t.Fatal(err)
		}
		p, err := cluster.NewProjection(nc, cfg)
		if err != nil {
			t.Fatal(err)
		}
		runCtx, stop := context.WithCancel(ctx)
		projectionDone := make(chan error, 1)
		go func() { projectionDone <- p.Run(runCtx) }()
		h.await("projection", func() bool {
			select {
			case err := <-projectionDone:
				t.Fatalf("projection failed: %v", err)
			default:
			}
			return p.Ready() == nil
		})
		store := cluster.NATSStore{JS: p.JS}
		owner, err := cluster.NewOwnerRuntime(nc, p, store)
		if err != nil {
			t.Fatal(err)
		}
		_ = owner.Track(globalCandidateRef())
		_ = owner.Track(candidateRef(h.id))
		writer := cluster.Writer{Store: store, NodeID: owner.NodeID, Projection: p, Lease: owner}
		router := &cluster.CommandRouter{NC: nc, Projection: p, Lease: owner, Writer: writer}
		objects, err := p.JS.ObjectStore("FS_OBJECTS")
		if err != nil {
			t.Fatal(err)
		}
		source := cluster.NavigationWeather{Writer: writer, Objects: cluster.NATSObjects{Store: objects}}
		base := cluster.MasterElectionPlanner(p, cluster.SessionLifecyclePlanner(func(_ context.Context, ref *pb.AggregateRef) (*cluster.Aggregate, error) { return p.Read(ref) }))
		c, err := NewDeadlineCandidate(router, source, base)
		if err != nil {
			t.Fatal(err)
		}
		router.Writer.Plan = c.Planner
		fanout := &cluster.SessionFanout{NC: nc, Projection: p, NodeID: owner.NodeID}
		effects := cluster.Effects{Owner: owner, Fanout: fanout}
		adapter := cluster.RoutedLifecycleStore{Router: router, Projection: p}
		work := cluster.SessionWork{Store: adapter, Registry: cluster.SessionRegistry{Store: adapter}, Projection: p, Owner: owner}
		c.BindWorker(&work)
		n := &deadlineNode{nc: nc, p: p, owner: owner, router: router, c: c, fanout: fanout, effects: effects, work: work, stop: stop}
		h.nodes[i] = n
		go func() { _ = owner.Run(runCtx) }()
		go func() { _ = router.Serve(runCtx) }()
		go func() { _ = c.Serve(runCtx) }()
		go func() { _ = fanout.ServeTargeted(runCtx) }()
		go func() { _ = effects.ServeResults(runCtx) }()
	}
	t.Cleanup(func() {
		for _, s := range h.sockets {
			_ = s.conn.Close()
		}
		for _, n := range h.nodes {
			if n.server != nil {
				n.server.CloseClientConnections()
				n.server.Close()
			}
			n.stop()
			n.nc.Close()
			_ = n.c.Close(context.Background())
		}
	})
	owner := h.owner()
	n := h.nodes[owner]
	req := &pb.CommandRequest{ProtocolRevision: 1, CommandId: uuid.NewString(), Aggregate: candidateRef(h.id), Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "session-registry"}, Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_CreateSession{CreateSession: &pb.CreateSession{Id: h.id, Airport: "EKCH", Name: "LIVE", WorkflowId: uuid.NewString()}}}}}
	h.await("session seed", func() bool { return candidateReply(n.router.Route(ctx, req)) == nil })
	h.await("all socket replicas apply admitted seed", func() bool {
		for _, replica := range h.nodes {
			state, err := replica.p.Read(candidateRef(h.id))
			if err != nil || state.Indexes[pb.EntityKind_SESSION][fmt.Sprint(h.id)] == nil {
				return false
			}
		}
		return true
	})
	for _, n := range h.nodes {
		n.server = httptest.NewServer(Handler{Projection: n.p, Fanout: n.fanout, Sessions: admittedSession{n.p, h.id}, Auth: deadlineAuth{}, Sync: cluster.SessionObservations{Store: cluster.RoutedLifecycleStore{Router: n.router, Projection: n.p}}, Controllers: cluster.ControllerSector{Store: cluster.RoutedLifecycleStore{Router: n.router, Projection: n.p}}, Inbound: func(ctx context.Context, id int32, connection, cid string, frame *es.Envelope) error {
			err := n.c.Inbound(ctx, id, connection, cid, frame)
			if err != nil {
				t.Logf("binary admission failed: %v", err)
			}
			return err
		}, Deadlines: n.c, RenderEffect: EffectRenderer(cluster.EffectSecrets{}), Effects: &n.effects})
	}
	return h
}
func (h *deadlineHarness) await(label string, check func() bool) {
	h.t.Helper()
	until := time.Now().Add(25 * time.Second)
	for time.Now().Before(until) && h.ctx.Err() == nil {
		if check() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	h.t.Fatalf("timed out: %s", label)
}
func (h *deadlineHarness) owner() int {
	h.t.Helper()
	owner := -1
	h.await("session owner", func() bool {
		for i, n := range h.nodes {
			if n != nil && !n.dead && n.owner.CanWrite(candidateRef(h.id)) {
				owner = i
				return true
			}
		}
		return false
	})
	return owner
}
func (h *deadlineHarness) state() *cluster.Aggregate {
	h.t.Helper()
	state, err := h.nodes[h.owner()].p.Read(candidateRef(h.id))
	if err != nil {
		h.t.Fatal(err)
	}
	return state
}
func (h *deadlineHarness) socket(index int, cid, callsign, position string) *deadlineSocket {
	h.t.Helper()
	n := h.nodes[index]
	dialer := websocket.Dialer{Subprotocols: []string{Subprotocol}}
	conn, _, err := dialer.Dial("ws"+strings.TrimPrefix(n.server.URL, "http"), nil)
	if err != nil {
		h.t.Fatal(err)
	}
	s := &deadlineSocket{conn: conn, cid: cid, node: index}
	h.sockets = append(h.sockets, s)
	s.send(h.t, &es.Envelope{Event: &es.Envelope_Token{Token: &es.TokenEvent{Token: cid, ProtocolRevision: 2}}})
	s.send(h.t, &es.Envelope{Event: &es.Envelope_Login{Login: &es.LoginEvent{Airport: "EKCH", Connection: "LIVE", Callsign: callsign, Position: position}}})
	go func() {
		for {
			kind, data, err := conn.ReadMessage()
			s.mu.Lock()
			if err != nil {
				s.err = err
				s.mu.Unlock()
				return
			}
			frame := &es.Envelope{}
			if kind != websocket.BinaryMessage || pb.UnmarshalStrict(data, frame) != nil {
				s.err = fmt.Errorf("invalid server frame")
				s.mu.Unlock()
				return
			}
			s.frames = append(s.frames, frame)
			s.mu.Unlock()
		}
	}()
	h.await("socket presence", func() bool {
		_, entries, err := h.nodes[h.owner()].p.ObservationSnapshot(h.id)
		if err != nil {
			return false
		}
		for _, entry := range entries {
			if p := entry.Value.GetClient(); p != nil && p.Cid == cid {
				return true
			}
		}
		return false
	})
	h.elect()
	return s
}
func (h *deadlineHarness) elect() {
	h.t.Helper()
	h.await("master election", func() bool {
		n := h.nodes[h.owner()]
		master, err := (cluster.MasterElection{Projection: n.p, Router: n.router, Lease: n.owner}).Reconcile(h.ctx, h.id)
		return err == nil && master != nil && master.Cid != "" && master.ConnectionId != ""
	})
}
func (h *deadlineHarness) frame(event *es.Envelope) *es.Envelope {
	state := h.state()
	h.await("socket replicas apply current generations", func() bool {
		for _, node := range h.nodes {
			if node.dead {
				continue
			}
			other, err := node.p.Read(candidateRef(h.id))
			if err != nil || other.Owner.GetEpoch() != state.Owner.GetEpoch() || other.Master.GetEpoch() != state.Master.GetEpoch() {
				return false
			}
		}
		return true
	})
	event.SessionId = h.id
	event.OwnerEpoch = state.Owner.Epoch
	event.MasterEpoch = state.Master.Epoch
	return event
}
func (h *deadlineHarness) sync(s *deadlineSocket, strips ...*es.Strip) {
	h.t.Helper()
	h.await("socket is the elected master", func() bool {
		state := h.state()
		if state.Master.GetCid() != s.cid {
			return false
		}
		return h.nodes[s.node].p.RequireMasterInbound(h.id, state.Master.ConnectionId, s.cid, state.Master.Epoch, false) == nil
	})
	s.send(h.t, h.frame(&es.Envelope{Event: &es.Envelope_Sync{Sync: &es.SyncEvent{Strips: strips, Runways: []*es.Runway{{Name: "22L", Arrival: true}, {Name: "22R", Departure: true}}}}}))
	h.await("operational master sync", func() bool {
		sync, err := h.nodes[h.owner()].p.OperationalSync(candidateRef(h.id))
		return err == nil && sync != nil
	})
}
func deadlineStrip(callsign string) *es.Strip {
	return &es.Strip{Callsign: callsign, Origin: "EKCH", Destination: "EGLL", HasFp: true, Runway: "22R", AssignedSquawk: "2101", Stand: "", Position: &es.Position{Lat: 55.62, Lon: 12.65}}
}
func (h *deadlineHarness) expire(key string) {
	h.t.Helper()
	n := h.nodes[h.owner()]
	old := h.state().Indexes[pb.EntityKind_SESSION_DEADLINE][key]
	if old == nil {
		h.t.Fatalf("deadline %s absent", key)
	}
	d := proto.Clone(old.Value.GetSessionDeadline()).(*pb.SessionDeadline)
	d.DueAt = timestamppb.New(time.Now().Add(-time.Millisecond))
	_, err := (cluster.SessionObservations{Store: cluster.RoutedLifecycleStore{Router: n.router, Projection: n.p}}).PutDeadline(h.ctx, h.id, uuid.NewString(), d, old.Revision)
	if err != nil {
		h.t.Fatal(err)
	}
	h.await("real worker consumes recovered deadline", func() bool {
		_ = n.work.ReconcileSession(h.ctx, h.id)
		return h.state().Indexes[pb.EntityKind_SESSION_DEADLINE][key] == nil
	})
}

func TestDeadlineBinarySharedCoverageReconnectAndReconciliation(t *testing.T) {
	h := newDeadlineHarness(t)
	owner := h.owner()
	s := h.socket(1-owner, "410001", "EKDK_FMP", "131.040")
	departure := deadlineStrip("SAS181")
	departure.Stand = "A12"
	h.sync(s, departure)
	other := h.socket(owner, "410002", "EKCH_A_TWR", "118.105")
	n := h.nodes[h.owner()]
	if err := n.c.SessionUpdate(h.ctx, h.id); err != nil {
		t.Fatal(err)
	}
	state := h.state()
	if len(state.Indexes[pb.EntityKind_STRIP]["SAS181"].Value.GetStrip().NextControllers) == 0 {
		t.Fatal("concrete route reconciliation produced no controller CID")
	}
	for _, cid := range state.Indexes[pb.EntityKind_STRIP]["SAS181"].Value.GetStrip().NextControllers {
		if state.Indexes[pb.EntityKind_CONTROLLER][cid] == nil {
			t.Fatal("route stored a frequency instead of CID")
		}
	}
	if len(state.Indexes[pb.EntityKind_SECTOR_OWNER]) == 0 {
		t.Fatal("concrete sector reconciliation produced no ownership")
	}
	if state.Indexes[pb.EntityKind_CONTROLLER][other.cid].Value.GetController().LayoutId == "" {
		t.Fatal("concrete layout reconciliation did not run")
	}
	// EuroScope reports without CIDs remain typed master observations; they
	// participate in coverage without inventing an authenticated identity.
	s.send(t, h.frame(&es.Envelope{Event: &es.Envelope_ControllerOnline{ControllerOnline: &es.ControllerOnlineEvent{Callsign: "EKCH_B_GND", Position: "121.905"}}}))
	h.await("master-observed controller online", func() bool { return h.state().Workflows[cluster.ControllerObservationID(h.id, "EKCH_B_GND")] != nil })
	observed, err := cluster.SharedEuroScopeControllers(n.p, h.state(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, controller := range observed {
		if controller.Callsign == "EKCH_B_GND" {
			found = true
			if controller.Cid != "" {
				t.Fatal("invented CID for network observation")
			}
		}
	}
	if !found {
		t.Fatal("master observation absent from shared coverage")
	}
	s.send(t, h.frame(&es.Envelope{Event: &es.Envelope_ControllerOffline{ControllerOffline: &es.ControllerOfflineEvent{Callsign: "EKCH_B_GND"}}}))
	h.await("reported controller durable offline", func() bool {
		return h.state().Indexes[pb.EntityKind_SESSION_DEADLINE]["controller-offline.EKCH_B_GND"] != nil
	})
	s.send(t, h.frame(&es.Envelope{Event: &es.Envelope_ControllerOnline{ControllerOnline: &es.ControllerOnlineEvent{Callsign: "EKCH_B_GND", Position: "121.905"}}}))
	h.await("new controller report invalidates offline deadline", func() bool {
		return h.state().Indexes[pb.EntityKind_SESSION_DEADLINE]["controller-offline.EKCH_B_GND"] == nil
	})
	_ = other.conn.Close()
	h.await("offline deadline", func() bool {
		_ = n.c.Recover(h.ctx, h.id)
		return h.state().Indexes[pb.EntityKind_SESSION_DEADLINE]["controller-offline.EKCH_A_TWR"] != nil
	})
	d := h.state().Indexes[pb.EntityKind_SESSION_DEADLINE]["controller-offline.EKCH_A_TWR"].Value.GetSessionDeadline()
	if time.Until(d.DueAt.AsTime()) > cluster.ControllerOfflineGrace || time.Until(d.DueAt.AsTime()) < 10*time.Second {
		t.Fatal("controller grace default changed")
	}
	reconnected := h.socket(1-owner, other.cid, "EKCH_A_TWR", "118.105")
	h.await("reconnect cancellation", func() bool {
		_ = n.c.Recover(h.ctx, h.id)
		return h.state().Indexes[pb.EntityKind_SESSION_DEADLINE][d.Id] == nil
	})
	_ = reconnected
	beforeRevision := h.state().Indexes[pb.EntityKind_CONTROLLER][other.cid].Revision
	s.send(t, h.frame(&es.Envelope{Event: &es.Envelope_ControllerOffline{ControllerOffline: &es.ControllerOfflineEvent{Callsign: "EKCH_A_TWR"}}}))
	h.await("offline binary event admitted", func() bool { return h.state().Indexes[pb.EntityKind_CONTROLLER][other.cid].Revision > beforeRevision })
	h.await("cross-node coverage cancels stale offline report", func() bool {
		_ = n.c.Recover(h.ctx, h.id)
		return h.state().Indexes[pb.EntityKind_SESSION_DEADLINE][d.Id] == nil
	})
	if h.state().Indexes[pb.EntityKind_CONTROLLER][other.cid] == nil {
		t.Fatal("another node's live controller was deleted")
	}
}

func TestDeadlineBinaryAircraftRecoveryRetentionAndNewObservation(t *testing.T) {
	h := newDeadlineHarness(t)
	s := h.socket(1-h.owner(), "420001", "EKDK_FMP", "131.040")
	h.sync(s, deadlineStrip("SAS182"))
	s.send(t, h.frame(&es.Envelope{Event: &es.Envelope_AircraftDisconnect{AircraftDisconnect: &es.AircraftDisconnectEvent{Callsign: "SAS182"}}}))
	h.await("shared aircraft deadline", func() bool {
		return h.state().Indexes[pb.EntityKind_SESSION_DEADLINE]["aircraft-disconnect.SAS182"] != nil
	})
	d := h.state().Indexes[pb.EntityKind_SESSION_DEADLINE]["aircraft-disconnect.SAS182"].Value.GetSessionDeadline()
	if time.Until(d.DueAt.AsTime()) < 55*time.Second || time.Until(d.DueAt.AsTime()) > cluster.AircraftDisconnectGrace {
		t.Fatal("aircraft grace default changed")
	}
	s.send(t, h.frame(&es.Envelope{Event: &es.Envelope_StripUpdate{StripUpdate: &es.StripUpdateEvent{Strip: deadlineStrip("SAS182")}}}))
	h.await("new aircraft observation cancels deadline", func() bool { return h.state().Indexes[pb.EntityKind_SESSION_DEADLINE][d.Id] == nil })
	// Simulate death after the KV tombstone and before durable scheduling.
	// A newly constructed concrete adapter recovers accepted shared state.
	n := h.nodes[h.owner()]
	writer, err := n.c.positionWriter(h.ctx, h.id, h.state().Master.ConnectionId)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := writer.QueueDisconnect(h.ctx, "SAS182", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if result := <-receipt; result.Err != nil {
		t.Fatal(result.Err)
	}
	replacement, err := NewDeadlineCandidate(n.router, n.c.Source, n.c.Next)
	if err != nil {
		t.Fatal(err)
	}
	defer replacement.Close(h.ctx)
	h.await("crash recovery schedules tombstone", func() bool {
		return replacement.Recover(h.ctx, h.id) == nil && h.state().Indexes[pb.EntityKind_SESSION_DEADLINE][d.Id] != nil
	})
	// Unavailable accepted VATSIM state must fail closed at the real planner.
	old := h.state()
	if retained, err := n.c.RetainedAircraft(old, "SAS182"); err == nil || retained {
		t.Fatal("missing accepted feed did not fail closed")
	}
	protected := deadlineStrip("SAS188")
	protected.Stand = "A12"
	s.send(t, h.frame(&es.Envelope{Event: &es.Envelope_StripUpdate{StripUpdate: &es.StripUpdateEvent{Strip: protected}}}))
	h.await("protected strip admitted", func() bool { return h.state().Indexes[pb.EntityKind_STRIP]["SAS188"] != nil })
	s.send(t, h.frame(&es.Envelope{Event: &es.Envelope_AircraftDisconnect{AircraftDisconnect: &es.AircraftDisconnectEvent{Callsign: "SAS188"}}}))
	h.await("protected disconnect deadline", func() bool {
		return h.state().Indexes[pb.EntityKind_SESSION_DEADLINE]["aircraft-disconnect.SAS188"] != nil
	})
	h.expire("aircraft-disconnect.SAS188")
	if h.state().Indexes[pb.EntityKind_STRIP]["SAS188"] == nil {
		t.Fatal("shared protected occupancy was deleted")
	}
	h.putVatsim("SAS182")
	h.expire(d.Id)
	state := h.state()
	if state.Indexes[pb.EntityKind_STRIP]["SAS182"] == nil || state.Indexes[pb.EntityKind_STRIP]["SAS182"].Value.GetStrip().EuroscopeObservedAt != nil {
		t.Fatal("accepted VATSIM retention did not preserve and retract EuroScope provenance")
	}
}

func (h *deadlineHarness) putVatsim(callsign string) {
	h.t.Helper()
	globalOwner := -1
	h.await("global owner", func() bool {
		for i, n := range h.nodes {
			if !n.dead && n.owner.CanWrite(globalCandidateRef()) {
				globalOwner = i
				return true
			}
		}
		return false
	})
	n := h.nodes[globalOwner]
	flights := []*pb.VatsimFlight{{Cid: "999", Callsign: "UNRELATED", State: "prefile", FlightPlan: &pb.VatsimFlightPlan{Origin: "EDDF", Destination: "EGLL"}}}
	if callsign != "" {
		flights = append(flights, &pb.VatsimFlight{Cid: "12345", Callsign: callsign, State: "online", FlightPlan: &pb.VatsimFlightPlan{Origin: "EKCH", Destination: "EGLL"}})
	}
	page := &pb.ProviderPage{Provider: "vatsim", Resource: "network-data/v3", Parsed: &pb.ProviderPage_Vatsim{Vatsim: &pb.VatsimPage{SnapshotAt: timestamppb.Now(), Flights: flights}}}
	name, sha, err := n.c.Source.PublishProvider(page)
	if err != nil {
		h.t.Fatal(err)
	}
	reply, err := n.c.Source.PutCheckpointFor(h.ctx, globalCandidateRef(), uuid.NewString(), &pb.ProviderCheckpoint{Provider: "vatsim", Resource: "network-data/v3", ObjectName: name, Sha256: sha})
	if err != nil || candidateReply(reply) != nil {
		h.t.Fatalf("feed checkpoint: %v %v", reply, err)
	}
	h.await("session source acceptance", func() bool {
		owner := h.nodes[h.owner()]
		return candidateReply((cluster.VatsimSessionAdapter{Source: owner.c.Source, Writer: owner.router.Writer}).Reconcile(h.ctx, h.id)) == nil
	})
}

func (h *deadlineHarness) killOwner() int {
	h.t.Helper()
	old := h.owner()
	n := h.nodes[old]
	n.dead = true
	n.stop()
	n.nc.Close()
	next := h.owner()
	if next == old {
		h.t.Fatal("owner did not transfer")
	}
	return next
}

func (h *deadlineHarness) manual(node int, cid, callsign, id string) *pb.CommandReply {
	h.t.Helper()
	revision := h.state().Indexes[pb.EntityKind_STRIP][callsign].Revision
	return h.nodes[node].router.Route(h.ctx, &pb.CommandRequest{ProtocolRevision: 1, CommandId: id, Aggregate: candidateRef(h.id), Actor: &pb.Actor{Kind: pb.Actor_CONTROLLER, Id: cid, SessionId: &h.id}, ExpectedEntityRevision: &revision,
		Command: &pb.CommandRequest_Client{Client: &pb.ClientCommand{Action: &pb.ClientCommand_Strip{Strip: &pb.StripAction{Callsign: callsign, Change: &pb.StripAction_GenerateSquawk{GenerateSquawk: &pb.GenerateSquawk{}}}}}}})
}

func TestSquawkBinaryQueueRateResultsCancellationAndTakeover(t *testing.T) {
	h := newDeadlineHarness(t)
	owner := h.owner()
	socket := h.socket(1-owner, "430001", "EKDK_FMP", "131.040")
	first, second, cancel := deadlineStrip("SAS183"), deadlineStrip("SAS184"), deadlineStrip("SAS185")
	first.AssignedSquawk, second.AssignedSquawk, cancel.AssignedSquawk = "", "", ""
	// Automatic queuing uses actual binary sync. Manual calls use the same
	// authenticated typed browser action/planner as the binary browser handler.
	second.Cleared, cancel.Cleared = true, true
	h.sync(socket, first, second, cancel)
	h.await("automatic durable squawk", func() bool {
		for _, e := range h.state().Effects {
			if e.GetGenerateSquawk().GetCallsign() == first.Callsign {
				return true
			}
		}
		return false
	})
	state := h.state()
	var automatic string
	for id, e := range state.Effects {
		if e.GetGenerateSquawk().GetCallsign() == first.Callsign {
			automatic = id
		}
	}
	seq := state.SubjectSequence
	if err := candidateReply(h.nodes[1-owner].c.RequestSquawk(h.ctx, h.id, automatic, first.Callsign)); err != nil {
		t.Fatal(err)
	}
	if h.state().SubjectSequence != seq {
		t.Fatal("identical UUID created another event")
	}
	pending := h.manual(1-owner, socket.cid, first.Callsign, uuid.NewString())
	if pending.GetOutcome().GetReasonCode() != "SQUAWK_ALREADY_PENDING" {
		t.Fatalf("pending rejection: %v", pending)
	}
	manual, cancelled := uuid.NewString(), uuid.NewString()
	if err := candidateReply(h.manual(1-owner, socket.cid, second.Callsign, manual)); err != nil {
		t.Fatal(err)
	}
	if err := candidateReply(h.manual(owner, socket.cid, cancel.Callsign, cancelled)); err != nil {
		t.Fatal(err)
	}
	n := h.nodes[h.owner()]
	if err := n.effects.Sweep(h.ctx); err != nil {
		t.Fatal(err)
	}
	h.await("first real binary dispatch", func() bool { return len(socket.generated()) == 1 })
	firstFrame := socket.generated()[0]
	if firstFrame.CommandId != automatic || firstFrame.GetGenerateSquawk().Callsign != first.Callsign {
		t.Fatal("queue bypassed accepted sequence")
	}
	nextAt := h.state().Indexes[pb.EntityKind_SESSION_SQUAWK_THROTTLE][fmt.Sprint(h.id)].Value.GetSessionSquawkThrottle().NextAllowedAt.AsTime()
	// Native invocation's result is delivered on the real binary socket/outbox path.
	socket.send(t, &es.Envelope{SessionId: h.id, CommandId: firstFrame.CommandId, OwnerEpoch: firstFrame.OwnerEpoch, MasterEpoch: firstFrame.MasterEpoch,
		Event: &es.Envelope_CommandResult{CommandResult: &es.CommandResultEvent{CommandId: firstFrame.CommandId, Status: es.CommandResultEvent_EXECUTED, Reason: es.CommandResultEvent_OK}}})
	h.await("durable plugin result", func() bool { return h.state().Effects[automatic].Status == pb.EffectRecord_EXECUTED })
	socket.send(t, h.frame(&es.Envelope{Event: &es.Envelope_AssignedSquawk{AssignedSquawk: &es.AssignedSquawkEvent{Callsign: cancel.Callsign, Squawk: "2104"}}}))
	h.await("assigned squawk admitted", func() bool {
		return h.state().Indexes[pb.EntityKind_STRIP][cancel.Callsign].Value.GetStrip().AssignedSquawk == "2104"
	})
	_ = n.effects.Sweep(h.ctx)
	if len(socket.generated()) != 1 || h.state().Effects[manual].Status != pb.EffectRecord_WAITING || h.state().Effects[cancelled].Status != pb.EffectRecord_FAILED {
		t.Fatal("throttle or preclaim cancellation failed")
	}
	// Kill after one dispatch and before the next queued request. Claim and
	// throttle are durable across the owner process death.
	next := h.killOwner()
	n = h.nodes[next]
	if err := n.effects.Sweep(h.ctx); err != nil {
		t.Fatal(err)
	}
	h.await("second claim after takeover", func() bool {
		_ = n.effects.Sweep(h.ctx)
		return h.state().Effects[manual].Status == pb.EffectRecord_DISPATCH_CLAIMED
	})
	h.await("second physical dispatch", func() bool { return len(socket.generated()) == 2 })
	secondAt := h.state().Indexes[pb.EntityKind_SESSION_SQUAWK_THROTTLE][fmt.Sprint(h.id)].Value.GetSessionSquawkThrottle().NextAllowedAt.AsTime().Add(-5 * time.Second)
	if secondAt.Before(nextAt) {
		t.Fatal("takeover shortened shared server-time throttle")
	}
	for i := 0; i < 3; i++ {
		_ = n.effects.Sweep(h.ctx)
	}
	if len(socket.generated()) != 2 {
		t.Fatal("claimed request dispatched again")
	}
}

type lostSquawkClaimAck struct {
	cluster.EventStore
	lost bool
}

func (s *lostSquawkClaimAck) Publish(ctx context.Context, subject string, revision uint64, data []byte) (uint64, error) {
	seq, err := s.EventStore.Publish(ctx, subject, revision, data)
	e := &pb.StateEvent{}
	if err == nil && !s.lost && proto.Unmarshal(data, e) == nil && e.GetEffectChanged().GetGenerateSquawk() != nil && e.GetEffectChanged().Status == pb.EffectRecord_DISPATCH_CLAIMED {
		s.lost = true
		return 0, errors.New("test lost committed squawk PubAck")
	}
	return seq, err
}

func TestSquawkBinaryLostClaimAckUnknownAndOwnerDeath(t *testing.T) {
	h := newDeadlineHarness(t)
	owner := h.owner()
	socket := h.socket(1-owner, "440001", "EKDK_FMP", "131.040")
	strip := deadlineStrip("SAS186")
	strip.AssignedSquawk = ""
	h.sync(socket, strip)
	h.await("automatic effect", func() bool { return len(h.state().Effects) == 1 })
	var id string
	for k := range h.state().Effects {
		id = k
	}
	n := h.nodes[owner]
	fault := &lostSquawkClaimAck{EventStore: n.owner.Store}
	n.owner.Store = fault
	_ = n.effects.Sweep(h.ctx)
	h.await("claim committed despite lost PubAck", func() bool { return h.state().Effects[id].Status == pb.EffectRecord_DISPATCH_CLAIMED })
	if !fault.lost || len(socket.generated()) != 0 {
		t.Fatal("ambiguous claim permitted socket send")
	}
	n = h.nodes[h.killOwner()]
	_ = n.effects.Sweep(h.ctx)
	if len(socket.generated()) != 0 {
		t.Fatal("new owner resent ambiguous claim")
	}
	until := time.Now().Add(35 * time.Second)
	for time.Now().Before(until) && h.state().Effects[id].Status != pb.EffectRecord_UNKNOWN {
		_ = n.effects.Sweep(h.ctx)
		time.Sleep(100 * time.Millisecond)
	}
	if h.state().Effects[id].Status != pb.EffectRecord_UNKNOWN || len(socket.generated()) != 0 {
		t.Fatal("claim uncertainty did not become UNKNOWN without a send")
	}
	if err := candidateReply(n.c.RequestSquawk(h.ctx, h.id, id, strip.Callsign)); err != nil {
		t.Fatal(err)
	}
	if len(socket.generated()) != 0 || h.state().Effects[id].Status != pb.EffectRecord_UNKNOWN {
		t.Fatal("retry requeued UNKNOWN")
	}
}

func TestDeadlineBinaryMasterChangeTakeoverRecoveryAndExpiry(t *testing.T) {
	h := newDeadlineHarness(t)
	oldOwner := h.owner()
	socket := h.socket(1-oldOwner, "450001", "EKDK_FMP", "131.040")
	h.sync(socket, deadlineStrip("SAS187"))
	n := h.nodes[oldOwner]
	writer, err := n.c.positionWriter(h.ctx, h.id, h.state().Master.ConnectionId)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := writer.QueueDisconnect(h.ctx, "SAS187", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if r := <-receipt; r.Err != nil {
		t.Fatal(r.Err)
	}
	// Before scheduling: the old process dies with only the accepted KV tombstone.
	next := h.killOwner()
	n = h.nodes[next]
	// Old generation cannot fire while waiting for the new master sync.
	if err := n.c.Recover(h.ctx, h.id); err != nil {
		t.Fatal(err)
	}
	if h.state().Indexes[pb.EntityKind_SESSION_DEADLINE]["aircraft-disconnect.SAS187"] != nil {
		t.Fatal("stale generation created live deadline")
	}
	h.elect()
	oldEpoch := h.state().Master.Epoch
	h.sync(socket) // absence in full sync produces a fresh tombstone/recovery
	h.await("takeover reconstructed deadline", func() bool {
		return h.state().Indexes[pb.EntityKind_SESSION_DEADLINE]["aircraft-disconnect.SAS187"] != nil
	})
	deadline := h.state().Indexes[pb.EntityKind_SESSION_DEADLINE]["aircraft-disconnect.SAS187"].Value.GetSessionDeadline()
	other := h.socket(next, "450002", "EKCH_A_TWR", "118.105")
	_ = socket.conn.Close()
	h.await("new master", func() bool { h.elect(); return h.state().Master.Epoch > oldEpoch && h.state().Master.Cid == other.cid })
	rearmed := h.state().Indexes[pb.EntityKind_SESSION_DEADLINE][deadline.Id].Value.GetSessionDeadline()
	if time.Until(rearmed.DueAt.AsTime()) < 40*time.Second || time.Until(rearmed.DueAt.AsTime()) > cluster.MasterTransferGrace {
		t.Fatal("master change did not rearm pending deadline to 45 seconds")
	}
	h.sync(other)
	h.putVatsim("")
	h.expire(deadline.Id)
	if h.state().Indexes[pb.EntityKind_STRIP]["SAS187"] != nil {
		t.Fatal("shared absence did not delete aircraft on recovered expiry")
	}
	if h.state().Indexes[pb.EntityKind_SESSION_DEADLINE][deadline.Id] != nil {
		t.Fatal("expiry was not consumed")
	}
}

func TestDeadlineBinaryScheduledSnapshotTakeoverExpiry(t *testing.T) {
	h := newDeadlineHarness(t)
	oldOwner := h.owner()
	socket := h.socket(1-oldOwner, "460001", "EKDK_FMP", "131.040")
	h.sync(socket, deadlineStrip("SAS189"))
	socket.send(t, h.frame(&es.Envelope{Event: &es.Envelope_AircraftDisconnect{AircraftDisconnect: &es.AircraftDisconnectEvent{Callsign: "SAS189"}}}))
	h.await("scheduled deadline", func() bool {
		return h.state().Indexes[pb.EntityKind_SESSION_DEADLINE]["aircraft-disconnect.SAS189"] != nil
	})
	n := h.nodes[oldOwner]
	state := h.state()
	if err := n.p.Snapshots.Save(state); err != nil {
		t.Fatal(err)
	}
	restored, err := n.p.Snapshots.Load(state.Ref)
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(restored.Indexes[pb.EntityKind_SESSION_DEADLINE]["aircraft-disconnect.SAS189"], state.Indexes[pb.EntityKind_SESSION_DEADLINE]["aircraft-disconnect.SAS189"]) {
		t.Fatal("verified snapshot lost scheduled deadline")
	}
	replay, err := cluster.NewProjection(h.nodes[1-oldOwner].nc, h.cfg)
	if err != nil {
		t.Fatal(err)
	}
	replayCtx, stop := context.WithCancel(h.ctx)
	defer stop()
	done := make(chan error, 1)
	go func() { done <- replay.Run(replayCtx) }()
	h.await("independent snapshot replay", func() bool {
		select {
		case err := <-done:
			t.Fatalf("snapshot replay: %v", err)
		default:
		}
		if replay.Ready() != nil {
			return false
		}
		a, err := replay.Read(state.Ref)
		return err == nil && a.Indexes[pb.EntityKind_SESSION_DEADLINE]["aircraft-disconnect.SAS189"] != nil
	})
	// After scheduling: takeover sees the same accepted deadline, fences its
	// old position generation, syncs fresh absence and consumes the real expiry.
	n = h.nodes[h.killOwner()]
	h.elect()
	h.sync(socket)
	h.putVatsim("")
	h.expire("aircraft-disconnect.SAS189")
	if h.state().Indexes[pb.EntityKind_STRIP]["SAS189"] != nil {
		t.Fatal("new owner failed shared absence expiry")
	}
	// A later pass cannot recreate an already consumed disconnect.
	if err := n.work.ReconcileSession(h.ctx, h.id); err != nil {
		t.Fatal(err)
	}
	if h.state().Indexes[pb.EntityKind_SESSION_DEADLINE]["aircraft-disconnect.SAS189"] != nil {
		t.Fatal("consumed expiry recreated after takeover")
	}
}

func TestDeadlineBinaryOfflineExpiryAndSessionDisconnectReconcile(t *testing.T) {
	h := newDeadlineHarness(t)
	master := h.socket(1-h.owner(), "470001", "EKDK_FMP", "131.040")
	strip := deadlineStrip("SAS190")
	strip.Stand = "A12"
	h.sync(master, strip)
	tower := h.socket(h.owner(), "470002", "EKCH_A_TWR", "118.105")
	n := h.nodes[h.owner()]
	h.await("debounced real reconciliation", func() bool {
		_ = n.work.ReconcileSession(h.ctx, h.id)
		return len(h.state().Indexes[pb.EntityKind_SECTOR_OWNER]) > 0
	})
	_ = tower.conn.Close()
	h.await("socket offline recovery", func() bool {
		_ = n.c.Recover(h.ctx, h.id)
		return h.state().Indexes[pb.EntityKind_SESSION_DEADLINE]["controller-offline.EKCH_A_TWR"] != nil
	})
	h.expire("controller-offline.EKCH_A_TWR")
	if h.state().Indexes[pb.EntityKind_CONTROLLER][tower.cid] != nil {
		t.Fatal("actual controller expiry did not remove disconnected CID")
	}
	h.await("offline route/sector cleanup", func() bool {
		_ = n.work.ReconcileSession(h.ctx, h.id)
		return len(h.state().Indexes[pb.EntityKind_SECTOR_OWNER]) == 0 && len(h.state().Indexes[pb.EntityKind_STRIP][strip.Callsign].Value.GetStrip().NextControllers) == 0
	})
	_ = master.conn.Close()
	h.await("session disconnect scheduled from shared absence", func() bool {
		_ = n.c.Recover(h.ctx, h.id)
		return h.state().Indexes[pb.EntityKind_SESSION_DEADLINE]["session-disconnect"] != nil
	})
	h.expire("controller-offline.EKDK_FMP")
	h.expire("session-disconnect")
	if len(h.state().Indexes[pb.EntityKind_CONTROLLER]) != 0 || len(h.state().Indexes[pb.EntityKind_SECTOR_OWNER]) != 0 {
		t.Fatal("concrete disconnect callback retained operational coverage")
	}
}
