CREATE TABLE efb_departure_stands (
    session_id INTEGER NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    callsign VARCHAR NOT NULL,
    state JSONB NOT NULL,
    PRIMARY KEY (session_id, callsign)
);
