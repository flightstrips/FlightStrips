package pdc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"FlightStrips/internal/cluster"
	"FlightStrips/internal/config"
	"FlightStrips/internal/natsresources"
	"FlightStrips/internal/testing/natscluster"
	"FlightStrips/internal/vatsim"
	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type hoppieFixture struct {
	mu       sync.Mutex
	poll     string
	polls    int
	sends    []string
	failSend bool
	server   *httptest.Server
}

func newHoppieFixture(t *testing.T) *hoppieFixture {
	f := &hoppieFixture{}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		q := r.URL.Query()
		if q.Get("logon") != "fixture-secret" {
			t.Error("provider secret missing")
		}
		if q.Get("type") == "poll" {
			f.polls++
			body := f.poll
			f.poll = ""
			fmt.Fprint(w, "ok"+body)
			return
		}
		f.sends = append(f.sends, q.Get("packet"))
		if f.failSend {
			w.WriteHeader(500)
			return
		}
		fmt.Fprint(w, "ok")
	}))
	t.Cleanup(f.server.Close)
	return f
}
func (f *hoppieFixture) queue(packet string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.poll = " {SAS101 telex {" + packet + "}}"
}
func (f *hoppieFixture) response(sequence uint64, unable bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	kind := "WILCO"
	if unable {
		kind = "UNABLE"
	}
	f.poll = fmt.Sprintf(" {SAS101 cpdlc {/data2/42/%d/N/%s}}", sequence, kind)
}
func (f *hoppieFixture) counts() (int, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.polls, len(f.sends)
}
func (f *hoppieFixture) clearances() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, m := range f.sends {
		if strings.Contains(m, "/WU/") {
			n++
		}
	}
	return n
}

type pdcReplica struct {
	nc         *nats.Conn
	projection *cluster.Projection
	owner      *cluster.OwnerRuntime
	c          *Candidate
	stop       context.CancelFunc
	stopped    bool
}
type pdcHarness struct {
	t       *testing.T
	ctx     context.Context
	cfg     natsresources.Config
	nodes   [2]*pdcReplica
	id      int32
	fixture *hoppieFixture
	offset  time.Duration
}

func newPdcHarness(t *testing.T, enabled bool) *pdcHarness {
	t.Helper()
	if os.Getenv("NATS_INTEGRATION") != "1" {
		t.Skip("requires pinned three-node NATS fixture")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	t.Cleanup(cancel)
	port := 4222
	if v := os.Getenv("NATS_TEST_PORT_BASE"); v != "" {
		var e error
		port, e = strconv.Atoi(v)
		if e != nil {
			t.Fatal(e)
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
	admin, e := natsresources.Connect(cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer admin.Close()
	if e = natscluster.WaitForQuorum(ctx, admin); e != nil {
		t.Fatal(e)
	}
	if e = natsresources.Bootstrap(ctx, admin, cfg); e != nil {
		t.Fatal(e)
	}
	cfg.URLs = urls("backend")
	h := &pdcHarness{t: t, ctx: ctx, cfg: cfg, id: int32(600000 + time.Now().UnixNano()%1000000000)}
	if enabled {
		h.fixture = newHoppieFixture(t)
	}
	h.start(0)
	h.start(1)
	t.Cleanup(func() {
		for _, n := range h.nodes {
			h.kill(n)
		}
	})
	n := h.current()
	w := n.c.Writer
	w.Plan = cluster.PlanSystemEntity
	seed := &pb.Session{Id: h.id, Airport: "EKCH", Name: "LIVE", NextStripId: 1, NextTacticalId: 1, NextMessageId: 1, Runways: []*pb.Runway{{Name: "22R", Departure: true}}, AvailableSids: []*pb.SidInfo{{Name: "NEXEN2C", Runway: "22R"}}, Master: &pb.MasterTerm{Cid: "123", ConnectionId: uuid.NewString(), Epoch: 1, OwnerEpoch: 1}}
	h.require(w.Execute(ctx, pdcSystem(h.id, uuid.NewString(), "fixture", proto.Uint64(0), pdcUpdate(fmt.Sprint(h.id), &pb.EntityRecord{Value: &pb.EntityRecord_Session{Session: seed}}))))
	h.require(w.Execute(ctx, pdcSystem(h.id, uuid.NewString(), "fixture", proto.Uint64(0), pdcUpdate("123", &pb.EntityRecord{Value: &pb.EntityRecord_Controller{Controller: &pb.Controller{Cid: "123", Callsign: "EKCH_DEL", Position: "119.900", Revision: 1}}}))))
	h.require(w.Execute(ctx, pdcSystem(h.id, uuid.NewString(), "fixture", proto.Uint64(0), pdcUpdate("DEL", &pb.EntityRecord{Value: &pb.EntityRecord_SectorOwner{SectorOwner: &pb.SectorOwner{Sector: "DEL", ControllerCid: "123", Position: "119.900", Identifier: "DEL"}}}))))
	strips := cluster.StripState{Store: cluster.LocalLifecycleStore{Writer: n.c.Writer}}
	reply, e := strips.Put(ctx, h.id, &pb.Strip{Callsign: "SAS101", AircraftType: "A320", Departure: "EKCH", Destination: "ENGM", Runway: "22R", Sid: "NEXEN2C", AssignedSquawk: "2345", Bay: "NOT_CLEARED", VatsimCid: "123", EngineType: "J"}, 0)
	if e != nil {
		t.Fatal(e)
	}
	h.require(reply)
	return h
}
func (h *pdcHarness) start(index int) {
	h.t.Helper()
	nc, e := natsresources.Connect(h.cfg)
	if e != nil {
		h.t.Fatal(e)
	}
	projection, e := cluster.NewProjection(nc, h.cfg)
	if e != nil {
		h.t.Fatal(e)
	}
	ctx, stop := context.WithCancel(h.ctx)
	go func() { _ = projection.Run(ctx) }()
	h.await("projection", func() bool { return projection.Ready() == nil })
	store := cluster.NATSStore{JS: projection.JS}
	owner, e := cluster.NewOwnerRuntime(nc, projection, store)
	if e != nil {
		h.t.Fatal(e)
	}
	if e = owner.Track(pdcRef(h.id)); e != nil {
		h.t.Fatal(e)
	}
	objects, e := projection.JS.ObjectStore("FS_OBJECTS")
	if e != nil {
		h.t.Fatal(e)
	}
	var client HoppieClientInterface
	if h.fixture != nil {
		p := NewClient("fixture-secret")
		p.baseURL = h.fixture.server.URL
		client = p
	}
	writer := cluster.Writer{Store: store, NodeID: owner.NodeID, Projection: projection, Lease: owner}
	objectStore := cluster.NATSObjects{Store: objects}
	frequencies, e := cluster.NewTransceiverSource(cluster.NavigationWeather{Writer: writer, Objects: objectStore})
	if e != nil {
		h.t.Fatal(e)
	}
	c, e := NewCandidate(writer, objectStore, client, cluster.PlanStrip, frequencies)
	if e != nil {
		h.t.Fatal(e)
	}
	c.Now = func() time.Time { return time.Now().UTC().Add(h.offset) }
	h.nodes[index] = &pdcReplica{nc: nc, projection: projection, owner: owner, c: c, stop: stop}
	go func() { _ = owner.Run(ctx) }()
}
func (h *pdcHarness) kill(n *pdcReplica) {
	if n != nil && !n.stopped {
		n.stopped = true
		n.stop()
		n.nc.Close()
	}
}
func (h *pdcHarness) await(label string, fn func() bool) {
	h.t.Helper()
	deadline := time.Now().Add(35 * time.Second)
	for time.Now().Before(deadline) && h.ctx.Err() == nil {
		if fn() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	h.t.Fatalf("timeout waiting for %s", label)
}
func (h *pdcHarness) current() *pdcReplica {
	h.t.Helper()
	var out *pdcReplica
	h.await("session owner", func() bool {
		for _, n := range h.nodes {
			if n != nil && !n.stopped && n.owner.CanWrite(pdcRef(h.id)) {
				out = n
				return true
			}
		}
		return false
	})
	return out
}
func (h *pdcHarness) require(r *pb.CommandReply) {
	h.t.Helper()
	if e := pdcReply(r); e != nil {
		h.t.Fatal(e)
	}
}
func (h *pdcHarness) step() {
	h.t.Helper()
	if e := h.current().c.PDC(h.ctx, h.id); e != nil {
		h.t.Fatal(e)
	}
}
func (h *pdcHarness) state() *cluster.Aggregate {
	h.t.Helper()
	a, e := h.current().c.Writer.Read(h.ctx, pdcRef(h.id))
	if e != nil {
		h.t.Fatal(e)
	}
	return a
}
func (h *pdcHarness) sequence() *pb.PdcSequence {
	return h.state().Indexes[pb.EntityKind_PDC_SEQUENCE]["SAS101"].Value.GetPdcSequence()
}
func (h *pdcHarness) due() { h.offset += 46 * time.Second }
func (h *pdcHarness) request(remarks string) {
	h.fixture.queue("REQUEST PREDEP CLEARANCE SAS101 A320 TO ENGM AT EKCH STAND A17 ATIS A " + remarks)
}
func (h *pdcHarness) action(actor pb.Actor_Kind, id string, action *pb.PdcAction) *pb.CommandRequest {
	return &pb.CommandRequest{ProtocolRevision: 1, CommandId: uuid.NewString(), Aggregate: pdcRef(h.id), Actor: &pb.Actor{Kind: actor, Id: id, SessionId: &h.id}, ExpectedEntityRevision: proto.Uint64(h.state().Indexes[pb.EntityKind_PDC_SEQUENCE]["SAS101"].GetRevision()), Command: &pb.CommandRequest_Client{Client: &pb.ClientCommand{Action: &pb.ClientCommand_Pdc{Pdc: action}}}}
}

// The harness limits flight reads to its own isolated session; the accepted
// entities and all writes still use the real projection and candidate policy.
type pdcSessionFlights struct{ h *pdcHarness }

func (f pdcSessionFlights) FindFlight(ctx context.Context, key string) (*cluster.FlightSnapshot, error) {
	a, e := f.h.current().c.Writer.Read(ctx, pdcRef(f.h.id))
	if e != nil {
		return nil, e
	}
	s := a.Indexes[pb.EntityKind_STRIP][key]
	if s == nil {
		return nil, cluster.ErrFlightNotFound
	}
	return &cluster.FlightSnapshot{SessionID: f.h.id, Airport: "EKCH", Strip: proto.Clone(s.Value.GetStrip()).(*pb.Strip), PDC: a.Indexes[pb.EntityKind_PDC_SEQUENCE][key].GetValue().GetPdcSequence(), PDCRevision: a.Indexes[pb.EntityKind_PDC_SEQUENCE][key].GetRevision()}, nil
}
func TestNATSPdcHTTPJSONThroughProductionPolicy(t *testing.T) {
	h := newPdcHarness(t, false)
	n := h.current()
	api := NewCandidateWebAPI(candidateAuth{}, candidateVerifier{}, true, pdcSessionFlights{h}, n.c, n.projection)
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)
	id := uuid.NewString()
	call := func(method, path, body, command string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer 123")
		if command != "" {
			r.Header.Set("Idempotency-Key", command)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	body := `{"callsign":"SAS101","aircraft_type":"A320","atis":"A","stand":"A17","remarks":"please review"}`
	first := call("POST", "/pdc/request", body, id)
	if first.Code != http.StatusAccepted && first.Code != http.StatusCreated {
		t.Fatalf("request JSON: %d %s", first.Code, first.Body.String())
	}
	before := h.state().Revision
	retry := call("POST", "/pdc/request", body, id)
	if retry.Code != first.Code || h.state().Revision != before {
		t.Fatal("HTTP retry repeated mutation")
	}
	issue := h.action(pb.Actor_CONTROLLER, "123", &pb.PdcAction{Callsign: "SAS101", Change: &pb.PdcAction_Issue{Issue: &pb.IssuePdc{RequestRemarks: "EXPECT DELAY"}}})
	h.require(n.c.Execute(h.ctx, issue))
	status := call("GET", "/pdc/status?callsign=SAS101", "", "")
	var result webPDCStatusResponse
	if e := json.Unmarshal(status.Body.Bytes(), &result); e != nil || status.Code != http.StatusOK || result.ClearanceText == nil || !strings.Contains(*result.ClearanceText, "CLRD TO: ENGM") {
		t.Fatalf("status JSON: %d %s", status.Code, status.Body.String())
	}
	ack := call("POST", "/pdc/acknowledge", `{"callsign":"SAS101"}`, uuid.NewString())
	if ack.Code != http.StatusAccepted && ack.Code != http.StatusOK {
		t.Fatalf("ack JSON: %d %s", ack.Code, ack.Body.String())
	}
	if h.sequence().State != "CONFIRMED" {
		t.Fatal("HTTP response did not reach production policy")
	}
	if bytes.Contains(status.Body.Bytes(), []byte("/data2/")) {
		t.Fatal("HTTP leaked provider envelope")
	}
}

func (h *pdcHarness) patchStrip(mutate func(*pb.Strip)) {
	a := h.state()
	old := a.Indexes[pb.EntityKind_STRIP]["SAS101"]
	s := proto.Clone(old.Value.GetStrip()).(*pb.Strip)
	mutate(s)
	reply, e := (cluster.StripState{Store: cluster.LocalLifecycleStore{Writer: h.current().c.Writer}}).Put(h.ctx, h.id, s, old.Revision)
	if e != nil {
		h.t.Fatal(e)
	}
	h.require(reply)
}
func TestNATSPdcProductionFaultsAndMandatoryRouteEffects(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*pb.Strip)
	}{
		{"reserved squawk", func(s *pb.Strip) { s.AssignedSquawk = "1234" }},
		{"restricted SID", func(s *pb.Strip) { s.Sid = "BETUD2C" }},
		{"inactive runway", func(s *pb.Strip) { s.Runway = "04R" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newPdcHarness(t, true)
			h.patchStrip(tc.mutate)
			h.request("")
			h.step()
			if h.sequence().State != "REQUESTED_WITH_FAULTS" || h.fixture.clearances() != 0 {
				t.Fatal("production faults bypassed", h.sequence())
			}
			if _, sends := h.fixture.counts(); sends != 1 {
				t.Fatal("fault status ack missing")
			}
		})
	}
	t.Run("mandatory route", func(t *testing.T) {
		h := newPdcHarness(t, true)
		h.patchStrip(func(s *pb.Strip) { s.Sid = "KOPEX2C" })
		w := h.current().c.Writer
		w.Plan = cluster.PlanSystemEntity
		h.require(w.Execute(h.ctx, pdcSystem(h.id, uuid.NewString(), "fixture", proto.Uint64(0), pdcUpdate("SAS101", &pb.EntityRecord{Value: &pb.EntityRecord_EcfmpState{EcfmpState: &pb.EcfmpState{Callsign: "SAS101", Restrictions: []*pb.EcfmpRestriction{{Kind: "mandatory_route", Routes: []string{"NEXEN T503 MICOS"}}}}}}))))
		h.request("")
		h.step()
		if h.sequence().State != "REQUESTED_WITH_FAULTS" {
			t.Fatal("mandatory route skipped review")
		}
		issue := h.action(pb.Actor_CONTROLLER, "123", &pb.PdcAction{Callsign: "SAS101", Change: &pb.PdcAction_Issue{Issue: &pb.IssuePdc{}}})
		h.require(h.current().c.Execute(h.ctx, issue))
		h.kill(h.current())
		h.current()
		h.step()
		if !strings.Contains(h.sequence().ClearanceText, "MANDATORY ROUTE: @NEXEN T503 MICOS@") {
			t.Fatal(h.sequence())
		}
		found := map[string]bool{}
		for _, e := range h.state().Effects {
			if f := e.GetSetFlightPlan(); f != nil {
				found[f.Field] = true
				if e.TargetCid != "123" {
					t.Fatal("route effect target changed")
				}
			}
		}
		if !found["SID"] || !found["ROUTE"] {
			t.Fatal("missing typed route/SID effects", found)
		}
	})
}

func TestNATSPdcAcceptedTransceiverFrequencyComposesClearance(t *testing.T) {
	h := newPdcHarness(t, false)
	n := h.current()
	global := &pb.AggregateRef{Target: &pb.AggregateRef_Global{Global: &pb.GlobalRef{}}}
	if err := n.owner.Track(global); err != nil {
		t.Fatal(err)
	}
	h.await("global transceiver owner", func() bool { return n.owner.CanWrite(global) })
	priority, err := getPdcAirborneControllerPriority(proto.String("NEXEN2C"))
	if err != nil || len(priority) == 0 {
		t.Fatal(priority, err)
	}
	position, err := config.GetPositionByName(priority[0])
	if err != nil {
		t.Fatal(err)
	}
	hz, err := strconv.ParseFloat(position.Frequency, 64)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `[{"callsign":"EKCH_DEL","transceivers":[{"frequency":%.0f}]}]`, hz*1000000)
	}))
	defer server.Close()
	nav := n.c.Source
	nav.Writer.Plan = cluster.PlanSystemEntity
	feed, err := cluster.NewTransceiverFeed(nav, vatsim.NewTransceiverProvider(server.URL, time.Second, server.Client()))
	if err != nil {
		t.Fatal(err)
	}
	if sent, err := feed.Refresh(h.ctx, time.Now()); err != nil || !sent {
		t.Fatal("accepted transceiver page missing", sent, err)
	}
	h.await("both accepted frequency readers", func() bool {
		for _, node := range h.nodes {
			frequencies := node.c.Transceivers[0].GetFrequencies("EKCH_DEL")
			if len(frequencies) != 1 || normalizeFrequency(frequencies[0]) != normalizeFrequency(position.Frequency) {
				return false
			}
		}
		return true
	})
	now := timestamppb.Now()
	connection := uuid.NewString()
	for key, value := range map[string]*pb.PresenceValue{
		"node." + n.owner.NodeID: {SchemaVersion: 1, Present: &pb.PresenceValue_Node{Node: &pb.NodePresence{NodeId: n.owner.NodeID, StartedAt: now, Ready: true}}},
		"client." + connection:   {SchemaVersion: 1, Present: &pb.PresenceValue_Client{Client: &pb.ClientPresence{ConnectionId: connection, NodeId: n.owner.NodeID, SessionId: h.id, Cid: "123", Kind: pb.ClientPresence_EUROSCOPE, ConnectedAt: now}}},
	} {
		data, err := proto.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = n.projection.Presence.Put(key, data); err != nil {
			t.Fatal(err)
		}
	}
	h.await("fresh operational controller", func() bool {
		_, presence, err := n.projection.ObservationSnapshot(h.id)
		if err != nil {
			return false
		}
		clientReady, nodeReady := false, false
		for _, item := range presence {
			if item.Value.GetClient().GetConnectionId() == connection {
				clientReady = true
			}
			if item.Value.GetNode().GetNodeId() == n.owner.NodeID && item.Value.GetNode().GetReady() {
				nodeReady = true
			}
		}
		return clientReady && nodeReady
	})
	request := h.action(pb.Actor_PILOT, "123", &pb.PdcAction{Callsign: "SAS101", Change: &pb.PdcAction_Issue{Issue: &pb.IssuePdc{RequestChannel: "WEB", Atis: "A", AircraftType: "A320"}}})
	h.require(n.c.Execute(h.ctx, request))
	if !strings.Contains(h.sequence().ClearanceText, "Departure frequency "+position.Frequency) {
		t.Fatal("accepted secondary radio missing from production clearance", h.sequence())
	}
}

func TestNATSPdcDeletedStripCancelsPendingClearance(t *testing.T) {
	h := newPdcHarness(t, true)
	h.request("MANUAL REVIEW")
	h.step()
	issue := h.action(pb.Actor_CONTROLLER, "123", &pb.PdcAction{Callsign: "SAS101", Change: &pb.PdcAction_Issue{Issue: &pb.IssuePdc{}}})
	h.require(h.current().c.Execute(h.ctx, issue))
	old := h.state().Indexes[pb.EntityKind_STRIP]["SAS101"]
	reply, err := (cluster.StripState{Store: cluster.LocalLifecycleStore{Writer: h.current().c.Writer}}).Delete(h.ctx, h.id, "SAS101", old.Revision)
	if err != nil {
		t.Fatal(err)
	}
	h.require(reply)
	h.kill(h.current())
	h.current()
	h.step()
	if h.fixture.clearances() != 0 {
		t.Fatal("deleted strip retained permission to send its pending clearance")
	}
}

func TestNATSPdcProductionPollingCorrelationAndTakeover(t *testing.T) {
	h := newPdcHarness(t, true)
	h.request("")
	h.step()
	if s := h.sequence(); s.State != "CLEARED" || !s.Sent || !strings.Contains(s.ClearanceText, "SID: @NEXEN2C@") || !strings.Contains(s.ClearanceText, "RWY: @22R@") {
		t.Fatalf("real policy clearance: %v", s)
	}
	polls, _ := h.fixture.counts()
	for _, n := range h.nodes {
		e := n.c.PDC(h.ctx, h.id)
		if n.owner.CanWrite(pdcRef(h.id)) && e != nil {
			t.Fatal(e)
		}
	}
	again, _ := h.fixture.counts()
	if again != polls || h.fixture.clearances() != 1 {
		t.Fatal("duplicate provider call")
	}
	first := h.current()
	h.kill(first)
	h.current()
	h.step()
	again, _ = h.fixture.counts()
	if again != polls || h.fixture.clearances() != 1 {
		t.Fatal("takeover repeated accepted slot")
	}
	// A different response sequence is durable but cannot acknowledge clearance.
	h.fixture.response(h.sequence().Sequence+1, false)
	h.due()
	h.step()
	if h.sequence().State != "CLEARED" {
		t.Fatal("stale response accepted")
	}
	h.fixture.response(h.sequence().Sequence, false)
	h.due()
	h.step()
	if h.sequence().State != "CONFIRMED" || h.sequence().PilotAcknowledgedAt == nil {
		t.Fatal("correlated WILCO not confirmed")
	}
	state := h.state()
	cleared := false
	for _, e := range state.Effects {
		if e.GetPdc().GetAction() == "SET_CLEARED_FLAG" && e.GetPdc().GetCleared() {
			cleared = true
		}
	}
	if !cleared {
		t.Fatal("missing Task 17 cleared effect")
	}
	// A full projection restart retains messages, workflows and terminal state.
	if err := h.current().projection.Snapshots.Save(h.state()); err != nil {
		t.Fatal(err)
	}
	for _, n := range h.nodes {
		h.kill(n)
	}
	h.start(0)
	h.start(1)
	h.current()
	h.step()
	if h.sequence().State != "CONFIRMED" || h.fixture.clearances() != 1 {
		t.Fatal("restart lost PDC outcome")
	}
	for _, e := range h.state().EntitiesByKind(pb.EntityKind_PDC_PROVIDER_MESSAGE) {
		bytes, _ := proto.Marshal(e.Value)
		if strings.Contains(string(bytes), "fixture-secret") || strings.Contains(string(bytes), "/data2/") {
			t.Fatal("raw frame or secret persisted")
		}
	}
}

func TestNATSPdcBrowserRemarksAndWebWithoutProvider(t *testing.T) {
	h := newPdcHarness(t, false)
	req := h.action(pb.Actor_PILOT, "123", &pb.PdcAction{Callsign: "SAS101", Change: &pb.PdcAction_Issue{Issue: &pb.IssuePdc{RequestChannel: "WEB", Atis: "A", AircraftType: "A320", RequestRemarks: "please review"}}})
	h.require(h.current().c.Execute(h.ctx, req))
	if h.sequence().State != "REQUESTED" && h.sequence().State != "REQUESTED_WITH_FAULTS" {
		t.Fatal("remarks skipped review", h.sequence())
	}
	// This is the exact Task 15 browser command body: request_remarks only.
	issue := h.action(pb.Actor_CONTROLLER, "123", &pb.PdcAction{Callsign: "SAS101", Change: &pb.PdcAction_Issue{Issue: &pb.IssuePdc{RequestRemarks: "EXPECT DELAY"}}})
	h.require(h.current().c.Execute(h.ctx, issue))
	before := proto.Clone(h.sequence()).(*pb.PdcSequence)
	if !strings.Contains(before.ClearanceText, "CLRD TO: ENGM") || !strings.Contains(before.ClearanceText, "SQK: 2345") || !strings.HasSuffix(before.ClearanceText, "EXPECT DELAY") {
		t.Fatal(before.ClearanceText)
	}
	h.require(h.current().c.Execute(h.ctx, issue))
	if !proto.Equal(before, h.sequence()) {
		t.Fatal("retry recomposed clearance")
	}
	changed := proto.Clone(issue).(*pb.CommandRequest)
	changed.GetClient().GetPdc().GetIssue().RequestRemarks = "DIFFERENT"
	if r := h.current().c.Execute(h.ctx, changed); r.Status != pb.CommandReply_INVALID_ARGUMENT {
		t.Fatal("changed retry accepted", r)
	}
	ack := h.action(pb.Actor_PILOT, "123", &pb.PdcAction{Callsign: "SAS101", Change: &pb.PdcAction_Acknowledge{Acknowledge: &pb.AcknowledgePdc{}}})
	h.require(h.current().c.Execute(h.ctx, ack))
	if h.sequence().State != "CONFIRMED" {
		t.Fatal("Web acknowledgment failed")
	}
	h.step()
}

func TestNATSPdcUnableTimeoutAndUncertainSend(t *testing.T) {
	t.Run("revert", func(t *testing.T) {
		h := newPdcHarness(t, true)
		h.request("")
		h.step()
		req := h.action(pb.Actor_CONTROLLER, "123", &pb.PdcAction{Callsign: "SAS101", Change: &pb.PdcAction_RevertToVoice{RevertToVoice: &pb.RevertPdcToVoice{}}})
		h.require(h.current().c.Execute(h.ctx, req))
		h.kill(h.current())
		h.current()
		h.step()
		s := h.state().Indexes[pb.EntityKind_STRIP]["SAS101"].Value.GetStrip()
		if h.sequence().State != "REVERT_TO_VOICE" || s.Bay != "NOT_CLEARED" || s.OwnerCid != "" {
			t.Fatal("revert lifecycle", s)
		}
		flag := false
		for _, e := range h.state().Effects {
			if p := e.GetPdc(); p != nil && p.Action == "SET_CLEARED_FLAG" && p.Cleared != nil && !*p.Cleared {
				flag = true
			}
		}
		if !flag {
			t.Fatal("revert cleared-flag effect missing")
		}
	})
	t.Run("UNABLE", func(t *testing.T) {
		h := newPdcHarness(t, true)
		h.request("")
		h.step()
		h.fixture.response(h.sequence().Sequence, true)
		h.due()
		h.step()
		if h.sequence().State != "FAILED" || h.sequence().Deadline != nil {
			t.Fatal("UNABLE not correlated")
		}
		if s := h.state().Indexes[pb.EntityKind_STRIP]["SAS101"].Value.GetStrip(); s.Bay != "NOT_CLEARED" || s.OwnerCid != "" {
			t.Fatal("UNABLE did not unclear strip", s)
		}
	})
	t.Run("timeout takeover", func(t *testing.T) {
		h := newPdcHarness(t, true)
		h.request("")
		h.step()
		h.kill(h.current())
		h.current()
		h.offset += 11 * time.Minute
		h.step()
		if h.sequence().State != "NO_RESPONSE" || h.sequence().Deadline != nil {
			t.Fatal("timeout not recovered")
		}
		if s := h.state().Indexes[pb.EntityKind_STRIP]["SAS101"].Value.GetStrip(); s.Bay != "NOT_CLEARED" || s.OwnerCid != "" {
			t.Fatal("timeout did not unclear strip", s)
		}
		sends := h.fixture.clearances()
		h.step()
		if h.fixture.clearances() != sends {
			t.Fatal("timeout resent clearance")
		}
		h.fixture.response(h.sequence().Sequence, false)
		h.due()
		h.step()
		if h.sequence().State != "NO_RESPONSE" {
			t.Fatal("late WILCO accepted")
		}
	})
	t.Run("uncertain provider send", func(t *testing.T) {
		h := newPdcHarness(t, true)
		h.fixture.failSend = true
		h.request("")
		if e := h.current().c.PDC(h.ctx, h.id); e == nil {
			t.Fatal("expected uncertain send")
		}
		if h.fixture.clearances() != 1 {
			t.Fatal("send fixture not invoked")
		}
		h.kill(h.current())
		h.current()
		h.fixture.failSend = false
		h.step()
		if h.fixture.clearances() != 1 || h.sequence().Sent {
			t.Fatal("uncertain send repeated or declared accepted")
		}
		failed := false
		for _, w := range h.state().Workflows {
			if w.Step == "pdc/outbound" && w.ReasonCode == "CALL_UNCERTAIN" {
				failed = true
			}
		}
		if !failed {
			t.Fatal("uncertainty not durable")
		}
		h.fixture.response(h.sequence().Sequence, false)
		h.due()
		h.step()
		if h.sequence().State != "CONFIRMED" || !h.sequence().Sent || h.fixture.clearances() != 1 {
			t.Fatal("correlated pilot response did not resolve uncertain delivery")
		}
	})
}

func TestNATSPdcAcceptedResponseWinsDeadlineAfterOwnerDeath(t *testing.T) {
	h := newPdcHarness(t, true)
	h.request("")
	h.step()
	h.fixture.response(h.sequence().Sequence, false)
	h.due()
	n := h.current()
	n.c.Writer.Store = &pdcFaultStore{EventStore: n.c.Writer.Store, after: true, kill: func() { h.kill(n) }, match: func(e *pb.StateEvent) bool {
		if e.Actor.GetId() != "pdc-poll" {
			return false
		}
		for _, c := range e.GetDomainChanged().Changes {
			if c.GetUpsert().GetProviderCheckpoint() != nil {
				return true
			}
		}
		return false
	}}
	_ = n.c.PDC(h.ctx, h.id)
	h.offset += 11 * time.Minute
	h.current()
	h.step()
	if h.sequence().State != "CONFIRMED" || h.sequence().Deadline != nil {
		t.Fatal("accepted on-time WILCO lost to takeover timeout", h.sequence())
	}
	if h.fixture.clearances() != 1 {
		t.Fatal("deadline recovery repeated clearance")
	}
}

type pdcFaultStore struct {
	cluster.EventStore
	once  sync.Once
	match func(*pb.StateEvent) bool
	after bool
	kill  func()
}

func (s *pdcFaultStore) Publish(ctx context.Context, subject string, expected uint64, data []byte) (uint64, error) {
	event := &pb.StateEvent{}
	if e := proto.Unmarshal(data, event); e != nil {
		return 0, e
	}
	hit := false
	if s.match(event) {
		s.once.Do(func() { hit = true })
	}
	if !hit {
		return s.EventStore.Publish(ctx, subject, expected, data)
	}
	if !s.after {
		s.kill()
		return 0, errors.New("owner died before commit")
	}
	seq, e := s.EventStore.Publish(ctx, subject, expected, data)
	s.kill()
	if e != nil {
		return seq, e
	}
	return 0, errors.New("owner died after commit before PubAck")
}
func TestNATSPdcOwnerDeathAtProviderBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, actor  string
		after        bool
		wantSends    int
		wantAccepted bool
	}{
		{"poll result committed", "pdc-poll", true, 1, true},
		{"poll result lost", "pdc-poll", false, 0, false},
		{"send result lost", "pdc-send-result", false, 1, false},
		{"send result committed", "pdc-send-result", true, 1, true},
		{"send intent not committed", "external-worker", false, 1, true},
		{"send intent committed", "external-worker", true, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newPdcHarness(t, true)
			h.request("")
			n := h.current()
			fault := &pdcFaultStore{EventStore: n.c.Writer.Store, after: tc.after, kill: func() { h.kill(n) }, match: func(e *pb.StateEvent) bool {
				if e.Actor.GetId() != tc.actor {
					return false
				}
				if tc.actor == "pdc-poll" {
					for _, change := range e.GetDomainChanged().Changes {
						if change.GetUpsert().GetProviderCheckpoint() != nil {
							return true
						}
					}
					return false
				}
				if tc.actor == "external-worker" {
					for _, w := range e.GetDomainChanged().Workflows {
						if w.Step == "external/hoppie/send" && w.Status == pb.WorkflowRecord_PENDING {
							return true
						}
					}
					return false
				}
				return true
			}}
			n.c.Writer.Store = fault
			_ = n.c.PDC(h.ctx, h.id)
			h.current()
			h.step()
			if got := h.fixture.clearances(); got != tc.wantSends {
				t.Fatalf("clearance sends=%d want=%d", got, tc.wantSends)
			}
			if tc.actor == "pdc-poll" && !tc.after {
				before, _ := h.fixture.counts()
				h.step()
				after, _ := h.fixture.counts()
				if after != before {
					t.Fatal("uncertain poll repeated same slot")
				}
				h.due()
				h.step()
				later, _ := h.fixture.counts()
				if later != before+1 {
					t.Fatal("later poll slot not recovered")
				}
			} else if s := h.sequence(); s == nil || s.Sent != tc.wantAccepted {
				t.Fatalf("send acceptance recovery: %v", s)
			}
		})
	}
}
