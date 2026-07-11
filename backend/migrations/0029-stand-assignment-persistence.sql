CREATE TABLE IF NOT EXISTS stand_assignments (
    id BIGSERIAL PRIMARY KEY,
    session_id INTEGER NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    callsign VARCHAR NOT NULL,
    stand VARCHAR NOT NULL,
    direction VARCHAR NOT NULL,
    stage VARCHAR NOT NULL,
    source VARCHAR NOT NULL,
    rule_id VARCHAR,
    tier INTEGER,
    matched_variant VARCHAR,
    eta TIMESTAMPTZ,
    eta_source VARCHAR,
    assigned_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ,
    manual BOOLEAN NOT NULL DEFAULT FALSE,
    acknowledged BOOLEAN NOT NULL DEFAULT FALSE,
    acknowledged_at TIMESTAMPTZ,
    acknowledged_by VARCHAR,
    vatsim_cid BIGINT,
    vatsim_revision BIGINT,
    version INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_stand_assignments_session_callsign
    ON stand_assignments (session_id, callsign);

CREATE INDEX IF NOT EXISTS idx_stand_assignments_session_stand
    ON stand_assignments (session_id, stand);

CREATE TABLE IF NOT EXISTS stand_blocks (
    id BIGSERIAL PRIMARY KEY,
    session_id INTEGER NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    stand VARCHAR NOT NULL,
    block_type VARCHAR NOT NULL,
    source VARCHAR NOT NULL,
    reason VARCHAR,
    callsign VARCHAR,
    created_by VARCHAR,
    expires_at TIMESTAMPTZ,
    manual BOOLEAN NOT NULL DEFAULT TRUE,
    version INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_stand_blocks_session_stand
    ON stand_blocks (session_id, stand);
