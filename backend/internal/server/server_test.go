package server

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/repository"
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
)

type expiredSessionCdmService struct {
	constructorCdmService
	airport string
	err     error
	removed int32
}

func (s *expiredSessionCdmService) RemoveSession(session int32) { s.removed = session }

func (s *expiredSessionCdmService) DeregisterMasterAirport(_ context.Context, airport string) error {
	s.airport = airport
	return s.err
}

type expiredSessionRepository struct {
	repository.SessionRepository
	deleted int32
}

func (r *expiredSessionRepository) Delete(_ context.Context, id int32) (int64, error) {
	r.deleted = id
	return 1, nil
}

type createSessionRepository struct {
	repository.SessionRepository
	created *models.Session
	id      int32
}

func (r *createSessionRepository) Get(context.Context, string, string) (*models.Session, error) {
	return nil, pgx.ErrNoRows
}

func (r *createSessionRepository) Create(_ context.Context, session *models.Session) (int32, error) {
	r.created = session
	return r.id, nil
}

func TestGetOrCreateSessionCreatesAuthoritativeSession(t *testing.T) {
	sessionRepo := &createSessionRepository{id: 42}
	server := &Server{sessionRepo: sessionRepo}

	session, err := server.GetOrCreateSession("EKCH", "LIVE")
	if err != nil {
		t.Fatalf("GetOrCreateSession returned an error: %v", err)
	}

	if sessionRepo.created == nil {
		t.Fatal("expected the new session to be persisted")
	}
	if session.Id != 42 || session.Name != "LIVE" || session.Airport != "EKCH" {
		t.Fatalf("unexpected returned session: %#v", session)
	}
}

type inboundSessionCdmService struct {
	constructorCdmService
	registered models.Session
}

func (s *inboundSessionCdmService) RegisterInboundSession(id int32, name, airport string) {
	s.registered = models.Session{ID: id, Name: name, Airport: airport}
}

func TestGetOrCreateSessionRegistersInboundMonitoringImmediately(t *testing.T) {
	for _, existing := range []bool{false, true} {
		var repo repository.SessionRepository = &createSessionRepository{id: 42}
		if existing {
			repo = &persistedPlaybackRepository{session: models.Session{ID: 42, Name: "LIVE", Airport: "EKCH"}}
		}
		cdmService := &inboundSessionCdmService{}
		server := &Server{sessionRepo: repo, cdmService: cdmService}
		_, err := server.GetOrCreateSession("EKCH", "LIVE")
		if err != nil {
			t.Fatal(err)
		}
		if cdmService.registered.ID != 42 || cdmService.registered.Airport != "EKCH" || cdmService.registered.Name != "LIVE" {
			t.Fatalf("session was not registered for immediate inbound monitoring: %#v", cdmService.registered)
		}
	}
}

type persistedPlaybackRepository struct {
	repository.SessionRepository
	session models.Session
}

func (r *persistedPlaybackRepository) Get(_ context.Context, name, airport string) (*models.Session, error) {
	if name != r.session.Name || airport != r.session.Airport {
		return nil, pgx.ErrNoRows
	}
	return &r.session, nil
}

func TestGetOrCreateSessionResumesPersistedPlaybackAfterRestart(t *testing.T) {
	sessionRepo := &persistedPlaybackRepository{session: models.Session{
		ID: 42, Name: "PLAYBACK_0123456789ABCDEF0123456789ABCDEF", Airport: "EKCH",
	}}
	for attempt := 0; attempt < 2; attempt++ {
		server := &Server{sessionRepo: sessionRepo}
		session, err := server.GetOrCreateSession("EKCH", sessionRepo.session.Name)
		if err != nil {
			t.Fatal(err)
		}
		if session.Id != 42 || session.Name != sessionRepo.session.Name {
			t.Fatalf("expected persisted playback session, got %#v", session)
		}
	}
}

func TestRemoveExpiredLiveSessionDeregistersMasterBeforeDelete(t *testing.T) {
	cdmService := &expiredSessionCdmService{}
	sessionRepo := &expiredSessionRepository{}
	server := &Server{cdmService: cdmService, sessionRepo: sessionRepo}

	server.removeExpiredSession(context.Background(), &models.Session{ID: 42, Name: "LIVE", Airport: "EKCH"})

	if cdmService.airport != "EKCH" {
		t.Fatalf("deregistered airport = %q, want EKCH", cdmService.airport)
	}
	if sessionRepo.deleted != 42 {
		t.Fatalf("deleted session = %d, want 42", sessionRepo.deleted)
	}
	if cdmService.removed != 42 {
		t.Fatalf("CDM work not cancelled: %d", cdmService.removed)
	}
}
