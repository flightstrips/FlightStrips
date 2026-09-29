package cluster

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	pb "FlightStrips/pkg/events/cluster"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// PdcTactical is a candidate-only adapter. Production continues to construct
// the PostgreSQL services until the coordinated cutover.
type PdcTactical struct{ Store LifecycleStore }

func (a PdcTactical) Execute(ctx context.Context, request *pb.CommandRequest) (*pb.CommandReply, error) {
	if a.Store == nil || request == nil || request.GetAggregate().GetSession() == nil ||
		(request.GetClient().GetPdc() == nil && request.GetClient().GetTactical() == nil) {
		return nil, fmt.Errorf("PDC or tactical session command required")
	}
	reply := a.Store.Execute(ctx, request)
	if reply == nil {
		return nil, fmt.Errorf("candidate store returned no reply")
	}
	if reply.Status != pb.CommandReply_COMMITTED && reply.Status != pb.CommandReply_PENDING {
		return reply, fmt.Errorf("candidate command: %s: %s", reply.Status, reply.GetOutcome().GetDetail())
	}
	if reply.GetOutcome().GetStatus() == pb.CommandOutcome_FAILED {
		return reply, fmt.Errorf("candidate command: %s", reply.Outcome.Detail)
	}
	return reply, nil
}

func (a PdcTactical) Pdc(ctx context.Context, session int32, callsign string) (*pb.PdcSequence, uint64, error) {
	if a.Store == nil {
		return nil, 0, fmt.Errorf("candidate store unavailable")
	}
	state, err := a.Store.Read(ctx, sessionRef(session))
	if err != nil {
		return nil, 0, err
	}
	key := strings.ToUpper(strings.TrimSpace(callsign))
	if e := state.Indexes[pb.EntityKind_PDC_SEQUENCE][key]; e != nil {
		return proto.Clone(e.GetValue().GetPdcSequence()).(*pb.PdcSequence), e.Revision, nil
	}
	return nil, 0, fmt.Errorf("PDC %s not found", key)
}

func (a PdcTactical) Tactical(ctx context.Context, session int32, id uint64) (*pb.TacticalStrip, uint64, error) {
	if a.Store == nil || id == 0 {
		return nil, 0, fmt.Errorf("candidate store or tactical ID unavailable")
	}
	state, err := a.Store.Read(ctx, sessionRef(session))
	if err != nil {
		return nil, 0, err
	}
	if e := state.Indexes[pb.EntityKind_TACTICAL_STRIP][strconv.FormatUint(id, 10)]; e != nil {
		return proto.Clone(e.GetValue().GetTacticalStrip()).(*pb.TacticalStrip), e.Revision, nil
	}
	return nil, 0, fmt.Errorf("tactical strip %d not found", id)
}

func PlanPdcTactical(_ context.Context, request *pb.CommandRequest, state *Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
	if request.GetAggregate().GetSession() == nil || state == nil {
		return nil, pb.CommandReply_INVALID_ARGUMENT, 0, fmt.Errorf("session aggregate required")
	}
	if action := request.GetClient().GetPdc(); action != nil {
		return planPdc(request, state, action)
	}
	if action := request.GetClient().GetTactical(); action != nil {
		return planTactical(request, state, action)
	}
	return nil, pb.CommandReply_INVALID_ARGUMENT, 0, fmt.Errorf("missing candidate action")
}

func candidateController(request *pb.CommandRequest, state *Aggregate) (*pb.Controller, error) {
	actor := request.GetActor()
	if actor.GetKind() != pb.Actor_CONTROLLER || actor.GetSessionId() != request.GetAggregate().GetSession().GetId() {
		return nil, fmt.Errorf("session controller required")
	}
	e := state.Indexes[pb.EntityKind_CONTROLLER][actor.GetId()]
	if e == nil || e.GetValue().GetController().GetObserver() {
		return nil, fmt.Errorf("active controller required")
	}
	return e.GetValue().GetController(), nil
}

func candidateRevision(request *pb.CommandRequest, old *pb.EntitySnapshot) (uint64, pb.CommandReply_Status, error) {
	var current uint64
	if old != nil {
		current = old.Revision
	}
	if request.ExpectedEntityRevision == nil {
		return current, pb.CommandReply_INVALID_ARGUMENT, fmt.Errorf("expected entity revision required")
	}
	if *request.ExpectedEntityRevision != current {
		return current, pb.CommandReply_REVISION_CONFLICT, fmt.Errorf("stale entity revision")
	}
	return current, pb.CommandReply_COMMITTED, nil
}

func candidateUpsert(key string, old *pb.EntitySnapshot, record *pb.EntityRecord) *pb.EntityChange {
	revision := uint64(1)
	if old != nil {
		revision = old.Revision + 1
	}
	return &pb.EntityChange{Key: key, Revision: revision, Operation: &pb.EntityChange_Upsert{Upsert: record}}
}

func candidateDelete(key string, old *pb.EntitySnapshot, kind pb.EntityKind) *pb.EntityChange {
	return &pb.EntityChange{Key: key, Revision: old.Revision + 1, Operation: &pb.EntityChange_Delete{Delete: &pb.DeleteEntity{Kind: kind}}}
}

func sortCandidateChanges(changes []*pb.EntityChange) {
	sort.Slice(changes, func(i, j int) bool {
		left, _ := changeKind(changes[i])
		right, _ := changeKind(changes[j])
		if left != right {
			return left < right
		}
		return changes[i].Key < changes[j].Key
	})
}

func planPdc(request *pb.CommandRequest, state *Aggregate, action *pb.PdcAction) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
	session, err := stripSession(request, state)
	if err != nil {
		return nil, pb.CommandReply_NOT_FOUND, 0, err
	}
	key := strings.ToUpper(strings.TrimSpace(action.Callsign))
	strip := state.Indexes[pb.EntityKind_STRIP][key]
	if key == "" || strip == nil {
		return nil, pb.CommandReply_NOT_FOUND, 0, fmt.Errorf("strip not found")
	}
	old := state.Indexes[pb.EntityKind_PDC_SEQUENCE][key]
	current, status, err := candidateRevision(request, old)
	if err != nil {
		return nil, status, current, err
	}
	actor := request.GetActor()
	controller := (*pb.Controller)(nil)
	if actor.GetKind() == pb.Actor_CONTROLLER {
		controller, err = candidateController(request, state)
		if err != nil {
			return nil, pb.CommandReply_UNAUTHORIZED, current, err
		}
	} else if actor.GetKind() == pb.Actor_PILOT && strings.EqualFold(actor.GetId(), key) && actor.GetSessionId() == request.GetAggregate().GetSession().GetId() {
		// Pilot requests are bound to their own callsign.
	} else if actor.GetKind() == pb.Actor_PROVIDER && actor.GetId() == "hoppie" && actor.GetSessionId() == request.GetAggregate().GetSession().GetId() {
		// The provider adapter authenticates the incoming Hoppie callsign.
	} else {
		return nil, pb.CommandReply_UNAUTHORIZED, current, fmt.Errorf("PDC actor does not own callsign")
	}
	sequence := &pb.PdcSequence{Callsign: key, State: "NONE"}
	if old != nil {
		sequence = proto.Clone(old.GetValue().GetPdcSequence()).(*pb.PdcSequence)
	}
	now := timestamppb.New(time.Now().UTC())
	change := &pb.DomainChange{}
	switch x := action.GetChange().(type) {
	case *pb.PdcAction_Issue:
		if x.Issue == nil {
			return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("missing PDC issue")
		}
		if controller == nil {
			// Pilot and Hoppie issue actions with empty clearance request review.
			if strings.TrimSpace(x.Issue.Clearance) != "" || sequence.State != "NONE" && sequence.State != "FAILED" && sequence.State != "REVERT_TO_VOICE" {
				return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("PDC request already pending or invalid")
			}
			channel := "WEB"
			if actor.GetKind() == pb.Actor_PROVIDER {
				channel = "CPDLC"
			}
			if x.Issue.RequestChannel != "" && x.Issue.RequestChannel != channel {
				return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("PDC request channel does not match actor")
			}
			sequence.State, sequence.RequestedAt, sequence.RequestChannel = "REQUESTED", now, channel
			sequence.RequestRemarks = x.Issue.RequestRemarks
			break
		}
		if sequence.State != "REQUESTED" && sequence.State != "REQUESTED_WITH_FAULTS" || strings.TrimSpace(x.Issue.Clearance) == "" {
			return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("PDC is not awaiting issuance")
		}
		master := session.GetValue().GetSession().GetMaster()
		if master == nil || master.Cid == "" {
			return nil, pb.CommandReply_UNAVAILABLE, current, fmt.Errorf("EuroScope master is unavailable")
		}
		s := proto.Clone(session.GetValue().GetSession()).(*pb.Session)
		if s.NextMessageId == 0 || s.NextMessageId == math.MaxUint64 {
			return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("PDC message sequence exhausted")
		}
		sequence.Sequence, sequence.State, sequence.IssuedAt = s.NextMessageId, "CLEARED", now
		sequence.IssuedByCid, sequence.Sent = actor.Id, false
		s.NextMessageId++
		change.Changes = append(change.Changes, candidateUpsert(session.Key, session, &pb.EntityRecord{Value: &pb.EntityRecord_Session{Session: s}}))
		deadline := timestamppb.New(now.AsTime().Add(10 * time.Minute))
		sequence.Deadline = deadline
		deadlineID := "pdc." + key
		oldDeadline := state.Indexes[pb.EntityKind_SESSION_DEADLINE][deadlineID]
		change.Changes = append(change.Changes, candidateUpsert(deadlineID, oldDeadline, &pb.EntityRecord{Value: &pb.EntityRecord_SessionDeadline{SessionDeadline: &pb.SessionDeadline{Id: deadlineID, Kind: "pdc-response", DueAt: deadline, Callsign: key, CommandId: request.CommandId}}}))
		change.Effects = []*pb.EffectRecord{{CommandId: request.CommandId, TargetCid: master.Cid, TargetConnectionId: &master.ConnectionId, OwnerEpoch: state.ownerEpoch(), MasterEpoch: master.Epoch, Status: pb.EffectRecord_WAITING, Payload: &pb.EffectRecord_Pdc{Pdc: &pb.PdcEffect{Callsign: key, Action: "ISSUE", Clearance: x.Issue.Clearance}}, DispatchDeadline: timestamppb.New(now.AsTime().Add(30 * time.Second))}}
	case *pb.PdcAction_RevertToVoice:
		if controller == nil || sequence.State == "NONE" {
			return nil, pb.CommandReply_UNAUTHORIZED, current, fmt.Errorf("controller request required")
		}
		if sequence.State == "CLEARED" {
			master := session.GetValue().GetSession().GetMaster()
			if master == nil || master.Cid == "" {
				return nil, pb.CommandReply_UNAVAILABLE, current, fmt.Errorf("EuroScope master is unavailable")
			}
			change.Effects = []*pb.EffectRecord{{CommandId: request.CommandId, TargetCid: master.Cid, TargetConnectionId: &master.ConnectionId, OwnerEpoch: state.ownerEpoch(), MasterEpoch: master.Epoch, Status: pb.EffectRecord_WAITING, Payload: &pb.EffectRecord_Pdc{Pdc: &pb.PdcEffect{Callsign: key, Action: "REVERT_TO_VOICE"}}, DispatchDeadline: timestamppb.New(now.AsTime().Add(30 * time.Second))}}
		}
		sequence.State = "REVERT_TO_VOICE"
		sequence.Deadline = nil
	case *pb.PdcAction_Acknowledge:
		if actor.GetKind() != pb.Actor_PILOT || sequence.State != "CLEARED" {
			return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("PDC is not awaiting pilot acknowledgment")
		}
		sequence.State, sequence.Deadline = "CONFIRMED", nil
	case *pb.PdcAction_Unable:
		if actor.GetKind() != pb.Actor_PILOT || sequence.State != "CLEARED" {
			return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("PDC is not awaiting pilot response")
		}
		sequence.State, sequence.Deadline = "FAILED", nil
	default:
		return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("unknown PDC action")
	}
	if sequence.Deadline == nil {
		deadlineID := "pdc." + key
		if oldDeadline := state.Indexes[pb.EntityKind_SESSION_DEADLINE][deadlineID]; oldDeadline != nil {
			change.Changes = append(change.Changes, candidateDelete(deadlineID, oldDeadline, pb.EntityKind_SESSION_DEADLINE))
		}
	}
	change.Changes = append(change.Changes, candidateUpsert(key, old, &pb.EntityRecord{Value: &pb.EntityRecord_PdcSequence{PdcSequence: sequence}}))
	sortCandidateChanges(change.Changes)
	return change, pb.CommandReply_COMMITTED, current, nil
}

func planTactical(request *pb.CommandRequest, state *Aggregate, action *pb.TacticalAction) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
	session, err := stripSession(request, state)
	if err != nil {
		return nil, pb.CommandReply_NOT_FOUND, 0, err
	}
	controller, err := candidateController(request, state)
	if err != nil {
		return nil, pb.CommandReply_UNAUTHORIZED, 0, err
	}
	key := strconv.FormatUint(action.StripId, 10)
	old := state.Indexes[pb.EntityKind_TACTICAL_STRIP][key]
	if action.GetCreate() != nil && action.StripId != 0 || action.GetCreate() == nil && old == nil {
		return nil, pb.CommandReply_NOT_FOUND, 0, fmt.Errorf("invalid tactical ID")
	}
	current, status, err := candidateRevision(request, old)
	if err != nil {
		return nil, status, current, err
	}
	change := &pb.DomainChange{}
	var tactical *pb.TacticalStrip
	if old != nil {
		tactical = proto.Clone(old.GetValue().GetTacticalStrip()).(*pb.TacticalStrip)
	}
	if create := action.GetCreate(); create != nil {
		kind := strings.ToUpper(strings.TrimSpace(create.Kind))
		if kind != "MEMAID" && kind != "CROSSING" && kind != "START" && kind != "LAND" || !validTacticalBay(kind, create.Bay, create.Label, session.GetValue().GetSession()) {
			return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("invalid tactical kind, bay or runway label")
		}
		s := proto.Clone(session.GetValue().GetSession()).(*pb.Session)
		if s.NextTacticalId == 0 || s.NextTacticalId == math.MaxUint64 {
			return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("tactical ID exhausted")
		}
		sequence, err := endOfStripBay(state, create.Bay, "")
		if err != nil {
			return nil, pb.CommandReply_INVALID_ARGUMENT, current, err
		}
		tactical = &pb.TacticalStrip{Id: s.NextTacticalId, Title: create.Title, Body: create.Body, Bay: create.Bay, Kind: kind, Label: create.Label, Aircraft: create.Aircraft, ProducedBy: controller.Callsign, OwnerCid: request.Actor.Id, Sequence: sequence, CreatedAt: timestamppb.New(time.Now().UTC())}
		s.NextTacticalId++
		change.Changes = append(change.Changes, candidateUpsert(session.Key, session, &pb.EntityRecord{Value: &pb.EntityRecord_Session{Session: s}}))
		key = strconv.FormatUint(tactical.Id, 10)
	} else {
		switch x := action.GetChange().(type) {
		case *pb.TacticalAction_Delete:
			if tactical.OwnerCid != request.Actor.Id {
				return nil, pb.CommandReply_UNAUTHORIZED, current, fmt.Errorf("only owner may delete tactical strip")
			}
			change.Changes = append(change.Changes, candidateDelete(key, old, pb.EntityKind_TACTICAL_STRIP))
			sortCandidateChanges(change.Changes)
			return change, pb.CommandReply_COMMITTED, current, nil
		case *pb.TacticalAction_Confirm:
			if tactical.Kind != "MEMAID" || tactical.OwnerCid == request.Actor.Id || tactical.Confirmed {
				return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("MEMAID requires another controller's confirmation")
			}
			tactical.Confirmed, tactical.ConfirmedBy = true, controller.Callsign
		case *pb.TacticalAction_ForceAssume:
			tactical.OwnerCid = request.Actor.Id
		case *pb.TacticalAction_Mark:
			if tactical.OwnerCid != request.Actor.Id || x.Mark == nil {
				return nil, pb.CommandReply_UNAUTHORIZED, current, fmt.Errorf("only owner may mark tactical strip")
			}
			tactical.Marked = x.Mark.Marked
		case *pb.TacticalAction_StartTimer:
			if tactical.OwnerCid != request.Actor.Id || tactical.Kind != "START" && tactical.Kind != "LAND" || tactical.TimerStartedAt != nil {
				return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("timer cannot be started")
			}
			tactical.TimerStartedAt = timestamppb.New(time.Now().UTC())
		case *pb.TacticalAction_Move:
			if tactical.OwnerCid != request.Actor.Id || x.Move == nil {
				return nil, pb.CommandReply_UNAUTHORIZED, current, fmt.Errorf("only owner may move tactical strip")
			}
			bay := tactical.Bay
			if x.Move.Bay != nil {
				bay = *x.Move.Bay
			}
			if !validTacticalBay(tactical.Kind, bay, tactical.Label, session.GetValue().GetSession()) {
				return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("invalid tactical bay")
			}
			tactical.Bay = bay
			ordered, err := reorderTactical(state, tactical, x.Move.InsertAfter)
			if err != nil {
				return nil, pb.CommandReply_INVALID_ARGUMENT, current, err
			}
			change.Changes = append(change.Changes, ordered...)
		default:
			return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("unknown tactical action")
		}
	}
	if old == nil || len(change.Changes) == 0 || !containsTacticalChange(change.Changes, key) {
		if old == nil || !proto.Equal(old.GetValue().GetTacticalStrip(), tactical) {
			change.Changes = append(change.Changes, candidateUpsert(key, old, &pb.EntityRecord{Value: &pb.EntityRecord_TacticalStrip{TacticalStrip: tactical}}))
		}
	}
	for _, c := range change.Changes {
		if t := c.GetUpsert().GetTacticalStrip(); t != nil {
			t.Revision = c.Revision
		}
	}
	sortCandidateChanges(change.Changes)
	return change, pb.CommandReply_COMMITTED, current, nil
}

func containsTacticalChange(changes []*pb.EntityChange, key string) bool {
	for _, c := range changes {
		if c.Key == key && c.GetUpsert().GetTacticalStrip() != nil {
			return true
		}
	}
	return false
}

func validTacticalBay(kind, bay, label string, session *pb.Session) bool {
	if !validStripBay(bay) || bay == "NOT_CLEARED" {
		return false
	}
	if kind == "MEMAID" && strings.TrimSpace(label) == "" {
		return false
	}
	if kind != "START" && kind != "LAND" {
		return true
	}
	switch bay {
	case "TWY_ARR", "TAXI", "TAXI_LWR":
		if label == "" {
			return false
		}
		for _, runway := range session.GetRunways() {
			if runway.Name == label {
				return true
			}
		}
		return false
	case "RWY_ARR", "DEPART":
		return label == ""
	}
	return false
}

func reorderTactical(state *Aggregate, target *pb.TacticalStrip, after *pb.StripRef) ([]*pb.EntityChange, error) {
	items := make([]orderedStrip, 0)
	for _, e := range state.Entities {
		if s := e.GetValue().GetStrip(); s != nil && s.Bay == target.Bay {
			items = append(items, orderedStrip{key: e.Key, seq: s.Sequence, strip: s, snapshot: e})
		}
		if t := e.GetValue().GetTacticalStrip(); t != nil && t.Bay == target.Bay && t.Id != target.Id {
			items = append(items, orderedStrip{key: e.Key, seq: t.Sequence, tactical: t, snapshot: e})
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].seq != items[j].seq {
			return items[i].seq < items[j].seq
		}
		return items[i].key < items[j].key
	})
	index := 0
	if after != nil {
		found := false
		for i, item := range items {
			if after.GetFlightCallsign() != "" && item.strip != nil && item.strip.Callsign == after.GetFlightCallsign() || after.GetTacticalId() != 0 && item.tactical != nil && item.tactical.Id == after.GetTacticalId() {
				index, found = i+1, true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("order predecessor absent from bay")
		}
	}
	items = append(items, orderedStrip{})
	copy(items[index+1:], items[index:])
	items[index] = orderedStrip{key: strconv.FormatUint(target.Id, 10), tactical: target, snapshot: state.Indexes[pb.EntityKind_TACTICAL_STRIP][strconv.FormatUint(target.Id, 10)]}
	if uint64(len(items)) > math.MaxUint64/stripOrderSpacing {
		return nil, fmt.Errorf("strip order exhausted")
	}
	changes := make([]*pb.EntityChange, 0)
	for i, item := range items {
		sequence := uint64(i+1) * stripOrderSpacing
		if item.strip != nil && item.strip.Sequence != sequence {
			s := proto.Clone(item.strip).(*pb.Strip)
			s.Sequence = sequence
			changes = append(changes, stripChange(item.snapshot, s))
		}
		if item.tactical != nil && (item.tactical.Sequence != sequence || item.tactical.Id == target.Id && !proto.Equal(item.snapshot.GetValue().GetTacticalStrip(), item.tactical)) {
			t := proto.Clone(item.tactical).(*pb.TacticalStrip)
			t.Sequence = sequence
			changes = append(changes, candidateUpsert(item.key, item.snapshot, &pb.EntityRecord{Value: &pb.EntityRecord_TacticalStrip{TacticalStrip: t}}))
		}
	}
	return changes, nil
}

// Both the writer and reducer enforce these invariants, including generic
// system replacements. A replay cannot introduce a second numeric ID or an
// unpaired PDC deadline even if a future planner makes a mistake.
func validatePdcTacticalState(state *Aggregate, domain *pb.DomainChange, staged map[string]*pb.EntitySnapshot) error {
	if state.Ref.GetSession() == nil || domain == nil {
		return nil
	}
	key := strconv.FormatInt(int64(state.Ref.GetSession().Id), 10)
	oldSession := state.Entities[key].GetValue().GetSession()
	newSession := staged[key].GetValue().GetSession()
	if oldSession == nil || newSession == nil {
		return nil
	}
	created := uint64(0)
	allocatedMessages := make(map[uint64]bool)
	allocateMessage := func(id uint64) error {
		if id < oldSession.NextMessageId || allocatedMessages[id] {
			return fmt.Errorf("message ID was not allocated from session")
		}
		allocatedMessages[id] = true
		return nil
	}
	for _, c := range domain.Changes {
		if t := c.GetUpsert().GetTacticalStrip(); t != nil {
			old := state.Indexes[pb.EntityKind_TACTICAL_STRIP][c.Key]
			if old == nil {
				if t.Id != oldSession.NextTacticalId+created {
					return fmt.Errorf("tactical ID was not allocated from session")
				}
				created++
			} else if t.Id != old.GetValue().GetTacticalStrip().Id {
				return fmt.Errorf("tactical ID changed")
			}
		}
		if p := c.GetUpsert().GetPdcSequence(); p != nil {
			old := state.Indexes[pb.EntityKind_PDC_SEQUENCE][c.Key].GetValue().GetPdcSequence()
			if p.State == "CLEARED" && (old == nil || old.State != "CLEARED") {
				if p.Deadline == nil || p.IssuedAt == nil {
					return fmt.Errorf("PDC message sequence was not allocated from session")
				}
				matched := false
				for _, effect := range domain.Effects {
					if effect.CommandId == domain.GetOutcome().GetCommandId() && effect.GetPdc().GetCallsign() == p.Callsign && effect.GetPdc().GetAction() == "ISSUE" && effect.Status == pb.EffectRecord_WAITING && effect.DispatchDeadline != nil {
						matched = true
					}
				}
				if !matched {
					return fmt.Errorf("PDC issuance is missing its requested effect")
				}
				if err := allocateMessage(p.Sequence); err != nil {
					return err
				}
			}
		}
		if message := c.GetUpsert().GetFrontendMessage(); message != nil && state.Indexes[pb.EntityKind_FRONTEND_MESSAGE][c.Key] == nil {
			if c.Key != strconv.FormatUint(message.Id, 10) {
				return fmt.Errorf("frontend message ID does not match its key")
			}
			if err := allocateMessage(message.Id); err != nil {
				return err
			}
		}
	}
	if oldSession.NextTacticalId > math.MaxUint64-created || newSession.NextTacticalId != oldSession.NextTacticalId+created {
		return fmt.Errorf("tactical allocation counter changed outside creation")
	}
	messageCount := uint64(len(allocatedMessages))
	if oldSession.NextMessageId > math.MaxUint64-messageCount || newSession.NextMessageId != oldSession.NextMessageId+messageCount {
		return fmt.Errorf("session message counter changed outside allocation")
	}
	for offset := uint64(0); offset < messageCount; offset++ {
		if !allocatedMessages[oldSession.NextMessageId+offset] {
			return fmt.Errorf("session message IDs are not contiguous")
		}
	}
	seen := map[uint64]bool{}
	for _, e := range staged {
		if t := e.GetValue().GetTacticalStrip(); t != nil {
			if t.Id == 0 || t.Id >= newSession.NextTacticalId || seen[t.Id] || t.Revision != e.Revision || t.Sequence == 0 || t.OwnerCid == "" || !validTacticalBay(t.Kind, t.Bay, t.Label, newSession) {
				return fmt.Errorf("invalid tactical identity, revision, bay or owner")
			}
			seen[t.Id] = true
		}
		if p := e.GetValue().GetPdcSequence(); p != nil {
			if p.Callsign == "" || staged[entitySlot(staged, pb.EntityKind_STRIP, p.Callsign)].GetValue().GetStrip() == nil || p.State == "CLEARED" && (p.Deadline == nil || p.IssuedAt == nil || p.Sequence == 0) || p.Sent && p.IssuedAt == nil {
				return fmt.Errorf("invalid PDC state")
			}
			deadlineID := "pdc." + p.Callsign
			deadline := staged[entitySlot(staged, pb.EntityKind_SESSION_DEADLINE, deadlineID)].GetValue().GetSessionDeadline()
			if p.Deadline != nil && (deadline == nil || !proto.Equal(deadline.DueAt, p.Deadline)) || p.Deadline == nil && deadline != nil {
				return fmt.Errorf("PDC deadline mismatch")
			}
		}
	}
	return nil
}
