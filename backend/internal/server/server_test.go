package server

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/repository"
	"FlightStrips/internal/shared"
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
)

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

type recordingCdmMasterService struct {
	shared.CdmService
	sessionID int32
	master    bool
	called    bool
}

func (s *recordingCdmMasterService) SetSessionCdmMaster(_ context.Context, sessionID int32, master bool) error {
	s.sessionID = sessionID
	s.master = master
	s.called = true
	return nil
}

func TestGetOrCreateSessionInitializesNewSessionAsCdmMaster(t *testing.T) {
	sessionRepo := &createSessionRepository{id: 42}
	cdmService := &recordingCdmMasterService{}
	server := &Server{sessionRepo: sessionRepo, cdmService: cdmService}

	session, err := server.GetOrCreateSession("EKCH", "LIVE")
	if err != nil {
		t.Fatalf("GetOrCreateSession returned an error: %v", err)
	}

	if sessionRepo.created == nil || !sessionRepo.created.CdmMaster {
		t.Fatalf("expected the new session to be persisted as CDM master, got %#v", sessionRepo.created)
	}
	if !cdmService.called || cdmService.sessionID != 42 || !cdmService.master {
		t.Fatalf("expected CDM master initialization for session 42, got called=%t session=%d master=%t", cdmService.called, cdmService.sessionID, cdmService.master)
	}
	if session.Id != 42 || session.Name != "LIVE" || session.Airport != "EKCH" {
		t.Fatalf("unexpected returned session: %#v", session)
	}
}
