CREATE TABLE IF NOT EXISTS controllers (
    callsign varchar PRIMARY KEY,
    airport varchar(4) NOT NULL,
    position varchar(7) NOT NULL,
    master boolean NOT NULL,
    connected boolean NOT NULL
);



CREATE TABLE IF NOT EXISTS events (
    id SERIAL PRIMARY KEY,
    type varchar,
    timestamp varchar,
    cid varchar,
    data varchar
);

CREATE TABLE IF NOT EXISTS strips (
    id varchar PRIMARY KEY,
    origin varchar,
    destination varchar,
    alternative varchar,
    route varchar,
    remarks varchar,
    assigned_squawk varchar,
    squawk varchar,
    sid varchar,
    cleared_altitude varchar,
    heading varchar,
    aircraft_type varchar,
    runway varchar,
    requested_altitude varchar,
    capabilities varchar,
    communication_type varchar,
    aircraft_category varchar,
    stand varchar,
    sequence varchar,
    state varchar,
    cleared bool,
    positionFrequency varchar,
    position_latitude varchar,
    position_longitude varchar,
    position_altitude varchar,
    tobt varchar,
    tsat varchar,
    ttot varchar,
    ctot varchar,
    aobt varchar,
    asat varchar
)
