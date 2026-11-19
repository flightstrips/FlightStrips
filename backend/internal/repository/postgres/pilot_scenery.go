package postgres

import (
	"context"
	"errors"
	"strings"

	"FlightStrips/internal/database"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PilotSceneryRepository stores the airport scenery each pilot declared in the EFB.
type PilotSceneryRepository struct {
	db database.DBTX
}

func NewPilotSceneryRepository(db *pgxpool.Pool) *PilotSceneryRepository {
	return &PilotSceneryRepository{db: db}
}

// PilotScenery returns the pilot's declared scenery at the airport, or "" when
// they have not declared one.
func (r *PilotSceneryRepository) PilotScenery(ctx context.Context, cid int64, airport string) (string, error) {
	var value string
	err := r.db.QueryRow(ctx, `SELECT scenery FROM pilot_sceneries WHERE cid = $1 AND airport = $2`,
		cid, strings.ToUpper(strings.TrimSpace(airport))).Scan(&value)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return value, err
}

// SetPilotScenery records the pilot's declared scenery at the airport.
func (r *PilotSceneryRepository) SetPilotScenery(ctx context.Context, cid int64, airport, scenery string) error {
	_, err := r.db.Exec(ctx, `
INSERT INTO pilot_sceneries (cid, airport, scenery, updated_at)
VALUES ($1, $2, $3, now())
ON CONFLICT (cid, airport) DO UPDATE SET scenery = EXCLUDED.scenery, updated_at = EXCLUDED.updated_at`,
		cid, strings.ToUpper(strings.TrimSpace(airport)), strings.ToLower(strings.TrimSpace(scenery)))
	return err
}
