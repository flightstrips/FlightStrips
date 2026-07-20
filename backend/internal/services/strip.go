package services

import (
	"FlightStrips/internal/repository"
	"FlightStrips/internal/shared"
	"context"
	"log/slog"

	internalModels "FlightStrips/internal/models"
)

const (
	// InitialOrderSpacing is the gap between strips when initially created or after recalculation
	InitialOrderSpacing = 1000
	// MinOrderGap is the minimum gap before recalculation is needed
	MinOrderGap = 5
)

type StripService struct {
	stripReader          StripReader
	lifecycleStore       StripLifecycleStore
	orderingStore        StripOrderingStore
	fieldStore           StripFieldStore
	ownerStore           StripOwnerStore
	cdmStore             StripCdmStore
	validationStore      StripValidationStatusStore
	manualFplStore       StripManualFplStore
	tacticalRepo         repository.TacticalStripRepository
	sectorOwnerRepo      repository.SectorOwnerRepository
	publisher            shared.StripEventPublisher
	esCommander          StripEuroscopeCommander
	coordRepo            CoordinationStore
	controllerRepo       ControllerReader
	sessionRepo          SessionReader
	routeRecalculator    RouteRecalculator
	routeComputer        StripRouteComputer
	routeDisplayComputer StripRouteDisplayComputer
	clearedOwnerResolver ClearedStripOwnerResolver
	pdcService           shared.PdcService
	cdmService           StripCdmService
	departureObserver    departurePositionObserver
}

type departurePositionObserver interface {
	ObserveDeparturePosition(ctx context.Context, session int32, strip *internalModels.Strip, latitude, longitude float64) error
}

func NewStripService(stripReader StripReader, options ...StripServiceOption) *StripService {
	service := &StripService{}
	if stripReader != nil {
		service.stripReader = stripReader
		service.configureStripStores(stripReader)
	}
	for _, option := range options {
		option(service)
	}
	return service
}

func (s *StripService) configureStripStores(store any) {
	if lifecycleStore, ok := store.(StripLifecycleStore); ok {
		s.lifecycleStore = lifecycleStore
	}
	if orderingStore, ok := store.(StripOrderingStore); ok {
		s.orderingStore = orderingStore
	}
	if fieldStore, ok := store.(StripFieldStore); ok {
		s.fieldStore = fieldStore
	}
	if ownerStore, ok := store.(StripOwnerStore); ok {
		s.ownerStore = ownerStore
	}
	if cdmStore, ok := store.(StripCdmStore); ok {
		s.cdmStore = cdmStore
	}
	if validationStore, ok := store.(StripValidationStatusStore); ok {
		s.validationStore = validationStore
	}
	if manualFplStore, ok := store.(StripManualFplStore); ok {
		s.manualFplStore = manualFplStore
	}
}

func (s *StripService) SetFrontendHub(publisher shared.StripEventPublisher) {
	s.publisher = publisher
}

func (s *StripService) SetEuroscopeHub(esCommander StripEuroscopeCommander) {
	s.esCommander = esCommander
}

func (s *StripService) SetSectorOwnerRepo(sectorOwnerRepo repository.SectorOwnerRepository) {
	s.sectorOwnerRepo = sectorOwnerRepo
}

func (s *StripService) SetTacticalStripRepo(tacticalRepo repository.TacticalStripRepository) {
	s.tacticalRepo = tacticalRepo
}

func (s *StripService) SetCoordinationRepo(coordRepo CoordinationStore) {
	s.coordRepo = coordRepo
}

func (s *StripService) SetControllerRepo(controllerRepo ControllerReader) {
	s.controllerRepo = controllerRepo
}

func (s *StripService) SetSessionRepo(sessionRepo SessionReader) {
	s.sessionRepo = sessionRepo
}

func (s *StripService) SetRouteRecalculator(routeRecalculator RouteRecalculator) {
	s.routeRecalculator = routeRecalculator
	if routeComputer, ok := routeRecalculator.(StripRouteComputer); ok {
		s.routeComputer = routeComputer
	}
	if displayComputer, ok := routeRecalculator.(StripRouteDisplayComputer); ok {
		s.routeDisplayComputer = displayComputer
	}
	if resolver, ok := routeRecalculator.(ClearedStripOwnerResolver); ok {
		s.clearedOwnerResolver = resolver
	}
}

func (s *StripService) SetRouteComputer(routeComputer StripRouteComputer) {
	s.routeComputer = routeComputer
}

func (s *StripService) SetPdcService(pdcService shared.PdcService) {
	s.pdcService = pdcService
}

func (s *StripService) getControllerRepository() ControllerReader {
	return s.controllerRepo
}

func (s *StripService) getSessionRepository() SessionReader {
	return s.sessionRepo
}

func (s *StripService) getCoordinationRepository() CoordinationStore {
	return s.coordRepo
}

func (s *StripService) getPdcService() shared.PdcService {
	return s.pdcService
}

func (s *StripService) getRouteRecalculator() RouteRecalculator {
	return s.routeRecalculator
}

func (s *StripService) getRouteComputer() StripRouteComputer {
	return s.routeComputer
}

func (s *StripService) recalculateRouteForStrip(ctx context.Context, session int32, callsign string) error {
	routeRecalculator := s.getRouteRecalculator()
	if routeRecalculator == nil {
		return nil
	}

	return routeRecalculator.UpdateRouteForStripContext(ctx, callsign, session, false)
}

func (s *StripService) SetCdmService(cdmService StripCdmService) {
	s.cdmService = cdmService
}

func (s *StripService) SetDeparturePositionObserver(observer departurePositionObserver) {
	s.departureObserver = observer
}

func (s *StripService) ClearMandatoryRouteCdm(ctx context.Context, sessionID int32, callsign string) {
	strip, err := s.stripReader.GetByCallsign(ctx, sessionID, callsign)
	if err != nil {
		slog.WarnContext(ctx, "Failed to get strip for mandatory route CDM clearing",
			slog.String("callsign", callsign), slog.Any("error", err))
		return
	}

	cdm := strip.CdmData
	if cdm == nil {
		return
	}

	hadMandatoryRoute := false
	for _, r := range cdm.EcfmpRestrictions {
		if r.Type == "mandatory_route" {
			hadMandatoryRoute = true
			break
		}
	}
	if !hadMandatoryRoute {
		return
	}

	cdmUpdated := cdm.Clone()
	filtered := make([]internalModels.EcfmpRestriction, 0, len(cdmUpdated.EcfmpRestrictions))
	for _, r := range cdmUpdated.EcfmpRestrictions {
		if r.Type == "mandatory_route" {
			continue
		}
		filtered = append(filtered, r)
	}
	cdmUpdated.EcfmpRestrictions = filtered
	if len(filtered) == 0 {
		cdmUpdated.EcfmpRestrictions = nil
	}

	if _, err := s.cdmStore.SetCdmData(ctx, sessionID, callsign, cdmUpdated); err != nil {
		slog.WarnContext(ctx, "Failed to clear mandatory route restriction from CDM data",
			slog.String("callsign", callsign), slog.Any("error", err))
		return
	}

	if s.publisher != nil {
		s.publisher.Broadcast(sessionID, shared.BuildFrontendCdmDataEvent(callsign, cdmUpdated))
	}
}

func (s *StripService) queueOrSendStripUpdate(ctx context.Context, session int32, callsign string, publish bool) {
	if publish {
		if s.publisher != nil {
			s.publisher.SendStripUpdate(session, callsign)
		}
		return
	}

	if syncState := shared.GetSyncState(ctx); syncState != nil {
		syncState.MarkStripUpdate(callsign)
	}
}
