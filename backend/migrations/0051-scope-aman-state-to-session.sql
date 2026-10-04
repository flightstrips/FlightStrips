-- Existing airport-wide queues have no trustworthy session ownership. Retain
-- them for legacy inspection; live sessions start with their own fresh state.
CREATE TABLE aman_session_states (
    session_id INTEGER NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    airport VARCHAR NOT NULL,
    revision BIGINT NOT NULL CHECK (revision >= 0),
    payload JSONB NOT NULL,
    PRIMARY KEY (session_id, airport)
);

CREATE TABLE aman_session_command_outcomes (
    session_id INTEGER NOT NULL,
    airport VARCHAR NOT NULL,
    command_id VARCHAR NOT NULL,
    payload JSONB NOT NULL,
    PRIMARY KEY (session_id, command_id),
    FOREIGN KEY (session_id, airport) REFERENCES aman_session_states(session_id, airport) ON DELETE CASCADE
);

CREATE TABLE aman_session_audit_records (
    id BIGSERIAL PRIMARY KEY,
    session_id INTEGER NOT NULL,
    airport VARCHAR NOT NULL,
    revision BIGINT NOT NULL,
    payload JSONB NOT NULL,
    FOREIGN KEY (session_id, airport) REFERENCES aman_session_states(session_id, airport) ON DELETE CASCADE
);

CREATE TABLE aman_session_validation_evidence (
    session_id INTEGER NOT NULL,
    airport VARCHAR NOT NULL,
    evidence_id VARCHAR NOT NULL,
    payload JSONB NOT NULL,
    PRIMARY KEY (session_id, evidence_id),
    FOREIGN KEY (session_id, airport) REFERENCES aman_session_states(session_id, airport) ON DELETE CASCADE
);

ALTER TABLE aman_coordination_requests ADD COLUMN session_id INTEGER REFERENCES sessions(id) ON DELETE CASCADE;
ALTER TABLE aman_coordination_requests DROP CONSTRAINT aman_coordination_requests_pkey;
ALTER TABLE aman_coordination_requests DROP CONSTRAINT aman_coordination_requests_command_id_key;
CREATE UNIQUE INDEX ux_aman_coordination_session_request ON aman_coordination_requests ((COALESCE(session_id,0)), request_id);
CREATE UNIQUE INDEX ux_aman_coordination_session_command ON aman_coordination_requests ((COALESCE(session_id,0)), command_id);
CREATE INDEX ix_aman_coordination_session_replay ON aman_coordination_requests (session_id, airport, created_at, request_id);
