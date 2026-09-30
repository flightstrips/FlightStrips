package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"FlightStrips/internal/cdm"
	"FlightStrips/internal/cluster"
	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type cdmProviderFixture struct {
	mu      sync.Mutex
	rows    cdm.BulkIFPSData
	masters []cdm.AirportMaster
	reads   int
	writes  []string
	onWrite func() error
}

func (f *cdmProviderFixture) IFPSByDepartureAirport(context.Context, string) (cdm.BulkIFPSData, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reads++
	return append(cdm.BulkIFPSData(nil), f.rows...), nil
}
func (f *cdmProviderFixture) IFPSByCallsignParsed(context.Context, string) (*cdm.IFPSData, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reads++
	if len(f.rows) == 0 {
		return nil, nil
	}
	row := f.rows[0]
	return &row, nil
}
func (f *cdmProviderFixture) AirportMasters(context.Context) ([]cdm.AirportMaster, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]cdm.AirportMaster(nil), f.masters...), nil
}
func (f *cdmProviderFixture) write(value string) error {
	f.mu.Lock()
	f.writes = append(f.writes, value)
	callback := f.onWrite
	f.mu.Unlock()
	if callback != nil {
		return callback()
	}
	return nil
}
func (f *cdmProviderFixture) SetMasterAirport(_ context.Context, a, p string) error {
	return f.write("MASTER/" + a + "/" + p)
}
func (f *cdmProviderFixture) ClearMasterAirport(_ context.Context, a, p string) error {
	return f.write("CLEAR/" + a + "/" + p)
}
func (f *cdmProviderFixture) IFPSDpi(_ context.Context, c, v string) error {
	return f.write(c + "/" + v)
}
func (f *cdmProviderFixture) IFPSSetCdmData(_ context.Context, p cdm.SetCdmDataParams) error {
	return f.write(p.Callsign + "/STATE/" + p.Tobt + "/" + p.Tsat + "/" + p.Ttot)
}
func (f *cdmProviderFixture) IFPSSetTobt(_ context.Context, c, v string, t int) error {
	return f.write(fmt.Sprintf("%s/TOBT/%s/%d", c, v, t))
}

type cdmHarness struct {
	*lifecycleHarness
	c        [2]*CdmCandidate
	provider *cdmProviderFixture
	config   *cdm.CdmAirportConfig
	airport  string
}

func newCdmHarness(t *testing.T, viff bool) *cdmHarness {
	seed := uuid.New()
	icao := string([]byte{'A' + seed[0]%26, 'A' + seed[1]%26, 'A' + seed[2]%26, 'A' + seed[3]%26})
	h := &cdmHarness{lifecycleHarness: newLifecycleHarness(t), provider: &cdmProviderFixture{}, config: cdm.NewDefaultAirportConfig(icao), airport: icao}
	state := h.state()
	for _, entry := range state.EntitiesByKind(pb.EntityKind_SESSION) {
		copy := proto.Clone(entry.GetValue().GetSession()).(*pb.Session)
		copy.Airport = icao
		copy.Runways = []*pb.Runway{{Name: "22R", Departure: true}, {Name: "22L", Arrival: true}}
		writer := h.nodes[h.owner(sessionRef(h.id))].candidate.Writer
		writer.Plan = cluster.PlanSystemEntity
		request := cdmSystem(h.id, uuid.NewString(), entry.Revision, &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: &pb.UpdateEntity{Key: entry.Key, Value: &pb.EntityRecord{Value: &pb.EntityRecord_Session{Session: copy}}}}})
		h.await("CDM airport fixture", func() bool { return cdmReply(writer.Execute(h.ctx, request)) == nil })
	}
	h.config.DeiceConfig.Medium = 5
	airport := &pb.AggregateRef{Target: &pb.AggregateRef_Airport{Airport: &pb.AirportRef{Icao: icao}}}
	for i, node := range h.nodes {
		if err := node.owner.Track(airport); err != nil {
			t.Fatal(err)
		}
		writer := node.candidate.Writer
		config := cluster.CdmConfigCandidateWorker{State: node.candidate.Source, Worker: cluster.ExternalCallWorker{Writer: writer}, Fetch: func(context.Context, string) (*cdm.CdmAirportConfig, error) { return h.config.Clone(), nil }, Now: func() time.Time { return h.now }}
		reads := cluster.ViffReadAdapter{State: node.candidate.Source, Worker: cluster.ExternalCallWorker{Writer: writer}, Now: func() time.Time { return h.now }}
		writes := cluster.ViffWriteAdapter{Worker: cluster.ExternalCallWorker{Writer: writer}}
		if viff {
			reads.Client = h.provider
			writes.Client = h.provider
		}
		candidate, err := NewCdmCandidate(writer, config, reads, writes)
		if err != nil {
			t.Fatal(err)
		}
		candidate.Now = func() time.Time { return h.now }
		h.c[i] = candidate
		node.work.CDM = candidate.CDM
	}
	configOwner := h.owner(airport)
	h.await("CDM configuration", func() bool {
		_, err := h.c[configOwner].Config.Refresh(h.ctx, icao, h.now.Add(time.Duration(h.id)*time.Nanosecond))
		return err == nil
	})
	for _, c := range h.c {
		h.await("config projection", func() bool { page, _, err := c.Config.Read(h.ctx, icao); return err == nil && page != nil })
	}
	h.seedStrip("SAS1", "NOT_CLEARED", "1205")
	h.seedStrip("SAS2", "NOT_CLEARED", "1205")
	return h
}
func (h *cdmHarness) seedStrip(callsign, bay, eobt string) {
	h.t.Helper()
	w := h.nodes[h.owner(sessionRef(h.id))].candidate.Writer
	w.Plan = cluster.PlanStrip
	stamp, err := cdmTimestamp(&eobt, h.now)
	if err != nil {
		h.t.Fatal(err)
	}
	s := &pb.Strip{Callsign: callsign, Departure: h.airport, Destination: "EDDF", AircraftType: "A320", AircraftCategory: "M", Bay: bay, Runway: "22R", Sid: "VEDAR4A", Eobt: stamp, Tobt: stamp, HasFlightPlan: true}
	request := cdmSystem(h.id, uuid.NewString(), 0, &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: &pb.UpdateEntity{Key: callsign, Value: &pb.EntityRecord{Value: &pb.EntityRecord_Strip{Strip: s}}}}})
	h.await("CDM strip fixture", func() bool {
		reply := w.Execute(h.ctx, request)
		if reply.Status != pb.CommandReply_COMMITTED && reply.Status != pb.CommandReply_UNAVAILABLE {
			h.t.Fatalf("fixture: %v", reply)
		}
		return cdmReply(reply) == nil
	})
}

func (h *cdmHarness) rebind(index int) {
	old := h.c[index]
	node := h.nodes[index]
	config, reads, writes := old.Config, old.Reads, old.Writes
	config.State, config.Worker.Writer = node.candidate.Source, node.candidate.Writer
	reads.State, reads.Worker.Writer = node.candidate.Source, node.candidate.Writer
	writes.Worker.Writer = node.candidate.Writer
	var err error
	h.c[index], err = NewCdmCandidate(node.candidate.Writer, config, reads, writes)
	if err != nil {
		h.t.Fatal(err)
	}
	h.c[index].Now = func() time.Time { return h.now }
	node.work.CDM = h.c[index].CDM
	if err := node.owner.Track(&pb.AggregateRef{Target: &pb.AggregateRef_Airport{Airport: &pb.AirportRef{Icao: h.airport}}}); err != nil {
		h.t.Fatal(err)
	}
}

func TestCdmCandidateTwoReplicaRestartPendingDebounce(t *testing.T) {
	h := newCdmHarness(t, false)
	h.tick()
	h.acceptedAction(pb.CdmAction{Callsign: "SAS1", Change: &pb.CdmAction_SetDeice{SetDeice: &pb.SetCdmDeice{Code: "H"}}})
	saved := h.state()
	deadline := proto.Clone(saved.Indexes[pb.EntityKind_SESSION_DEADLINE]["cdm-debounce/session"]).(*pb.EntitySnapshot)
	if err := h.nodes[h.owner(sessionRef(h.id))].projection.Snapshots.Save(saved); err != nil {
		t.Fatal(err)
	}
	for _, node := range h.nodes {
		node.stop()
		node.nc.Close()
	}
	for i := range h.nodes {
		h.start(i)
		h.rebind(i)
	}
	if !proto.Equal(h.state().Indexes[pb.EntityKind_SESSION_DEADLINE][deadline.Key], deadline) {
		t.Fatal("restart replaced queued debounce identity")
	}
	h.now = h.now.Add(time.Second)
	h.tick()
	data := h.state().Indexes[pb.EntityKind_CDM_STATE]["SAS1"].Value.GetCdmState()
	if data.Deice != "H" || data.Recalculation != pb.CdmState_NONE || h.state().Indexes[pb.EntityKind_SESSION_DEADLINE][deadline.Key] != nil {
		t.Fatal("restart lost pending sequence work")
	}
}

func TestCdmCandidateTwoReplicaPushbackVerificationTakeover(t *testing.T) {
	h := newCdmHarness(t, true)
	h.tick()
	operation := h.acceptedAction(pb.CdmAction{Callsign: "SAS1", Change: &pb.CdmAction_PreparePushback{PreparePushback: &pb.CdmAutomaticAction{}}})
	h.provider.rows = cdm.BulkIFPSData{{Callsign: "SAS1", Departure: h.airport, Arrival: "EDDF", TOBT: "1200", CDMData: cdm.CDMData{TOBT: "1200"}}}
	h.now = h.now.Add(time.Second)
	h.tick() // sequence commit and authoritative export
	h.tick() // first accepted matching observation
	state := h.state()
	data := state.Indexes[pb.EntityKind_CDM_STATE]["SAS1"].Value.GetCdmState()
	if data.Pushback.Matches != 1 || data.Pushback.Completed {
		t.Fatalf("first probe %v", data.Pushback)
	}
	queued := proto.Clone(state.Indexes[pb.EntityKind_SESSION_DEADLINE]["cdm-pushback/SAS1"]).(*pb.EntitySnapshot)
	owner := h.owner(sessionRef(h.id))
	h.nodes[owner].stop()
	h.nodes[owner].nc.Close()
	if h.owner(sessionRef(h.id)) == owner {
		t.Fatal("pushback takeover missing")
	}
	if !proto.Equal(h.state().Indexes[pb.EntityKind_SESSION_DEADLINE][queued.Key], queued) {
		t.Fatal("takeover rearmed probe")
	}
	h.now = h.now.Add(time.Second)
	h.tick()
	service, _ := NewCdmActionService(cluster.LocalLifecycleStore{Writer: h.c[h.owner(sessionRef(h.id))].Writer})
	result, err := service.PushbackResult(h.ctx, h.id, "SAS1", operation)
	if err != nil || !result.Completed || !result.Verified {
		t.Fatalf("durable acknowledgement %+v %v", result, err)
	}
	if h.state().Indexes[pb.EntityKind_SESSION_DEADLINE][queued.Key] != nil {
		t.Fatal("completed probe deadline remains")
	}
	h.provider.mu.Lock()
	reads, writes := h.provider.reads, len(h.provider.writes)
	h.provider.mu.Unlock()
	h.tick()
	h.provider.mu.Lock()
	defer h.provider.mu.Unlock()
	if reads != h.provider.reads || writes != len(h.provider.writes) {
		t.Fatal("completed pushback repeated provider calls")
	}
}

func TestCdmCandidateTwoReplicaOperationalActions(t *testing.T) {
	h := newCdmHarness(t, true)
	h.tick()
	for _, action := range []*pb.CdmAction{
		{Change: &pb.CdmAction_SetTobt{SetTobt: &pb.SetTobt{HhmmUtc: "1215"}}},
		{Change: &pb.CdmAction_SetEobt{SetEobt: &pb.SetTobt{HhmmUtc: "1210"}}},
		{Change: &pb.CdmAction_SetAsrt{SetAsrt: &pb.SetTobt{HhmmUtc: "1201"}}},
		{Change: &pb.CdmAction_SetTsac{SetTsac: &pb.SetTobt{HhmmUtc: "1202"}}},
		{Change: &pb.CdmAction_SetDeice{SetDeice: &pb.SetCdmDeice{Code: "M"}}},
		{Change: &pb.CdmAction_SetCtot{SetCtot: &pb.SetCdmCtot{Value: timestamppb.New(h.now.Add(30 * time.Minute))}}},
		{Change: &pb.CdmAction_RemoveCtot{RemoveCtot: &pb.RemoveCdmCtot{}}},
		{Change: &pb.CdmAction_GroundState{GroundState: &pb.CdmGroundState{State: "PUSH"}}},
		{Change: &pb.CdmAction_RecordAobt{RecordAobt: &pb.CdmAutomaticAction{}}},
		{Change: &pb.CdmAction_RecordAtot{RecordAtot: &pb.CdmAutomaticAction{}}},
		{Change: &pb.CdmAction_RecordAtot{RecordAtot: &pb.CdmAutomaticAction{}}},
		{Change: &pb.CdmAction_ClearanceTobt{ClearanceTobt: &pb.CdmAutomaticAction{}}},
		{Change: &pb.CdmAction_LogonEobt{LogonEobt: &pb.SetTobt{HhmmUtc: "1205"}}},
		{Change: &pb.CdmAction_BetterTobt{BetterTobt: &pb.CdmAutomaticAction{}}},
	} {
		action.Callsign = "SAS1"
		h.acceptedAction(*action)
		h.now = h.now.Add(time.Second)
		h.tick()
	}
	data := h.state().Indexes[pb.EntityKind_CDM_STATE]["SAS1"].Value.GetCdmState()
	strip := h.state().Indexes[pb.EntityKind_STRIP]["SAS1"].Value.GetStrip()
	if strip.Aobt == nil || strip.Asat == nil || strip.Asrt.AsTime().Format("1504") != "1201" || strip.Tsac.AsTime().Format("1504") != "1202" || data.Atot == nil || data.Deice != "M" || data.AtotViffPending || !strings.HasPrefix(strip.GetOperationalStatus(), "REQTOBT/") {
		t.Fatalf("operational result incomplete: %v %v", data, strip)
	}
	h.provider.mu.Lock()
	defer h.provider.mu.Unlock()
	atot, aobt, tobt := 0, 0, 0
	for _, call := range h.provider.writes {
		if strings.HasPrefix(call, "SAS1/ATOT/") {
			atot++
		}
		if strings.HasPrefix(call, "SAS1/AOBT/") {
			aobt++
		}
		if strings.HasPrefix(call, "SAS1/TOBT/1215/") {
			tobt++
		}
	}
	if atot != 1 || aobt != 1 || tobt != 1 {
		t.Fatalf("actual/TOBT export multiplicity: %d %d %d", atot, aobt, tobt)
	}
}

func TestCdmCandidateTwoReplicaMasterRegistration(t *testing.T) {
	h := newCdmHarness(t, true)
	registry := &pb.SessionRegistry{Id: h.id, Airport: h.airport, Name: "LIVE", State: pb.SessionRegistry_ACTIVE, WorkflowId: uuid.NewString()}
	key := fmt.Sprint(h.id)
	put := func(expected uint64) {
		w := h.c[h.owner(globalRef())].Writer
		w.Plan = cluster.PlanSystemEntity
		request := cdmSystem(h.id, uuid.NewString(), expected, &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: &pb.UpdateEntity{Key: key, Value: &pb.EntityRecord{Value: &pb.EntityRecord_SessionRegistry{SessionRegistry: registry}}}}})
		request.Aggregate = globalRef()
		h.await("master registry fixture", func() bool {
			reply := w.Execute(h.ctx, request)
			if reply.Status == pb.CommandReply_COMMITTED && reply.GetOutcome().GetStatus() == pb.CommandOutcome_FAILED {
				t.Fatalf("registry fixture %v", reply)
			}
			return cdmReply(reply) == nil
		})
	}
	put(0)
	airport := &pb.AggregateRef{Target: &pb.AggregateRef_Airport{Airport: &pb.AirportRef{Icao: h.airport}}}
	owner := h.owner(airport)
	c := h.c[owner]
	h.await("accepted master page", func() bool { _, err := c.Reads.Masters(h.ctx, h.airport, h.now); return err == nil })
	for attempt := 0; attempt < 2; attempt++ {
		h.await("master reconciliation", func() bool { return c.ReconcileMaster(h.ctx, h.airport) == nil })
	}
	h.provider.masters = []cdm.AirportMaster{{ICAO: h.airport, Position: cdm.DefaultMasterPosition}}
	h.now = h.now.Add(time.Second)
	h.await("registered master page", func() bool { _, err := c.Reads.Masters(h.ctx, h.airport, h.now); return err == nil })
	registry.State = pb.SessionRegistry_DELETING
	put(1)
	for attempt := 0; attempt < 2; attempt++ {
		h.await("master deregistration", func() bool { return c.ReconcileMaster(h.ctx, h.airport) == nil })
	}
	h.provider.mu.Lock()
	defer h.provider.mu.Unlock()
	if len(h.provider.writes) != 2 || !strings.HasPrefix(h.provider.writes[0], "MASTER/") || !strings.HasPrefix(h.provider.writes[1], "CLEAR/") {
		t.Fatalf("master duplicate calls %v", h.provider.writes)
	}
}

func TestCdmCandidateTwoReplicaProviderCommitBoundaries(t *testing.T) {
	for _, boundary := range []string{"intent", "result"} {
		for _, after := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s_after_%t", boundary, after), func(t *testing.T) {
				h := newCdmHarness(t, true)
				h.tick()
				command := h.acceptedAction(pb.CdmAction{Callsign: "SAS1", Change: &pb.CdmAction_RecordAtot{RecordAtot: &pb.CdmAutomaticAction{}}})
				operation := lifecycleID(command, "viff/atot")
				owner := h.owner(sessionRef(h.id))
				c := h.c[owner]
				fault := &cdmCommitFault{EventStore: c.Writes.Worker.Writer.Store, after: after, kill: func() { h.nodes[owner].stop(); h.nodes[owner].nc.Close() }, filter: func(e *pb.StateEvent) bool {
					if boundary == "result" {
						return e.GetActor().GetId() == "viff-result"
					}
					for _, w := range e.GetDomainChanged().GetWorkflows() {
						if w.WorkflowId == operation && w.Status == pb.WorkflowRecord_PENDING {
							return true
						}
					}
					return false
				}}
				c.Writes.Worker.Writer.Store = fault
				_ = c.CDM(h.ctx, h.id)
				if !fault.fired {
					t.Fatal("provider boundary was not reached")
				}
				if h.owner(sessionRef(h.id)) == owner {
					t.Fatal("provider takeover missing")
				}
				h.tick()
				h.tick()
				result := h.state().Workflows[operation]
				want := pb.WorkflowRecord_COMPLETED
				if boundary == "intent" && after || boundary == "result" && !after {
					want = pb.WorkflowRecord_FAILED
				}
				if result == nil || result.Status != want || want == pb.WorkflowRecord_FAILED && result.ReasonCode != "CALL_UNCERTAIN" {
					t.Fatalf("provider outcome %v, want %v", result, want)
				}
				h.provider.mu.Lock()
				defer h.provider.mu.Unlock()
				calls := 0
				for _, call := range h.provider.writes {
					if strings.HasPrefix(call, "SAS1/ATOT/") {
						calls++
					}
				}
				wantCalls := 1
				if boundary == "intent" && after {
					wantCalls = 0
				}
				if calls != wantCalls {
					t.Fatalf("unsafe call count %d want %d", calls, wantCalls)
				}
			})
		}
	}
}
func (h *cdmHarness) tick() {
	h.t.Helper()
	lastError := ""
	h.await("CDM callback", func() bool {
		err := h.nodes[h.owner(sessionRef(h.id))].work.CDM(h.ctx, h.id)
		if err != nil && !lifecycleTransient(err) && err.Error() != lastError {
			h.t.Logf("retry CDM: %v", err)
			lastError = err.Error()
		}
		return err == nil
	})
}
func (h *cdmHarness) action(command string, change pb.CdmAction, expected uint64) *pb.CommandReply {
	h.t.Helper()
	owner := h.owner(sessionRef(h.id))
	c := h.c[owner]
	writer := c.Writer
	writer.Plan = c.Planner(cluster.PlanStrip)
	service, err := NewCdmActionService(cluster.LocalLifecycleStore{Writer: writer})
	if err != nil {
		h.t.Fatal(err)
	}
	actor := &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "cdm-operations", SessionId: proto.Int32(h.id)}
	request := &pb.CommandRequest{ProtocolRevision: 1, CommandId: command, Aggregate: sessionRef(h.id), Actor: actor, ExpectedEntityRevision: &expected, Command: &pb.CommandRequest_Client{Client: &pb.ClientCommand{Action: &pb.ClientCommand_Cdm{Cdm: &change}}}}
	return service.Execute(h.ctx, request)
}
func (h *cdmHarness) acceptedAction(change pb.CdmAction) string {
	h.t.Helper()
	id := uuid.NewString()
	expected := standRevision(h.state().Indexes[pb.EntityKind_CDM_STATE][change.Callsign])
	h.await("CDM action", func() bool {
		reply := h.action(id, change, expected)
		if reply.Status == pb.CommandReply_COMMITTED && reply.GetOutcome().GetStatus() == pb.CommandOutcome_FAILED {
			h.t.Fatalf("action failed: %v", reply)
		}
		if reply.Status != pb.CommandReply_COMMITTED && reply.Status != pb.CommandReply_UNAVAILABLE {
			h.t.Fatalf("action: %v", reply)
		}
		return cdmReply(reply) == nil
	})
	return id
}

func TestCdmCandidateTwoReplicaLocalSequenceDebounceAndTakeover(t *testing.T) {
	h := newCdmHarness(t, false)
	owner := h.owner(sessionRef(h.id))
	if err := h.c[1-owner].CDM(h.ctx, h.id); err == nil {
		t.Fatal("nonowner ran CDM")
	}
	h.tick()
	state := h.state()
	first := state.Indexes[pb.EntityKind_CDM_STATE]["SAS1"]
	second := state.Indexes[pb.EntityKind_CDM_STATE]["SAS2"]
	if first == nil || second == nil || first.Value.GetCdmState().Ttot == nil || second.Value.GetCdmState().Ttot == nil || proto.Equal(first.Value.GetCdmState().Ttot, second.Value.GetCdmState().Ttot) {
		t.Fatalf("real sequence did not separate slots: %v %v", first, second)
	}
	if first.Value.GetCdmState().Calculation == nil {
		t.Fatal("calculation evidence not durable")
	}
	if err := h.c[owner].Recalculate(h.ctx, h.id); err != nil {
		t.Fatal(err)
	}
	queued := proto.Clone(h.state().Indexes[pb.EntityKind_SESSION_DEADLINE]["cdm-debounce/session"]).(*pb.EntitySnapshot)
	if err := h.c[owner].Recalculate(h.ctx, h.id); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(h.state().Indexes[pb.EntityKind_SESSION_DEADLINE][queued.Key], queued) {
		t.Fatal("repeated recalculation callback rearmed episode")
	}
	h.now = h.now.Add(time.Second)
	h.tick()
	// Accepted repeated action UUID cannot rearm its debounce episode.
	action := pb.CdmAction{Callsign: "SAS1", Change: &pb.CdmAction_SetTobt{SetTobt: &pb.SetTobt{HhmmUtc: "1215"}}}
	command := h.acceptedAction(action)
	state = h.state()
	deadline := proto.Clone(state.Indexes[pb.EntityKind_SESSION_DEADLINE]["cdm-debounce/session"]).(*pb.EntitySnapshot)
	expected := first.Revision
	if reply := h.action(command, action, expected); cdmReply(reply) != nil {
		t.Fatalf("action replay: %v", reply)
	}
	if !proto.Equal(h.state().Indexes[pb.EntityKind_SESSION_DEADLINE][deadline.Key], deadline) {
		t.Fatal("replay rearmed debounce")
	}
	// A separate controller edit between scheduling and firing is preserved.
	h.edit("SAS1", func(s *pb.Strip) { s.Remarks = "controller edit"; s.Heading = proto.Int32(123) })
	h.nodes[owner].stop()
	h.nodes[owner].nc.Close()
	next := h.owner(sessionRef(h.id))
	if next == owner {
		t.Fatal("no takeover")
	}
	h.now = h.now.Add(time.Second)
	h.tick()
	state = h.state()
	if state.Indexes[pb.EntityKind_SESSION_DEADLINE][deadline.Key] != nil {
		t.Fatal("debounce not consumed")
	}
	s := state.Indexes[pb.EntityKind_STRIP]["SAS1"].Value.GetStrip()
	if s.Remarks != "controller edit" || s.GetHeading() != 123 {
		t.Fatal("CDM replaced controller edit")
	}
	if state.Indexes[pb.EntityKind_CDM_STATE]["SAS1"].Value.GetCdmState().Recalculation != pb.CdmState_NONE {
		t.Fatal("sequence remained pending")
	}
	before := state.Revision
	h.tick()
	if h.state().Revision != before {
		t.Fatal("replayed tick duplicated sequence")
	}
}

func TestCdmCandidateTwoReplicaViffPolicyAndUncertainActualTime(t *testing.T) {
	h := newCdmHarness(t, true)
	h.provider.rows = cdm.BulkIFPSData{{Callsign: "SAS1", Departure: h.airport, Arrival: "EDDF", CTOT: "1230"}}
	h.tick()
	h.provider.rows[0].CDMData = cdm.CDMData{ReqTOBT: "1210", ReqTOBTType: "PILOT"}
	h.now = h.now.Add(31 * time.Second)
	h.tick()
	state := h.state()
	data := state.Indexes[pb.EntityKind_CDM_STATE]["SAS1"].Value.GetCdmState()
	if data.GetCtot() == nil || data.GetTobtConfirmedBy() != "PILOT" || data.Tobt.AsTime().Format("1504") != "1210" {
		t.Fatalf("vIFF policy missing: %v", data)
	}
	if len(data.PendingExports) != 0 || data.ViffRequestSyncPending {
		t.Fatal("request export did not finish")
	}
	h.provider.mu.Lock()
	reads := h.provider.reads
	writes := len(h.provider.writes)
	h.provider.mu.Unlock()
	h.tick()
	h.provider.mu.Lock()
	if reads != h.provider.reads || writes != len(h.provider.writes) {
		t.Fatal("replayed slot repeated provider call")
	}
	h.provider.mu.Unlock()
	// Recalculation precedes the accepted derived export; ATOT uncertainty
	// remains under its original ID after owner death around provider result.
	command := h.acceptedAction(pb.CdmAction{Callsign: "SAS1", Change: &pb.CdmAction_RecordAtot{RecordAtot: &pb.CdmAutomaticAction{}}})
	owner := h.owner(sessionRef(h.id))
	h.provider.onWrite = func() error {
		h.nodes[owner].stop()
		h.nodes[owner].nc.Close()
		return errors.New("provider response lost")
	}
	_ = h.c[owner].CDM(h.ctx, h.id)
	next := h.owner(sessionRef(h.id))
	if next == owner {
		t.Fatal("ATOT owner did not move")
	}
	h.provider.onWrite = nil
	h.provider.mu.Lock()
	atotCalls := func() int {
		count := 0
		for _, call := range h.provider.writes {
			if strings.HasPrefix(call, "SAS1/ATOT/") {
				count++
			}
		}
		return count
	}
	calls := atotCalls()
	h.provider.mu.Unlock()
	h.tick()
	h.provider.mu.Lock()
	if calls != 1 || atotCalls() != calls {
		t.Fatal("uncertain unsafe export repeated")
	}
	h.provider.mu.Unlock()
	intent := lifecycleID(command, "viff/atot")
	result := h.state().Workflows[intent]
	if result == nil || result.Status != pb.WorkflowRecord_FAILED || result.ReasonCode != "CALL_UNCERTAIN" {
		t.Fatalf("uncertainty not retained: %v", result)
	}
}

func TestCdmCandidateTypedCalculationRoundtrip(t *testing.T) {
	anchor := time.Date(2026, 9, 30, 23, 55, 0, 0, time.UTC)
	data := &pb.CdmState{Callsign: "SAS1", Tobt: timestamppb.New(anchor), Atot: timestamppb.New(anchor), Recalculation: pb.CdmState_IMPROVE_ONLY, TobtConfirmedBy: proto.String("Pilot"), TobtManuallyConfirmed: true, ViffProposalTsat: timestamppb.New(anchor.Add(5 * time.Minute)), Calculation: &pb.CdmState_Calculation{BaseTime: timestamppb.New(anchor), BaseSource: proto.String("TOBT"), TaxiMinutes: proto.Int32(10), ReasonMarkers: []*pb.CdmState_ReasonMarker{{Kind: "RATE", Message: "departure spacing", RequiredSpacingMinutes: proto.Float64(3)}}}}
	state := cluster.NewAggregate(sessionRef(1))
	strip := &pb.Strip{Callsign: "SAS1", Departure: "EKCH", Eobt: timestamppb.New(anchor)}
	state.Indexes[pb.EntityKind_CDM_STATE] = map[string]*pb.EntitySnapshot{"SAS1": {Key: "SAS1", Revision: 1, Value: &pb.EntityRecord{Value: &pb.EntityRecord_CdmState{CdmState: data}}}}
	entry := &pb.EntitySnapshot{Key: "SAS1", Revision: 1, Value: &pb.EntityRecord{Value: &pb.EntityRecord_Strip{Strip: strip}}}
	m := cdmModel(state, entry, 1)
	_, result, err := cdmRecords(strip, data, m.CdmData, anchor)
	if err != nil || !proto.Equal(result, data) {
		t.Fatalf("CDM roundtrip %v\n%v\n%v", err, result, data)
	}
}

type cdmObjectHook struct {
	cluster.BinaryObjects
	mu     sync.Mutex
	fired  bool
	change func()
}

func (o *cdmObjectHook) GetBytes(name string) ([]byte, error) {
	data, err := o.BinaryObjects.GetBytes(name)
	if err == nil {
		o.mu.Lock()
		first := !o.fired
		o.fired = true
		o.mu.Unlock()
		if first {
			o.change()
		}
	}
	return data, err
}

func TestCdmCandidateTwoReplicaStaleConfigurationAndControllerInputs(t *testing.T) {
	for _, source := range []string{"configuration", "strip"} {
		t.Run(source, func(t *testing.T) {
			h := newCdmHarness(t, false)
			h.tick()
			h.acceptedAction(pb.CdmAction{Callsign: "SAS1", Change: &pb.CdmAction_SetDeice{SetDeice: &pb.SetCdmDeice{Code: "M"}}})
			h.now = h.now.Add(time.Second)
			owner := h.owner(sessionRef(h.id))
			c := h.c[owner]
			deadline := proto.Clone(h.state().Indexes[pb.EntityKind_SESSION_DEADLINE]["cdm-debounce/session"]).(*pb.EntitySnapshot)
			revision := h.state().Indexes[pb.EntityKind_CDM_STATE]["SAS1"].Revision
			objects := c.Config.State.Objects
			changed := false
			hook := &cdmObjectHook{BinaryObjects: objects, change: func() {
				changed = true
				if source == "strip" {
					h.edit("SAS1", func(s *pb.Strip) { s.Remarks = "new controller input" })
				} else {
					h.config.DefaultRate = 12
					airport := &pb.AggregateRef{Target: &pb.AggregateRef_Airport{Airport: &pb.AirportRef{Icao: h.airport}}}
					configOwner := h.owner(airport)
					h.await("replacement config", func() bool {
						_, err := h.c[configOwner].Config.Refresh(h.ctx, h.airport, h.now.Add(time.Second))
						return err == nil
					})
				}
			}}
			c.Config.State.Objects = hook
			err := c.fire(h.ctx, h.id, deadline)
			c.Config.State.Objects = objects
			if !changed || err == nil || !strings.Contains(err.Error(), "UNAVAILABLE") {
				t.Fatalf("stale %s result admitted: %v", source, err)
			}
			if h.state().Indexes[pb.EntityKind_CDM_STATE]["SAS1"].Revision != revision || !proto.Equal(h.state().Indexes[pb.EntityKind_SESSION_DEADLINE][deadline.Key], deadline) {
				t.Fatal("stale calculation changed state or consumed deadline")
			}
			h.tick()
			if source == "strip" && h.state().Indexes[pb.EntityKind_STRIP]["SAS1"].Value.GetStrip().Remarks != "new controller input" {
				t.Fatal("rederived result overwrote edit")
			}
		})
	}
}

type cdmCommitFault struct {
	cluster.EventStore
	after  bool
	fired  bool
	filter func(*pb.StateEvent) bool
	kill   func()
}

func (s *cdmCommitFault) Publish(ctx context.Context, subject string, expected uint64, data []byte) (uint64, error) {
	event := &pb.StateEvent{}
	err := pb.UnmarshalStrict(data, event)
	if err != nil {
		return 0, err
	}
	if s.fired || !s.filter(event) {
		return s.EventStore.Publish(ctx, subject, expected, data)
	}
	s.fired = true
	if !s.after {
		s.kill()
		return 0, errors.New("owner died before accepted commit")
	}
	sequence, err := s.EventStore.Publish(ctx, subject, expected, data)
	s.kill()
	if err != nil {
		return sequence, err
	}
	return 0, errors.New("owner died after accepted commit before PubAck")
}
func TestCdmCandidateTwoReplicaAtomicSequenceCommitBoundaries(t *testing.T) {
	for _, after := range []bool{false, true} {
		t.Run(fmt.Sprintf("after_commit_%t", after), func(t *testing.T) {
			h := newCdmHarness(t, false)
			owner := h.owner(sessionRef(h.id))
			c := h.c[owner]
			original := c.Writer.Store
			fault := &cdmCommitFault{EventStore: original, after: after, kill: func() { h.nodes[owner].stop(); h.nodes[owner].nc.Close() }, filter: func(e *pb.StateEvent) bool {
				count := 0
				for _, change := range e.GetDomainChanged().GetChanges() {
					if change.GetUpsert().GetCdmState() != nil {
						count++
					}
				}
				return count == 2
			}}
			c.Writer.Store = fault
			h.await("sequence crash boundary", func() bool { _ = c.CDM(h.ctx, h.id); return fault.fired })
			if h.owner(sessionRef(h.id)) == owner {
				t.Fatal("no sequence takeover")
			}
			h.tick()
			subject, _ := cluster.Subject(sessionRef(h.id))
			events, err := h.c[h.owner(sessionRef(h.id))].Writer.Store.Replay(h.ctx, subject)
			if err != nil {
				t.Fatal(err)
			}
			results := 0
			for _, applied := range events {
				event := &pb.StateEvent{}
				err := pb.UnmarshalStrict(applied.Data, event)
				if err != nil {
					t.Fatal(err)
				}
				cdms, strips := 0, 0
				for _, change := range event.GetDomainChanged().GetChanges() {
					if change.GetUpsert().GetCdmState() != nil {
						cdms++
					}
					if change.GetUpsert().GetStrip() != nil {
						strips++
					}
				}
				if cdms == 2 && strips == 2 {
					results++
				}
			}
			if results != 1 {
				t.Fatalf("expected one atomic two-strip result, got %d", results)
			}
			state := h.state()
			if err := h.nodes[h.owner(sessionRef(h.id))].projection.Snapshots.Save(state); err != nil {
				t.Fatal(err)
			}
			h.tick()
			if h.state().Revision != state.Revision {
				t.Fatal("snapshot/replay changed completed sequence")
			}
		})
	}
}

func TestCdmCandidateTwoReplicaReadyDeiceAndCtotValidation(t *testing.T) {
	h := newCdmHarness(t, true)
	h.tick()
	owner := h.owner(sessionRef(h.id))
	w := h.c[owner].Writer
	w.Plan = cluster.PlanControllerSector
	controller := &pb.Controller{Cid: "123", Callsign: "EKCH_A_TWR", Position: "EKCH_A_TWR", Section: "TWR", Revision: 1}
	request := cdmSystem(h.id, uuid.NewString(), 0, &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: &pb.UpdateEntity{Key: "123", Value: &pb.EntityRecord{Value: &pb.EntityRecord_Controller{Controller: controller}}}}})
	h.await("validation controller", func() bool {
		reply := w.Execute(h.ctx, request)
		if reply.Status != pb.CommandReply_COMMITTED && reply.Status != pb.CommandReply_UNAVAILABLE {
			t.Fatalf("controller fixture: %v", reply)
		}
		return cdmReply(reply) == nil
	})
	// The existing browser combines EOBT with other strip edits; it must use
	// actual CDM confirmation/clamping policy and one original strip revision.
	stripRevision := h.state().Indexes[pb.EntityKind_STRIP]["SAS2"].Revision
	browserRequest := &pb.CommandRequest{ProtocolRevision: 1, CommandId: uuid.NewString(), Aggregate: sessionRef(h.id), Actor: &pb.Actor{Kind: pb.Actor_CONTROLLER, Id: "123", SessionId: proto.Int32(h.id)}, ExpectedEntityRevision: &stripRevision, Command: &pb.CommandRequest_Client{Client: &pb.ClientCommand{Action: &pb.ClientCommand_Strip{Strip: &pb.StripAction{Callsign: "SAS2", Change: &pb.StripAction_UpdateData{UpdateData: &pb.UpdateStripData{Eobt: timestamppb.New(h.now.Add(12 * time.Minute)), Remarks: proto.String("browser edit"), Sid: proto.String("NEW4A")}}}}}}}
	w.Plan = h.c[owner].Planner(cluster.PlanStrip)
	h.await("browser CDM EOBT", func() bool {
		reply := w.Execute(h.ctx, browserRequest)
		if reply.Status == pb.CommandReply_COMMITTED && reply.GetOutcome().GetStatus() == pb.CommandOutcome_FAILED {
			t.Fatalf("browser EOBT %v", reply)
		}
		return cdmReply(reply) == nil
	})
	browserStrip := h.state().Indexes[pb.EntityKind_STRIP]["SAS2"].Value.GetStrip()
	if browserStrip.Remarks != "browser edit" || browserStrip.Sid != "NEW4A" || browserStrip.Eobt.AsTime().Format("1504") != "1212" || h.state().Indexes[pb.EntityKind_CDM_STATE]["SAS2"].Value.GetCdmState().Tobt.AsTime().Format("1504") != "1212" {
		t.Fatal("browser combined EOBT lost policy or controller fields")
	}
	h.edit("SAS1", func(s *pb.Strip) { s.OwnerCid = "123"; s.Bay = "TAXI_LWR" })
	h.acceptedAction(pb.CdmAction{Callsign: "SAS1", Change: &pb.CdmAction_SetCtot{SetCtot: &pb.SetCdmCtot{Value: timestamppb.New(h.now.Add(30 * time.Minute))}}})
	validation := h.state().Indexes[pb.EntityKind_STRIP]["SAS1"].Value.GetStrip().Validation
	if validation == nil || validation.IssueType != "CTOT" || validation.Action.GetAssignHoldingPoint() == nil {
		t.Fatalf("real CTOT validation missing: %v", validation)
	}
	h.edit("SAS1", func(s *pb.Strip) { s.Validation.Active = false })
	h.now = h.now.Add(time.Minute)
	h.tick()
	current := h.state().Indexes[pb.EntityKind_STRIP]["SAS1"].Value.GetStrip().Validation
	if current == nil || current.Active || current.ActivationKey != validation.ActivationKey {
		t.Fatal("periodic validation lost acknowledgement")
	}
	h.acceptedAction(pb.CdmAction{Callsign: "SAS2", Change: &pb.CdmAction_SetDeice{SetDeice: &pb.SetCdmDeice{Code: "M"}}})
	h.acceptedAction(pb.CdmAction{Callsign: "SAS2", Change: &pb.CdmAction_SetReady{SetReady: &pb.SetCdmReady{Ready: true}}})
	h.now = h.now.Add(time.Second)
	h.tick()
	data := h.state().Indexes[pb.EntityKind_CDM_STATE]["SAS2"].Value.GetCdmState()
	if data.Deice != "M" || !data.Ready || data.ReadySyncPending || data.Recalculation != pb.CdmState_NONE {
		t.Fatalf("READY/deice result missing %v", data)
	}
	h.provider.mu.Lock()
	defer h.provider.mu.Unlock()
	readyIndex, stateIndex := -1, -1
	for i, call := range h.provider.writes {
		if call == "SAS2/REA/1" {
			readyIndex = i
		}
		if readyIndex >= 0 && strings.HasPrefix(call, "SAS2/STATE/") {
			stateIndex = i
			break
		}
	}
	if readyIndex < 0 || stateIndex <= readyIndex {
		t.Fatalf("READY ordering wrong: %v", h.provider.writes)
	}
}
