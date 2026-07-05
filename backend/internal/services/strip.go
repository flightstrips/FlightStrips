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
	stripRepo       repository.StripRepository
	tacticalRepo    repository.TacticalStripRepository
	sectorOwnerRepo repository.SectorOwnerRepository
	publisher       shared.StripEventPublisher
	esCommander     shared.EuroscopeStripCommander
	coordRepo       repository.CoordinationRepository
	controllerRepo  repository.ControllerRepository
	cdmService      shared.CdmService
}

func NewStripService(stripRepo repository.StripRepository) *StripService {
	return &StripService{
		stripRepo: stripRepo,
	}
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

func (s *StripService) SetCoordinationRepo(coordRepo repository.CoordinationRepository) {
	s.coordRepo = coordRepo
}

func (s *StripService) SetControllerRepo(controllerRepo repository.ControllerRepository) {
	s.controllerRepo = controllerRepo
}

func (s *StripService) getControllerRepository() repository.ControllerRepository {
	if s.controllerRepo != nil {
		return s.controllerRepo
	}
	if s.publisher == nil {
		return nil
	}
	server := s.publisher.GetServer()
	if server == nil {
		return nil
	}
	return server.GetControllerRepository()
}

func (s *StripService) getSessionRepository() repository.SessionRepository {
	if s.publisher == nil {
		return nil
	}
	server := s.publisher.GetServer()
	if server == nil {
		return nil
	}
	return server.GetSessionRepository()
}

func (s *StripService) getServer() shared.Server {
	if s.publisher == nil {
		return nil
	}
	return s.publisher.GetServer()
}

func (s *StripService) getCoordinationRepository() repository.CoordinationRepository {
	if s.coordRepo != nil {
		return s.coordRepo
	}
	server := s.getServer()
	if server == nil {
		return nil
	}
	return server.GetCoordinationRepository()
}

func (s *StripService) getPdcService() shared.PdcService {
	server := s.getServer()
	if server == nil {
		return nil
	}
	return server.GetPdcService()
}

func (s *StripService) recalculateRouteForStrip(ctx context.Context, session int32, callsign string) error {
	server := s.getServer()
	if server == nil {
		return nil
	}

	return server.UpdateRouteForStripContext(ctx, callsign, session, false)
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
