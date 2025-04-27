CREATE TABLE IF NOT EXISTS airports (
    name varchar(4) PRIMARY KEY
);

CREATE TABLE IF NOT EXISTS sessions (
    id SERIAL PRIMARY KEY,
    name text NOT NULL,
    airport varchar(4) REFERENCES airports(name) ON DELETE CASCADE NOT NULL
);

CREATE TABLE IF NOT EXISTS airport_master_orders (
    id SERIAL PRIMARY KEY,
    airport varchar(4) REFERENCES airports(name) ON DELETE CASCADE NOT NULL,
    position varchar(7) NOT NULL,
    priority integer NOT NULL
);

CREATE TABLE IF NOT EXISTS controllers (
    id SERIAL PRIMARY KEY,
    session integer references sessions(id) ON DELETE CASCADE NOT NULL,
    callsign varchar NOT NULL,
    airport varchar(4) NOT NULL,
    position varchar(7) NOT NULL,
    cid varchar(10),
    last_seen_euroscope timestamp,
    last_seen_frontend timestamp
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_controllers_session_callsign ON controllers (session, callsign);


CREATE UNIQUE INDEX IF NOT EXISTS ux_sessions_name_airport ON sessions (name, airport);

CREATE TABLE IF NOT EXISTS strips (
    id SERIAL PRIMARY KEY,
    version integer NOT NULL,
    callsign varchar NOT NULL,
    session integer references sessions(id) ON DELETE CASCADE NOT NULL,
    origin varchar(4) NOT NULL,
    destination varchar(4) NOT NULL,
    alternative varchar(4),
    route varchar,
    remarks varchar,
    assigned_squawk varchar,
    squawk varchar,
    sid varchar,
    cleared_altitude integer,
    heading integer,
    aircraft_type varchar,
    runway varchar,
    requested_altitude integer,
    capabilities varchar,
    communication_type varchar,
    aircraft_category varchar,
    stand varchar,
    sequence integer,
    state varchar,
    cleared bool,
    owner varchar,
    bay varchar,
    position_latitude double precision,
    position_longitude double precision,
    position_altitude integer,
    tobt varchar,
    tsat varchar,
    ttot varchar,
    ctot varchar,
    aobt varchar,
    asat varchar
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_strips_session_callsign ON strips (session, callsign);

