CREATE TABLE IF NOT EXISTS pdc_web_requests (
    id BIGSERIAL PRIMARY KEY,
    session_id INTEGER NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    callsign VARCHAR(255) NOT NULL,
    vatsim_cid VARCHAR(32) NOT NULL,
    atis CHAR(1) NOT NULL,
    stand VARCHAR(64),
    remarks TEXT,
    status VARCHAR(32) NOT NULL,
    clearance_text TEXT,
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_pdc_web_requests_session_callsign_status
    ON pdc_web_requests (session_id, callsign, status);

CREATE INDEX IF NOT EXISTS idx_pdc_web_requests_vatsim_cid
    ON pdc_web_requests (vatsim_cid);
