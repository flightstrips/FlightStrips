package server

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/pdc"
	"FlightStrips/internal/repository"
	"FlightStrips/internal/shared"
	"context"
	"errors"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Server struct {
	dbPool       *pgxpool.Pool
	euroscopeHub shared.EuroscopeHub
	frontendHub  shared.FrontendHub
	cdmService   shared.CdmService
	pdcService   *pdc.Service
	
	// Repositories
	stripRepo      repository.StripRepository
	controllerRepo repository.ControllerRepository
	sessionRepo    repository.SessionRepository
	sectorRepo     repository.SectorOwnerRepository
	coordRepo      repository.CoordinationRepository
}

func NewServer(
	dbPool *pgxpool.Pool,
	euroscopeHub shared.EuroscopeHub,
	frontendHub shared.FrontendHub,
	cdmService shared.CdmService,
	pdcService *pdc.Service,
	stripRepo repository.StripRepository,
	controllerRepo repository.ControllerRepository,
	sessionRepo repository.SessionRepository,
	sectorRepo repository.SectorOwnerRepository,
	coordRepo repository.CoordinationRepository,
) *Server {
	server := Server{
		dbPool:         dbPool,
		euroscopeHub:   euroscopeHub,
		frontendHub:    frontendHub,
		cdmService:     cdmService,
		pdcService:     pdcService,
		stripRepo:      stripRepo,
		controllerRepo: controllerRepo,
		sessionRepo:    sessionRepo,
		sectorRepo:     sectorRepo,
		coordRepo:      coordRepo,
	}

	go server.monitorSessions()

	return &server
}

func (s *Server) GetDatabasePool() *pgxpool.Pool {
	return s.dbPool
}

func (s *Server) GetStripRepository() repository.StripRepository {
	return s.stripRepo
}

func (s *Server) GetControllerRepository() repository.ControllerRepository {
	return s.controllerRepo
}

func (s *Server) GetSessionRepository() repository.SessionRepository {
	return s.sessionRepo
}

func (s *Server) GetSectorOwnerRepository() repository.SectorOwnerRepository {
	return s.sectorRepo
}

func (s *Server) GetCoordinationRepository() repository.CoordinationRepository {
	return s.coordRepo
}

func (s *Server) GetEuroscopeHub() shared.EuroscopeHub {
	return s.euroscopeHub
}

func (s *Server) GetFrontendHub() shared.FrontendHub {
	return s.frontendHub
}

func (s *Server) GetCdmService() shared.CdmService {
	return s.cdmService
}

func (s *Server) GetPdcService() shared.PdcService {
	return s.pdcService
}

func (s *Server) GetOrCreateSession(airport string, name string) (shared.Session, error) {
	sessionRepo := s.sessionRepo

	session, err := sessionRepo.Get(context.Background(), name, airport)

	if err == nil {
		return shared.Session{Name: session.Name, Airport: session.Airport, Id: session.ID}, nil
	}

	if errors.Is(err, pgx.ErrNoRows) {
		log.Println("Creating session:", name, "for airport:", airport)
		newSession := &models.Session{Name: name, Airport: airport}
		id, err := sessionRepo.Create(context.Background(), newSession)

		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				session, err = sessionRepo.Get(context.Background(), name, airport)
				if err != nil {
					return shared.Session{Name: name, Airport: airport, Id: id}, err
				}
			}

			return shared.Session{}, err

		}

		return shared.Session{Name: name, Airport: airport, Id: id}, err
	}

	return shared.Session{}, nil
}

func (s *Server) monitorSessions() {
	for {
		expired := time.Now().Add(-time.Minute * 5).UTC()
		sessionRepo := s.sessionRepo

		sessions, err := sessionRepo.GetExpiredSessions(context.Background(), &expired)

		if err != nil {
			log.Println("Failed to get expired sessions:", err)
		}

		for _, session := range sessions {
			log.Println("Removing expired session:", session)
			count, err := sessionRepo.Delete(context.Background(), session.ID)
			if err != nil {
				log.Println("Failed to remove expired session:", session, err)
			}

			if count != 1 {
				log.Println("Failed to remove expired session (no changes):", session, err)
			}
		}

		time.Sleep(time.Minute)
	}
}
