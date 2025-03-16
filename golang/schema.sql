CREATE TABLE IF NOT EXISTS controllers (
    id SERIAL PRIMARY KEY,
    session integer references sessions(id) NOT NULL,
    callsign varchar NOT NULL,
    airport varchar(4) NOT NULL,
    position varchar(7) NOT NULL,
    cid varchar(10),
    last_seen_euroscope timestamp,
    last_seen_frontend timestamp
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_controllers_session_callsign ON controllers (session, callsign);

CREATE TABLE IF NOT EXISTS sessions (
    id SERIAL PRIMARY KEY,
    name text NOT NULL,
    airport varchar(4) REFERENCES airports(name) NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_sessions_name_airport ON sessions (name, airport);

CREATE TABLE IF NOT EXISTS strips (
    id SERIAL PRIMARY KEY,
    callsign varchar NOT NULL,
    session integer references sessions(id) NOT NULL,
    origin varchar(4) NOT NULL,
    destination varchar(4) NOT NULL,
    alternative varchar(4),
    route varchar,
    remarks varchar,
    assigned_squawk varchar,
    squawk varchar,
    sid varchar,
    cleared_altitude varchar,
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
    position_latitude varchar,
    position_longitude varchar,
    position_altitude varchar,
    tobt varchar,
    tsat varchar,
    ttot varchar,
    ctot varchar,
    aobt varchar,
    asat varchar
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_strips_session_callsign ON strips (session, callsign);

CREATE TABLE IF NOT EXISTS airports (
    name varchar(4) PRIMARY KEY
);

CREATE TABLE IF NOT EXISTS airport_master_orders (
    id SERIAL PRIMARY KEY,
    airport varchar(4) REFERENCES airports(name),
    position varchar(7) NOT NULL,
    priority integer NOT NULL
);

INSERT INTO airports (name) VALUES ('EKCH');

