package postgres

import (
	"FlightStrips/internal/database"
	"FlightStrips/internal/models"
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type coordinationRepository struct {
	queries *database.Queries
}

// NewCoordinationRepository creates a new CoordinationRepository implementation
func NewCoordinationRepository(db *pgxpool.Pool) *coordinationRepository {
	return &coordinationRepository{
		queries: database.New(db),
	}
}

// coordinationToModel converts database.Coordination to models.Coordination
func coordinationToModel(db database.Coordination) *models.Coordination {
	return &models.Coordination{
		ID:            db.ID,
		Session:       db.Session,
		StripID:       db.StripID,
		FromPosition:  db.FromPosition,
		ToPosition:    db.ToPosition,
		CoordinatedAt: PgTimestampToTime(db.CoordinatedAt),
	}
}

// Create inserts a new coordination
func (r *coordinationRepository) Create(ctx context.Context, coordination *models.Coordination) error {
	_, err := r.queries.CreateCoordination(ctx, database.CreateCoordinationParams{
		Session:      coordination.Session,
		StripID:      coordination.StripID,
		FromPosition: coordination.FromPosition,
		ToPosition:   coordination.ToPosition,
	})
	return err
}

// GetByID retrieves a coordination by ID
func (r *coordinationRepository) GetByID(ctx context.Context, id int32) (*models.Coordination, error) {
	// This would require a new SQL query
	return nil, nil
}

// GetByStripID retrieves a coordination for a strip by stripID
func (r *coordinationRepository) GetByStripID(ctx context.Context, session int32, stripID int32) (*models.Coordination, error) {
	dbCoordination, err := r.queries.GetCoordinationByStripID(ctx, database.GetCoordinationByStripIDParams{
		Session: session,
		StripID: stripID,
	})
	if err != nil {
		return nil, err
	}
	return coordinationToModel(dbCoordination), nil
}

// GetByStripCallsign retrieves a coordination by strip callsign
func (r *coordinationRepository) GetByStripCallsign(ctx context.Context, session int32, callsign string) (*models.Coordination, error) {
	dbCoordination, err := r.queries.GetCoordinationByStripCallsign(ctx, database.GetCoordinationByStripCallsignParams{
		Session:  session,
		Callsign: callsign,
	})
	if err != nil {
		return nil, err
	}
	return coordinationToModel(dbCoordination), nil
}

// ListBySession retrieves all coordinations for a session
func (r *coordinationRepository) ListBySession(ctx context.Context, session int32) ([]*models.Coordination, error) {
	dbCoordinations, err := r.queries.ListCoordinationsBySession(ctx, session)
	if err != nil {
		return nil, err
	}

	coordinations := make([]*models.Coordination, len(dbCoordinations))
	for i, dbCoordination := range dbCoordinations {
		coordinations[i] = coordinationToModel(dbCoordination)
	}
	return coordinations, nil
}

// ListByStrip retrieves all coordinations for a strip
func (r *coordinationRepository) ListByStrip(ctx context.Context, session int32, stripID int32) ([]*models.Coordination, error) {
	dbCoordination, err := r.queries.GetCoordinationByStripID(ctx, database.GetCoordinationByStripIDParams{
		Session: session,
		StripID: stripID,
	})
	if err != nil {
		return nil, err
	}

	return []*models.Coordination{coordinationToModel(dbCoordination)}, nil
}

// Delete removes a coordination by ID
func (r *coordinationRepository) Delete(ctx context.Context, id int32) error {
	_, err := r.queries.DeleteCoordinationByID(ctx, id)
	return err
}
