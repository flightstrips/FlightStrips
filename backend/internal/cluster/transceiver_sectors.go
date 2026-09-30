package cluster

import (
	"context"
	"fmt"
	"strings"

	pb "FlightStrips/pkg/events/cluster"
)

const transceiverSectorStep = "transceiver/sectors/"

// TransceiverSectorPlanner is Task18c's source port. The session owner receives
// a coherent immutable GetFrequencies lookup and its accepted source revision.
// Task20 binds the concrete sector/layout/route planner here. The returned
// domain diff and revision acknowledgment commit together under the owner CAS.
type TransceiverSectorPlanner func(context.Context, *pb.CommandRequest, *Aggregate, TransceiverGeneration) (*pb.DomainChange, pb.CommandReply_Status, uint64, error)

type TransceiverSectorReconciler struct {
	source *TransceiverSource
	writer Writer
	plan   TransceiverSectorPlanner
}

func NewTransceiverSectorReconciler(source *TransceiverSource, writer Writer, plan TransceiverSectorPlanner) (*TransceiverSectorReconciler, error) {
	if source == nil || writer.Store == nil || plan == nil {
		return nil, fmt.Errorf("transceiver reconciliation requires source, session writer and sector planner")
	}
	if err := transceiverNATSWriter(writer, true); err != nil {
		return nil, err
	}
	return &TransceiverSectorReconciler{source: source, writer: writer, plan: plan}, nil
}

// AppliedRevision is durable session state. Comparing it with Generation on
// each session-owner pass recovers missed notifications and owner death, even
// if the global checkpoint was committed before any local callback ran.
func (r *TransceiverSectorReconciler) AppliedRevision(ctx context.Context, sessionID int32) (uint64, error) {
	state, err := (NavigationWeather{Writer: r.writer}).read(ctx, sessionRef(sessionID))
	if err != nil {
		return 0, err
	}
	return transceiverAppliedRevision(state), nil
}

func transceiverAppliedRevision(state *Aggregate) uint64 {
	var revision uint64
	for _, workflow := range state.Workflows {
		if strings.HasPrefix(workflow.Step, transceiverSectorStep) && workflow.Status == pb.WorkflowRecord_COMPLETED && workflow.GetSourceRevision() > revision {
			revision = workflow.GetSourceRevision()
		}
	}
	return revision
}

// Reconcile is scheduled on session owners, independently of provider polling.
// A failure leaves the accepted source revision pending for a later pass.
func (r *TransceiverSectorReconciler) Reconcile(ctx context.Context, sessionID int32) *pb.CommandReply {
	if sessionID <= 0 {
		return &pb.CommandReply{Status: pb.CommandReply_INVALID_ARGUMENT}
	}
	generation, err := r.source.Generation(ctx)
	if err != nil {
		return &pb.CommandReply{Status: pb.CommandReply_UNAVAILABLE, Detail: err.Error()}
	}
	id, _ := ProviderEventCommandID("transceiver-sectors", "vatsim", fmt.Sprintf("%d/%d/%s", sessionID, generation.Revision, generation.Sha256))
	ref := sessionRef(sessionID)
	step := transceiverSectorStep + generation.Sha256
	derived, _ := AmanIntentID(id, step)
	workflow := &pb.WorkflowRecord{WorkflowId: id, Source: ref, Destination: ref, Step: step, DerivedCommandId: derived, Status: pb.WorkflowRecord_COMPLETED, SourceRevision: &generation.Revision}
	request := &pb.CommandRequest{ProtocolRevision: 1, CommandId: derived, Aggregate: ref, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "transceiver-sectors"}, Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_AdvanceWorkflow{AdvanceWorkflow: &pb.AdvanceWorkflow{Workflow: workflow}}}}}
	writer := r.writer
	writer.Plan = func(ctx context.Context, req *pb.CommandRequest, state *Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
		latest, err := r.source.Generation(ctx)
		if err != nil {
			return nil, pb.CommandReply_UNAVAILABLE, 0, err
		}
		if latest.Revision != generation.Revision || latest.Sha256 != generation.Sha256 || transceiverAppliedRevision(state) > generation.Revision {
			return nil, pb.CommandReply_REVISION_CONFLICT, 0, fmt.Errorf("transceiver generation superseded")
		}
		if transceiverAppliedRevision(state) == generation.Revision {
			return &pb.DomainChange{}, pb.CommandReply_COMMITTED, 0, nil
		}
		session := state.Indexes[pb.EntityKind_SESSION][fmt.Sprint(sessionID)].GetValue().GetSession()
		if session == nil || session.Tombstoned {
			return nil, pb.CommandReply_UNAVAILABLE, 0, fmt.Errorf("live session unavailable")
		}
		change, status, revision, err := r.plan(ctx, req, state, generation)
		if err != nil || status != pb.CommandReply_COMMITTED || change == nil {
			// The source handoff remains pending. Do not ledger-cache a policy
			// failure under the generation UUID and prevent a later healthy pass.
			if err == nil {
				err = fmt.Errorf("sector reconciliation not accepted: %s", status)
			}
			return nil, pb.CommandReply_UNAVAILABLE, revision, err
		}
		change.Workflows = append(change.Workflows, workflow)
		return change, status, revision, nil
	}
	return writer.Execute(ctx, request)
}
