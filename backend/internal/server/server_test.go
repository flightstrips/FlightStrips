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
}

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
}
