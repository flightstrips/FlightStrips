// Package amancandidate binds the existing operational AMAN policy to accepted
// NATS state. Construction is dormant; app.Build does not start this worker.
package amancandidate

import (
	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/navdata"
	"FlightStrips/internal/aman/operational"
	"FlightStrips/internal/aman/predictor"
	"FlightStrips/internal/aman/terminal"
	"FlightStrips/internal/cluster"
	cr "FlightStrips/internal/coordinationrequest"
	pb "FlightStrips/pkg/events/cluster"
	"context"
	"fmt"
	"google.golang.org/protobuf/proto"
	"sort"
	"sync"
	"time"
)

type Options struct {
	Source            cluster.NavigationWeather
	State             cluster.AmanAdapter
	Sessions          cluster.LifecycleStore
	Projection        *cluster.Projection
	RouteWorker       cluster.ExternalCallWorker
	RouteResolver     navdata.RouteResolver
	Terminal          terminal.Configuration
	AircraftEngines   operational.AircraftEngineReference
	Mode              aman.RolloutMode
	SourceMode        aman.ObservationSourceMode
	VatsimStaleAfter  time.Duration
	HoldingEATEnabled bool
	Now               func() time.Time
	WindForAirport    func(string, time.Time) predictor.WindProfileReader
}
type Worker struct {
	options      Options
	candidate    cluster.AmanCandidateWorker
	vatsimMu     sync.Mutex
	vatsimInputs *acceptedVatsimGeneration
}

func New(options Options) (*Worker, error) {
	if options.State.Writer.Store == nil || options.Source.Writer.Store == nil || options.Sessions == nil || options.RouteResolver == nil {
		return nil, fmt.Errorf("AMAN candidate requires airport state, accepted inputs, session routing and route resolver")
	}
	if err := options.Terminal.ValidateOperationalSettings(); err != nil {
		return nil, err
	}
	if !options.Mode.Valid() {
		return nil, fmt.Errorf("invalid AMAN rollout mode")
	}
	if options.SourceMode == "" {
		options.SourceMode = aman.ObservationSourceHybrid
	}
	if !options.SourceMode.Valid() {
		return nil, fmt.Errorf("invalid AMAN source mode")
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.VatsimStaleAfter <= 0 {
		options.VatsimStaleAfter = 30 * time.Second
	}
	w := &Worker{options: options}
	w.candidate = cluster.AmanCandidateWorker{Source: options.Source, State: options.State, Intents: cluster.AmanIntentRunner{AirportWriter: options.State.Writer, Destination: options.Sessions, Step: w.Step}}
	return w, nil
}
func airportRef(icao string) *pb.AggregateRef {
	return &pb.AggregateRef{Target: &pb.AggregateRef_Airport{Airport: &pb.AirportRef{Icao: icao}}}
}
func globalRef() *pb.AggregateRef {
	return &pb.AggregateRef{Target: &pb.AggregateRef_Global{Global: &pb.GlobalRef{}}}
}
func sessionRef(id int32) *pb.AggregateRef {
	return &pb.AggregateRef{Target: &pb.AggregateRef_Session{Session: &pb.SessionRef{Id: id}}}
}
func (w *Worker) owner(icao string) bool {
	lease := w.options.State.Writer.Lease
	return lease == nil || lease.CanWrite(airportRef(icao))
}
func (w *Worker) ObserveVatsim(ctx context.Context, icao, callsign string) *pb.CommandReply {
	if !w.owner(icao) {
		return &pb.CommandReply{Status: pb.CommandReply_NOT_OWNER}
	}
	return w.candidate.ObserveVatsim(ctx, icao, callsign, w.EvaluateObservation)
}
func (w *Worker) ObserveMissingVatsim(ctx context.Context, icao, callsign string) *pb.CommandReply {
	if !w.owner(icao) {
		return &pb.CommandReply{Status: pb.CommandReply_NOT_OWNER}
	}
	return w.candidate.ObserveMissingVatsim(ctx, icao, callsign, w.EvaluateObservation)
}
func (w *Worker) Reconcile(ctx context.Context, icao string, deadline time.Time) *pb.CommandReply {
	if !w.owner(icao) {
		return &pb.CommandReply{Status: pb.CommandReply_NOT_OWNER}
	}
	return w.candidate.Reconcile(ctx, icao, deadline, w.EvaluateReconciliation)
}
func (w *Worker) Resume(ctx context.Context, icao string) error { return w.candidate.Resume(ctx, icao) }

func (w *Worker) EvaluateObservation(ctx context.Context, board cluster.AmanBoard, _ *pb.VatsimFlight, observation *pb.VatsimObservation, request *pb.CommandRequest) (cluster.AmanTransition, error) {
	return w.evaluate(ctx, request.Aggregate.GetAirport().Icao, board, w.options.Now().UTC(), observation, request.CommandId, request.Actor)
}
func (w *Worker) EvaluateReconciliation(ctx context.Context, board cluster.AmanBoard, at time.Time) (cluster.AmanTransition, error) {
	airport := string(w.options.Terminal.Airport)
	if board.Airport != nil {
		airport = board.Airport.Airport
	}
	id, err := cluster.ProviderEventCommandID("aman-reconcile", "aman", airport+"/"+at.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return cluster.AmanTransition{}, err
	}
	return w.evaluate(ctx, airport, board, at, nil, id, &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "aman-reconcile"})
}

// evaluationRepository captures the production policy's result for one CAS
// command. It never persists state or replaces the accepted NATS repository.
type evaluationRepository struct {
	initial aman.AirportState
	exists  bool
	commit  *aman.StateCommit
}

func (r *evaluationRepository) LoadAirportState(context.Context, string) (aman.AirportState, error) {
	if !r.exists {
		return aman.AirportState{}, &aman.DomainError{Class: aman.ErrorNotFound, Message: "airport not initialized"}
	}
	return r.initial, nil
}
func (r *evaluationRepository) Commit(_ context.Context, c aman.StateCommit) (aman.CommitResult, error) {
	if err := c.Validate(); err != nil {
		return aman.CommitResult{}, err
	}
	r.commit = &c
	return aman.CommitResult{State: c.State}, nil
}

type evaluationPublisher struct{}

func (evaluationPublisher) PublishAMANState(context.Context, aman.AirportState) error { return nil }

func (w *Worker) evaluate(ctx context.Context, airport string, board cluster.AmanBoard, at time.Time, observation *pb.VatsimObservation, id string, actor *pb.Actor) (cluster.AmanTransition, error) {
	if airport != string(w.options.Terminal.Airport) {
		return cluster.AmanTransition{}, fmt.Errorf("airport differs from configured terminal")
	}
	repo := &evaluationRepository{}
	if board.Airport != nil {
		var err error
		repo.initial, err = decodeBoard(board)
		if err != nil {
			return cluster.AmanTransition{}, err
		}
		repo.exists = true
	}
	nav, navErr := loadNavigation(ctx, w.options.Source, w.options.RouteWorker, w.options.RouteResolver, airport)
	config, mode := nav.config, w.options.Mode
	if navErr != nil {
		nav.snapshot = navdata.ActiveGeometrySnapshot{}
		// Missing accepted configuration cannot enable authoritative decisions.
		// Retain constructor identity only to publish degraded health/board state.
		config = w.options.Terminal
		if mode == aman.ModeAuthoritative {
			mode = aman.ModeReadOnly
		}
	}
	inputs, err := w.acceptedInputs(ctx, airport, board, at, observation)
	if err != nil {
		return cluster.AmanTransition{}, err
	}
	service, err := operational.New(operational.Dependencies{Repository: repo, Materializer: nav, Geometry: nav, Wind: w.windReader(airport, at), Runways: inputs, AircraftEngines: w.options.AircraftEngines,
		Terminal: config, TMAVolume: nav.tma, Airports: []string{airport}, Mode: mode, SourceMode: w.options.SourceMode, Publisher: evaluationPublisher{}, Now: func() time.Time { return at }})
	if err != nil {
		return cluster.AmanTransition{}, err
	}
	if w.options.SourceMode.UsesVATSIM() {
		if err = service.ObserveSourceHealth(ctx, inputs.vatsimStatus, inputs.vatsimAt); err != nil {
			return cluster.AmanTransition{}, err
		}
	}
	for _, o := range inputs.observations {
		if err = service.Observe(ctx, o); err != nil {
			return cluster.AmanTransition{}, err
		}
	}
	if err = service.EvaluateAccepted(ctx, airport); err != nil {
		return cluster.AmanTransition{}, err
	}
	state := repo.initial
	if repo.commit != nil {
		state = repo.commit.State
	}
	// Every accepted source identity and tick records a coherent replacement
	// revision, even when the operational flight set is otherwise unchanged.
	if repo.commit == nil {
		state.Revision++
		state.GeneratedAt = at.UTC().Truncate(time.Second)
	}
	health := service.TechnicalHealth(ctx)
	result := encodeBoard(state, health)
	if err := attachDisplay(result.Airport, state, health); err != nil {
		return cluster.AmanTransition{}, err
	}
	result.Airport.ConfiguredMode = string(w.options.Mode)
	desired := operational.HoldingEATs(state, health, w.options.HoldingEATEnabled, nav.snapshot)
	for _, f := range result.Flights {
		if value, ok := desired[f.Callsign]; ok {
			f.HoldingEatProjection = encodeAmanHoldingClearance(&value)
		}
	}
	for _, m := range nav.snapshot.TimelineMappings {
		result.Airport.TimelineMappings = append(result.Airport.TimelineMappings, encodeAmanTimelineMapping(&m))
	}
	for _, f := range result.Flights {
		for _, o := range inputs.observations {
			if o.Callsign == f.Callsign {
				copy := o
				f.SourceObservations = append(f.SourceObservations, encodeAmanFlightObservation(&copy))
			}
		}
	}
	if repo.commit != nil {
		for i, a := range repo.commit.AuditRecords {
			audit, err := convertAudit(a, id, i, actor)
			if err != nil {
				return cluster.AmanTransition{}, err
			}
			result.Audits = append(result.Audits, audit)
		}
	}
	// Coordination expiry was atomic with the SQL board commit; retain that
	// semantic here in the accepted airport event and append its typed audit.
	tracking := map[string]cr.ControllerID{}
	for _, session := range inputs.sessions {
		for _, entity := range session.EntitiesByKind(pb.EntityKind_STRIP) {
			strip := entity.Value.GetStrip()
			owner := cr.ControllerID(strip.TrackingController)
			if prior, found := tracking[strip.Callsign]; found && prior != owner {
				return cluster.AmanTransition{}, fmt.Errorf("ambiguous tracking controller for %s", strip.Callsign)
			}
			tracking[strip.Callsign] = owner
		}
	}
	for _, c := range board.Coordinations {
		if c.State != "pending" {
			continue
		}
		if owner, found := tracking[c.Callsign]; found && string(owner) != c.RecipientController {
			request, e := decodeCoordination(c, airport)
			if e != nil {
				return cluster.AmanTransition{}, e
			}
			request, e = request.TransferRecipient(id, uint64(state.Revision), owner, at)
			if e != nil {
				return cluster.AmanTransition{}, e
			}
			c = encodeCoordination(request)
			result.Coordinations = append(result.Coordinations, c)
			auditID, _ := cluster.AmanIntentID(id, "coordination-transfer/"+c.Id)
			result.Audits = append(result.Audits, &pb.AmanAudit{Id: auditID, AirportRevision: uint64(state.Revision), CreatedAt: timestamp(at), Actor: actor, Fact: &pb.AmanAudit_Coordination{Coordination: &pb.AmanCoordinationAudit{RequestId: c.Id, Before: "pending", After: "pending", Reason: "recipient_transferred"}}})
		}
		for _, f := range state.Flights {
			if f.Callsign != c.Callsign {
				continue
			}
			reason := ""
			switch {
			case f.State == aman.StateLanded:
				reason = "flight_completed"
			case f.State == aman.StateGoAround:
				reason = "go_around_confirmed"
			case f.State == aman.StateRemoved:
				reason = "authoritative_removal"
			case f.SequenceDisposition == aman.SequenceDispositionDesequenced:
				reason = "desequenced"
			}
			if reason != "" {
				updated := proto.Clone(c).(*pb.AmanCoordination)
				updated.State = "expired"
				updated.UpdatedAt = timestamp(state.GeneratedAt)
				updated.ResolvedAt = timestamp(state.GeneratedAt)
				updated.Expiry = &pb.AmanCoordinationExpiry{FactId: id, FactRevision: uint64(state.Revision), Reason: reason, ExpiredAt: timestamp(state.GeneratedAt)}
				replaced := false
				for i, prior := range result.Coordinations {
					if prior.Id == c.Id {
						result.Coordinations[i] = updated
						replaced = true
					}
				}
				if !replaced {
					result.Coordinations = append(result.Coordinations, updated)
				}
				auditID, _ := cluster.AmanIntentID(id, "coordination-expiry/"+c.Id)
				result.Audits = append(result.Audits, &pb.AmanAudit{Id: auditID, AirportRevision: uint64(state.Revision), CreatedAt: timestamp(state.GeneratedAt), Actor: actor, Fact: &pb.AmanAudit_Coordination{Coordination: &pb.AmanCoordinationAudit{RequestId: c.Id, Before: c.State, After: "expired", Reason: reason}}})
			}
		}
	}
	result.Workflows, err = w.holdingIntents(ctx, id, state, health, nav.snapshot, inputs.sessions)
	if err != nil {
		return cluster.AmanTransition{}, err
	}
	sort.Slice(result.Flights, func(i, j int) bool { return result.Flights[i].Callsign < result.Flights[j].Callsign })
	return result, nil
}

func (w *Worker) windReader(airport string, at time.Time) predictor.WindProfileReader {
	if w.options.WindForAirport != nil {
		return w.options.WindForAirport(airport, at)
	}
	return &committedWind{state: w.options.Source, airport: airport, at: at}
}
