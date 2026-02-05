package postgres

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// PgTimestampToTime converts pgtype.Timestamp to *time.Time
func PgTimestampToTime(ts pgtype.Timestamp) *time.Time {
	if !ts.Valid {
		return nil
	}
	return &ts.Time
}

// TimeToPgTimestamp converts *time.Time to pgtype.Timestamp
func TimeToPgTimestamp(t *time.Time) pgtype.Timestamp {
	if t == nil {
		return pgtype.Timestamp{Valid: false}
	}
	return pgtype.Timestamp{Time: *t, Valid: true}
}
