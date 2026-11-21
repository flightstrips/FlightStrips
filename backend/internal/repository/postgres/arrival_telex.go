package postgres

import (
	"context"
	"errors"

	"FlightStrips/internal/arrivalinfo"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// arrivalTelexStore persists automatic arrival telex messages.
type arrivalTelexStore struct{ pool *pgxpool.Pool }

func NewArrivalTelexStore(pool *pgxpool.Pool) arrivalinfo.Store {
	return &arrivalTelexStore{pool: pool}
}

func (r *arrivalTelexStore) ListEpisode(ctx context.Context, session int32, episode string) ([]arrivalinfo.SentMessage, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, session_id, callsign, arrival_episode, kind, message, COALESCE(stand, ''), sent_at
FROM arrival_telex_messages WHERE session_id = $1 AND arrival_episode = $2 ORDER BY sent_at, id`, session, episode)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var messages []arrivalinfo.SentMessage
	for rows.Next() {
		var message arrivalinfo.SentMessage
		var kind string
		if err := rows.Scan(&message.ID, &message.Session, &message.Callsign, &message.Episode, &kind, &message.Text, &message.Stand, &message.SentAt); err != nil {
			return nil, err
		}
		message.Kind = arrivalinfo.Kind(kind)
		message.SentAt = message.SentAt.UTC()
		messages = append(messages, message)
	}
	return messages, rows.Err()
}

func (r *arrivalTelexStore) Reserve(ctx context.Context, message arrivalinfo.SentMessage) (int64, bool, error) {
	var stand *string
	if message.Stand != "" {
		stand = &message.Stand
	}
	var id int64
	err := r.pool.QueryRow(ctx, `INSERT INTO arrival_telex_messages (session_id, callsign, arrival_episode, kind, message, stand, sent_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (session_id, arrival_episode) WHERE kind = 'arrival_info' DO NOTHING
RETURNING id`, message.Session, message.Callsign, message.Episode, string(message.Kind), message.Text, stand, message.SentAt).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return id, true, nil
}

func (r *arrivalTelexStore) Release(ctx context.Context, id int64) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM arrival_telex_messages WHERE id = $1`, id)
	return err
}
