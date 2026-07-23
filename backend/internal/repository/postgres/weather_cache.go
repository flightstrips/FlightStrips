package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"FlightStrips/internal/aman/predictor/openmeteo"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// amanWeatherCache is a durable, shared cache for provider-derived wind
// samples and the outbound Open-Meteo request budget.
type amanWeatherCache struct{ pool *pgxpool.Pool }

func NewAMANWeatherCache(pool *pgxpool.Pool) *amanWeatherCache { return &amanWeatherCache{pool: pool} }

func (r *amanWeatherCache) Load(ctx context.Context, keys []string) ([]openmeteo.CachedSample, error) {
	if len(keys) == 0 {
		return nil, nil
	}
	rows, err := r.pool.Query(ctx, `SELECT cache_key,levels,observed_at,expires_at FROM aman_weather_cache WHERE cache_key = ANY($1)`, keys)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]openmeteo.CachedSample, 0, len(keys))
	for rows.Next() {
		var value openmeteo.CachedSample
		var levels []byte
		if err := rows.Scan(&value.Key, &levels, &value.ObservedAt, &value.ExpiresAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(levels, &value.Levels); err != nil {
			return nil, fmt.Errorf("decode weather levels for %q: %w", value.Key, err)
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (r *amanWeatherCache) Store(ctx context.Context, values []openmeteo.CachedSample) error {
	for _, value := range values {
		levels, err := json.Marshal(value.Levels)
		if err != nil {
			return err
		}
		if _, err := r.pool.Exec(ctx, `INSERT INTO aman_weather_cache (cache_key,levels,observed_at,expires_at) VALUES ($1,$2,$3,$4) ON CONFLICT (cache_key) DO UPDATE SET levels=EXCLUDED.levels, observed_at=EXCLUDED.observed_at, expires_at=EXCLUDED.expires_at`, value.Key, levels, value.ObservedAt, value.ExpiresAt); err != nil {
			return err
		}
	}
	return nil
}

func (r *amanWeatherCache) ReserveRequest(ctx context.Context, now time.Time) (bool, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('aman-open-meteo-request-budget'))`); err != nil {
		return false, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM aman_weather_request_ledger WHERE requested_at < $1`, now.Add(-24*time.Hour)); err != nil {
		return false, err
	}
	var day, hour, minute int
	if err := tx.QueryRow(ctx, `SELECT count(*) FILTER (WHERE requested_at >= $1), count(*) FILTER (WHERE requested_at >= $2), count(*) FILTER (WHERE requested_at >= $3) FROM aman_weather_request_ledger`, now.Add(-24*time.Hour), now.Add(-time.Hour), now.Add(-time.Minute)).Scan(&day, &hour, &minute); err != nil {
		return false, err
	}
	if day >= 9000 || hour >= 4000 || minute >= 500 {
		return false, tx.Commit(ctx)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO aman_weather_request_ledger (requested_at) VALUES ($1)`, now); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}

var _ openmeteo.PersistentCache = (*amanWeatherCache)(nil)
