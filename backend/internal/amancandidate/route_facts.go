package amancandidate

import (
	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/routefact"
	"FlightStrips/internal/cluster"
	cr "FlightStrips/internal/coordinationrequest"
	"FlightStrips/internal/models"
	"FlightStrips/internal/services"
	pb "FlightStrips/pkg/events/cluster"
	"context"
	"fmt"
	"google.golang.org/protobuf/proto"
	"sort"
	"strings"
	"time"
)

type factStrips struct{ state *cluster.Aggregate }

func (s factStrips) GetByCallsign(_ context.Context, id int32, callsign string) (*models.Strip, error) {
	if s.state.Ref.GetSession().Id != id {
		return nil, fmt.Errorf("session mismatch")
	}
	strip := s.state.Indexes[pb.EntityKind_STRIP][callsign].GetValue().GetStrip()
	if strip == nil {
		return nil, fmt.Errorf("strip absent")
	}
	value := services.CandidateModelStrip(strip, id)
	value.TrackingController = strip.TrackingController
	return value, nil
}

// The production route service requests reevaluation after its detached commit.
// The owner planner performs that reevaluation before publishing one CAS event.
type factReconcile struct{ requested bool }

func (r *factReconcile) Reconcile(context.Context) { r.requested = true }

func (r *coordinationCapture) CorrelateAccepted(_ context.Context, fact cr.ClearanceFact) (cr.CommitResult, error) {
	requests := make([]cr.Request, 0, len(r.board.Coordinations))
	for _, v := range r.board.Coordinations {
		request, err := decodeCoordination(v, r.airport)
		if err != nil {
			return cr.CommitResult{}, err
		}
		requests = append(requests, request)
	}
	sort.Slice(requests, func(i, j int) bool {
		if requests[i].CreatedAt.Equal(requests[j].CreatedAt) {
			return requests[i].ID < requests[j].ID
		}
		return requests[i].CreatedAt.Before(requests[j].CreatedAt)
	})
	for i := len(requests) - 1; i >= 0; i-- {
		request := requests[i]
		if request.Clearance != nil && request.Clearance.FactID == fact.FactID {
			return cr.CommitResult{Request: request, Revision: r.revision, Duplicate: true}, nil
		}
	}
	for i := len(requests) - 1; i >= 0; i-- {
		request := requests[i]
		if request.Callsign != fact.Callsign || request.Kind != fact.Kind || request.State != cr.StateAccepted || request.Clearance != nil || request.ResolvedAt.After(fact.ObservedAt) {
			continue
		}
		match := request.Kind == cr.KindSpeed && strings.EqualFold(request.Payload.Speed.Requested, fact.Value) || request.Kind == cr.KindRouteDirect && request.Payload.RouteDirect.DirectTo != "" && strings.EqualFold(request.Payload.RouteDirect.DirectTo, fact.Value)
		if !match {
			continue
		}
		value, err := request.Correlate(cr.ClearanceAudit{FactID: fact.FactID, Kind: fact.Kind, Value: fact.Value, Issuer: fact.Issuer, ObservedAt: fact.ObservedAt})
		if err != nil {
			return cr.CommitResult{}, err
		}
		r.changed = append(r.changed, value)
		return cr.CommitResult{Request: value, Revision: r.revision + 1}, nil
	}
	return cr.CommitResult{Revision: r.revision}, nil
}

func (w *Worker) PlanRouteFact(ctx context.Context, req *pb.CommandRequest, state, session *cluster.Aggregate, controller *pb.Controller) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
	input := req.GetSystem().GetReportAmanRouteFact()
	board, err := cluster.ReadAmanBoard(state)
	if err != nil {
		return nil, pb.CommandReply_UNAVAILABLE, 0, err
	}
	current := board.Airport.GetRevision()
	if input == nil || input.ObservedAt == nil || input.ObservedAt.CheckValid() != nil || controller == nil || controller.Observer || board.Airport == nil {
		return nil, pb.CommandReply_INVALID_ARGUMENT, current, fmt.Errorf("invalid authenticated route fact")
	}
	initial, err := decodeBoard(board)
	if err != nil {
		return nil, pb.CommandReply_UNAVAILABLE, current, err
	}
	at := w.options.Now().UTC()
	nav, err := loadNavigation(ctx, w.options.Source, w.options.RouteWorker, w.options.RouteResolver, initial.Airport)
	if err != nil {
		return nil, pb.CommandReply_UNAVAILABLE, current, err
	}
	repo := &evaluationRepository{initial: initial, exists: true}
	reconcile := &factReconcile{}
	capture := &coordinationCapture{board: board, airport: initial.Airport, revision: current}
	service, err := routefact.New(routefact.Dependencies{Repository: repo, Strips: factStrips{session}, Geometry: nav, Publisher: evaluationPublisher{}, Reconciler: reconcile, Now: func() time.Time { return at }, NewID: func() string { return req.CommandId }, Correlator: cr.NewService(capture, capture)})
	if err != nil {
		return nil, pb.CommandReply_UNAVAILABLE, current, err
	}
	switch fact := input.Fact.(type) {
	case *pb.ReportAmanRouteFact_DirectToFix:
		fix := fact.DirectToFix
		err = service.ReportDirectTo(ctx, session.Ref.GetSession().Id, initial.Airport, input.Callsign, controller.Callsign, &fix, input.ObservedAt.AsTime().UTC())
	case *pb.ReportAmanRouteFact_AssignedSpeed:
		err = service.ReportSpeed(ctx, session.Ref.GetSession().Id, initial.Airport, input.Callsign, controller.Callsign, fact.AssignedSpeed, input.ObservedAt.AsTime().UTC())
	default:
		err = fmt.Errorf("route fact kind required")
	}
	if err != nil {
		return nil, pb.CommandReply_INVALID_ARGUMENT, current, err
	}
	if repo.commit == nil && len(capture.changed) == 0 {
		return &pb.DomainChange{}, pb.CommandReply_COMMITTED, current, nil
	}
	next := initial
	if repo.commit != nil {
		next = repo.commit.State
	} else {
		next.Revision++
		next.GeneratedAt = at
	}
	for i := range next.Flights {
		if next.Flights[i].Slot != nil {
			next.Flights[i].Slot.Revision = next.Revision
		}
		next.Flights[i].QueueOffers = nil
	}
	transition := encodeBoard(next, aman.TechnicalHealth{})
	preserveBoardMetadata(&transition, board)
	if err := refreshDisplay(transition.Airport, next, board.Airport); err != nil {
		return nil, pb.CommandReply_UNAVAILABLE, current, err
	}
	if reconcile.requested {
		// Evaluate the fact and the resulting sequence in one accepted revision.
		// The detached route service's intermediate commit is never published.
		transition.Airport.Revision = current
		for _, flight := range transition.Flights {
			if flight.Slot != nil {
				flight.Slot.Revision = current
			}
		}
		inputBoard := cluster.AmanBoard{Airport: transition.Airport, Flights: transition.Flights, Coordinations: board.Coordinations}
		transition, err = w.evaluate(ctx, initial.Airport, inputBoard, at, nil, req.CommandId, req.Actor)
		if err != nil {
			return nil, pb.CommandReply_UNAVAILABLE, current, err
		}
	}
	for _, value := range capture.changed {
		encoded := encodeCoordination(value)
		replaced := false
		for i, existing := range transition.Coordinations {
			if existing.Id == encoded.Id {
				transition.Coordinations[i], replaced = encoded, true
				break
			}
		}
		if !replaced {
			transition.Coordinations = append(transition.Coordinations, encoded)
		}
		id, _ := cluster.AmanIntentID(req.CommandId, "clearance/"+string(value.ID))
		transition.Audits = append(transition.Audits, &pb.AmanAudit{Id: id, AirportRevision: uint64(next.Revision), CreatedAt: timestamp(at), Actor: req.Actor, Fact: &pb.AmanAudit_Coordination{Coordination: &pb.AmanCoordinationAudit{RequestId: string(value.ID), Before: string(value.State), After: string(value.State), Reason: "clearance_correlated"}}})
	}
	auditSeed, _ := cluster.AmanIntentID(req.CommandId, "route-fact-audit")
	for i, a := range repoAudit(repo) {
		v, e := convertAudit(a, auditSeed, i, req.Actor)
		if e != nil {
			return nil, pb.CommandReply_INVALID_ARGUMENT, current, e
		}
		transition.Audits = append(transition.Audits, v)
	}
	copy := proto.Clone(req).(*pb.CommandRequest)
	copy.ExpectedEntityRevision = &current
	return cluster.PlanAmanTransition(copy, state, transition)
}
func repoAudit(repo *evaluationRepository) []aman.AuditRecord {
	if repo.commit == nil {
		return nil
	}
	return repo.commit.AuditRecords
}
func preserveBoardMetadata(t *cluster.AmanTransition, b cluster.AmanBoard) {
	t.Airport.Health = b.Airport.Health
	t.Airport.ConfiguredMode = b.Airport.ConfiguredMode
	t.Airport.TimelineMappings = b.Airport.TimelineMappings
	for _, f := range t.Flights {
		for _, v := range b.Flights {
			if f.Callsign == v.Callsign {
				f.SourceObservations = v.SourceObservations
				f.HoldingEatProjection = v.HoldingEatProjection
			}
		}
	}
}
