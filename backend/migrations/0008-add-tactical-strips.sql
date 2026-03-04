CREATE TABLE tactical_strips (
    id          BIGSERIAL PRIMARY KEY,
    session_id  INT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    type        TEXT NOT NULL,
    bay         TEXT NOT NULL,
    label       TEXT NOT NULL,
    aircraft    TEXT,
    produced_by TEXT NOT NULL,
    sequence    INT NOT NULL DEFAULT 0,
    timer_start TIMESTAMPTZ,
    confirmed   BOOLEAN NOT NULL DEFAULT FALSE,
    confirmed_by TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX ON tactical_strips (session_id, bay);
