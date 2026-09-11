CREATE TABLE IF NOT EXISTS aman_coordination_requests (
    request_id TEXT PRIMARY KEY,
    airport TEXT NOT NULL,
    command_id TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL,
    payload JSONB NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_aman_coordination_requests_airport_replay
    ON aman_coordination_requests (airport, created_at, request_id);
