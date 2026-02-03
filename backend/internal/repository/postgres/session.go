package postgres

import (
	"FlightStrips/internal/database"
	"FlightStrips/internal/models"
	pkgModels "FlightStrips/pkg/models"
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type sessionRepository struct {
	queries *database.Queries
}

// NewSessionRepository creates a new SessionRepository implementation
func NewSessionRepository(db *pgxpool.Pool) *sessionRepository {
	return &sessionRepository{
		queries: database.New(db),
	}
}

// sessionToModel converts database.Session to models.Session
func sessionToModel(db database.Session) *models.Session {
	return &models.Session{
		ID:                 db.ID,
		Name:               db.Name,
		Airport:            db.Airport,
		ActiveRunways:      db.ActiveRunways,
		PdcSequence:        db.PdcSequence,
		PdcMessageSequence: db.PdcMessageSequence,
	}
}

// Create inserts a new session
func (r *sessionRepository) Create(ctx context.Context, session *models.Session) (int32, error) {
	return r.queries.InsertSession(ctx, database.InsertSessionParams{
		Name:    session.Name,
		Airport: session.Airport,
	})
}

// GetByID retrieves a session by ID
func (r *sessionRepository) GetByID(ctx context.Context, id int32) (*models.Session, error) {
	dbSession, err := r.queries.GetSessionById(ctx, id)
	if err != nil {
		return nil, err
	}
	return sessionToModel(dbSession), nil
}

// Get retrieves a session by name and airport (alias for GetByNameAndAirport)
func (r *sessionRepository) Get(ctx context.Context, name string, airport string) (*models.Session, error) {
	return r.GetByNameAndAirport(ctx, name, airport)
}

// GetByNameAndAirport retrieves a session by name and airport
func (r *sessionRepository) GetByNameAndAirport(ctx context.Context, name string, airport string) (*models.Session, error) {
	dbSession, err := r.queries.GetSession(ctx, database.GetSessionParams{
		Airport: airport,
		Name:    name,
	})
	if err != nil {
		return nil, err
	}
	return sessionToModel(dbSession), nil
}

// GetByNames retrieves sessions by name
func (r *sessionRepository) GetByNames(ctx context.Context, name string) ([]*models.Session, error) {
	dbSessions, err := r.queries.GetSessionsByNames(ctx, name)
	if err != nil {
		return nil, err
	}

	sessions := make([]*models.Session, len(dbSessions))
	for i, dbSession := range dbSessions {
		sessions[i] = sessionToModel(dbSession)
	}
	return sessions, nil
}

// GetExpiredSessions retrieves sessions that have expired
func (r *sessionRepository) GetExpiredSessions(ctx context.Context, expiredBefore *time.Time) ([]*models.Session, error) {
	sessionIds, err := r.queries.GetExpiredSessions(ctx, TimeToPgTimestamp(expiredBefore))
	if err != nil {
		return nil, err
	}

	sessions := make([]*models.Session, len(sessionIds))
	for i, sessionId := range sessionIds {
		session, err := r.GetByID(ctx, sessionId)
		if err != nil {
			return nil, err
		}
		sessions[i] = session
	}
	return sessions, nil
}

// List retrieves all sessions
func (r *sessionRepository) List(ctx context.Context) ([]*models.Session, error) {
	dbSessions, err := r.queries.GetSessions(ctx)
	if err != nil {
		return nil, err
	}

	sessions := make([]*models.Session, len(dbSessions))
	for i, dbSession := range dbSessions {
		sessions[i] = sessionToModel(dbSession)
	}
	return sessions, nil
}

// Delete removes a session by ID
func (r *sessionRepository) Delete(ctx context.Context, id int32) (int64, error) {
	return r.queries.DeleteSession(ctx, id)
}

// UpdateActiveRunways updates the active runways for a session
func (r *sessionRepository) UpdateActiveRunways(ctx context.Context, id int32, activeRunways pkgModels.ActiveRunways) error {
	return r.queries.UpdateActiveRunways(ctx, database.UpdateActiveRunwaysParams{
		ID:            id,
		ActiveRunways: activeRunways,
	})
}

// IncrementPdcSequence increments and returns the PDC sequence
func (r *sessionRepository) IncrementPdcSequence(ctx context.Context, id int32) (int32, error) {
	return r.queries.GetNextPdcSequence(ctx, id)

}

// IncrementPdcMessageSequence increments and returns the PDC message sequence
func (r *sessionRepository) IncrementPdcMessageSequence(ctx context.Context, id int32) (int32, error) {
	return r.queries.GetNextMessageSequence(ctx, id)
}
