package cluster

import (
	"context"
	"fmt"
	"sort"
	"strings"

	pb "FlightStrips/pkg/events/cluster"
	"google.golang.org/protobuf/proto"
)

// ExternalCallWorker is the opt-in, owner-fenced entry point for airport and
// global provider calls. The production SQL workers remain wired until task 20.
// A committed intent grants exactly one attempt. A pending intent observed on
// takeover is uncertain, even when the old owner might not have sent the call.
type ExternalCallWorker struct {
	Writer Writer
	// ResultWriter reads the destination command ledger during recovery. It may
	// equal Writer when the result belongs to the source aggregate.
	ResultWriter Writer
}

type ExternalCallSpec struct {
	Source      *pb.AggregateRef
	Destination *pb.AggregateRef
	WorkflowID  string
	Step        string
	// Reserve runs after the source intent is committed and before Fetch. A
	// quota-consuming call must reserve a durable global slot here.
	Reserve func(context.Context) (bool, error)
	Fetch   func(context.Context) (proto.Message, error)
	// Commit persists the typed result using the supplied stable command ID.
	Commit func(context.Context, string, proto.Message) *pb.CommandReply
}

// Run sends a provider request only when this invocation committed a new
// intent and still owns its source. A lost PubAck, replay, or takeover never
// grants permission to send another request.
func (w ExternalCallWorker) Run(ctx context.Context, spec ExternalCallSpec) (bool, error) {
	switch w.Writer.Store.(type) {
	case NATSStore, *NATSStore:
		if w.Writer.Lease == nil {
			return false, fmt.Errorf("NATS external call requires an owner lease")
		}
	}
	if _, err := Subject(spec.Source); err != nil {
		return false, err
	}
	if _, err := Subject(spec.Destination); err != nil {
		return false, err
	}
	if !canonicalUUID(spec.WorkflowID) || strings.TrimSpace(spec.Step) == "" || spec.Fetch == nil || spec.Commit == nil {
		return false, fmt.Errorf("invalid external call specification")
	}
	stepID, err := AmanIntentID(spec.WorkflowID, spec.Step)
	if err != nil {
		return false, err
	}
	intent := &pb.WorkflowRecord{WorkflowId: spec.WorkflowID, Source: proto.Clone(spec.Source).(*pb.AggregateRef), Destination: proto.Clone(spec.Destination).(*pb.AggregateRef), Step: spec.Step, DerivedCommandId: stepID, Status: pb.WorkflowRecord_PENDING}
	written, fresh := w.advance(ctx, intent, "intent")
	if written == nil || written.Status != pb.CommandReply_COMMITTED || written.GetOutcome().GetStatus() != pb.CommandOutcome_SUCCEEDED {
		return false, fmt.Errorf("external call intent was not committed: %v", written)
	}
	if !fresh {
		return false, nil
	}
	if w.Writer.Lease != nil && !w.Writer.Lease.CanDispatchDurable(spec.Source) {
		return false, fmt.Errorf("source owner lease lost before provider call")
	}
	if spec.Reserve != nil {
		reserved, err := spec.Reserve(ctx)
		if err != nil || !reserved {
			// A consumed reservation with an uncertain acknowledgment must not
			// turn into a second request on takeover.
			_ = w.resolveOne(ctx, intent)
			return false, err
		}
	}
	if w.Writer.Lease != nil && !w.Writer.Lease.CanDispatchDurable(spec.Source) {
		return false, fmt.Errorf("source owner lease lost before provider call")
	}
	result, err := spec.Fetch(ctx)
	if err != nil || result == nil {
		_ = w.resolveOne(ctx, intent)
		return true, fmt.Errorf("provider call uncertain: %v", err)
	}
	// Provider completion is a durable prerequisite for resolving its one-shot
	// intent. Production callbacks use the ordinary writer/router entry point;
	// carry the durability requirement through that callback and any forwarding.
	reply := spec.Commit(context.WithValue(ctx, durableExecutionKey{}, true), stepID, result)
	if reply == nil || reply.Status != pb.CommandReply_COMMITTED || reply.GetOutcome().GetStatus() != pb.CommandOutcome_SUCCEEDED || reply.StreamSequence == nil {
		_ = w.resolveOne(ctx, intent)
		return true, fmt.Errorf("provider result commit uncertain: %v", reply)
	}
	completed := proto.Clone(intent).(*pb.WorkflowRecord)
	completed.Status = pb.WorkflowRecord_COMPLETED
	completed.DestinationStreamSequence = reply.StreamSequence
	ack, _ := w.advance(ctx, completed, "complete")
	if ack == nil || ack.Status != pb.CommandReply_COMMITTED || ack.GetOutcome().GetStatus() != pb.CommandOutcome_SUCCEEDED {
		return true, fmt.Errorf("provider result committed but intent completion uncertain: %v", ack)
	}
	return true, nil
}

// Resume records a proven result or marks a pending call uncertain. It never
// invokes Fetch. The stable result command ID lets a new owner prove a result
// commit that preceded the old owner's failure.
func (w ExternalCallWorker) Resume(ctx context.Context, source *pb.AggregateRef) error {
	switch w.Writer.Store.(type) {
	case NATSStore, *NATSStore:
		if w.Writer.Lease == nil {
			return fmt.Errorf("NATS external call recovery requires an owner lease")
		}
	}
	subject, err := Subject(source)
	if err != nil {
		return err
	}
	state, err := w.Writer.load(ctx, subject, source)
	if err != nil {
		return err
	}
	ids := make([]string, 0, len(state.Workflows))
	for id, record := range state.Workflows {
		if record.Status == pb.WorkflowRecord_PENDING && proto.Equal(record.Source, source) && strings.HasPrefix(record.Step, "external/") {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	for _, id := range ids {
		if err := w.resolveOne(ctx, state.Workflows[id]); err != nil {
			return err
		}
	}
	return nil
}

func (w ExternalCallWorker) resolveOne(ctx context.Context, pending *pb.WorkflowRecord) error {
	record := proto.Clone(pending).(*pb.WorkflowRecord)
	resultWriter := w.ResultWriter
	if resultWriter.Store == nil {
		resultWriter = w.Writer
	}
	destinationSubject, err := Subject(record.Destination)
	if err != nil {
		return err
	}
	var destination *Aggregate
	if resultWriter.Projection != nil {
		if ctx.Value(durableExecutionKey{}) == true && resultWriter.Lease != nil && resultWriter.Lease.CanCommitLocal(record.Destination) {
			destination, err = resultWriter.Projection.readDurableCommandCheckpoint(record.Destination, record.DerivedCommandId, true)
		} else {
			destination, err = resultWriter.Projection.commandCheckpoint(record.Destination, record.DerivedCommandId)
		}
	} else {
		destination, err = resultWriter.load(ctx, destinationSubject, record.Destination)
	}
	if err != nil {
		return err
	}
	outcome, err := destination.LookupOutcome(record.DerivedCommandId)
	if err != nil {
		return err
	}
	if outcome != nil && outcome.Status == pb.CommandOutcome_SUCCEEDED {
		record.Status = pb.WorkflowRecord_COMPLETED
		sequence := outcome.CommittedStreamSequence
		record.DestinationStreamSequence = &sequence
	} else {
		record.Status = pb.WorkflowRecord_FAILED
		record.ReasonCode = "CALL_UNCERTAIN"
	}
	ack, _ := w.advance(ctx, record, "recover")
	if ack == nil || ack.Status != pb.CommandReply_COMMITTED || ack.GetOutcome().GetStatus() != pb.CommandOutcome_SUCCEEDED {
		return fmt.Errorf("external call recovery not committed: %v", ack)
	}
	return nil
}

func (w ExternalCallWorker) advance(ctx context.Context, record *pb.WorkflowRecord, action string) (*pb.CommandReply, bool) {
	id, err := AmanIntentID(record.WorkflowId, "external-"+action)
	if err != nil {
		return nil, false
	}
	request := &pb.CommandRequest{ProtocolRevision: 1, CommandId: id, Aggregate: record.Source, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "external-worker"}, Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_AdvanceWorkflow{AdvanceWorkflow: &pb.AdvanceWorkflow{Workflow: record}}}}}
	writer := w.Writer
	writer.Plan = PlanExternalWorkflow
	return writer.ExecuteFresh(ctx, request)
}

// PlanExternalWorkflow keeps intent and terminal state on the source subject.
// The writer's subject CAS and owner epoch fence both replicas.
func PlanExternalWorkflow(_ context.Context, request *pb.CommandRequest, state *Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
	record := request.GetSystem().GetAdvanceWorkflow().GetWorkflow()
	if record == nil || !proto.Equal(record.Source, request.Aggregate) || !canonicalUUID(record.WorkflowId) || !strings.HasPrefix(record.Step, "external/") || !canonicalUUID(record.DerivedCommandId) {
		return nil, pb.CommandReply_INVALID_ARGUMENT, 0, fmt.Errorf("invalid external call workflow")
	}
	stepID, err := AmanIntentID(record.WorkflowId, record.Step)
	if err != nil || stepID != record.DerivedCommandId {
		return nil, pb.CommandReply_INVALID_ARGUMENT, 0, fmt.Errorf("external call step identity mismatch")
	}
	old, err := state.LookupWorkflow(record.WorkflowId)
	if err != nil {
		return nil, pb.CommandReply_UNAVAILABLE, 0, err
	}
	if old == nil {
		if record.Status != pb.WorkflowRecord_PENDING {
			return nil, pb.CommandReply_INVALID_ARGUMENT, 0, fmt.Errorf("external call requires pending intent")
		}
	} else {
		if old.Status != pb.WorkflowRecord_PENDING || !proto.Equal(old.Source, record.Source) || !proto.Equal(old.Destination, record.Destination) || old.Step != record.Step || old.DerivedCommandId != record.DerivedCommandId {
			return nil, pb.CommandReply_INVALID_ARGUMENT, 0, fmt.Errorf("external call intent changed")
		}
		if record.Status != pb.WorkflowRecord_COMPLETED && (record.Status != pb.WorkflowRecord_FAILED || record.ReasonCode != "CALL_UNCERTAIN") {
			return nil, pb.CommandReply_INVALID_ARGUMENT, 0, fmt.Errorf("invalid external call outcome")
		}
		if record.Status == pb.WorkflowRecord_COMPLETED && (record.DestinationStreamSequence == nil || *record.DestinationStreamSequence == 0) {
			return nil, pb.CommandReply_INVALID_ARGUMENT, 0, fmt.Errorf("missing committed result")
		}
	}
	return &pb.DomainChange{Workflows: []*pb.WorkflowRecord{proto.Clone(record).(*pb.WorkflowRecord)}}, pb.CommandReply_COMMITTED, 0, nil
}
