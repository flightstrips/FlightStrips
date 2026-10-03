package cluster

import (
	"context"
	"fmt"
	"sort"
	"strings"

	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
)

// AmanTransition is the complete, already evaluated result of one AMAN action.
// The operational owner supplies typed values; the adapter validates and CAS
// commits them as one airport event. The current SQL service does not use it.
type AmanTransition struct {
	Request *pb.CommandRequest
	Airport *pb.AmanAirport
	// Flights is the complete active board. Missing flights are deleted.
	Flights []*pb.AmanFlight
	// The remaining slices contain only newly accepted or changed records.
	Coordinations []*pb.AmanCoordination
	Audits        []*pb.AmanAudit
	Validations   []*pb.AmanValidation
	Observations  []*pb.VatsimObservation
	Workflows     []*pb.WorkflowRecord
}

type AmanAdapter struct{ Writer Writer }

func airportRef(icao string) *pb.AggregateRef {
	return &pb.AggregateRef{Target: &pb.AggregateRef_Airport{Airport: &pb.AirportRef{Icao: icao}}}
}

// Commit retains Writer's durable command ledger, owner fencing, CAS retry,
// and projection barrier. A domain command's request is the idempotency input;
// callers must use the same UUID and typed action when retrying that action.
func (a AmanAdapter) Commit(ctx context.Context, transition AmanTransition) *pb.CommandReply {
	w := a.Writer
	w.Plan = func(_ context.Context, request *pb.CommandRequest, state *Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
		return planAman(request, state, transition)
	}
	return w.Execute(ctx, transition.Request)
}

// PlanAmanTransition applies the same board validation for the application
// command chain; the caller must evaluate actual operational policy on state.
func PlanAmanTransition(request *pb.CommandRequest, state *Aggregate, transition AmanTransition) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
	return planAman(request, state, transition)
}

func planAman(request *pb.CommandRequest, state *Aggregate, t AmanTransition) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
	if request.GetAggregate().GetAirport() == nil || t.Airport == nil || t.Airport.Airport != request.GetAggregate().GetAirport().Icao {
		return nil, pb.CommandReply_INVALID_ARGUMENT, 0, fmt.Errorf("AMAN transition airport mismatch")
	}
	// Bind the evaluated result to an actual typed action. The action is what
	// the command ledger hashes; a synthetic envelope must not smuggle a board.
	switch {
	case request.GetClient().GetAman() != nil:
	case request.GetSystem().GetReportAmanRouteFact() != nil:
	case request.GetSystem().GetUpdateEntity() != nil:
		update := request.GetSystem().GetUpdateEntity()
		matched := update.Key == t.Airport.Airport && proto.Equal(update.Value.GetAmanAirport(), t.Airport)
		if !matched {
			for _, observation := range t.Observations {
				if observation != nil && update.Key == observation.ProviderId && proto.Equal(update.Value.GetVatsimObservation(), observation) {
					id, err := AmanObservationCommandID("vatsim", observation.ProviderId)
					matched = err == nil && request.CommandId == id
					break
				}
			}
		}
		if !matched {
			return nil, pb.CommandReply_INVALID_ARGUMENT, 0, fmt.Errorf("AMAN system update does not match typed state or observation")
		}
	default:
		return nil, pb.CommandReply_INVALID_ARGUMENT, 0, fmt.Errorf("AMAN transition requires a typed AMAN action")
	}
	oldAirport := state.Indexes[pb.EntityKind_AMAN_AIRPORT][t.Airport.Airport]
	current := uint64(0)
	if oldAirport != nil {
		current = oldAirport.GetValue().GetAmanAirport().Revision
	}
	if request.ExpectedEntityRevision == nil || *request.ExpectedEntityRevision != current {
		return nil, pb.CommandReply_REVISION_CONFLICT, current, fmt.Errorf("stale AMAN airport revision")
	}
	if t.Airport.Revision != current+1 {
		return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("AMAN airport revision must increase once")
	}
	if err := validateAmanBoard(t); err != nil {
		return nil, pb.CommandReply_INVALID_ARGUMENT, current, err
	}
	change := &pb.DomainChange{}
	seen := map[string]bool{}
	add := func(kind pb.EntityKind, key string, record *pb.EntityRecord, appendOnly bool) error {
		if key == "" || seen[fmt.Sprintf("%d/%s", kind, key)] {
			return fmt.Errorf("duplicate or empty AMAN entity key")
		}
		seen[fmt.Sprintf("%d/%s", kind, key)] = true
		old := state.Indexes[kind][key]
		if appendOnly && old != nil {
			return fmt.Errorf("append-only AMAN identity already exists")
		}
		if old != nil && proto.Equal(old.Value, record) {
			return nil
		}
		revision := uint64(1)
		if old != nil {
			revision = old.Revision + 1
		}
		change.Changes = append(change.Changes, &pb.EntityChange{Key: key, Revision: revision, Operation: &pb.EntityChange_Upsert{Upsert: proto.Clone(record).(*pb.EntityRecord)}})
		return nil
	}
	if err := add(pb.EntityKind_AMAN_AIRPORT, t.Airport.Airport, &pb.EntityRecord{Value: &pb.EntityRecord_AmanAirport{AmanAirport: t.Airport}}, false); err != nil {
		return nil, pb.CommandReply_INVALID_ARGUMENT, current, err
	}
	active := map[string]bool{}
	for _, flight := range t.Flights {
		if flight == nil || flight.Callsign != strings.ToUpper(strings.TrimSpace(flight.Callsign)) || flight.Callsign == "" || active[flight.Callsign] {
			return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("invalid or duplicate AMAN callsign")
		}
		active[flight.Callsign] = true
		if err := add(pb.EntityKind_AMAN_FLIGHT, flight.Callsign, &pb.EntityRecord{Value: &pb.EntityRecord_AmanFlight{AmanFlight: flight}}, false); err != nil {
			return nil, pb.CommandReply_INVALID_ARGUMENT, current, err
		}
	}
	// A complete board replacement removes flights absent from the new board.
	for _, old := range state.EntitiesByKind(pb.EntityKind_AMAN_FLIGHT) {
		if !active[old.Key] {
			change.Changes = append(change.Changes, &pb.EntityChange{Key: old.Key, Revision: old.Revision + 1, Operation: &pb.EntityChange_Delete{Delete: &pb.DeleteEntity{Kind: pb.EntityKind_AMAN_FLIGHT}}})
		}
	}
	for _, item := range t.Coordinations {
		if item == nil {
			return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("nil AMAN coordination")
		}
		if err := add(pb.EntityKind_AMAN_COORDINATION, item.Id, &pb.EntityRecord{Value: &pb.EntityRecord_AmanCoordination{AmanCoordination: item}}, false); err != nil {
			return nil, pb.CommandReply_INVALID_ARGUMENT, current, err
		}
	}
	for _, item := range t.Audits {
		if item == nil || item.AirportRevision != t.Airport.Revision || item.GetFact() == nil || !proto.Equal(item.Actor, request.Actor) {
			return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("invalid AMAN audit fact or revision")
		}
		if err := add(pb.EntityKind_AMAN_AUDIT, item.Id, &pb.EntityRecord{Value: &pb.EntityRecord_AmanAudit{AmanAudit: item}}, true); err != nil {
			return nil, pb.CommandReply_INVALID_ARGUMENT, current, err
		}
	}
	for _, item := range t.Validations {
		if item == nil {
			return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("nil AMAN validation")
		}
		if err := add(pb.EntityKind_AMAN_VALIDATION, item.Id, &pb.EntityRecord{Value: &pb.EntityRecord_AmanValidation{AmanValidation: item}}, true); err != nil {
			return nil, pb.CommandReply_INVALID_ARGUMENT, current, err
		}
	}
	for _, item := range t.Observations {
		if item == nil {
			return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("nil VATSIM observation")
		}
		if err := add(pb.EntityKind_VATSIM_OBSERVATION, item.ProviderId, &pb.EntityRecord{Value: &pb.EntityRecord_VatsimObservation{VatsimObservation: item}}, true); err != nil {
			return nil, pb.CommandReply_INVALID_ARGUMENT, current, err
		}
	}
	workflowIDs := map[string]bool{}
	for _, workflow := range t.Workflows {
		if workflow == nil || !proto.Equal(workflow.Source, request.Aggregate) || workflow.Destination.GetSession() == nil || workflow.SourceRevision == nil || *workflow.SourceRevision != t.Airport.Revision || !canonicalUUID(workflow.WorkflowId) || !canonicalUUID(workflow.DerivedCommandId) || workflow.Status != pb.WorkflowRecord_PENDING || workflowIDs[workflow.WorkflowId] {
			return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("invalid AMAN intent source or identity")
		}
		workflowIDs[workflow.WorkflowId] = true
		derived, err := AmanIntentID(workflow.WorkflowId, workflow.Step)
		if err != nil || workflow.DerivedCommandId != derived {
			return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("AMAN intent step ID mismatch")
		}
		prior, err := state.LookupWorkflow(workflow.WorkflowId)
		if err != nil {
			return nil, pb.CommandReply_UNAVAILABLE, current, err
		}
		if prior != nil {
			return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("AMAN intent already exists")
		}
		change.Workflows = append(change.Workflows, proto.Clone(workflow).(*pb.WorkflowRecord))
	}
	sort.Slice(change.Changes, func(i, j int) bool {
		iKind, _ := changeKind(change.Changes[i])
		jKind, _ := changeKind(change.Changes[j])
		if iKind != jKind {
			return iKind < jKind
		}
		return change.Changes[i].Key < change.Changes[j].Key
	})
	return change, pb.CommandReply_COMMITTED, current, nil
}

func validateAmanBoard(t AmanTransition) error {
	if t.Airport.PolicyVersion == "" || t.Airport.GeneratedAt == nil || t.Airport.GeneratedAt.CheckValid() != nil {
		return fmt.Errorf("incomplete AMAN airport state")
	}
	groups := map[string]bool{}
	selected := ""
	for _, group := range t.Airport.RunwayGroups {
		if group == nil || group.Id == "" || groups[group.Id] {
			return fmt.Errorf("duplicate or missing AMAN runway group")
		}
		groups[group.Id] = true
		if group.Selected {
			if selected != "" {
				return fmt.Errorf("multiple selected AMAN runway groups")
			}
			selected = group.Id
		}
		if group.RateEffectiveAt != nil && group.ActiveRatePerHour == 0 {
			return fmt.Errorf("AMAN active rate is zero")
		}
		for i, rate := range group.RateSchedule {
			if rate == nil || rate.ArrivalsPerHour == 0 || rate.EffectiveAt == nil || (i > 0 && !rate.EffectiveAt.AsTime().After(group.RateSchedule[i-1].EffectiveAt.AsTime())) {
				return fmt.Errorf("invalid AMAN rate schedule")
			}
		}
		for i, gap := range group.Gaps {
			if gap == nil || gap.Id == "" || gap.Start == nil || gap.End == nil || !gap.Start.AsTime().Before(gap.End.AsTime()) || (i > 0 && !group.Gaps[i-1].End.AsTime().Before(gap.Start.AsTime())) {
				return fmt.Errorf("invalid or overlapping AMAN gaps")
			}
		}
	}
	if len(t.Airport.ActiveRunwayGroupIds) > 0 {
		if selected == "" {
			return fmt.Errorf("active AMAN runways require a selected group")
		}
		active := map[string]bool{}
		for _, id := range t.Airport.ActiveRunwayGroupIds {
			if !groups[id] || active[id] {
				return fmt.Errorf("invalid active AMAN runway group")
			}
			active[id] = true
		}
		if !active[selected] {
			return fmt.Errorf("selected AMAN runway group is inactive")
		}
	}
	for _, flight := range t.Flights {
		if flight == nil || flight.UpdatedAt == nil || flight.UpdatedAt.CheckValid() != nil {
			return fmt.Errorf("incomplete AMAN flight")
		}
		if observation := flight.LatestObservation; observation != nil && (observation.Callsign != flight.Callsign || observation.ReconciledAt == nil || observation.SourceStatus == "") {
			return fmt.Errorf("AMAN flight observation identity or provenance mismatch")
		}
		if slot := flight.Slot; slot != nil && (slot.Time == nil || slot.RunwayGroupId == "" || slot.Sequence == 0 || slot.Revision != t.Airport.Revision || !groups[slot.RunwayGroupId]) {
			return fmt.Errorf("AMAN flight slot is invalid")
		}
		if flight.FreezeReason != "none" && (flight.FrozenAt == nil || flight.FrozenOperationalTeta == nil) {
			return fmt.Errorf("AMAN frozen flight lacks capture")
		}
		if flight.FreezeReason == "none" && (flight.FrozenAt != nil || flight.FrozenOperationalTeta != nil || flight.FrozenSlot != nil) {
			return fmt.Errorf("unfrozen AMAN flight retains capture")
		}
		for _, offer := range flight.QueueOffers {
			if offer == nil || offer.Callsign != flight.Callsign || offer.AirportRevision != t.Airport.Revision || offer.CandidateSlot == nil || offer.CandidateSlot.Revision != t.Airport.Revision || offer.ExpiresAt == nil || !offer.ExpiresAt.AsTime().After(t.Airport.GeneratedAt.AsTime()) {
				return fmt.Errorf("AMAN queue offer revision or expiry is invalid")
			}
		}
	}
	return nil
}

// AmanBoard is a coherent, typed read model from one airport checkpoint.
type AmanBoard struct {
	Revision      uint64
	Airport       *pb.AmanAirport
	Flights       []*pb.AmanFlight
	Coordinations []*pb.AmanCoordination
	Audits        []*pb.AmanAudit
	Validations   []*pb.AmanValidation
	Observations  []*pb.VatsimObservation
}

func ReadAmanBoard(state *Aggregate) (AmanBoard, error) {
	if state == nil || state.Ref.GetAirport() == nil {
		return AmanBoard{}, fmt.Errorf("AMAN read requires airport projection")
	}
	board := AmanBoard{Revision: state.Revision}
	for _, item := range state.EntitiesByKind(pb.EntityKind_AMAN_AIRPORT) {
		board.Airport = item.GetValue().GetAmanAirport()
	}
	for _, item := range state.EntitiesByKind(pb.EntityKind_AMAN_FLIGHT) {
		board.Flights = append(board.Flights, item.GetValue().GetAmanFlight())
	}
	for _, item := range state.EntitiesByKind(pb.EntityKind_AMAN_COORDINATION) {
		board.Coordinations = append(board.Coordinations, item.GetValue().GetAmanCoordination())
	}
	for _, item := range state.EntitiesByKind(pb.EntityKind_AMAN_AUDIT) {
		board.Audits = append(board.Audits, item.GetValue().GetAmanAudit())
	}
	for _, item := range state.EntitiesByKind(pb.EntityKind_AMAN_VALIDATION) {
		board.Validations = append(board.Validations, item.GetValue().GetAmanValidation())
	}
	for _, item := range state.EntitiesByKind(pb.EntityKind_VATSIM_OBSERVATION) {
		board.Observations = append(board.Observations, item.GetValue().GetVatsimObservation())
	}
	sort.Slice(board.Audits, func(i, j int) bool {
		if board.Audits[i].AirportRevision != board.Audits[j].AirportRevision {
			return board.Audits[i].AirportRevision < board.Audits[j].AirportRevision
		}
		return board.Audits[i].Id < board.Audits[j].Id
	})
	return board, nil
}

func (a AmanAdapter) Read(ctx context.Context, icao string) (AmanBoard, error) {
	ref := airportRef(icao)
	subject, err := Subject(ref)
	if err != nil {
		return AmanBoard{}, err
	}
	var state *Aggregate
	if a.Writer.Projection != nil {
		state, err = a.Writer.Projection.ReadEntityKinds(ref, pb.EntityKind_AMAN_AIRPORT, pb.EntityKind_AMAN_FLIGHT, pb.EntityKind_AMAN_COORDINATION, pb.EntityKind_AMAN_AUDIT, pb.EntityKind_AMAN_VALIDATION, pb.EntityKind_VATSIM_OBSERVATION)
	} else {
		state, err = a.Writer.load(ctx, subject, ref)
	}
	if err != nil {
		return AmanBoard{}, err
	}
	return ReadAmanBoard(state)
}

// AmanIntentID derives the destination command identity from the workflow ID
// and step so a resumed worker cannot apply the session mutation twice.
func AmanIntentID(workflowID, step string) (string, error) {
	if !canonicalUUID(workflowID) || strings.TrimSpace(step) == "" {
		return "", fmt.Errorf("invalid AMAN intent identity")
	}
	namespace := uuid.MustParse(workflowID)
	return uuid.NewSHA1(namespace, []byte(step)).String(), nil
}

// AmanObservationCommandID gives retries of one provider observation the same
// durable identity even when the provider resends it after a process restart.
func AmanObservationCommandID(provider, observationID string) (string, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	observationID = strings.TrimSpace(observationID)
	if provider == "" || observationID == "" {
		return "", fmt.Errorf("missing AMAN observation identity")
	}
	return ProviderEventCommandID("aman-observation", provider, observationID)
}

// AmanVatsimObservationRequest binds a provider identity to one typed source
// fact. The operational evaluator supplies the resulting complete board.
func AmanVatsimObservationRequest(icao string, observation *pb.VatsimObservation, expectedRevision uint64) (*pb.CommandRequest, error) {
	if observation == nil || observation.ProviderId == "" {
		return nil, fmt.Errorf("missing VATSIM observation")
	}
	ref := airportRef(icao)
	if _, err := Subject(ref); err != nil {
		return nil, err
	}
	id, err := AmanObservationCommandID("vatsim", observation.ProviderId)
	if err != nil {
		return nil, err
	}
	return &pb.CommandRequest{ProtocolRevision: 1, CommandId: id, Aggregate: ref, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "vatsim-adapter"}, ExpectedEntityRevision: &expectedRevision, Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: &pb.UpdateEntity{Key: observation.ProviderId, Value: &pb.EntityRecord{Value: &pb.EntityRecord_VatsimObservation{VatsimObservation: proto.Clone(observation).(*pb.VatsimObservation)}}}}}}}, nil
}

// PlanAmanWorkflow advances only a pending intent on the airport owner. A
// source revision change supersedes work that has not committed. An earlier
// destination commit remains recoverable after a newer airport transition.
func PlanAmanWorkflow(_ context.Context, request *pb.CommandRequest, state *Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
	want := request.GetSystem().GetAdvanceWorkflow().GetWorkflow()
	if request.GetAggregate().GetAirport() == nil || want == nil || !proto.Equal(want.Source, request.Aggregate) {
		return nil, pb.CommandReply_INVALID_ARGUMENT, 0, fmt.Errorf("invalid AMAN workflow target")
	}
	old, err := state.LookupWorkflow(want.WorkflowId)
	if err != nil {
		return nil, pb.CommandReply_UNAVAILABLE, 0, err
	}
	if old == nil || old.Status != pb.WorkflowRecord_PENDING || old.DerivedCommandId != want.DerivedCommandId || old.SourceRevision == nil || want.SourceRevision == nil || *old.SourceRevision != *want.SourceRevision || !proto.Equal(old.Destination, want.Destination) {
		return nil, pb.CommandReply_INVALID_ARGUMENT, 0, fmt.Errorf("AMAN workflow is not pending")
	}
	if want.Status != pb.WorkflowRecord_COMPLETED && want.Status != pb.WorkflowRecord_SUPERSEDED {
		return nil, pb.CommandReply_INVALID_ARGUMENT, 0, fmt.Errorf("invalid AMAN workflow transition")
	}
	current := uint64(0)
	if airport := state.Indexes[pb.EntityKind_AMAN_AIRPORT][request.Aggregate.GetAirport().Icao]; airport != nil {
		current = airport.GetValue().GetAmanAirport().Revision
	}
	copy := proto.Clone(old).(*pb.WorkflowRecord)
	if want.Status == pb.WorkflowRecord_COMPLETED && want.DestinationStreamSequence != nil && *want.DestinationStreamSequence > 0 {
		copy.Status = pb.WorkflowRecord_COMPLETED
		copy.DestinationStreamSequence = want.DestinationStreamSequence
	} else if current != *old.SourceRevision {
		copy.Status = pb.WorkflowRecord_SUPERSEDED
		copy.ReasonCode = "SOURCE_REVISION_CHANGED"
		copy.DestinationStreamSequence = nil
	} else {
		return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("completed AMAN workflow requires destination commit")
	}
	return &pb.DomainChange{Workflows: []*pb.WorkflowRecord{copy}}, pb.CommandReply_COMMITTED, current, nil
}

type AmanIntentRunner struct {
	AirportWriter Writer
	Destination   LifecycleStore
	// Step constructs a typed destination command for the named intent. It must
	// use the recorded derived command ID and carry the source revision.
	Step func(*pb.WorkflowRecord) (*pb.CommandRequest, error)
}

// Resume reads the durable airport projection after takeover and retries each
// pending step. A destination retry uses exactly the recorded UUID.
func (r AmanIntentRunner) Resume(ctx context.Context, icao string) error {
	ref := airportRef(icao)
	if r.AirportWriter.Lease != nil && !r.AirportWriter.Lease.CanWrite(ref) {
		return fmt.Errorf("AMAN intent runner is not airport owner")
	}
	subject, err := Subject(ref)
	if err != nil {
		return err
	}
	state, err := r.AirportWriter.load(ctx, subject, ref)
	if err != nil {
		return err
	}
	ids := make([]string, 0, len(state.Workflows))
	for id, workflow := range state.Workflows {
		if workflow.Status == pb.WorkflowRecord_PENDING && proto.Equal(workflow.Source, ref) {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	for _, id := range ids {
		if r.AirportWriter.Lease != nil && !r.AirportWriter.Lease.CanWrite(ref) {
			return fmt.Errorf("AMAN intent runner lost airport ownership")
		}
		workflow := state.Workflows[id]
		currentState, err := r.AirportWriter.load(ctx, subject, ref)
		if err != nil {
			return err
		}
		current := uint64(0)
		if airport := currentState.Indexes[pb.EntityKind_AMAN_AIRPORT][icao]; airport != nil {
			current = airport.GetValue().GetAmanAirport().Revision
		}
		advance := proto.Clone(workflow).(*pb.WorkflowRecord)
		advance.Status = pb.WorkflowRecord_SUPERSEDED
		// Consult the destination ledger before source supersession. Its durable
		// result proves a lost reply even when the airport has since advanced.
		if r.Destination == nil {
			return fmt.Errorf("AMAN destination adapter unavailable")
		}
		destination, err := r.Destination.Read(ctx, workflow.Destination)
		if err != nil {
			return err
		}
		outcome, err := destination.LookupOutcome(workflow.DerivedCommandId)
		if err != nil {
			return err
		}
		if outcome != nil && (outcome.Status == pb.CommandOutcome_SUCCEEDED || outcome.Status == pb.CommandOutcome_ACCEPTED || destination.Effects[workflow.DerivedCommandId] != nil) {
			if outcome.Actor.GetKind() != pb.Actor_SYSTEM || outcome.Actor.Id != "aman-intent" || !proto.Equal(outcome.Aggregate, workflow.Destination) || outcome.CommittedStreamSequence == 0 {
				return fmt.Errorf("AMAN destination outcome identity mismatch")
			}
			advance.Status = pb.WorkflowRecord_COMPLETED
			sequence := outcome.CommittedStreamSequence
			advance.DestinationStreamSequence = &sequence
		} else if workflow.SourceRevision != nil && current == *workflow.SourceRevision {
			if r.Destination == nil || r.Step == nil {
				return fmt.Errorf("AMAN destination adapter unavailable")
			}
			step, err := r.Step(proto.Clone(workflow).(*pb.WorkflowRecord))
			if err != nil {
				return err
			}
			if step == nil || step.CommandId != workflow.DerivedCommandId || !proto.Equal(step.Aggregate, workflow.Destination) {
				return fmt.Errorf("AMAN destination command identity mismatch")
			}
			reply := r.Destination.Execute(ctx, step)
			if reply != nil && (reply.Status == pb.CommandReply_REVISION_CONFLICT || reply.GetOutcome().GetStatus() == pb.CommandOutcome_FAILED && reply.GetOutcome().ReasonCode == pb.CommandReply_REVISION_CONFLICT.String()) {
				latest, err := r.AirportWriter.load(ctx, subject, ref)
				if err != nil {
					return err
				}
				airport := latest.Indexes[pb.EntityKind_AMAN_AIRPORT][icao]
				if airport == nil || airport.GetValue().GetAmanAirport().Revision == *workflow.SourceRevision {
					return fmt.Errorf("AMAN destination conflict without source supersession")
				}
			} else if reply == nil || (reply.Status != pb.CommandReply_COMMITTED && reply.Status != pb.CommandReply_PENDING) || reply.Outcome == nil || (reply.Outcome.Status != pb.CommandOutcome_SUCCEEDED && reply.Outcome.Status != pb.CommandOutcome_ACCEPTED) || reply.StreamSequence == nil {
				return fmt.Errorf("AMAN destination step %s is not confirmed", id)
			} else {
				advance.Status = pb.WorkflowRecord_COMPLETED
				advance.DestinationStreamSequence = reply.StreamSequence
			}
		}
		commandID, err := AmanIntentID(id, "record-"+advance.Status.String())
		if err != nil {
			return err
		}
		request := &pb.CommandRequest{ProtocolRevision: 1, CommandId: commandID, Aggregate: ref, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "aman-intent"}, Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_AdvanceWorkflow{AdvanceWorkflow: &pb.AdvanceWorkflow{Workflow: advance}}}}}
		writer := r.AirportWriter
		writer.Plan = PlanAmanWorkflow
		reply := writer.Execute(ctx, request)
		if reply == nil || reply.Status != pb.CommandReply_COMMITTED || reply.Outcome == nil || reply.Outcome.Status != pb.CommandOutcome_SUCCEEDED {
			return fmt.Errorf("AMAN intent %s completion is not confirmed", id)
		}
	}
	return nil
}
