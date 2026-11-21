-- Automatic Hoppie telex messages sent to arriving aircraft. Each row is
-- written before the telex is sent and removed again if Hoppie rejects it, so
-- a restart can never send the same arrival information twice.
CREATE TABLE arrival_telex_messages (
    id BIGSERIAL PRIMARY KEY,
    session_id INTEGER NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    callsign TEXT NOT NULL,
    -- The arriving strip's id. It survives a go-around but not a new flight.
    arrival_episode BIGINT NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('arrival_info', 'stand_update')),
    message TEXT NOT NULL,
    stand TEXT,
    sent_at TIMESTAMPTZ NOT NULL
);

CREATE UNIQUE INDEX ux_arrival_telex_messages_arrival_info
    ON arrival_telex_messages (session_id, arrival_episode)
    WHERE kind = 'arrival_info';

CREATE INDEX ix_arrival_telex_messages_episode
    ON arrival_telex_messages (session_id, arrival_episode, sent_at, id);
