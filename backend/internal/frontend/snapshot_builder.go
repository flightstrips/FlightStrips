package frontend

import (
	"FlightStrips/internal/clx"
	"FlightStrips/internal/config"
	internalModels "FlightStrips/internal/models"
	"FlightStrips/internal/repository"
	"FlightStrips/internal/shared"
	frontendEvents "FlightStrips/pkg/events/frontend"
	pkgModels "FlightStrips/pkg/models"
	"context"
	"fmt"
	"log/slog"
)

type InitialSnapshotRequest struct {
	SessionID int32
	Position  string
	Airport   string
	Callsign  string
	UserCID   string
	ReadOnly  bool
}

type SnapshotBuilderDependencies struct {
	ControllerRepo     repository.ControllerRepository
	StripRepo          repository.StripRepository
	SectorRepo         repository.SectorOwnerRepository
	SessionRepo        repository.SessionRepository
	CoordinationRepo   repository.CoordinationRepository
	TacticalStripRepo  repository.TacticalStripRepository
	EuroscopeHub       shared.EuroscopeHub
	BuildClxContext    func(sessionID int32) clx.Context
	PopulateNextStrips func(ctx context.Context, strips []*internalModels.Strip, sessionID int32)
	LoadMessages       func(sessionID int32) []frontendEvents.MessageReceivedEvent
	LoadCachedAtis     func(sessionID int32) *frontendEvents.AtisUpdateEvent
}

type SnapshotBuilder struct {
	controllerRepo     repository.ControllerRepository
	stripRepo          repository.StripRepository
	sectorRepo         repository.SectorOwnerRepository
	sessionRepo        repository.SessionRepository
	coordinationRepo   repository.CoordinationRepository
	tacticalStripRepo  repository.TacticalStripRepository
	euroscopeHub       shared.EuroscopeHub
	buildClxContext    func(sessionID int32) clx.Context
	populateNextStrips func(ctx context.Context, strips []*internalModels.Strip, sessionID int32)
	loadMessages       func(sessionID int32) []frontendEvents.MessageReceivedEvent
	loadCachedAtis     func(sessionID int32) *frontendEvents.AtisUpdateEvent
}

func NewSnapshotBuilder(deps SnapshotBuilderDependencies) *SnapshotBuilder {
	return &SnapshotBuilder{
		controllerRepo:     deps.ControllerRepo,
		stripRepo:          deps.StripRepo,
		sectorRepo:         deps.SectorRepo,
		sessionRepo:        deps.SessionRepo,
		coordinationRepo:   deps.CoordinationRepo,
		tacticalStripRepo:  deps.TacticalStripRepo,
		euroscopeHub:       deps.EuroscopeHub,
		buildClxContext:    deps.BuildClxContext,
		populateNextStrips: deps.PopulateNextStrips,
		loadMessages:       deps.LoadMessages,
		loadCachedAtis:     deps.LoadCachedAtis,
	}
}

func (b *SnapshotBuilder) Build(ctx context.Context, request InitialSnapshotRequest) (frontendEvents.InitialEvent, *frontendEvents.AtisUpdateEvent, error) {
	if b.controllerRepo == nil || b.stripRepo == nil || b.sectorRepo == nil || b.sessionRepo == nil || b.coordinationRepo == nil {
		return frontendEvents.InitialEvent{}, nil, fmt.Errorf("snapshot builder missing required repository dependencies")
	}

	controllers, err := b.controllerRepo.ListBySession(ctx, request.SessionID)
	if err != nil {
		return frontendEvents.InitialEvent{}, nil, fmt.Errorf("list controllers: %w", err)
	}

	dbSession, err := b.sessionRepo.GetByID(ctx, request.SessionID)
	if err != nil {
		return frontendEvents.InitialEvent{}, nil, fmt.Errorf("get session: %w", err)
	}

	strips, err := b.stripRepo.List(ctx, request.SessionID)
	if err != nil {
		return frontendEvents.InitialEvent{}, nil, fmt.Errorf("list strips: %w", err)
	}

	sectors, err := b.sectorRepo.ListBySession(ctx, request.SessionID)
	if err != nil {
		return frontendEvents.InitialEvent{}, nil, fmt.Errorf("list sectors: %w", err)
	}

	sectorsMap := make(map[string]*internalModels.SectorOwner, len(sectors))
	for _, sector := range sectors {
		sectorsMap[sector.Position] = sector
	}

	controllerModels, me, layout, positionAvailable := b.buildControllerModels(request, controllers, sectorsMap)

	if b.populateNextStrips != nil {
		b.populateNextStrips(ctx, strips, request.SessionID)
	}

	stripModels := make([]frontendEvents.Strip, 0, len(strips))
	clxContext := clx.Context{}
	if b.buildClxContext != nil {
		clxContext = b.buildClxContext(request.SessionID)
	}
	for _, strip := range strips {
		stripModels = append(stripModels, MapStripToFrontendModelWithClx(strip, clxContext))
	}

	if request.ReadOnly {
		me = buildFrontendController(request.Callsign, request.Position, sectorsMap)
	}

	coordinations, err := b.coordinationRepo.ListBySession(ctx, request.SessionID)
	if err != nil {
		return frontendEvents.InitialEvent{}, nil, fmt.Errorf("list coordinations: %w", err)
	}

	coordinationModels := buildCoordinationModels(strips, coordinations)
	tacticalStripModels := b.loadTacticalStrips(ctx, request.SessionID)
	storedMessages := b.snapshotMessages(request.SessionID)
	availableSids := b.loadAvailableSIDs(ctx, request.SessionID)
	initialCFLByRunway := buildInitialCFLByRunway()
	departureMismatch, arrivalMismatch, localIP := b.loadClientRuntimeState(request)

	event := frontendEvents.InitialEvent{
		Contsollers:    controllerModels,
		Strips:         stripModels,
		TacticalStrips: tacticalStripModels,
		Me:             me,
		Callsign:       request.Callsign,
		Airport:        request.Airport,
		Layout:         layout,
		RunwaySetup: frontendEvents.RunwayConfiguration{
			Departure:         cloneStringSlice(dbSession.ActiveRunways.DepartureRunways),
			Arrival:           cloneStringSlice(dbSession.ActiveRunways.ArrivalRunways),
			RunwayStatus:      dbSession.ActiveRunways.RunwayStatus,
			DepartureMismatch: departureMismatch,
			ArrivalMismatch:   arrivalMismatch,
		},
		Coordinations:      coordinationModels,
		Messages:           storedMessages,
		AvailableSids:      availableSids,
		InitialCFLByRunway: initialCFLByRunway,
		TransitionAltitude: int32(config.GetTransitionAltitude()),
		ReadOnly:           request.ReadOnly,
		PositionAvailable:  positionAvailable,
		LocalIP:            localIP,
	}

	return event, b.cachedAtisEvent(request.SessionID), nil
}

func (b *SnapshotBuilder) buildControllerModels(request InitialSnapshotRequest, controllers []*internalModels.Controller, sectorsMap map[string]*internalModels.SectorOwner) ([]frontendEvents.Controller, frontendEvents.Controller, string, bool) {
	controllerModels := make([]frontendEvents.Controller, 0, len(controllers))
	me := frontendEvents.Controller{}
	layout := ""
	positionAvailable := !request.ReadOnly

	for _, controller := range controllers {
		if isObserverController(controller, b.euroscopeHub) {
			continue
		}

		model := buildFrontendController(controller.Callsign, controller.Position, sectorsMap)
		controllerModels = append(controllerModels, model)

		if controller.Position == request.Position {
			positionAvailable = true
		}

		switch {
		case !request.ReadOnly && controller.Callsign == request.Callsign:
			me = model
			if controller.Layout != nil {
				layout = *controller.Layout
			}
		case controller.Position == request.Position && layout == "" && controller.Layout != nil:
			// Dual-login fallback: reuse the layout already stored for the position.
			layout = *controller.Layout
		}
	}

	return controllerModels, me, layout, positionAvailable
}

func buildCoordinationModels(strips []*internalModels.Strip, coordinations []*internalModels.Coordination) []frontendEvents.SyncCoordination {
	stripCallsignByID := make(map[int32]string, len(strips))
	for _, strip := range strips {
		stripCallsignByID[strip.ID] = strip.Callsign
	}

	models := make([]frontendEvents.SyncCoordination, 0, len(coordinations))
	for _, coord := range coordinations {
		callsign, ok := stripCallsignByID[coord.StripID]
		if !ok {
			continue
		}

		models = append(models, frontendEvents.SyncCoordination{
			Callsign:     callsign,
			From:         coord.FromPosition,
			To:           coord.ToPosition,
			IsTagRequest: coord.IsTagRequest,
		})
	}

	return models
}

func (b *SnapshotBuilder) loadTacticalStrips(ctx context.Context, sessionID int32) []frontendEvents.TacticalStripPayload {
	if b.tacticalStripRepo == nil {
		return nil
	}

	tacticalStrips, err := b.tacticalStripRepo.ListBySession(ctx, sessionID)
	if err != nil {
		slog.Error("Failed to list tactical strips", slog.Any("error", err), slog.Int("session", int(sessionID)))
		return nil
	}

	models := make([]frontendEvents.TacticalStripPayload, 0, len(tacticalStrips))
	for _, strip := range tacticalStrips {
		models = append(models, MapTacticalStripToPayload(strip))
	}
	return models
}

func (b *SnapshotBuilder) loadAvailableSIDs(ctx context.Context, sessionID int32) pkgModels.AvailableSids {
	sids, err := b.sessionRepo.GetSessionSids(ctx, sessionID)
	if err != nil {
		slog.Error("Failed to load available SIDs on connect", slog.Any("error", err), slog.Int("session", int(sessionID)))
		return pkgModels.AvailableSids{}
	}
	return sids
}

func buildInitialCFLByRunway() map[string]int32 {
	initialCFLByRunway := make(map[string]int32)
	for runway, cfl := range config.GetInitialCFLByRunway() {
		initialCFLByRunway[runway] = int32(cfl)
	}
	return initialCFLByRunway
}

func (b *SnapshotBuilder) loadClientRuntimeState(request InitialSnapshotRequest) (bool, bool, string) {
	if b.euroscopeHub == nil {
		return false, false, ""
	}

	departureMismatch, arrivalMismatch := b.euroscopeHub.GetRunwayMismatchStatus(request.SessionID, request.UserCID)
	localIP := b.euroscopeHub.GetClientLocalIP(request.SessionID, request.UserCID)
	return departureMismatch, arrivalMismatch, localIP
}

func (b *SnapshotBuilder) snapshotMessages(sessionID int32) []frontendEvents.MessageReceivedEvent {
	if b.loadMessages == nil {
		return nil
	}
	return b.loadMessages(sessionID)
}

func (b *SnapshotBuilder) cachedAtisEvent(sessionID int32) *frontendEvents.AtisUpdateEvent {
	if b.loadCachedAtis == nil {
		return nil
	}
	return b.loadCachedAtis(sessionID)
}

func cloneStringSlice(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}

	result := make([]string, len(values))
	copy(result, values)
	return result
}
