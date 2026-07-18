CREATE TABLE IF NOT EXISTS aman_airport_states (
    airport VARCHAR PRIMARY KEY,
    revision BIGINT NOT NULL CHECK (revision >= 0),
    generated_at TIMESTAMPTZ NOT NULL,
    policy_version VARCHAR NOT NULL,
    mode VARCHAR NOT NULL,
    authoritative BOOLEAN NOT NULL,
    runway_groups JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS aman_flights (
    flight_id VARCHAR PRIMARY KEY,
    airport VARCHAR NOT NULL REFERENCES aman_airport_states(airport) ON DELETE CASCADE,
    vatsim_cid VARCHAR NOT NULL,
    current_callsign VARCHAR NOT NULL,
    state VARCHAR NOT NULL,
    data_status VARCHAR NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    payload JSONB NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_aman_flights_active_vatsim_cid
    ON aman_flights (vatsim_cid)
    WHERE state <> 'removed';

CREATE INDEX IF NOT EXISTS idx_aman_flights_airport ON aman_flights (airport, flight_id);

CREATE TABLE IF NOT EXISTS aman_command_outcomes (
    command_id VARCHAR PRIMARY KEY,
    airport VARCHAR NOT NULL REFERENCES aman_airport_states(airport) ON DELETE CASCADE,
    revision BIGINT NOT NULL,
    payload JSONB NOT NULL,
    recorded_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS aman_audit_records (
    id BIGSERIAL PRIMARY KEY,
    airport VARCHAR NOT NULL REFERENCES aman_airport_states(airport) ON DELETE CASCADE,
    revision BIGINT NOT NULL,
    category VARCHAR NOT NULL,
    payload JSONB NOT NULL,
    recorded_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_aman_audit_records_airport_revision
    ON aman_audit_records (airport, revision, id);

CREATE TABLE IF NOT EXISTS aman_validation_evidence (
    evidence_id VARCHAR PRIMARY KEY,
    airport VARCHAR NOT NULL REFERENCES aman_airport_states(airport) ON DELETE CASCADE,
    kind VARCHAR NOT NULL,
    payload JSONB NOT NULL,
    recorded_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_aman_validation_evidence_airport
    ON aman_validation_evidence (airport, evidence_id);
