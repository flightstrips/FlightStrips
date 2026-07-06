package services

import (
	"FlightStrips/internal/repository"
	"FlightStrips/internal/shared"
	"context"
)

const (
	// InitialOrderSpacing is the gap between strips when initially created or after recalculation
	InitialOrderSpacing = 1000
	// MinOrderGap is the minimum gap before recalculation is needed
	MinOrderGap = 5
)

type StripService struct {
	stripRepo         repository.StripRepository
	tacticalRepo      repository.TacticalStripRepository
	sectorOwnerRepo   repository.SectorOwnerRepository
	publisher         shared.StripEventPublisher
	esCommander       shared.EuroscopeStripCommander
	coordRepo         CoordinationStore
	controllerRepo    ControllerReader
	sessionRepo       SessionReader
	routeRecalculator RouteRecalculator
	routeComputer     StripRouteComputer
	pdcService        shared.PdcService
	cdmService        shared.CdmService
}

func NewStripService(stripRepo repository.StripRepository, options ...StripServiceOption) *StripService {
	service := &StripService{
		stripRepo: stripRepo,
	}
	for _, option := range options {
		option(service)
	}
	return service
}

func (s *StripService) SetFrontendHub(publisher shared.StripEventPublisher) {
	s.publisher = publisher
}

func (s *StripService) SetEuroscopeHub(esCommander shared.EuroscopeStripCommander) {
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

func (s *StripService) SetCdmService(cdmService shared.CdmService) {
	s.cdmService = cdmService
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
