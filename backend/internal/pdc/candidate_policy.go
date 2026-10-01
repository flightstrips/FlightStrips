package pdc

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"FlightStrips/internal/cluster"
	"FlightStrips/internal/config"
	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	pb "FlightStrips/pkg/events/cluster"
	pkgmodels "FlightStrips/pkg/models"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Candidate is constructed by Task 20 only. Plan is the common owner planner
// for browser/controller, HTTP pilot and authenticated provider commands.
type Candidate struct {
	Writer       cluster.Writer
	Source       cluster.NavigationWeather
	Provider     HoppieClientInterface
	Transceivers []TransceiverLookup
	Next         cluster.Planner
	Now          func() time.Time
	mu           sync.Mutex // Serializes local passes; NATS still owns every decision.
}

func NewCandidate(writer cluster.Writer, objects cluster.BinaryObjects, provider HoppieClientInterface, next cluster.Planner, transceivers ...TransceiverLookup) (*Candidate, error) {
	if writer.Store == nil || writer.Projection == nil || writer.Lease == nil || objects == nil || next == nil {
		return nil, fmt.Errorf("PDC candidate requires owner writer, projection, objects and next planner")
	}
	for _, source := range transceivers {
		if source == nil {
			return nil, fmt.Errorf("PDC candidate transceiver source is nil")
		}
	}
	c := &Candidate{Writer: writer, Source: cluster.NavigationWeather{Writer: writer, Objects: objects}, Provider: provider, Next: next, Transceivers: append([]TransceiverLookup(nil), transceivers...)}
	c.Writer.Plan = c.Plan
	return c, nil
}
func (c *Candidate) clock() time.Time {
	if c.Now != nil {
		return c.Now().UTC()
	}
	return time.Now().UTC()
}
func pdcRef(id int32) *pb.AggregateRef {
	return &pb.AggregateRef{Target: &pb.AggregateRef_Session{Session: &pb.SessionRef{Id: id}}}
}
func pdcID(parent, step string) string { id, _ := cluster.AmanIntentID(parent, step); return id }
func pdcUpsert(old *pb.EntitySnapshot, key string, v *pb.EntityRecord) *pb.EntityChange {
	rev := uint64(1)
	if old != nil {
		rev = old.Revision + 1
	}
	return &pb.EntityChange{Key: key, Revision: rev, Operation: &pb.EntityChange_Upsert{Upsert: v}}
}
func pdcDelete(old *pb.EntitySnapshot, kind pb.EntityKind) *pb.EntityChange {
	return &pb.EntityChange{Key: old.Key, Revision: old.Revision + 1, Operation: &pb.EntityChange_Delete{Delete: &pb.DeleteEntity{Kind: kind}}}
}
func pdcSort(d *pb.DomainChange) {
	sort.Slice(d.Changes, func(i, j int) bool {
		a, b := d.Changes[i], d.Changes[j]
		kind := func(c *pb.EntityChange) int32 {
			if c.GetDelete() != nil {
				return int32(c.GetDelete().Kind)
			}
			m := c.GetUpsert().ProtoReflect()
			return int32(m.WhichOneof(m.Descriptor().Oneofs().ByName("value")).Number())
		}
		if kind(a) != kind(b) {
			return kind(a) < kind(b)
		}
		return a.Key < b.Key
	})
}
func pdcReject(s pb.CommandReply_Status, msg string) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
	return nil, s, 0, fmt.Errorf("%s", msg)
}

func (c *Candidate) Plan(ctx context.Context, r *pb.CommandRequest, a *cluster.Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
	if r.GetAggregate().GetSession() == nil {
		return c.Next(ctx, r, a)
	}
	if m := r.GetSystem().GetApplyPdcProviderMessage().GetMessage(); m != nil {
		return c.incoming(ctx, r, a, m)
	}
	if r.Actor.GetKind() == pb.Actor_SYSTEM {
		switch r.Actor.Id {
		case "pdc-poll":
			return c.planPoll(ctx, r, a)
		case "pdc-send-result":
			return c.planSendResult(r, a)
		case "pdc-plugin":
			return c.planPlugin(r, a)
		}
		if r.Actor.Id == "session-worker" && r.GetSystem().GetRemoveEntity().GetKind() == pb.EntityKind_SESSION_DEADLINE {
			deadline := a.Indexes[pb.EntityKind_SESSION_DEADLINE][r.GetSystem().GetRemoveEntity().Key]
			if deadline.GetValue().GetSessionDeadline().GetKind() == "pdc-response" {
				return c.timeout(r, a, deadline)
			}
		}
	}
	if action := r.GetClient().GetPdc(); action != nil {
		return c.action(ctx, r, a, action)
	}
	return c.Next(ctx, r, a)
}

type pdcTransition struct {
	c                         *Candidate
	r                         *pb.CommandRequest
	a                         *cluster.Aggregate
	d                         *pb.DomainChange
	sessionOld, stripOld, old *pb.EntitySnapshot
	session                   *pb.Session
	strip                     *pb.Strip
	seq                       *pb.PdcSequence
}

func (c *Candidate) transition(r *pb.CommandRequest, a *cluster.Aggregate, key string) (*pdcTransition, error) {
	seed := a.Indexes[pb.EntityKind_SESSION][strconv.Itoa(int(r.Aggregate.GetSession().Id))]
	if seed == nil || seed.GetValue().GetSession().Tombstoned {
		return nil, fmt.Errorf("session unavailable")
	}
	t := &pdcTransition{c: c, r: r, a: a, d: &pb.DomainChange{}, sessionOld: seed, session: proto.Clone(seed.GetValue().GetSession()).(*pb.Session), stripOld: a.Indexes[pb.EntityKind_STRIP][key], old: a.Indexes[pb.EntityKind_PDC_SEQUENCE][key], seq: &pb.PdcSequence{Callsign: key, State: "NONE"}}
	if t.stripOld != nil {
		t.strip = proto.Clone(t.stripOld.GetValue().GetStrip()).(*pb.Strip)
	}
	if t.old != nil {
		t.seq = proto.Clone(t.old.GetValue().GetPdcSequence()).(*pb.PdcSequence)
	}
	return t, nil
}
func (t *pdcTransition) allocate() (uint64, error) {
	if t.session.NextMessageId == 0 || t.session.NextMessageId == math.MaxUint64 {
		return 0, fmt.Errorf("PDC message sequence exhausted")
	}
	n := t.session.NextMessageId
	t.session.NextMessageId++
	return n, nil
}
func (t *pdcTransition) outbound(kind pb.PdcProviderMessage_Kind, sequence uint64, response *uint64, text *string, reason string) error {
	if t.c.Provider == nil {
		return nil
	}
	if sequence == 0 {
		var err error
		sequence, err = t.allocate()
		if err != nil {
			return err
		}
	}
	id := pdcID(t.r.CommandId, "hoppie-"+strconv.Itoa(int(kind)))
	m := &pb.PdcProviderMessage{MessageId: id, From: t.session.Airport, To: t.seq.Callsign, Transport: pb.PdcProviderMessage_TRANSPORT_CPDLC, Sequence: sequence, ResponseTo: response, Kind: kind, ClearanceText: text, ReasonCode: reason}
	t.d.Changes = append(t.d.Changes, pdcUpsert(nil, id, &pb.EntityRecord{Value: &pb.EntityRecord_PdcProviderMessage{PdcProviderMessage: m}}))
	t.d.Workflows = append(t.d.Workflows, &pb.WorkflowRecord{WorkflowId: id, Source: t.r.Aggregate, Destination: t.r.Aggregate, Step: "pdc/outbound", DerivedCommandId: pdcID(id, "send"), Status: pb.WorkflowRecord_PENDING})
	return nil
}
func (t *pdcTransition) effect(action string, cleared *bool) error {
	master := t.session.Master
	if master == nil || master.Cid == "" {
		return fmt.Errorf("EuroScope master is unavailable")
	}
	remarks := ""
	if t.seq.State == "REQUESTED" || t.seq.State == "REQUESTED_WITH_FAULTS" {
		remarks = t.seq.RequestRemarks
	}
	t.d.Effects = []*pb.EffectRecord{{CommandId: t.r.CommandId, TargetCid: master.Cid, TargetConnectionId: &master.ConnectionId, OwnerEpoch: t.a.Owner.Epoch, MasterEpoch: master.Epoch, Status: pb.EffectRecord_WAITING, DispatchDeadline: timestamppb.New(t.c.clock().Add(30 * time.Second)), Payload: &pb.EffectRecord_Pdc{Pdc: &pb.PdcEffect{Callsign: t.seq.Callsign, Action: action, Clearance: t.seq.ClearanceText, Cleared: cleared, State: proto.String(t.seq.State), Remarks: proto.String(remarks)}}}}
	return nil
}

func (t *pdcTransition) pluginIntent(field string) {
	id := pdcID(t.r.CommandId, "plugin-"+field)
	t.d.Workflows = append(t.d.Workflows, &pb.WorkflowRecord{WorkflowId: id, Source: t.r.Aggregate, Destination: t.r.Aggregate, Step: "pdc/plugin/" + field + "/" + t.seq.Callsign + "/" + t.r.CommandId, DerivedCommandId: id, SourceRevision: proto.Uint64(t.stripOld.Revision + 1), Status: pb.WorkflowRecord_PENDING})
}
func (t *pdcTransition) fail(state string) error {
	if t.strip.Bay != "NOT_CLEARED" {
		var order uint64
		for _, e := range t.a.Entities {
			if s := e.GetValue().GetStrip(); s != nil && s.Bay == "NOT_CLEARED" && s.Sequence > order {
				order = s.Sequence
			}
			if s := e.GetValue().GetTacticalStrip(); s != nil && s.Bay == "NOT_CLEARED" && s.Sequence > order {
				order = s.Sequence
			}
		}
		if order > math.MaxUint64-1000 {
			return fmt.Errorf("strip order exhausted")
		}
		t.strip.Bay, t.strip.Sequence = "NOT_CLEARED", order+1000
	}
	t.strip.OwnerCid = ""
	t.strip.PreviousControllers = nil
	t.strip.RunwayCleared = false
	t.strip.RunwayConfirmed = false
	t.seq.State, t.seq.Deadline = state, nil
	t.pluginIntent("SET_CLEARED_FLAG")
	return nil
}
func (t *pdcTransition) finish() (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
	if !proto.Equal(t.sessionOld.GetValue().GetSession(), t.session) {
		t.d.Changes = append(t.d.Changes, pdcUpsert(t.sessionOld, t.sessionOld.Key, &pb.EntityRecord{Value: &pb.EntityRecord_Session{Session: t.session}}))
	}
	if t.strip != nil && (t.old != nil || t.seq.State != "NONE") {
		t.strip.PdcState, t.strip.PdcRequestRemarks = t.seq.State, ""
		if t.seq.State == "REQUESTED" || t.seq.State == "REQUESTED_WITH_FAULTS" {
			t.strip.PdcRequestRemarks = t.seq.RequestRemarks
		}
		if !proto.Equal(t.stripOld.GetValue().GetStrip(), t.strip) {
			t.strip.Revision = t.stripOld.Revision + 1
			t.d.Changes = append(t.d.Changes, pdcUpsert(t.stripOld, t.strip.Callsign, &pb.EntityRecord{Value: &pb.EntityRecord_Strip{Strip: t.strip}}))
		}
		if !proto.Equal(t.old.GetValue().GetPdcSequence(), t.seq) {
			change := pdcUpsert(t.old, t.seq.Callsign, &pb.EntityRecord{Value: &pb.EntityRecord_PdcSequence{PdcSequence: t.seq}})
			t.d.Changes = append(t.d.Changes, change)
			deadlineID := "pdc." + t.seq.Callsign
			oldDeadline := t.a.Indexes[pb.EntityKind_SESSION_DEADLINE][deadlineID]
			if t.seq.Deadline != nil {
				t.d.Changes = append(t.d.Changes, pdcUpsert(oldDeadline, deadlineID, &pb.EntityRecord{Value: &pb.EntityRecord_SessionDeadline{SessionDeadline: &pb.SessionDeadline{Id: deadlineID, Kind: "pdc-response", DueAt: t.seq.Deadline, Callsign: t.seq.Callsign, CommandId: t.r.CommandId, SourceRevision: change.Revision}}}))
			} else if oldDeadline != nil {
				t.d.Changes = append(t.d.Changes, pdcDelete(oldDeadline, pb.EntityKind_SESSION_DEADLINE))
			}
		}
	}
	pdcSort(t.d)
	return t.d, pb.CommandReply_COMMITTED, 0, nil
}

func (c *Candidate) action(ctx context.Context, r *pb.CommandRequest, a *cluster.Aggregate, action *pb.PdcAction) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
	t, err := c.transition(r, a, strings.ToUpper(strings.TrimSpace(action.Callsign)))
	if err != nil {
		return pdcReject(pb.CommandReply_NOT_FOUND, err.Error())
	}
	if t.strip == nil {
		return pdcReject(pb.CommandReply_NOT_FOUND, "strip not found")
	}
	current := t.old.GetRevision()
	if r.ExpectedEntityRevision == nil || *r.ExpectedEntityRevision != current {
		return nil, pb.CommandReply_REVISION_CONFLICT, current, fmt.Errorf("stale PDC revision")
	}
	actor := r.Actor
	controller := actor.Kind == pb.Actor_CONTROLLER && !a.Indexes[pb.EntityKind_CONTROLLER][actor.Id].GetValue().GetController().GetObserver() && a.Indexes[pb.EntityKind_CONTROLLER][actor.Id] != nil
	pilot := actor.Kind == pb.Actor_PILOT && actor.Id != "" && actor.Id == t.strip.VatsimCid
	if pilot && action.GetIssue() == nil {
		correlation, _ := cluster.ProviderEventCommandID("pdc-pilot", "web", fmt.Sprintf("%d/%s", t.session.Id, t.seq.Callsign))
		request, err := a.LookupWorkflow(correlation)
		if err != nil {
			return pdcReject(pb.CommandReply_UNAVAILABLE, err.Error())
		}
		pilot = false
		if request != nil {
			outcome, err := a.LookupOutcome(request.DerivedCommandId)
			if err != nil {
				return pdcReject(pb.CommandReply_UNAVAILABLE, err.Error())
			}
			pilot = outcome.GetActor().GetId() == actor.Id
		}
	}
	if actor.GetSessionId() != r.Aggregate.GetSession().Id || !controller && !pilot {
		return pdcReject(pb.CommandReply_UNAUTHORIZED, "PDC actor does not own callsign")
	}
	switch x := action.Change.(type) {
	case *pb.PdcAction_Issue:
		if controller {
			if t.seq.State != "REQUESTED" && t.seq.State != "REQUESTED_WITH_FAULTS" {
				return pdcReject(pb.CommandReply_INVALID_ARGUMENT, "PDC is not awaiting issuance")
			}
			if x.Issue.GetClearance() != "" {
				return pdcReject(pb.CommandReply_INVALID_ARGUMENT, "controller supplies remarks, not clearance text")
			}
			err = t.issue(ctx, x.Issue.GetRequestRemarks(), actor.Id)
		} else {
			if x.Issue == nil || x.Issue.Clearance != "" || x.Issue.RequestChannel != "WEB" || len(x.Issue.Atis) != 1 || x.Issue.Atis[0] < 'A' || x.Issue.Atis[0] > 'Z' || !strings.EqualFold(x.Issue.AircraftType, strings.SplitN(t.strip.AircraftType, "/", 2)[0]) {
				return pdcReject(pb.CommandReply_INVALID_ARGUMENT, "invalid ATIS or aircraft type")
			}
			err = t.request(ctx, "WEB", x.Issue.Atis, x.Issue.Stand, x.Issue.AircraftType, x.Issue.RequestRemarks)
		}
	case *pb.PdcAction_RevertToVoice:
		if !controller || t.seq.State == "NONE" {
			return pdcReject(pb.CommandReply_UNAUTHORIZED, "controller request required")
		}
		if err = t.fail("REVERT_TO_VOICE"); err != nil {
			return pdcReject(pb.CommandReply_INVALID_ARGUMENT, err.Error())
		}
		if t.seq.RequestChannel == "CPDLC" {
			err = t.outbound(pb.PdcProviderMessage_KIND_REVERT_TO_VOICE, 0, nil, nil, "")
		}
		if err == nil {
			err = t.effect("REVERT_TO_VOICE", nil)
		}
	case *pb.PdcAction_Acknowledge, *pb.PdcAction_Unable:
		if !pilot || t.seq.RequestChannel != "WEB" || !t.awaiting() {
			return pdcReject(pb.CommandReply_INVALID_ARGUMENT, "PDC is not awaiting pilot response")
		}
		_, unable := x.(*pb.PdcAction_Unable)
		err = t.respond(unable)
	default:
		return pdcReject(pb.CommandReply_INVALID_ARGUMENT, "unknown PDC action")
	}
	if err != nil {
		return pdcReject(pb.CommandReply_INVALID_ARGUMENT, err.Error())
	}
	return t.finish()
}
func (t *pdcTransition) awaiting() bool {
	return t.awaitingAt(t.c.clock())
}
func (t *pdcTransition) awaitingAt(observed time.Time) bool {
	return t.seq.State == "CLEARED" && t.seq.Deadline != nil && observed.Before(t.seq.Deadline.AsTime())
}
func (t *pdcTransition) providerAttempt() (*pb.WorkflowRecord, error) {
	for _, e := range t.a.EntitiesByKind(pb.EntityKind_PDC_PROVIDER_MESSAGE) {
		m := e.Value.GetPdcProviderMessage()
		if m.From == t.session.Airport && m.To == t.seq.Callsign && m.Sequence == t.seq.Sequence && m.Kind == pb.PdcProviderMessage_KIND_CLEARANCE {
			intent, err := t.a.LookupWorkflow(m.MessageId)
			if err != nil {
				return nil, err
			}
			if intent != nil {
				attempt, err := t.a.LookupWorkflow(intent.DerivedCommandId)
				if err != nil {
					return nil, err
				}
				if attempt != nil {
					return intent, nil
				}
			}
		}
	}
	return nil, nil
}
func (t *pdcTransition) request(ctx context.Context, channel, atis, stand, aircraft, remarks string) error {
	if t.strip.Bay != "NOT_CLEARED" {
		return fmt.Errorf("strip is already cleared")
	}
	if t.seq.State != "NONE" && t.seq.State != "FAILED" && t.seq.State != "REVERT_TO_VOICE" && t.seq.State != "NO_RESPONSE" {
		return fmt.Errorf("PDC request already pending")
	}
	// Replace prior pilot correlation on a new request; issuance remains frozen
	// under its original command UUID in the owner ledger.
	t.seq = &pb.PdcSequence{Callsign: t.seq.Callsign, State: "REQUESTED", RequestChannel: channel, Atis: atis, Stand: stand, AircraftType: aircraft, RequestRemarks: remarks, RequestedAt: timestamppb.New(t.c.clock())}
	if channel == "WEB" {
		id, _ := cluster.ProviderEventCommandID("pdc-pilot", "web", fmt.Sprintf("%d/%s", t.session.Id, t.seq.Callsign))
		t.d.Workflows = append(t.d.Workflows, &pb.WorkflowRecord{WorkflowId: id, Source: t.r.Aggregate, Destination: t.r.Aggregate, Step: "pdc/pilot", DerivedCommandId: t.r.CommandId, Status: pb.WorkflowRecord_PENDING})
	}
	strip, session := t.policyInputs()
	outcome := evaluatePdcRequest(strip, session, remarks)
	if len(PDCStripValidationFaults(strip, session.ActiveRunways.DepartureRunways, session.AvailableSids)) > 0 || outcome.State == StateRequestedWithFaults {
		t.seq.State = "REQUESTED_WITH_FAULTS"
	}
	if outcome.AutoIssue && t.seq.State == "REQUESTED" {
		if err := t.issue(ctx, "", t.session.Master.GetCid()); err == nil {
			return nil
		}
		t.seq.State = "REQUESTED_WITH_FAULTS"
	}
	if channel == "CPDLC" {
		if err := t.outbound(pb.PdcProviderMessage_KIND_STATUS, 0, nil, nil, ""); err != nil {
			return err
		}
	}
	// A request can remain reviewable when no master is connected.
	if t.session.Master.GetCid() != "" {
		return t.effect("STATE_CHANGE", nil)
	}
	return nil
}
func (t *pdcTransition) issue(ctx context.Context, remarks, cid string) error {
	options, review, err := t.clearance(ctx, remarks)
	if err != nil {
		return err
	}
	if t.session.Master.GetCid() == "" {
		return fmt.Errorf("EuroScope master is unavailable")
	}
	if t.seq.RequestChannel == "CPDLC" && t.session.NextMessageId > math.MaxInt32 {
		return fmt.Errorf("Hoppie message sequence exhausted")
	}
	var order uint64
	if t.strip.Bay != "CLEARED" {
		for _, e := range t.a.Entities {
			if s := e.GetValue().GetStrip(); s != nil && s.Bay == "CLEARED" && s.Sequence > order {
				order = s.Sequence
			}
			if s := e.GetValue().GetTacticalStrip(); s != nil && s.Bay == "CLEARED" && s.Sequence > order {
				order = s.Sequence
			}
		}
		if order > math.MaxUint64-1000 {
			return fmt.Errorf("strip order exhausted")
		}
	}
	n, err := t.allocate()
	if err != nil {
		return err
	}
	t.seq.Sequence, t.seq.State, t.seq.IssuedAt = n, "CLEARED", timestamppb.New(t.c.clock())
	t.seq.IssuedByCid = cid
	t.seq.Sent = t.seq.RequestChannel == "WEB"
	t.seq.ClearanceText = buildWebPDCClearance(options)
	if t.seq.RequestChannel == "CPDLC" {
		options.Sequence, options.PdcSequence = int32(n), int32(n)
		t.seq.ClearanceText = buildPDCClearanceProse(options, t.c.clock())
	}
	t.seq.Deadline = timestamppb.New(t.c.clock().Add(10 * time.Minute))
	if review != nil {
		for _, field := range []string{"SID", "ROUTE"} {
			value := review.SID
			previous := t.strip.Sid
			if field == "ROUTE" {
				value, previous = review.Route, t.strip.Route
			}
			if value != previous {
				t.pluginIntent(field)
			}
		}
		t.strip.Sid, t.strip.Route = review.SID, review.Route
	}
	if t.strip.Bay != "CLEARED" {
		t.strip.Bay, t.strip.Sequence = "CLEARED", order+1000
	}
	t.strip.StartRequested = false
	if t.seq.RequestChannel == "CPDLC" {
		if err = t.outbound(pb.PdcProviderMessage_KIND_CLEARANCE, n, nil, proto.String(t.seq.ClearanceText), ""); err != nil {
			return err
		}
	}
	return t.effect("ISSUE", nil)
}
func (t *pdcTransition) respond(unable bool) error {
	t.seq.Deadline = nil
	if unable {
		if err := t.fail("FAILED"); err != nil {
			return err
		}
		return t.effect("STATE_CHANGE", nil)
	}
	if t.strip.Bay == "NOT_CLEARED" || t.strip.Bay == "UNKNOWN" || t.strip.Bay == "" {
		var order uint64
		for _, e := range t.a.Entities {
			if s := e.GetValue().GetStrip(); s != nil && s.Bay == "CLEARED" && s.Sequence > order {
				order = s.Sequence
			}
			if s := e.GetValue().GetTacticalStrip(); s != nil && s.Bay == "CLEARED" && s.Sequence > order {
				order = s.Sequence
			}
		}
		if order > math.MaxUint64-1000 {
			return fmt.Errorf("strip order exhausted")
		}
		t.strip.Bay, t.strip.Sequence = "CLEARED", order+1000
	}
	t.seq.State = "CONFIRMED"
	t.pluginIntent("STATE_CHANGE")
	t.seq.PilotAcknowledgedAt = timestamppb.New(t.c.clock())
	if t.seq.IssuedByCid != "" {
		t.strip.OwnerCid = t.seq.IssuedByCid
	}
	if t.seq.RequestChannel == "CPDLC" {
		if err := t.outbound(pb.PdcProviderMessage_KIND_CONFIRMED, 0, &t.seq.Sequence, nil, ""); err != nil {
			return err
		}
	}
	return t.effect("SET_CLEARED_FLAG", proto.Bool(true))
}

func (c *Candidate) incoming(ctx context.Context, r *pb.CommandRequest, a *cluster.Aggregate, m *pb.PdcProviderMessage) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
	if r.Actor.Kind != pb.Actor_PROVIDER || r.Actor.Id != "hoppie" || r.Actor.GetSessionId() != r.Aggregate.GetSession().Id || r.CommandId != m.MessageId {
		return pdcReject(pb.CommandReply_UNAUTHORIZED, "authenticated Hoppie adapter required")
	}
	if err := cluster.ValidatePdcProviderMessage(m); err != nil {
		return pdcReject(pb.CommandReply_INVALID_ARGUMENT, err.Error())
	}
	t, err := c.transition(r, a, m.From)
	if err != nil {
		return pdcReject(pb.CommandReply_NOT_FOUND, err.Error())
	}
	if m.To != t.session.Airport || m.ProviderAcceptedAt != nil {
		return pdcReject(pb.CommandReply_INVALID_ARGUMENT, "incoming PDC station or acceptance mismatch")
	}
	// Use the accepted poll observation for the response deadline. An owner
	// dying after poll commit must not turn an on-time WILCO into a timeout.
	observed := c.clock()
	if checkpoint := a.Indexes[pb.EntityKind_PROVIDER_CHECKPOINT]["hoppie.station/"+t.session.Airport]; checkpoint != nil {
		value := checkpoint.Value.GetProviderCheckpoint()
		page, err := c.Source.ReadProvider(value.ObjectName, value.Sha256, "hoppie", "station/"+t.session.Airport)
		if err != nil {
			return pdcReject(pb.CommandReply_UNAVAILABLE, err.Error())
		}
		for _, accepted := range page.GetHoppie().Messages {
			if proto.Equal(accepted, m) {
				observed = page.GetHoppie().ObservedAt.AsTime()
				break
			}
		}
	}
	if old := a.Indexes[pb.EntityKind_PDC_PROVIDER_MESSAGE][m.MessageId]; old != nil {
		if !proto.Equal(old.Value.GetPdcProviderMessage(), m) {
			return pdcReject(pb.CommandReply_INVALID_ARGUMENT, "provider identity changed")
		}
		return &pb.DomainChange{}, pb.CommandReply_COMMITTED, 0, nil
	}
	t.d.Changes = append(t.d.Changes, pdcUpsert(nil, m.MessageId, &pb.EntityRecord{Value: &pb.EntityRecord_PdcProviderMessage{PdcProviderMessage: copyProvider(m)}}))
	reject := func(kind pb.PdcProviderMessage_Kind, reason string) error {
		if m.From == "" {
			return nil
		} // No safe reply address exists for a malformed header.
		return t.outbound(kind, 0, nil, nil, reason)
	}
	switch m.Kind {
	case pb.PdcProviderMessage_KIND_REQUEST:
		req := m.Request
		switch {
		case req.Callsign != m.From:
			err = reject(pb.PdcProviderMessage_KIND_MALFORMED, "CALLSIGN_MISMATCH")
		case len(req.Atis) != 1 || req.Atis[0] < 'A' || req.Atis[0] > 'Z':
			err = reject(pb.PdcProviderMessage_KIND_MALFORMED, "INVALID_ATIS")
		case t.strip == nil || req.Departure != t.session.Airport || req.Departure != t.strip.Departure || req.Destination != t.strip.Destination:
			err = reject(pb.PdcProviderMessage_KIND_FLIGHT_PLAN_NOT_HELD, "FLIGHT_PLAN_NOT_HELD")
		case !strings.EqualFold(req.AircraftType, strings.SplitN(t.strip.AircraftType, "/", 2)[0]):
			err = reject(pb.PdcProviderMessage_KIND_INVALID_AIRCRAFT_TYPE, "TYPE_MISMATCH")
		case t.seq.State == "CONFIRMED" || (t.seq.State == "NONE" || t.seq.State == "FAILED" || t.seq.State == "NO_RESPONSE" || t.seq.State == "REVERT_TO_VOICE") && t.strip.Bay != "NOT_CLEARED":
			err = reject(pb.PdcProviderMessage_KIND_FLIGHT_PLAN_NOT_HELD, "ALREADY_CLEARED")
		case t.seq.State != "NONE" && t.seq.State != "FAILED" && t.seq.State != "REVERT_TO_VOICE" && t.seq.State != "NO_RESPONSE":
			err = reject(pb.PdcProviderMessage_KIND_UNAVAILABLE, "ALREADY_PENDING")
		default:
			err = t.request(ctx, "CPDLC", req.Atis, req.Stand, req.AircraftType, req.Remarks)
		}
	case pb.PdcProviderMessage_KIND_WILCO, pb.PdcProviderMessage_KIND_UNABLE:
		attempt, lookupErr := t.providerAttempt()
		if lookupErr != nil {
			return pdcReject(pb.CommandReply_UNAVAILABLE, lookupErr.Error())
		}
		if t.strip == nil || t.seq.RequestChannel != "CPDLC" || !t.awaitingAt(observed) || m.ResponseTo == nil || *m.ResponseTo != t.seq.Sequence || attempt == nil {
			if err = reject(pb.PdcProviderMessage_KIND_NOT_SUPPORTED, "STALE_RESPONSE"); err != nil {
				return pdcReject(pb.CommandReply_UNAVAILABLE, err.Error())
			}
			return t.finish()
		} // Durable rejection without changing current clearance.
		// A correlated pilot response proves delivery without retrying an
		// uncertain provider attempt or inventing an API acceptance timestamp.
		if !t.seq.Sent {
			t.seq.Sent = true
			intent := proto.Clone(attempt).(*pb.WorkflowRecord)
			intent.Status, intent.ReasonCode = pb.WorkflowRecord_COMPLETED, "PILOT_RESPONSE_PROVES_DELIVERY"
			t.d.Workflows = append(t.d.Workflows, intent)
		}
		err = t.respond(m.Kind == pb.PdcProviderMessage_KIND_UNABLE)
		if m.Kind == pb.PdcProviderMessage_KIND_WILCO {
			t.seq.PilotAcknowledgedAt = timestamppb.New(observed)
		}
	default:
		err = reject(pb.PdcProviderMessage_KIND_NOT_SUPPORTED, m.ReasonCode)
	}
	if err != nil {
		return pdcReject(pb.CommandReply_UNAVAILABLE, err.Error())
	}
	return t.finish()
}

func (t *pdcTransition) policyInputs() (*models.Strip, *models.Session) {
	s := t.strip
	model := &models.Strip{Callsign: s.Callsign, Origin: s.Departure, Destination: s.Destination, Runway: optionalString(s.Runway), Sid: optionalString(s.Sid), Heading: s.Heading, ClearedAltitude: s.ClearedAltitude, AssignedSquawk: optionalString(s.AssignedSquawk), AircraftType: optionalString(s.AircraftType), EngineType: s.EngineType, Route: optionalString(s.Route)}
	session := &models.Session{ID: t.session.Id, Airport: t.session.Airport, Name: t.session.Name}
	for _, r := range t.session.Runways {
		if r.Departure {
			session.ActiveRunways.DepartureRunways = append(session.ActiveRunways.DepartureRunways, r.Name)
		}
	}
	for _, s := range t.session.AvailableSids {
		session.AvailableSids = append(session.AvailableSids, pkgmodels.SidInfo{Name: s.Name, Runway: s.Runway})
	}
	if e := t.a.Indexes[pb.EntityKind_ECFMP_STATE][s.Callsign]; e != nil {
		model.CdmData = &models.CdmData{}
		for _, r := range e.Value.GetEcfmpState().Restrictions {
			model.CdmData.EcfmpRestrictions = append(model.CdmData.EcfmpRestrictions, models.EcfmpRestriction{Type: r.Kind, Routes: r.Routes})
		}
	}
	return model, session
}
func (t *pdcTransition) clearance(_ context.Context, remarks string) (ClearanceOptions, *mandatoryRouteReview, error) {
	strip, session := t.policyInputs()
	review := resolveMandatoryRouteReview(strip, session.AvailableSids)
	if t.strip.Runway == "" || (t.strip.Sid == "" && (t.strip.GetHeading() == 0 || t.strip.GetClearedAltitude() <= 0)) && (review == nil || review.SID == "") {
		return ClearanceOptions{}, nil, fmt.Errorf("strip missing required clearance data")
	}
	squawk, err := getAssignedPDCSquawk(strip)
	if err != nil {
		return ClearanceOptions{}, nil, err
	}
	options := ClearanceOptions{Callsign: t.strip.Callsign, Origin: t.strip.Departure, Destination: t.strip.Destination, Runway: t.strip.Runway, Squawk: squawk, Atis: t.seq.Atis, Remarks: remarks, SID: t.strip.Sid}
	for _, e := range t.a.EntitiesByKind(pb.EntityKind_ATIS) {
		if e.Value.GetAtis().DepartureCode != "" {
			options.Atis = e.Value.GetAtis().DepartureCode
		}
	}
	if t.seq.RequestChannel == "WEB" {
		options.Atis = t.seq.Atis
	}
	if t.strip.GetHeading() != 0 && t.strip.GetClearedAltitude() > 0 {
		options.SID = ""
		options.Heading = fmt.Sprintf("%03d", t.strip.GetHeading())
		options.Vectors = "FIRST WAYPOINT"
		options.ClimbTo = fmt.Sprintf("%04d FT", t.strip.GetClearedAltitude())
		if t.strip.GetClearedAltitude() > 5000 {
			options.ClimbTo = fmt.Sprintf("FL%03d", t.strip.GetClearedAltitude()/100)
		}
	}
	if review != nil {
		if review.SID == "" {
			return ClearanceOptions{}, nil, fmt.Errorf("mandatory route requires matching SID")
		}
		options.SID, options.Route = review.SID, review.Route
	}
	for _, sector := range []string{"SQ", "DEL"} {
		for _, e := range t.a.EntitiesByKind(pb.EntityKind_SECTOR_OWNER) {
			s := e.Value.GetSectorOwner()
			if s.Sector == sector && s.Position != "" {
				options.NextFrequency = s.Position
				break
			}
		}
		if options.NextFrequency != "" {
			break
		}
	}
	if options.NextFrequency == "" {
		return ClearanceOptions{}, nil, fmt.Errorf("no frequency found for sector SQ or DEL")
	}
	priority, err := getPdcAirborneControllerPriority(optionalString(options.SID))
	if err != nil {
		return ClearanceOptions{}, nil, err
	}
	options.DepartureFrequency = "122.8"
	online := map[string]struct{}{}
	_, presence, err := t.c.Writer.Projection.ObservationSnapshot(t.session.Id)
	if err != nil {
		return ClearanceOptions{}, nil, err
	}
	nodes := map[string]bool{}
	for _, p := range presence {
		if n := p.Value.GetNode(); n != nil && n.Ready && t.c.clock().Sub(p.Observed) >= 0 && t.c.clock().Sub(p.Observed) < 10*time.Second {
			nodes[n.NodeId] = true
		}
	}
	for _, p := range presence {
		if client := p.Value.GetClient(); client != nil && client.SessionId == t.session.Id && client.Kind == pb.ClientPresence_EUROSCOPE && !client.Observer && nodes[client.NodeId] && t.c.clock().Sub(p.Observed) >= 0 && t.c.clock().Sub(p.Observed) < 10*time.Second {
			if e := t.a.Indexes[pb.EntityKind_CONTROLLER][client.Cid]; e != nil {
				controller := e.Value.GetController()
				model := &models.Controller{Callsign: controller.Callsign, Position: controller.Position, Observer: controller.Observer}
				if !shared.IsOperationalController(model) {
					continue
				}
				if position, ok := resolvePdcOperationalPosition(model); ok && shared.IsOperationalControllerForPosition(model, position) {
					addNormalizedFrequency(online, position.Frequency)
				} else {
					addNormalizedFrequency(online, controller.Position)
				}
				for _, source := range t.c.Transceivers {
					for _, frequency := range source.GetFrequencies(controller.Callsign) {
						addNormalizedFrequency(online, frequency)
					}
				}
			}
		}
	}
	if len(online) == 0 {
		for _, e := range t.a.EntitiesByKind(pb.EntityKind_SECTOR_OWNER) {
			addNormalizedFrequency(online, e.Value.GetSectorOwner().Position)
		}
	}
	for _, name := range priority {
		pos, e := config.GetPositionByName(name)
		if e != nil {
			continue
		}
		if _, available := online[normalizeFrequency(pos.Frequency)]; available {
			options.DepartureFrequency = pos.Frequency
			break
		}
	}
	return options, review, nil
}

func (c *Candidate) timeout(r *pb.CommandRequest, a *cluster.Aggregate, deadline *pb.EntitySnapshot) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
	d := deadline.Value.GetSessionDeadline()
	if r.ExpectedEntityRevision == nil || *r.ExpectedEntityRevision != deadline.Revision || d.DueAt == nil || c.clock().Before(d.DueAt.AsTime()) {
		return pdcReject(pb.CommandReply_REVISION_CONFLICT, "PDC deadline rearmed or not elapsed")
	}
	t, err := c.transition(r, a, d.Callsign)
	if err != nil {
		return pdcReject(pb.CommandReply_NOT_FOUND, err.Error())
	}
	if t.old == nil || t.old.Revision != d.SourceRevision || t.seq.State != "CLEARED" || !proto.Equal(t.seq.Deadline, d.DueAt) {
		return &pb.DomainChange{Changes: []*pb.EntityChange{pdcDelete(deadline, pb.EntityKind_SESSION_DEADLINE)}}, pb.CommandReply_COMMITTED, 0, nil
	}
	if err = t.fail("NO_RESPONSE"); err != nil {
		return pdcReject(pb.CommandReply_UNAVAILABLE, err.Error())
	}
	if t.seq.RequestChannel == "CPDLC" {
		if err = t.outbound(pb.PdcProviderMessage_KIND_NO_RESPONSE, 0, &t.seq.Sequence, nil, ""); err != nil {
			return pdcReject(pb.CommandReply_UNAVAILABLE, err.Error())
		}
	}
	if t.session.Master.GetCid() != "" {
		_ = t.effect("STATE_CHANGE", nil)
	}
	return t.finish()
}

// Keep UUID validation local to the adapter boundary.
func validPdcUUID(id string) bool { v, e := uuid.Parse(id); return e == nil && v.String() == id }
