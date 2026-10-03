package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"FlightStrips/internal/aman/terminal"
	"FlightStrips/internal/amancandidate"
	"FlightStrips/internal/cluster"
	"FlightStrips/internal/config"
	"FlightStrips/internal/euroscopebinary"
	"FlightStrips/internal/frontendbinary"
	"FlightStrips/internal/natsresources"
	"FlightStrips/internal/pdc"
	"FlightStrips/internal/services"
	pb "FlightStrips/pkg/events/cluster"

	"github.com/nats-io/nats.go"
)

// NATSConfig configures the sole operational runtime.
// Resources must already have been created by the administrator bootstrap.
type NATSConfig struct {
	Resources      natsresources.Config
	EffectKeyID    string
	EffectKeyFiles map[string]string
	Airports       []string
}

// NATSDependencies supplies transport configuration, never policy or persistence.
type NATSDependencies struct {
	HTTPClient       *http.Client
	CDMBaseURL       string
	AIRACBaseURL     string
	MetarBaseURL     string
	ATISURL          string
	OpenMeteoBaseURL string
	HoppieBaseURL    string
}

type natsRuntime struct {
	ctx            context.Context
	cancel         context.CancelFunc
	admissionCtx   context.Context
	stopAdmissions context.CancelFunc
	lifecycle      sync.Mutex
	closeMu        sync.Mutex
	requests       sync.WaitGroup
	nc             *nats.Conn
	projection     *cluster.Projection
	owner          *cluster.OwnerRuntime
	async          *cluster.AsyncSessionOwners
	router         *cluster.CommandRouter
	registry       cluster.SessionRegistry
	source         cluster.NavigationWeather
	secrets        cluster.EffectSecrets
	fanout         *cluster.SessionFanout
	effects        *cluster.Effects
	deadlines      *euroscopebinary.DeadlineCandidate
	cdm            *services.CdmCandidate
	cdmActions     *services.CdmActionService
	pdc            *pdc.Candidate
	stands         cluster.StandState
	aman           *amancandidate.Worker
	navigation     *amancandidate.Worker
	terminal       terminal.Configuration
	work           *cluster.SessionWork
	supervisors    []func(context.Context) error
	wg             sync.WaitGroup
	once           sync.Once
	closing        atomic.Bool
	workerErr      atomic.Pointer[error]
	metrics        runtimeMetrics
	vatsimPasses   map[string]vatsimPass
}

func globalNATSRef() *pb.AggregateRef {
	return &pb.AggregateRef{Target: &pb.AggregateRef_Global{Global: &pb.GlobalRef{}}}
}
func airportNATSRef(icao string) *pb.AggregateRef {
	return &pb.AggregateRef{Target: &pb.AggregateRef_Airport{Airport: &pb.AirportRef{Icao: icao}}}
}
func sessionNATSRef(id int32) *pb.AggregateRef {
	return &pb.AggregateRef{Target: &pb.AggregateRef_Session{Session: &pb.SessionRef{Id: id}}}
}

// BuildNATS starts replay and owner routing so both construction and connection
// admission can wait on accepted state. StartWorkers starts the domain/provider
// supervisors once. Resources are verified, never created by the backend.
func BuildNATS(ctx context.Context, cfg Config, deps Dependencies) (result *App, err error) {
	stage := "configuration"
	defer func() {
		if err != nil {
			err = &startupStageError{stage: stage, cause: err}
		}
	}()
	deps = faultDependencies(deps)
	cfg = cfg.withDefaults()
	cfg.Navigation = cfg.Navigation.Normalize()
	if cfg.EnableTestTools && isLiveEnvironment(cfg.Environment) {
		return nil, errors.New("ENABLE_TEST_TOOLS cannot be enabled in a live environment")
	}
	if cfg.EnableTestTools {
		return nil, errors.New("SAT scenario/replay tools are disabled; ENABLE_TEST_TOOLS must remain false")
	}
	if err = cfg.Navigation.Validate(); err != nil {
		return nil, err
	}
	if err = cfg.AMAN.Validate(); err != nil {
		return nil, err
	}
	stage = "authentication"
	auth, err := buildAuthenticationService(cfg, deps.AuthenticationService)
	if err != nil {
		return nil, err
	}
	stage = "transport_connect"
	nc, err := natsresources.Connect(cfg.NATS.Resources)
	if err != nil {
		return nil, err
	}
	runtimeCtx, cancel := context.WithCancel(ctx)
	r := &natsRuntime{ctx: runtimeCtx, cancel: cancel, nc: nc}
	r.admissionCtx, r.stopAdmissions = context.WithCancel(runtimeCtx)
	defer func() {
		if err != nil {
			cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
			defer stop()
			_ = r.close(cleanup)
		}
	}()
	stage = "resource_verify"
	if err = natsresources.Verify(ctx, nc, cfg.NATS.Resources); err != nil {
		return nil, fmt.Errorf("verify NATS resources: %w", err)
	}
	stage = "projection_construct"
	r.projection, err = cluster.NewProjection(nc, cfg.NATS.Resources)
	if err != nil {
		return nil, err
	}
	stage = "projection_replay"
	r.run("projection", r.projection.Run)
	if err = r.await(ctx, r.projection.Ready); err != nil {
		return nil, err
	}
	store := measuredStore{EventStore: cluster.NATSStore{JS: r.projection.JS}, metrics: &r.metrics}
	stage = "owner_construct"
	r.owner, err = cluster.NewOwnerRuntime(nc, r.projection, store)
	if err != nil {
		return nil, err
	}
	r.async = cluster.NewAsyncSessionOwners(r.projection, r.owner, store)
	r.projection.Async = r.async
	r.async.RegisterPositionTranslator(r.projection.TranslatePositionSources)
	writer := cluster.Writer{Store: store, Projection: r.projection, Lease: r.owner, NodeID: r.owner.NodeID}
	r.router = &cluster.CommandRouter{NC: nc, Projection: r.projection, Lease: r.owner, Writer: writer}
	routed := cluster.RoutedLifecycleStore{Router: r, Projection: r.projection}
	r.registry = cluster.SessionRegistry{Store: routed}
	stage = "objects_open"
	objects, err := r.projection.JS.ObjectStore(cfg.NATS.Resources.Names.Objects)
	if err != nil {
		return nil, err
	}
	r.source = cluster.NavigationWeather{Writer: writer, Objects: cluster.NATSObjects{Store: objects}, Cache: cluster.NewVerifiedObjectCache(32 * 1024 * 1024)}
	r.projection.SetObjectCache(r.source.Cache)
	stage = "effect_secrets"
	r.secrets, err = cluster.LoadEffectSecrets(objects, cfg.NATS.EffectKeyID, cfg.NATS.EffectKeyFiles)
	if err != nil {
		return nil, err
	}
	stage = "domain_construct"
	readiness := configureStandAssignment(cfg.EnableStandAssignment, cfg.StandAssignmentAircraftJSON)
	r.stands = cluster.StandState{Store: routed, Projection: r.projection, Stands: config.GetStandCapabilities(), Policy: config.GetAirlineAssignment(), Aircraft: config.GetAircraftReference(), Engines: config.GetAircraftEngineReference(), Borders: config.GetAirportCountries()}
	transceivers, err := cluster.NewTransceiverSource(r.source)
	if err != nil {
		return nil, err
	}
	provider := deps.PDCClient
	if provider == nil && cfg.HoppieLogon != "" {
		provider = pdc.NewClientWithTransport(cfg.HoppieLogon, deps.NATS.HoppieBaseURL, deps.NATS.HTTPClient)
	}
	base := cluster.SessionLifecyclePlanner(routed.ReadDurable)
	sessionPlan := func(ctx context.Context, req *pb.CommandRequest, state *cluster.Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
		if req.GetSystem().GetCreateSession() != nil || req.GetSystem().GetDeleteSession() != nil {
			return base(ctx, req, state)
		}
		if req.GetSystem().GetApplyAmanSession() != nil {
			return amancandidate.DestinationPlanner(routed)(ctx, req, state)
		}
		return cluster.PlanStripCommands(r.stands.PlanStand)(ctx, req, state)
	}
	if cfg.EnablePDC {
		r.pdc, err = pdc.NewCandidate(writer, r.source.Objects, provider, sessionPlan, transceivers)
		if err != nil {
			return nil, err
		}
	}
	r.cdm, err = buildNATSCdm(cfg, deps, writer, r.source)
	if err != nil {
		return nil, err
	}
	r.cdmActions, err = services.NewCdmActionService(cluster.RoutedLifecycleStore{Router: r.router, Projection: r.projection})
	if err != nil {
		return nil, err
	}
	plan := r.cdm.Planner(sessionPlan)
	if r.pdc != nil {
		plan = r.pdc.Bind(plan)
	}
	plan = (cluster.PrivateMessagePlanner{Secrets: r.secrets, Projection: r.projection, Fallback: plan}).Plan
	r.deadlines, err = euroscopebinary.NewDeadlineCandidate(r.router, r.source, plan)
	if err != nil {
		return nil, err
	}
	r.deadlines.Coverage = transceivers.GetFrequencies
	r.deadlines.NextInbound = r.inbound
	r.deadlines.ObservedStrip = r.observedStrip
	plan = cluster.MasterElectionPlanner(r.projection, r.deadlines.Planner)
	r.router.Writer.Plan = func(ctx context.Context, req *pb.CommandRequest, state *cluster.Aggregate) (*pb.DomainChange, pb.CommandReply_Status, uint64, error) {
		if req.Aggregate.GetSession() != nil {
			return plan(ctx, req, state)
		}
		if req.GetSystem().GetCreateSession() != nil || req.GetSystem().GetDeleteSession() != nil {
			return base(ctx, req, state)
		}
		if req.GetSystem().GetAdvanceWorkflow() != nil {
			return cluster.PlanAmanWorkflow(ctx, req, state)
		}
		if req.GetClient().GetAman() != nil {
			return r.planAMAN(ctx, req, state)
		}
		if req.GetSystem().GetReportAmanRouteFact() != nil {
			return r.planRouteFact(ctx, req, state)
		}
		if req.Actor.GetId() == "provider-quota" {
			return planNATSQuota(ctx, req, state)
		}
		if req.Actor.GetId() == "ecfmp-http" {
			return r.planECFMP(ctx, req, state)
		}
		return cluster.PlanControllerSector(ctx, req, state)
	}
	r.fanout = &cluster.SessionFanout{NC: nc, Projection: r.projection, NodeID: r.owner.NodeID}
	r.effects = &cluster.Effects{Owner: r.owner, Fanout: r.fanout, Secrets: r.secrets}
	r.work = &cluster.SessionWork{Registry: r.registry, Store: routed, Projection: r.projection, Owner: r.owner}
	r.deadlines.BindWorker(r.work)
	if r.pdc != nil {
		r.work.PDC = func(ctx context.Context, id int32) error {
			state, err := r.projection.Read(sessionNATSRef(id))
			if err != nil {
				return err
			}
			seed := state.Indexes[pb.EntityKind_SESSION][fmt.Sprint(id)].GetValue().GetSession()
			// The production Hoppie service polls only LIVE sessions. A training
			// session must never consume another session's airport inbox.
			if seed == nil || seed.Name != "LIVE" {
				return nil
			}
			return r.pdc.PDC(ctx, id)
		}
	}
	r.work.CDM = r.cdm.CDM
	traffic, err := services.NewTrafficCandidate(writer)
	if err != nil {
		return nil, err
	}
	if cfg.EnableTraffic {
		r.work.Traffic = traffic.Traffic
	}
	if cfg.EnableStandAssignment && readiness.Ready {
		lifecycle, e := services.NewVatsimLifecycleCandidate(r.source, writer, r.stands, r.secrets)
		if e != nil {
			return nil, e
		}
		lifecycle.AllowPrefiles = cfg.EnableStandAssignmentPrefiles
		lifecycle.HoldDuration, lifecycle.BlockExtension = cfg.StandAssignmentHoldDuration, cfg.StandAssignmentBlockExtension
		lifecycle.ESMessages = &cfg.EnableStandAssignmentESMessages
		lifecycle.Positions = r.deadlines.Positions
		r.work.Departure, r.work.Arrival = r.liveSessionWorker(lifecycle.Departure), r.liveSessionWorker(lifecycle.Arrival)
	}
	stage = "providers_construct"
	if err = r.assembleProviders(cfg, deps, transceivers); err != nil {
		return nil, err
	}
	stage = "owner_track"
	if err = r.owner.Track(globalNATSRef()); err != nil {
		return nil, err
	}
	for _, icao := range cfg.NATS.Airports {
		if err = r.owner.Track(airportNATSRef(icao)); err != nil {
			return nil, err
		}
	}
	stage = "runtime_supervisors"
	r.run("owner", r.owner.Run)
	r.run("router", r.router.Serve)
	r.run("quota admissions", r.serveQuota)
	r.run("socket admission", r.deadlines.Serve)
	r.run("socket fanout", r.fanout.ServeTargeted)
	r.run("aggregate discovery", r.discover)
	front := frontendbinary.Handler{Projection: r.projection, Router: r, Auth: auth, NodeID: r.owner.NodeID}
	euro := euroscopebinary.Handler{Projection: r.projection, Fanout: r.fanout, Sessions: natsSessions{r}, Auth: auth, Sync: cluster.SessionObservations{Store: routed}, Controllers: cluster.ControllerSector{Store: routed}, Inbound: r.deadlines.Inbound, Deadlines: r.deadlines, Effects: r.effects, RenderEffect: euroscopebinary.EffectRenderer(r.secrets)}
	stage = "http_construct"
	handler, err := r.buildHTTP(cfg, deps, auth, readiness, front, euro)
	if err != nil {
		return nil, err
	}
	return &App{natsRuntime: r, handler: r.admissions(handler), standAssignmentReadiness: readiness}, nil
}

func (r *natsRuntime) run(name string, fn func(context.Context) error) {
	r.lifecycle.Lock()
	defer r.lifecycle.Unlock()
	if r.closing.Load() {
		return
	}
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		if err := fn(r.ctx); err != nil && r.ctx.Err() == nil {
			wrapped := fmt.Errorf("%s: %w", name, err)
			r.workerErr.Store(&wrapped)
			slog.Error("NATS runtime stopped", "worker", name, "error_type", fmt.Sprintf("%T", err))
			r.cancel()
		}
	}()
}
func (r *natsRuntime) await(ctx context.Context, ready func() error) error {
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	for {
		if err := r.workerErr.Load(); err != nil {
			return *err
		}
		if ready() == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-r.ctx.Done():
			return r.ctx.Err()
		case <-tick.C:
		}
	}
}
func (r *natsRuntime) ready() error {
	if r.closing.Load() || r.ctx.Err() != nil {
		return errors.New("runtime stopping")
	}
	if err := r.workerErr.Load(); err != nil {
		return *err
	}
	return r.owner.Ready()
}
func (r *natsRuntime) Route(ctx context.Context, req *pb.CommandRequest) *pb.CommandReply {
	if r.closing.Load() || r.ctx.Err() != nil {
		return &pb.CommandReply{ProtocolRevision: 1, CommandId: req.GetCommandId(), Status: pb.CommandReply_UNAVAILABLE}
	}
	_ = r.owner.Track(req.GetAggregate())
	if client := req.GetClient(); client.GetCdm() != nil || client.GetValidation().GetUpdateTobt() != nil || client.GetStrip().GetUpdateData().GetEobt() != nil {
		return r.cdmActions.Execute(ctx, req)
	}
	return r.router.Route(ctx, req)
}

// RouteDurable is the cross-aggregate lifecycle prerequisite boundary. Session
// identity and deletion must survive before the global registry advances.
func (r *natsRuntime) RouteDurable(ctx context.Context, req *pb.CommandRequest) *pb.CommandReply {
	if r.closing.Load() || r.ctx.Err() != nil {
		return &pb.CommandReply{ProtocolRevision: 1, CommandId: req.GetCommandId(), Status: pb.CommandReply_UNAVAILABLE}
	}
	_ = r.owner.Track(req.GetAggregate())
	return r.router.RouteDurable(ctx, req)
}
func (r *natsRuntime) startWorkers(ctx context.Context) {
	r.once.Do(func() {
		r.run("worker context", func(run context.Context) error {
			select {
			case <-run.Done():
			case <-ctx.Done():
				r.cancel()
			}
			return nil
		})
		r.run("session supervisor", r.work.Run)
		r.run("effects", r.effects.Run)
		for i, worker := range r.supervisors {
			r.run(fmt.Sprintf("provider supervisor %d", i), worker)
		}
	})
}
func (r *natsRuntime) discover(ctx context.Context) error {
	return periodic(ctx, 250*time.Millisecond, func(ctx context.Context, _ time.Time) error {
		entities, err := r.projection.ReadEntities(globalNATSRef(), pb.EntityKind_SESSION_REGISTRY)
		if err != nil {
			return err
		}
		for _, entity := range entities {
			s := entity.Value.GetSessionRegistry()
			if s.State == pb.SessionRegistry_DELETED {
				continue
			}
			if err = r.owner.Track(sessionNATSRef(s.Id)); err != nil {
				return err
			}
			if err = r.owner.Track(airportNATSRef(s.Airport)); err != nil {
				return err
			}
		}
		if r.owner.CanWrite(globalNATSRef()) {
			return r.registry.Recover(ctx)
		}
		return nil
	})
}
func periodic(ctx context.Context, interval time.Duration, step func(context.Context, time.Time) error) error {
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := step(ctx, time.Now().UTC()); err != nil && ctx.Err() == nil {
			slog.DebugContext(ctx, "owner pass deferred", "error_type", fmt.Sprintf("%T", err))
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
		}
	}
}

type natsSessions struct{ r *natsRuntime }

func (s natsSessions) GetOrCreateSession(ctx context.Context, airport, name string) (*pb.Session, error) {
	// A restarted node may still need the previous eight-second owner lease
	// and node-presence window to expire before the cross-aggregate seed/active
	// workflow can finish. Keep authentication reads separately bounded.
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		session, err := s.r.registry.GetOrCreateSession(ctx, airport, name)
		if err == nil {
			return session, nil
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("session lookup: %w (last attempt: %v)", ctx.Err(), err)
		case <-s.r.ctx.Done():
			return nil, s.r.ctx.Err()
		case <-tick.C:
		}
	}
}
func (r *natsRuntime) admissions(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/healthz" || req.URL.Path == "/readyz" || req.URL.Path == "/metrics" {
			next.ServeHTTP(w, req)
			return
		}
		r.lifecycle.Lock()
		if r.closing.Load() {
			r.lifecycle.Unlock()
			http.Error(w, "application unavailable", http.StatusServiceUnavailable)
			return
		}
		r.requests.Add(1)
		r.lifecycle.Unlock()
		defer r.requests.Done()
		if req.URL.Path != "/healthz" && req.URL.Path != "/readyz" && r.ready() != nil {
			http.Error(w, "application unavailable", http.StatusServiceUnavailable)
			return
		}
		ctx, cancel := context.WithCancel(req.Context())
		defer cancel()
		stop := context.AfterFunc(r.admissionCtx, cancel)
		defer stop()
		next.ServeHTTP(w, req.WithContext(ctx))
	})
}

// Stop admissions and join sockets; drain position barriers while fencing is
// live; cancel and join all workers; drain NATS last. Repeated calls are safe.
func (r *natsRuntime) close(ctx context.Context) error {
	r.closeMu.Lock()
	defer r.closeMu.Unlock()
	r.lifecycle.Lock()
	r.closing.Store(true)
	r.lifecycle.Unlock()
	if r.stopAdmissions != nil {
		r.stopAdmissions()
	}
	admitted := make(chan struct{})
	go func() { r.requests.Wait(); close(admitted) }()
	select {
	case <-ctx.Done():
		r.cancel()
		r.nc.Close()
		return ctx.Err()
	case <-admitted:
	}
	// Accepted position receipts drain while projection and owner fencing are
	// still running. No new operational request can enter after the gate closes.
	if r.deadlines != nil {
		if err := r.deadlines.Close(ctx); err != nil {
			r.cancel()
			r.nc.Close()
			return err
		}
	}
	// Position writers may still append their already admitted work during
	// their drain. Seal session turns afterwards, then flush all accepted RAM
	// state while lease renewal, projection and the broker remain alive.
	if r.async != nil {
		r.async.BeginDrain()
		if err := r.async.Drain(ctx); err != nil {
			r.cancel()
			if r.nc != nil {
				r.nc.Close()
			}
			return fmt.Errorf("flush accepted session state: %w", err)
		}
	}
	r.cancel()
	joined := make(chan struct{})
	go func() { r.wg.Wait(); close(joined) }()
	select {
	case <-ctx.Done():
		if r.nc != nil {
			r.nc.Close()
		}
		return ctx.Err()
	case <-joined:
	}
	if r.nc != nil && !r.nc.IsClosed() {
		if err := r.nc.Drain(); err != nil {
			r.nc.Close()
			return err
		}
		tick := time.NewTicker(10 * time.Millisecond)
		defer tick.Stop()
		for !r.nc.IsClosed() {
			select {
			case <-ctx.Done():
				r.nc.Close()
				return ctx.Err()
			case <-tick.C:
			}
		}
	}
	return nil
}
