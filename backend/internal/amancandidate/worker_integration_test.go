package amancandidate

import (
	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/navdata"
	"FlightStrips/internal/aman/navdata/airacnet"
	"FlightStrips/internal/aman/terminal"
	"FlightStrips/internal/cluster"
	"FlightStrips/internal/natsresources"
	"FlightStrips/internal/testing/natscluster"
	pb "FlightStrips/pkg/events/cluster"
	"context"
	"crypto/sha256"
	"fmt"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

type replica struct {
	nc     *nats.Conn
	p      *cluster.Projection
	owner  *cluster.OwnerRuntime
	writer cluster.Writer
	source cluster.NavigationWeather
	worker *Worker
	router *cluster.CommandRouter
	stop   context.CancelFunc
}
type fixture struct {
	ctx      context.Context
	nodes    [2]*replica
	airport  string
	session  int32
	now      time.Time
	terminal terminal.Configuration
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	if os.Getenv("NATS_INTEGRATION") != "1" {
		t.Skip("requires pinned three-node NATS fixture")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)
	port := 4222
	if v := os.Getenv("NATS_TEST_PORT_BASE"); v != "" {
		var err error
		port, err = strconv.Atoi(v)
		require.NoError(t, err)
	}
	urls := func(user string) []string {
		var r []string
		for i := 0; i < 3; i++ {
			r = append(r, fmt.Sprintf("nats://%s:%s-local-only@127.0.0.1:%d", user, user, port+i))
		}
		return r
	}
	cfg := natsresources.Config{URLs: urls("bootstrap"), ConnectTimeout: 3 * time.Second, RequestTimeout: 3 * time.Second, Names: natsresources.RequiredNames}
	admin, err := natsresources.Connect(cfg)
	require.NoError(t, err)
	defer admin.Close()
	require.NoError(t, natscluster.WaitForQuorum(ctx, admin))
	require.NoError(t, natsresources.Bootstrap(ctx, admin, cfg))
	cfg.URLs = urls("backend")
	seed := uuid.New()
	airport := string([]byte{'A' + seed[0]%26, 'A' + seed[1]%26, 'A' + seed[2]%26, 'A' + seed[3]%26})
	f := &fixture{ctx: ctx, airport: airport, session: int32(seed[4])<<16 | int32(seed[5])<<8 | int32(seed[6]), now: time.Now().UTC().Truncate(time.Second)}
	f.session += 1000
	f.terminal = terminal.Configuration{Airport: navdata.AirportID(airport), ConfigVersion: "fixture-v1", RunwayGroups: []terminal.RunwayGroup{{ID: "ARRIVAL-22", Aliases: []aman.RunwayGroupID{"22L"}, Runways: []navdata.RunwayID{"22L"}, FinalApproaches: []terminal.FinalApproachDefinition{{Runway: "22L", Threshold: terminal.ThresholdDefinition{Position: terminal.CoordinateDefinition{LatitudeDeg: 55.8, LongitudeDeg: 12}}}}}},
		Feeders: []terminal.Feeder{{ID: "TESPI"}}, Paths: []terminal.Path{{Feeder: "TESPI", STARFamily: "TESPI", FeederFix: "TNO", RunwayGroup: "ARRIVAL-22", Fixes: []navdata.FixID{"TESPI", "TNO", "THR"}, SelectedHolding: "TESPI-HOLD"}}}
	for i := range f.nodes {
		nc, err := natsresources.Connect(cfg)
		require.NoError(t, err)
		p, err := cluster.NewProjection(nc, cfg)
		require.NoError(t, err)
		runCtx, stop := context.WithCancel(ctx)
		go func() { _ = p.Run(runCtx) }()
		require.Eventually(t, func() bool { return p.Ready() == nil }, 10*time.Second, 20*time.Millisecond)
		store := cluster.NATSStore{JS: p.JS}
		owner, err := cluster.NewOwnerRuntime(nc, p, store)
		require.NoError(t, err)
		for _, ref := range []*pb.AggregateRef{globalRef(), airportRef(airport), sessionRef(f.session)} {
			require.NoError(t, owner.Track(ref))
		}
		writer := cluster.Writer{Store: store, NodeID: owner.NodeID, Projection: p, Lease: owner}
		objects, err := p.JS.ObjectStore("FS_OBJECTS")
		require.NoError(t, err)
		source := cluster.NavigationWeather{Writer: writer, Objects: cluster.NATSObjects{Store: objects}}
		destination := DestinationPlanner(cluster.LocalLifecycleStore{Writer: writer})
		routerWriter := writer
		routerWriter.Plan = func(ctx context.Context, r *pb.CommandRequest, s *cluster.Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
			if r.GetSystem().GetApplyAmanSession() != nil {
				return destination(ctx, r, s)
			}
			return cluster.PlanStrip(ctx, r, s)
		}
		router := &cluster.CommandRouter{NC: nc, Projection: p, Lease: owner, Writer: routerWriter}
		resolver, err := airacnet.New(airacnet.Config{BaseURL: "http://127.0.0.1:1", Retries: 0, Timeout: time.Second, Checkpoints: airacnet.NewMemoryCheckpoints()})
		require.NoError(t, err)
		worker, err := New(Options{Source: source, State: cluster.AmanAdapter{Writer: writer}, Sessions: cluster.RoutedLifecycleStore{Router: router, Projection: p}, Projection: p, RouteWorker: cluster.ExternalCallWorker{Writer: writer}, RouteResolver: resolver, Terminal: f.terminal, Mode: aman.ModeAuthoritative, SourceMode: aman.ObservationSourceHybrid, VatsimStaleAfter: 10 * time.Minute, HoldingEATEnabled: true, Now: func() time.Time { return f.now }})
		require.NoError(t, err)
		f.nodes[i] = &replica{nc: nc, p: p, owner: owner, writer: writer, source: source, worker: worker, router: router, stop: stop}
		go func() { _ = owner.Run(runCtx) }()
		go func() { _ = router.Serve(runCtx) }()
	}
	t.Cleanup(func() {
		for _, n := range f.nodes {
			n.stop()
			n.nc.Close()
		}
	})
	return f
}
func (f *fixture) owner(t *testing.T, ref *pb.AggregateRef, exclude string) int {
	t.Helper()
	found := -1
	require.Eventually(t, func() bool {
		for i, n := range f.nodes {
			if n.owner.NodeID != exclude && n.owner.CanWrite(ref) {
				found = i
				return true
			}
		}
		return false
	}, 15*time.Second, 30*time.Millisecond)
	return found
}
func committed(t *testing.T, r *pb.CommandReply) {
	t.Helper()
	if r.Status == pb.CommandReply_PENDING {
		require.Equal(t, pb.CommandOutcome_ACCEPTED, r.GetOutcome().GetStatus(), r)
	} else {
		require.Equal(t, pb.CommandReply_COMMITTED, r.Status, r)
		require.Equal(t, pb.CommandOutcome_SUCCEEDED, r.GetOutcome().GetStatus(), r)
	}
}

// Lease renewal deliberately fences command admission while its PubAck and
// projection barrier are pending. Retry only that explicit pre-publish
// availability response, retaining the same immutable command identity.
func admitted(t *testing.T, call func() (*pb.CommandReply, error)) *pb.CommandReply {
	t.Helper()
	var reply *pb.CommandReply
	var err error
	require.Eventually(t, func() bool {
		reply, err = call()
		return reply == nil || (reply.Status != pb.CommandReply_NOT_OWNER && (reply.Status != pb.CommandReply_UNAVAILABLE || reply.Detail != "owner lease or projection unavailable"))
	}, 5*time.Second, 20*time.Millisecond)
	require.NoError(t, err)
	return reply
}
func (f *fixture) entity(t *testing.T, ref *pb.AggregateRef, key string, value *pb.EntityRecord) {
	t.Helper()
	i := f.owner(t, ref, "")
	w := f.nodes[i].writer
	w.Plan = cluster.PlanSystemEntity
	state, err := (cluster.LocalLifecycleStore{Writer: w}).Read(f.ctx, ref)
	require.NoError(t, err)
	revision := uint64(0)
	if old := state.Entities[key]; old != nil {
		revision = old.Revision
	}
	if value.GetStrip() != nil {
		w.Plan = cluster.PlanStrip
	}
	id := uuid.NewString()
	committed(t, admitted(t, func() (*pb.CommandReply, error) {
		return w.Execute(f.ctx, &pb.CommandRequest{ProtocolRevision: 1, CommandId: id, Aggregate: ref, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "fixture"}, ExpectedEntityRevision: &revision, Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: &pb.UpdateEntity{Key: key, Value: value}}}}}), nil
	}))
}
func (f *fixture) seed(t *testing.T) {
	sessionOwner := f.owner(t, sessionRef(f.session), "")
	owned, err := (cluster.LocalLifecycleStore{Writer: f.nodes[sessionOwner].writer}).Read(f.ctx, sessionRef(f.session))
	require.NoError(t, err)
	f.entity(t, globalRef(), fmt.Sprint(f.session), &pb.EntityRecord{Value: &pb.EntityRecord_SessionRegistry{SessionRegistry: &pb.SessionRegistry{Id: f.session, Airport: f.airport, Name: "LIVE", State: pb.SessionRegistry_ACTIVE}}})
	f.entity(t, sessionRef(f.session), fmt.Sprint(f.session), &pb.EntityRecord{Value: &pb.EntityRecord_Session{Session: &pb.Session{Id: f.session, Airport: f.airport, Name: "LIVE", NextStripId: 1, Runways: []*pb.Runway{{Name: "22L", Arrival: true}}, Master: &pb.MasterTerm{Cid: "1234567", ConnectionId: uuid.NewString(), Epoch: 1, OwnerEpoch: owned.Owner.Epoch}}}})
	owner := f.owner(t, sessionRef(f.session), "")
	w := f.nodes[owner].writer
	w.Plan = cluster.PlanStrip
	zero := uint64(0)
	strip := &pb.Strip{Callsign: "SAS123", Destination: f.airport, Departure: "ENGM", Route: "TESPI", Bay: "CLEARED", Hold: "TESPI", HoldType: "enroute", Remarks: "controller remark", OwnerCid: "1234567", VatsimOnly: true}
	update := &pb.UpdateEntity{Key: strip.Callsign, Value: &pb.EntityRecord{Value: &pb.EntityRecord_Strip{Strip: strip}}}
	id := uuid.NewString()
	committed(t, admitted(t, func() (*pb.CommandReply, error) {
		return w.Execute(f.ctx, &pb.CommandRequest{ProtocolRevision: 1, CommandId: id, Aggregate: sessionRef(f.session), Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "fixture"}, ExpectedEntityRevision: &zero, Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: update}}}}), nil
	}))
	f.seedNavigation(t)
	f.vatsim(t, true)
}
func (f *fixture) vatsim(t *testing.T, present bool) {
	page := &pb.ProviderPage{Provider: "vatsim", Resource: "network-data/v3", Parsed: &pb.ProviderPage_Vatsim{Vatsim: &pb.VatsimPage{SnapshotAt: timestamp(f.now)}}}
	if present {
		page.GetVatsim().Flights = []*pb.VatsimFlight{{Cid: "12345", Callsign: "SAS123", State: "online", Latitude: 56.01, Longitude: 12, Altitude: 10000, Groundspeed: 180, LastUpdated: timestamp(f.now), FlightPlan: &pb.VatsimFlightPlan{Origin: "ENGM", Destination: f.airport, AircraftShort: "C172", Aircraft: "C172/L", RequestedLevel: "10000", Route: "TESPI", Revision: 1}}}
	}
	n := f.nodes[f.owner(t, globalRef(), "")]
	name, sha, err := n.source.PublishProvider(page)
	require.NoError(t, err)
	id := uuid.NewString()
	r := admitted(t, func() (*pb.CommandReply, error) {
		return n.source.PutCheckpointFor(f.ctx, globalRef(), id, &pb.ProviderCheckpoint{Provider: "vatsim", Resource: "network-data/v3", ObjectName: name, Sha256: sha})
	})
	committed(t, r)
}
func (f *fixture) seedNavigation(t *testing.T) {
	n := f.nodes[f.owner(t, airportRef(f.airport), "")]
	version := navdata.DatasetVersion{Cycle: "2609", SourceRevision: "fixture", EffectiveFrom: f.now.Add(-24 * time.Hour), EffectiveUntil: f.now.Add(24 * time.Hour)}
	provenance := navdata.Provenance{SourceID: "fixture", SourceRevision: "fixture", ImportedAt: f.now.Add(-time.Minute), EffectiveFrom: version.EffectiveFrom, EffectiveUntil: version.EffectiveUntil}
	fixes := []navdata.Fix{{ID: "ORIGIN", Position: navdata.Coordinate{LatitudeDeg: 56.1, LongitudeDeg: 12}, Provenance: provenance}, {ID: "TESPI", Position: navdata.Coordinate{LatitudeDeg: 56, LongitudeDeg: 12}, Provenance: provenance}, {ID: "TNO", Position: navdata.Coordinate{LatitudeDeg: 55.9, LongitudeDeg: 12}, Provenance: provenance}, {ID: "THR", Position: navdata.Coordinate{LatitudeDeg: 55.8, LongitudeDeg: 12}, Provenance: provenance}}
	a, b, c, d := navdata.FixID("ORIGIN"), navdata.FixID("TESPI"), navdata.FixID("TNO"), navdata.FixID("THR")
	legs := []navdata.ProcedureLeg{{ID: "TERMINAL-1", PathTerminator: navdata.PathTF, FromFix: &b, ToFix: &c}, {ID: "TERMINAL-2", PathTerminator: navdata.PathTF, FromFix: &c, ToFix: &d}}
	seconds := int64(60)
	holding := navdata.HoldingPattern{ID: "TESPI-HOLD", Fix: "TESPI", InboundCourseTrueDeg: 180, TurnDirection: navdata.TurnRight, LegTimeSeconds: &seconds, Termination: navdata.HoldingManual, Provenance: provenance}
	path := navdata.TerminalPath{Version: version, Airport: navdata.AirportID(f.airport), Feeder: "TESPI", STARFamily: "TESPI", FeederFix: "TNO", RunwayGroup: "ARRIVAL-22", Legs: legs, HoldingIDs: []navdata.HoldingID{holding.ID}, Coverage: navdata.CoverageComplete, Digest: strings.Repeat("a", 64), Provenance: provenance}
	base := func() *pb.NavData {
		return &pb.NavData{Airport: f.airport, Version: encodeNavDatasetVersion(&version), SchemaVersion: navdata.CanonicalSchemaVersion, Provenance: encodeNavProvenance(&provenance), ImportedAt: timestamp(provenance.ImportedAt), ValidatedAt: timestamp(provenance.ImportedAt), ValidationState: "validated", Digest: strings.Repeat("b", 64)}
	}
	airport := base()
	airport.Fragment = &pb.NavData_AirportFragment{AirportFragment: &pb.NavAirportFragment{Airport: &pb.NavAirport{Icao: f.airport, Name: "fixture", Position: encodeNavCoordinate(&fixes[3].Position), Provenance: encodeNavProvenance(&provenance)}}}
	fix := base()
	fix.Fragment = &pb.NavData_FixFragment{FixFragment: &pb.NavFixFragment{Coverage: "complete"}}
	for i := range fixes {
		fix.GetFixFragment().Fixes = append(fix.GetFixFragment().Fixes, encodeNavFix(&fixes[i]))
	}
	term := base()
	term.Fragment = &pb.NavData_TerminalFragment{TerminalFragment: &pb.NavTerminalFragment{Airport: f.airport, ConfigVersion: f.terminal.ConfigVersion, Paths: []*pb.NavTerminalPath{encodeNavTerminalPath(&path)}, Holdings: []*pb.NavHolding{encodeNavHolding(&holding)}, OperationalPolicy: TerminalPolicy(f.terminal)}}
	var refs []*pb.NavObjectRef
	for _, data := range []*pb.NavData{airport, fix, term} {
		ref, err := n.source.PublishNav(data)
		require.NoError(t, err)
		refs = append(refs, ref)
	}
	manifest := &pb.NavManifest{Airport: f.airport, Cycle: version.Cycle, Active: true, Objects: refs, SourceRevision: 1, SourceSha256: strings.Repeat("d", 64)}
	encoded, err := (proto.MarshalOptions{Deterministic: true}).Marshal(manifest)
	require.NoError(t, err)
	manifest.Digest = fmt.Sprintf("%x", sha256.Sum256(encoded))
	id := uuid.NewString()
	r := admitted(t, func() (*pb.CommandReply, error) { return n.source.ActivateManifest(f.ctx, id, manifest) })
	committed(t, r)
	group := aman.RunwayGroupID("ARRIVAL-22")
	q := navdata.RouteQuery{Version: version, Origin: "ENGM", Destination: navdata.AirportID(f.airport), FiledRoute: "TESPI", RunwayGroup: &group}
	geometry := navdata.RouteGeometry{Version: version, Legs: []navdata.ProcedureLeg{{ID: "ROUTE", PathTerminator: navdata.PathTF, FromFix: &a, ToFix: &b}}, Coverage: navdata.CoverageComplete, TotalDistanceNM: 6, Provenance: provenance, Digest: strings.Repeat("c", 64)}
	candidate := navdata.RouteCandidate{Query: q, ResolverVersion: "airacnet-route-v4", SchemaVersion: navdata.CanonicalSchemaVersion, Geometry: geometry, CreatedAt: f.now}
	key, err := candidate.PersistenceKey()
	require.NoError(t, err)
	route := base()
	route.Digest = geometry.Digest
	route.Fragment = &pb.NavData_RouteCandidate{RouteCandidate: &pb.NavRouteCandidate{Query: encodeNavRouteQuery(&q), ResolverVersion: candidate.ResolverVersion, SchemaVersion: candidate.SchemaVersion, Geometry: encodeNavRouteGeometry(&geometry), CreatedAt: timestamp(f.now)}}
	ref, err := n.source.PublishNav(route)
	require.NoError(t, err)
	id = uuid.NewString()
	r = admitted(t, func() (*pb.CommandReply, error) {
		return n.source.PutRouteCache(f.ctx, f.airport, id, &pb.NavRouteCache{RouteKey: string(key), ResolverVersion: candidate.ResolverVersion, SchemaVersion: candidate.SchemaVersion, ObjectName: ref.ObjectName, Sha256: ref.Sha256})
	})
	committed(t, r)
	wind := &pb.ProviderPage{Provider: "openmeteo", Resource: "fixture", Parsed: &pb.ProviderPage_OpenMeteo{OpenMeteo: &pb.OpenMeteoPage{SourceId: "open-meteo-gfs", SourceRevision: "fixture", ObservedAt: timestamp(f.now.Add(-time.Minute)), ExpiresAt: timestamp(f.now.Add(2 * time.Hour)), Samples: []*pb.OpenMeteoSample{{LatitudeDegrees: 55.8, LongitudeDegrees: 12, ForecastAt: timestamp(f.now), Levels: []*pb.OpenMeteoWindLevel{{AltitudeFeet: 10000}}}}}}}
	name, sha, err := n.source.PublishProvider(wind)
	require.NoError(t, err)
	id = uuid.NewString()
	r = admitted(t, func() (*pb.CommandReply, error) {
		return n.source.PutCheckpoint(f.ctx, f.airport, id, &pb.ProviderCheckpoint{Provider: "openmeteo", Resource: "fixture", ObjectName: name, Sha256: sha})
	})
	committed(t, r)
}

func TestOperationalTwoReplicaNATS(t *testing.T) {
	f := newFixture(t)
	f.seed(t)
	first := f.owner(t, airportRef(f.airport), "")
	w := f.nodes[first].worker
	require.Equal(t, pb.CommandReply_NOT_OWNER, f.nodes[1-first].worker.ObserveVatsim(f.ctx, f.airport, "SAS123").Status)
	require.Error(t, f.nodes[1-first].worker.Resume(f.ctx, f.airport))
	r := w.ObserveVatsim(f.ctx, f.airport, "SAS123")
	committed(t, r)
	board, err := w.options.State.Read(f.ctx, f.airport)
	require.NoError(t, err)
	require.Len(t, board.Flights, 1)
	require.NotNil(t, board.Flights[0].Prediction)
	require.NotNil(t, board.Flights[0].Slot)
	require.NotNil(t, board.Flights[0].RouteProgress)
	require.True(t, board.Airport.Health.Ready)
	require.Equal(t, "aman-cph-v3", board.Airport.PolicyVersion)
	require.Equal(t, string(aman.PredictionBasisRETA), board.Flights[0].Prediction.Basis)
	committed(t, admitted(t, func() (*pb.CommandReply, error) { return w.ObserveVatsim(f.ctx, f.airport, "SAS123"), nil }))
	again, err := w.options.State.Read(f.ctx, f.airport)
	require.NoError(t, err)
	require.Equal(t, board.Revision, again.Revision)
	f.hold(t, w)
	source, err := w.options.Sessions.Read(f.ctx, airportRef(f.airport))
	require.NoError(t, err)
	require.Len(t, source.Workflows, 1)
	var intent *pb.WorkflowRecord
	for _, i := range source.Workflows {
		intent = i
	}
	require.Equal(t, pb.WorkflowRecord_PENDING, intent.Status)
	step, err := w.BuildStep(f.ctx, intent)
	require.NoError(t, err)
	require.NotEmpty(t, step.GetSystem().GetApplyAmanSession().GetHoldingEat().HoldEat)
	// Controller edits between evaluation and dispatch must survive the patch.
	session, err := w.options.Sessions.Read(f.ctx, sessionRef(f.session))
	require.NoError(t, err)
	strip := proto.Clone(session.Indexes[pb.EntityKind_STRIP]["SAS123"].GetValue().GetStrip()).(*pb.Strip)
	strip.Remarks = "later controller edit"
	f.entity(t, sessionRef(f.session), strip.Callsign, &pb.EntityRecord{Value: &pb.EntityRecord_Strip{Strip: strip}})
	committed(t, admitted(t, func() (*pb.CommandReply, error) { return w.options.Sessions.Execute(f.ctx, step), nil }))
	destination, err := w.options.Sessions.Read(f.ctx, sessionRef(f.session))
	require.NoError(t, err)
	before := destination.Indexes[pb.EntityKind_STRIP]["SAS123"].Revision
	require.Len(t, destination.Effects, 1)
	require.Equal(t, step.GetSystem().GetApplyAmanSession().GetHoldingEat().HoldEat, destination.Effects[step.CommandId].GetAmanHoldingEat().Eat)
	committed(t, admitted(t, func() (*pb.CommandReply, error) { return w.options.Sessions.Execute(f.ctx, step), nil }))
	replay, err := w.options.Sessions.Read(f.ctx, sessionRef(f.session))
	require.NoError(t, err)
	require.Equal(t, before, replay.Indexes[pb.EntityKind_STRIP]["SAS123"].Revision)
	// Advance the board before owner death as well: the committed destination
	// outcome must win over source supersession during takeover.
	f.now = f.now.Add(time.Second)
	committed(t, admitted(t, func() (*pb.CommandReply, error) { return w.Reconcile(f.ctx, f.airport, f.now), nil }))
	deadID := f.nodes[first].owner.NodeID
	f.nodes[first].stop()
	f.nodes[first].nc.Close()
	second := f.owner(t, airportRef(f.airport), deadID)
	w = f.nodes[second].worker
	f.owner(t, sessionRef(f.session), deadID)
	require.NoError(t, w.Resume(f.ctx, f.airport))
	source, err = w.options.Sessions.Read(f.ctx, airportRef(f.airport))
	require.NoError(t, err)
	require.Equal(t, pb.WorkflowRecord_COMPLETED, source.Workflows[intent.WorkflowId].Status)
	destination, err = w.options.Sessions.Read(f.ctx, sessionRef(f.session))
	require.NoError(t, err)
	require.Equal(t, before, destination.Indexes[pb.EntityKind_STRIP]["SAS123"].Revision)
	require.Len(t, destination.Effects, 1)
	require.Equal(t, "later controller edit", destination.Indexes[pb.EntityKind_STRIP]["SAS123"].GetValue().GetStrip().Remarks)
	f.now = f.now.Add(time.Minute)
	f.vatsim(t, false)
	committed(t, admitted(t, func() (*pb.CommandReply, error) { return w.ObserveMissingVatsim(f.ctx, f.airport, "SAS123"), nil }))
	board, err = w.options.State.Read(f.ctx, f.airport)
	require.NoError(t, err)
	require.Nil(t, board.Flights[0].Slot)
	require.True(t, board.Flights[0].LatestObservation.Missing)
	f.now = f.now.Add(time.Minute)
	committed(t, admitted(t, func() (*pb.CommandReply, error) { return w.Reconcile(f.ctx, f.airport, f.now), nil }))
	board, err = w.options.State.Read(f.ctx, f.airport)
	require.NoError(t, err)
	require.Equal(t, "removed", board.Flights[0].State)
}

func (f *fixture) hold(t *testing.T, w *Worker) {
	board, err := w.options.State.Read(f.ctx, f.airport)
	require.NoError(t, err)
	// A real accepted controller freeze/holding fact is the policy input. The
	// next production reconciliation derives the EAT and destination intent.
	revision := board.Airport.Revision
	next := proto.Clone(board.Airport).(*pb.AmanAirport)
	next.Revision++
	flight := proto.Clone(board.Flights[0]).(*pb.AmanFlight)
	capture := f.now.Add(20 * time.Minute)
	flight.FreezeReason = "manual"
	flight.FrozenAt = timestamp(f.now)
	flight.FrozenOperationalTeta = timestamp(capture)
	flight.Slot.Time = timestamp(capture)
	flight.Slot.Revision = next.Revision
	flight.FrozenSlot = proto.Clone(flight.Slot).(*pb.AmanSlot)
	flight.HoldingClearance = &pb.AmanHoldingClearance{Hold: "TESPI", HoldType: "enroute", ObservedAt: timestamp(f.now)}
	flight.QueueOffers = nil
	request := &pb.CommandRequest{ProtocolRevision: 1, CommandId: uuid.NewString(), Aggregate: airportRef(f.airport), Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "fixture-controller-fact"}, ExpectedEntityRevision: &revision, Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: &pb.UpdateEntity{Key: f.airport, Value: &pb.EntityRecord{Value: &pb.EntityRecord_AmanAirport{AmanAirport: next}}}}}}}
	committed(t, admitted(t, func() (*pb.CommandReply, error) {
		return w.options.State.Commit(f.ctx, cluster.AmanTransition{Request: request, Airport: next, Flights: []*pb.AmanFlight{flight}}), nil
	}))
	f.now = f.now.Add(time.Second)
	f.vatsim(t, true)
	committed(t, admitted(t, func() (*pb.CommandReply, error) { return w.Reconcile(f.ctx, f.airport, f.now), nil }))
}
