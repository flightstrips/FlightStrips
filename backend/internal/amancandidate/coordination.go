package amancandidate

import (
	"FlightStrips/internal/aman"
	"FlightStrips/internal/cluster"
	cr "FlightStrips/internal/coordinationrequest"
	pb "FlightStrips/pkg/events/cluster"
	"context"
	"errors"
	"fmt"
	"google.golang.org/protobuf/proto"
)

// The coordinator service retains authorization and request policy. This
// detached repository collects its result for one atomic airport CAS event.
type coordinationCapture struct {
	board    cluster.AmanBoard
	airport  string
	revision uint64
	changed  []cr.Request
	owners   map[string]cr.ControllerID
}

func (r *coordinationCapture) TrackingController(_ context.Context, _ string, callsign cr.Callsign) (cr.ControllerID, error) {
	return r.owners[string(callsign)], nil
}
func (r *coordinationCapture) Get(_ context.Context, airport string, id cr.RequestID) (cr.Request, error) {
	for _, value := range r.board.Coordinations {
		if value.Id == string(id) && airport == r.airport {
			return decodeCoordination(value, airport)
		}
	}
	return cr.Request{}, cr.ErrRequestNotFound
}
func (r *coordinationCapture) Submit(_ context.Context, request cr.Request, expected uint64) (cr.CommitResult, error) {
	if expected != r.revision {
		return cr.CommitResult{}, cr.ErrRevisionConflict
	}
	if err := request.Validate(); err != nil {
		return cr.CommitResult{}, err
	}
	result := cr.CommitResult{Request: request, Revision: expected + 1}
	for _, value := range r.board.Coordinations {
		if value.Id == string(request.ID) {
			return cr.CommitResult{}, cr.ErrCommandConflict
		}
		previous, err := decodeCoordination(value, r.airport)
		if err != nil {
			return cr.CommitResult{}, err
		}
		if previous.State == cr.StatePending && previous.Callsign == request.Callsign && previous.Kind == request.Kind {
			previous, err = previous.Supersede(request.ID, request.CreatedAt)
			if err != nil {
				return cr.CommitResult{}, err
			}
			result.SupersededRequest = &previous
			r.changed = append(r.changed, previous)
			id := previous.ID
			request.Supersedes = &id
		}
	}
	if err := request.Validate(); err != nil {
		return cr.CommitResult{}, err
	}
	result.Request = request
	r.changed = append(r.changed, request)
	return result, nil
}
func (r *coordinationCapture) Decide(ctx context.Context, id cr.RequestID, decision cr.Decision, expected uint64) (cr.CommitResult, error) {
	if expected != r.revision {
		return cr.CommitResult{}, cr.ErrRevisionConflict
	}
	request, err := r.Get(ctx, r.airport, id)
	if err != nil {
		return cr.CommitResult{}, err
	}
	request, err = request.Decide(decision.CommandID, decision.Actor, decision.Role, decision.AuthoritativeRecipient, decision.AfterState, decision.Reason, decision.ReceivedAt)
	if err != nil {
		return cr.CommitResult{}, err
	}
	r.changed = append(r.changed, request)
	return cr.CommitResult{Request: request, Revision: expected + 1}, nil
}
func (r *coordinationCapture) TransferPending(_ context.Context, fact cr.OwnershipFact) (cr.TransferResult, error) {
	result := cr.TransferResult{Revision: r.revision}
	for _, value := range r.board.Coordinations {
		request, err := decodeCoordination(value, r.airport)
		if err != nil {
			return result, err
		}
		if request.State != cr.StatePending || request.Callsign != fact.Callsign || request.RecipientController == fact.Owner {
			continue
		}
		request, err = request.TransferRecipient(fact.FactID, fact.Revision, fact.Owner, fact.ObservedAt)
		if err != nil {
			return result, err
		}
		r.changed = append(r.changed, request)
		result.Requests = append(result.Requests, request)
	}
	if len(r.changed) > 0 {
		result.Revision++
	}
	return result, nil
}

func (w *Worker) planCoordination(ctx context.Context, req *pb.CommandRequest, state *cluster.Aggregate, board cluster.AmanBoard, role string) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
	airport := req.Aggregate.GetAirport().Icao
	at := w.options.Now().UTC()
	current := board.Airport.Revision
	inputs, err := w.acceptedInputs(ctx, airport, board, at, nil)
	if err != nil {
		return nil, pb.CommandReply_UNAVAILABLE, current, err
	}
	capture := &coordinationCapture{board: board, airport: airport, revision: current, owners: map[string]cr.ControllerID{}}
	for _, a := range inputs.sessions {
		for _, entity := range a.EntitiesByKind(pb.EntityKind_STRIP) {
			s := entity.Value.GetStrip()
			if s.TrackingController != "" {
				owner := cr.ControllerID(s.TrackingController)
				if prior := capture.owners[s.Callsign]; prior != "" && prior != owner {
					return nil, pb.CommandReply_UNAVAILABLE, current, fmt.Errorf("ambiguous tracking controller")
				}
				capture.owners[s.Callsign] = owner
			}
		}
	}
	service := cr.NewService(capture, capture)
	auth := cr.CommandContext{Airport: airport, Actor: req.Actor.Id, Role: role, ReceivedAt: at}
	action := req.GetClient().GetAman()
	switch {
	case action.GetSubmitCoordinationRequest() != nil:
		v := action.GetSubmitCoordinationRequest()
		if v.CoordinationRequestId != string(cr.IDForCommand(req.CommandId)) {
			return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("coordination identity must derive from command ID")
		}
		command := cr.SubmitCommand{CommandID: req.CommandId, ExpectedRevision: current, Callsign: cr.Callsign(v.Callsign)}
		switch p := v.Request.(type) {
		case *pb.AmanSubmitCoordination_RouteDirect:
			command.Kind = cr.KindRouteDirect
			command.Payload.RouteDirect = &cr.RouteDirectPayload{Route: p.RouteDirect.GetRoute(), DirectTo: p.RouteDirect.GetDirectTo()}
		case *pb.AmanSubmitCoordination_Speed:
			command.Kind = cr.KindSpeed
			command.Payload.Speed = &cr.SpeedPayload{Requested: p.Speed.Requested}
		default:
			return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("missing coordination payload")
		}
		_, err = service.Submit(ctx, auth, command)
	case action.GetAcceptCoordinationRequest() != nil:
		v := action.GetAcceptCoordinationRequest()
		_, err = service.Accept(ctx, auth, cr.DecisionCommand{CommandID: req.CommandId, ExpectedRevision: current, RequestID: cr.RequestID(v.CoordinationRequestId), Reason: v.GetReason()})
	case action.GetRejectCoordinationRequest() != nil:
		v := action.GetRejectCoordinationRequest()
		_, err = service.Reject(ctx, auth, cr.DecisionCommand{CommandID: req.CommandId, ExpectedRevision: current, RequestID: cr.RequestID(v.CoordinationRequestId), Reason: v.GetReason()})
	}
	if err != nil {
		if errors.Is(err, cr.ErrUnauthorized) || errors.Is(err, cr.ErrWrongRecipient) {
			return nil, pb.CommandReply_UNAUTHORIZED, current, err
		}
		return nil, pb.CommandReply_INVALID_ARGUMENT, current, err
	}
	initial, err := decodeBoard(board)
	if err != nil {
		return nil, pb.CommandReply_INVALID_ARGUMENT, current, err
	}
	initial.Revision++
	initial.GeneratedAt = at
	for i := range initial.Flights {
		f := &initial.Flights[i]
		if f.Slot != nil {
			f.Slot.Revision = initial.Revision
		}
		f.QueueOffers = nil
	}
	t := encodeBoard(initial, aman.TechnicalHealth{})
	t.Airport.Health = board.Airport.Health
	t.Airport.ConfiguredMode = board.Airport.ConfiguredMode
	t.Airport.TimelineMappings = board.Airport.TimelineMappings
	for _, f := range t.Flights {
		for _, prior := range board.Flights {
			if f.Callsign == prior.Callsign {
				f.SourceObservations = prior.SourceObservations
				f.HoldingEatProjection = prior.HoldingEatProjection
			}
		}
	}
	for i, request := range capture.changed {
		t.Coordinations = append(t.Coordinations, encodeCoordination(request))
		id, _ := cluster.AmanIntentID(req.CommandId, fmt.Sprintf("coordination-audit/%d", i))
		before := ""
		for _, old := range board.Coordinations {
			if old.Id == string(request.ID) {
				before = old.State
			}
		}
		reason := ""
		if request.Decision != nil {
			reason = request.Decision.Reason
		}
		t.Audits = append(t.Audits, &pb.AmanAudit{Id: id, AirportRevision: uint64(initial.Revision), CreatedAt: timestamp(at), Actor: req.Actor, Fact: &pb.AmanAudit_Coordination{Coordination: &pb.AmanCoordinationAudit{RequestId: string(request.ID), Before: before, After: string(request.State), Reason: reason}}})
	}
	copy := proto.Clone(req).(*pb.CommandRequest)
	copy.ExpectedEntityRevision = &current
	return cluster.PlanAmanTransition(copy, state, t)
}

func encodeCoordination(r cr.Request) *pb.AmanCoordination {
	v := &pb.AmanCoordination{Id: string(r.ID), CommandId: r.CommandID, Callsign: string(r.Callsign), RecipientController: string(r.RecipientController), RecipientStatus: string(r.RecipientStatus), State: string(r.State), SubmittedBy: r.SubmittedBy, SubmittedRole: r.SubmittedRole, CreatedAt: timestamp(r.CreatedAt), UpdatedAt: timestamp(r.UpdatedAt), ResolvedAt: optionalTimestamp(r.ResolvedAt), Supersedes: optionalString[string](r.Supersedes), SupersededBy: optionalString[string](r.SupersededBy)}
	if p := r.Payload.RouteDirect; p != nil {
		v.Request = &pb.AmanCoordination_RouteDirect{RouteDirect: &pb.AmanRouteDirect{Route: optionalText(p.Route), DirectTo: optionalText(p.DirectTo)}}
	}
	if p := r.Payload.Speed; p != nil {
		v.Request = &pb.AmanCoordination_Speed{Speed: &pb.AmanSpeed{Requested: p.Requested}}
	}
	if d := r.Decision; d != nil {
		v.Decision = &pb.AmanCoordinationDecision{CommandId: d.CommandID, Airport: d.Airport, Actor: d.Actor, Role: d.Role, AuthoritativeRecipient: string(d.AuthoritativeRecipient), RequestId: string(d.RequestID), RequestKind: string(d.RequestKind), BeforeState: string(d.BeforeState), AfterState: string(d.AfterState), Reason: d.Reason, ReceivedAt: timestamp(d.ReceivedAt)}
	}
	for _, t := range r.RecipientTransfers {
		v.RecipientTransfers = append(v.RecipientTransfers, &pb.AmanRecipientTransfer{OwnershipFact: t.OwnershipFact, OwnershipRevision: t.OwnershipRevision, PreviousRecipient: string(t.PreviousRecipient), NewRecipient: string(t.NewRecipient), TransferredAt: timestamp(t.TransferredAt)})
	}
	if e := r.Expiry; e != nil {
		v.Expiry = &pb.AmanCoordinationExpiry{FactId: e.FactID, FactRevision: e.FactRevision, Reason: string(e.Reason), ExpiredAt: timestamp(e.ExpiredAt)}
	}
	if c := r.Clearance; c != nil {
		v.Clearance = &pb.AmanClearance{FactId: c.FactID, Kind: string(c.Kind), Value: c.Value, Issuer: c.Issuer, ObservedAt: timestamp(c.ObservedAt)}
	}
	return v
}
func decodeCoordination(v *pb.AmanCoordination, airport string) (cr.Request, error) {
	r := cr.Request{ID: cr.RequestID(v.Id), CommandID: v.CommandId, Airport: airport, Callsign: cr.Callsign(v.Callsign), RecipientController: cr.ControllerID(v.RecipientController), RecipientStatus: cr.RecipientStatus(v.RecipientStatus), State: cr.State(v.State), SubmittedBy: v.SubmittedBy, SubmittedRole: v.SubmittedRole, CreatedAt: instant(v.CreatedAt), UpdatedAt: instant(v.UpdatedAt), ResolvedAt: optionalInstant(v.ResolvedAt), Supersedes: optionalString[cr.RequestID](v.Supersedes), SupersededBy: optionalString[cr.RequestID](v.SupersededBy)}
	if p := v.GetRouteDirect(); p != nil {
		r.Kind = cr.KindRouteDirect
		r.Payload.RouteDirect = &cr.RouteDirectPayload{Route: p.GetRoute(), DirectTo: p.GetDirectTo()}
	}
	if p := v.GetSpeed(); p != nil {
		r.Kind = cr.KindSpeed
		r.Payload.Speed = &cr.SpeedPayload{Requested: p.Requested}
	}
	if d := v.Decision; d != nil {
		r.Decision = &cr.Decision{CommandID: d.CommandId, Airport: d.Airport, Actor: d.Actor, Role: d.Role, AuthoritativeRecipient: cr.ControllerID(d.AuthoritativeRecipient), RequestID: cr.RequestID(d.RequestId), RequestKind: cr.Kind(d.RequestKind), BeforeState: cr.State(d.BeforeState), AfterState: cr.State(d.AfterState), Reason: d.Reason, ReceivedAt: instant(d.ReceivedAt)}
	}
	for _, t := range v.RecipientTransfers {
		r.RecipientTransfers = append(r.RecipientTransfers, cr.RecipientTransfer{OwnershipFact: t.OwnershipFact, OwnershipRevision: t.OwnershipRevision, PreviousRecipient: cr.ControllerID(t.PreviousRecipient), NewRecipient: cr.ControllerID(t.NewRecipient), TransferredAt: instant(t.TransferredAt)})
	}
	if e := v.Expiry; e != nil {
		r.Expiry = &cr.Expiry{FactID: e.FactId, FactRevision: e.FactRevision, Reason: cr.ExpiryReason(e.Reason), ExpiredAt: instant(e.ExpiredAt)}
	}
	if c := v.Clearance; c != nil {
		r.Clearance = &cr.ClearanceAudit{FactID: c.FactId, Kind: cr.Kind(c.Kind), Value: c.Value, Issuer: c.Issuer, ObservedAt: instant(c.ObservedAt)}
	}
	return r, r.Validate()
}
