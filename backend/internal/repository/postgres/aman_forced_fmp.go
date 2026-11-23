package postgres

import (
	"context"
	"errors"

	"FlightStrips/internal/aman"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type amanForcedFMPStore struct {
	pool *pgxpool.Pool
}

// NewAMANForcedFMPStore persists forced FMP grants per session.
func NewAMANForcedFMPStore(pool *pgxpool.Pool) aman.ForcedFMPStore {
	return &amanForcedFMPStore{pool: pool}
}

func (s *amanForcedFMPStore) LoadForcedFMP(ctx context.Context, sessionID int32) (aman.ForcedFMPGrant, bool, error) {
	grant := aman.ForcedFMPGrant{SessionID: sessionID}
	err := s.pool.QueryRow(ctx, `SELECT airport, callsign, cid, granted_at FROM aman_forced_fmp WHERE session_id=$1`, sessionID).
		Scan(&grant.Airport, &grant.Callsign, &grant.CID, &grant.GrantedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return aman.ForcedFMPGrant{}, false, nil
	}
	if err != nil {
		return aman.ForcedFMPGrant{}, false, err
	}
	grant.GrantedAt = grant.GrantedAt.UTC()
	return grant, true, nil
}

func (s *amanForcedFMPStore) SaveForcedFMP(ctx context.Context, grant aman.ForcedFMPGrant) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO aman_forced_fmp (session_id, airport, callsign, cid, granted_at)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (session_id) DO UPDATE SET airport=EXCLUDED.airport, callsign=EXCLUDED.callsign, cid=EXCLUDED.cid, granted_at=EXCLUDED.granted_at`,
		grant.SessionID, grant.Airport, grant.Callsign, grant.CID, grant.GrantedAt.UTC())
	return err
}

func (s *amanForcedFMPStore) ClearForcedFMP(ctx context.Context, sessionID int32) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM aman_forced_fmp WHERE session_id=$1`, sessionID)
	return err
}
