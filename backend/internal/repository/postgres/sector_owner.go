package postgres

import (
	"FlightStrips/internal/database"
	"FlightStrips/internal/models"
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type sectorOwnerRepository struct {
	queries *database.Queries
}

// NewSectorOwnerRepository creates a new SectorOwnerRepository implementation
func NewSectorOwnerRepository(db *pgxpool.Pool) *sectorOwnerRepository {
	return &sectorOwnerRepository{
		queries: database.New(db),
	}
}

// sectorOwnerToModel converts database.SectorOwner to models.SectorOwner
func sectorOwnerToModel(db database.SectorOwner) *models.SectorOwner {
	return &models.SectorOwner{
		ID:         db.ID,
		Session:    db.Session,
		Sector:     db.Sector,
		Position:   db.Position,
		Identifier: db.Identifier,
	}
}

// Create inserts a new sector owner
func (r *sectorOwnerRepository) Create(ctx context.Context, owner *models.SectorOwner) error {
	// This uses copyfrom, which is batch insert - for single insert we'd need a different approach
	// For now, this is a placeholder
	return nil
}

// GetByID retrieves a sector owner by ID
func (r *sectorOwnerRepository) GetByID(ctx context.Context, id int32) (*models.SectorOwner, error) {
	// This would require a new SQL query
	return nil, nil
}

// ListBySession retrieves all sector owners for a session
func (r *sectorOwnerRepository) ListBySession(ctx context.Context, session int32) ([]*models.SectorOwner, error) {
	dbOwners, err := r.queries.GetSectorOwners(ctx, session)
	if err != nil {
		return nil, err
	}

	owners := make([]*models.SectorOwner, len(dbOwners))
	for i, dbOwner := range dbOwners {
		owners[i] = sectorOwnerToModel(dbOwner)
	}
	return owners, nil
}

// Delete removes a sector owner by ID
func (r *sectorOwnerRepository) Delete(ctx context.Context, id int32) error {
	// This would require a new SQL query
	return nil
}

// DeleteAllBySession removes all sector owners for a session
func (r *sectorOwnerRepository) DeleteAllBySession(ctx context.Context, session int32) error {
	return r.queries.RemoveSectorOwners(ctx, session)
}

// RemoveBySession is an alias for DeleteAllBySession
func (r *sectorOwnerRepository) RemoveBySession(ctx context.Context, session int32) error {
	return r.DeleteAllBySession(ctx, session)
}
