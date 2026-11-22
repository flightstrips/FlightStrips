-- Last time the VATSIM feed showed a departure's pilot online. Unlike
-- strips.vatsim_seen_at, offline prefiles never refresh it, so it can serve as
-- the feed half of the departure-block heartbeat.
CREATE TABLE departure_online_sightings (
    strip_id INTEGER PRIMARY KEY REFERENCES strips(id) ON DELETE CASCADE,
    seen_at TIMESTAMPTZ NOT NULL
);
