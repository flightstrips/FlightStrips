package postgres

import (
	"context"
	"time"
)

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
