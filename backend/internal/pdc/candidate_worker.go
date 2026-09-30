package pdc

import (
	"context"
	"encoding/binary"
	"fmt"
	"sort"
	"strconv"
	"time"

	"FlightStrips/internal/cluster"
	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func pdcSystem(id int32, command, actor string, rev *uint64, action *pb.SystemCommand) *pb.CommandRequest {
	return &pb.CommandRequest{ProtocolRevision: 1, CommandId: command, Aggregate: pdcRef(id), Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: actor, SessionId: &id}, ExpectedEntityRevision: rev, Command: &pb.CommandRequest_System{System: action}}
}
func pdcUpdate(key string, v *pb.EntityRecord) *pb.SystemCommand {
	return &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: &pb.UpdateEntity{Key: key, Value: v}}}
}
func pdcReply(reply *pb.CommandReply) error {
	if reply == nil || (reply.Status != pb.CommandReply_COMMITTED && reply.Status != pb.CommandReply_PENDING) || reply.Outcome == nil || reply.Outcome.Status == pb.CommandOutcome_FAILED {
		return fmt.Errorf("PDC command not committed: %v", reply)
	}
	return nil
}

// PDC is the SessionWork callback. It starts no goroutine or local timer.
// Every pass reconstructs accepted pages, one-shot calls and due timeouts.
func (c *Candidate) PDC(ctx context.Context, id int32) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	ref := pdcRef(id)
	if c.Writer.Lease == nil || !c.Writer.Lease.CanWrite(ref) {
		return fmt.Errorf("PDC session is not owned")
	}
	worker := cluster.ExternalCallWorker{Writer: c.Writer}
	if err := worker.Resume(ctx, ref); err != nil {
		return err
	}
	state, err := c.Writer.Read(ctx, ref)
	if err != nil {
		return err
	}
	station := state.Indexes[pb.EntityKind_SESSION][strconv.Itoa(int(id))].Value.GetSession().Airport
	if c.Provider != nil {
		if err = c.processPage(ctx, id, station); err != nil {
			return err
		}
		state, err = c.Writer.Read(ctx, ref)
		if err != nil {
			return err
		}
	}
	for _, deadline := range state.EntitiesByKind(pb.EntityKind_SESSION_DEADLINE) {
		d := deadline.Value.GetSessionDeadline()
		if d.Kind == "pdc-response" && !c.clock().Before(d.DueAt.AsTime()) {
			req := pdcSystem(id, pdcID(d.CommandId, "timeout-"+strconv.FormatUint(deadline.Revision, 10)), "session-worker", proto.Uint64(deadline.Revision), &pb.SystemCommand{Action: &pb.SystemCommand_RemoveEntity{RemoveEntity: &pb.RemoveEntity{Key: deadline.Key, Kind: pb.EntityKind_SESSION_DEADLINE}}})
			if err = pdcReply(c.Writer.Execute(ctx, req)); err != nil {
				return err
			}
		}
	}
	if c.Provider == nil {
		return c.processPlugins(ctx, id)
	} // Web policy and response deadlines still run.
	if err = c.processPlugins(ctx, id); err != nil {
		return err
	}
	if err = c.sendPending(ctx, id, worker); err != nil {
		return err
	}
	state, err = c.Writer.Read(ctx, ref)
	if err != nil {
		return err
	}
	key := "pdc-poll." + station
	deadline := state.Indexes[pb.EntityKind_SESSION_DEADLINE][key]
	if deadline == nil {
		command := uuid.NewString()
		d := &pb.SessionDeadline{Id: key, Kind: "pdc-poll", DueAt: timestamppb.New(c.clock()), CommandId: pdcID(command, "slot"), SourceRevision: 1}
		if err = pdcReply(c.Writer.Execute(ctx, pdcSystem(id, command, "pdc-poll", proto.Uint64(0), pdcUpdate(key, &pb.EntityRecord{Value: &pb.EntityRecord_SessionDeadline{SessionDeadline: d}})))); err != nil {
			return err
		}
		state, err = c.Writer.Read(ctx, ref)
		if err != nil {
			return err
		}
		deadline = state.Indexes[pb.EntityKind_SESSION_DEADLINE][key]
	}
	d := deadline.Value.GetSessionDeadline()
	if c.clock().Before(d.DueAt.AsTime()) {
		return nil
	}
	// A prior attempt is recovered, never repeated. Rearming creates a later
	// slot with a distinct workflow even when the previous poll was uncertain.
	if prior := state.Workflows[d.CommandId]; prior != nil {
		if prior.Status == pb.WorkflowRecord_PENDING {
			return nil
		}
		next := nextPdcPoll(d, c.clock())
		return pdcReply(c.Writer.Execute(ctx, pdcSystem(id, pdcID(d.CommandId, "rearm"), "pdc-poll", proto.Uint64(deadline.Revision), pdcUpdate(key, &pb.EntityRecord{Value: &pb.EntityRecord_SessionDeadline{SessionDeadline: next}}))))
	}
	resource := "station/" + station
	_, err = worker.Run(ctx, cluster.ExternalCallSpec{Source: ref, Destination: ref, WorkflowID: d.CommandId, Step: "external/hoppie/poll",
		Fetch: func(ctx context.Context) (proto.Message, error) {
			raw, err := c.Provider.Poll(ctx, station)
			if err != nil {
				return nil, err
			}
			page := &pb.HoppiePollPage{Station: station, PollId: d.CommandId, ObservedAt: timestamppb.New(c.clock())}
			seen := map[string]bool{}
			for _, message := range raw {
				m, err := ParseProviderMessage(station, message)
				if err != nil {
					return nil, err
				}
				if !seen[m.MessageId] {
					page.Messages = append(page.Messages, m)
					seen[m.MessageId] = true
				}
			}
			return &pb.ProviderPage{Provider: "hoppie", Resource: resource, Parsed: &pb.ProviderPage_Hoppie{Hoppie: page}}, nil
		},
		Commit: func(ctx context.Context, command string, value proto.Message) *pb.CommandReply {
			page := value.(*pb.ProviderPage)
			name, sha, e := c.Source.PublishProvider(page)
			if e != nil {
				return &pb.CommandReply{Status: pb.CommandReply_UNAVAILABLE}
			}
			checkpoint := &pb.ProviderCheckpoint{Provider: "hoppie", Resource: resource, ObjectName: name, Sha256: sha}
			return c.Writer.Execute(ctx, pdcSystem(id, command, "pdc-poll", nil, pdcUpdate("hoppie."+resource, &pb.EntityRecord{Value: &pb.EntityRecord_ProviderCheckpoint{ProviderCheckpoint: checkpoint}})))
		}})
	if err != nil {
		return err
	}
	if err = c.processPage(ctx, id, station); err != nil {
		return err
	}
	if err = c.processPlugins(ctx, id); err != nil {
		return err
	}
	return c.sendPending(ctx, id, worker)
}

// Deterministic sampling chooses the existing 25–45s interval once per slot.
// It remains identical across retry, failover and snapshot recovery.
func nextPdcPoll(d *pb.SessionDeadline, accepted time.Time) *pb.SessionDeadline {
	v, _ := uuid.Parse(d.CommandId)
	delay := time.Duration(25+binary.BigEndian.Uint32(v[:4])%21) * time.Second
	return &pb.SessionDeadline{Id: d.Id, Kind: "pdc-poll", DueAt: timestamppb.New(accepted.Add(delay)), CommandId: pdcID(d.CommandId, "next-slot"), SourceRevision: d.SourceRevision + 1}
}
func (c *Candidate) processPage(ctx context.Context, id int32, station string) error {
	checkpoint, page, err := c.Source.CheckpointFor(ctx, pdcRef(id), "hoppie", "station/"+station)
	if err != nil {
		return err
	}
	if checkpoint == nil {
		return nil
	}
	state, err := c.Writer.Read(ctx, pdcRef(id))
	if err != nil {
		return err
	}
	for _, m := range page.GetHoppie().Messages {
		if prior := state.Ledger[m.MessageId]; prior != nil {
			continue
		}
		req := &pb.CommandRequest{ProtocolRevision: 1, CommandId: m.MessageId, Aggregate: pdcRef(id), Actor: &pb.Actor{Kind: pb.Actor_PROVIDER, Id: "hoppie", SessionId: &id}, Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_ApplyPdcProviderMessage{ApplyPdcProviderMessage: &pb.ApplyPdcProviderMessage{Message: m}}}}}
		if err = pdcReply(c.Writer.Execute(ctx, req)); err != nil {
			return err
		}
	}
	return nil
}

func (c *Candidate) planPoll(_ context.Context, r *pb.CommandRequest, a *cluster.Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
	u := r.GetSystem().GetUpdateEntity()
	if u == nil {
		return pdcReject(pb.CommandReply_INVALID_ARGUMENT, "PDC poll requires typed result")
	}
	seed := a.Indexes[pb.EntityKind_SESSION][strconv.Itoa(int(r.Aggregate.GetSession().Id))].Value.GetSession()
	key := "pdc-poll." + seed.Airport
	old := a.Indexes[pb.EntityKind_SESSION_DEADLINE][key]
	if d := u.Value.GetSessionDeadline(); d != nil {
		if u.Key != key || d.Id != key || d.Kind != "pdc-poll" || d.DueAt == nil || !validPdcUUID(d.CommandId) || r.ExpectedEntityRevision == nil || *r.ExpectedEntityRevision != old.GetRevision() {
			return pdcReject(pb.CommandReply_REVISION_CONFLICT, "PDC poll deadline changed")
		}
		if old != nil {
			prior := old.Value.GetSessionDeadline()
			attempt := a.Workflows[prior.CommandId]
			minimum := nextPdcPoll(prior, prior.DueAt.AsTime())
			if attempt == nil || attempt.Status != pb.WorkflowRecord_FAILED || d.CommandId != minimum.CommandId || d.SourceRevision != minimum.SourceRevision || d.DueAt.AsTime().Before(minimum.DueAt.AsTime()) {
				return pdcReject(pb.CommandReply_INVALID_ARGUMENT, "PDC poll cannot rearm unaccepted slot")
			}
		}
		return &pb.DomainChange{Changes: []*pb.EntityChange{pdcUpsert(old, key, u.Value)}}, pb.CommandReply_COMMITTED, 0, nil
	}
	checkpoint := u.Value.GetProviderCheckpoint()
	if checkpoint == nil || checkpoint.Provider != "hoppie" || checkpoint.Resource != "station/"+seed.Airport || u.Key != "hoppie."+checkpoint.Resource || old == nil {
		return pdcReject(pb.CommandReply_INVALID_ARGUMENT, "PDC poll checkpoint mismatch")
	}
	page, err := c.Source.ReadProvider(checkpoint.ObjectName, checkpoint.Sha256, checkpoint.Provider, checkpoint.Resource)
	if err != nil {
		return pdcReject(pb.CommandReply_UNAVAILABLE, err.Error())
	}
	poll := page.GetHoppie()
	d := old.Value.GetSessionDeadline()
	attempt := a.Workflows[d.CommandId]
	if poll == nil || poll.PollId != d.CommandId || attempt == nil || attempt.Status != pb.WorkflowRecord_PENDING || attempt.DerivedCommandId != r.CommandId {
		return pdcReject(pb.CommandReply_INVALID_ARGUMENT, "PDC poll result identity mismatch")
	}
	// Checkpoint and the next deadline commit atomically before any message.
	changes := []*pb.EntityChange{pdcUpsert(a.Indexes[pb.EntityKind_PROVIDER_CHECKPOINT][u.Key], u.Key, u.Value), pdcUpsert(old, key, &pb.EntityRecord{Value: &pb.EntityRecord_SessionDeadline{SessionDeadline: nextPdcPoll(d, poll.ObservedAt.AsTime())}})}
	out := &pb.DomainChange{Changes: changes}
	pdcSort(out)
	return out, pb.CommandReply_COMMITTED, 0, nil
}

func (c *Candidate) sendPending(ctx context.Context, id int32, worker cluster.ExternalCallWorker) error {
	state, err := c.Writer.Read(ctx, pdcRef(id))
	if err != nil {
		return err
	}
	keys := []string{}
	for key, w := range state.Workflows {
		if w.Step == "pdc/outbound" && w.Status == pb.WorkflowRecord_PENDING {
			keys = append(keys, key)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		return state.Indexes[pb.EntityKind_PDC_PROVIDER_MESSAGE][keys[i]].Value.GetPdcProviderMessage().Sequence < state.Indexes[pb.EntityKind_PDC_PROVIDER_MESSAGE][keys[j]].Value.GetPdcProviderMessage().Sequence
	})
	for _, key := range keys {
		fresh, err := c.Writer.Read(ctx, pdcRef(id))
		if err != nil {
			return err
		}
		message := fresh.Indexes[pb.EntityKind_PDC_PROVIDER_MESSAGE][key]
		m := message.Value.GetPdcProviderMessage()
		intent := fresh.Workflows[key]
		attempt := fresh.Workflows[intent.DerivedCommandId]
		if attempt == nil {
			_, err = worker.Run(ctx, cluster.ExternalCallSpec{Source: pdcRef(id), Destination: pdcRef(id), WorkflowID: intent.DerivedCommandId, Step: "external/hoppie/send", Reserve: func(ctx context.Context) (bool, error) {
				a, e := c.Writer.Read(ctx, pdcRef(id))
				if e != nil {
					return false, e
				}
				if m.Kind == pb.PdcProviderMessage_KIND_CLEARANCE {
					s := a.Indexes[pb.EntityKind_PDC_SEQUENCE][m.To].GetValue().GetPdcSequence()
					return s != nil && s.State == "CLEARED" && s.Sequence == m.Sequence && c.clock().Before(s.Deadline.AsTime()), nil
				}
				return true, nil
			},
				Fetch: func(ctx context.Context) (proto.Message, error) {
					packet, e := renderProviderMessage(m)
					if e != nil {
						return nil, e
					}
					if e = c.Provider.SendCPDLC(ctx, m.From, m.To, packet); e != nil {
						return nil, e
					}
					result := copyProvider(m)
					result.ProviderAcceptedAt = timestamppb.New(c.clock())
					return result, nil
				},
				Commit: func(ctx context.Context, command string, value proto.Message) *pb.CommandReply {
					return c.Writer.Execute(ctx, pdcSystem(id, command, "pdc-send-result", proto.Uint64(message.Revision), pdcUpdate(key, &pb.EntityRecord{Value: &pb.EntityRecord_PdcProviderMessage{PdcProviderMessage: value.(*pb.PdcProviderMessage)}})))
				}})
			if err != nil {
				return err
			}
		}
		// Resolve an uncertain attempt without a second provider send. The durable
		// outbound intent is terminal even if no successful acceptance was proven.
		fresh, err = c.Writer.Read(ctx, pdcRef(id))
		if err != nil {
			return err
		}
		attempt = fresh.Workflows[intent.DerivedCommandId]
		if attempt != nil && attempt.Status == pb.WorkflowRecord_FAILED {
			req := pdcSystem(id, pdcID(key, "uncertain"), "pdc-send-result", proto.Uint64(message.Revision), pdcUpdate(key, &pb.EntityRecord{Value: &pb.EntityRecord_PdcProviderMessage{PdcProviderMessage: m}}))
			if err = pdcReply(c.Writer.Execute(ctx, req)); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *Candidate) planSendResult(r *pb.CommandRequest, a *cluster.Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
	u := r.GetSystem().GetUpdateEntity()
	if u == nil || u.Value.GetPdcProviderMessage() == nil {
		return pdcReject(pb.CommandReply_INVALID_ARGUMENT, "parsed PDC send result required")
	}
	old := a.Indexes[pb.EntityKind_PDC_PROVIDER_MESSAGE][u.Key]
	intent := a.Workflows[u.Key]
	if old == nil || intent == nil || intent.Step != "pdc/outbound" || r.ExpectedEntityRevision == nil || *r.ExpectedEntityRevision != old.Revision {
		return pdcReject(pb.CommandReply_REVISION_CONFLICT, "PDC send intent changed")
	}
	m := u.Value.GetPdcProviderMessage()
	original := old.Value.GetPdcProviderMessage()
	copy := copyProvider(m)
	copy.ProviderAcceptedAt = original.ProviderAcceptedAt
	if !proto.Equal(copy, original) {
		return pdcReject(pb.CommandReply_INVALID_ARGUMENT, "PDC send body changed")
	}
	attempt := a.Workflows[intent.DerivedCommandId]
	if attempt == nil {
		return pdcReject(pb.CommandReply_INVALID_ARGUMENT, "PDC send attempt missing")
	}
	terminal := proto.Clone(intent).(*pb.WorkflowRecord)
	if m.ProviderAcceptedAt == nil {
		if attempt.Status != pb.WorkflowRecord_FAILED || r.CommandId != pdcID(u.Key, "uncertain") {
			return pdcReject(pb.CommandReply_INVALID_ARGUMENT, "PDC send uncertainty not proven")
		}
		terminal.Status, terminal.ReasonCode = pb.WorkflowRecord_FAILED, "CALL_UNCERTAIN"
		return &pb.DomainChange{Workflows: []*pb.WorkflowRecord{terminal}}, pb.CommandReply_COMMITTED, 0, nil
	}
	if r.CommandId != attempt.DerivedCommandId || attempt.Status != pb.WorkflowRecord_PENDING || terminal.Status != pb.WorkflowRecord_PENDING {
		return pdcReject(pb.CommandReply_INVALID_ARGUMENT, "PDC send result identity mismatch")
	}
	terminal.Status = pb.WorkflowRecord_COMPLETED
	out := &pb.DomainChange{Changes: []*pb.EntityChange{pdcUpsert(old, u.Key, u.Value)}, Workflows: []*pb.WorkflowRecord{terminal}}
	if m.Kind == pb.PdcProviderMessage_KIND_CLEARANCE {
		seq := a.Indexes[pb.EntityKind_PDC_SEQUENCE][m.To]
		s := seq.GetValue().GetPdcSequence()
		if s != nil && s.State == "CLEARED" && s.Sequence == m.Sequence {
			t, err := c.transition(r, a, m.To)
			if err != nil {
				return pdcReject(pb.CommandReply_UNAVAILABLE, err.Error())
			}
			t.d = out
			t.seq.Sent = true
			return t.finish()
		}
	}
	pdcSort(out)
	return out, pb.CommandReply_COMMITTED, 0, nil
}

// Candidate bindings are explicit so controller/HTTP routing uses exactly the
// same policy without starting the candidate in the SQL application.
func (c *Candidate) Execute(ctx context.Context, r *pb.CommandRequest) *pb.CommandReply {
	return c.Writer.Execute(ctx, r)
}
func (c *Candidate) Bind(next cluster.Planner) cluster.Planner {
	c.Next = next // Construction-time binding, before any worker/router starts.
	return c.Plan
}
