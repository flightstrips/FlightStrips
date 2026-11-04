package postgres

import (
	"context"
	"time"

	"FlightStrips/internal/models"
)

// The controller row is a projection of all live memberships sharing this
// session/callsign. Observer activity cannot refresh operational freshness.
func (r *controllerRepository) UpsertMembershipController(ctx context.Context, row *models.Controller, refresh bool) error {
	_, err := r.membershipDB.Exec(ctx, `INSERT INTO controllers(session,callsign,position,observer,cid,last_seen_euroscope)
 VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(session,callsign) DO UPDATE SET
 position=EXCLUDED.position,observer=EXCLUDED.observer,cid=EXCLUDED.cid,
 last_seen_euroscope=CASE WHEN EXCLUDED.observer THEN NULL WHEN $7 THEN EXCLUDED.last_seen_euroscope ELSE controllers.last_seen_euroscope END`, row.Session, row.Callsign, row.Position, row.Observer, row.Cid, row.LastSeenEuroscope, refresh)
	return err
}

// Membership heartbeats/detach use both authenticated CID and exact controller
// row. A second physical instance for that CID cannot refresh a departed row.
func (r *controllerRepository) SetEuroscopeMembershipSeen(ctx context.Context, cid string, session int32, callsign string, seen *time.Time) (int64, error) {
	tag, err := r.membershipDB.Exec(ctx, `UPDATE controllers SET last_seen_euroscope=$1 WHERE cid=$2 AND session=$3 AND callsign=$4`, seen, cid, session, callsign)
	return tag.RowsAffected(), err
}
func (r *controllerRepository) DetachMembership(ctx context.Context, cid string, session int32, callsign string) (int64, error) {
	tag, err := r.membershipDB.Exec(ctx, `UPDATE controllers SET cid=NULL,last_seen_euroscope=NULL WHERE cid=$1 AND session=$2 AND callsign=$3`, cid, session, callsign)
	return tag.RowsAffected(), err
}

func (r *controllerRepository) SetFrontendMembershipSeen(ctx context.Context, cid string, session int32, callsign string, seen *time.Time) (int64, error) {
	tag, err := r.membershipDB.Exec(ctx, `UPDATE controllers SET last_seen_frontend=$1 WHERE cid=$2 AND session=$3 AND callsign=$4`, seen, cid, session, callsign)
	return tag.RowsAffected(), err
}
