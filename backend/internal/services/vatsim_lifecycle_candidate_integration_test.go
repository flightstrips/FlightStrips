package services

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"FlightStrips/internal/cluster"
	"FlightStrips/internal/natsresources"
	"FlightStrips/internal/testing/natscluster"
	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type lifecycleReplica struct {
	nc         *nats.Conn
	projection *cluster.Projection
	owner      *cluster.OwnerRuntime
	candidate  *VatsimLifecycleCandidate
	work       cluster.SessionWork
	stop       context.CancelFunc
}
type lifecycleHarness struct {
	t     *testing.T
	ctx   context.Context
	cfg   natsresources.Config
	nodes [2]*lifecycleReplica
	id    int32
	now   time.Time
}

func newLifecycleHarness(t *testing.T) *lifecycleHarness {
	t.Helper()
	if os.Getenv("NATS_INTEGRATION") != "1" {
		t.Skip("requires pinned three-node NATS fixture")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	t.Cleanup(cancel)
	port := 4222
	if raw := os.Getenv("NATS_TEST_PORT_BASE"); raw != "" {
		var err error
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
	h := &lifecycleHarness{t: t, ctx: ctx, cfg: cfg, id: int32(400000 + time.Now().UnixNano()%1000000), now: time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)}
	h.start(0)
	h.start(1)
	t.Cleanup(func() {
		for _, n := range h.nodes {
			n.stop()
			n.nc.Close()
		}
	})
	first := h.owner(sessionRef(h.id))
	writer := h.nodes[first].candidate.Writer
	writer.Plan = cluster.SessionLifecyclePlanner(func(_ context.Context, ref *pb.AggregateRef) (*cluster.Aggregate, error) {
		return writer.Projection.Read(ref)
	})
	request := &pb.CommandRequest{ProtocolRevision: 1, CommandId: uuid.NewString(), Aggregate: sessionRef(h.id), Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "lifecycle-test"}, Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_CreateSession{CreateSession: &pb.CreateSession{Id: h.id, Airport: "EKCH", Name: "LIVE", WorkflowId: uuid.NewString()}}}}}
	if err = lifecycleReply(writer.Execute(ctx, request)); err != nil {
		t.Fatal(err)
	}
	return h
}
func (h *lifecycleHarness) start(index int) {
	h.t.Helper()
	nc, err := natsresources.Connect(h.cfg)
	if err != nil {
		h.t.Fatal(err)
	}
	projection, err := cluster.NewProjection(nc, h.cfg)
	if err != nil {
		h.t.Fatal(err)
	}
	runCtx, stop := context.WithCancel(h.ctx)
	go func() { _ = projection.Run(runCtx) }()
	h.await("projection readiness", func() bool { return projection.Ready() == nil })
	store := cluster.NATSStore{JS: projection.JS}
	owner, err := cluster.NewOwnerRuntime(nc, projection, store)
	if err != nil {
		h.t.Fatal(err)
	}
	if err = owner.Track(globalRef()); err != nil {
		h.t.Fatal(err)
	}
	if err = owner.Track(sessionRef(h.id)); err != nil {
		h.t.Fatal(err)
	}
	objects, err := projection.JS.ObjectStore("FS_OBJECTS")
	if err != nil {
		h.t.Fatal(err)
	}
	writer := cluster.Writer{Store: store, NodeID: owner.NodeID, Projection: projection, Lease: owner}
	source := cluster.NavigationWeather{Writer: writer, Objects: cluster.NATSObjects{Store: objects}}
	stands, policy := lifecyclePolicyFixture(h.t)
	secrets := cluster.EffectSecrets{Objects: objects, ActiveKeyID: "test", Keys: map[string][]byte{"test": make([]byte, 32)}}
	candidate, err := NewVatsimLifecycleCandidate(source, writer, cluster.StandState{Stands: stands, Policy: policy, Now: func() time.Time { return h.now }}, secrets)
	if err != nil {
		h.t.Fatal(err)
	}
	h.nodes[index] = &lifecycleReplica{nc: nc, projection: projection, owner: owner, candidate: candidate, stop: stop}
	h.nodes[index].work.Departure = candidate.Departure
	h.nodes[index].work.Arrival = candidate.Arrival
	go func() { _ = owner.Run(runCtx) }()
}
func (h *lifecycleHarness) await(what string, check func() bool) {
	h.t.Helper()
	for h.ctx.Err() == nil {
		if check() {
			return
		}
		time.Sleep(30 * time.Millisecond)
	}
	h.t.Fatalf("timed out: %s", what)
}
func (h *lifecycleHarness) owner(ref *pb.AggregateRef) int {
	h.t.Helper()
	selected := -1
	h.await("owner", func() bool {
		for i, n := range h.nodes {
			if n != nil && n.owner.CanWrite(ref) {
				selected = i
				return true
			}
		}
		return false
	})
	return selected
}
func (h *lifecycleHarness) state() *cluster.Aggregate {
	h.t.Helper()
	owner := h.owner(sessionRef(h.id))
	state, err := h.nodes[owner].projection.Read(sessionRef(h.id))
	if err != nil {
		h.t.Fatal(err)
	}
	return state
}
func (h *lifecycleHarness) put(flights ...*pb.VatsimFlight) {
	h.t.Helper()
	flights = append(flights, &pb.VatsimFlight{Cid: "999", Callsign: "UNRELATED", State: "prefile", FlightPlan: &pb.VatsimFlightPlan{Origin: "EDDF", Destination: "EGLL", Revision: 1}})
	n := h.nodes[h.owner(globalRef())]
	page := &pb.ProviderPage{Provider: "vatsim", Resource: "network-data/v3", Parsed: &pb.ProviderPage_Vatsim{Vatsim: &pb.VatsimPage{SnapshotAt: timestamppb.New(h.now), Flights: flights}}}
	name, sha, err := n.candidate.Source.PublishProvider(page)
	if err != nil {
		h.t.Fatal(err)
	}
	reply, err := n.candidate.Source.PutCheckpointFor(h.ctx, globalRef(), uuid.NewString(), &pb.ProviderCheckpoint{Provider: "vatsim", Resource: "network-data/v3", ObjectName: name, Sha256: sha})
	if err != nil || lifecycleReply(reply) != nil {
		h.t.Fatalf("checkpoint %v %v", reply, err)
	}
	for _, node := range h.nodes {
		if node.nc.IsClosed() {
			continue
		}
		h.await("checkpoint replication", func() bool {
			cp, _, _, err := node.candidate.Source.CheckpointRevisionFor(h.ctx, globalRef(), "vatsim", "network-data/v3")
			return err == nil && cp != nil && cp.Sha256 == sha
		})
	}
}
func lifecycleFlight(callsign, origin, destination, state string) *pb.VatsimFlight {
	return &pb.VatsimFlight{Cid: "12345", Callsign: callsign, State: state, Latitude: 50, Longitude: 8, Altitude: 30000, FlightPlan: &pb.VatsimFlightPlan{Origin: origin, Destination: destination, AircraftShort: "A320", Revision: 1}}
}
func (h *lifecycleHarness) callbacks() {
	h.t.Helper()
	// The real SessionWork retries on its next tick when renewal temporarily
	// fences a callback. Exercise that same admission rule without bypassing it.
	for _, departure := range []bool{true, false} {
		h.await("admitted lifecycle callback", func() bool {
			node := h.nodes[h.owner(sessionRef(h.id))]
			callback := node.work.Arrival
			if departure {
				callback = node.work.Departure
			}
			err := callback(h.ctx, h.id)
			if err != nil && !lifecycleTransient(err) {
				h.t.Fatal(err)
			}
			return err == nil
		})
	}
}

func lifecycleTransient(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	return strings.Contains(message, "status:UNAVAILABLE") || strings.Contains(message, "status: UNAVAILABLE") || strings.Contains(message, "session is not owned") || strings.Contains(message, "master sync is unavailable") || strings.Contains(message, "lifecycle inputs did not settle")
}
func (h *lifecycleHarness) edit(callsign string, mutate func(*pb.Strip)) {
	h.t.Helper()
	n := h.nodes[h.owner(sessionRef(h.id))]
	state := h.state()
	old := state.Indexes[pb.EntityKind_STRIP][callsign]
	s := proto.Clone(old.GetValue().GetStrip()).(*pb.Strip)
	mutate(s)
	writer := n.candidate.Writer
	writer.Plan = cluster.PlanStrip
	strips := cluster.StripState{Store: cluster.LocalLifecycleStore{Writer: writer}}
	reply, err := strips.Put(h.ctx, h.id, s, old.Revision)
	if err != nil || lifecycleReply(reply) != nil {
		h.t.Fatalf("edit strip: %v %v", reply, err)
	}
}

type lifecycleFaultStore struct {
	cluster.EventStore
	mu      sync.Mutex
	after   bool
	kill    func()
	request *pb.StateEvent
	fired   bool
}

func (s *lifecycleFaultStore) Publish(ctx context.Context, subject string, expected uint64, data []byte) (uint64, error) {
	event := &pb.StateEvent{}
	if err := pb.UnmarshalStrict(data, event); err != nil {
		return 0, err
	}
	if event.GetActor().GetId() != "vatsim-lifecycle" {
		return s.EventStore.Publish(ctx, subject, expected, data)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fired {
		return 0, errors.New("owner stopped")
	}
	s.fired = true
	s.request = event
	if !s.after {
		s.kill()
		return 0, errors.New("owner died before lifecycle commit")
	}
	seq, err := s.EventStore.Publish(ctx, subject, expected, data)
	s.kill()
	if err != nil {
		return seq, err
	}
	return 0, errors.New("owner died after lifecycle commit before PubAck")
}

func TestVatsimLifecycleTwoReplicaCommitFailuresAndReservationRecovery(t *testing.T) {
	for _, after := range []bool{false, true} {
		t.Run(fmt.Sprintf("after_commit_%t", after), func(t *testing.T) {
			h := newLifecycleHarness(t)
			flight := lifecycleFlight("SAS123", "EKCH", "EDDF", "prefile")
			h.put(flight)
			first := h.owner(sessionRef(h.id))
			second := 1 - first
			for _, n := range h.nodes {
				n.candidate.AllowPrefiles = true
			}
			if err := h.nodes[second].work.Departure(h.ctx, h.id); err == nil {
				t.Fatal("nonowner lifecycle accepted")
			}
			original := h.nodes[first].candidate.Writer.Store
			fault := &lifecycleFaultStore{EventStore: original, after: after, kill: func() { h.nodes[first].stop(); h.nodes[first].nc.Close() }}
			h.nodes[first].candidate.Writer.Store = fault
			h.await("production transition failure boundary", func() bool {
				err := h.nodes[first].work.Departure(h.ctx, h.id)
				if fault.fired {
					return true
				}
				if err != nil && !lifecycleTransient(err) {
					t.Fatal(err)
				}
				return false
			})
			if owner := h.owner(sessionRef(h.id)); owner != second {
				t.Fatal("takeover owner mismatch")
			}
			h.callbacks()
			state := h.state()
			a := state.Indexes[pb.EntityKind_STAND_ASSIGNMENT]["SAS123"].GetValue().GetStandAssignment()
			if a == nil || a.Stage != StageReserved || a.ExpiresAt == nil {
				t.Fatalf("reservation missing after takeover: %v", a)
			}
			if err := h.nodes[second].projection.Snapshots.Save(state); err != nil {
				t.Fatal(err)
			}
			revision := a.Revision
			effects := len(state.Effects)
			h.callbacks()
			state = h.state()
			if state.Indexes[pb.EntityKind_STAND_ASSIGNMENT]["SAS123"].Revision != revision || len(state.Effects) != effects {
				t.Fatal("replay duplicated transition/effect")
			}
			h.start(first)
			h.nodes[first].candidate.AllowPrefiles = true
			h.await("snapshot replay", func() bool {
				s, err := h.nodes[first].projection.Read(sessionRef(h.id))
				return err == nil && s.Indexes[pb.EntityKind_STAND_ASSIGNMENT]["SAS123"] != nil
			})
			h.now = h.now.Add(16 * time.Minute)
			h.callbacks()
			if h.state().Indexes[pb.EntityKind_STAND_ASSIGNMENT]["SAS123"] != nil {
				t.Fatal("persisted reservation deadline not recovered with prefile present")
			}
			h.callbacks()
			if h.state().Indexes[pb.EntityKind_STAND_ASSIGNMENT]["SAS123"] != nil {
				t.Fatal("replayed generation recreated an expired reservation")
			}
			h.put()
			h.callbacks()
			if h.state().Indexes[pb.EntityKind_STAND_ASSIGNMENT]["SAS123"] != nil {
				t.Fatal("persisted reservation deadline not recovered")
			}
		})
	}
}

func TestVatsimLifecycleTwoReplicaArrivalCancellationWrongStandAndEffects(t *testing.T) {
	h := newLifecycleHarness(t)
	dep := lifecycleFlight("SASDEP", "EKCH", "EDDF", "prefile")
	arr := lifecycleFlight("SASARR", "EDDF", "EKCH", "online")
	h.put(dep, arr)
	h.callbacks()
	if h.state().Indexes[pb.EntityKind_STAND_ASSIGNMENT]["SASDEP"] != nil {
		t.Fatal("default prefile allocation enabled")
	}
	h.edit("SASARR", func(s *pb.Strip) { s.Eldt = timestamppb.New(h.now.Add(30 * time.Minute)) })
	h.callbacks()
	if h.state().Indexes[pb.EntityKind_STAND_ASSIGNMENT]["SASARR"].GetValue().GetStandAssignment().Stage != StageEstimated {
		t.Fatal("arrival ESTIMATED missing")
	}
	h.edit("SASARR", func(s *pb.Strip) { s.Eldt = timestamppb.New(h.now.Add(8 * time.Minute)) })
	h.callbacks()
	if h.state().Indexes[pb.EntityKind_STAND_ASSIGNMENT]["SASARR"].GetValue().GetStandAssignment().Stage != StageAssigned {
		t.Fatal("arrival ASSIGNED missing")
	}
	h.edit("SASARR", func(s *pb.Strip) { s.Eldt = timestamppb.New(h.now.Add(time.Minute)) })
	h.callbacks()
	if h.state().Indexes[pb.EntityKind_STAND_ASSIGNMENT]["SASARR"].GetValue().GetStandAssignment().Stage != StageConfirmed {
		t.Fatal("arrival CONFIRMED missing")
	}
	first := h.owner(sessionRef(h.id))
	second := 1 - first
	n := h.nodes[first]
	presence, err := cluster.NewSocketPresenceLease(n.projection.Presence, n.owner.NodeID, h.id, "777777", "EKCH_DEL", "DEL", false, pb.ClientPresence_EUROSCOPE)
	if err != nil {
		t.Fatal(err)
	}
	socketCtx, stopSocket := context.WithCancel(h.ctx)
	defer stopSocket()
	go func() { _ = presence.Run(socketCtx) }()
	h.await("DEL presence", func() bool { return n.candidate.deliveryCID(h.state(), h.id) == "777777" })
	standWriter := n.candidate.Writer
	standState := n.candidate.Stands
	standWriter.Plan = standState.PlanStand
	zero := uint64(0)
	stripWriter := n.candidate.Writer
	stripWriter.Plan = cluster.PlanStrip
	stripState := cluster.StripState{Store: cluster.LocalLifecycleStore{Writer: stripWriter}}
	if reply, err := stripState.Put(h.ctx, h.id, &pb.Strip{Callsign: "BLOCKER", Departure: "EKCH", Destination: "EDDF", AircraftType: "A320", Bay: "NOT_CLEARED", HasFlightPlan: true}, 0); err != nil || lifecycleReply(reply) != nil {
		t.Fatalf("blocker strip: %v %v", reply, err)
	}
	blockRequest := &pb.CommandRequest{ProtocolRevision: 1, CommandId: uuid.NewString(), Aggregate: sessionRef(h.id), Actor: &pb.Actor{Kind: pb.Actor_CONTROLLER, Id: "777777"}, ExpectedEntityRevision: &zero, Command: &pb.CommandRequest_Client{Client: &pb.ClientCommand{Action: &pb.ClientCommand_Stand{Stand: &pb.StandAction{Callsign: "BLOCKER", Stand: "B1", Change: &pb.StandAction_Manual{Manual: &pb.ManualStand{}}}}}}}
	if err = lifecycleReply(standWriter.Execute(h.ctx, blockRequest)); err != nil {
		t.Fatal(err)
	}
	a1, _ := n.candidate.Stands.Stands.Lookup("EKCH", "B1")
	dep.State = "online"
	dep.Latitude, dep.Longitude, dep.Altitude = a1.Latitude, a1.Longitude, 0
	h.put(dep, arr)
	h.edit("SASDEP", func(s *pb.Strip) { s.VatsimOnly = false; s.Bay = "NOT_CLEARED"; s.GroundState = "STAND" })
	h.callbacks()
	state := h.state()
	assignment := state.Indexes[pb.EntityKind_STAND_ASSIGNMENT]["SASDEP"].GetValue().GetStandAssignment()
	if assignment == nil || assignment.Stand == "B1" || assignment.ConflictReason == nil || !strings.HasPrefix(*assignment.ConflictReason, wrongStandPendingPrefix) {
		t.Fatalf("protected wrong-stand episode missing: %v", assignment)
	}
	messages := 0
	for _, effect := range state.Effects {
		if secret := effect.GetPrivateMessage(); secret != nil {
			messages++
			if effect.TargetCid != "777777" {
				t.Fatal("message target CID changed")
			}
			body, err := n.candidate.Secrets.OpenPrivateMessage(effect.CommandId, effect.TargetCid, secret.ObjectName, secret.Sha256)
			if err != nil || !strings.Contains(body, "PLEASE RELOCATE") {
				t.Fatalf("wrong-stand durable message: %q %v", body, err)
			}
		}
	}
	if messages != 1 {
		t.Fatalf("want one effect, got %d", messages)
	}
	version := assignment.Revision
	h.callbacks()
	if h.state().Indexes[pb.EntityKind_STAND_ASSIGNMENT]["SASDEP"].Revision != version {
		t.Fatal("wrong-stand replay duplicated")
	}
	n.stop()
	n.nc.Close()
	if h.owner(sessionRef(h.id)) != second {
		t.Fatal("wrong takeover")
	}
	h.callbacks()
	if len(h.state().Effects) != len(state.Effects) {
		t.Fatal("takeover repeated warning effect")
	}
	h.now = h.now.Add(6 * time.Minute)
	h.put(dep)
	h.callbacks()
	if h.state().Indexes[pb.EntityKind_STAND_ASSIGNMENT]["SASARR"] != nil {
		t.Fatal("disappearance did not cancel arrival")
	}
	h.now = h.now.Add(time.Hour)
	h.callbacks()
	assignment = h.state().Indexes[pb.EntityKind_STAND_ASSIGNMENT]["SASDEP"].GetValue().GetStandAssignment()
	if assignment == nil || assignment.ExpiresAt != nil {
		t.Fatal("wrong-stand physical protection lost")
	}
}

func TestVatsimLifecycleTwoReplicaPlanIdentityParkedArrivalAndBlockDeadlines(t *testing.T) {
	h := newLifecycleHarness(t)
	dep := lifecycleFlight("SASDEP", "EKCH", "EDDF", "prefile")
	for _, n := range h.nodes {
		n.candidate.AllowPrefiles = true
	}
	h.put(dep)
	h.callbacks()
	assignment := func(key string) *pb.StandAssignment {
		return h.state().Indexes[pb.EntityKind_STAND_ASSIGNMENT][key].GetValue().GetStandAssignment()
	}
	old := proto.Clone(assignment("SASDEP")).(*pb.StandAssignment)
	h.now = h.now.Add(time.Minute)
	dep.FlightPlan.Revision++
	h.put(dep)
	h.callbacks()
	if a := assignment("SASDEP"); a.Stand != old.Stand || !a.ExpiresAt.AsTime().After(old.ExpiresAt.AsTime()) || a.GetVatsimRevision() != dep.FlightPlan.Revision {
		t.Fatalf("plan change failed to renew in place: %v", a)
	}
	old = proto.Clone(assignment("SASDEP")).(*pb.StandAssignment)
	h.now = h.now.Add(time.Minute)
	dep.Cid = "654321"
	h.put(dep)
	h.callbacks()
	if a := assignment("SASDEP"); a.GetVatsimCid() != 654321 || !a.ExpiresAt.AsTime().After(old.ExpiresAt.AsTime()) {
		t.Fatalf("changed CID with the same revision was ignored: %v", a)
	}
	arr := lifecycleFlight("SASARR", "EDDF", "EKCH", "online")
	first := h.owner(sessionRef(h.id))
	n := h.nodes[first]
	stand, _ := n.candidate.Stands.Stands.Lookup("EKCH", "A2")
	arr.Latitude, arr.Longitude, arr.Altitude = stand.Latitude, stand.Longitude, 0
	h.put(dep, arr)
	h.callbacks()
	h.edit("SASARR", func(s *pb.Strip) { s.VatsimOnly = false; s.GroundState = "PARK"; s.Aldt = timestamppb.New(h.now) })
	h.callbacks()
	if a := assignment("SASARR"); a == nil || a.Stage != StageConfirmed || a.Stand != "A2" || a.ExpiresAt == nil {
		t.Fatalf("parked physical arrival lost retention: %v", a)
	}
	standWriter := n.candidate.Writer
	standWriter.Plan = n.candidate.Stands.PlanStand
	zero := uint64(0)
	block := &pb.CommandRequest{ProtocolRevision: 1, CommandId: uuid.NewString(), Aggregate: sessionRef(h.id), Actor: &pb.Actor{Kind: pb.Actor_CONTROLLER, Id: "777777"}, ExpectedEntityRevision: &zero, Command: &pb.CommandRequest_Client{Client: &pb.ClientCommand{Action: &pb.ClientCommand_Stand{Stand: &pb.StandAction{Stand: "B1", Change: &pb.StandAction_CreateBlock{CreateBlock: &pb.CreateStandBlock{Reason: "closed", ExpiresAt: timestamppb.New(h.now.Add(10 * time.Minute))}}}}}}}
	if err := lifecycleReply(standWriter.Execute(h.ctx, block)); err != nil {
		t.Fatal(err)
	}
	state := h.state()
	if err := n.projection.Snapshots.Save(state); err != nil {
		t.Fatal(err)
	}
	n.stop()
	n.nc.Close()
	if h.owner(sessionRef(h.id)) != 1-first {
		t.Fatal("wrong takeover")
	}
	h.now = h.now.Add(31 * time.Minute)
	h.put(arr)
	h.callbacks()
	state = h.state()
	if state.Indexes[pb.EntityKind_STAND_ASSIGNMENT]["SASARR"] != nil || state.Indexes[pb.EntityKind_STAND_BLOCK]["B1"] != nil || state.Indexes[pb.EntityKind_STRIP]["SASDEP"] != nil {
		t.Fatal("persisted arrival/block deadlines or disappeared prefile cleanup were lost")
	}
	h.callbacks()
	if assignment("SASARR") != nil {
		t.Fatal("retained parked strip recreated an expired assignment")
	}
}
