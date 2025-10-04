-- name: InsertAirport :exec
INSERT INTO airports (name)
VALUES ($1);

-- name: InsertController :exec
INSERT INTO controllers (callsign, session, position, cid, last_seen_euroscope, last_seen_frontend)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: BulkInsertControllers :copyfrom
INSERT INTO controllers (callsign, session, position)
VALUES ($1, $2, $3);

-- name: SetControllerPosition :execrows
UPDATE controllers
SET position = $1
WHERE callsign = $2 AND session = $3;

-- name: SetControllerEuroscopeSeen :execrows
UPDATE controllers
SET last_seen_euroscope = $1
WHERE cid = @cid::text AND session = $2;

-- name: SetControllerFrontendSeen :execrows
UPDATE controllers
SET last_seen_frontend = $1
WHERE cid = @cid::text AND session = $2;

-- name: SetControllerCid :execrows
UPDATE controllers
SET cid = $1
WHERE callsign = $2 AND session = $3;

-- name: GetControllerByCid :one
SELECT *
FROM controllers
WHERE cid = @cid::text LIMIT 1;

-- name: ListControllers :many
SELECT *
FROM controllers
WHERE session = $1
ORDER BY callsign;

-- name: RemoveController :execrows
DELETE
FROM controllers
WHERE callsign = $1 AND session = $2;

-- name: GetController :one
SELECT *
FROM controllers
WHERE callsign = $1 AND session = $2;

-- name: ListSessionsByAirport :many
SELECT *
FROM sessions
WHERE airport = $1;

-- name: DeleteSession :execrows
DELETE
FROM sessions
WHERE id = $1;

-- name: InsertSession :one
INSERT INTO sessions (name, airport)
VALUES ($1, $2) RETURNING id;

-- name: GetSession :one
SELECT *
FROM sessions
WHERE airport = $1
  AND name = $2;

-- name: GetSessionById :one
SELECT *
FROM sessions
WHERE id = $1;

-- name: GetExpiredSessions :many
SELECT id
FROM sessions
WHERE NOT EXISTS (SELECT 1 FROM controllers WHERE last_seen_euroscope > @expired_time);

-- name: InsertStrip :exec
INSERT INTO strips (version, callsign, session, origin, destination, alternative, route, remarks, assigned_squawk,
                    squawk, sid, cleared_altitude, heading, aircraft_type, runway, requested_altitude, capabilities,
                    communication_type, aircraft_category, stand, sequence, state, cleared, owner, bay,
                    position_latitude, position_longitude, position_altitude, tobt, eobt)
VALUES (1, $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23,
        $24, $25, $26, $27, $28, $29);

-- name: UpdateStrip :execrows
UPDATE strips
SET (version, origin, destination, alternative, route, remarks, assigned_squawk, squawk, sid, cleared_altitude, heading, aircraft_type, runway, requested_altitude, capabilities, communication_type, aircraft_category, stand, sequence, state, cleared, owner, bay, position_latitude, position_longitude, position_altitude, tobt, eobt
        ) = (
             version + 1, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22,
             $23, $24, $25, $26, $27, $28, $29)
WHERE callsign = $1 AND session = $2;

-- name: GetStrip :one
SELECT *
FROM strips
WHERE callsign = $1 AND session = $2;

-- name: ListStrips :many
SELECT *
FROM strips
WHERE session = $1
ORDER BY callsign;

-- name: UpdateStripSquawkByID :execrows
UPDATE strips
SET squawk  = $1,
    version = version + 1
WHERE callsign = $2 AND session = $3 AND (version = sqlc.narg('version') OR sqlc.narg('version') IS NULL);

-- name: UpdateStripAssignedSquawkByID :execrows
UPDATE strips
SET assigned_squawk = $1,
    version         = version + 1
WHERE callsign = $2 AND session = $3 AND (version = sqlc.narg('version') OR sqlc.narg('version') IS NULL);

-- name: UpdateStripClearedAltitudeByID :execrows
UPDATE strips
SET cleared_altitude = $1,
    version          = version + 1
WHERE callsign = $2 AND session = $3 AND (version = sqlc.narg('version') OR sqlc.narg('version') IS NULL);

-- name: UpdateStripRequestedAltitudeByID :execrows
UPDATE strips
SET requested_altitude = $1,
    version            = version + 1
WHERE callsign = $2 AND session = $3 AND (version = sqlc.narg('version') OR sqlc.narg('version') IS NULL);

-- name: UpdateStripCommunicationTypeByID :execrows
UPDATE strips
SET communication_type = $1,
    version            = version + 1
WHERE callsign = $2 AND session = $3 AND (version = sqlc.narg('version') OR sqlc.narg('version') IS NULL);

-- name: UpdateStripGroundStateByID :execrows
UPDATE strips
SET state   = $1,
    bay     = $2,
    version = version + 1
WHERE callsign = $3 AND session = $4 AND (version = sqlc.narg('version') OR sqlc.narg('version') IS NULL);

-- name: UpdateStripClearedFlagByID :execrows
UPDATE strips
SET cleared = $1,
    bay     = $2,
    version = version + 1
WHERE callsign = $3 AND session = $4 AND (version = sqlc.narg('version') OR sqlc.narg('version') IS NULL);

-- name: UpdateStripAircraftPositionByID :execrows
UPDATE strips
SET position_latitude  = $1,
    position_longitude = $2,
    position_altitude  = $3,
    bay                = $4
WHERE callsign = $5 AND session = $6 AND (version = sqlc.narg('version') OR sqlc.narg('version') IS NULL);

-- name: UpdateStripHeadingByID :execrows
UPDATE strips
SET heading = $1,
    version = version + 1
WHERE callsign = $2 AND session = $3 AND (version = sqlc.narg('version') OR sqlc.narg('version') IS NULL);

-- name: UpdateStripStandByID :execrows
UPDATE strips
SET stand   = $1,
    version = version + 1
WHERE callsign = $2 AND session = $3 AND (version = sqlc.narg('version') OR sqlc.narg('version') IS NULL);

-- name: RemoveStripByID :exec
DELETE
FROM strips
WHERE callsign = $1 AND session = $2;

-- name: ListStripsByOrigin :many
SELECT *
FROM strips
WHERE origin = $1 AND session = $2
ORDER BY callsign;

-- name: SetStripOwner :execrows
UPDATE strips
SET owner   = $1,
    version = version + 1
WHERE callsign = $2 AND session = $3 AND version = $4;

-- name: CreateCoordination :one
INSERT INTO coordinations (session, strip_id, from_position, to_position)
VALUES ($1, $2, $3, $4) RETURNING *;

-- name: ListCoordinationsBySession :many
SELECT *
FROM coordinations
WHERE session = $1
ORDER BY coordinated_at DESC;

-- name: DeleteCoordinationByID :execrows
DELETE
FROM coordinations
WHERE id = $1;

-- name: GetCoordinationByStripID :one
SELECT *
FROM coordinations
WHERE session = $1 AND strip_id = $2;

-- name: GetCoordinationByStripCallsign :one
SELECT c.*
FROM coordinations c
         JOIN strips s ON s.id = c.strip_id
WHERE s.session = $1
  AND s.callsign = $2;

-- name: GetSectorOwners :many
SELECT * FROM sector_owners
WHERE session = $1;

-- name: GetControllers :many
SELECT * FROM controllers
WHERE session = $1;

-- name: UpdateActiveRunways :exec
UPDATE sessions SET active_runways = $2 WHERE id = $1;