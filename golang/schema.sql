CREATE TABLE controllers (
  cid VARCHAR(256) PRIMARY KEY,
  airport VARCHAR(256),
  position VARCHAR(256)
);

CREATE TABLE events (
    id VARCHAR(256) PRIMARY KEY,
    type VARCHAR(256),
    timestamp VARCHAR(256),
    cid VARCHAR(256),
    data VARCHAR(256)
);

CREATE TABLE strips (
    id VARCHAR(256) PRIMARY KEY,
    origin VARCHAR(256),
    destination VARCHAR(256),
    alternative VARCHAR(256),
    route VARCHAR(256),
    remarks VARCHAR(256),
    assigned_squawk VARCHAR(256),
    squawk VARCHAR(256),
    sid VARCHAR(256),
    cleared_altitude VARCHAR(256),
    heading VARCHAR(256),
    aircraft_type VARCHAR(256),
    runway VARCHAR(256),
    requested_altitude VARCHAR(256),
    capabilities VARCHAR(256),
    communication_type VARCHAR(256),
    aircraft_category VARCHAR(256),
    stand VARCHAR(256),
    sequence VARCHAR(256),
    state VARCHAR(256),
    cleared INT(1),
    positionFrequency VARCHAR(256),
    position_latitude VARCHAR(256),
    position_longitude VARCHAR(256),
    position_altitude VARCHAR(256),
    TOBT VARCHAR(256),
    TSAT VARCHAR(256),
    TTOT VARCHAR(256),
    CTOT VARCHAR(256),
    AOBT VARCHAR(256),
    ASAT VARCHAR(256) 
)