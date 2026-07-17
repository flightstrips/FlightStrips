package server

import (
	"FlightStrips/internal/dependencies"
	"FlightStrips/internal/models"
	"FlightStrips/internal/repository"
	"FlightStrips/internal/shared"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Server struct {
	dbPool             *pgxpool.Pool
	euroscopeHub       shared.EuroscopeHub
	frontendHub        shared.FrontendHub
	cdmService         shared.CdmService
	frequencyProviders []TransceiverLookup
	sessionLocks       sessionRecalcLockManager

	// Repositories
	stripRepo           repository.StripRepository
	controllerRepo      repository.ControllerRepository
	sessionRepo         repository.SessionRepository
	sectorRepo          repository.SectorOwnerRepository
	coordRepo           repository.CoordinationRepository
	tacticalStripRepo   repository.TacticalStripRepository
	standAssignmentRepo repository.StandAssignmentRepository
}

type TransceiverLookup interface {
	GetFrequencies(callsign string) []string
}

type Dependencies struct {
	DBPool             *pgxpool.Pool
	Euroscope          shared.EuroscopeHub
	Frontend           shared.FrontendHub
	CDM                shared.CdmService
	FrequencyProviders []TransceiverLookup
	Strips             repository.StripRepository
	Controllers        repository.ControllerRepository
	Sessions           repository.SessionRepository
	Sectors            repository.SectorOwnerRepository
	Coordinations      repository.CoordinationRepository
	TacticalStrips     repository.TacticalStripRepository
	StandAssignments   repository.StandAssignmentRepository
}

func NewServer(deps Dependencies) (*Server, error) {
	required := []struct {
		name  string
		value any
	}{
		{"database pool", deps.DBPool},
		{"EuroScope hub", deps.Euroscope},
		{"frontend hub", deps.Frontend},
		{"CDM service", deps.CDM},
		{"strip repository", deps.Strips},
		{"controller repository", deps.Controllers},
		{"session repository", deps.Sessions},
		{"sector repository", deps.Sectors},
		{"coordination repository", deps.Coordinations},
		{"tactical strip repository", deps.TacticalStrips},
	}
	for _, dependency := range required {
		if dependencies.IsNil(dependency.value) {
			return nil, fmt.Errorf("server requires %s", dependency.name)
		}
	}
	for i, provider := range deps.FrequencyProviders {
		if dependencies.IsNil(provider) {
			return nil, fmt.Errorf("server frequency provider %d is nil", i)
		}
	}

	return &Server{
		dbPool:              deps.DBPool,
		euroscopeHub:        deps.Euroscope,
		frontendHub:         deps.Frontend,
		cdmService:          deps.CDM,
		frequencyProviders:  append([]TransceiverLookup(nil), deps.FrequencyProviders...),
		stripRepo:           deps.Strips,
		controllerRepo:      deps.Controllers,
		sessionRepo:         deps.Sessions,
		sectorRepo:          deps.Sectors,
		coordRepo:           deps.Coordinations,
		tacticalStripRepo:   deps.TacticalStrips,
		standAssignmentRepo: deps.StandAssignments,
	}, nil
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

func (s *Server) GetTacticalStripRepository() repository.TacticalStripRepository {
	return s.tacticalStripRepo
}

func (s *Server) GetStandAssignmentRepository() repository.StandAssignmentRepository {
	return s.standAssignmentRepo
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

func (s *Server) GetOrCreateSession(airport string, name string) (shared.Session, error) {
	sessionRepo := s.sessionRepo

	session, err := sessionRepo.Get(context.Background(), name, airport)

	if err == nil {
		return shared.Session{Name: session.Name, Airport: session.Airport, Id: session.ID}, nil
	}

	if errors.Is(err, pgx.ErrNoRows) {
		slog.Debug("Creating session", slog.String("name", name), slog.String("airport", airport))
		newSession := &models.Session{Name: name, Airport: airport, CdmMaster: true}
		id, err := sessionRepo.Create(context.Background(), newSession)

		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				session, err = sessionRepo.Get(context.Background(), name, airport)
				if err == nil {
					return shared.Session{Name: session.Name, Airport: session.Airport, Id: session.ID}, nil
				}
			}

			return shared.Session{}, err

		}

		if err := s.cdmService.SetSessionCdmMaster(context.Background(), id, true); err != nil {
			return shared.Session{}, fmt.Errorf("initialize CDM master for new session %d: %w", id, err)
		}

		return shared.Session{Name: name, Airport: airport, Id: id}, nil
	}

	return shared.Session{}, nil
}

func (s *Server) StartSessionMonitor(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		expired := time.Now().Add(-time.Minute * 5).UTC()
		sessions, err := s.sessionRepo.GetExpiredSessions(ctx, &expired)

		if err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.ErrorContext(ctx, "Failed to get expired sessions", slog.Any("error", err))
		}

		for _, session := range sessions {
			slog.InfoContext(ctx, "Removing expired session", slog.Int("session", int(session.ID)))

			if session.CdmMaster {
				if err := s.cdmService.SetSessionCdmMaster(ctx, session.ID, false); err != nil {
					slog.ErrorContext(ctx, "Failed to deregister CDM master for expired session", slog.Int("session", int(session.ID)), slog.Any("error", err))
				}
			}

			count, err := s.sessionRepo.Delete(ctx, session.ID)
			if err != nil {
				slog.ErrorContext(ctx, "Failed to remove expired session", slog.Int("session", int(session.ID)), slog.Any("error", err))
			}

			if count != 1 {
				slog.WarnContext(ctx, "Failed to remove expired session (no changes)", slog.Int("session", int(session.ID)))
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
