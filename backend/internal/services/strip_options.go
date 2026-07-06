package services

import (
	"FlightStrips/internal/repository"
	"FlightStrips/internal/shared"
)

type StripServiceOption func(*StripService)

func WithStripEventPublisher(publisher shared.StripEventPublisher) StripServiceOption {
	return func(s *StripService) {
		s.publisher = publisher
	}
}

func WithEuroscopeCommander(esCommander shared.EuroscopeStripCommander) StripServiceOption {
	return func(s *StripService) {
		s.esCommander = esCommander
	}
}

func WithSectorOwnerRepository(sectorOwnerRepo repository.SectorOwnerRepository) StripServiceOption {
	return func(s *StripService) {
		s.sectorOwnerRepo = sectorOwnerRepo
	}
}

func WithTacticalStripRepository(tacticalRepo repository.TacticalStripRepository) StripServiceOption {
	return func(s *StripService) {
		s.tacticalRepo = tacticalRepo
	}
}

func WithCoordinationStore(coordStore CoordinationStore) StripServiceOption {
	return func(s *StripService) {
		s.coordRepo = coordStore
	}
}

func WithControllerReader(controllerReader ControllerReader) StripServiceOption {
	return func(s *StripService) {
		s.controllerRepo = controllerReader
	}
}

func WithSessionReader(sessionReader SessionReader) StripServiceOption {
	return func(s *StripService) {
		s.sessionRepo = sessionReader
	}
}

func WithRouteRecalculator(routeRecalculator RouteRecalculator) StripServiceOption {
	return func(s *StripService) {
		s.routeRecalculator = routeRecalculator
		if routeComputer, ok := routeRecalculator.(StripRouteComputer); ok {
			s.routeComputer = routeComputer
		}
	}
}

func WithRouteComputer(routeComputer StripRouteComputer) StripServiceOption {
	return func(s *StripService) {
		s.routeComputer = routeComputer
	}
}

func WithPdcService(pdcService shared.PdcService) StripServiceOption {
	return func(s *StripService) {
		s.pdcService = pdcService
	}
}

func WithCdmService(cdmService shared.CdmService) StripServiceOption {
	return func(s *StripService) {
		s.cdmService = cdmService
	}
}
