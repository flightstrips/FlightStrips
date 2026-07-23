-- Durable Open-Meteo cache and quota ledger. These are runtime-provider
-- artifacts, deliberately separate from canonical navigation data.
CREATE TABLE IF NOT EXISTS aman_weather_cache (
    cache_key VARCHAR PRIMARY KEY,
    levels JSONB NOT NULL,
    observed_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    CHECK (expires_at >= observed_at)
);

CREATE TABLE IF NOT EXISTS aman_weather_request_ledger (
    request_id BIGSERIAL PRIMARY KEY,
    requested_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_aman_weather_request_ledger_requested_at
    ON aman_weather_request_ledger (requested_at);
