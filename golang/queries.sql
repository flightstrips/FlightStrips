-- name: InsertAirport :exec
INSERT INTO airports (name) VALUES ($1);

-- name: InsertController :exec
INSERT INTO controllers (
    callsign, session, airport, position, cid, last_seen_euroscope, last_seen_frontend
) VALUES (
             $1, $2, $3, $4, $5, $6, $7
         );

-- name: BulkInsertControllers :copyfrom
INSERT INTO controllers (
    callsign, session, airport, position
) VALUES (
             $1, $2, $3, $4
         );

-- name: SetControllerPosition :execrows
UPDATE controllers SET position = $1 WHERE callsign = $2 AND session = $3;

-- name: SetControllerEuroscopeSeen :execrows
UPDATE controllers SET last_seen_euroscope = $1 WHERE callsign = $2 AND session = $3;

-- name: ListControllers :many
SELECT * FROM controllers WHERE session = $1 ORDER BY callsign;

-- name: RemoveController :execrows
DELETE FROM controllers WHERE callsign = $1 AND session = $2;

-- name: GetController :one
SELECT * FROM controllers WHERE callsign = $1 AND session = $2;

-- name: ListSessionsByAirport :many
SELECT * FROM sessions WHERE airport = $1;

-- name: DeleteSession :execrows
DELETE FROM sessions WHERE id = $1;

-- name: InsertSession :one
INSERT INTO sessions (name, airport) VALUES ($1, $2) RETURNING id;

-- name: GetSession :one
SELECT * FROM sessions WHERE airport = $1 AND name = $2;

-- name: GetExpiredSessions :many
SELECT id FROM sessions WHERE NOT EXISTS (SELECT 1 FROM controllers WHERE last_seen_euroscope < @expired_time);

-- name: InsertStrip :exec
INSERT INTO strips (version, callsign, session, origin, destination, alternative, route, remarks, assigned_squawk, squawk, sid, cleared_altitude, heading, aircraft_type, runway, requested_altitude, capabilities, communication_type, aircraft_category, stand, sequence, state, cleared, owner, position_latitude, position_longitude, position_altitude, tobt
) VALUES (
    1, $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27);

-- name: UpdateStripSquawkByID :execrows
UPDATE strips SET squawk = $1, version = version + 1 WHERE callsign = $2 AND session = $3 AND (version = sqlc.narg('version') OR sqlc.narg('version') IS NULL);

-- name: UpdateStripAssignedSquawkByID :execrows
UPDATE strips SET assigned_squawk = $1, version = version + 1 WHERE callsign = $2 AND session = $3 AND (version = sqlc.narg('version') OR sqlc.narg('version') IS NULL);

-- name: UpdateStripClearedAltitudeByID :execrows
UPDATE strips SET cleared_altitude = $1, version = version + 1 WHERE callsign = $2 AND session = $3 AND (version = sqlc.narg('version') OR sqlc.narg('version') IS NULL);

-- name: UpdateStripRequestedAltitudeByID :execrows
UPDATE strips SET requested_altitude = $1, version = version + 1 WHERE callsign = $2 AND session = $3 AND (version = sqlc.narg('version') OR sqlc.narg('version') IS NULL);

-- name: UpdateStripCommunicationTypeByID :execrows
UPDATE strips SET communication_type = $1, version = version + 1 WHERE callsign = $2 AND session = $3 AND (version = sqlc.narg('version') OR sqlc.narg('version') IS NULL); 

-- name: UpdateStripGroundStateByID :execrows
UPDATE strips SET state = $1, version = version + 1 WHERE callsign = $2 AND session = $3 AND (version = sqlc.narg('version') OR sqlc.narg('version') IS NULL);

-- name: UpdateStripClearedFlagByID :execrows
UPDATE strips SET cleared = $1, version = version + 1 WHERE callsign = $2 AND session = $3 AND (version = sqlc.narg('version') OR sqlc.narg('version') IS NULL); 

-- name: UpdateStripAircraftPositionByID :execrows
UPDATE strips SET position_latitude = $1, position_longitude = $2, position_altitude = $3 WHERE callsign = $4 AND session = $5 AND (version = sqlc.narg('version') OR sqlc.narg('version') IS NULL);

-- name: UpdateStripHeadingByID :execrows
UPDATE strips SET heading = $1, version = version + 1 WHERE callsign = $2 AND session = $3 AND (version = sqlc.narg('version') OR sqlc.narg('version') IS NULL);

-- name: UpdateStripStandByID :execrows
UPDATE strips SET stand = $1, version = version + 1 WHERE callsign = $2 AND session = $3 AND (version = sqlc.narg('version') OR sqlc.narg('version') IS NULL);

-- name: RemoveStripByID :exec
DELETE FROM strips WHERE callsign = $1 AND session = $2;

-- name: ListStripsByOrigin :many
SELECT * FROM strips WHERE origin = $1 AND session = $2 ORDER BY callsign;

