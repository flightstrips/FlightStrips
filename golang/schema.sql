CREATE TABLE IF NOT EXISTS controllers (
  cid text PRIMARY KEY,
  airport text,
  position text
);

CREATE TABLE IF NOT EXISTS events (
    id text PRIMARY KEY,
    type text,
    timestamp text,
    cid text,
    data text
);

CREATE TABLE IF NOT EXISTS strips (
    id text PRIMARY KEY,
    origin text,
    destination text,
    alternative text,
    route text,
    remarks text,
    assigned_squawk text,
    squawk text,
    sid text,
    cleared_altitude text,
    heading text,
    aircraft_type text,
    runway text,
    requested_altitude text,
    capabilities text,
    communication_type text,
    aircraft_category text,
    stand text,
    sequence text,
    state text,
    cleared bool,
    positionFrequency text,
    position_latitude text,
    position_longitude text,
    position_altitude text,
    tobt text,
    tsat text,
    ttot text,
    ctot text,
    aobt text,
    asat text
)