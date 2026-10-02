package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"FlightStrips/internal/cluster"
	"FlightStrips/internal/euroscopebinary"
	"FlightStrips/internal/frontendbinary"
	"FlightStrips/internal/natsresources"
	"FlightStrips/internal/testing/natscluster"
	pb "FlightStrips/pkg/events/cluster"
	es "FlightStrips/pkg/events/euroscope"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

type faultSocket struct {
	conn        *websocket.Conn
	mu          sync.Mutex
	frames      []*es.Envelope
	discardSync bool
}

func (f *entrypointFixture) seededSession() (string, *pb.AggregateRef, []*faultSocket) {
	f.t.Helper()
	name := "TASK22-" + strings.ToUpper(uuid.NewString())
	plugins := f.concurrentPlugins(name)
	ref := sessionFaultRef(f.session(name))
	f.await("both controllers and elected master", func() bool {
		state, err := f.projection.Read(ref)
		return err == nil && state.Master.GetCid() != "" && len(state.EntitiesByKind(pb.EntityKind_CONTROLLER)) == 2
	})
	f.assertOneMaster(ref, plugins)
	state := f.state(ref)
	master := 0
	if state.Master.Cid == "222222" {
		master = 1
	}
	sendEntrypointFrame(f.t, plugins[master].conn, &es.Envelope{SessionId: ref.GetSession().Id, CommandId: uuid.NewString(), OwnerEpoch: state.Owner.Epoch, MasterEpoch: state.Master.Epoch, Event: &es.Envelope_Sync{Sync: &es.SyncEvent{Strips: []*es.Strip{{Callsign: "SAS123", Origin: "EKCH", Destination: "ESSA", AircraftType: "A320", AssignedSquawk: "1001", Sid: "ODN1C", Runway: "22R", HasFp: true}}, Runways: []*es.Runway{{Name: "22R", Departure: true}, {Name: "22L", Arrival: true}}}}})
	f.await("accepted sync", func() bool {
		state, err := f.projection.Read(ref)
		return err == nil && state.Indexes[pb.EntityKind_STRIP]["SAS123"] != nil && state.Sync != nil
	})
	return name, ref, plugins
}

func (f *entrypointFixture) assertOneMaster(ref *pb.AggregateRef, plugins []*faultSocket) {
	f.t.Helper()
	var roles [2]string
	f.await("one binary master role for the accepted epochs", func() bool {
		state := f.state(ref)
		masters := 0
		for i, socket := range plugins {
			roles[i] = ""
			socket.mu.Lock()
			for _, frame := range socket.frames {
				info := frame.GetSessionInfo()
				if info != nil && info.MasterEpoch == state.Master.Epoch && info.OwnerEpoch == state.Owner.Epoch {
					roles[i] = info.Role
				}
			}
			socket.mu.Unlock()
			want := "slave"
			if state.Master.Cid == []string{"111111", "222222"}[i] {
				want = "master"
			}
			if roles[i] != want {
				return false
			}
			if roles[i] == "master" {
				masters++
			}
		}
		return masters == 1
	})
	f.t.Logf("MASTER_ROLES roles=%v owner_epoch=%d master_epoch=%d cid=%s", roles, f.state(ref).Owner.Epoch, f.state(ref).Master.Epoch, f.state(ref).Master.Cid)
}

func (f *entrypointFixture) concurrentPlugins(name string) []*faultSocket {
	f.t.Helper()
	plugins := make([]*faultSocket, 2)
	start := make(chan struct{})
	var group sync.WaitGroup
	for node := 0; node < 2; node++ {
		group.Add(1)
		go func(node int) {
			defer group.Done()
			<-start
			plugins[node] = f.plugin(node, name, []string{"111111", "222222"}[node])
		}(node)
	}
	close(start)
	group.Wait()
	return plugins
}

func (f *entrypointFixture) assertInitialParity(name string) {
	f.t.Helper()
	var digest string
	f.await("coherent initial projection parity on both compiled servers", func() bool {
		a, b := f.front(0, name), f.front(1, name)
		defer a.Close()
		defer b.Close()
		f.initialMu.Lock()
		first := proto.Clone(f.initial[0]).(*pb.FrontendInitial)
		second := proto.Clone(f.initial[1]).(*pb.FrontendInitial)
		f.initialMu.Unlock()
		if first.AggregateRevision != second.AggregateRevision || first.AirportAggregateRevision != second.AirportAggregateRevision {
			return false
		}
		normalize := func(initial *pb.FrontendInitial) []byte {
			sort.Slice(initial.Entities, func(i, j int) bool {
				a, b := initial.Entities[i], initial.Entities[j]
				if a.Key != b.Key {
					return a.Key < b.Key
				}
				kind := func(e *pb.EntitySnapshot) int {
					return int(e.Value.ProtoReflect().WhichOneof(e.Value.ProtoReflect().Descriptor().Oneofs().ByName("value")).Number())
				}
				return kind(a) < kind(b)
			})
			encoded, err := (proto.MarshalOptions{Deterministic: true}).Marshal(&pb.FrontendInitial{Entities: initial.Entities, AggregateRevision: initial.AggregateRevision, AirportAggregateRevision: initial.AirportAggregateRevision})
			require.NoError(f.t, err)
			return encoded
		}
		firstBytes, secondBytes := normalize(first), normalize(second)
		digest = fixtureDigest(firstBytes)
		return string(firstBytes) == string(secondBytes)
	})
	f.t.Logf("INITIAL_PARITY entities_and_revisions_sha256=%s", digest)
}

func TestServerNATSControllerFreeLongOutage(t *testing.T) {
	if os.Getenv("NATS_TASK22_LONG") != "1" {
		t.Skip("explicitly enable the real over-five-minute outage")
	}
	runControllerFreeOutage(t, 305*time.Second)
}

func TestServerNATSExpiredLeasesRecover(t *testing.T) {
	if os.Getenv("NATS_TASK22") != "1" {
		t.Skip("requires explicit disposable Task22 fixture")
	}
	runControllerFreeOutage(t, 15*time.Second)
}

func runControllerFreeOutage(t *testing.T, duration time.Duration) {
	f := newEntrypointFixture(t, true)
	_, ref, plugins := f.seededSession()
	// Exercise an actual owner RAM overlay, then observe its persisted value
	// before removing controllers and testing recovery of the idle owner.
	state := f.state(ref)
	master := 0
	if state.Master.Cid == "222222" {
		master = 1
	}
	sendEntrypointFrame(t, plugins[master].conn, &es.Envelope{SessionId: ref.GetSession().Id, CommandId: uuid.NewString(), OwnerEpoch: state.Owner.Epoch, MasterEpoch: state.Master.Epoch, Event: &es.Envelope_AircraftPositionUpdate{AircraftPositionUpdate: &es.AircraftPositionUpdateEvent{Callsign: "SAS123", Lat: 55.62, Lon: 12.65, Altitude: 1000}}})
	js, err := f.nc.JetStream()
	require.NoError(t, err)
	positions, err := js.KeyValue(f.resources.Names.Positions)
	require.NoError(t, err)
	positionKey := fmt.Sprintf("%d.SAS123.%d", ref.GetSession().Id, state.Owner.Epoch)
	f.await("owner RAM position persisted before controller-free outage", func() bool {
		entry, getErr := positions.Get(positionKey)
		if getErr != nil {
			return false
		}
		value := &pb.PositionValue{}
		return pb.UnmarshalStrict(entry.Value(), value) == nil && value.GetPosition() != nil && value.OwnerEpoch == state.Owner.Epoch
	})
	for _, socket := range plugins {
		require.NoError(t, socket.conn.Close())
	}
	seedKey := fmt.Sprint(ref.GetSession().Id)
	f.await("persisted controller-free marker", func() bool {
		return f.state(ref).Indexes[pb.EntityKind_SESSION][seedKey].GetValue().GetSession().FirstNoControllerAt != nil
	})
	before := proto.Clone(f.state(ref).Indexes[pb.EntityKind_SESSION][seedKey].GetValue().GetSession()).(*pb.Session)
	started := time.Now()
	for i, p := range f.brokers {
		t.Logf("STOP owned broker=%d pid=%d executable=%s", i, p.command.Process.Pid, p.command.Path)
		p.stop()
	}
	f.await("both compiled processes reject quorum outage", func() bool {
		a, _ := entrypointStatus(f.addresses[0], "/readyz", "")
		b, _ := entrypointStatus(f.addresses[1], "/readyz", "")
		return a == 503 && b == 503
	})
	// Real wall time is intentional. No accelerated domain clock is used.
	time.Sleep(duration)
	require.GreaterOrEqual(t, time.Since(started), duration)
	for i := range f.brokers {
		f.brokers[i] = f.startBroker(i)
	}
	f.ready()
	t.Cleanup(func() {
		if t.Failed() {
			seed := f.state(ref).Indexes[pb.EntityKind_SESSION][seedKey].GetValue().GetSession()
			t.Logf("LONG_OUTAGE_DIAGNOSTIC tombstoned=%t before=%s after=%s paused_at=%s sequence=%d", seed.Tombstoned, before.FirstNoControllerAt.AsTime().Format(time.RFC3339Nano), seed.FirstNoControllerAt.AsTime().Format(time.RFC3339Nano), seed.CleanupPausedAt.AsTime().Format(time.RFC3339Nano), f.state(ref).StreamSequence)
		}
	})
	f.await("outage time persisted without deleting session", func() bool {
		state := f.state(ref)
		seed := state.Indexes[pb.EntityKind_SESSION][seedKey].GetValue().GetSession()
		return !seed.Tombstoned && seed.CleanupPausedAt != nil && seed.FirstNoControllerAt.AsTime().After(before.FirstNoControllerAt.AsTime().Add(duration-time.Second))
	})
	after := f.state(ref).Indexes[pb.EntityKind_SESSION][seedKey].GetValue().GetSession()
	require.False(t, after.Tombstoned)
	t.Logf("LONG_OUTAGE elapsed=%s first_no_controller_before=%s after=%s cleanup_paused_at=%s sequence=%d", time.Since(started), before.FirstNoControllerAt.AsTime().Format(time.RFC3339Nano), after.FirstNoControllerAt.AsTime().Format(time.RFC3339Nano), after.CleanupPausedAt.AsTime().Format(time.RFC3339Nano), f.state(ref).StreamSequence)
}

func TestServerNATSSnapshotStaleEpochAndVersionFence(t *testing.T) {
	if os.Getenv("NATS_TASK22") != "1" {
		t.Skip("requires explicitly owned Task22 fixture")
	}
	f := newEntrypointFixture(t, true)
	name, ref, plugins := f.seededSession()
	fronts := []*websocket.Conn{f.front(0, name), f.front(1, name)}
	state := f.state(ref)
	revision := state.Indexes[pb.EntityKind_STRIP]["SAS123"].Revision
	ids := []string{uuid.NewString(), uuid.NewString()}
	var writes sync.WaitGroup
	for i := 0; i < 2; i++ {
		writes.Add(1)
		go func(i int) {
			defer writes.Done()
			sendEntrypointFrame(t, fronts[i], markedCommand(ids[i], "SAS123", revision, true))
		}(i)
	}
	writes.Wait()
	f.await("both raced commands have durable outcomes", func() bool { return f.state(ref).Ledger[ids[0]] != nil && f.state(ref).Ledger[ids[1]] != nil })
	state = f.state(ref)
	success, conflict := 0, 0
	for _, id := range ids {
		outcome := state.Ledger[id]
		if outcome.Status == pb.CommandOutcome_SUCCEEDED {
			success++
		}
		if outcome.Status == pb.CommandOutcome_FAILED && outcome.ReasonCode == "REVISION_CONFLICT" {
			conflict++
		}
		t.Logf("STRIP_RACE id=%s status=%s reason=%s sequence=%d", id, outcome.Status, outcome.ReasonCode, outcome.CommittedStreamSequence)
	}
	f.assertInitialParity(name)
	require.Equal(t, 1, success)
	require.Equal(t, 1, conflict)
	// An authenticated master socket cannot reuse its older master epoch.
	master := 0
	if state.Master.Cid == "222222" {
		master = 1
	}
	staleID := uuid.NewString()
	sendEntrypointFrame(t, plugins[master].conn, &es.Envelope{SessionId: ref.GetSession().Id, CommandId: staleID, OwnerEpoch: state.Owner.Epoch, MasterEpoch: state.Master.Epoch - 1, Event: &es.Envelope_AssignedSquawk{AssignedSquawk: &es.AssignedSquawkEvent{Callsign: "SAS123", Squawk: "7777"}}})
	time.Sleep(500 * time.Millisecond)
	require.Equal(t, "1001", f.state(ref).Indexes[pb.EntityKind_STRIP]["SAS123"].GetValue().GetStrip().AssignedSquawk)
	require.Nil(t, f.state(ref).Ledger[staleID])
	nonMasterID := uuid.NewString()
	sendEntrypointFrame(t, plugins[1-master].conn, &es.Envelope{SessionId: ref.GetSession().Id, CommandId: nonMasterID, OwnerEpoch: state.Owner.Epoch, MasterEpoch: state.Master.Epoch, Event: &es.Envelope_AssignedSquawk{AssignedSquawk: &es.AssignedSquawkEvent{Callsign: "SAS123", Squawk: "7777"}}})
	time.Sleep(500 * time.Millisecond)
	require.Equal(t, "1001", f.state(ref).Indexes[pb.EntityKind_STRIP]["SAS123"].GetValue().GetStrip().AssignedSquawk)
	require.Nil(t, f.state(ref).Ledger[nonMasterID], "current epochs cannot authorize a non-master socket")
	js, err := f.nc.JetStream()
	require.NoError(t, err)
	store := cluster.NATSStore{JS: js}
	// Publish a valid stale epoch through the real subject CAS. It must advance
	// replay while leaving entity revisions, ledger and effects untouched.
	staleID = uuid.NewString()
	state = f.state(ref)
	oldStrip := proto.Clone(state.Indexes[pb.EntityKind_STRIP]["SAS123"])
	event := &pb.StateEvent{SchemaVersion: 1, EventId: uuid.NewString(), CommandId: &staleID, Aggregate: ref, AggregateRevision: state.Revision + 1, OwnerEpoch: state.Owner.Epoch - 1, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "stale-owner"}, Fact: &pb.StateEvent_DomainChanged{DomainChanged: &pb.DomainChange{}}}
	data, err := proto.Marshal(event)
	require.NoError(t, err)
	subject, err := cluster.Subject(ref)
	require.NoError(t, err)
	var sequence uint64
	f.await("stale epoch appended with exact CAS", func() bool {
		sequence, err = store.Publish(f.ctx, subject, f.state(ref).SubjectSequence, data)
		return err == nil
	})
	require.NoError(t, f.projection.WaitApplied(f.ctx, sequence))
	require.True(t, proto.Equal(oldStrip, f.state(ref).Indexes[pb.EntityKind_STRIP]["SAS123"]))
	require.Nil(t, f.state(ref).Ledger[staleID])
	t.Logf("STALE owner_epoch=%d command_id=%s sequence=%d no_effect=true", event.OwnerEpoch, staleID, sequence)
	// Store a verified snapshot then replace only this fixture's object bytes.
	state = f.state(ref)
	require.NoError(t, f.projection.Snapshots.Save(state))
	adminCfg := f.resources
	for i, u := range adminCfg.URLs {
		adminCfg.URLs[i] = strings.ReplaceAll(u, "backend:backend-local-only", "bootstrap:bootstrap-local-only")
	}
	admin, err := natsresources.Connect(adminCfg)
	require.NoError(t, err)
	defer admin.Close()
	adminJS, err := admin.JetStream()
	require.NoError(t, err)
	objects, err := adminJS.ObjectStore(f.resources.Names.Objects)
	require.NoError(t, err)
	indexEntry, err := f.projection.Snapshots.Index.Get(fmt.Sprintf("session.%d", ref.GetSession().Id))
	require.NoError(t, err)
	index := &pb.SnapshotIndex{}
	require.NoError(t, pb.UnmarshalStrict(indexEntry.Value(), index))
	_, err = objects.PutBytes(index.ObjectName, []byte("task22-corrupt-snapshot"))
	require.NoError(t, err)
	for i := range f.apps {
		f.apps[i].stop()
		f.restart(i)
	}
	for _, id := range ids {
		require.Equal(t, f.outcome(0, id), f.outcome(1, id))
		require.NotEmpty(t, f.outcome(0, id))
	}
	t.Logf("SNAPSHOT corrupt_object=%s original_sha256=%s replay_sequence=%d", index.ObjectName, index.Sha256, f.state(ref).StreamSequence)
	// One pinned broker loss retains R3 quorum and acknowledged command state.
	f.brokers[2].stop()
	f.ready()
	for _, id := range ids {
		require.NotEmpty(t, f.outcome(1, id))
	}
	f.brokers[2] = f.startBroker(2)
	require.Eventually(t, func() bool { return natscluster.WaitForQuorum(f.ctx, admin) == nil }, 30*time.Second, 100*time.Millisecond)
	// Unknown internal versions poison this disposable stream deliberately. Both
	// readers must fence admissions; readiness cannot silently skip the event.
	unknown := &pb.StateEvent{SchemaVersion: 2, EventId: uuid.NewString(), Aggregate: &pb.AggregateRef{Target: &pb.AggregateRef_Airport{Airport: &pb.AirportRef{Icao: "ZZZZ"}}}, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "future-reader-fixture"}, Fact: &pb.StateEvent_OwnerClaimed{OwnerClaimed: &pb.OwnerTerm{NodeId: "future-reader-fixture", Epoch: 1}}}
	data, err = proto.Marshal(unknown)
	require.NoError(t, err)
	subject, err = cluster.Subject(unknown.Aggregate)
	require.NoError(t, err)
	sequence, err = store.Publish(f.ctx, subject, 0, data)
	require.NoError(t, err)
	f.await("unknown event fences both readers", func() bool {
		a, _ := entrypointStatus(f.addresses[0], "/readyz", "")
		b, _ := entrypointStatus(f.addresses[1], "/readyz", "")
		return a == 503 && b == 503
	})
	for _, address := range f.addresses {
		status, _ := entrypointStatus(address, "/healthz", "")
		require.Equal(t, 200, status)
		status, _ = entrypointStatus(address, "/euroscopeEvents", "")
		require.Equal(t, 503, status)
	}
	t.Logf("VERSION_FENCE schema_version=2 stream_sequence=%d readiness=[503,503] liveness=[200,200]", sequence)
}

func (f *entrypointFixture) dial(node int, path, protocol string) *websocket.Conn {
	f.t.Helper()
	c, _, err := (&websocket.Dialer{Subprotocols: []string{protocol}}).Dial("ws://"+f.addresses[node]+path, nil)
	require.NoError(f.t, err)
	f.t.Cleanup(func() { _ = c.Close() })
	return c
}

func fixtureToken(t *testing.T, cid string) string {
	signed := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"exp": time.Now().Add(time.Hour).Unix(), "aud": "backend-dev", "vatsim/cid": cid, "vatsim/rating": 5})
	signed.Header["kid"] = "fixture"
	token, err := signed.SignedString([]byte("entrypoint-fixture-signing-secret"))
	require.NoError(t, err)
	return token
}

func (f *entrypointFixture) plugin(node int, name, cid string, role ...string) *faultSocket {
	f.t.Helper()
	s := &faultSocket{conn: f.dial(node, "/euroscopeEvents", euroscopebinary.Subprotocol)}
	sendEntrypointFrame(f.t, s.conn, &es.Envelope{Event: &es.Envelope_Token{Token: &es.TokenEvent{Token: fixtureToken(f.t, cid), ProtocolRevision: 2}}})
	callsign, position := fmt.Sprintf("EKCH_%s_TWR", cid), "118.100"
	if len(role) > 0 {
		callsign = role[0]
		position = "119.905"
	}
	sendEntrypointFrame(f.t, s.conn, &es.Envelope{Event: &es.Envelope_Login{Login: &es.LoginEvent{Airport: "EKCH", Connection: name, Callsign: callsign, Position: position}}})
	go func() {
		for {
			kind, data, err := s.conn.ReadMessage()
			if err != nil {
				return
			}
			frame := &es.Envelope{}
			if kind != websocket.BinaryMessage || pb.UnmarshalStrict(data, frame) != nil {
				return
			}
			s.mu.Lock()
			if !s.discardSync || frame.GetBackendSync() == nil {
				s.frames = append(s.frames, frame)
			}
			s.mu.Unlock()
		}
	}()
	return s
}

func (s *faultSocket) effect(id string) []*es.Envelope {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result []*es.Envelope
	for _, frame := range s.frames {
		if frame.CommandId == id && frame.GetSendPrivateMessage() != nil {
			result = append(result, frame)
		}
	}
	return result
}

func sessionFaultRef(id int32) *pb.AggregateRef {
	return &pb.AggregateRef{Target: &pb.AggregateRef_Session{Session: &pb.SessionRef{Id: id}}}
}
func (f *entrypointFixture) state(ref *pb.AggregateRef) *cluster.Aggregate {
	f.t.Helper()
	state, err := f.projection.Read(ref)
	require.NoError(f.t, err)
	return state
}
func (f *entrypointFixture) await(label string, fn func() bool) {
	f.t.Helper()
	require.Eventually(f.t, fn, 45*time.Second, 25*time.Millisecond, label)
}
func (f *entrypointFixture) session(name string) int32 {
	f.t.Helper()
	var id int32
	f.await("one concurrent session registration", func() bool {
		state, err := f.projection.Read(&pb.AggregateRef{Target: &pb.AggregateRef_Global{Global: &pb.GlobalRef{}}})
		if err != nil {
			return false
		}
		count := 0
		for _, entity := range state.EntitiesByKind(pb.EntityKind_SESSION_REGISTRY) {
			if entry := entity.Value.GetSessionRegistry(); entry.Name == name {
				count++
				id = entry.Id
			}
		}
		return count == 1 && id > 0
	})
	return id
}
func (f *entrypointFixture) front(node int, name string) *websocket.Conn {
	f.t.Helper()
	c := f.dial(node, "/frontEndEvents", frontendbinary.Subprotocol)
	sendEntrypointFrame(f.t, c, &pb.FrontendFrame{ProtocolRevision: 2, Frame: &pb.FrontendFrame_Authenticate{Authenticate: &pb.FrontendAuthenticate{BearerToken: f.token, Airport: "EKCH", SessionName: name}}})
	_ = c.SetReadDeadline(time.Now().Add(15 * time.Second))
	kind, data, err := c.ReadMessage()
	require.NoError(f.t, err)
	require.Equal(f.t, websocket.BinaryMessage, kind)
	initial := &pb.FrontendFrame{}
	require.NoError(f.t, pb.UnmarshalStrict(data, initial))
	require.NotNil(f.t, initial.GetInitial())
	f.initialMu.Lock()
	f.initial[node] = proto.Clone(initial.GetInitial()).(*pb.FrontendInitial)
	f.initialMu.Unlock()
	_ = c.SetReadDeadline(time.Time{})
	// One reader verifies strict per-aggregate ordering, including events buffered
	// while the initial frame was assembled. It never treats send completion as success.
	readerDone := make(chan struct{})
	f.t.Cleanup(func() { _ = c.Close(); <-readerDone })
	go func() {
		defer close(readerDone)
		last := map[string]uint64{"session": initial.GetInitial().AggregateRevision, "airport": initial.GetInitial().AirportAggregateRevision}
		for {
			kind, data, err := c.ReadMessage()
			if err != nil {
				return
			}
			frame := &pb.FrontendFrame{}
			if kind != websocket.BinaryMessage || pb.UnmarshalStrict(data, frame) != nil {
				f.t.Error("invalid binary frontend frame")
				return
			}
			if result := frame.GetActionResult(); result != nil {
				f.t.Logf("FRONTEND_RESULT id=%s status=%s reason=%s detail=%s", result.RequestId, result.Status, result.ReasonCode, result.Detail)
			}
			if delta := frame.GetDelta(); delta != nil {
				key := "session"
				if delta.Aggregate.GetAirport() != nil {
					key = "airport"
				}
				if delta.AggregateRevision <= last[key] {
					f.t.Errorf("snapshot/delta ordering: %s %d <= %d", key, delta.AggregateRevision, last[key])
					return
				}
				last[key] = delta.AggregateRevision
			}
		}
	}()
	return c
}
func faultCommand(id string, action *pb.ClientCommand, revision *uint64) *pb.FrontendFrame {
	return &pb.FrontendFrame{ProtocolRevision: 2, Frame: &pb.FrontendFrame_Command{Command: &pb.FrontendCommand{RequestId: id, ExpectedEntityRevision: revision, Action: action}}}
}
func markedCommand(id, callsign string, revision uint64, marked bool) *pb.FrontendFrame {
	return faultCommand(id, &pb.ClientCommand{Action: &pb.ClientCommand_Strip{Strip: &pb.StripAction{Callsign: callsign, Change: &pb.StripAction_SetMarked{SetMarked: &pb.SetMarked{Marked: marked}}}}}, &revision)
}
func privateCommand(id string) *pb.FrontendFrame {
	return faultCommand(id, &pb.ClientCommand{Action: &pb.ClientCommand_Message{Message: &pb.MessageAction{Send: &pb.MessageAction_PrivateMessage{PrivateMessage: &pb.PrivateMessage{TargetCid: "SAS123", Text: "local fault fixture"}}}}}, nil)
}
func (f *entrypointFixture) outcome(node int, id string) string {
	status, body := entrypointStatus(f.addresses[node], "/api/commands/"+id, f.token)
	if status != 200 {
		return ""
	}
	var result struct{ Status string }
	if json.Unmarshal(body, &result) != nil {
		return ""
	}
	return result.Status
}
func (f *entrypointFixture) arm(point, id string) {
	f.t.Helper()
	dir := filepath.Join(f.dir, "gate")
	_ = os.Remove(filepath.Join(dir, "reached.json"))
	data, err := json.Marshal(map[string]string{"Point": point, "CommandID": id})
	require.NoError(f.t, err)
	require.NoError(f.t, os.WriteFile(filepath.Join(dir, "control.json"), data, 0600))
}
func (f *entrypointFixture) killAt(point, id string) int {
	f.t.Helper()
	var checkpoint struct {
		Point     string
		CommandID string `json:"command_id"`
		Sequence  uint64 `json:"stream_sequence"`
		PID       int
		At        time.Time
	}
	f.await("fault barrier "+point, func() bool {
		data, err := os.ReadFile(filepath.Join(f.dir, "gate", "reached.json"))
		return err == nil && json.Unmarshal(data, &checkpoint) == nil && checkpoint.Point == point && checkpoint.CommandID == id
	})
	index := -1
	for i, p := range f.apps {
		if p.command.Process.Pid == checkpoint.PID && !p.stopped {
			index = i
		}
	}
	require.NotEqual(f.t, -1, index, "checkpoint PID must match a live process created by this fixture")
	f.t.Logf("FAULT point=%s command_id=%s stream_sequence=%d pid=%d reached=%s killed=%s", point, id, checkpoint.Sequence, checkpoint.PID, checkpoint.At.Format(time.RFC3339Nano), time.Now().UTC().Format(time.RFC3339Nano))
	f.apps[index].stop()
	require.NoError(f.t, os.Remove(filepath.Join(f.dir, "gate", "control.json")))
	return index
}
func (f *entrypointFixture) restart(node int) {
	f.apps[node] = startFixtureProcess(f.t, f.binary, f.backend, f.env, "-addr", f.addresses[node])
	f.ready()
}

func TestServerNATSFaultBoundaries(t *testing.T) {
	if os.Getenv("NATS_TASK22") != "1" {
		t.Skip("requires explicit Task22 native disposable fixture")
	}
	// The complete matrix uses real processes, the production entrypoint and
	// authentication, binary frontend/plugin clients, real PubAck and JSON HTTP.
	for _, point := range []string{"before-publish-domain", "after-puback-domain", "before-publish-effect-DISPATCH_CLAIMED", "after-puback-effect-DISPATCH_CLAIMED", "before-socket-write", "after-socket-write", "before-publish-effect-EXECUTED", "after-puback-effect-EXECUTED"} {
		t.Run(point, func(t *testing.T) {
			f := newEntrypointFixture(t, true)
			name := "TASK22-" + strings.ToUpper(uuid.NewString())
			callsign := "SAS123"
			plugins := f.concurrentPlugins(name)
			session := f.session(name)
			ref := sessionFaultRef(session)
			f.await("master and both controllers", func() bool {
				s, e := f.projection.Read(ref)
				return e == nil && s.Master.GetCid() != "" && len(s.EntitiesByKind(pb.EntityKind_CONTROLLER)) == 2
			})
			f.assertOneMaster(ref, plugins)
			state := f.state(ref)
			master := 0
			if state.Master.Cid == "222222" {
				master = 1
			}
			sendEntrypointFrame(t, plugins[master].conn, &es.Envelope{SessionId: session, CommandId: uuid.NewString(), OwnerEpoch: state.Owner.Epoch, MasterEpoch: state.Master.Epoch, Event: &es.Envelope_Sync{Sync: &es.SyncEvent{Strips: []*es.Strip{{Callsign: callsign, Origin: "EKCH", Destination: "EGLL", AircraftType: "A320", AssignedSquawk: "1001", Sid: "ODN1C", Runway: "22R", HasFp: true}}, Runways: []*es.Runway{{Name: "22R", Departure: true}, {Name: "22L", Arrival: true}}}}})
			f.await("accepted sync", func() bool {
				s, e := f.projection.Read(ref)
				return e == nil && s.Indexes[pb.EntityKind_STRIP][callsign] != nil && s.Sync != nil
			})
			// Keep the effect target on the other node from the owner. An owner
			// crash then tests recovery without conflating a missing target socket.
			_, presence, err := f.projection.ObservationSnapshot(session)
			require.NoError(t, err)
			ownerNode := 0
			for _, entry := range presence {
				client := entry.Value.GetClient()
				if client != nil && client.Cid == "222222" && client.NodeId == f.state(ref).Owner.NodeId {
					ownerNode = 1
				}
			}
			targetNode := 1 - ownerNode
			targetCID := []string{"111111", "222222"}[targetNode]
			f.token = fixtureToken(t, targetCID)
			front := f.front(targetNode, name)
			id := uuid.NewString()
			domain := strings.HasSuffix(point, "domain")
			command := privateCommand(id)
			if domain {
				command = markedCommand(id, callsign, f.state(ref).Indexes[pb.EntityKind_STRIP][callsign].Revision, true)
			}
			f.arm(point, id)
			sendEntrypointFrame(t, front, command)
			if strings.HasSuffix(point, "EXECUTED") {
				f.await("one CID-targeted socket delivery", func() bool { return len(plugins[targetNode].effect(id)) == 1 })
				frame := plugins[targetNode].effect(id)[0]
				sendEntrypointFrame(t, plugins[targetNode].conn, &es.Envelope{SessionId: session, CommandId: id, OwnerEpoch: frame.OwnerEpoch, MasterEpoch: frame.MasterEpoch, Event: &es.Envelope_CommandResult{CommandResult: &es.CommandResultEvent{CommandId: id, Status: es.CommandResultEvent_EXECUTED, Reason: es.CommandResultEvent_OK}}})
			}
			node := f.killAt(point, id)
			survivor := 1 - node
			f.await("surviving compiled server ready", func() bool { status, _ := entrypointStatus(f.addresses[survivor], "/readyz", ""); return status == 200 })
			if point == "before-publish-domain" {
				require.Nil(t, f.state(ref).Ledger[id], "no commit before publish")

			}
			if point == "before-publish-domain" {
				require.Nil(t, f.state(ref).Ledger[id])
				f.restart(node)
				require.Nil(t, f.state(ref).Ledger[id], "no automatic command replay after restart")
				t.Logf("OUTCOME command_id=%s status=NOT_FOUND stream_sequence=0", id)
				return
			}
			if domain {
				f.await("durable successful command", func() bool { return f.outcome(survivor, id) == "succeeded" })
			} else {
				want := "unknown"
				if point == "after-puback-effect-EXECUTED" {
					want = "succeeded"
				}
				// Before a claim is published another owner may dispatch once. Supply
				// no result, so the exact terminal result must still be UNKNOWN.
				f.await("durable effect outcome "+want, func() bool { return f.outcome(survivor, id) == want })
				state := f.state(ref)
				effect := state.Effects[id]
				require.Equal(t, targetCID, effect.TargetCid)
				require.Empty(t, plugins[1-targetNode].effect(id), "CID must never change")
				count := len(plugins[targetNode].effect(id))
				require.LessOrEqual(t, count, 1, "no duplicate external effect")
				if point == "after-puback-effect-DISPATCH_CLAIMED" || point == "before-socket-write" {
					require.Zero(t, count, "claim committed without socket delivery")
				}
				if strings.HasSuffix(point, "EXECUTED") || point == "after-socket-write" {
					require.Equal(t, 1, count)
				}
			}
			f.restart(node)
			for i := 0; i < 2; i++ {
				require.Equal(t, f.outcome(survivor, id), f.outcome(i, id), "restarted replica JSON parity")
			}
			state = f.state(ref)
			outcome := state.Ledger[id]
			require.NotNil(t, outcome)
			t.Logf("OUTCOME command_id=%s status=%s reason=%s sequence=%d revision=%d effect=%s", id, outcome.Status, outcome.ReasonCode, outcome.CommittedStreamSequence, outcome.AggregateRevision, state.Effects[id].GetStatus())
			// Replaying a terminal logical action must not create a second effect.
			if !domain {
				reply := f.route(&pb.CommandRequest{ProtocolRevision: 1, CommandId: id, Aggregate: ref, Actor: &pb.Actor{Kind: pb.Actor_CONTROLLER, Id: targetCID, SessionId: &session}, Command: &pb.CommandRequest_Client{Client: command.GetCommand().Action}})
				require.Equal(t, outcome.Status, reply.GetOutcome().GetStatus(), "same logical action replays its exact durable outcome")
				time.Sleep(time.Second)
				require.LessOrEqual(t, len(plugins[targetNode].effect(id)), 1)
			}
		})
	}
}
