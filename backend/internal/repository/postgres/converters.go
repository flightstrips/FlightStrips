package postgres

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// PgTimestamptzToTime converts a nullable PostgreSQL timestamptz to *time.Time.
func PgTimestamptzToTime(ts pgtype.Timestamptz) *time.Time {
	if !ts.Valid {
		return nil
	}
	t := ts.Time
	return &t
}

// TimeToPgTimestamptz converts an optional time.Time to a PostgreSQL
// timestamptz value suitable for sqlc parameters.
func TimeToPgTimestamptz(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{Valid: false}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

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
